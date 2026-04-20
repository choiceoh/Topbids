import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Notification } from '@/lib/types'

export function useNotifications() {
  return useQuery({
    queryKey: queryKeys.notifications.list(),
    queryFn: () => api.getList<Notification>('/notifications?limit=20'),
  })
}

export function useUnreadCount() {
  return useQuery({
    queryKey: queryKeys.notifications.unreadCount(),
    queryFn: () => api.get<{ count: number }>('/notifications/unread-count'),
  })
}

export function useMarkRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.patch<{ status: string }>(`/notifications/${id}/read`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
  })
}

export function useMarkAllRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<{ status: string }>('/notifications/read-all', {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
  })
}
