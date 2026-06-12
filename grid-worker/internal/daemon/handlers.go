package daemon

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	adapterChatGPT "github.com/grid-computing/grid-worker/internal/adapters/chatgpt"
	adapterClaude "github.com/grid-computing/grid-worker/internal/adapters/claude"
	adapterCopilot "github.com/grid-computing/grid-worker/internal/adapters/copilot"
	adapterGemini "github.com/grid-computing/grid-worker/internal/adapters/gemini"
	"github.com/grid-computing/grid-worker/internal/adapters"
	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/internal/controlplane"
	"github.com/grid-computing/grid-worker/internal/executor"
	"github.com/grid-computing/grid-worker/internal/reporter"
	"github.com/grid-computing/grid-worker/internal/scanner"
	"github.com/grid-computing/grid-worker/internal/workspace"
)

// statusResponse is the payload returned by the status command.
type statusResponse struct {
	State      string `json:"state"`
	PolicyMode string `json:"policy_mode"`
	WorkerID   string `json:"worker_id"`
}

// approvePayload is the payload for the approve command.
type approvePayload struct {
	TaskID string `json:"task_id"`
}

// handleStatusCmd returns the current daemon state.
func (d *Daemon) handleStatusCmd(_ json.RawMessage) (any, error) {
	return statusResponse{
		State:      d.fsm.String(),
		PolicyMode: d.cfg.Policy.Mode,
		WorkerID:   d.cfg.Server.WorkerID,
	}, nil
}

// handlePauseCmd transitions the daemon to PAUSED state.
func (d *Daemon) handlePauseCmd(_ json.RawMessage) (any, error) {
	current := d.fsm.State()
	if current != StateIdle {
		return nil, fmt.Errorf("can only pause from IDLE state (current: %s)", current)
	}
	if err := d.fsm.Transition(StateIdle, StatePaused); err != nil {
		return nil, err
	}
	d.cfg.Policy.Mode = "paused"
	return map[string]string{"state": "PAUSED"}, nil
}

// handleResumeCmd transitions the daemon from PAUSED back to IDLE.
func (d *Daemon) handleResumeCmd(_ json.RawMessage) (any, error) {
	current := d.fsm.State()
	if current != StatePaused {
		return nil, fmt.Errorf("can only resume from PAUSED state (current: %s)", current)
	}
	if err := d.fsm.Transition(StatePaused, StateIdle); err != nil {
		return nil, err
	}
	d.cfg.Policy.Mode = "auto"
	return map[string]string{"state": "IDLE"}, nil
}

// handleApproveCmd approves a pending manual task.
func (d *Daemon) handleApproveCmd(payload json.RawMessage) (any, error) {
	var p approvePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid approve payload: %w", err)
	}
	if p.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !d.approval.Approve(p.TaskID) {
		return nil, fmt.Errorf("task %q not found in pending approvals", p.TaskID)
	}
	return map[string]string{"approved": p.TaskID}, nil
}

// handleRevokeCmd stops the current task and transitions to PAUSED.
func (d *Daemon) handleRevokeCmd(_ json.RawMessage) (any, error) {
	if d.cancelFn != nil {
		d.log.Warn().Msg("revoke command received, cancelling current task")
	}
	return map[string]string{"state": "revoking"}, nil
}

// buildExecutor wires up the executor and all its dependencies.
func buildExecutor(cfg *config.Config, cpClient *controlplane.Client, log zerolog.Logger) (*executor.Executor, error) {
	// Build adapter registry
	reg := adapters.NewRegistry()
	reg.Register(adapterClaude.New(log))
	reg.Register(adapterCopilot.New(log))
	reg.Register(adapterGemini.New(log))
	reg.Register(adapterChatGPT.New(log))

	// Build workspace manager
	wsMgr := workspace.New(cfg.Workspace.BasePath, log)

	// Build scanner
	var ruleset *scanner.Ruleset
	if cfg.Security.RulesetPath != "" {
		var err error
		ruleset, err = scanner.LoadRuleset(cfg.Security.RulesetPath)
		if err != nil {
			return nil, fmt.Errorf("load scanner ruleset: %w", err)
		}
	} else {
		ruleset = scanner.DefaultRuleset()
	}
	sc := scanner.New(ruleset, cfg.Security.ScanConcurrency, log)

	// Build reporter
	rep := reporter.New(cpClient, log)

	// Build executor
	exec := executor.New(reg, wsMgr, sc, rep, cpClient, cfg, log)
	return exec, nil
}
