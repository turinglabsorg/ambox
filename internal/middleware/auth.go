package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/store"
)

type contextKey string

const AgentContextKey contextKey = "agent"

func AgentFromContext(ctx context.Context) *store.Agent {
	agent, _ := ctx.Value(AgentContextKey).(*store.Agent)
	return agent
}

func Auth(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			apiKey := strings.TrimPrefix(authHeader, "Bearer ")
			if apiKey == authHeader {
				http.Error(w, `{"error":"invalid authorization format, use Bearer"}`, http.StatusUnauthorized)
				return
			}

			prefix := crypto.APIKeyPrefix(apiKey)
			agent, err := s.GetAgentByPrefix(r.Context(), prefix)
			if err != nil {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			if !crypto.VerifyAPIKey(apiKey, agent.APIKeyHash) {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), AgentContextKey, agent)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
