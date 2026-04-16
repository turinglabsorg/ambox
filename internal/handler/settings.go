package handler

import (
	"encoding/json"
	"net/http"

	"github.com/turinglabs/ambox/internal/middleware"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type SettingsRequest struct {
	TTLSeconds  *int64  `json:"ttl_seconds,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	var req SettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := bson.M{}
	if req.TTLSeconds != nil {
		update["ttl_seconds"] = *req.TTLSeconds
	}
	if req.DisplayName != nil {
		update["display_name"] = *req.DisplayName
	}

	if len(update) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	if err := h.store.UpdateAgent(r.Context(), agent.ID, update); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
