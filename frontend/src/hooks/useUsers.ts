import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { User } from '@/lib/types'

export function useUsers() {
  return useQuery({
    queryKey: queryKeys.auth.users(),
    queryFn: () => api.get<User[]>('/users'),
    staleTime: 60_000,
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      email: string
      name: string
      password: string
      role: string
      subsidiary_id?: string | null
      department_id?: string | null
      position?: string
      title?: string
      phone?: string
      joined_at?: string | null
    }) => api.post<{ id: string }>('/users', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.auth.users() }),
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...body }: {
      id: string
      name?: string
      email?: string
      role?: string
      is_active?: boolean
      subsidiary_id?: string | null
      department_id?: string | null
      position?: string
      title?: string
      phone?: string
      avatar?: string
      joined_at?: string | null
      password?: string
    }) => api.patch<{ status: string }>(`/users/${id}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.auth.users() })
      qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
    },
  })
}

export function useUpdateMe() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { name?: string; phone?: string; avatar?: string; position?: string; title?: string }) =>
      api.patch<{ status: string }>('/auth/me', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.auth.me() }),
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (body: { current_password: string; new_password: string }) =>
      api.post<{ status: string }>('/auth/password', body),
  })
}
