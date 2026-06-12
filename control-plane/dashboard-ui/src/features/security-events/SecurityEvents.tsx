import { memo, useCallback, useMemo, useRef, useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'
import { ShieldAlert, Eye, Bell } from 'lucide-react'
import type { SecurityEvent, Severity } from '../../types'
import { useSSESubscription } from '../../context/SSEContext'
import { VirtualTable, type ColumnDef } from '../../components/VirtualTable'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const SEV_BADGE: Record<Severity, string> = {
  info:     'bg-gray-700 text-gray-300',
  low:      'bg-blue-900 text-blue-300',
  medium:   'bg-yellow-900 text-yellow-300',
  high:     'bg-orange-900 text-orange-300',
  critical: 'bg-red-900 text-red-300 font-bold',
}

function SeverityBadge({ sev }: { sev: Severity }) {
  return (
    <span
      className={clsx('px-2 py-0.5 rounded text-xs uppercase tracking-wide', SEV_BADGE[sev])}
    >
      {sev}
    </span>
  )
}

const NEW_HIGHLIGHT_MS = 3_000

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface SecurityEventsProps {
  initialEvents: SecurityEvent[]
}

export const SecurityEvents = memo(function SecurityEvents({
  initialEvents,
}: SecurityEventsProps) {
  const [events, setEvents] = useState<SecurityEvent[]>(initialEvents)
  const newIds = useRef<Set<string>>(new Set())
  const [, forceRender] = useState(0)

  useSSESubscription('security_event', (sse) => {
    const ev = sse.payload
    newIds.current.add(ev.id)
    setEvents((prev) => [ev, ...prev].slice(0, 1000))

    // Remove highlight after 3s
    setTimeout(() => {
      newIds.current.delete(ev.id)
      forceRender((n) => n + 1)
    }, NEW_HIGHLIGHT_MS)
  })

  const unreviewedCount = useMemo(
    () => events.filter((e) => !e.reviewed).length,
    [events],
  )

  const criticalCount = useMemo(
    () => events.filter((e) => e.severity === 'critical').length,
    [events],
  )

  const columns = useMemo(
    (): ColumnDef<SecurityEvent>[] => [
      {
        key: 'ts',
        header: 'Time',
        width: '130px',
        sortable: true,
        render: (e) => (
          <span className="text-xs text-grid-muted" title={e.ts}>
            {formatDistanceToNow(new Date(e.ts), { addSuffix: true })}
          </span>
        ),
      },
      {
        key: 'worker_hostname',
        header: 'Worker',
        width: '150px',
        sortable: true,
        render: (e) => (
          <span
            className="font-mono text-xs text-grid-accent truncate"
            title={e.worker_hostname}
          >
            {e.worker_hostname.slice(-12)}
          </span>
        ),
      },
      {
        key: 'rule_name',
        header: 'Rule',
        sortable: true,
        render: (e) => (
          <span className="text-xs text-gray-300" title={e.rule_id}>
            {e.rule_name}
          </span>
        ),
      },
      {
        key: 'severity',
        header: 'Severity',
        width: '100px',
        sortable: true,
        render: (e) => <SeverityBadge sev={e.severity} />,
      },
      {
        key: 'action_taken',
        header: 'Action',
        width: '130px',
        render: (e) => (
          <span className="text-xs text-gray-400">{e.action_taken}</span>
        ),
      },
      {
        key: 'reviewed',
        header: '',
        width: '36px',
        render: (e) =>
          e.reviewed ? (
            <Eye size={13} className="text-grid-muted" />
          ) : (
            <span className="h-2 w-2 rounded-full bg-grid-warning inline-block" />
          ),
      },
    ],
    [],
  )

  const getRowKey = useCallback((e: SecurityEvent) => e.id, [])

  return (
    <div className="flex flex-col gap-4">
      {/* Alert banner */}
      {criticalCount > 0 && (
        <div className="flex items-center gap-3 bg-grid-danger-bg border border-grid-danger rounded-lg px-4 py-3 animate-fade-in">
          <ShieldAlert size={18} className="text-grid-danger flex-shrink-0" />
          <span className="text-sm text-grid-danger font-semibold">
            {criticalCount} critical security event{criticalCount > 1 ? 's' : ''} require attention
          </span>
        </div>
      )}

      <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-grid-border">
          <ShieldAlert size={15} className="text-grid-muted" />
          <h3 className="text-sm font-semibold text-white">Security Events</h3>
          {unreviewedCount > 0 && (
            <span className="flex items-center gap-1 ml-1 px-2 py-0.5 rounded-full bg-grid-warning/20 text-grid-warning text-xs font-semibold">
              <Bell size={10} />
              {unreviewedCount}
            </span>
          )}
          <span className="ml-auto text-xs text-grid-muted">
            {events.length} events
          </span>
        </div>

        <VirtualTable
          data={events}
          columns={columns}
          containerHeight={360}
          getRowKey={getRowKey}
          rowHeight={44}
          emptyMessage="No security events"
        />
      </div>
    </div>
  )
})
