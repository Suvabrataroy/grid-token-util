package chatgpt

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

// Adapter is the ChatGPT/sgpt CLI agent adapter.
type Adapter struct {
	log zerolog.Logger
}

// New creates a new ChatGPT adapter using the sgpt CLI.
func New(log zerolog.Logger) *Adapter {
	return &Adapter{
		log: log.With().Str("adapter", "chatgpt").Logger(),
	}
}

// ID returns the adapter identifier.
func (a *Adapter) ID() string {
	return "chatgpt"
}

// Version queries the sgpt binary for its version.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	sgptPath, err := exec.LookPath("sgpt")
	if err != nil {
		return "", fmt.Errorf("sgpt binary not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, sgptPath, "--version")
	cmd.Env = minimalEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sgpt --version failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Execute runs the sgpt CLI in the given workspace directory.
func (a *Adapter) Execute(ctx context.Context, req *adapters.ExecuteRequest) (*adapters.ExecuteResult, error) {
	sgptPath, err := exec.LookPath("sgpt")
	if err != nil {
		return nil, fmt.Errorf("sgpt binary not found on PATH: %w", err)
	}

	// Explicit argv — no shell interpolation
	argv := []string{
		"--code",
		req.Description,
	}

	cmd := exec.CommandContext(ctx, sgptPath, argv...)
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
			return nil, fmt.Errorf("execute sgpt: %w", err)
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

	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USERPROFILE", "OPENAI_API_KEY"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	return env
}
