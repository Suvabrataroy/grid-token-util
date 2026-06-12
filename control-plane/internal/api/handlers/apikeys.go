package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/security"
)

// APIKeyPGStore is the persistence interface required by APIKeyHandler.
type APIKeyPGStore interface {
	CreateAPIKey(ctx context.Context, k *domain.APIKey) error
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID) error
	RotateAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID, newHash, newPrefix string) (*domain.APIKey, error)
}

// APIKeyHandler handles API key lifecycle endpoints.
type APIKeyHandler struct {
	store  APIKeyPGStore
	argon  security.ArgonParams
}

// NewAPIKeyHandler creates an APIKeyHandler.
func NewAPIKeyHandler(store APIKeyPGStore, argon security.ArgonParams) *APIKeyHandler {
	return &APIKeyHandler{store: store, argon: argon}
}

// createAPIKeyRequest is the body for POST /api/v1/api-keys.
type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	WorkerID  *string    `json:"worker_id,omitempty"`
}

// CreateAPIKey handles POST /api/v1/api-keys.
// The plain-text key is returned exactly once in the response and is never
// stored server-side.
func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	plaintext, prefix, hash, err := security.GenerateAPIKeyWithParams(h.argon)
	if err != nil {
		log.Error().Err(err).Msg("apikeys: generate")
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}

	var workerID *uuid.UUID
	if req.WorkerID != nil {
		id, err := uuid.Parse(*req.WorkerID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid worker_id")
			return
		}
		workerID = &id
	}

	k := &domain.APIKey{
		ID:        uuid.New(),
		OrgUnitID: orgID,
		WorkerID:  workerID,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Name:      req.Name,
		Scopes:    req.Scopes,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: req.ExpiresAt,
	}

	if err := h.store.CreateAPIKey(r.Context(), k); err != nil {
		log.Error().Err(err).Msg("apikeys: create")
		writeError(w, http.StatusInternalServerError, "failed to store key")
		return
	}

	// Respond with the plain-text key — this is the ONLY opportunity to
	// retrieve it.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         k.ID,
		"key":        plaintext, // one-time reveal
		"key_prefix": prefix,
		"name":       k.Name,
		"scopes":     k.Scopes,
		"created_at": k.CreatedAt,
		"expires_at": k.ExpiresAt,
	})
}

// RotateAPIKey handles POST /api/v1/api-keys/:id/rotate.
func (h *APIKeyHandler) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	idStr := chi.URLParam(r, "id")
	keyID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key ID")
		return
	}

	plaintext, prefix, hash, err := security.GenerateAPIKeyWithParams(h.argon)
	if err != nil {
		log.Error().Err(err).Msg("apikeys: generate for rotation")
		writeError(w, http.StatusInternalServerError, "failed to generate new key")
		return
	}

	// orgID is passed into the store so the UPDATE is org-scoped; the DB
	// returns no rows (and an error) if the key belongs to a different org.
	k, err := h.store.RotateAPIKey(r.Context(), keyID, orgID, hash, prefix)
	if err != nil {
		log.Error().Err(err).Str("key_id", keyID.String()).Msg("apikeys: rotate")
		writeError(w, http.StatusNotFound, "key not found or access denied")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         k.ID,
		"key":        plaintext, // one-time reveal
		"key_prefix": prefix,
		"name":       k.Name,
	})
}

// RevokeAPIKey handles DELETE /api/v1/api-keys/:id.
func (h *APIKeyHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	idStr := chi.URLParam(r, "id")
	keyID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key ID")
		return
	}

	// orgID is passed so the DELETE is org-scoped; prevents cross-org revocation.
	if err := h.store.RevokeAPIKey(r.Context(), keyID, orgID); err != nil {
		log.Error().Err(err).Str("key_id", keyID.String()).Msg("apikeys: revoke")
		writeError(w, http.StatusNotFound, "key not found or access denied")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
