package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	gwservice "github.com/grid-computing/grid-worker/internal/service"
)

// NewInstallCmd returns the `install` subcommand.
func NewInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install grid-worker as a system service",
		Long:  "Installs the grid-worker daemon as a system service (systemd, launchd, or Windows Service).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall()
		},
	}
}

func runInstall() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determine executable path: %w", err)
	}

	if err := gwservice.Install(cfg, execPath); err != nil {
		return fmt.Errorf("install service: %w", err)
	}

	fmt.Println("grid-worker installed as system service")
	fmt.Println("Start it with: grid-worker-ctl start  (or: systemctl start grid-worker)")
	return nil
}
