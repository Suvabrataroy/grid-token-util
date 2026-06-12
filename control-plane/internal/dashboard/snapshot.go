package dashboard

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/store/postgres"
	"github.com/rs/zerolog"
)

// TokenUsageStat aggregates AI token consumption for a single agent over a
// reporting period.
type TokenUsageStat struct {
	AIAgent      string `json:"ai_agent"`
	TotalTokens  int64  `json:"total_tokens"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Period       string `json:"period"` // "today" | "this_month"
}

// LeaderboardEntry represents one row on the brownie points leaderboard.
type LeaderboardEntry struct {
	WorkerID    uuid.UUID  `json:"worker_id"`
	OrgUnitID   uuid.UUID  `json:"org_unit_id"`
	TotalPoints int64      `json:"total_points"`
	EventCount  int64      `json:"event_count"`
	LastEventAt *time.Time `json:"last_event_at,omitempty"`
}

// SecurityEventSummary is a condensed view of a security-relevant audit entry.
type SecurityEventSummary struct {
	OccurredAt time.Time      `json:"occurred_at"`
	WorkerID   string         `json:"worker_id"`
	Action     string         `json:"action"`
	Details    map[string]any `json:"details"`
}

// Snapshot holds the initial state for all 7 dashboard panels.
type Snapshot struct {
	Workers         []*domain.Worker       `json:"workers"`
	QueuedTasks     []*domain.Task         `json:"queued_tasks"`
	RunningTasks    []*domain.Task         `json:"running_tasks"`
	RecentCompleted []*domain.Task         `json:"recent_completed"`
	TokenUsage      []TokenUsageStat       `json:"token_usage"`
	Leaderboard     []LeaderboardEntry     `json:"leaderboard"`
	SecurityEvents  []SecurityEventSummary `json:"security_events"`
	GeneratedAt     time.Time              `json:"generated_at"`
}

// SnapshotBuilder builds dashboard snapshots using parallel Postgres queries.
type SnapshotBuilder struct {
	store *postgres.Store
	log   zerolog.Logger
}

// NewSnapshotBuilder creates a SnapshotBuilder backed by the given store.
func NewSnapshotBuilder(store *postgres.Store, log zerolog.Logger) *SnapshotBuilder {
	return &SnapshotBuilder{store: store, log: log}
}

// Build runs 7 parallel queries with a 2.5s deadline and returns the composite
// snapshot.  A partial result is returned even when individual queries fail
// (graceful degradation).
func (b *SnapshotBuilder) Build(ctx context.Context, orgUnitID uuid.UUID) (*Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		snapshot = &Snapshot{GeneratedAt: time.Now().UTC()}
		firstErr error
	)

	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	// Query 1: Active workers
	wg.Add(1)
	go func() {
		defer wg.Done()
		workers, err := b.store.ListWorkers(ctx, orgUnitID)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.Workers = workers
		mu.Unlock()
	}()

	// Query 2: Queued tasks
	wg.Add(1)
	go func() {
		defer wg.Done()
		tasks, err := b.store.ListTasksByOrgAndState(ctx, orgUnitID, "queued", 50, 0)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.QueuedTasks = tasks
		mu.Unlock()
	}()

	// Query 3: Running tasks
	wg.Add(1)
	go func() {
		defer wg.Done()
		tasks, err := b.store.ListTasksByOrgAndState(ctx, orgUnitID, "running", 50, 0)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.RunningTasks = tasks
		mu.Unlock()
	}()

	// Query 4: Recent completions (last 24 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		tasks, err := b.store.ListRecentCompletedTasks(ctx, orgUnitID, 24*time.Hour, 50)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.RecentCompleted = tasks
		mu.Unlock()
	}()

	// Query 5: Token usage by agent (last 30 days)
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats, err := b.store.GetTokenUsageByAgent(ctx, orgUnitID, 30*24*time.Hour)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.TokenUsage = stats
		mu.Unlock()
	}()

	// Query 6: Top brownie earners
	wg.Add(1)
	go func() {
		defer wg.Done()
		entries, err := b.store.GetLeaderboardFull(ctx, orgUnitID, 10)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.Leaderboard = entries
		mu.Unlock()
	}()

	// Query 7: Recent security events (last 24 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		events, err := b.store.GetSecurityEvents(ctx, orgUnitID, 24*time.Hour, 20)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		snapshot.SecurityEvents = events
		mu.Unlock()
	}()

	wg.Wait()

	// Return partial snapshot even if some queries failed (graceful degradation).
	if firstErr != nil {
		b.log.Warn().Err(firstErr).Msg("snapshot: one or more queries failed (partial result returned)")
	}

	return snapshot, nil
}
