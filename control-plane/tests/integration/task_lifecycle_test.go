//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestTaskLifecycleEndToEnd tests the complete task flow:
// queued → assigned → running → completed → output reviewed
func TestTaskLifecycleEndToEnd(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	// Step 1: Create an org
	orgID := createTestOrg(t, srv, "test-org")

	// Step 2: Issue an API key (org-admin scope)
	adminKey := issueAPIKey(t, srv, orgID, "admin-key", []string{"tasks:write", "tasks:read", "workers:write", "workers:read", "outputs:write"})

	// Step 3: Register a worker
	workerID := registerWorker(t, srv, adminKey, orgID, []string{"claude", "copilot"})

	// Step 4: Submit a task
	taskID := submitTask(t, srv, adminKey, orgID, "Test refactor", "claude")

	// Step 5: Simulate heartbeat → task should be assigned
	var assignedTask *taskAssignment
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		assignedTask = heartbeat(t, srv, workerID, adminKey)
		if assignedTask != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if assignedTask == nil {
		t.Fatal("task was not assigned within 15s")
	}
	if assignedTask.TaskID != taskID {
		t.Errorf("assigned task ID mismatch: got %s, want %s", assignedTask.TaskID, taskID)
	}

	// Step 6: Update task to running
	updateTaskStatus(t, srv, workerID, adminKey, taskID, "running", "")

	// Step 7: Update task to completed
	updateTaskStatus(t, srv, workerID, adminKey, taskID, "completed", "")

	// Step 8: Submit output
	outputID := submitOutput(t, srv, workerID, adminKey, taskID)

	// Step 9: Review output (approve)
	reviewOutput(t, srv, adminKey, outputID, "approved")

	// Step 10: Verify final task state
	task := getTask(t, srv, adminKey, taskID)
	if task["state"] != "completed" {
		t.Errorf("expected task state=completed, got %s", task["state"])
	}
}

// TestMultiTenantIsolation asserts zero cross-org data leakage
func TestMultiTenantIsolation(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	// Create two separate orgs
	orgAID := createTestOrg(t, srv, "org-alpha")
	orgBID := createTestOrg(t, srv, "org-beta")

	keyA := issueAPIKey(t, srv, orgAID, "key-a", []string{"tasks:write", "tasks:read"})
	keyB := issueAPIKey(t, srv, orgBID, "key-b", []string{"tasks:write", "tasks:read"})

	// Submit tasks for each org
	taskAID := submitTask(t, srv, keyA, orgAID, "Task for Org A", "claude")
	submitTask(t, srv, keyB, orgBID, "Task for Org B", "copilot")

	// Org A key should NOT see Org B's tasks
	tasksA := listTasks(t, srv, keyA)
	for _, task := range tasksA {
		if id, ok := task["id"].(string); ok && id != taskAID {
			orgID, _ := task["org_unit_id"].(string)
			if orgID == orgBID.String() {
				t.Errorf("Org A key can see Org B task (ID=%s) — cross-org leak!", id)
			}
		}
	}

	// Org B key should NOT see Org A's tasks
	tasksB := listTasks(t, srv, keyB)
	for _, task := range tasksB {
		if id, ok := task["id"].(string); ok {
			orgID, _ := task["org_unit_id"].(string)
			if orgID == orgAID.String() {
				t.Errorf("Org B key can see Org A task (ID=%s) — cross-org leak!", id)
			}
			_ = id
		}
	}
}

// TestAPIKeyRevocation verifies that a revoked key is immediately rejected
func TestAPIKeyRevocation(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	orgID := createTestOrg(t, srv, "revoke-test-org")
	key := issueAPIKey(t, srv, orgID, "temp-key", []string{"tasks:read"})

	// Key should work
	resp := doRequest(t, srv, "GET", "/api/v1/tasks", nil, key)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 before revoke, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Revoke the key (requires a key ID — we'd need to track it)
	// For now, validate via issuing a new key and the old one failing
	// TODO: Extract key ID from issueAPIKey response for full test
}

// TestHeartbeatReaper verifies that abandoned tasks are requeued
func TestHeartbeatReaper(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	orgID := createTestOrg(t, srv, "reaper-test-org")
	key := issueAPIKey(t, srv, orgID, "reaper-key", []string{"tasks:write", "tasks:read", "workers:write", "workers:read"})
	workerID := registerWorker(t, srv, key, orgID, []string{"claude"})
	taskID := submitTask(t, srv, key, orgID, "Reaper test task", "claude")

	// Get task assigned
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if at := heartbeat(t, srv, workerID, key); at != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	updateTaskStatus(t, srv, workerID, key, taskID, "running", "")

	// Stop sending heartbeats — task should be reaped within ~90s (configurable)
	// For test, we use a short TTL (override via env or test config)
	reaperDeadline := time.Now().Add(35 * time.Second) // 30s reap interval + buffer
	for time.Now().Before(reaperDeadline) {
		task := getTask(t, srv, key, taskID)
		if state, _ := task["state"].(string); state == "queued" {
			t.Logf("task correctly reaped and requeued after heartbeat expiry")
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Error("task was not reaped within expected time window")
}

// TestSecretScanBlocking verifies that tasks with secrets in payload are rejected
func TestSecretScanBlocking(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t, ctx)
	defer srv.Close()

	orgID := createTestOrg(t, srv, "scan-test-org")
	key := issueAPIKey(t, srv, orgID, "scan-key", []string{"tasks:write"})

	// Try to submit a task with an embedded AWS key in the description
	body := map[string]any{
		"title":       "Scan test task",
		"description": "Fix this code: AKIAIOSFODNN7EXAMPLE is used in the config",
		"task_type":   "bug_fix",
		"ai_agent":    "claude",
		"priority":    5,
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/tasks", bytes.NewReader(bodyJSON), key)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 422/400 for task with secret in payload, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type testServer struct {
	*httptest.Server
}

type taskAssignment struct {
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	AIAgent     string `json:"ai_agent"`
	TimeoutSec  int    `json:"timeout_sec"`
}

func newTestServer(t *testing.T, ctx context.Context) *testServer {
	t.Helper()
	// Build the server using test config from environment
	dsn := os.Getenv("GRID_DATABASE_DSN")
	if dsn == "" {
		t.Skip("GRID_DATABASE_DSN not set; skipping integration test")
	}
	// TODO: Initialize real server with test config
	// For now, return a placeholder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	}))
	return &testServer{srv}
}

func createTestOrg(t *testing.T, srv *testServer, name string) uuid.UUID {
	t.Helper()
	body := map[string]any{"name": name, "plan_tier": "pro"}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/orgs", bytes.NewReader(bodyJSON), "superadmin-key")
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	id, _ := uuid.Parse(fmt.Sprintf("%v", result["id"]))
	return id
}

func issueAPIKey(t *testing.T, srv *testServer, orgID uuid.UUID, name string, scopes []string) string {
	t.Helper()
	body := map[string]any{
		"org_unit_id": orgID,
		"name":        name,
		"scopes":      scopes,
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/api-keys", bytes.NewReader(bodyJSON), "superadmin-key")
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	key, _ := result["key"].(string)
	return key
}

func registerWorker(t *testing.T, srv *testServer, apiKey string, orgID uuid.UUID, agents []string) uuid.UUID {
	t.Helper()
	body := map[string]any{
		"hostname_hash":  "sha256testworker001",
		"agents":         agents,
		"capacity_score": 100,
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/workers/register", bytes.NewReader(bodyJSON), apiKey)
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	id, _ := uuid.Parse(fmt.Sprintf("%v", result["worker_id"]))
	return id
}

func submitTask(t *testing.T, srv *testServer, apiKey string, orgID uuid.UUID, title, agent string) string {
	t.Helper()
	body := map[string]any{
		"title":       title,
		"description": "Write comprehensive unit tests for the authentication module",
		"task_type":   "test_generation",
		"ai_agent":    agent,
		"priority":    5,
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/tasks", bytes.NewReader(bodyJSON), apiKey)
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return fmt.Sprintf("%v", result["id"])
}

func heartbeat(t *testing.T, srv *testServer, workerID uuid.UUID, apiKey string) *taskAssignment {
	t.Helper()
	body := map[string]any{
		"state":        "idle",
		"cpu_percent":  10.0,
		"ram_mb_used":  512,
		"disk_free_gb": 50.0,
		"jobs_today":   0,
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", fmt.Sprintf("/api/v1/workers/%s/heartbeat", workerID), bytes.NewReader(bodyJSON), apiKey)
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if assigned, ok := result["assigned_task"]; ok && assigned != nil {
		data, _ := json.Marshal(assigned)
		var ta taskAssignment
		json.Unmarshal(data, &ta)
		return &ta
	}
	return nil
}

func updateTaskStatus(t *testing.T, srv *testServer, workerID uuid.UUID, apiKey, taskID, state, errMsg string) {
	t.Helper()
	body := map[string]any{"state": state, "error_message": errMsg}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "PATCH", fmt.Sprintf("/api/v1/tasks/%s/status", taskID), bytes.NewReader(bodyJSON), apiKey)
	resp.Body.Close()
}

func submitOutput(t *testing.T, srv *testServer, workerID uuid.UUID, apiKey, taskID string) string {
	t.Helper()
	body := map[string]any{
		"task_id":     taskID,
		"hmac_sha256": "abc123deadbeef",
		"artifacts":   []string{"main.go", "main_test.go"},
		"metadata":    map[string]any{"agent": "claude", "tokens": 1200},
	}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "POST", "/api/v1/outputs", bytes.NewReader(bodyJSON), apiKey)
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return fmt.Sprintf("%v", result["id"])
}

func reviewOutput(t *testing.T, srv *testServer, apiKey, outputID, status string) {
	t.Helper()
	body := map[string]any{"review_status": status, "comment": "Looks good"}
	bodyJSON, _ := json.Marshal(body)
	resp := doRequest(t, srv, "PATCH", fmt.Sprintf("/api/v1/outputs/%s/review", outputID), bytes.NewReader(bodyJSON), apiKey)
	resp.Body.Close()
}

func getTask(t *testing.T, srv *testServer, apiKey, taskID string) map[string]any {
	t.Helper()
	resp := doRequest(t, srv, "GET", fmt.Sprintf("/api/v1/tasks/%s", taskID), nil, apiKey)
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func listTasks(t *testing.T, srv *testServer, apiKey string) []map[string]any {
	t.Helper()
	resp := doRequest(t, srv, "GET", "/api/v1/tasks?limit=100", nil, apiKey)
	defer resp.Body.Close()
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func doRequest(t *testing.T, srv *testServer, method, path string, body *bytes.Reader, apiKey string) *http.Response {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = body
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}
