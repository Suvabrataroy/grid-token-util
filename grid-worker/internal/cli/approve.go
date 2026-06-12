package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/control"
)

// NewApproveCmd returns the `approve` subcommand.
func NewApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <task-id>",
		Short: "Approve a pending manual task",
		Long:  "Approves a task that is waiting for manual approval (policy.mode = manual).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApprove(args[0])
		},
	}
}

func runApprove(taskID string) error {
	payload, err := json.Marshal(map[string]string{"task_id": taskID})
	if err != nil {
		return err
	}

	resp, err := control.SendCommand(socketPath(), control.Command{
		Type:    "approve",
		Payload: json.RawMessage(payload),
	})
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	fmt.Printf("task %q approved\n", taskID)
	return nil
}
