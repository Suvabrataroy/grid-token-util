import { Loader2, AlertCircle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchTasks } from '../api/client'
import { TaskOverview } from '../features/task-overview/TaskOverview'

export default function TasksPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => fetchTasks(undefined, 200),
    staleTime: 15_000,
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
        <Loader2 size={20} className="animate-spin" />
        <span className="text-sm">Loading tasks…</span>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
        <AlertCircle size={20} />
        <span className="text-sm">{error?.message ?? 'Failed to load tasks'}</span>
      </div>
    )
  }

  return (
    <div className="max-w-[1400px] mx-auto">
      <TaskOverview initialTasks={data} />
    </div>
  )
}
