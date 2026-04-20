import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'

interface AIHealthResponse {
  available: boolean
}

export function useAIHealth() {
  return useQuery({
    queryKey: ['ai-health'],
    queryFn: () => api.get<AIHealthResponse>('/ai/health'),
    refetchInterval: 30_000,
    staleTime: 15_000,
    retry: false,
  })
}
