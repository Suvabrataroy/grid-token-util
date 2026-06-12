package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/control"
)

// NewPauseCmd returns the `pause` subcommand.
func NewPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause the daemon (stop accepting new tasks)",
		Long:  "Sends a pause command to the running daemon via the control socket.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPause()
		},
	}
}

func runPause() error {
	resp, err := control.SendCommand(socketPath(), control.Command{Type: "pause"})
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	fmt.Println("daemon paused")
	return nil
}
