/**
 * Authentication hooks.
 *
 * Auth state is managed via React Query with aggressive caching
 * (staleTime: Infinity) since the current user rarely changes mid-session.
 * Login seeds the cache to avoid an extra /api/auth/me fetch.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { User } from '@/lib/types'

interface LoginInput {
  email: string
  password: string
}

interface LoginResponse {
  token: string
  user: User
}

/**
 * Fetch the current authenticated user from GET /api/auth/me.
 *
 * - `staleTime: Infinity` — cached until explicitly invalidated (login/logout/role change).
 * - `retry: false` — a 401 means the user is unauthenticated; retrying would be pointless.
 */
export function useCurrentUser() {
  return useQuery({
    queryKey: queryKeys.auth.me(),
    queryFn: () => api.get<User>('/auth/me'),
    staleTime: Infinity, // never stale until invalidated
    retry: false, // 401 should not retry — useAuth handles it
  })
}

/**
 * Login mutation. On success, seeds the React Query cache with the returned
 * user via `setQueryData` so the app can render immediately without an
 * extra GET /api/auth/me round-trip.
 */
export function useLogin() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (input: LoginInput) => api.post<LoginResponse>('/auth/login', input),
    onSuccess: (data) => {
      // Seed the /me cache with the user we just got back so the layout
      // doesn't have to refetch.
      queryClient.setQueryData(queryKeys.auth.me(), data.user)
      navigate('/', { replace: true })
    },
  })
}

/**
 * Logout mutation. Clears all auth-related query cache via `removeQueries`
 * and redirects to /login.
 */
export function useLogout() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: () => api.post<void>('/auth/logout'),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.auth.all })
      navigate('/login', { replace: true })
    },
  })
}

/**
 * Check if a user holds at least one of the given roles.
 * Returns false for null/undefined users (unauthenticated state).
 */
export function hasRole(user: User | null | undefined, roles: User['role'][]): boolean {
  if (!user) return false
  return roles.includes(user.role)
}

/**
 * Check if a user can manage (edit/delete) a collection.
 * Allowed when the user is a director or PM, or is the collection's creator.
 */
export function canManageCollection(
  user: User | null | undefined,
  collectionCreatedBy?: string,
): boolean {
  if (!user) return false
  if (hasRole(user, ['director', 'pm'])) return true
  return !!collectionCreatedBy && collectionCreatedBy === user.id
}
