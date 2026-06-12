package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/controlplane"
	"github.com/grid-computing/grid-worker/internal/preflight"
)

// NewPreflightCmd returns the `preflight` subcommand.
func NewPreflightCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Run pre-flight checks",
		Long:  "Execute all pre-flight checks and display results. Useful for diagnosing setup issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreflight(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func runPreflight(jsonOutput bool) error {
	if cfg == nil {
		return fmt.Errorf("configuration not loaded")
	}

	ctx := cmd_ctx()

	cpClient := controlplane.New(cfg.Server, logger)
	runner := preflight.New(cfg, cpClient, logger)

	results, err := runner.RunAll(ctx)
	if err != nil {
		return fmt.Errorf("preflight runner error: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tMESSAGE")
	fmt.Fprintln(w, "--\t----\t------\t-------")

	hasFailures := false
	for _, r := range results {
		statusIcon := "✓"
		switch r.Status {
		case "fail":
			statusIcon = "✗"
			hasFailures = true
		case "warn":
			statusIcon = "!"
		}
		fmt.Fprintf(w, "%s\t%s\t%s %s\t%s\n", r.ID, r.Name, statusIcon, r.Status, r.Message)
	}
	w.Flush()

	if hasFailures {
		fmt.Fprintln(os.Stderr, "\nPre-flight checks FAILED. Please fix the issues above before starting the daemon.")
		os.Exit(1)
	}

	return nil
}
