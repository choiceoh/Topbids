import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Automation, AutomationRun, CreateAutomationReq } from '@/lib/types'

export function useAutomations(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.automations.list(collectionId ?? ''),
    queryFn: () => api.get<Automation[]>(`/schema/collections/${collectionId}/automations`),
    enabled: !!collectionId,
  })
}

export function useAutomation(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.automations.detail(id ?? ''),
    queryFn: () => api.get<Automation>(`/schema/automations/${id}`),
    enabled: !!id,
  })
}

export function useCreateAutomation(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAutomationReq) =>
      api.post<{ id: string }>(`/schema/collections/${collectionId}/automations`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.automations.list(collectionId) })
    },
  })
}

export function useUpdateAutomation(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: CreateAutomationReq & { id: string }) =>
      api.patch<void>(`/schema/automations/${id}`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.automations.list(collectionId) })
    },
  })
}

export function useDeleteAutomation(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/schema/automations/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.automations.list(collectionId) })
    },
  })
}

export function useAutomationRuns(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.automations.runs(id ?? ''),
    queryFn: () => api.getList<AutomationRun>(`/schema/automations/${id}/runs`),
    enabled: !!id,
  })
}
