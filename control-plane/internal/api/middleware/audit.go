package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/domain"
)

// mutatingMethods is the set of HTTP methods that modify state and therefore
// warrant an audit trail.
var mutatingMethods = map[string]struct{}{
	http.MethodPost:   {},
	http.MethodPatch:  {},
	http.MethodPut:    {},
	http.MethodDelete: {},
}

// responseWriter wraps http.ResponseWriter to capture the status code after
// the handler completes.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Status() int {
	if rw.status == 0 {
		return http.StatusOK
	}
	return rw.status
}

// Audit returns a middleware that writes an AuditEvent for every state-changing
// request.  The event is written asynchronously after the response is sent so
// that it does not delay the client.
func Audit(auditor AuditWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := mutatingMethods[r.Method]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(wrapped, r)

			// Capture context values after the handler runs (e.g., route params
			// are not available until after chi routing).
			orgID, _ := OrgUnitIDFromContext(r.Context())
			apiKeyID, hasAPIKey := APIKeyIDFromContext(r.Context())

			actorType := domain.ActorTypeSystem
			actorID := "system"
			if hasAPIKey {
				actorType = domain.ActorTypeAPIKey
				actorID = apiKeyID.String()
			}

			// Derive resource type and ID from the URL path.
			resourceType, resourceID := extractResource(r)

			ev := &domain.AuditEvent{
				ID:           uuid.New(),
				OrgUnitID:    orgID,
				ActorType:    actorType,
				ActorID:      actorID,
				Action:       r.Method + " " + r.URL.Path,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Details: map[string]any{
					"status":      wrapped.Status(),
					"duration_ms": time.Since(start).Milliseconds(),
					"user_agent":  r.UserAgent(),
				},
				IPAddress:  realIP(r),
				OccurredAt: time.Now().UTC(),
			}

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := auditor.WriteAuditEvent(ctx, ev); err != nil {
					log.Warn().Err(err).Str("action", ev.Action).Msg("audit: write event failed")
				}
			}()
		})
	}
}

// extractResource attempts to derive a resource type and optional resource ID
// from the request URL path using chi's route context if available.
func extractResource(r *http.Request) (resourceType string, resourceID *string) {
	// Try chi route params first.
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		for i, key := range rctx.URLParams.Keys {
			if strings.EqualFold(key, "id") && i < len(rctx.URLParams.Values) {
				v := rctx.URLParams.Values[i]
				resourceID = &v
				break
			}
		}
	}

	// Derive resource type from path segments.
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, seg := range segments {
		switch seg {
		case "tasks", "workers", "api-keys", "orgs", "outputs":
			resourceType = seg
			// The segment immediately after the resource name might be the ID.
			if i+1 < len(segments) && resourceID == nil {
				v := segments[i+1]
				if v != "" && v != "register" && v != "heartbeat" && v != "rotate" {
					resourceID = &v
				}
			}
			return
		}
	}

	if len(segments) > 0 {
		resourceType = segments[len(segments)-1]
	}
	return
}
