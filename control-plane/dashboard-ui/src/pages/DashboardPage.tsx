import { type ReactNode } from 'react'
import { Loader2, AlertCircle } from 'lucide-react'
import { useDashboardSnapshot } from '../hooks/useDashboardSnapshot'
import { WorkerHealth }    from '../features/worker-health/WorkerHealth'
import { TaskOverview }    from '../features/task-overview/TaskOverview'
import { TokenUsage }      from '../features/token-usage/TokenUsage'
import { UserActivity }    from '../features/user-activity/UserActivity'
import { SystemStatus }    from '../features/system-status/SystemStatus'
import { SecurityEvents }  from '../features/security-events/SecurityEvents'
import { BrowniePoints }   from '../features/brownie-points/BrowniePoints'

// ---------------------------------------------------------------------------
// Panel wrapper
// ---------------------------------------------------------------------------

function Panel({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-semibold text-grid-muted uppercase tracking-widest">
        {title}
      </h2>
      {children}
    </section>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DashboardPage() {
  const { data, isLoading, isError, error } = useDashboardSnapshot()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
        <Loader2 size={20} className="animate-spin" />
        <span className="text-sm">Loading snapshot…</span>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
        <AlertCircle size={20} />
        <span className="text-sm">
          {error?.message ?? 'Failed to load dashboard data'}
        </span>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-8 max-w-[1600px] mx-auto">
      {/* Row 1: Workers + Tasks */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <Panel title="Worker Health">
          <WorkerHealth initialWorkers={data.workers} />
        </Panel>
        <Panel title="Task Overview">
          <TaskOverview initialTasks={data.recent_tasks} />
        </Panel>
      </div>

      {/* Row 2: Token usage + System status */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <Panel title="Token Usage">
          <TokenUsage data={data.token_usage} />
        </Panel>
        <Panel title="System Status">
          <SystemStatus initial={data.metrics} />
        </Panel>
      </div>

      {/* Row 3: Security + Brownie */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <Panel title="Security Events">
          <SecurityEvents initialEvents={data.security_events} />
        </Panel>
        <Panel title="Leaderboard">
          <BrowniePoints initialLeaderboard={data.leaderboard} />
        </Panel>
      </div>

      {/* Row 4: Activity */}
      <Panel title="User Activity">
        <UserActivity events={data.audit_events} />
      </Panel>
    </div>
  )
}
