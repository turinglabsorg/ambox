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
		addr := normalizeRecipientEmail(to)
		h.processInboundForRecipient(r, addr, &payload.Data, emailContent)
	}

	w.WriteHeader(http.StatusOK)
}

func normalizeRecipientEmail(email string) string {
	addr := strings.ToLower(strings.TrimSpace(email))
	local, domain, ok := strings.Cut(addr, "@")
	if !ok {
		return addr
	}
	if plus := strings.Index(local, "+"); plus > 0 {
		local = local[:plus]
	}
	return local + "@" + domain
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

	// Fetch attachments from both Resend surfaces. Some inbound payloads expose
	// base64 attachment content on the email detail response, while others only
	// expose metadata/download URLs via the attachments endpoint.
	var attachments []store.Attachment
	nextNonceIndex := 3
	seenAttachments := map[string]bool{}

	storeAttachment := func(filename, contentType string, sizeBytes int64, attData []byte) {
		if filename == "" {
			log.Printf("skipping attachment with empty filename for %s", data.EmailID)
			return
		}
		if seenAttachments[filename] {
			return
		}

		nonceIndex := nextNonceIndex
		nextNonceIndex++

		encAtt, err := crypto.EncryptAttachment(pubKey, attData, nonceIndex)
		if err != nil {
			log.Printf("failed to encrypt attachment %s: %v", filename, err)
			return
		}

		gcsPath := fmt.Sprintf("%s/%s/%s.enc", agent.ID, msgID, filename)
		encBytes, err := base64.StdEncoding.DecodeString(encAtt.Ciphertext)
		if err != nil {
			log.Printf("failed to decode encrypted attachment: %v", err)
			return
		}

		if h.gcs != nil {
			if err := h.gcs.Upload(r.Context(), gcsPath, encBytes, "application/octet-stream"); err != nil {
				log.Printf("failed to upload attachment to gcs: %v", err)
				return
			}
		}

		if sizeBytes == 0 {
			sizeBytes = int64(len(attData))
		}
		seenAttachments[filename] = true
		attachments = append(attachments, store.Attachment{
			Filename:    filename,
			ContentType: contentType,
			SizeBytes:   sizeBytes,
			GCSPath:     gcsPath,
			WrappedKey:  encAtt.WrappedKey,
			NonceIndex:  nonceIndex,
		})
	}

	for _, inlineAtt := range content.Attachments {
		if inlineAtt.Content == "" {
			continue
		}
		attData, err := base64.StdEncoding.DecodeString(inlineAtt.Content)
		if err != nil {
			log.Printf("failed to decode inline attachment %s: %v", inlineAtt.Filename, err)
			continue
		}
		storeAttachment(inlineAtt.Filename, inlineAtt.ContentType, int64(len(attData)), attData)
	}

	attList, err := h.resend.ListInboundAttachments(r.Context(), data.EmailID)
	if err != nil {
		log.Printf("failed to list attachments for %s: %v", data.EmailID, err)
	}
	for _, attMeta := range attList {
		attData, err := h.resend.DownloadAttachment(r.Context(), attMeta.DownloadURL)
		if err != nil {
			log.Printf("failed to download attachment %s: %v", attMeta.Filename, err)
			continue
		}
		storeAttachment(attMeta.Filename, attMeta.ContentType, attMeta.Size, attData)
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
