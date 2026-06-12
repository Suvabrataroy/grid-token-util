package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/control"
)

// NewResumeCmd returns the `resume` subcommand.
func NewResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume the daemon from paused state",
		Long:  "Sends a resume command to the running daemon via the control socket.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume()
		},
	}
}

func runResume() error {
	resp, err := control.SendCommand(socketPath(), control.Command{Type: "resume"})
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	fmt.Println("daemon resumed")
	return nil
}
