package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/control"
)

// NewStatusCmd returns the `status` subcommand.
func NewStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		Long:  "Connect to the running daemon via the control socket and print FSM state, current task, and stats.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func runStatus(jsonOutput bool) error {
	resp, err := control.SendCommand(socketPath(), control.Command{Type: "status"})
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	if jsonOutput {
		fmt.Println(string(resp.Data))
		return nil
	}

	// Parse and display as table
	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Println(string(resp.Data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tVALUE")
	fmt.Fprintln(w, "-----\t-----")
	for k, v := range data {
		fmt.Fprintf(w, "%s\t%v\n", k, v)
	}
	return w.Flush()
}
