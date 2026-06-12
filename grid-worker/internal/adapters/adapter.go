package adapters

import "context"

// ExecuteRequest is the input to an agent adapter's Execute method.
type ExecuteRequest struct {
	WorkspacePath string
	TaskType      string
	Description   string
	Options       map[string]string // allowlisted keys only
}

// ExecuteResult is the output of an agent adapter's Execute method.
type ExecuteResult struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Artifacts []string // relative paths to files created/modified
}

// AgentAdapter is the interface implemented by all AI agent adapters.
type AgentAdapter interface {
	// ID returns the unique identifier for this adapter (e.g., "claude", "copilot").
	ID() string

	// Version queries the agent binary for its version string.
	Version(ctx context.Context) (string, error)

	// Execute runs the agent with the given request and returns the result.
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error)
}
