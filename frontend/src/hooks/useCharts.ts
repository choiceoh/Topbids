import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'

export interface Chart {
  id: string
  collection_id: string
  name: string
  chart_type: string
  config: Record<string, unknown>
  sort_order: number
  created_at: string
  updated_at: string
}

interface CreateChartInput {
  name: string
  chart_type: string
  config?: Record<string, unknown>
  sort_order?: number
}

export function useCharts(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.charts.list(collectionId ?? ''),
    queryFn: () => api.getList<Chart>(`/schema/collections/${collectionId}/charts`),
    enabled: !!collectionId,
  })
}

export function useCreateChart(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateChartInput) =>
      api.post<Chart>(`/schema/collections/${collectionId}/charts`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.charts.list(collectionId) })
    },
  })
}

export function useDeleteChart(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (chartId: string) =>
      api.del(`/schema/charts/${chartId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.charts.list(collectionId) })
    },
  })
}
