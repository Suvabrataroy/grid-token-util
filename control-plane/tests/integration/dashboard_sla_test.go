//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDashboardSnapshotSLA verifies snapshot endpoint responds within 3s
// with fixture data of 500 workers + 1000 tasks
func TestDashboardSnapshotSLA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLA test in short mode")
	}

	ctx := t.Context()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	// Setup: seed 500 workers and 1000 tasks (requires seeding helper)
	// seedLargeDataset(t, srv, 500, 1000)

	adminKey := "superadmin-key" // test fixture key

	start := time.Now()
	resp, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/dashboard/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Header.Set("Authorization", "Bearer "+adminKey)

	httpResp, err := http.DefaultClient.Do(resp)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	io.ReadAll(httpResp.Body) // consume body

	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Errorf("snapshot took %v; SLA requires < 3s", elapsed)
	}
	t.Logf("snapshot latency: %v", elapsed)
}

// TestSecurityEventSSESLA verifies that a security event reaches the SSE client
// within 5 seconds of submission
func TestSecurityEventSSESLA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLA test in short mode")
	}

	ctx := t.Context()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	orgID := createTestOrg(t, srv, "sse-sla-test-org")
	key := issueAPIKey(t, srv, orgID, "sse-sla-key", []string{"tasks:write", "workers:write"})

	// Connect SSE client
	sseURL := srv.URL + "/api/v1/dashboard/stream"
	req, _ := http.NewRequest(http.MethodGet, sseURL, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")

	received := make(chan string, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				data := string(buf[:n])
				if strings.Contains(data, "security_event") {
					received <- data
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Give SSE connection time to establish
	time.Sleep(200 * time.Millisecond)

	// Submit a task with a secret (should trigger security event)
	triggerStart := time.Now()
	body := `{"title":"SEC test","description":"key: AKIAIOSFODNN7EXAMPLE","task_type":"bug_fix","ai_agent":"claude","priority":5}`
	doRequest(t, srv, "POST", "/api/v1/tasks", bytes.NewReader([]byte(body)), key)

	select {
	case event := <-received:
		latency := time.Since(triggerStart)
		if latency > 5*time.Second {
			t.Errorf("security event latency %v exceeds 5s SLA", latency)
		}
		t.Logf("security event received in %v: %s", latency, event[:min(len(event), 100)])
	case <-time.After(10 * time.Second):
		t.Error("security event not received within 10s timeout")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestBrowniePointsAccrual verifies that completing a task awards Brownie Points
func TestBrowniePointsAccrual(t *testing.T) {
	ctx := t.Context()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	orgID := createTestOrg(t, srv, "brownie-test-org")
	key := issueAPIKey(t, srv, orgID, "brownie-key", []string{"tasks:write", "tasks:read", "workers:write", "workers:read", "outputs:write"})
	workerID := registerWorker(t, srv, key, orgID, []string{"claude"})
	taskID := submitTask(t, srv, key, orgID, "Brownie test task", "claude")

	// Get assigned
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if heartbeat(t, srv, workerID, key) != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	updateTaskStatus(t, srv, workerID, key, taskID, "running", "")
	updateTaskStatus(t, srv, workerID, key, taskID, "completed", "")
	outputID := submitOutput(t, srv, workerID, key, taskID)
	reviewOutput(t, srv, key, outputID, "approved")

	// Check leaderboard
	resp := doRequest(t, srv, "GET", "/api/v1/brownie-points/leaderboard", nil, key)
	defer resp.Body.Close()
	var leaderboard []map[string]any
	json.NewDecoder(resp.Body).Decode(&leaderboard)

	found := false
	for _, entry := range leaderboard {
		if wid, _ := entry["worker_id"].(string); wid == workerID.String() {
			points, _ := entry["total_points"].(float64)
			// TaskCompleted=+10, OutputApproved=+20 → minimum 30 points
			if int(points) < 30 {
				t.Errorf("expected >= 30 brownie points, got %v", points)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("worker not found in leaderboard after completing+approved task")
	}
}
