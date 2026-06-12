package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/adapters"
)

// allowedOptions is the set of option keys that may be passed to the claude CLI.
var allowedOptions = map[string]bool{
	"model":      true,
	"max-tokens": true,
}

// claudeOutput represents the JSON output structure from the Claude CLI.
type claudeOutput struct {
	Result    string   `json:"result"`
	Artifacts []string `json:"artifacts,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Adapter is the Claude Code AI agent adapter.
type Adapter struct {
	log zerolog.Logger
}

// New creates a new Claude adapter.
func New(log zerolog.Logger) *Adapter {
	return &Adapter{
		log: log.With().Str("adapter", "claude").Logger(),
	}
}

// ID returns the adapter identifier.
func (a *Adapter) ID() string {
	return "claude"
}

// Version queries the claude binary for its version.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude binary not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, claudePath, "--version")
	cmd.Env = minimalEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude --version failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Execute runs the Claude CLI in the given workspace directory.
func (a *Adapter) Execute(ctx context.Context, req *adapters.ExecuteRequest) (*adapters.ExecuteResult, error) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude binary not found on PATH: %w", err)
	}

	// Build argument list using explicit argv (never shell interpolation)
	argv := []string{
		"--no-interactive",
		"--output-format", "json",
		"-p", req.Description,
	}

	// Apply allowlisted options
	for k, v := range req.Options {
		if allowedOptions[k] {
			argv = append(argv, "--"+k, v)
		} else {
			a.log.Warn().Str("option", k).Msg("ignoring non-allowlisted claude option")
		}
	}

	cmd := exec.CommandContext(ctx, claudePath, argv...)
	cmd.Dir = req.WorkspacePath
	cmd.Env = minimalEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execute claude: %w", err)
		}
	}

	result := &adapters.ExecuteResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	// Parse JSON output for artifacts if exit was successful
	if exitCode == 0 && len(stdout.Bytes()) > 0 {
		var output claudeOutput
		if jsonErr := json.Unmarshal(stdout.Bytes(), &output); jsonErr == nil {
			result.Artifacts = output.Artifacts
		}
	}

	return result, nil
}

// minimalEnv returns a minimal environment for the subprocess containing only
// PATH, HOME, and TMPDIR to avoid leaking sensitive environment variables.
func minimalEnv() []string {
	env := make([]string, 0, 3)

	if v := os.Getenv("PATH"); v != "" {
		env = append(env, "PATH="+v)
	}
	if v := os.Getenv("HOME"); v != "" {
		env = append(env, "HOME="+v)
	}
	if v := os.Getenv("TMPDIR"); v != "" {
		env = append(env, "TMPDIR="+v)
	} else if v := os.Getenv("TMP"); v != "" {
		env = append(env, "TMP="+v)
	} else if v := os.Getenv("TEMP"); v != "" {
		env = append(env, "TEMP="+v)
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		// Windows equivalent of HOME
		env = append(env, "USERPROFILE="+v)
	}

	return env
}
