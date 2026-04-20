import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { CollectionMember } from '@/lib/types'

export function useMembers(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.members.list(collectionId ?? ''),
    queryFn: () => api.get<CollectionMember[]>(`/schema/collections/${collectionId}/members`),
    enabled: !!collectionId,
  })
}

export function useAddMember(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { user_id: string, role: string }) =>
      api.post<CollectionMember>(`/schema/collections/${collectionId}/members`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.members.list(collectionId) })
    },
  })
}

export function useUpdateMember(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, role }: { userId: string, role: string }) =>
      api.patch<{ status: string }>(`/schema/collections/${collectionId}/members/${userId}`, { role }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.members.list(collectionId) })
    },
  })
}

export function useRemoveMember(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) =>
      api.del<{ status: string }>(`/schema/collections/${collectionId}/members/${userId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.members.list(collectionId) })
    },
  })
}
