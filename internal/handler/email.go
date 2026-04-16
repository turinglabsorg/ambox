package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/turinglabs/ambox/internal/middleware"
)

func (h *Handler) DeleteEmail(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	emailID := r.PathValue("id")

	if emailID == "" {
		writeError(w, http.StatusBadRequest, "email id required")
		return
	}

	email, err := h.store.GetEmail(r.Context(), emailID, agent.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}

	if h.gcs != nil {
		for _, att := range email.Attachments {
			h.gcs.Delete(r.Context(), att.GCSPath)
		}
	}

	if err := h.store.DeleteEmail(r.Context(), emailID, agent.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete email")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type MoveRequest struct {
	Folder string `json:"folder"`
}

func (h *Handler) MoveEmail(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	emailID := r.PathValue("id")

	var req MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Folder == "" {
		writeError(w, http.StatusBadRequest, "folder required")
		return
	}

	if err := h.store.MoveEmail(r.Context(), emailID, agent.ID, req.Folder); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "moved", "folder": req.Folder})
}

func (h *Handler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	emailID := r.PathValue("id")
	filename := r.PathValue("filename")

	email, err := h.store.GetEmail(r.Context(), emailID, agent.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}

	var found bool
	for _, att := range email.Attachments {
		if att.Filename == filename {
			found = true
			if h.gcs == nil {
				writeError(w, http.StatusInternalServerError, "storage not configured")
				return
			}
			data, err := h.gcs.Download(r.Context(), att.GCSPath)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to download attachment")
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+att.Filename+".enc\"")
			w.Header().Set("X-Ambox-Wrapped-Key", att.WrappedKey)
			w.Header().Set("X-Ambox-Nonce-Index", json.Number(fmt.Sprintf("%d", att.NonceIndex)).String())
			w.Write(data)
			return
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "attachment not found")
	}
}
