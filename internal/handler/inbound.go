package handler

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/forward"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/store"
)

func (h *Handler) Inbound(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	if err := resend.VerifyWebhookSignature(h.cfg.ResendWebhookSecret, r.Header, body); err != nil {
		log.Printf("webhook signature verification failed: %v", err)
		writeError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}

	payload, err := resend.ParseInboundWebhook(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook payload")
		return
	}

	if payload.Type != "email.received" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("inbound email_id=%s from=%s to=%v subject=%s",
		payload.Data.EmailID, payload.Data.From, payload.Data.To, payload.Data.Subject)

	// Fetch full email content via Resend API
	emailContent, err := h.resend.GetInboundEmail(r.Context(), payload.Data.EmailID)
	if err != nil {
		log.Printf("failed to fetch inbound email content: %v", err)
		writeError(w, http.StatusBadGateway, "failed to fetch email content")
		return
	}

	for _, to := range payload.Data.To {
		addr := strings.ToLower(strings.TrimSpace(to))
		h.processInboundForRecipient(r, addr, &payload.Data, emailContent)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) processInboundForRecipient(r *http.Request, recipientEmail string, data *resend.InboundWebhookData, content *resend.InboundEmailContent) {
	agent, err := h.store.GetAgentByEmail(r.Context(), recipientEmail)
	if err != nil {
		log.Printf("no agent for email %s: %v", recipientEmail, err)
		return
	}

	pubKey, err := crypto.ParsePublicKey([]byte(agent.PublicKeyPEM))
	if err != nil {
		log.Printf("invalid public key for agent %s: %v", agent.ID, err)
		return
	}

	emailBody := content.HTML
	if emailBody == "" {
		emailBody = content.Text
	}

	folder := "inbox"
	if h.classifier != nil {
		bodyPreview := content.Text
		if bodyPreview == "" {
			bodyPreview = content.HTML
		}
		folder = h.classifier.Classify(r.Context(), data.From, data.Subject, bodyPreview)
	}

	enc, err := crypto.EncryptEmail(pubKey, data.Subject, emailBody)
	if err != nil {
		log.Printf("failed to encrypt email for agent %s: %v", agent.ID, err)
		return
	}

	now := time.Now().UTC()
	msgID := fmt.Sprintf("msg_%s", generateShortID())

	var expiresAt *time.Time
	if agent.TTLSeconds > 0 {
		t := now.Add(time.Duration(agent.TTLSeconds) * time.Second)
		expiresAt = &t
	}

	// Fetch attachments via Resend API
	var attachments []store.Attachment
	attList, err := h.resend.ListInboundAttachments(r.Context(), data.EmailID)
	if err != nil {
		log.Printf("failed to list attachments for %s: %v", data.EmailID, err)
	}
	for i, attMeta := range attList {
		nonceIndex := 3 + i

		attData, err := h.resend.DownloadAttachment(r.Context(), attMeta.DownloadURL)
		if err != nil {
			log.Printf("failed to download attachment %s: %v", attMeta.Filename, err)
			continue
		}

		encAtt, err := crypto.EncryptAttachment(pubKey, attData, nonceIndex)
		if err != nil {
			log.Printf("failed to encrypt attachment %s: %v", attMeta.Filename, err)
			continue
		}

		gcsPath := fmt.Sprintf("%s/%s/%s.enc", agent.ID, msgID, attMeta.Filename)
		encBytes, err := base64.StdEncoding.DecodeString(encAtt.Ciphertext)
		if err != nil {
			log.Printf("failed to decode encrypted attachment: %v", err)
			continue
		}

		if h.gcs != nil {
			if err := h.gcs.Upload(r.Context(), gcsPath, encBytes, "application/octet-stream"); err != nil {
				log.Printf("failed to upload attachment to gcs: %v", err)
				continue
			}
		}

		attachments = append(attachments, store.Attachment{
			Filename:    attMeta.Filename,
			ContentType: attMeta.ContentType,
			SizeBytes:   attMeta.Size,
			GCSPath:     gcsPath,
			WrappedKey:  encAtt.WrappedKey,
			NonceIndex:  nonceIndex,
		})
	}

	email := &store.Email{
		ID:               msgID,
		AgentID:          agent.ID,
		Folder:           folder,
		From:             data.From,
		To:               data.To,
		SubjectEncrypted: enc.SubjectEncrypted,
		BodyEncrypted:    enc.BodyEncrypted,
		WrappedKey:       enc.WrappedKey,
		ResendID:         data.EmailID,
		Attachments:      attachments,
		Classification:   folder,
		ReceivedAt:       now,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
	}

	if err := h.store.InsertEmail(r.Context(), email); err != nil {
		log.Printf("failed to store email for agent %s: %v", agent.ID, err)
		return
	}

	log.Printf("stored inbound email %s for agent %s in folder %s", msgID, agent.ID, folder)

	if agent.WebhookURL != "" {
		h.forwarder.NotifyAsync(agent.WebhookURL, agent.WebhookSecret, &forward.Notification{
			Type:       "email.received",
			EmailID:    msgID,
			From:       data.From,
			ReceivedAt: now.Format(time.RFC3339),
			Folder:     folder,
		})
	}
}
