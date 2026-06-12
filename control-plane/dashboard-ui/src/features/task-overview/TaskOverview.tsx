import { memo, useCallback, useMemo, useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'
import { ClipboardList } from 'lucide-react'
import type { Task, TaskState } from '../../types'
import { useSSESubscription } from '../../context/SSEContext'
import { VirtualTable, type ColumnDef } from '../../components/VirtualTable'
import { StatCard } from '../../components/StatCard'
import { isToday } from 'date-fns'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STATE_BADGE: Record<TaskState, { label: string; cls: string }> = {
  queued:    { label: 'Queued',    cls: 'bg-gray-700 text-gray-300' },
  assigned:  { label: 'Assigned',  cls: 'bg-blue-900 text-blue-300' },
  running:   { label: 'Running',   cls: 'bg-yellow-900 text-yellow-300' },
  completed: { label: 'Done',      cls: 'bg-green-900 text-green-300' },
  failed:    { label: 'Failed',    cls: 'bg-red-900 text-red-300' },
  cancelled: { label: 'Cancelled', cls: 'bg-gray-800 text-gray-500' },
  review:    { label: 'Review',    cls: 'bg-purple-900 text-purple-300' },
}

function StateBadge({ state }: { state: TaskState }) {
  const { label, cls } = STATE_BADGE[state]
  return (
    <span className={clsx('px-2 py-0.5 rounded text-xs font-medium', cls)}>
      {label}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface TaskOverviewProps {
  initialTasks: Task[]
}

export const TaskOverview = memo(function TaskOverview({
  initialTasks,
}: TaskOverviewProps) {
  const [tasks, setTasks] = useState<Task[]>(initialTasks)

  useSSESubscription('task_update', (event) => {
    setTasks((prev) => {
      const idx = prev.findIndex((t) => t.id === event.payload.id)
      if (idx === -1) return [event.payload, ...prev].slice(0, 500)
      const next = [...prev]
      next[idx] = event.payload
      return next
    })
  })

  const metrics = useMemo(() => {
    let queued = 0, running = 0, completedToday = 0, failedToday = 0
    for (const t of tasks) {
      if (t.state === 'queued') queued++
      if (t.state === 'running' || t.state === 'assigned') running++
      if (t.state === 'completed' && t.completed_at && isToday(new Date(t.completed_at)))
        completedToday++
      if (t.state === 'failed' && t.completed_at && isToday(new Date(t.completed_at)))
        failedToday++
    }
    return { queued, running, completedToday, failedToday }
  }, [tasks])

  const columns = useMemo(
    (): ColumnDef<Task>[] => [
      {
        key: 'title',
        header: 'Title',
        sortable: true,
        render: (t) => (
          <span className="text-xs text-white truncate" title={t.title}>
            {t.title}
          </span>
        ),
      },
      {
        key: 'type',
        header: 'Type',
        width: '100px',
        sortable: true,
        render: (t) => (
          <span className="text-xs text-grid-muted">{t.type}</span>
        ),
      },
      {
        key: 'agent',
        header: 'Agent',
        width: '100px',
        sortable: true,
        render: (t) => (
          <span className="font-mono text-xs text-grid-accent">{t.agent}</span>
        ),
      },
      {
        key: 'state',
        header: 'State',
        width: '90px',
        sortable: true,
        render: (t) => <StateBadge state={t.state} />,
      },
      {
        key: 'org_name',
        header: 'Org',
        width: '110px',
        sortable: true,
        render: (t) => (
          <span className="text-xs text-gray-400">{t.org_name}</span>
        ),
      },
      {
        key: 'submitted_at',
        header: 'Submitted',
        width: '130px',
        sortable: true,
        render: (t) => (
          <span className="text-xs text-grid-muted" title={t.submitted_at}>
            {formatDistanceToNow(new Date(t.submitted_at), { addSuffix: true })}
          </span>
        ),
      },
      {
        key: 'assigned_worker_hostname',
        header: 'Worker',
        width: '140px',
        render: (t) =>
          t.assigned_worker_hostname ? (
            <span
              className="font-mono text-xs text-gray-400 truncate"
              title={t.assigned_worker_hostname}
            >
              {t.assigned_worker_hostname.slice(-12)}
            </span>
          ) : (
            <span className="text-xs text-gray-700">—</span>
          ),
      },
    ],
    [],
  )

  const getRowKey = useCallback((t: Task) => t.id, [])

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard title="Queued"      value={metrics.queued}         colorClass="text-gray-300" />
        <StatCard title="Running"     value={metrics.running}        colorClass="text-yellow-400" />
        <StatCard title="Done Today"  value={metrics.completedToday} colorClass="text-grid-success" />
        <StatCard title="Failed Today"value={metrics.failedToday}    colorClass="text-grid-danger" />
      </div>

      <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-grid-border">
          <ClipboardList size={15} className="text-grid-muted" />
          <h3 className="text-sm font-semibold text-white">Recent Tasks</h3>
          <span className="ml-auto text-xs text-grid-muted">
            {tasks.length} loaded
          </span>
        </div>
        <VirtualTable
          data={tasks}
          columns={columns}
          containerHeight={360}
          getRowKey={getRowKey}
          rowHeight={44}
          emptyMessage="No tasks found"
        />
      </div>
    </div>
  )
})
