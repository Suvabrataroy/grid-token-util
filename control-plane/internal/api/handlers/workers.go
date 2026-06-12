package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/domain"
)

// WorkerPGStore is the Postgres interface required by WorkerHandler.
type WorkerPGStore interface {
	RegisterWorker(ctx context.Context, w *domain.Worker) error
	UpdateWorkerState(ctx context.Context, workerID uuid.UUID, state domain.WorkerState) error
	UpdateWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, agents []string) error
	GetWorker(ctx context.Context, id uuid.UUID) (*domain.Worker, error)
	ListWorkers(ctx context.Context, orgUnitID uuid.UUID) ([]*domain.Worker, error)
	GetAssignedTaskForWorker(ctx context.Context, workerID uuid.UUID) (*domain.Task, error)
}

// WorkerRedisStore is the Redis interface required by WorkerHandler.
type WorkerRedisStore interface {
	SetWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, ttlSec int) error
}

// WorkerAssignedTaskStore is the task-related interface for the heartbeat handler.
type WorkerAssignedTaskStore interface {
	GetTask(ctx context.Context, id uuid.UUID) (*domain.Task, error)
}

// WorkerHandler handles worker registration and heartbeat endpoints.
type WorkerHandler struct {
	pgStore    WorkerPGStore
	redisStore WorkerRedisStore
	heartbeatTTLSec int
}

// NewWorkerHandler creates a WorkerHandler with the given dependencies.
func NewWorkerHandler(pgStore WorkerPGStore, redisStore WorkerRedisStore, heartbeatTTLSec int) *WorkerHandler {
	return &WorkerHandler{
		pgStore:         pgStore,
		redisStore:      redisStore,
		heartbeatTTLSec: heartbeatTTLSec,
	}
}

// registerWorkerRequest is the body for POST /api/v1/workers/register.
// Clients may send either hostname_hash (pre-hashed, privacy-preserving) or
// hostname (server will hash it).
type registerWorkerRequest struct {
	HostnameHash  string   `json:"hostname_hash"`  // pre-hashed by the worker client
	Hostname      string   `json:"hostname"`        // raw hostname (server hashes it)
	Agents        []string `json:"agents"`
	CapacityScore float64  `json:"capacity_score"` // accept float64; truncated to int
}

// Register handles POST /api/v1/workers/register.
func (h *WorkerHandler) Register(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	var req registerWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Accept pre-hashed hostname_hash (privacy-preserving) or raw hostname.
	hostnameHash := req.HostnameHash
	if hostnameHash == "" {
		if req.Hostname == "" {
			writeError(w, http.StatusBadRequest, "hostname or hostname_hash is required")
			return
		}
		hash := sha256.Sum256([]byte(req.Hostname))
		hostnameHash = hex.EncodeToString(hash[:])
	}

	const maxAgents = 50
	const maxAgentNameLen = 100
	if len(req.Agents) > maxAgents {
		writeError(w, http.StatusBadRequest, "agents list exceeds maximum allowed size")
		return
	}
	for _, a := range req.Agents {
		if len(a) == 0 || len(a) > maxAgentNameLen {
			writeError(w, http.StatusBadRequest, "agent name must be between 1 and 100 characters")
			return
		}
	}

	capacityScore := int(req.CapacityScore)
	if capacityScore == 0 {
		capacityScore = 100
	}
	if capacityScore < 0 || capacityScore > 100 {
		writeError(w, http.StatusBadRequest, "capacity_score must be between 0 and 100")
		return
	}

	now := time.Now().UTC()
	worker := &domain.Worker{
		ID:              uuid.New(),
		OrgUnitID:       orgID,
		HostnameHash:    hostnameHash,
		Agents:          req.Agents,
		State:           domain.WorkerStateIdle,
		CapacityScore:   capacityScore,
		LastHeartbeatAt: &now,
		CreatedAt:       now,
	}

	if err := h.pgStore.RegisterWorker(r.Context(), worker); err != nil {
		log.Error().Err(err).Msg("workers: register")
		writeError(w, http.StatusInternalServerError, "failed to register worker")
		return
	}

	// Set initial heartbeat in Redis.
	if err := h.redisStore.SetWorkerHeartbeat(r.Context(), worker.ID, h.heartbeatTTLSec); err != nil {
		log.Warn().Err(err).Str("worker_id", worker.ID.String()).Msg("workers: set initial heartbeat in redis")
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"worker_id": worker.ID,
		"state":     worker.State,
	})
}

// List handles GET /api/v1/workers — returns all workers for the caller's org unit.
func (h *WorkerHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	workers, err := h.pgStore.ListWorkers(r.Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("workers: list")
		writeError(w, http.StatusInternalServerError, "failed to list workers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workers": workers,
		"count":   len(workers),
	})
}

// Get handles GET /api/v1/workers/:id — returns a single worker scoped to the caller's org.
func (h *WorkerHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	idStr := chi.URLParam(r, "id")
	workerID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker ID")
		return
	}

	worker, err := h.pgStore.GetWorker(r.Context(), workerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}

	if worker.OrgUnitID != orgID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, worker)
}

// Heartbeat handles POST /api/v1/workers/:id/heartbeat.
func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	idStr := chi.URLParam(r, "id")
	workerID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker ID")
		return
	}

	// Verify worker belongs to the caller's org.
	worker, err := h.pgStore.GetWorker(r.Context(), workerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}
	if worker.OrgUnitID != orgID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	type heartbeatRequest struct {
		Agents []string `json:"agents"`
	}
	var req heartbeatRequest
	// Body is optional for a heartbeat.
	_ = json.NewDecoder(r.Body).Decode(&req)

	const maxAgents = 50
	const maxAgentNameLen = 100
	if len(req.Agents) > maxAgents {
		writeError(w, http.StatusBadRequest, "agents list exceeds maximum allowed size")
		return
	}
	for _, a := range req.Agents {
		if len(a) == 0 || len(a) > maxAgentNameLen {
			writeError(w, http.StatusBadRequest, "agent name must be between 1 and 100 characters")
			return
		}
	}

	agents := req.Agents
	if agents == nil {
		agents = worker.Agents
	}

	// Update heartbeat in both Redis (TTL) and Postgres (timestamp).
	if err := h.redisStore.SetWorkerHeartbeat(r.Context(), workerID, h.heartbeatTTLSec); err != nil {
		log.Warn().Err(err).Str("worker_id", workerID.String()).Msg("workers: set heartbeat redis")
	}
	if err := h.pgStore.UpdateWorkerHeartbeat(r.Context(), workerID, agents); err != nil {
		log.Error().Err(err).Str("worker_id", workerID.String()).Msg("workers: update heartbeat pg")
		writeError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	// Check for a task the scheduler has assigned to this worker.
	resp := map[string]any{
		"ok":           true,
		"worker_id":    workerID,
		"next_ttl_sec": h.heartbeatTTLSec,
	}
	if assigned, err := h.pgStore.GetAssignedTaskForWorker(r.Context(), workerID); err != nil {
		log.Warn().Err(err).Str("worker_id", workerID.String()).Msg("workers: get assigned task")
	} else if assigned != nil {
		// Convert domain.Task options (map[string]any) to map[string]string for the worker client.
		options := make(map[string]string)
		for k, v := range assigned.Options {
			if str, ok := v.(string); ok {
				options[k] = str
			}
		}
		repoURL, _ := assigned.Options["repo_url"].(string)
		branch, _ := assigned.Options["branch"].(string)
		timeoutSec := 3600
		if t, ok := assigned.Options["timeout_sec"].(float64); ok {
			timeoutSec = int(t)
		}
		resp["assigned_task"] = map[string]any{
			"task_id":     assigned.ID.String(),
			"title":       assigned.Title,
			"description": assigned.Description,
			"task_type":   string(assigned.TaskType),
			"ai_agent":    assigned.AIAgent,
			"options":     options,
			"repo_url":    repoURL,
			"branch":      branch,
			"timeout_sec": timeoutSec,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
