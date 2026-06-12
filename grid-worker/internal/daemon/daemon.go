package daemon

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/kardianos/service"
	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/internal/control"
	"github.com/grid-computing/grid-worker/internal/controlplane"
	"github.com/grid-computing/grid-worker/internal/executor"
	"github.com/grid-computing/grid-worker/internal/policy"
	"github.com/grid-computing/grid-worker/internal/preflight"
	"github.com/grid-computing/grid-worker/pkg/platform"
)

// Daemon is the main orchestrator implementing the kardianos/service interface.
type Daemon struct {
	cfg       *config.Config
	fsm       *FSM
	precheck  *preflight.Runner
	heartbeat *controlplane.HeartbeatLoop
	exec      *executor.Executor
	policyEng *policy.Engine
	approval  *policy.ApprovalStore
	control   *control.Server
	log       zerolog.Logger

	cancelFn    context.CancelFunc
	executing   atomic.Bool
	taskChannel chan *controlplane.TaskAssignment
}

// New constructs a fully-wired Daemon from the given config.
func New(cfg *config.Config, log zerolog.Logger) (*Daemon, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if err := platform.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("ensure dirs: %w", err)
	}

	// Control plane client
	cpClient := controlplane.New(cfg.Server, log)

	// Build sub-components (wiring deferred to run time)
	fsm := NewFSM(log)
	policyEng := policy.New(cfg, log)
	approvalStore := policy.NewApprovalStore()
	preRunner := preflight.New(cfg, cpClient, log)
	controlServer := control.New(platform.SocketPath(), log)

	d := &Daemon{
		cfg:         cfg,
		fsm:         fsm,
		precheck:    preRunner,
		policyEng:   policyEng,
		approval:    approvalStore,
		control:     controlServer,
		log:         log.With().Str("component", "daemon").Logger(),
		taskChannel: make(chan *controlplane.TaskAssignment, 1),
	}

	// Build executor (depends on several sub-components)
	exec, err := buildExecutor(cfg, cpClient, log)
	if err != nil {
		return nil, fmt.Errorf("build executor: %w", err)
	}
	d.exec = exec

	// Build heartbeat loop
	d.heartbeat = controlplane.NewHeartbeatLoop(
		cpClient,
		d.collectStats,
		d.onTaskAssigned,
		cfg.Server.HeartbeatSec,
		log,
	)

	// Register control socket command handlers
	d.registerControlHandlers(controlServer, approvalStore)

	return d, nil
}

// Start implements service.Interface — called by kardianos/service when the daemon starts.
func (d *Daemon) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelFn = cancel

	go d.run(ctx)
	return nil
}

// Stop implements service.Interface — called by kardianos/service on shutdown.
func (d *Daemon) Stop(_ service.Service) error {
	d.log.Info().Msg("service stop requested")
	if d.cancelFn != nil {
		d.cancelFn()
	}
	d.control.Stop()
	return nil
}

// run is the main daemon loop.
func (d *Daemon) run(ctx context.Context) {
	HandleSignals(ctx, d.cancelFn, d.onConfigReload, d.log)

	// Step 1: Run preflight (INIT → PREFLIGHT → IDLE or SHUTDOWN)
	if err := d.fsm.Transition(StateInit, StatePreflight); err != nil {
		d.log.Fatal().Err(err).Msg("FSM transition to PREFLIGHT failed")
	}

	d.log.Info().Msg("running preflight checks")
	results, err := d.precheck.RunAll(ctx)
	if err != nil {
		d.log.Fatal().Err(err).Msg("preflight runner error")
	}

	if preflight.HasFailures(results) {
		d.log.Fatal().Msg("preflight checks failed, shutting down")
		_ = d.fsm.Transition(StatePreflight, StateShutdown)
		return
	}

	if err := d.fsm.Transition(StatePreflight, StateIdle); err != nil {
		d.log.Fatal().Err(err).Msg("FSM transition to IDLE failed")
	}

	d.log.Info().Msg("preflight passed, daemon is IDLE")

	// Step 2: Start control socket server
	go func() {
		if err := d.control.Start(ctx); err != nil {
			d.log.Error().Err(err).Msg("control socket server error")
		}
	}()

	// Step 3: Start heartbeat loop
	go d.heartbeat.Start(ctx)

	// Step 4: Main task execution loop
	for {
		select {
		case <-ctx.Done():
			d.log.Info().Msg("context cancelled, initiating shutdown")
			d.shutdown()
			return

		case task := <-d.taskChannel:
			d.handleTask(ctx, task)
		}
	}
}

// handleTask processes a task assignment through the policy engine and executor.
func (d *Daemon) handleTask(ctx context.Context, task *controlplane.TaskAssignment) {
	// Check current FSM state
	currentState := d.fsm.State()
	if currentState != StateIdle {
		d.log.Warn().
			Str("state", string(currentState)).
			Str("task_id", task.TaskID).
			Msg("received task but not in IDLE state, deferring")
		return
	}

	// Policy evaluation
	decision := d.policyEng.Evaluate(task)
	if !decision.Allow {
		// Check if manual mode and approved
		if d.cfg.Policy.Mode == "manual" && d.approval.IsApproved(task.TaskID) {
			d.approval.ConsumeApproval(task.TaskID)
			// Proceed to execute
		} else {
			d.log.Info().
				Str("task_id", task.TaskID).
				Str("reason", decision.Reason).
				Msg("task denied by policy")
			return
		}
	}

	// Transition to EXECUTING
	if err := d.fsm.Transition(StateIdle, StateExecuting); err != nil {
		d.log.Error().Err(err).Msg("FSM transition to EXECUTING failed")
		return
	}
	d.executing.Store(true)

	// Execute task
	go func() {
		defer func() {
			d.executing.Store(false)
			if transErr := d.fsm.Transition(StateExecuting, StateIdle); transErr != nil {
				d.log.Error().Err(transErr).Msg("FSM transition back to IDLE failed")
			}
		}()

		d.log.Info().Str("task_id", task.TaskID).Msg("beginning task execution")
		if execErr := d.exec.Execute(ctx, task); execErr != nil {
			d.log.Error().Err(execErr).Str("task_id", task.TaskID).Msg("task execution failed")
		}
	}()
}

// onTaskAssigned is the callback invoked by the heartbeat loop when a task arrives.
func (d *Daemon) onTaskAssigned(task *controlplane.TaskAssignment) {
	select {
	case d.taskChannel <- task:
	default:
		d.log.Warn().
			Str("task_id", task.TaskID).
			Msg("task channel full, dropping task")
	}
}

// collectStats gathers current worker metrics for the heartbeat.
func (d *Daemon) collectStats() controlplane.HeartbeatRequest {
	batteryPct, _ := platform.BatteryPercent()

	req := controlplane.HeartbeatRequest{
		State:          d.fsm.String(),
		BatteryPercent: batteryPct,
	}

	return req
}

// onConfigReload handles a SIGHUP by reloading the configuration file.
func (d *Daemon) onConfigReload() {
	d.log.Info().Msg("reloading configuration")
	newCfg, err := config.Load("")
	if err != nil {
		d.log.Error().Err(err).Msg("config reload failed")
		return
	}
	d.cfg = newCfg
	d.log.Info().Msg("configuration reloaded")
}

// shutdown performs graceful drain: waits for current task to finish before halting.
func (d *Daemon) shutdown() {
	d.log.Info().Msg("shutting down gracefully")

	// Wait for executing task to complete (with a short timeout)
	if d.executing.Load() {
		d.log.Info().Msg("waiting for current task to complete before shutdown")
	}

	currentState := d.fsm.State()
	if currentState != StateShutdown {
		_ = d.fsm.Transition(currentState, StateShutdown)
	}
}

// registerControlHandlers wires up IPC command handlers on the control socket server.
func (d *Daemon) registerControlHandlers(srv *control.Server, approvalStore *policy.ApprovalStore) {
	srv.Register("status", d.handleStatusCmd)
	srv.Register("pause", d.handlePauseCmd)
	srv.Register("resume", d.handleResumeCmd)
	srv.Register("approve", d.handleApproveCmd)
	srv.Register("revoke", d.handleRevokeCmd)
}
