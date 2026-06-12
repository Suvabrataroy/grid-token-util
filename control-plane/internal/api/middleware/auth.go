// Package middleware provides HTTP middleware for authentication, auditing, and
// rate limiting.
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/security"
)

type contextKey int

const (
	ctxOrgUnitID contextKey = iota
	ctxWorkerID
	ctxAPIKeyID
	ctxScopes
)

// APIKeyStore is the minimal interface the auth middleware requires.
type APIKeyStore interface {
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
}

// AuditWriter is the minimal interface for writing audit events.
type AuditWriter interface {
	WriteAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
}

// Auth returns a middleware that validates Bearer tokens using Argon2id
// verification.  It injects orgUnitID and (optionally) workerID into the
// request context.
func Auth(store APIKeyStore, auditor AuditWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeUnauthorized(w, "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeUnauthorized(w, "malformed Authorization header")
				return
			}

			plaintext := parts[1]
			// The prefix is "gw_" + first 8 chars of the base64-encoded key.
			if len(plaintext) < len(security.KeyPrefix)+security.KeyPrefixLen {
				writeUnauthorized(w, "invalid key format")
				return
			}
			prefix := plaintext[:len(security.KeyPrefix)+security.KeyPrefixLen]

			apiKey, err := store.GetAPIKeyByPrefix(r.Context(), prefix)
			if err != nil {
				log.Warn().Err(err).Str("prefix", prefix).Msg("auth: key lookup failed")
				writeUnauthorized(w, "invalid or expired key")
				writeAuthAuditEvent(r, auditor, domain.ActorTypeAPIKey, prefix, uuid.Nil, "auth_failed", "invalid_key")
				return
			}

			if !security.VerifyAPIKey(plaintext, apiKey.KeyHash) {
				log.Warn().Str("key_id", apiKey.ID.String()).Msg("auth: key verification failed")
				writeUnauthorized(w, "invalid credentials")
				// OrgUnitID is now populated because we found the key record.
				writeAuthAuditEvent(r, auditor, domain.ActorTypeAPIKey, apiKey.ID.String(), apiKey.OrgUnitID, "auth_failed", "bad_credentials")
				return
			}

			// Best-effort last-used update — never block on this.
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if err := store.UpdateAPIKeyLastUsed(bgCtx, apiKey.ID); err != nil {
					log.Warn().Err(err).Msg("auth: update last used")
				}
			}()

			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxOrgUnitID, apiKey.OrgUnitID)
			ctx = context.WithValue(ctx, ctxWorkerID, apiKey.WorkerID)
			ctx = context.WithValue(ctx, ctxAPIKeyID, apiKey.ID)
			ctx = context.WithValue(ctx, ctxScopes, apiKey.Scopes)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ScopeRequired returns a middleware that enforces the presence of a specific
// scope string on the authenticated API key.
func ScopeRequired(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes, ok := r.Context().Value(ctxScopes).([]string)
			if !ok {
				writeForbidden(w, "no scopes on token")
				return
			}
			for _, s := range scopes {
				if s == scope || s == "admin" {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeForbidden(w, "insufficient scope: "+scope)
		})
	}
}

// OrgUnitIDFromContext extracts the authenticated org unit ID from the context.
func OrgUnitIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxOrgUnitID).(uuid.UUID)
	return v, ok
}

// WorkerIDFromContext extracts the authenticated worker ID from the context.
// Returns nil if the key is not associated with a specific worker.
func WorkerIDFromContext(ctx context.Context) (*uuid.UUID, bool) {
	v, ok := ctx.Value(ctxWorkerID).(*uuid.UUID)
	return v, ok
}

// APIKeyIDFromContext extracts the API key ID from the context.
func APIKeyIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxAPIKeyID).(uuid.UUID)
	return v, ok
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="grid-control-plane"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"` + jsonEscape(msg) + `"}`))
}

func writeForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"forbidden","message":"` + jsonEscape(msg) + `"}`))
}

func writeAuthAuditEvent(r *http.Request, auditor AuditWriter, actorType domain.ActorType, actorID string, orgUnitID uuid.UUID, action, detail string) {
	if auditor == nil {
		return
	}
	ev := &domain.AuditEvent{
		ID:           uuid.New(),
		OrgUnitID:    orgUnitID,
		ActorType:    actorType,
		ActorID:      actorID,
		Action:       action,
		ResourceType: "api_key",
		Details:      map[string]any{"reason": detail, "path": r.URL.Path},
		IPAddress:    realIP(r),
		OccurredAt:   time.Now().UTC(),
	}
	bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := auditor.WriteAuditEvent(bgCtx, ev); err != nil {
		log.Warn().Err(err).Msg("auth: write audit event")
	}
}

// jsonEscape escapes a string for safe embedding inside a JSON string value.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// realIP extracts the originating IP address, respecting X-Forwarded-For.
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Strip port suffix.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
