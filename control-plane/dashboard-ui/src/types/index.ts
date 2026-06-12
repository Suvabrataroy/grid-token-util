// ---------------------------------------------------------------------------
// Core domain types – mirror Go structs in internal/domain/
// ---------------------------------------------------------------------------

export type WorkerState = 'online' | 'idle' | 'busy' | 'paused' | 'offline'
export type TaskState =
  | 'queued'
  | 'assigned'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'review'
export type Severity = 'info' | 'low' | 'medium' | 'high' | 'critical'

export interface Worker {
  id: string
  hostname: string
  org_id: string
  org_name: string
  state: WorkerState
  agents: string[]
  last_heartbeat: string       // ISO-8601
  registered_at: string        // ISO-8601
  version: string
  labels: Record<string, string>
  brownie_points: number
  tasks_completed: number
  tasks_failed: number
}

export interface Task {
  id: string
  title: string
  type: string
  agent: string
  state: TaskState
  org_id: string
  org_name: string
  submitted_by: string
  submitted_at: string         // ISO-8601
  assigned_worker_id?: string
  assigned_worker_hostname?: string
  started_at?: string          // ISO-8601
  completed_at?: string        // ISO-8601
  error_message?: string
  priority: number
}

export interface OrgUnit {
  id: string
  name: string
  parent_id?: string
  created_at: string
  active: boolean
}

export interface APIKey {
  id: string
  name: string
  org_id: string
  prefix: string
  created_at: string
  last_used_at?: string
  expires_at?: string
  revoked: boolean
}

export interface AuditEvent {
  id: string
  ts: string                   // ISO-8601
  actor: string
  actor_type: 'user' | 'worker' | 'system'
  action: string
  resource_type: string
  resource_id: string
  org_id: string
  ip_address?: string
  details?: Record<string, unknown>
}

export interface OutputPackage {
  id: string
  task_id: string
  worker_id: string
  created_at: string
  size_bytes: number
  checksum: string
  storage_path: string
}

export interface BrownieEntry {
  worker_id: string
  hostname: string
  org_id: string
  org_name: string
  points: number
  points_delta: number         // change since yesterday (+/-)
  tasks_completed: number
  total_events: number
  rank: number
}

export interface TokenUsageDay {
  date: string                 // YYYY-MM-DD
  agent: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
}

export interface TokenUsageOrg {
  org_id: string
  org_name: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
}

export interface TokenUsage {
  daily: TokenUsageDay[]
  by_org: TokenUsageOrg[]
  month_input: number
  month_output: number
  month_total: number
}

export interface SystemMetrics {
  ts: string
  db_latency_ms: number
  redis_latency_ms: number
  scheduler_running: boolean
  queue_depth: number
  worker_count: number
  active_tasks: number
  response_time_p50_ms: number
  response_time_p95_ms: number
  response_time_p99_ms: number
  version: string
  build_sha: string
}

export interface SecurityEvent {
  id: string
  ts: string                   // ISO-8601
  worker_id: string
  worker_hostname: string
  rule_id: string
  rule_name: string
  severity: Severity
  action_taken: string
  reviewed: boolean
  details?: Record<string, unknown>
}

// ---------------------------------------------------------------------------
// SSE event union
// ---------------------------------------------------------------------------

export interface SSETaskUpdate {
  type: 'task_update'
  payload: Task
}

export interface SSEWorkerUpdate {
  type: 'worker_update'
  payload: Worker
}

export interface SSESecurityEvent {
  type: 'security_event'
  payload: SecurityEvent
}

export interface SSEBrownieUpdate {
  type: 'brownie_update'
  payload: BrownieEntry
}

export interface SSESystemMetrics {
  type: 'system_metrics'
  payload: SystemMetrics
}

export type SSEEvent =
  | SSETaskUpdate
  | SSEWorkerUpdate
  | SSESecurityEvent
  | SSEBrownieUpdate
  | SSESystemMetrics

// ---------------------------------------------------------------------------
// Dashboard snapshot – single fetch for initial page load
// ---------------------------------------------------------------------------

export interface DashboardSnapshot {
  workers: Worker[]
  recent_tasks: Task[]
  token_usage: TokenUsage
  leaderboard: BrownieEntry[]
  security_events: SecurityEvent[]
  metrics: SystemMetrics
  audit_events: AuditEvent[]
  generated_at: string         // ISO-8601
}

// ---------------------------------------------------------------------------
// UI helpers
// ---------------------------------------------------------------------------

export type ConnectionState = 'connected' | 'reconnecting' | 'disconnected'
