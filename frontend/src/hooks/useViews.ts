/**
 * CRUD hooks for collection views (list, kanban, calendar, gallery, gantt, etc.).
 *
 * Views define how entries are displayed — each view has a type, field
 * visibility/ordering, default sort, and filter configuration. All mutations
 * invalidate the view list cache for the parent collection.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { CreateViewReq, UpdateViewReq, View } from '@/lib/types'

/** Fetch all views for a collection. Disabled when collectionId is undefined. */
export function useViews(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.views.list(collectionId ?? ''),
    queryFn: () => api.get<View[]>(`/schema/collections/${collectionId}/views`),
    enabled: !!collectionId,
  })
}

/** Create a new view (e.g., adding a Kanban board to a collection). */
export function useCreateView(collectionId: string) {
  const qc = useQueryClient()
  const key = queryKeys.views.list(collectionId)
  return useMutation({
    mutationFn: (body: CreateViewReq) =>
      api.post<View>(`/schema/collections/${collectionId}/views`, body),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData<View[]>(key)
      qc.setQueryData<View[]>(key, (old) => [
        ...(old ?? []),
        { id: `_optimistic_${Date.now()}`, collection_id: collectionId, ...body } as View,
      ])
      return { prev }
    },
    onError: (_err, _body, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev)
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: key })
    },
  })
}

/** Update view config (field visibility, sort order, filters, etc.). */
export function useUpdateView(collectionId: string) {
  const qc = useQueryClient()
  const key = queryKeys.views.list(collectionId)
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: UpdateViewReq }) =>
      api.patch<View>(`/schema/views/${id}`, body),
    onMutate: async ({ id, body }) => {
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData<View[]>(key)
      qc.setQueryData<View[]>(key, (old) =>
        (old ?? []).map((v) => (v.id === id ? { ...v, ...body } : v)),
      )
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev)
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: key })
    },
  })
}

/** Delete a view. The UI should prevent deleting the last remaining view. */
export function useDeleteView(collectionId: string) {
  const qc = useQueryClient()
  const key = queryKeys.views.list(collectionId)
  return useMutation({
    mutationFn: (id: string) => api.del<{ status: string }>(`/schema/views/${id}`),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData<View[]>(key)
      qc.setQueryData<View[]>(key, (old) =>
        (old ?? []).filter((v) => v.id !== id),
      )
      return { prev }
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev)
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: key })
    },
  })
}
