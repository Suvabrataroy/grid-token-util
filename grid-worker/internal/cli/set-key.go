package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/grid-computing/grid-worker/pkg/platform"
)

// NewSetKeyCmd returns the `set-key` subcommand.
func NewSetKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-key <api-key>",
		Short: "Save an API key to the configuration file",
		Long:  "Writes the given API key into the grid-worker config file (creates it if it doesn't exist).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetKey(args[0])
		},
	}
}

func runSetKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("api-key cannot be empty")
	}

	// Determine config file path
	configDir := platform.ConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if cfgFile != "" {
		configPath = cfgFile
	}

	// Load existing config or start fresh
	existing := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// Set the API key nested under "server"
	serverSection, ok := existing["server"].(map[string]any)
	if !ok {
		serverSection = make(map[string]any)
	}
	serverSection["api_key"] = apiKey
	existing["server"] = serverSection

	// Write back
	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("API key saved to %s\n", configPath)
	return nil
}
