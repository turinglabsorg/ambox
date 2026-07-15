package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/middleware"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/store"
)

const (
	maxOutgoingAttachments      = 20
	maxOutgoingEmailPayloadSize = 35 * 1024 * 1024
)

type SendRequest struct {
	To          []string                `json:"to"`
	CC          []string                `json:"cc,omitempty"`
	BCC         []string                `json:"bcc,omitempty"`
	Subject     string                  `json:"subject"`
	BodyHTML    string                  `json:"body_html,omitempty"`
	BodyText    string                  `json:"body_text,omitempty"`
	ReplyTo     string                  `json:"reply_to,omitempty"`
	Attachments []SendAttachmentRequest `json:"attachments,omitempty"`
}

type SendAttachmentRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content"`
}

type preparedAttachment struct {
	Filename    string
	ContentType string
	Content     string
	Data        []byte
}

type SendResponse struct {
	MessageID string `json:"message_id"`
	ResendID  string `json:"resend_id"`
	Email     string `json:"email"`
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		writeError(w, http.StatusUnauthorized, "agent authentication required")
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.To) == 0 {
		writeError(w, http.StatusBadRequest, "at least one recipient required")
		return
	}
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject required")
		return
	}

	attachments, err := prepareOutgoingAttachments(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(attachments) > 0 && h.gcs == nil {
		writeError(w, http.StatusServiceUnavailable, "attachment storage is not configured")
		return
	}

	pubKey, err := crypto.ParsePublicKey([]byte(agent.PublicKeyPEM))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid agent public key")
		return
	}

	body := req.BodyHTML
	if body == "" {
		body = req.BodyText
	}

	enc, err := crypto.EncryptEmail(pubKey, req.Subject, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt email")
		return
	}

	now := time.Now().UTC()
	msgID := fmt.Sprintf("msg_%s", generateShortID())
	storedAttachments := make([]store.Attachment, 0, len(attachments))
	uploadedPaths := make([]string, 0, len(attachments))
	resendAttachments := make([]resend.Attachment, 0, len(attachments))

	for index, attachment := range attachments {
		nonceIndex := index + 3
		encAttachment, err := crypto.EncryptAttachment(pubKey, attachment.Data, nonceIndex)
		if err != nil {
			h.cleanupUploadedAttachments(r, uploadedPaths)
			writeError(w, http.StatusInternalServerError, "failed to encrypt attachment")
			return
		}

		ciphertext, err := base64.StdEncoding.DecodeString(encAttachment.Ciphertext)
		if err != nil {
			h.cleanupUploadedAttachments(r, uploadedPaths)
			writeError(w, http.StatusInternalServerError, "failed to prepare encrypted attachment")
			return
		}

		gcsPath := fmt.Sprintf("%s/%s/%s.enc", agent.ID, msgID, attachment.Filename)
		if err := h.gcs.Upload(r.Context(), gcsPath, ciphertext, "application/octet-stream"); err != nil {
			h.cleanupUploadedAttachments(r, uploadedPaths)
			writeError(w, http.StatusInternalServerError, "failed to store encrypted attachment")
			return
		}
		uploadedPaths = append(uploadedPaths, gcsPath)

		storedAttachments = append(storedAttachments, store.Attachment{
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			SizeBytes:   int64(len(attachment.Data)),
			GCSPath:     gcsPath,
			WrappedKey:  encAttachment.WrappedKey,
			NonceIndex:  nonceIndex,
		})
		resendAttachments = append(resendAttachments, resend.Attachment{
			Filename: attachment.Filename,
			Content:  attachment.Content,
		})
	}

	from := agent.Email
	if agent.DisplayName != "" {
		from = fmt.Sprintf("%s <%s>", agent.DisplayName, agent.Email)
	}

	resendResp, err := h.resend.SendEmail(r.Context(), &resend.SendRequest{
		From:        from,
		To:          req.To,
		CC:          req.CC,
		BCC:         req.BCC,
		Subject:     req.Subject,
		HTML:        req.BodyHTML,
		Text:        req.BodyText,
		ReplyTo:     req.ReplyTo,
		Attachments: resendAttachments,
	})
	if err != nil {
		h.cleanupUploadedAttachments(r, uploadedPaths)
		writeError(w, http.StatusBadGateway, "failed to send email: "+err.Error())
		return
	}

	var expiresAt *time.Time
	if agent.TTLSeconds > 0 {
		t := now.Add(time.Duration(agent.TTLSeconds) * time.Second)
		expiresAt = &t
	}

	email := &store.Email{
		ID:               msgID,
		AgentID:          agent.ID,
		Folder:           "sent",
		From:             agent.Email,
		To:               req.To,
		SubjectEncrypted: enc.SubjectEncrypted,
		BodyEncrypted:    enc.BodyEncrypted,
		WrappedKey:       enc.WrappedKey,
		ResendID:         resendResp.ID,
		Attachments:      storedAttachments,
		ReceivedAt:       now,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
	}

	if err := h.store.InsertEmail(r.Context(), email); err != nil {
		h.cleanupUploadedAttachments(r, uploadedPaths)
		writeError(w, http.StatusInternalServerError, "failed to store email")
		return
	}

	writeJSON(w, http.StatusOK, SendResponse{
		MessageID: msgID,
		ResendID:  resendResp.ID,
		Email:     agent.Email,
	})
}

func prepareOutgoingAttachments(req *SendRequest) ([]preparedAttachment, error) {
	if len(req.Attachments) > maxOutgoingAttachments {
		return nil, fmt.Errorf("a maximum of %d attachments is allowed", maxOutgoingAttachments)
	}

	payloadSize := len(req.Subject) + len(req.BodyHTML) + len(req.BodyText)
	seenFilenames := make(map[string]struct{}, len(req.Attachments))
	attachments := make([]preparedAttachment, 0, len(req.Attachments))

	for _, attachment := range req.Attachments {
		filename := strings.TrimSpace(attachment.Filename)
		if filename == "" {
			return nil, fmt.Errorf("attachment filename is required")
		}
		if len(filename) > 255 {
			return nil, fmt.Errorf("attachment filename must be at most 255 characters")
		}
		if strings.ContainsAny(filename, `/\\`) || filename == "." || filename == ".." {
			return nil, fmt.Errorf("attachment filename must not contain a path")
		}

		filenameKey := strings.ToLower(filename)
		if _, exists := seenFilenames[filenameKey]; exists {
			return nil, fmt.Errorf("attachment filenames must be unique")
		}
		seenFilenames[filenameKey] = struct{}{}

		if attachment.Content == "" {
			return nil, fmt.Errorf("attachment content is required")
		}
		data, err := base64.StdEncoding.DecodeString(attachment.Content)
		if err != nil {
			return nil, fmt.Errorf("attachment %q content must be valid base64", filename)
		}
		content := base64.StdEncoding.EncodeToString(data)
		payloadSize += len(content)
		if payloadSize > maxOutgoingEmailPayloadSize {
			return nil, fmt.Errorf("email content and attachments must not exceed 35 MiB after base64 encoding")
		}

		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		} else if _, _, err := mime.ParseMediaType(contentType); err != nil {
			return nil, fmt.Errorf("attachment %q has an invalid content type", filename)
		}

		attachments = append(attachments, preparedAttachment{
			Filename:    filename,
			ContentType: contentType,
			Content:     content,
			Data:        data,
		})
	}

	return attachments, nil
}

func (h *Handler) cleanupUploadedAttachments(r *http.Request, paths []string) {
	if h.gcs == nil {
		return
	}
	for _, path := range paths {
		_ = h.gcs.Delete(r.Context(), path)
	}
}
