import { memo, useMemo, useState, type ReactNode } from 'react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'
import { Activity, User, Bot, Settings } from 'lucide-react'
import type { AuditEvent } from '../../types'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ACTOR_ICON: Record<AuditEvent['actor_type'], ReactNode> = {
  user:   <User size={13} className="text-blue-400" />,
  worker: <Bot size={13} className="text-green-400" />,
  system: <Settings size={13} className="text-grid-muted" />,
}

const ACTION_COLOR: Record<string, string> = {
  submit:   'text-blue-400',
  complete: 'text-green-400',
  fail:     'text-red-400',
  assign:   'text-yellow-400',
  review:   'text-purple-400',
  create:   'text-grid-accent',
  delete:   'text-red-500',
  update:   'text-orange-400',
}

function actionColor(action: string): string {
  const key = Object.keys(ACTION_COLOR).find((k) =>
    action.toLowerCase().includes(k),
  )
  return key ? ACTION_COLOR[key] : 'text-gray-400'
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface UserActivityProps {
  events: AuditEvent[]
}

export const UserActivity = memo(function UserActivity({
  events,
}: UserActivityProps) {
  const [filter, setFilter] = useState('')

  const filtered = useMemo(() => {
    if (!filter.trim()) return events.slice(0, 100)
    const q = filter.toLowerCase()
    return events
      .filter(
        (e) =>
          e.actor.toLowerCase().includes(q) ||
          e.action.toLowerCase().includes(q) ||
          e.resource_type.toLowerCase().includes(q) ||
          e.resource_id.toLowerCase().includes(q),
      )
      .slice(0, 100)
  }, [events, filter])

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <Activity size={15} className="text-grid-muted flex-shrink-0" />
        <h3 className="text-sm font-semibold text-white">User Activity</h3>
        <input
          type="text"
          placeholder="Filter…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="ml-auto w-48 text-xs bg-grid-bg border border-grid-border rounded px-2 py-1 text-gray-300 placeholder-gray-600 focus:outline-none focus:border-grid-accent"
        />
      </div>

      <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
        <div className="overflow-y-auto" style={{ maxHeight: 400 }}>
          {filtered.length === 0 ? (
            <div className="text-center text-grid-muted text-sm py-10">
              No activity events
            </div>
          ) : (
            <div className="divide-y divide-grid-border/40">
              {filtered.map((event) => (
                <div
                  key={event.id}
                  className="flex items-start gap-3 px-4 py-3 hover:bg-white/5 transition-colors animate-fade-in"
                >
                  {/* Actor icon */}
                  <span className="mt-0.5 flex-shrink-0">
                    {ACTOR_ICON[event.actor_type]}
                  </span>

                  {/* Content */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-baseline gap-1.5 flex-wrap">
                      <span className="text-xs font-semibold text-white truncate">
                        {event.actor}
                      </span>
                      <span
                        className={clsx(
                          'text-xs font-medium',
                          actionColor(event.action),
                        )}
                      >
                        {event.action}
                      </span>
                      <span className="text-xs text-grid-muted">
                        {event.resource_type}
                      </span>
                      <span
                        className="text-xs font-mono text-gray-500 truncate max-w-[120px]"
                        title={event.resource_id}
                      >
                        {event.resource_id.slice(0, 12)}…
                      </span>
                    </div>
                    {event.org_id && (
                      <span className="text-xs text-gray-600">
                        org: {event.org_id.slice(0, 8)}
                      </span>
                    )}
                  </div>

                  {/* Time */}
                  <span
                    className="text-xs text-grid-muted flex-shrink-0"
                    title={event.ts}
                  >
                    {formatDistanceToNow(new Date(event.ts), { addSuffix: true })}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
})
