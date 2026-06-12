package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/security"
)

// TaskStore is the persistence interface required by TaskHandler.
type TaskStore interface {
	CreateTask(ctx context.Context, t *domain.Task) error
	GetTask(ctx context.Context, id uuid.UUID) (*domain.Task, error)
	UpdateTaskState(ctx context.Context, id uuid.UUID, state domain.TaskState, workerID *uuid.UUID) error
	ListTasksByOrg(ctx context.Context, orgUnitID uuid.UUID, limit, offset int) ([]*domain.Task, error)
}

// TaskQueue is the Redis queue interface required by TaskHandler.
type TaskQueue interface {
	Enqueue(ctx context.Context, taskID uuid.UUID, priority int, queuedAt time.Time) error
}

// TaskHandler handles task-related HTTP endpoints.
type TaskHandler struct {
	store   TaskStore
	queue   TaskQueue
	scanner *security.Scanner
}

// NewTaskHandler creates a TaskHandler.
func NewTaskHandler(store TaskStore, queue TaskQueue, scanner *security.Scanner) *TaskHandler {
	return &TaskHandler{store: store, queue: queue, scanner: scanner}
}

// createTaskRequest is the request body for task creation.
type createTaskRequest struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	TaskType    string         `json:"task_type"`
	Priority    int            `json:"priority"`
	AIAgent     string         `json:"ai_agent"`
	MaxRetries  int            `json:"max_retries"`
	Options     map[string]any `json:"options"`
}

// CreateTask handles POST /api/v1/tasks.
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields.
	if req.Title == "" || req.Description == "" || req.AIAgent == "" || req.TaskType == "" {
		writeError(w, http.StatusBadRequest, "title, description, ai_agent, and task_type are required")
		return
	}

	// Secret scan the description and title.
	combined := req.Title + "\n" + req.Description
	findings, err := h.scanner.ScanText(combined)
	if err != nil {
		log.Error().Err(err).Msg("tasks: secret scan failed")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(findings) > 0 {
		log.Warn().
			Str("org_id", orgID.String()).
			Int("finding_count", len(findings)).
			Msg("tasks: secret found in task payload")
		writeError(w, http.StatusUnprocessableEntity, "task payload contains potential secrets")
		return
	}

	if req.Priority == 0 {
		req.Priority = 5
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.Options == nil {
		req.Options = map[string]any{}
	}

	now := time.Now().UTC()
	task := &domain.Task{
		ID:          uuid.New(),
		OrgUnitID:   orgID,
		Title:       req.Title,
		Description: req.Description,
		TaskType:    domain.TaskType(req.TaskType),
		Priority:    req.Priority,
		AIAgent:     req.AIAgent,
		State:       domain.TaskStateQueued,
		QueuedAt:    now,
		MaxRetries:  req.MaxRetries,
		Options:     req.Options,
	}

	if wid, ok := middleware.WorkerIDFromContext(r.Context()); ok && wid != nil {
		task.SubmitterID = wid
	}

	if err := h.store.CreateTask(r.Context(), task); err != nil {
		log.Error().Err(err).Msg("tasks: create")
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	if err := h.queue.Enqueue(r.Context(), task.ID, task.Priority, task.QueuedAt); err != nil {
		log.Error().Err(err).Str("task_id", task.ID.String()).Msg("tasks: enqueue")
		// Task is persisted; the scheduler will pick it up on next scan.
	}

	writeJSON(w, http.StatusCreated, task)
}

// GetTask handles GET /api/v1/tasks/:id.
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	idStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	task, err := h.store.GetTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Scope check: task must belong to the caller's org unit.
	if task.OrgUnitID != orgID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// updateTaskStateRequest is the body for PATCH /api/v1/tasks/:id/status.
type updateTaskStateRequest struct {
	State        string  `json:"state"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

// UpdateTaskStatus handles PATCH /api/v1/tasks/:id/status (worker-only).
func (h *TaskHandler) UpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	workerID, ok := middleware.WorkerIDFromContext(r.Context())
	if !ok || workerID == nil {
		writeError(w, http.StatusForbidden, "worker authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req updateTaskStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	newState := domain.TaskState(req.State)
	if err := validateStateTransition(newState); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Fetch task to verify both worker assignment and org ownership.
	task, err := h.store.GetTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Ensure the task belongs to the worker's org (prevents cross-org manipulation).
	if task.OrgUnitID != orgID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if task.AssignedWorkerID == nil || *task.AssignedWorkerID != *workerID {
		writeError(w, http.StatusForbidden, "task not assigned to this worker")
		return
	}

	if err := h.store.UpdateTaskState(r.Context(), taskID, newState, workerID); err != nil {
		log.Error().Err(err).Msg("tasks: update state")
		writeError(w, http.StatusInternalServerError, "failed to update task state")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"state": string(newState)})
}

// ListTasks handles GET /api/v1/tasks with pagination query params.
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	tasks, err := h.store.ListTasksByOrg(r.Context(), orgID, limit, offset)
	if err != nil {
		log.Error().Err(err).Msg("tasks: list")
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tasks":  tasks,
		"limit":  limit,
		"offset": offset,
	})
}

// validateStateTransition ensures only valid target states are accepted from
// a worker PATCH.
func validateStateTransition(s domain.TaskState) error {
	switch s {
	case domain.TaskStateRunning, domain.TaskStateCompleted, domain.TaskStateFailed:
		return nil
	default:
		return errors.New("invalid state transition; allowed: running, completed, failed")
	}
}

// writeError writes a structured JSON error response.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
