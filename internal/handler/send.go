package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/middleware"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/store"
)

type SendRequest struct {
	To       []string `json:"to"`
	CC       []string `json:"cc,omitempty"`
	BCC      []string `json:"bcc,omitempty"`
	Subject  string   `json:"subject"`
	BodyHTML string   `json:"body_html,omitempty"`
	BodyText string   `json:"body_text,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
}

type SendResponse struct {
	MessageID string `json:"message_id"`
	ResendID  string `json:"resend_id"`
	Email     string `json:"email"`
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

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

	from := agent.Email
	if agent.DisplayName != "" {
		from = fmt.Sprintf("%s <%s>", agent.DisplayName, agent.Email)
	}

	resendResp, err := h.resend.SendEmail(r.Context(), &resend.SendRequest{
		From:    from,
		To:      req.To,
		CC:      req.CC,
		BCC:     req.BCC,
		Subject: req.Subject,
		HTML:    req.BodyHTML,
		Text:    req.BodyText,
		ReplyTo: req.ReplyTo,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to send email: "+err.Error())
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
		ReceivedAt:       now,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
	}

	if err := h.store.InsertEmail(r.Context(), email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store email")
		return
	}

	writeJSON(w, http.StatusOK, SendResponse{
		MessageID: msgID,
		ResendID:  resendResp.ID,
		Email:     agent.Email,
	})
}
