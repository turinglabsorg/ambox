package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/turinglabs/ambox/internal/middleware"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type WebhookConfigRequest struct {
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"`
}

func (h *Handler) ConfigureWebhook(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	var req WebhookConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.URL != "" {
		pingBody, _ := json.Marshal(map[string]string{"type": "ping"})
		httpReq, err := http.NewRequestWithContext(r.Context(), "POST", req.URL, bytes.NewReader(pingBody))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid webhook url")
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "webhook url unreachable")
			return
		}
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeError(w, http.StatusUnprocessableEntity, "webhook url returned non-2xx")
			return
		}
	}

	update := bson.M{
		"webhook_url":    req.URL,
		"webhook_secret": req.Secret,
	}

	if err := h.store.UpdateAgent(r.Context(), agent.ID, update); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
