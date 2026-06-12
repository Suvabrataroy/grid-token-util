package copilot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/adapters"
)

// Adapter is the GitHub Copilot CLI agent adapter.
type Adapter struct {
	log zerolog.Logger
}

// New creates a new Copilot adapter.
func New(log zerolog.Logger) *Adapter {
	return &Adapter{
		log: log.With().Str("adapter", "copilot").Logger(),
	}
}

// ID returns the adapter identifier.
func (a *Adapter) ID() string {
	return "copilot"
}

// Version queries the gh CLI for its version (Copilot is a gh extension).
func (a *Adapter) Version(ctx context.Context) (string, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return "", fmt.Errorf("gh binary not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, ghPath, "--version")
	cmd.Env = minimalEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh --version failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Execute runs the GitHub Copilot CLI in the given workspace directory.
// It uses the `gh copilot suggest` subcommand.
func (a *Adapter) Execute(ctx context.Context, req *adapters.ExecuteRequest) (*adapters.ExecuteResult, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("gh binary not found on PATH: %w", err)
	}

	// Explicit argv — no shell, no string interpolation
	argv := []string{
		"copilot",
		"suggest",
		"-t", "shell",
		req.Description,
	}

	cmd := exec.CommandContext(ctx, ghPath, argv...)
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
			return nil, fmt.Errorf("execute gh copilot: %w", err)
		}
	}

	return &adapters.ExecuteResult{
		ExitCode:  exitCode,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Artifacts: []string{},
	}, nil
}

// minimalEnv returns a minimal environment for the subprocess.
func minimalEnv() []string {
	env := make([]string, 0, 4)

	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USERPROFILE", "GH_TOKEN"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	return env
}
