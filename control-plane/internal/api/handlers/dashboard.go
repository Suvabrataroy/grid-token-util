package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/brownie"
	"github.com/grid-computing/control-plane/internal/dashboard"
	"github.com/grid-computing/control-plane/internal/domain"
)

// DashboardHubAdapter is the subset of dashboard.Hub used by DashboardHandler.
type DashboardHubAdapter interface {
	ServeSSE(w http.ResponseWriter, r *http.Request, orgID uuid.UUID)
}

// SnapshotSource provides the data queries needed to build a dashboard snapshot.
type SnapshotSource interface {
	// Worker queries
	ListWorkers(ctx context.Context, orgUnitID uuid.UUID) ([]*domain.Worker, error)
	// Task queries
	ListTasksByOrg(ctx context.Context, orgUnitID uuid.UUID, limit, offset int) ([]*domain.Task, error)
	// Audit log
	QueryAuditLog(ctx context.Context, orgUnitID uuid.UUID, from, to time.Time, limit, offset int) ([]*domain.AuditEvent, error)
	// Brownie leaderboard
	GetLeaderboard(ctx context.Context, orgUnitID uuid.UUID, limit int) ([]brownie.LeaderboardEntry, error)
}

// DashboardHandler serves real-time and snapshot dashboard data.
type DashboardHandler struct {
	hub         DashboardHubAdapter
	snapshotter SnapshotSource
}

// NewDashboardHandler creates a DashboardHandler.
func NewDashboardHandler(hub DashboardHubAdapter, snapshotter SnapshotSource) *DashboardHandler {
	return &DashboardHandler{hub: hub, snapshotter: snapshotter}
}

// Stream handles GET /api/v1/dashboard/stream — SSE endpoint.
func (h *DashboardHandler) Stream(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	h.hub.ServeSSE(w, r, orgID)
}

// dashboardSnapshot is the full response structure for GET /snapshot.
type dashboardSnapshot struct {
	GeneratedAt      time.Time                     `json:"generated_at"`
	ActiveWorkers    []*domain.Worker              `json:"active_workers"`
	QueuedTasks      []*domain.Task                `json:"queued_tasks"`
	RunningTasks     []*domain.Task                `json:"running_tasks"`
	RecentCompleted  []*domain.Task                `json:"recent_completed"`
	TopBrownieEarners []brownie.LeaderboardEntry   `json:"top_brownie_earners"`
	RecentSecurityEvents []*domain.AuditEvent      `json:"recent_security_events"`
}

// Snapshot handles GET /api/v1/dashboard/snapshot.
// It runs 7 parallel queries and assembles the results.
func (h *DashboardHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgUnitIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing org context")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		mu            sync.Mutex
		snap          dashboardSnapshot
		errs          []error
		wg            sync.WaitGroup
	)

	snap.GeneratedAt = time.Now().UTC()

	// Helper that runs a function in a goroutine and collects errors.
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}

	// 1. Active workers (idle + busy)
	run(func() error {
		workers, err := h.snapshotter.ListWorkers(ctx, orgID)
		if err != nil {
			return err
		}
		var active []*domain.Worker
		for _, w := range workers {
			if w.State == domain.WorkerStateIdle || w.State == domain.WorkerStateBusy {
				active = append(active, w)
			}
		}
		mu.Lock()
		snap.ActiveWorkers = active
		mu.Unlock()
		return nil
	})

	// 2. Queued tasks
	run(func() error {
		tasks, err := h.snapshotter.ListTasksByOrg(ctx, orgID, 50, 0)
		if err != nil {
			return err
		}
		var queued []*domain.Task
		for _, t := range tasks {
			if t.State == domain.TaskStateQueued {
				queued = append(queued, t)
			}
		}
		mu.Lock()
		snap.QueuedTasks = queued
		mu.Unlock()
		return nil
	})

	// 3. Running tasks
	run(func() error {
		tasks, err := h.snapshotter.ListTasksByOrg(ctx, orgID, 50, 0)
		if err != nil {
			return err
		}
		var running []*domain.Task
		for _, t := range tasks {
			if t.State == domain.TaskStateRunning || t.State == domain.TaskStateAssigned {
				running = append(running, t)
			}
		}
		mu.Lock()
		snap.RunningTasks = running
		mu.Unlock()
		return nil
	})

	// 4. Recent completions
	run(func() error {
		tasks, err := h.snapshotter.ListTasksByOrg(ctx, orgID, 20, 0)
		if err != nil {
			return err
		}
		var completed []*domain.Task
		for _, t := range tasks {
			if t.State == domain.TaskStateCompleted || t.State == domain.TaskStateFailed {
				completed = append(completed, t)
			}
		}
		mu.Lock()
		snap.RecentCompleted = completed
		mu.Unlock()
		return nil
	})

	// 5. Token usage — omitted in snapshot (no dedicated store interface here);
	//    future iteration can add a TokenUsageStore interface.

	// 6. Top brownie earners
	run(func() error {
		entries, err := h.snapshotter.GetLeaderboard(ctx, orgID, 10)
		if err != nil {
			return err
		}
		mu.Lock()
		snap.TopBrownieEarners = entries
		mu.Unlock()
		return nil
	})

	// 7. Recent security events (from audit log, last 24 hours)
	run(func() error {
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour)
		events, err := h.snapshotter.QueryAuditLog(ctx, orgID, from, to, 20, 0)
		if err != nil {
			return err
		}
		// Filter to security-relevant actions only.
		var secEvents []*domain.AuditEvent
		for _, ev := range events {
			if ev.Action == "auth_failed" || ev.ResourceType == "api_key" {
				secEvents = append(secEvents, ev)
			}
		}
		mu.Lock()
		snap.RecentSecurityEvents = secEvents
		mu.Unlock()
		return nil
	})

	wg.Wait()

	if len(errs) > 0 {
		log.Warn().Int("error_count", len(errs)).Msg("dashboard: snapshot partial errors")
	}

	writeJSON(w, http.StatusOK, snap)
}

// ensure dashboard.Hub satisfies the adapter interface at compile time.
var _ DashboardHubAdapter = (*dashboard.Hub)(nil)
