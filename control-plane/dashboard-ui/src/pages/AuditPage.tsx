import { useState } from 'react'
import { subDays } from 'date-fns'
import { Loader2, AlertCircle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchAuditLog } from '../api/client'
import { UserActivity } from '../features/user-activity/UserActivity'

export default function AuditPage() {
  const [days, setDays] = useState(7)

  const from = subDays(new Date(), days)
  const to   = new Date()

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['audit', days],
    queryFn: () => fetchAuditLog(from, to),
    staleTime: 60_000,
  })

  return (
    <div className="max-w-[1200px] mx-auto flex flex-col gap-4">
      {/* Range selector */}
      <div className="flex items-center gap-3">
        <span className="text-xs text-grid-muted">Show last</span>
        {[1, 7, 14, 30].map((d) => (
          <button
            key={d}
            onClick={() => setDays(d)}
            className={`px-3 py-1 rounded text-xs border transition-colors ${
              days === d
                ? 'border-grid-accent text-grid-accent bg-grid-accent/10'
                : 'border-grid-border text-grid-muted hover:text-white hover:border-gray-500'
            }`}
          >
            {d}d
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
          <Loader2 size={20} className="animate-spin" />
          <span className="text-sm">Loading audit log…</span>
        </div>
      )}

      {isError && (
        <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
          <AlertCircle size={20} />
          <span className="text-sm">{error?.message ?? 'Failed to load audit log'}</span>
        </div>
      )}

      {data && <UserActivity events={data} />}
    </div>
  )
}
