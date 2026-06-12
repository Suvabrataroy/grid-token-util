package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/store/postgres"
	"github.com/rs/zerolog"
)

// AuditHandler handles read-only HTTP requests against the audit log.
type AuditHandler struct {
	store *postgres.Store
	log   zerolog.Logger
}

// NewAuditHandler creates an AuditHandler backed by the given store.
func NewAuditHandler(store *postgres.Store, log zerolog.Logger) *AuditHandler {
	return &AuditHandler{store: store, log: log}
}

// List handles GET /audit – returns paginated audit events for the authenticated org unit.
//
// Query parameters:
//   - from   RFC3339 timestamp (default: 24 hours ago)
//   - to     RFC3339 timestamp (default: now)
//   - limit  integer 1–1000    (default: 100)
//   - offset integer >= 0      (default: 0)
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	orgUnitID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC()

	if s := q.Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = t
		}
	}
	if s := q.Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = t
		}
	}

	limit := 100
	offset := 0
	if s := q.Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	if s := q.Get("offset"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 {
			offset = v
		}
	}

	events, err := h.store.QueryAuditLog(r.Context(), orgUnitID, from, to, limit, offset)
	if err != nil {
		h.log.Error().Err(err).Msg("query audit log")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
