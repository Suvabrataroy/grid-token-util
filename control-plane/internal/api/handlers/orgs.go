package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/store/postgres"
	"github.com/rs/zerolog"
)

// OrgHandler handles HTTP requests for org unit CRUD operations.
type OrgHandler struct {
	store *postgres.Store
	log   zerolog.Logger
}

// NewOrgHandler creates an OrgHandler backed by the given store.
func NewOrgHandler(store *postgres.Store, log zerolog.Logger) *OrgHandler {
	return &OrgHandler{store: store, log: log}
}

// Create handles POST /orgs – creates a new org unit.
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		PlanTier string `json:"plan_tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if req.PlanTier == "" {
		req.PlanTier = "free"
	}
	now := time.Now().UTC()
	org := &domain.OrgUnit{
		ID:        uuid.New(),
		Name:      req.Name,
		PlanTier:  req.PlanTier,
		Policy:    make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.CreateOrg(r.Context(), org); err != nil {
		h.log.Error().Err(err).Msg("create org")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(org)
}

// Get handles GET /orgs/{id} – retrieves a single org unit by ID.
func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid org id"}`, http.StatusBadRequest)
		return
	}
	org, err := h.store.GetOrg(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// List handles GET /orgs – returns all org units.
func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.store.ListOrgs(r.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("list orgs")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orgs)
}

// UpdatePolicy handles PUT /orgs/{id}/policy – replaces the org policy document.
func (h *OrgHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid org id"}`, http.StatusBadRequest)
		return
	}
	var policy domain.OrgPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateOrgPolicy(r.Context(), id, policy); err != nil {
		h.log.Error().Err(err).Msg("update org policy")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
