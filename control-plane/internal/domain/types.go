// Package domain defines the core data types for the grid computing control plane.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// WorkerState represents the operational state of a worker node.
type WorkerState string

const (
	WorkerStateIdle    WorkerState = "idle"
	WorkerStateBusy    WorkerState = "busy"
	WorkerStatePaused  WorkerState = "paused"
	WorkerStateOffline WorkerState = "offline"
)

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	TaskStateQueued    TaskState = "queued"
	TaskStateAssigned  TaskState = "assigned"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
)

// TaskType represents the category of work a task involves.
type TaskType string

const (
	TaskTypeCodeGeneration  TaskType = "code_generation"
	TaskTypeCodeReview      TaskType = "code_review"
	TaskTypeRefactor        TaskType = "refactor"
	TaskTypeTestGeneration  TaskType = "test_generation"
	TaskTypeDocumentation   TaskType = "documentation"
	TaskTypeBugFix          TaskType = "bug_fix"
)

// ReviewStatus represents the review state of an output package.
type ReviewStatus string

const (
	ReviewStatusPending           ReviewStatus = "pending"
	ReviewStatusApproved          ReviewStatus = "approved"
	ReviewStatusRejected          ReviewStatus = "rejected"
	ReviewStatusChangesRequested  ReviewStatus = "changes_requested"
)

// ActorType represents who performed an audited action.
type ActorType string

const (
	ActorTypeAPIKey ActorType = "api_key"
	ActorTypeSystem ActorType = "system"
)

// OrgUnit represents an organisational unit (tenant) within the grid.
type OrgUnit struct {
	ID        uuid.UUID      `json:"id"`
	Name      string         `json:"name"`
	PlanTier  string         `json:"plan_tier"`
	Policy    map[string]any `json:"policy"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Worker represents a compute node registered with the control plane.
type Worker struct {
	ID              uuid.UUID   `json:"id"`
	OrgUnitID       uuid.UUID   `json:"org_unit_id"`
	HostnameHash    string      `json:"hostname_hash"`
	Agents          []string    `json:"agents"`
	State           WorkerState `json:"state"`
	LastHeartbeatAt *time.Time  `json:"last_heartbeat_at,omitempty"`
	CapacityScore   int         `json:"capacity_score"`
	CreatedAt       time.Time   `json:"created_at"`
}

// Task represents a unit of AI coding work to be executed on a worker.
type Task struct {
	ID               uuid.UUID      `json:"id"`
	OrgUnitID        uuid.UUID      `json:"org_unit_id"`
	SubmitterID      *uuid.UUID     `json:"submitter_id,omitempty"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	TaskType         TaskType       `json:"task_type"`
	Priority         int            `json:"priority"`
	AIAgent          string         `json:"ai_agent"`
	State            TaskState      `json:"state"`
	AssignedWorkerID *uuid.UUID     `json:"assigned_worker_id,omitempty"`
	QueuedAt         time.Time      `json:"queued_at"`
	AssignedAt       *time.Time     `json:"assigned_at,omitempty"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	FailedAt         *time.Time     `json:"failed_at,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	RetryCount       int            `json:"retry_count"`
	MaxRetries       int            `json:"max_retries"`
	Options          map[string]any `json:"options"`
}

// APIKey represents an authentication credential for API access.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	OrgUnitID  uuid.UUID  `json:"org_unit_id"`
	WorkerID   *uuid.UUID `json:"worker_id,omitempty"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// AuditEvent records a security-relevant action performed in the system.
type AuditEvent struct {
	ID           uuid.UUID      `json:"id"`
	OrgUnitID    uuid.UUID      `json:"org_unit_id"`
	ActorType    ActorType      `json:"actor_type"`
	ActorID      string         `json:"actor_id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *string        `json:"resource_id,omitempty"`
	Details      map[string]any `json:"details"`
	IPAddress    string         `json:"ip_address"`
	OccurredAt   time.Time      `json:"occurred_at"`
}

// ArtifactMeta contains metadata about a submitted output artifact.
type ArtifactMeta struct {
	RelPath string `json:"rel_path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// OutputPackage represents artefacts submitted by a worker upon task completion.
type OutputPackage struct {
	ID           uuid.UUID      `json:"id"`
	TaskID       uuid.UUID      `json:"task_id"`
	WorkerID     uuid.UUID      `json:"worker_id"`
	HMACSha256   string         `json:"hmac_sha256"`
	Artifacts    []ArtifactMeta `json:"artifacts"`
	Metadata     map[string]any `json:"metadata"`
	ReviewStatus ReviewStatus   `json:"review_status"`
	ReviewerID   *uuid.UUID     `json:"reviewer_id,omitempty"`
	SubmittedAt  time.Time      `json:"submitted_at"`
	ReviewedAt   *time.Time     `json:"reviewed_at,omitempty"`
}

// BrownieEvent records a single award or deduction of brownie points for a worker.
type BrownieEvent struct {
	ID          uuid.UUID  `json:"id"`
	WorkerID    uuid.UUID  `json:"worker_id"`
	OrgUnitID   uuid.UUID  `json:"org_unit_id"`
	Points      int        `json:"points"`
	Reason      string     `json:"reason"`
	ReferenceID *uuid.UUID `json:"reference_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TokenUsage records AI token consumption for a task execution.
type TokenUsage struct {
	ID           uuid.UUID `json:"id"`
	TaskID       uuid.UUID `json:"task_id"`
	WorkerID     uuid.UUID `json:"worker_id"`
	OrgUnitID    uuid.UUID `json:"org_unit_id"`
	AIAgent      string    `json:"ai_agent"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	RecordedAt   time.Time `json:"recorded_at"`
}

// TimeWindow defines a recurring time range within which certain operations are permitted.
type TimeWindow struct {
	DayOfWeek []int  `json:"day_of_week"` // 0=Sunday … 6=Saturday
	StartHour int    `json:"start_hour"`
	EndHour   int    `json:"end_hour"`
	Timezone  string `json:"timezone"`
}

// OrgPolicy defines the operational rules and constraints for an org unit.
type OrgPolicy struct {
	ExecutionWindows   []TimeWindow `json:"execution_windows"`
	MaxConcurrentJobs  int          `json:"max_concurrent_jobs"`
	AllowedAgents      []string     `json:"allowed_agents"`
	AutoMerge          bool         `json:"auto_merge"`
	RequireReview      bool         `json:"require_review"`
	MaxRetries         int          `json:"max_retries"`
}
