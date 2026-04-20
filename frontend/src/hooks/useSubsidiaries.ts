import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Subsidiary } from '@/lib/types'

export function useSubsidiaries() {
  return useQuery({
    queryKey: queryKeys.subsidiaries.list(),
    queryFn: () => api.get<Subsidiary[]>('/subsidiaries'),
  })
}

export function useCreateSubsidiary() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; external_code?: string; sort_order?: number; is_active?: boolean }) =>
      api.post<Subsidiary>('/subsidiaries', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subsidiaries.all }),
  })
}

export function useUpdateSubsidiary() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; external_code?: string; sort_order?: number; is_active?: boolean }) =>
      api.patch<{ status: string }>(`/subsidiaries/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subsidiaries.all }),
  })
}

export function useDeleteSubsidiary() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.del<{ status: string }>(`/subsidiaries/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subsidiaries.all }),
  })
}
