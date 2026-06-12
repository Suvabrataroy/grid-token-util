import { memo, useCallback, useMemo, useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'
import { Monitor, Wifi, WifiOff } from 'lucide-react'
import type { Worker, WorkerState } from '../../types'
import { useSSESubscription } from '../../context/SSEContext'
import { VirtualTable, type ColumnDef } from '../../components/VirtualTable'
import { StatCard } from '../../components/StatCard'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STATE_DOT: Record<WorkerState, string> = {
  online:  'bg-grid-success',
  idle:    'bg-blue-400',
  busy:    'bg-yellow-400',
  paused:  'bg-grid-warning',
  offline: 'bg-gray-600',
}

const STATE_LABEL: Record<WorkerState, string> = {
  online:  'Online',
  idle:    'Idle',
  busy:    'Busy',
  paused:  'Paused',
  offline: 'Offline',
}

function StateDot({ state }: { state: WorkerState }) {
  return (
    <span className="flex items-center gap-1.5">
      <span
        className={clsx('h-2 w-2 rounded-full flex-shrink-0', STATE_DOT[state])}
      />
      <span className="text-xs">{STATE_LABEL[state]}</span>
    </span>
  )
}

function shortHash(hostname: string): string {
  const parts = hostname.split('-')
  if (parts.length >= 2) {
    const last = parts[parts.length - 1]
    return last.length > 8 ? `…${last.slice(-8)}` : hostname
  }
  return hostname.length > 20 ? `${hostname.slice(0, 8)}…${hostname.slice(-8)}` : hostname
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface WorkerHealthProps {
  initialWorkers: Worker[]
}

export const WorkerHealth = memo(function WorkerHealth({
  initialWorkers,
}: WorkerHealthProps) {
  const [workers, setWorkers] = useState<Map<string, Worker>>(
    () => new Map(initialWorkers.map((w) => [w.id, w])),
  )

  useSSESubscription('worker_update', (event) => {
    setWorkers((prev) => {
      const next = new Map(prev)
      next.set(event.payload.id, event.payload)
      return next
    })
  })

  const workerList = useMemo(() => Array.from(workers.values()), [workers])

  const counts = useMemo(() => {
    const c: Record<WorkerState, number> = {
      online: 0, idle: 0, busy: 0, paused: 0, offline: 0,
    }
    workerList.forEach((w) => c[w.state]++)
    return c
  }, [workerList])

  const columns = useMemo(
    (): ColumnDef<Worker>[] => [
      {
        key: 'hostname',
        header: 'Worker',
        width: '200px',
        sortable: true,
        render: (w) => (
          <span className="font-mono text-xs text-grid-accent" title={w.hostname}>
            {shortHash(w.hostname)}
          </span>
        ),
      },
      {
        key: 'org_name',
        header: 'Org',
        width: '120px',
        sortable: true,
        render: (w) => (
          <span className="text-xs text-gray-400">{w.org_name}</span>
        ),
      },
      {
        key: 'state',
        header: 'State',
        width: '100px',
        sortable: true,
        render: (w) => <StateDot state={w.state} />,
      },
      {
        key: 'last_heartbeat',
        header: 'Last Heartbeat',
        width: '140px',
        sortable: true,
        render: (w) => (
          <span className="text-xs text-grid-muted" title={w.last_heartbeat}>
            {formatDistanceToNow(new Date(w.last_heartbeat), { addSuffix: true })}
          </span>
        ),
      },
      {
        key: 'agents',
        header: 'Agents',
        render: (w) => (
          <div className="flex flex-wrap gap-1">
            {w.agents.map((a) => (
              <span
                key={a}
                className="px-1.5 py-0.5 rounded text-xs bg-grid-bg text-grid-muted border border-grid-border"
              >
                {a}
              </span>
            ))}
          </div>
        ),
      },
    ],
    [],
  )

  const getRowKey = useCallback((w: Worker) => w.id, [])

  return (
    <div className="flex flex-col gap-4">
      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        <StatCard
          title="Online"
          value={counts.online}
          icon={<Wifi size={16} />}
          colorClass="text-grid-success"
        />
        <StatCard
          title="Idle"
          value={counts.idle}
          colorClass="text-blue-400"
        />
        <StatCard
          title="Busy"
          value={counts.busy}
          colorClass="text-yellow-400"
        />
        <StatCard
          title="Paused"
          value={counts.paused}
          colorClass="text-grid-warning"
        />
        <StatCard
          title="Offline"
          value={counts.offline}
          icon={<WifiOff size={16} />}
          colorClass="text-gray-500"
        />
      </div>

      {/* Worker list */}
      <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-grid-border">
          <Monitor size={15} className="text-grid-muted" />
          <h3 className="text-sm font-semibold text-white">Workers</h3>
          <span className="ml-auto text-xs text-grid-muted">
            {workerList.length} total
          </span>
        </div>
        <VirtualTable
          data={workerList}
          columns={columns}
          containerHeight={360}
          getRowKey={getRowKey}
          rowHeight={44}
          emptyMessage="No workers registered"
        />
      </div>
    </div>
  )
})
