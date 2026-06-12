package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/store/postgres"
	"github.com/rs/zerolog"
)

// BrownieHandler handles HTTP requests for brownie points leaderboard data.
type BrownieHandler struct {
	store *postgres.Store
	log   zerolog.Logger
}

// NewBrownieHandler creates a BrownieHandler backed by the given store.
func NewBrownieHandler(store *postgres.Store, log zerolog.Logger) *BrownieHandler {
	return &BrownieHandler{store: store, log: log}
}

// Leaderboard handles GET /brownie/leaderboard – returns the top-N workers
// ranked by total brownie points for the authenticated org unit.
//
// Query parameters:
//   - limit  integer 1–100 (default: 10)
func (h *BrownieHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	orgUnitID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	entries, err := h.store.GetLeaderboard(r.Context(), orgUnitID, limit)
	if err != nil {
		h.log.Error().Err(err).Msg("get leaderboard")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
