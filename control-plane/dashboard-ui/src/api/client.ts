import type {
  DashboardSnapshot,
  Worker,
  Task,
  AuditEvent,
  BrownieEntry,
} from '../types'

const BASE = '/api/v1'

async function request<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Accept': 'application/json' },
  })
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`API ${res.status}: ${text}`)
  }
  return res.json() as Promise<T>
}

export async function fetchSnapshot(): Promise<DashboardSnapshot> {
  return request<DashboardSnapshot>('/dashboard/snapshot')
}

export async function fetchWorkers(orgId?: string): Promise<Worker[]> {
  const qs = orgId ? `?org_id=${encodeURIComponent(orgId)}` : ''
  return request<Worker[]>(`/workers${qs}`)
}

export async function fetchTasks(
  orgId?: string,
  limit = 100,
): Promise<Task[]> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (orgId) params.set('org_id', orgId)
  return request<Task[]>(`/tasks?${params.toString()}`)
}

export async function fetchAuditLog(
  from: Date,
  to: Date,
): Promise<AuditEvent[]> {
  const params = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
  })
  return request<AuditEvent[]>(`/audit?${params.toString()}`)
}

export async function fetchLeaderboard(orgId?: string): Promise<BrownieEntry[]> {
  const qs = orgId ? `?org_id=${encodeURIComponent(orgId)}` : ''
  return request<BrownieEntry[]>(`/leaderboard${qs}`)
}
