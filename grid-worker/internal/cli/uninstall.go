package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	gwservice "github.com/grid-computing/grid-worker/internal/service"
)

// NewUninstallCmd returns the `uninstall` subcommand.
func NewUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the grid-worker system service",
		Long:  "Removes the grid-worker daemon system service (systemd, launchd, or Windows Service).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall()
		},
	}
}

func runUninstall() error {
	if err := gwservice.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}

	fmt.Println("grid-worker system service removed")
	return nil
}
