/**
 * CRUD hooks for saved views (user-created filter/sort/visibility presets).
 *
 * Saved views differ from "views" (useViews): a View defines the display
 * type (list, kanban, calendar), while a SavedView is a named preset of
 * filters, sort order, and column visibility within a view. Think of them
 * as bookmarks for frequently-used query configurations.
 *
 * All mutations invalidate the saved-view list for the parent collection.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { CreateSavedViewReq, SavedView, UpdateSavedViewReq } from '@/lib/types'

/** Fetch all saved views for a collection. Disabled when collectionId is undefined. */
export function useSavedViews(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.savedViews.list(collectionId ?? ''),
    queryFn: () => api.get<SavedView[]>(`/schema/collections/${collectionId}/saved-views`),
    enabled: !!collectionId,
  })
}

/** Create a new saved view preset from the current filter/sort/visibility state. */
export function useCreateSavedView(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateSavedViewReq) =>
      api.post<SavedView>(`/schema/collections/${collectionId}/saved-views`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.savedViews.list(collectionId) })
    },
  })
}

/** Update a saved view's name, filters, sort, or visibility config. */
export function useUpdateSavedView(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: UpdateSavedViewReq }) =>
      api.patch<SavedView>(`/schema/saved-views/${id}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.savedViews.list(collectionId) })
    },
  })
}

/** Delete a saved view preset. */
export function useDeleteSavedView(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.del<{ status: string }>(`/schema/saved-views/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.savedViews.list(collectionId) })
    },
  })
}
