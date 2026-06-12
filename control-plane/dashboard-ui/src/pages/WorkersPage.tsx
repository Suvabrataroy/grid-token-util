import { Loader2, AlertCircle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchWorkers } from '../api/client'
import { WorkerHealth } from '../features/worker-health/WorkerHealth'

export default function WorkersPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['workers'],
    queryFn: () => fetchWorkers(),
    staleTime: 30_000,
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
        <Loader2 size={20} className="animate-spin" />
        <span className="text-sm">Loading workers…</span>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
        <AlertCircle size={20} />
        <span className="text-sm">{error?.message ?? 'Failed to load workers'}</span>
      </div>
    )
  }

  return (
    <div className="max-w-[1400px] mx-auto">
      <WorkerHealth initialWorkers={data} />
    </div>
  )
}
