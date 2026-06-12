import { useQuery } from '@tanstack/react-query'
import { fetchSnapshot } from '../api/client'
import type { DashboardSnapshot } from '../types'

export function useDashboardSnapshot() {
  return useQuery<DashboardSnapshot, Error>({
    queryKey: ['dashboard', 'snapshot'],
    queryFn: fetchSnapshot,
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })
}
