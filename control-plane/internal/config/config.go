// Package config handles loading and validation of control-plane configuration.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	TLSCertFile     string `mapstructure:"tls_cert_file"`
	TLSKeyFile      string `mapstructure:"tls_key_file"`
	ReadTimeoutSec  int    `mapstructure:"read_timeout_sec"`
	WriteTimeoutSec int    `mapstructure:"write_timeout_sec"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN            string `mapstructure:"dsn"`
	MaxOpenConns   int    `mapstructure:"max_open_conns"`
	MaxIdleConns   int    `mapstructure:"max_idle_conns"`
	ConnTimeoutSec int    `mapstructure:"conn_timeout_sec"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// SecurityConfig holds cryptographic parameters.
type SecurityConfig struct {
	ArgonMemory      uint32 `mapstructure:"argon_memory"`
	ArgonIterations  uint32 `mapstructure:"argon_iterations"`
	ArgonParallelism uint8  `mapstructure:"argon_parallelism"`
	ArgonSaltLen     int    `mapstructure:"argon_salt_len"`
	ArgonKeyLen      uint32 `mapstructure:"argon_key_len"`
	HMACSecret       string `mapstructure:"hmac_secret"`
}

// SchedulerConfig holds background job settings.
type SchedulerConfig struct {
	TickIntervalSec   int `mapstructure:"tick_interval_sec"`
	ReaperIntervalSec int `mapstructure:"reaper_interval_sec"`
	HeartbeatTTLSec   int `mapstructure:"heartbeat_ttl_sec"`
}

// DashboardConfig holds dashboard-specific settings.
type DashboardConfig struct {
	SnapshotCacheTTLSec int `mapstructure:"snapshot_cache_ttl_sec"`
	MaxSSEClients       int `mapstructure:"max_sse_clients"`
}

// LoggingConfig holds log output settings.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // "json" or "console"
}

// Config is the root configuration structure for the control-plane server.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Security  SecurityConfig  `mapstructure:"security"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Dashboard DashboardConfig `mapstructure:"dashboard"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

// Load reads configuration from the given file (if non-empty) and environment
// variables with the GRID_ prefix, then validates the result.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// ── defaults ──────────────────────────────────────────────────────────────
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout_sec", 30)
	v.SetDefault("server.write_timeout_sec", 60)

	v.SetDefault("database.max_open_conns", 20)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_timeout_sec", 10)

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("security.argon_memory", 65536)      // 64 MiB
	v.SetDefault("security.argon_iterations", 3)
	v.SetDefault("security.argon_parallelism", 2)
	v.SetDefault("security.argon_salt_len", 16)
	v.SetDefault("security.argon_key_len", 32)

	v.SetDefault("scheduler.tick_interval_sec", 5)
	v.SetDefault("scheduler.reaper_interval_sec", 30)
	v.SetDefault("scheduler.heartbeat_ttl_sec", 90)

	v.SetDefault("dashboard.snapshot_cache_ttl_sec", 10)
	v.SetDefault("dashboard.max_sse_clients", 500)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// ── file ──────────────────────────────────────────────────────────────────
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/grid-control-plane")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("config: read file: %w", err)
		}
		// No config file is acceptable; env vars or defaults will be used.
	}

	// ── environment variables ─────────────────────────────────────────────────
	v.SetEnvPrefix("GRID")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// ── unmarshal ─────────────────────────────────────────────────────────────
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks that required configuration fields are present and sensible.
func Validate(cfg *Config) error {
	var errs []string

	if cfg.Database.DSN == "" {
		errs = append(errs, "database.dsn is required")
	}
	if cfg.Security.HMACSecret == "" {
		errs = append(errs, "security.hmac_secret is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}
	if cfg.Security.ArgonMemory < 8192 {
		errs = append(errs, "security.argon_memory must be at least 8192 KiB")
	}
	if cfg.Security.ArgonIterations < 1 {
		errs = append(errs, "security.argon_iterations must be at least 1")
	}
	if cfg.Scheduler.HeartbeatTTLSec < 10 {
		errs = append(errs, "scheduler.heartbeat_ttl_sec must be at least 10 seconds")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
