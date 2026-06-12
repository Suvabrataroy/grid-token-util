// Package api wires all HTTP handlers and middleware chains together.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api/handlers"
	"github.com/grid-computing/control-plane/internal/api/middleware"
	"github.com/grid-computing/control-plane/internal/brownie"
	"github.com/grid-computing/control-plane/internal/domain"
	"github.com/grid-computing/control-plane/internal/security"
	pgstore "github.com/grid-computing/control-plane/internal/store/postgres"
)

// FullStore is a convenience interface that the Postgres store satisfies.
// It covers everything the router needs from the database layer.
type FullStore interface {
	// Tasks
	CreateTask(ctx context.Context, t *domain.Task) error
	GetTask(ctx context.Context, id uuid.UUID) (*domain.Task, error)
	UpdateTaskState(ctx context.Context, id uuid.UUID, state domain.TaskState, workerID *uuid.UUID) error
	ListTasksByOrg(ctx context.Context, orgUnitID uuid.UUID, limit, offset int) ([]*domain.Task, error)

	// Workers
	RegisterWorker(ctx context.Context, w *domain.Worker) error
	UpdateWorkerState(ctx context.Context, workerID uuid.UUID, state domain.WorkerState) error
	UpdateWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, agents []string) error
	GetWorker(ctx context.Context, id uuid.UUID) (*domain.Worker, error)
	ListWorkers(ctx context.Context, orgUnitID uuid.UUID) ([]*domain.Worker, error)
	GetAssignedTaskForWorker(ctx context.Context, workerID uuid.UUID) (*domain.Task, error)

	// API Keys
	CreateAPIKey(ctx context.Context, k *domain.APIKey) error
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID) error
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
	RotateAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID, newHash, newPrefix string) (*domain.APIKey, error)

	// Audit
	WriteAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	QueryAuditLog(ctx context.Context, orgUnitID uuid.UUID, from, to time.Time, limit, offset int) ([]*domain.AuditEvent, error)

	// Brownie leaderboard
	GetLeaderboard(ctx context.Context, orgUnitID uuid.UUID, limit int) ([]brownie.LeaderboardEntry, error)

	// Health
	Ping(ctx context.Context) error
}

// FullRedisStore is a convenience interface that the Redis store satisfies.
type FullRedisStore interface {
	SetWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, ttlSec int) error
	RateLimitIncr(ctx context.Context, key string, window time.Duration) (int64, error)
	Ping(ctx context.Context) error
}

// RouterDeps bundles every dependency required to build the router.
type RouterDeps struct {
	PGStore    FullStore
	RedisStore FullRedisStore

	// ConcreteStore is the concrete Postgres store used by handlers that
	// require direct access (OrgHandler, AuditHandler, BrownieHandler, OutputHandler).
	ConcreteStore *pgstore.Store

	// BrownieEng is the brownie points engine used by OutputHandler.
	BrownieEng *brownie.Engine

	// Explicit health pingers (usually the same objects as PGStore/RedisStore).
	DBPinger    handlers.Pinger
	RedisPinger handlers.Pinger

	// Dashboard.
	Hub         handlers.DashboardHubAdapter
	Snapshotter handlers.SnapshotSource

	// Task queue for enqueuing after creation.
	TaskQueue handlers.TaskQueue

	ArgonParams     security.ArgonParams
	HeartbeatTTLSec int
	RatePerMin      int
	HMACSecret      string
}

// NewRouter constructs and returns the fully configured chi.Router.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ──────────────────────────────────────────────────────
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(requestLogger())
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))

	// ── Health endpoints (no auth) ─────────────────────────────────────────────
	health := handlers.NewHealthHandler(deps.DBPinger, deps.RedisPinger)
	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)

	// ── Middleware instances ───────────────────────────────────────────────────
	authMiddleware := middleware.Auth(deps.PGStore, deps.PGStore)
	auditMiddleware := middleware.Audit(deps.PGStore)
	rateLimitMiddleware := middleware.RateLimit(deps.RedisStore, deps.RatePerMin)

	// ── Handler instances ──────────────────────────────────────────────────────
	scanner := security.NewDefaultScanner()
	taskHandler := handlers.NewTaskHandler(deps.PGStore, deps.TaskQueue, scanner)
	workerHandler := handlers.NewWorkerHandler(deps.PGStore, deps.RedisStore, deps.HeartbeatTTLSec)
	apiKeyHandler := handlers.NewAPIKeyHandler(deps.PGStore, deps.ArgonParams)
	dashboardHandler := handlers.NewDashboardHandler(deps.Hub, deps.Snapshotter)

	// Handlers that require the concrete postgres store (legacy coupling accepted).
	var (
		orgHandler    *handlers.OrgHandler
		auditHandler  *handlers.AuditHandler
		brownieHandler *handlers.BrownieHandler
		outputHandler *handlers.OutputHandler
	)
	if deps.ConcreteStore != nil {
		orgHandler = handlers.NewOrgHandler(deps.ConcreteStore, log.Logger)
		auditHandler = handlers.NewAuditHandler(deps.ConcreteStore, log.Logger)
		brownieHandler = handlers.NewBrownieHandler(deps.ConcreteStore, log.Logger)
		if deps.BrownieEng != nil {
			outputHandler = handlers.NewOutputHandler(deps.ConcreteStore, deps.BrownieEng, deps.HMACSecret, log.Logger)
		}
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(auditMiddleware)
		r.Use(rateLimitMiddleware)

		// ── Tasks ──────────────────────────────────────────────────────────────
		r.Route("/tasks", func(r chi.Router) {
			r.Get("/", taskHandler.ListTasks)
			r.With(middleware.ScopeRequired("tasks:write")).
				Post("/", taskHandler.CreateTask)
			r.Get("/{id}", taskHandler.GetTask)
			r.With(middleware.ScopeRequired("tasks:write")).
				Patch("/{id}/status", taskHandler.UpdateTaskStatus)
		})

		// ── Workers ────────────────────────────────────────────────────────────
		r.Route("/workers", func(r chi.Router) {
			r.Get("/", workerHandler.List)
			r.Get("/{id}", workerHandler.Get)
			r.With(middleware.ScopeRequired("workers:register")).
				Post("/register", workerHandler.Register)
			r.With(middleware.ScopeRequired("workers:heartbeat")).
				Post("/{id}/heartbeat", workerHandler.Heartbeat)
		})

		// ── API Keys ───────────────────────────────────────────────────────────
		r.Route("/api-keys", func(r chi.Router) {
			r.With(middleware.ScopeRequired("keys:write")).
				Post("/", apiKeyHandler.CreateAPIKey)
			r.With(middleware.ScopeRequired("keys:write")).
				Post("/{id}/rotate", apiKeyHandler.RotateAPIKey)
			r.With(middleware.ScopeRequired("keys:write")).
				Delete("/{id}", apiKeyHandler.RevokeAPIKey)
		})

		// ── Dashboard ──────────────────────────────────────────────────────────
		r.Route("/dashboard", func(r chi.Router) {
			r.Get("/stream", dashboardHandler.Stream)
			r.Get("/snapshot", dashboardHandler.Snapshot)
		})

		// ── Orgs (super-admin scope) ───────────────────────────────────────────
		if orgHandler != nil {
			r.Route("/orgs", func(r chi.Router) {
				r.With(middleware.ScopeRequired("admin")).Get("/", orgHandler.List)
				r.With(middleware.ScopeRequired("admin")).Post("/", orgHandler.Create)
				r.With(middleware.ScopeRequired("admin")).Get("/{id}", orgHandler.Get)
				r.With(middleware.ScopeRequired("admin")).Put("/{id}/policy", orgHandler.UpdatePolicy)
			})
		}

		// ── Audit log ─────────────────────────────────────────────────────────
		if auditHandler != nil {
			r.Route("/audit", func(r chi.Router) {
				r.Get("/", auditHandler.List)
			})
		}

		// ── Brownie Points ────────────────────────────────────────────────────
		if brownieHandler != nil {
			r.Route("/brownie", func(r chi.Router) {
				r.Get("/leaderboard", brownieHandler.Leaderboard)
			})
		}

		// ── Outputs ───────────────────────────────────────────────────────────
		if outputHandler != nil {
			r.Route("/outputs", func(r chi.Router) {
				r.With(middleware.ScopeRequired("workers:heartbeat")).
					Post("/", outputHandler.Submit)
				r.With(middleware.ScopeRequired("tasks:write")).
					Post("/{id}/review", outputHandler.Review)
			})
		}
	})

	return r
}

// requestLogger returns a zerolog-based request logging middleware.
func requestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("request_id", chimw.GetReqID(r.Context())).
				Msg("request")
			next.ServeHTTP(w, r)
		})
	}
}
