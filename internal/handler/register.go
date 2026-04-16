package handler

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/store"
)

var agentIDRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)

type RegisterRequest struct {
	AgentID     string `json:"agent_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	TTLSeconds  int64  `json:"ttl_seconds,omitempty"`
}

type RegisterResponse struct {
	AgentID       string `json:"agent_id"`
	Email         string `json:"email"`
	APIKey        string `json:"api_key"`
	PrivateKeyPEM string `json:"private_key_pem"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		req.AgentID = generateShortID()
	}

	if !agentIDRegex.MatchString(req.AgentID) {
		writeError(w, http.StatusBadRequest, "agent_id must be 3-32 chars, lowercase alphanumeric and hyphens")
		return
	}

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate keypair")
		return
	}

	apiKey, err := crypto.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate api key")
		return
	}

	apiKeyHash, err := crypto.HashAPIKey(apiKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash api key")
		return
	}

	now := time.Now().UTC()
	agent := &store.Agent{
		ID:           req.AgentID,
		Email:        fmt.Sprintf("%s@%s", req.AgentID, h.cfg.EmailDomain),
		DisplayName:  req.DisplayName,
		APIKeyHash:   apiKeyHash,
		APIKeyPrefix: crypto.APIKeyPrefix(apiKey),
		PublicKeyPEM: string(kp.PublicKeyPEM),
		WebhookURL:   req.WebhookURL,
		TTLSeconds:   req.TTLSeconds,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.store.CreateAgent(r.Context(), agent); err != nil {
		log.Printf("create agent error: %v", err)
		writeError(w, http.StatusConflict, "agent_id already taken: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, RegisterResponse{
		AgentID:       agent.ID,
		Email:         agent.Email,
		APIKey:        apiKey,
		PrivateKeyPEM: string(kp.PrivateKeyPEM),
	})
}

func generateShortID() string {
	b := make([]byte, 4)
	crand.Read(b)
	return fmt.Sprintf("%x", b)
}
