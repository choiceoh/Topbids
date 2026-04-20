import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Comment } from '@/lib/types'

export function useComments(slug: string | undefined, recordId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.comments.list(slug ?? '', recordId ?? ''),
    queryFn: () =>
      api.getList<Comment>(`/data/${slug}/${recordId}/comments?limit=100`),
    enabled: !!slug && !!recordId,
  })
}

export function useCreateComment(slug: string, recordId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: string) =>
      api.post<Comment>(`/data/${slug}/${recordId}/comments`, { body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.comments.list(slug, recordId) })
      qc.invalidateQueries({ queryKey: queryKeys.notifications.unreadCount() })
    },
  })
}

export function useDeleteComment(slug: string, recordId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (commentId: string) =>
      api.del<{ status: string }>(`/data/${slug}/${recordId}/comments/${commentId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.comments.list(slug, recordId) })
    },
  })
}
