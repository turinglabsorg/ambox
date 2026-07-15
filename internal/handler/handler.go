package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/turinglabs/ambox/internal/classify"
	"github.com/turinglabs/ambox/internal/config"
	"github.com/turinglabs/ambox/internal/forward"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type emailStore interface {
	CreateAgent(context.Context, *store.Agent) error
	GetAgentByEmail(context.Context, string) (*store.Agent, error)
	UpdateAgent(context.Context, string, bson.M) error
	InsertEmail(context.Context, *store.Email) error
	ListEmails(context.Context, store.InboxQuery) ([]store.Email, error)
	GetEmail(context.Context, string, string) (*store.Email, error)
	DeleteEmail(context.Context, string, string) error
	MoveEmail(context.Context, string, string, string) error
}

type emailProvider interface {
	SendEmail(context.Context, *resend.SendRequest) (*resend.SendResponse, error)
	GetInboundEmail(context.Context, string) (*resend.InboundEmailContent, error)
	ListInboundAttachments(context.Context, string) ([]resend.AttachmentMeta, error)
	DownloadAttachment(context.Context, string) ([]byte, error)
}

type attachmentStorage interface {
	Upload(context.Context, string, []byte, string) error
	Download(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

type Handler struct {
	store      emailStore
	resend     emailProvider
	classifier *classify.Classifier
	forwarder  *forward.Forwarder
	gcs        attachmentStorage
	cfg        *config.Config
}

func New(s emailStore, r emailProvider, cl *classify.Classifier, f *forward.Forwarder, g attachmentStorage, cfg *config.Config) *Handler {
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
