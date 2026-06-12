package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/control"
)

// NewRevokeCmd returns the `revoke` subcommand.
func NewRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke",
		Short: "Immediately stop the current task and pause",
		Long:  "Sends a revoke command to the running daemon, which stops the current task and transitions to paused state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRevoke()
		},
	}
}

func runRevoke() error {
	resp, err := control.SendCommand(socketPath(), control.Command{Type: "revoke"})
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	fmt.Println("current task revoked, daemon pausing")
	return nil
}
