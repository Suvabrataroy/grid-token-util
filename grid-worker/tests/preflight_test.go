package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestPreflightPF04WorkspaceRW verifies PF-04: workspace path r/w check.
func TestPreflightPF04WorkspaceRW(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "grid-pf-test")
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("PF-04: failed to create test file in workspace: %v", err)
	}
	f.Close()
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("PF-04: failed to remove test file: %v", err)
	}
	t.Log("PF-04 workspace r/w: PASS")
}

// TestPreflightPF07GitVersion verifies PF-07: git >= 2.30.
func TestPreflightPF07GitVersion(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found on PATH; skipping PF-07 test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git --version failed: %v", err)
	}

	// Parse "git version 2.43.0"
	parts := strings.Fields(string(out))
	if len(parts) < 3 {
		t.Fatalf("unexpected git --version output: %q", string(out))
	}
	versionStr := parts[2]
	major, minor, ok := parseVersion(versionStr)
	if !ok {
		t.Fatalf("failed to parse git version from %q", versionStr)
	}

	if major < 2 || (major == 2 && minor < 30) {
		t.Errorf("PF-07: git version %s is below minimum 2.30", versionStr)
	} else {
		t.Logf("PF-07: git version %s >= 2.30 ✓", versionStr)
	}
}

// TestPreflightPF09TimeSync verifies PF-09: system time is within a reasonable range.
func TestPreflightPF09TimeSync(t *testing.T) {
	now := time.Now()
	minTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	maxTime := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)

	if now.Before(minTime) || now.After(maxTime) {
		t.Errorf("PF-09: system time %v is outside reasonable range [%v, %v]", now, minTime, maxTime)
	} else {
		t.Logf("PF-09: system time %v within range ✓", now.Format(time.RFC3339))
	}
}

// TestPreflightPF10Resources verifies PF-10: CPU and runtime.NumCPU() is detectable.
func TestPreflightPF10Resources(t *testing.T) {
	numCPU := runtime.NumCPU()
	t.Logf("PF-10: detected %d CPUs", numCPU)
	if numCPU < 1 {
		t.Error("PF-10: runtime.NumCPU() returned 0; something is wrong")
	}
	if numCPU < 2 {
		t.Logf("PF-10: WARNING: < 2 CPUs detected; PF-10 would warn and reduce MaxJobs (not fatal)")
	} else {
		t.Logf("PF-10: CPU count %d >= 2 ✓", numCPU)
	}
}

// TestPathTraversalPrevention verifies that path traversal in workspace is blocked.
func TestPathTraversalPrevention(t *testing.T) {
	workspaceRoot := t.TempDir()

	cases := []struct {
		path    string
		allowed bool
	}{
		{filepath.Join(workspaceRoot, "main.go"), true},
		{filepath.Join(workspaceRoot, "subdir", "file.txt"), true},
		{filepath.Join(workspaceRoot, "..", "etc", "passwd"), false},
		{filepath.Join(workspaceRoot, "..", "..", "secret"), false},
		{"/etc/passwd", false},
		{"../outside", false},
	}

	for _, tc := range cases {
		abs, err := filepath.Abs(tc.path)
		if err != nil {
			t.Errorf("Abs(%q) failed: %v", tc.path, err)
			continue
		}
		contained := strings.HasPrefix(abs, workspaceRoot+string(filepath.Separator)) ||
			abs == workspaceRoot
		if contained != tc.allowed {
			t.Errorf("path %q: expected contained=%v, got %v", tc.path, tc.allowed, contained)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseVersion(s string) (major, minor int, ok bool) {
	var maj, min, patch int
	if _, err := fmt.Sscanf(s, "%d.%d.%d", &maj, &min, &patch); err == nil {
		return maj, min, true
	}
	if _, err := fmt.Sscanf(s, "%d.%d", &maj, &min); err == nil {
		return maj, min, true
	}
	return 0, 0, false
}
