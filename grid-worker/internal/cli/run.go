package cli

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/daemon"
)

// NewRunCmd returns the `run` subcommand that starts the daemon in the foreground.
func NewRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the grid-worker daemon in the foreground",
		Long:  "Start the grid-worker daemon directly (not as a system service). Used by the service manager.",
		RunE:  runDaemon,
	}
}

func runDaemon(cmd *cobra.Command, _ []string) error {
	if cfg == nil {
		return fmt.Errorf("configuration not loaded")
	}

	d, err := daemon.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("create daemon: %w", err)
	}

	svcConfig := &service.Config{
		Name:        "grid-worker",
		DisplayName: "Grid Worker Daemon",
		Description: "Grid Worker distributed AI coding agent daemon",
	}

	svc, err := service.New(d, svcConfig)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	if service.Interactive() {
		// Running interactively — start directly
		if err := d.Start(svc); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
		// Block until the process is interrupted
		select {}
	}

	// Running as a service
	if err := svc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "service run error: %v\n", err)
		os.Exit(1)
	}

	return nil
}
