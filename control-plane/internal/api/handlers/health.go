// Package handlers contains HTTP request handlers for the control-plane API.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger is satisfied by both the Postgres store and the Redis store.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler handles liveness and readiness probes.
type HealthHandler struct {
	db    Pinger
	redis Pinger
}

// NewHealthHandler creates a HealthHandler with the supplied dependencies.
func NewHealthHandler(db Pinger, redis Pinger) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Liveness handles GET /healthz.
// Always returns 200 {"status":"ok"} as long as the process is running.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness handles GET /readyz.
// Returns 200 only when both the database and Redis are reachable.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	type check struct {
		name string
		ping func() error
	}

	checks := []check{
		{"postgres", func() error { return h.db.Ping(ctx) }},
		{"redis", func() error { return h.redis.Ping(ctx) }},
	}

	details := make(map[string]string, len(checks))
	allOK := true

	for _, c := range checks {
		if err := c.ping(); err != nil {
			details[c.name] = "unhealthy: " + err.Error()
			allOK = false
		} else {
			details[c.name] = "ok"
		}
	}

	status := "ok"
	code := http.StatusOK
	if !allOK {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{
		"status":  status,
		"checks":  details,
		"time":    time.Now().UTC(),
	})
}

// writeJSON is a convenience helper that serialises v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "")
	_ = enc.Encode(v)
}
