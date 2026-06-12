import { Loader2, AlertCircle } from 'lucide-react'
import { useDashboardSnapshot } from '../hooks/useDashboardSnapshot'
import { SecurityEvents } from '../features/security-events/SecurityEvents'

export default function SecurityPage() {
  const { data, isLoading, isError, error } = useDashboardSnapshot()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
        <Loader2 size={20} className="animate-spin" />
        <span className="text-sm">Loading security events…</span>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
        <AlertCircle size={20} />
        <span className="text-sm">{error?.message ?? 'Failed to load security events'}</span>
      </div>
    )
  }

  return (
    <div className="max-w-[1200px] mx-auto">
      <SecurityEvents initialEvents={data.security_events} />
    </div>
  )
}
