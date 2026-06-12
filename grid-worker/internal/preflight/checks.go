package preflight

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/internal/controlplane"
	"github.com/grid-computing/grid-worker/internal/scanner"
)

// CheckResult holds the outcome of a single pre-flight check.
type CheckResult struct {
	ID      string // PF-01 .. PF-10
	Name    string
	Status  string // pass | warn | fail
	Message string
}

// Runner runs all pre-flight checks required before the daemon enters IDLE state.
type Runner struct {
	cfg    *config.Config
	client *controlplane.Client
	log    zerolog.Logger
}

// New creates a Runner with the given config and control plane client.
func New(cfg *config.Config, client *controlplane.Client, log zerolog.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		client: client,
		log:    log.With().Str("component", "preflight").Logger(),
	}
}

// RunAll executes all pre-flight checks and returns their results.
// Returns an error only on a programming-level failure (not on check failures).
// Checks with status "fail" must be addressed before the daemon can start.
func (r *Runner) RunAll(ctx context.Context) ([]CheckResult, error) {
	checks := []func(context.Context) CheckResult{
		r.checkAPIKey,
		r.checkTLS,
		r.checkOrgUnit,
		r.checkWorkspaceRW,
		r.checkDiskQuota,
		r.checkAgentBinaries,
		r.checkGitVersion,
		r.checkScannerRuleset,
		r.checkSystemTime,
		r.checkHardware,
	}

	results := make([]CheckResult, 0, len(checks))
	for _, fn := range checks {
		result := fn(ctx)
		r.log.Info().
			Str("check", result.ID).
			Str("name", result.Name).
			Str("status", result.Status).
			Str("message", result.Message).
			Msg("preflight check completed")
		results = append(results, result)
	}

	return results, nil
}

// checkAPIKey validates that an API key is configured and accepted by the server.
func (r *Runner) checkAPIKey(ctx context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-01",
		Name: "API Key Validation",
	}

	if r.cfg.Server.APIKey == "" {
		result.Status = "fail"
		result.Message = "server.api_key is not configured"
		return result
	}

	if err := r.client.ValidateAPIKey(ctx); err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("API key rejected by server: %v", err)
		return result
	}

	result.Status = "pass"
	result.Message = "API key present and validated by server"
	return result
}

// checkTLS verifies that a TLS handshake can be completed to the control plane URL.
func (r *Runner) checkTLS(ctx context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-02",
		Name: "TLS Handshake",
	}

	serverURL := r.cfg.Server.URL
	if serverURL == "" {
		result.Status = "fail"
		result.Message = "server.url is not configured"
		return result
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: r.cfg.Server.TLSSkipVerify, //nolint:gosec
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, serverURL, nil)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("failed to build request: %v", err)
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("TLS connection failed: %v", err)
		return result
	}
	resp.Body.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("TLS handshake successful (HTTP %d)", resp.StatusCode)
	return result
}

// checkOrgUnit verifies that the org unit is resolvable (implicit in API key validation).
func (r *Runner) checkOrgUnit(ctx context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-03",
		Name: "Org Unit Resolution",
	}

	// Org unit resolution is implicit in a successful API key validation.
	// A 200 response from ValidateAPIKey confirms the org unit is known.
	if err := r.client.ValidateAPIKey(ctx); err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("org unit not resolvable (API key validation failed): %v", err)
		return result
	}

	result.Status = "pass"
	result.Message = "org unit resolved via API key validation"
	return result
}

// checkWorkspaceRW verifies that the workspace directory is readable and writable.
func (r *Runner) checkWorkspaceRW(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-04",
		Name: "Workspace Read/Write",
	}

	wsPath := r.cfg.Workspace.BasePath
	if wsPath == "" {
		result.Status = "fail"
		result.Message = "workspace.base_path is not configured"
		return result
	}

	if err := os.MkdirAll(wsPath, 0700); err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("cannot create workspace directory: %v", err)
		return result
	}

	testFile := filepath.Join(wsPath, ".preflight-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("workspace not writable: %v", err)
		return result
	}

	data, err := os.ReadFile(testFile)
	if err != nil || string(data) != "test" {
		result.Status = "fail"
		result.Message = "workspace not readable"
		_ = os.Remove(testFile)
		return result
	}

	_ = os.Remove(testFile)

	result.Status = "pass"
	result.Message = fmt.Sprintf("workspace %q is readable and writable", wsPath)
	return result
}

// checkDiskQuota warns if available disk is below the configured quota.
func (r *Runner) checkDiskQuota(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-05",
		Name: "Disk Space",
	}

	wsPath := r.cfg.Workspace.BasePath
	if wsPath == "" {
		result.Status = "warn"
		result.Message = "workspace.base_path is not configured, cannot check disk"
		return result
	}

	usage, err := disk.Usage(wsPath)
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot determine disk usage: %v", err)
		return result
	}

	freeGB := float64(usage.Free) / (1 << 30)
	requiredGB := r.cfg.Workspace.DiskQuotaGB

	if freeGB < requiredGB {
		result.Status = "warn"
		result.Message = fmt.Sprintf("disk free %.2f GB is below quota %.2f GB", freeGB, requiredGB)
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("disk free %.2f GB (quota %.2f GB)", freeGB, requiredGB)
	return result
}

// checkAgentBinaries verifies that at least one permitted agent binary is on PATH.
func (r *Runner) checkAgentBinaries(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-06",
		Name: "Agent Binaries",
	}

	agentBinaries := map[string][]string{
		"claude":   {"claude"},
		"copilot":  {"gh"},
		"gemini":   {"gemini"},
		"chatgpt":  {"sgpt"},
		"custom":   {},
	}

	permitted := r.cfg.Execution.Agents
	if len(permitted) == 0 {
		result.Status = "warn"
		result.Message = "no agents configured in execution.agents"
		return result
	}

	var found []string
	var missing []string

	for _, agent := range permitted {
		binaries, known := agentBinaries[agent]
		if !known {
			continue
		}
		for _, bin := range binaries {
			path, err := exec.LookPath(bin)
			if err == nil {
				found = append(found, fmt.Sprintf("%s (%s)", agent, path))
			} else {
				missing = append(missing, fmt.Sprintf("%s (binary: %s)", agent, bin))
			}
		}
	}

	if len(found) == 0 && len(missing) > 0 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("no agent binaries found on PATH: %s", strings.Join(missing, ", "))
		return result
	}

	var msgs []string
	if len(found) > 0 {
		msgs = append(msgs, "found: "+strings.Join(found, ", "))
	}
	if len(missing) > 0 {
		result.Status = "warn"
		msgs = append(msgs, "missing: "+strings.Join(missing, ", "))
		result.Message = strings.Join(msgs, "; ")
		return result
	}

	result.Status = "pass"
	result.Message = strings.Join(msgs, "; ")
	return result
}

// checkGitVersion verifies that git >= 2.30 is available on PATH.
func (r *Runner) checkGitVersion(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-07",
		Name: "Git Version",
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		result.Status = "fail"
		result.Message = "git not found on PATH"
		return result
	}

	cmd := exec.Command(gitPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("failed to run git --version: %v", err)
		return result
	}

	// Output is like: "git version 2.43.0"
	versionStr := strings.TrimSpace(string(out))
	parts := strings.Fields(versionStr)
	if len(parts) < 3 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("unexpected git --version output: %q", versionStr)
		return result
	}

	verParts := strings.Split(parts[2], ".")
	if len(verParts) < 2 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot parse git version: %q", parts[2])
		return result
	}

	major, err1 := strconv.Atoi(verParts[0])
	minor, err2 := strconv.Atoi(verParts[1])
	if err1 != nil || err2 != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot parse git version numbers: %q", parts[2])
		return result
	}

	if major < 2 || (major == 2 && minor < 30) {
		result.Status = "fail"
		result.Message = fmt.Sprintf("git %s is below required version 2.30", parts[2])
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("git %s found at %s", parts[2], gitPath)
	return result
}

// checkScannerRuleset verifies that the secret scanner ruleset is present and parseable.
func (r *Runner) checkScannerRuleset(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-08",
		Name: "Scanner Ruleset",
	}

	if !r.cfg.Security.ScanEnabled {
		result.Status = "pass"
		result.Message = "secret scanning is disabled"
		return result
	}

	var ruleset *scanner.Ruleset
	var err error

	if r.cfg.Security.RulesetPath != "" {
		ruleset, err = scanner.LoadRuleset(r.cfg.Security.RulesetPath)
		if err != nil {
			result.Status = "fail"
			result.Message = fmt.Sprintf("failed to load ruleset from %q: %v", r.cfg.Security.RulesetPath, err)
			return result
		}
		result.Message = fmt.Sprintf("custom ruleset loaded from %q (%d rules)", r.cfg.Security.RulesetPath, len(ruleset.Rules))
	} else {
		ruleset = scanner.DefaultRuleset()
		result.Message = fmt.Sprintf("default ruleset loaded (%d rules)", len(ruleset.Rules))
	}

	if len(ruleset.Rules) == 0 {
		result.Status = "warn"
		result.Message = "ruleset loaded but contains no rules"
		return result
	}

	result.Status = "pass"
	return result
}

// checkSystemTime verifies that the local system time is within 60 seconds of the server.
func (r *Runner) checkSystemTime(ctx context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-09",
		Name: "System Time Sync",
	}

	serverTime, err := r.client.ServerTime(ctx)
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot retrieve server time: %v", err)
		return result
	}

	localTime := time.Now().UTC()
	drift := math.Abs(localTime.Sub(serverTime).Seconds())

	if drift > 60 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("clock drift %.1fs exceeds 60s limit (local: %s, server: %s)",
			drift, localTime.Format(time.RFC3339), serverTime.Format(time.RFC3339))
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("clock drift %.1fs within limit", drift)
	return result
}

// checkHardware warns if CPU < 2 cores or RAM < 4 GB, and reduces MaxJobs if needed.
func (r *Runner) checkHardware(_ context.Context) CheckResult {
	result := CheckResult{
		ID:   "PF-10",
		Name: "Hardware Requirements",
	}

	cpuCount, err := cpu.Counts(true)
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot determine CPU count: %v", err)
		return result
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("cannot determine RAM: %v", err)
		return result
	}

	ramGB := float64(memInfo.Total) / (1 << 30)

	var warnings []string

	if cpuCount < 2 {
		warnings = append(warnings, fmt.Sprintf("CPU cores: %d (minimum 2 recommended)", cpuCount))
		if r.cfg.Workspace.MaxJobs > 1 {
			r.cfg.Workspace.MaxJobs = 1
			warnings = append(warnings, "MaxJobs reduced to 1")
		}
	}

	if ramGB < 4.0 {
		warnings = append(warnings, fmt.Sprintf("RAM: %.1f GB (minimum 4 GB recommended)", ramGB))
		if r.cfg.Workspace.MaxJobs > 1 {
			r.cfg.Workspace.MaxJobs = 1
			warnings = append(warnings, "MaxJobs reduced to 1")
		}
	}

	if runtime.GOOS == "windows" && cpuCount == 0 {
		// Windows CPU detection may differ
		cpuCount = runtime.NumCPU()
	}

	if len(warnings) > 0 {
		result.Status = "warn"
		result.Message = strings.Join(warnings, "; ")
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("CPU: %d cores, RAM: %.1f GB", cpuCount, ramGB)
	return result
}

// HasFailures returns true if any check in results has status "fail".
func HasFailures(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == "fail" {
			return true
		}
	}
	return false
}

// sentinel to suppress unused import error
var _ = errors.New
