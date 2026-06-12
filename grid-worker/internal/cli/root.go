package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/pkg/platform"
)

var (
	cfgFile string
	verbose bool
	cfg     *config.Config
	logger  zerolog.Logger
)

// NewRootCmd creates and returns the root Cobra command.
func NewRootCmd(version, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:   "grid-worker",
		Short: "Grid Worker — distributed AI coding agent daemon",
		Long: `grid-worker is a client daemon that executes AI coding tasks
dispatched by a grid control plane. It manages workspaces, runs
AI agents (Claude, Copilot, Gemini, ChatGPT), scans for secrets,
and reports results back to the control plane.`,
		Version:          fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config load for completion and help commands
			if cmd.Name() == "completion" || cmd.Name() == "help" {
				return nil
			}
			return initConfig()
		},
	}

	// Persistent flags available to all subcommands
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.grid-worker/config.yaml)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose (debug) logging")

	// Add subcommands
	root.AddCommand(NewRunCmd())
	root.AddCommand(NewStatusCmd())
	root.AddCommand(NewPauseCmd())
	root.AddCommand(NewResumeCmd())
	root.AddCommand(NewApproveCmd())
	root.AddCommand(NewRevokeCmd())
	root.AddCommand(NewLogsCmd())
	root.AddCommand(NewPreflightCmd())
	root.AddCommand(NewSetKeyCmd())
	root.AddCommand(NewInstallCmd())
	root.AddCommand(NewUninstallCmd())

	return root
}

// initConfig loads configuration and initialises the logger.
func initConfig() error {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		// Non-fatal: use defaults
		cfg = &config.Config{}
	}

	// Configure logger
	level := cfg.Logging.Level
	if verbose {
		level = "debug"
	}

	logger = buildLogger(level, cfg.Logging.Format, cfg.Logging.File)
	return nil
}

// buildLogger creates a zerolog.Logger from the given settings.
func buildLogger(level, format, file string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	var w io.Writer = os.Stderr

	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			w = f
		}
	}

	if format == "text" {
		w = zerolog.ConsoleWriter{Out: w}
	}

	return zerolog.New(w).Level(lvl).With().Timestamp().Logger()
}

// socketPath returns the path to the control socket.
func socketPath() string {
	return platform.SocketPath()
}
