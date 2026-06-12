package controlplane

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/config"
)

// Client is a typed HTTP client for communicating with the grid control plane.
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
	workerID   string
	log        zerolog.Logger
}

// New creates a new Client configured from the given ServerConfig.
func New(cfg config.ServerConfig, log zerolog.Logger) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec // user-controlled opt-in
		},
	}

	return &Client{
		baseURL:  cfg.URL,
		apiKey:   cfg.APIKey,
		workerID: cfg.WorkerID,
		log:      log.With().Str("component", "controlplane-client").Logger(),
		httpClient: &http.Client{
			Timeout:   time.Duration(cfg.PollTimeoutSec) * time.Second,
			Transport: transport,
		},
	}
}

// SetWorkerID updates the worker ID (called after successful registration).
func (c *Client) SetWorkerID(id string) {
	c.workerID = id
}

// RegisterRequest is the payload sent during worker registration.
type RegisterRequest struct {
	HostnameHash  string  `json:"hostname_hash"`
	Agents        []string `json:"agents"`
	OSInfo        string  `json:"os_info"`
	CapacityScore float64 `json:"capacity_score"`
}

// RegisterResponse is the response from the registration endpoint.
type RegisterResponse struct {
	WorkerID string `json:"worker_id"`
}

// HeartbeatRequest is the payload sent on each heartbeat tick.
type HeartbeatRequest struct {
	State           string  `json:"state"`
	CPUPercent      float64 `json:"cpu_percent"`
	RAMMBUsed       int     `json:"ram_mb_used"`
	DiskFreeGB      float64 `json:"disk_free_gb"`
	BatteryPercent  float64 `json:"battery_percent"`
	JobsToday       int     `json:"jobs_today"`
}

// HeartbeatResponse is the response from the heartbeat endpoint.
type HeartbeatResponse struct {
	AssignedTask *TaskAssignment `json:"assigned_task,omitempty"`
}

// TaskAssignment represents a task assigned by the control plane.
type TaskAssignment struct {
	TaskID      string            `json:"task_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	TaskType    string            `json:"task_type"`
	AIAgent     string            `json:"ai_agent"`
	Options     map[string]string `json:"options,omitempty"`
	RepoURL     string            `json:"repo_url,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	TimeoutSec  int               `json:"timeout_sec"`
}

// TaskStatusRequest is the payload for task status updates.
type TaskStatusRequest struct {
	State        string `json:"state"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// OutputSubmission is the payload for submitting task output to the control plane.
type OutputSubmission struct {
	TaskID       string         `json:"task_id"`
	HMACSha256   string         `json:"hmac_sha256"`
	Artifacts    []ArtifactMeta `json:"artifacts"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	XHMACHeader  string         `json:"-"` // set as HTTP header
}

// ArtifactMeta contains metadata about a submitted artifact.
type ArtifactMeta struct {
	RelPath string `json:"rel_path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// Register registers this worker with the control plane.
func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.post(ctx, "/api/v1/workers/register", req, &resp); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	return &resp, nil
}

// Heartbeat sends a heartbeat and receives any pending task assignment.
func (c *Client) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	path := fmt.Sprintf("/api/v1/workers/%s/heartbeat", c.workerID)
	var resp HeartbeatResponse
	if err := c.post(ctx, path, req, &resp); err != nil {
		return nil, fmt.Errorf("heartbeat: %w", err)
	}
	return &resp, nil
}

// UpdateTaskStatus updates the status of a task on the control plane.
func (c *Client) UpdateTaskStatus(ctx context.Context, taskID string, req *TaskStatusRequest) error {
	path := fmt.Sprintf("/api/v1/tasks/%s/status", taskID)
	if err := c.patch(ctx, path, req, nil); err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

// SubmitOutput submits task output artifacts to the control plane.
func (c *Client) SubmitOutput(ctx context.Context, output *OutputSubmission) error {
	path := "/api/v1/outputs"

	body, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("X-Worker-ID", c.workerID)
	if output.XHMACHeader != "" {
		httpReq.Header.Set("X-HMAC-SHA256", output.XHMACHeader)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("submit output request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("submit output: status %d: %s", resp.StatusCode, string(respBody))
}

// ValidateAPIKey validates the configured API key against the control plane
// by calling a lightweight authenticated endpoint.
func (c *Client) ValidateAPIKey(ctx context.Context) error {
	// Use the workers list endpoint with auth — a 200 or 404 confirms the key is valid;
	// a 401 means the key is rejected.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/workers", nil)
	if err != nil {
		return fmt.Errorf("create validate request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("validate API key request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("API key is invalid or expired")
	}
	if resp.StatusCode >= 500 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("server error during validation: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ServerTime returns the server's current time by inspecting the Date header
// of the /healthz liveness probe (no auth required, minimal overhead).
func (c *Client) ServerTime(ctx context.Context) (time.Time, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodHead, c.baseURL+"/healthz", nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("create time request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return time.Time{}, fmt.Errorf("time request: %w", err)
	}
	defer resp.Body.Close()

	dateHeader := resp.Header.Get("Date")
	if dateHeader == "" {
		// Fall back to local time if the server doesn't set Date header
		// (most Go HTTP servers don't set it automatically).
		// Return the server's time as "now" since we know reachability.
		return time.Now().UTC(), nil
	}

	t, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse server Date header: %w", err)
	}

	return t, nil
}

// patch marshals req as JSON, PATCHes to path, and optionally unmarshals the response into out.
func (c *Client) patch(ctx context.Context, path string, req any, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, req, out)
}

// post marshals req as JSON, POSTs to path, and optionally unmarshals the response into out.
func (c *Client) post(ctx context.Context, path string, req any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, req, out)
}

// doJSON is the shared implementation for post/patch.
func (c *Client) doJSON(ctx context.Context, method, path string, req any, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.workerID != "" {
		httpReq.Header.Set("X-Worker-ID", c.workerID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return &AuthError{StatusCode: resp.StatusCode, Message: string(respBody)}
	case resp.StatusCode >= 500:
		return &ServerError{StatusCode: resp.StatusCode, Message: string(respBody)}
	case resp.StatusCode >= 400:
		return fmt.Errorf("client error %d: %s", resp.StatusCode, string(respBody))
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// AuthError represents an HTTP 401 response.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication error %d: %s", e.StatusCode, e.Message)
}

// ServerError represents an HTTP 5xx response.
type ServerError struct {
	StatusCode int
	Message    string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error %d: %s", e.StatusCode, e.Message)
}
