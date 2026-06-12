package gemini

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

const defaultModel = "gemini-2.5-pro"

// allowedOptions is the set of option keys that may be passed to the gemini CLI.
var allowedOptions = map[string]bool{
	"model": true,
}

// Adapter is the Gemini CLI agent adapter.
type Adapter struct {
	log zerolog.Logger
}

// New creates a new Gemini adapter.
func New(log zerolog.Logger) *Adapter {
	return &Adapter{
		log: log.With().Str("adapter", "gemini").Logger(),
	}
}

// ID returns the adapter identifier.
func (a *Adapter) ID() string {
	return "gemini"
}

// Version queries the gemini binary for its version.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	geminiPath, err := exec.LookPath("gemini")
	if err != nil {
		return "", fmt.Errorf("gemini binary not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, geminiPath, "--version")
	cmd.Env = minimalEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gemini --version failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Execute runs the Gemini CLI in the given workspace directory.
func (a *Adapter) Execute(ctx context.Context, req *adapters.ExecuteRequest) (*adapters.ExecuteResult, error) {
	geminiPath, err := exec.LookPath("gemini")
	if err != nil {
		return nil, fmt.Errorf("gemini binary not found on PATH: %w", err)
	}

	// Determine model from options or use default
	model := defaultModel
	if m, ok := req.Options["model"]; ok && allowedOptions["model"] {
		model = m
	}

	// Explicit argv — no shell interpolation
	argv := []string{
		"--model", model,
		"--prompt", req.Description,
	}

	// Log any non-allowlisted options that were ignored
	for k := range req.Options {
		if !allowedOptions[k] {
			a.log.Warn().Str("option", k).Msg("ignoring non-allowlisted gemini option")
		}
	}

	cmd := exec.CommandContext(ctx, geminiPath, argv...)
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
			return nil, fmt.Errorf("execute gemini: %w", err)
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

	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USERPROFILE", "GEMINI_API_KEY"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	return env
}
