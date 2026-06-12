package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/brownie"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/store/postgres"
	"github.com/rs/zerolog"
)

// OutputHandler handles HTTP requests for output submission and review.
type OutputHandler struct {
	store      *postgres.Store
	brownie    *brownie.Engine
	hmacSecret string
	log        zerolog.Logger
}

// NewOutputHandler creates an OutputHandler backed by the given store and brownie engine.
func NewOutputHandler(store *postgres.Store, be *brownie.Engine, hmacSecret string, log zerolog.Logger) *OutputHandler {
	return &OutputHandler{store: store, brownie: be, hmacSecret: hmacSecret, log: log}
}

// Submit handles POST /outputs – workers submit completed task artefacts.
func (h *OutputHandler) Submit(w http.ResponseWriter, r *http.Request) {
	workerID, ok := middleware.WorkerIDFromContext(r.Context())
	if !ok || workerID == nil {
		http.Error(w, `{"error":"worker authentication required"}`, http.StatusForbidden)
		return
	}

	var req struct {
		TaskID     string               `json:"task_id"`
		HMACSha256 string               `json:"hmac_sha256"`
		Artifacts  []domain.ArtifactMeta `json:"artifacts"`
		Metadata   map[string]any       `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Verify HMAC from header matches body field
	sigHeader := r.Header.Get("X-HMAC-SHA256")
	if sigHeader != req.HMACSha256 {
		http.Error(w, `{"error":"HMAC mismatch"}`, http.StatusUnprocessableEntity)
		return
	}

	taskID, err := uuid.Parse(req.TaskID)
	if err != nil {
		http.Error(w, `{"error":"invalid task_id"}`, http.StatusBadRequest)
		return
	}

	output := &domain.OutputPackage{
		ID:           uuid.New(),
		TaskID:       taskID,
		WorkerID:     *workerID,
		HMACSha256:   req.HMACSha256,
		Artifacts:    req.Artifacts,
		Metadata:     req.Metadata,
		ReviewStatus: domain.ReviewStatusPending,
		SubmittedAt:  time.Now().UTC(),
	}

	if err := h.store.CreateOutput(r.Context(), output); err != nil {
		h.log.Error().Err(err).Msg("create output")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Award brownie points for task submission
	orgUnitID, _ := middleware.OrgUnitIDFromContext(r.Context())
	if err := h.brownie.Award(r.Context(), *workerID, orgUnitID, brownie.PointsTaskCompleted, "task_completed", &taskID); err != nil {
		h.log.Warn().Err(err).Msg("award brownie points for submission")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"id": output.ID, "status": "accepted"})
}

// Review handles POST /outputs/{id}/review – approves, rejects, or requests changes on an output.
func (h *OutputHandler) Review(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	reviewerID, ok := middleware.APIKeyIDFromContext(r.Context())
	if !ok || reviewerID == uuid.Nil {
		http.Error(w, `{"error":"reviewer authentication required"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid output id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		ReviewStatus string `json:"review_status"`
		Comment      string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	validStatuses := map[string]bool{
		"approved":          true,
		"rejected":          true,
		"changes_requested": true,
	}
	if !validStatuses[req.ReviewStatus] {
		http.Error(w, `{"error":"invalid review_status"}`, http.StatusBadRequest)
		return
	}

	// Fetch the output before modifying it so we can verify org ownership.
	output, err := h.store.GetOutput(r.Context(), id)
	if err != nil {
		h.log.Warn().Err(err).Str("output_id", id.String()).Msg("fetch output for review")
		http.Error(w, `{"error":"output not found"}`, http.StatusNotFound)
		return
	}

	// Verify the output's task belongs to the caller's org.
	task, err := h.store.GetTask(r.Context(), output.TaskID)
	if err != nil {
		h.log.Error().Err(err).Str("task_id", output.TaskID.String()).Msg("fetch task for output ownership check")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if task.OrgUnitID != orgID {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	if err := h.store.UpdateOutputReview(r.Context(), id, domain.ReviewStatus(req.ReviewStatus), reviewerID); err != nil {
		h.log.Error().Err(err).Msg("update output review")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Award or deduct brownie points based on review decision.
	// PointsOutputRejected is negative; Deduct expects a positive value.
	switch req.ReviewStatus {
	case "approved":
		if err := h.brownie.Award(r.Context(), output.WorkerID, orgID, brownie.PointsOutputApproved, "output_approved", &id); err != nil {
			h.log.Warn().Err(err).Msg("award brownie points for approved output")
		}
	case "rejected":
		if err := h.brownie.Deduct(r.Context(), output.WorkerID, orgID, -brownie.PointsOutputRejected, "output_rejected", &id); err != nil {
			h.log.Warn().Err(err).Msg("deduct brownie points for rejected output")
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
