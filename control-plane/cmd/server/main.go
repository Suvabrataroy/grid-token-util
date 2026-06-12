// Command server is the control-plane HTTP server for the AI coding grid.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/api"
	"github.com/grid-computing/control-plane/internal/brownie"
	"github.com/grid-computing/control-plane/internal/config"
	"github.com/grid-computing/control-plane/internal/dashboard"
	"github.com/grid-computing/control-plane/internal/queue"
	"github.com/grid-computing/control-plane/internal/scheduler"
	"github.com/grid-computing/control-plane/internal/security"
	pgstore "github.com/grid-computing/control-plane/internal/store/postgres"
	redisstore "github.com/grid-computing/control-plane/internal/store/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("server: fatal error")
	}
}

func run() error {
	// ── Config ────────────────────────────────────────────────────────────────
	cfgFile := os.Getenv("GRID_CONFIG_FILE")
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── Logging ───────────────────────────────────────────────────────────────
	setupLogging(cfg.Logging)

	log.Info().Str("version", "1.0.0").Msg("control-plane: starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Postgres ──────────────────────────────────────────────────────────────
	pg, err := pgstore.New(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pg.Close()
	log.Info().Msg("postgres: connected")

	// ── Redis ─────────────────────────────────────────────────────────────────
	redis, err := redisstore.New(cfg.Redis)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer func() { _ = redis.Close() }()
	if err := redis.Ping(ctx); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	log.Info().Msg("redis: connected")

	// ── Task queue ────────────────────────────────────────────────────────────
	taskQueue := queue.New(redis.Client())

	// ── Brownie engine ────────────────────────────────────────────────────────
	brownieEngine := brownie.NewEngine(pg)

	// ── SSE Hub ───────────────────────────────────────────────────────────────
	hub := dashboard.NewHub(cfg.Dashboard.MaxSSEClients)

	// ── Scheduler ─────────────────────────────────────────────────────────────
	sched := scheduler.NewScheduler(
		pg, pg, pg, taskQueue,
		cfg.Scheduler.TickIntervalSec,
	)
	sched.Start(ctx)
	defer sched.Stop()

	// ── Reaper ────────────────────────────────────────────────────────────────
	reaper := scheduler.NewReaper(
		pg, pg, redis, brownieEngine,
		cfg.Scheduler.HeartbeatTTLSec,
		cfg.Scheduler.ReaperIntervalSec,
	)
	reaper.Start(ctx)
	defer reaper.Stop()

	// ── Argon2id parameters ───────────────────────────────────────────────────
	argonParams := security.ArgonParams{
		Memory:      cfg.Security.ArgonMemory,
		Iterations:  cfg.Security.ArgonIterations,
		Parallelism: cfg.Security.ArgonParallelism,
		SaltLen:     cfg.Security.ArgonSaltLen,
		KeyLen:      cfg.Security.ArgonKeyLen,
	}

	// ── HTTP Router ───────────────────────────────────────────────────────────
	deps := api.RouterDeps{
		PGStore:         pg,
		RedisStore:      redis,
		ConcreteStore:   pg,
		BrownieEng:      brownieEngine,
		DBPinger:        pg,
		RedisPinger:     redis,
		Hub:             hub,
		Snapshotter:     pg,
		TaskQueue:       taskQueue,
		ArgonParams:     argonParams,
		HeartbeatTTLSec: cfg.Scheduler.HeartbeatTTLSec,
		RatePerMin:      300,
		HMACSecret:      cfg.Security.HMACSecret,
	}
	handler := api.NewRouter(deps)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// TLS if configured.
	if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" {
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS13,
		}
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", addr).Msg("http: listening")
		if cfg.Server.TLSCertFile != "" {
			errCh <- srv.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("shutdown: signal received")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown: http server shutdown error")
	}

	log.Info().Msg("control-plane: shutdown complete")
	return nil
}

func setupLogging(cfg config.LoggingConfig) {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.Format == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
}
