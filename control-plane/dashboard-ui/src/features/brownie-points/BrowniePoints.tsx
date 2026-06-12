import { memo, useMemo, useState } from 'react'
import clsx from 'clsx'
import { TrendingUp, TrendingDown, Minus, Award } from 'lucide-react'
import type { BrownieEntry } from '../../types'
import { useSSESubscription } from '../../context/SSEContext'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function Medal({ rank }: { rank: number }) {
  if (rank === 1) return <span title="Gold"   className="text-yellow-400 text-sm">🥇</span>
  if (rank === 2) return <span title="Silver" className="text-gray-300 text-sm">🥈</span>
  if (rank === 3) return <span title="Bronze" className="text-orange-400 text-sm">🥉</span>
  return <span className="text-xs text-grid-muted font-mono tabular-nums w-5 text-center">{rank}</span>
}

function TrendIcon({ delta }: { delta: number }) {
  if (delta > 0)
    return (
      <span className="flex items-center gap-0.5 text-green-400 text-xs">
        <TrendingUp size={12} />+{delta}
      </span>
    )
  if (delta < 0)
    return (
      <span className="flex items-center gap-0.5 text-red-400 text-xs">
        <TrendingDown size={12} />{delta}
      </span>
    )
  return (
    <span className="flex items-center gap-0.5 text-grid-muted text-xs">
      <Minus size={12} />0
    </span>
  )
}

function shortHost(hostname: string): string {
  const parts = hostname.split('-')
  if (parts.length > 2) return parts.slice(-2).join('-')
  return hostname.length > 18 ? `${hostname.slice(0, 8)}…${hostname.slice(-6)}` : hostname
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface BrowniePointsProps {
  initialLeaderboard: BrownieEntry[]
}

export const BrowniePoints = memo(function BrowniePoints({
  initialLeaderboard,
}: BrowniePointsProps) {
  const [entries, setEntries] = useState<BrownieEntry[]>(initialLeaderboard)

  useSSESubscription('brownie_update', (sse) => {
    setEntries((prev) => {
      const next = prev.map((e) =>
        e.worker_id === sse.payload.worker_id ? sse.payload : e,
      )
      // If not found, append
      if (!prev.find((e) => e.worker_id === sse.payload.worker_id)) {
        next.push(sse.payload)
      }
      // Re-rank
      next.sort((a, b) => b.points - a.points)
      next.forEach((e, i) => { e.rank = i + 1 })
      return next
    })
  })

  const top10 = useMemo(() => entries.slice(0, 10), [entries])
  const maxPoints = top10[0]?.points ?? 1

  return (
    <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-grid-border">
        <Award size={15} className="text-yellow-400" />
        <h3 className="text-sm font-semibold text-white">Brownie Points</h3>
        <span className="ml-auto text-xs text-grid-muted">Top 10</span>
      </div>

      {top10.length === 0 ? (
        <div className="text-center text-grid-muted text-sm py-10">
          No leaderboard data
        </div>
      ) : (
        <div className="divide-y divide-grid-border/40">
          {top10.map((entry) => (
            <div
              key={entry.worker_id}
              className={clsx(
                'flex items-center gap-3 px-4 py-3 hover:bg-white/5 transition-colors',
                entry.rank <= 3 && 'bg-white/[0.02]',
              )}
            >
              {/* Rank / Medal */}
              <div className="w-6 flex justify-center flex-shrink-0">
                <Medal rank={entry.rank} />
              </div>

              {/* Worker info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-baseline gap-2">
                  <span
                    className="font-mono text-xs text-white truncate"
                    title={entry.hostname}
                  >
                    {shortHost(entry.hostname)}
                  </span>
                  <span className="text-xs text-grid-muted truncate">
                    {entry.org_name}
                  </span>
                </div>
                {/* Progress bar */}
                <div className="mt-1.5 h-1 bg-grid-border rounded-full overflow-hidden">
                  <div
                    className={clsx(
                      'h-full rounded-full transition-all duration-700',
                      entry.rank === 1 ? 'bg-yellow-400' :
                      entry.rank === 2 ? 'bg-gray-400' :
                      entry.rank === 3 ? 'bg-orange-500' :
                      'bg-grid-accent',
                    )}
                    style={{ width: `${(entry.points / maxPoints) * 100}%` }}
                  />
                </div>
              </div>

              {/* Stats */}
              <div className="flex flex-col items-end gap-0.5 flex-shrink-0">
                <span className="text-sm font-bold tabular-nums text-white">
                  {entry.points.toLocaleString()}
                </span>
                <TrendIcon delta={entry.points_delta} />
              </div>

              {/* Total events */}
              <div className="hidden sm:flex flex-col items-end flex-shrink-0 w-16">
                <span className="text-xs text-grid-muted">events</span>
                <span className="text-xs font-mono text-gray-400 tabular-nums">
                  {entry.total_events.toLocaleString()}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
})
