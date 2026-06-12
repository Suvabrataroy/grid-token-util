package config

// Config is the root configuration struct for the grid-worker daemon.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"    yaml:"server"`
	Workspace WorkspaceConfig `mapstructure:"workspace" yaml:"workspace"`
	Security  SecurityConfig  `mapstructure:"security"  yaml:"security"`
	Execution ExecutionConfig `mapstructure:"execution" yaml:"execution"`
	Policy    PolicyConfig    `mapstructure:"policy"    yaml:"policy"`
	Logging   LoggingConfig   `mapstructure:"logging"   yaml:"logging"`
}

// ServerConfig holds control plane connection settings.
type ServerConfig struct {
	URL             string `mapstructure:"url"              yaml:"url"`
	APIKey          string `mapstructure:"api_key"          yaml:"api_key"`
	WorkerID        string `mapstructure:"worker_id"        yaml:"worker_id"`
	HeartbeatSec    int    `mapstructure:"heartbeat_sec"    yaml:"heartbeat_sec"`
	PollTimeoutSec  int    `mapstructure:"poll_timeout_sec" yaml:"poll_timeout_sec"`
	TLSSkipVerify   bool   `mapstructure:"tls_skip_verify"  yaml:"tls_skip_verify"`
	RetryMax        int    `mapstructure:"retry_max"        yaml:"retry_max"`
}

// WorkspaceConfig holds workspace management settings.
type WorkspaceConfig struct {
	BasePath     string  `mapstructure:"base_path"     yaml:"base_path"`
	DiskQuotaGB  float64 `mapstructure:"disk_quota_gb" yaml:"disk_quota_gb"`
	MaxJobs      int     `mapstructure:"max_jobs"      yaml:"max_jobs"`
	CleanupAfter bool    `mapstructure:"cleanup_after" yaml:"cleanup_after"`
}

// SecurityConfig holds security and secret scanning settings.
type SecurityConfig struct {
	HMACSecret      string `mapstructure:"hmac_secret"      yaml:"hmac_secret"`
	ScanEnabled     bool   `mapstructure:"scan_enabled"     yaml:"scan_enabled"`
	RulesetPath     string `mapstructure:"ruleset_path"     yaml:"ruleset_path"`
	ScanConcurrency int    `mapstructure:"scan_concurrency" yaml:"scan_concurrency"`
}

// ExecutionConfig holds task execution limits and permissions.
type ExecutionConfig struct {
	TimeoutSec    int      `mapstructure:"timeout_sec"    yaml:"timeout_sec"`
	MaxCPUPercent float64  `mapstructure:"max_cpu_percent" yaml:"max_cpu_percent"`
	MaxRAMMB      int      `mapstructure:"max_ram_mb"     yaml:"max_ram_mb"`
	Agents        []string `mapstructure:"agents"         yaml:"agents"`
}

// PolicyConfig holds scheduling and approval policy settings.
type PolicyConfig struct {
	Mode    string             `mapstructure:"mode"    yaml:"mode"`
	Windows []TimeWindowConfig `mapstructure:"windows" yaml:"windows"`
}

// TimeWindowConfig defines an allowed time window for task execution.
type TimeWindowConfig struct {
	DayOfWeek []int  `mapstructure:"day_of_week" yaml:"day_of_week"`
	StartHour int    `mapstructure:"start_hour"  yaml:"start_hour"`
	EndHour   int    `mapstructure:"end_hour"    yaml:"end_hour"`
	Timezone  string `mapstructure:"timezone"    yaml:"timezone"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"  yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
	File   string `mapstructure:"file"   yaml:"file"`
}
