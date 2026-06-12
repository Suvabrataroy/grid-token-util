package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/adapters"
	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/internal/controlplane"
	"github.com/grid-computing/grid-worker/internal/reporter"
	"github.com/grid-computing/grid-worker/internal/scanner"
	"github.com/grid-computing/grid-worker/internal/workspace"
)

// Executor orchestrates the full lifecycle of a single task execution.
type Executor struct {
	registry  *adapters.Registry
	workspace *workspace.Manager
	scanner   *scanner.Scanner
	reporter  *reporter.Reporter
	client    *controlplane.Client
	cfg       *config.Config
	log       zerolog.Logger
}

// New creates a new Executor.
func New(
	registry *adapters.Registry,
	ws *workspace.Manager,
	sc *scanner.Scanner,
	rep *reporter.Reporter,
	client *controlplane.Client,
	cfg *config.Config,
	log zerolog.Logger,
) *Executor {
	return &Executor{
		registry:  registry,
		workspace: ws,
		scanner:   sc,
		reporter:  rep,
		client:    client,
		cfg:       cfg,
		log:       log.With().Str("component", "executor").Logger(),
	}
}

// Execute runs a task through its complete lifecycle:
// 1. Create workspace
// 2. Clone repo (if RepoURL set)
// 3. Pre-exec secret scan (fail on critical findings)
// 4. Get adapter and run with timeout
// 5. Post-exec patch scan (fail on critical findings)
// 6. Pack output
// 7. Report output
// 8. Cleanup workspace
func (e *Executor) Execute(ctx context.Context, task *controlplane.TaskAssignment) error {
	taskLog := e.log.With().Str("task_id", task.TaskID).Str("agent", task.AIAgent).Logger()

	taskLog.Info().Str("title", task.Title).Msg("starting task execution")

	// Determine timeout
	timeoutSec := task.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = e.cfg.Execution.TimeoutSec
	}
	if timeoutSec <= 0 {
		timeoutSec = 3600
	}

	taskCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Step 1: Create workspace
	e.updateStatus(ctx, task.TaskID, "preparing", "")
	wsPath, err := e.workspace.Create(task.TaskID)
	if err != nil {
		e.updateStatus(ctx, task.TaskID, "failed", fmt.Sprintf("create workspace: %v", err))
		return fmt.Errorf("create workspace: %w", err)
	}

	// Ensure cleanup on exit if configured
	if e.cfg.Workspace.CleanupAfter {
		defer func() {
			if cleanupErr := e.workspace.Cleanup(task.TaskID); cleanupErr != nil {
				taskLog.Warn().Err(cleanupErr).Msg("workspace cleanup failed")
			}
		}()
	}

	// Step 2: Clone repo
	if task.RepoURL != "" {
		e.updateStatus(ctx, task.TaskID, "cloning", "")
		taskLog.Info().Str("repo", task.RepoURL).Str("branch", task.Branch).Msg("cloning repository")

		if err := workspace.CloneRepo(taskCtx, task.RepoURL, task.Branch, wsPath); err != nil {
			e.updateStatus(ctx, task.TaskID, "failed", fmt.Sprintf("clone repo: %v", err))
			return fmt.Errorf("clone repo: %w", err)
		}
	}

	// Step 3: Pre-execution secret scan
	if e.cfg.Security.ScanEnabled {
		e.updateStatus(ctx, task.TaskID, "scanning_pre", "")
		taskLog.Info().Msg("running pre-execution secret scan")

		findings, err := e.scanner.ScanDirectory(taskCtx, wsPath)
		if err != nil {
			taskLog.Warn().Err(err).Msg("pre-execution scan error (non-fatal)")
		} else {
			if hasCritical(findings) {
				msg := fmt.Sprintf("pre-execution scan found critical secrets: %s", formatFindings(findings))
				e.updateStatus(ctx, task.TaskID, "failed", msg)
				return fmt.Errorf("pre-execution scan: critical secrets found in repository")
			}
			if len(findings) > 0 {
				taskLog.Warn().Int("count", len(findings)).Msg("pre-execution scan: non-critical findings")
			}
		}
	}

	// Step 4: Get adapter and execute
	adapter, ok := e.registry.Get(task.AIAgent)
	if !ok {
		msg := fmt.Sprintf("no adapter registered for agent %q", task.AIAgent)
		e.updateStatus(ctx, task.TaskID, "failed", msg)
		return fmt.Errorf(msg)
	}

	// Start resource monitor
	execCtx, execCancel := context.WithCancel(taskCtx)
	monitor := NewMonitor(
		e.cfg.Execution.MaxCPUPercent,
		e.cfg.Execution.MaxRAMMB,
		func() {
			taskLog.Error().Msg("resource limit exceeded, killing task")
			execCancel()
		},
		taskLog,
	)
	go monitor.Start(execCtx)
	defer execCancel()

	// Start disk quota monitor
	quotaMonitor := workspace.NewQuotaMonitor(wsPath, e.cfg.Workspace.DiskQuotaGB, func() {
		taskLog.Error().Msg("disk quota exceeded, killing task")
		execCancel()
	}, taskLog)
	go quotaMonitor.Start(execCtx)

	e.updateStatus(ctx, task.TaskID, "executing", "")
	taskLog.Info().Str("adapter", task.AIAgent).Msg("executing task with agent")

	execReq := &adapters.ExecuteRequest{
		WorkspacePath: wsPath,
		TaskType:      task.TaskType,
		Description:   task.Description,
		Options:       task.Options,
	}

	execResult, err := adapter.Execute(execCtx, execReq)
	execCancel() // stop monitors

	if err != nil {
		e.updateStatus(ctx, task.TaskID, "failed", fmt.Sprintf("execution error: %v", err))
		return fmt.Errorf("execute agent: %w", err)
	}

	if execResult.ExitCode != 0 {
		taskLog.Warn().
			Int("exit_code", execResult.ExitCode).
			Str("stderr", truncate(execResult.Stderr, 512)).
			Msg("agent exited with non-zero status")
	}

	// Step 5: Post-execution patch scan
	if e.cfg.Security.ScanEnabled && execResult.Stdout != "" {
		e.updateStatus(ctx, task.TaskID, "scanning_post", "")
		taskLog.Info().Msg("running post-execution output scan")

		findings, err := e.scanner.ScanPatch(ctx, execResult.Stdout)
		if err != nil {
			taskLog.Warn().Err(err).Msg("post-execution scan error (non-fatal)")
		} else if hasCritical(findings) {
			msg := fmt.Sprintf("post-execution scan found critical secrets in output: %s", formatFindings(findings))
			e.updateStatus(ctx, task.TaskID, "failed", msg)
			return fmt.Errorf("post-execution scan: critical secrets found in output")
		}
	}

	// Step 6: Pack output
	e.updateStatus(ctx, task.TaskID, "packing", "")

	pkg, err := reporter.Pack(
		task.TaskID,
		e.cfg.Server.WorkerID,
		wsPath,
		e.cfg.Security.HMACSecret,
		execResult.Artifacts,
	)
	if err != nil {
		e.updateStatus(ctx, task.TaskID, "failed", fmt.Sprintf("pack output: %v", err))
		return fmt.Errorf("pack output: %w", err)
	}

	// Add execution metadata
	pkg.Metadata["exit_code"] = execResult.ExitCode
	pkg.Metadata["task_type"] = task.TaskType
	pkg.Metadata["agent"] = task.AIAgent

	// Step 7: Report output
	e.updateStatus(ctx, task.TaskID, "reporting", "")

	if err := e.reporter.Submit(ctx, pkg); err != nil {
		e.updateStatus(ctx, task.TaskID, "failed", fmt.Sprintf("report output: %v", err))
		return fmt.Errorf("report output: %w", err)
	}

	// Step 8: Update final status
	e.updateStatus(ctx, task.TaskID, "completed", "")
	taskLog.Info().Msg("task completed successfully")

	return nil
}

// updateStatus sends a task status update to the control plane, logging errors.
// Internal intermediate states are mapped to the API-valid "running" state.
func (e *Executor) updateStatus(ctx context.Context, taskID, state, errMsg string) {
	apiState := state
	switch state {
	case "preparing", "cloning", "scanning_pre", "executing", "scanning_post", "packing", "reporting":
		apiState = "running"
	}
	req := &controlplane.TaskStatusRequest{
		State:        apiState,
		ErrorMessage: errMsg,
	}
	if err := e.client.UpdateTaskStatus(ctx, taskID, req); err != nil {
		e.log.Warn().Err(err).Str("task_id", taskID).Str("state", state).Msg("failed to update task status")
	}
}

// hasCritical returns true if any finding has critical severity.
func hasCritical(findings []scanner.Finding) bool {
	for _, f := range findings {
		if f.Severity == "critical" {
			return true
		}
	}
	return false
}

// formatFindings returns a short summary of findings.
func formatFindings(findings []scanner.Finding) string {
	if len(findings) == 0 {
		return "none"
	}
	result := ""
	for i, f := range findings {
		if i > 0 {
			result += "; "
		}
		result += fmt.Sprintf("%s (%s) at %s:%d", f.RuleID, f.Severity, f.FilePath, f.Line)
		if i >= 4 {
			result += fmt.Sprintf(" and %d more", len(findings)-5)
			break
		}
	}
	return result
}

// truncate returns s truncated to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
