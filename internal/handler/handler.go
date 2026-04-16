package handler

import (
	"encoding/json"
	"net/http"

	"github.com/turinglabs/ambox/internal/classify"
	"github.com/turinglabs/ambox/internal/forward"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/config"
	"github.com/turinglabs/ambox/internal/storage"
	"github.com/turinglabs/ambox/internal/store"
)

type Handler struct {
	store      *store.Store
	resend     *resend.Client
	classifier *classify.Classifier
	forwarder  *forward.Forwarder
	gcs        *storage.GCS
	cfg        *config.Config
}

func New(s *store.Store, r *resend.Client, cl *classify.Classifier, f *forward.Forwarder, g *storage.GCS, cfg *config.Config) *Handler {
	return &Handler{
		store:      s,
		resend:     r,
		classifier: cl,
		forwarder:  f,
		gcs:        g,
		cfg:        cfg,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
