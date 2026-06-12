package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/grid-computing/grid-worker/pkg/platform"
)

// Load reads the configuration from cfgFile (or the default location if empty)
// and returns a populated Config struct. Environment variables with the GW_ prefix
// are also applied and override file-based config.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Environment variable settings
	v.SetEnvPrefix("GW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(platform.ConfigDir())
		v.AddConfigPath(".")
	}

	// Apply defaults
	setDefaults(v)

	// Read config file (non-fatal if not found)
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return cfg, nil
}

// setDefaults configures Viper with default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.heartbeat_sec", 30)
	v.SetDefault("server.poll_timeout_sec", 30)
	v.SetDefault("server.tls_skip_verify", false)
	v.SetDefault("server.retry_max", 3)

	// Workspace defaults
	v.SetDefault("workspace.base_path", platform.WorkspaceDir())
	v.SetDefault("workspace.disk_quota_gb", 10.0)
	v.SetDefault("workspace.max_jobs", 1)
	v.SetDefault("workspace.cleanup_after", true)

	// Security defaults
	v.SetDefault("security.scan_enabled", true)
	v.SetDefault("security.scan_concurrency", 4)

	// Execution defaults
	v.SetDefault("execution.timeout_sec", 3600)
	v.SetDefault("execution.max_cpu_percent", 80.0)
	v.SetDefault("execution.max_ram_mb", 2048)

	// Policy defaults
	v.SetDefault("policy.mode", "auto")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
}

// Validate checks that the required configuration fields are present and valid.
func Validate(cfg *Config) error {
	var errs []string

	if cfg.Server.URL == "" {
		errs = append(errs, "server.url is required")
	}
	if cfg.Server.APIKey == "" {
		errs = append(errs, "server.api_key is required")
	}
	if cfg.Security.HMACSecret == "" {
		errs = append(errs, "security.hmac_secret is required")
	}
	if len(cfg.Security.HMACSecret) < 32 {
		errs = append(errs, "security.hmac_secret must be at least 32 characters")
	}

	mode := cfg.Policy.Mode
	if mode != "auto" && mode != "manual" && mode != "paused" {
		errs = append(errs, fmt.Sprintf("policy.mode must be one of auto|manual|paused, got %q", mode))
	}

	logFormat := cfg.Logging.Format
	if logFormat != "json" && logFormat != "text" {
		errs = append(errs, fmt.Sprintf("logging.format must be json or text, got %q", logFormat))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// Watch registers a callback invoked whenever the configuration file changes.
// The callback receives a freshly loaded Config. Errors during reload are logged
// but do not stop the watcher.
func Watch(cfg *Config, onChange func(*Config)) error {
	v := viper.New()
	v.SetEnvPrefix("GW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(platform.ConfigDir())
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("reading config for watcher: %w", err)
		}
	}

	v.OnConfigChange(func(e fsnotify.Event) {
		newCfg := &Config{}
		if err := v.Unmarshal(newCfg); err != nil {
			return
		}
		onChange(newCfg)
	})
	v.WatchConfig()

	return nil
}
