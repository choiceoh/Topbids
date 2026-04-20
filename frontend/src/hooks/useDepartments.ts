import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Department } from '@/lib/types'

export function useDepartments() {
  return useQuery({
    queryKey: queryKeys.departments.list(),
    queryFn: () => api.get<Department[]>('/departments'),
  })
}

export function useCreateDepartment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; parent_id?: string | null; subsidiary_id?: string | null; sort_order?: number }) =>
      api.post<{ id: string }>('/departments', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.departments.all }),
  })
}

export function useUpdateDepartment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; parent_id?: string | null; subsidiary_id?: string | null; sort_order?: number }) =>
      api.patch<{ status: string }>(`/departments/${id}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.departments.all }),
  })
}

export function useDeleteDepartment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.del<{ status: string }>(`/departments/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.departments.all }),
  })
}
