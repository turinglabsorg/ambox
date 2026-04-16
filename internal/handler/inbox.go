package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/turinglabs/ambox/internal/middleware"
	"github.com/turinglabs/ambox/internal/store"
)

type InboxResponse struct {
	Emails  []store.Email `json:"emails"`
	HasMore bool          `json:"has_more"`
}

func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "inbox"
	}

	var since *time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since format, use RFC3339")
			return
		}
		since = &t
	}

	limit := int64(50)
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.ParseInt(l, 10, 64)
		if err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	cursor := r.URL.Query().Get("cursor")

	emails, err := h.store.ListEmails(r.Context(), store.InboxQuery{
		AgentID: agent.ID,
		Folder:  folder,
		Since:   since,
		Cursor:  cursor,
		Limit:   limit + 1,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch emails")
		return
	}

	hasMore := int64(len(emails)) > limit
	if hasMore {
		emails = emails[:limit]
	}

	writeJSON(w, http.StatusOK, InboxResponse{
		Emails:  emails,
		HasMore: hasMore,
	})
}
