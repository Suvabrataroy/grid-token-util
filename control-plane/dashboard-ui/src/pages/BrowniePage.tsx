import { Loader2, AlertCircle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchLeaderboard } from '../api/client'
import { BrowniePoints } from '../features/brownie-points/BrowniePoints'

export default function BrowniePage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['leaderboard'],
    queryFn: () => fetchLeaderboard(),
    staleTime: 30_000,
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-muted gap-2">
        <Loader2 size={20} className="animate-spin" />
        <span className="text-sm">Loading leaderboard…</span>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex items-center justify-center h-64 text-grid-danger gap-2">
        <AlertCircle size={20} />
        <span className="text-sm">{error?.message ?? 'Failed to load leaderboard'}</span>
      </div>
    )
  }

  return (
    <div className="max-w-[800px] mx-auto">
      <BrowniePoints initialLeaderboard={data} />
    </div>
  )
}
