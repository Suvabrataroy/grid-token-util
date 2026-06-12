package custom

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/adapters"
)

// Config holds the configuration for a custom adapter.
type Config struct {
	// CommandTemplate is a Go text/template string that renders to the command to execute.
	// Available template variables:
	//   .Description - the task description
	//   .WorkspacePath - the workspace directory
	//   .Options - map of allowlisted options
	CommandTemplate string

	// AllowedOptions lists the option keys that may be used in the template.
	AllowedOptions []string
}

// Adapter is a configurable custom AI agent adapter.
type Adapter struct {
	config Config
	tmpl   *template.Template
	log    zerolog.Logger
}

// templateData is passed to the command template.
type templateData struct {
	Description   string
	WorkspacePath string
	Options       map[string]string
}

// New creates a new custom adapter from the given config.
func New(cfg Config, log zerolog.Logger) (*Adapter, error) {
	tmpl, err := template.New("command").Parse(cfg.CommandTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse command template: %w", err)
	}

	return &Adapter{
		config: cfg,
		tmpl:   tmpl,
		log:    log.With().Str("adapter", "custom").Logger(),
	}, nil
}

// ID returns the adapter identifier.
func (a *Adapter) ID() string {
	return "custom"
}

// Version returns a placeholder version since custom adapters are user-defined.
func (a *Adapter) Version(_ context.Context) (string, error) {
	return "custom-adapter-v1", nil
}

// Execute renders the command template and runs the resulting command.
// Only allowlisted option keys are passed through to the template.
func (a *Adapter) Execute(ctx context.Context, req *adapters.ExecuteRequest) (*adapters.ExecuteResult, error) {
	// Build allowlisted options map
	allowed := make(map[string]bool, len(a.config.AllowedOptions))
	for _, k := range a.config.AllowedOptions {
		allowed[k] = true
	}

	filteredOptions := make(map[string]string)
	for k, v := range req.Options {
		if allowed[k] {
			filteredOptions[k] = v
		} else {
			a.log.Warn().Str("option", k).Msg("ignoring non-allowlisted custom option")
		}
	}

	data := templateData{
		Description:   req.Description,
		WorkspacePath: req.WorkspacePath,
		Options:       filteredOptions,
	}

	var rendered bytes.Buffer
	if err := a.tmpl.Execute(&rendered, data); err != nil {
		return nil, fmt.Errorf("render command template: %w", err)
	}

	// Split the rendered command into argv.
	// The template must render to a valid command line with space-separated args.
	// For safety, we parse it as fields (not shell-expanded).
	parts := strings.Fields(rendered.String())
	if len(parts) == 0 {
		return nil, fmt.Errorf("command template rendered to empty string")
	}

	binaryPath, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("custom binary %q not found on PATH: %w", parts[0], err)
	}

	cmd := exec.CommandContext(ctx, binaryPath, parts[1:]...)
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
			return nil, fmt.Errorf("execute custom command: %w", err)
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

	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USERPROFILE"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	return env
}
