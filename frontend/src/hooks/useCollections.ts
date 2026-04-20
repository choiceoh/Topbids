/**
 * CRUD hooks for collection (app) metadata.
 *
 * Each "collection" maps to a dynamically-created PostgreSQL table.
 * These hooks manage the schema-level metadata (name, icon, fields),
 * not the row data itself (see useEntries for that).
 *
 * Destructive mutations (delete collection, add/delete field) use a
 * preview-then-confirm two-step pattern: the first call (confirm=false)
 * returns a preview of what will be affected; the second call
 * (confirm=true) executes the change and invalidates the cache.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Collection, CreateCollectionReq, CreateFieldIn, Field } from '@/lib/types'

// --- Reads ---

/** Fetch the full list of collections the current user can access. */
export function useCollections() {
  return useQuery({
    queryKey: queryKeys.collections.list(),
    queryFn: () => api.get<Collection[]>('/schema/collections'),
  })
}

/**
 * Fetch row counts for all collections in a single bulk request.
 * Returns a Record<slug, number>. Uses staleTime of 60s to avoid
 * excessive refetching on the app list page.
 */
export function useCollectionCounts() {
  return useQuery({
    queryKey: [...queryKeys.collections.all, 'counts'],
    queryFn: () => api.get<Record<string, number>>('/schema/collections/counts'),
    staleTime: 60_000,
  })
}

/**
 * Fetch a single collection by ID, including its fields array.
 * Disabled when `id` is undefined (`enabled: !!id`) to support
 * conditional rendering before the ID is known.
 */
export function useCollection(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.collections.detail(id ?? ''),
    queryFn: () => api.get<Collection>(`/schema/collections/${id}`),
    enabled: !!id,
  })
}

// --- Mutations ---

/**
 * Create a new collection. On success, optimistically appends the new
 * collection to the list cache, then invalidates to pull the canonical
 * version (which includes server-computed fields like slug).
 */
export function useCreateCollection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateCollectionReq) =>
      api.post<Collection>('/schema/collections', input),
    onSuccess: (created) => {
      // Optimistic merge: add to the list cache so the new collection appears
      // immediately without a refetch round-trip.
      qc.setQueryData<Collection[]>(queryKeys.collections.list(), (old) =>
        old ? [...old, created] : [created],
      )
      // Invalidate to pull the canonical version (includes computed fields).
      qc.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

/** Update collection metadata (label, icon, access config, etc.). */
export function useUpdateCollection(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: Partial<Pick<Collection, 'label' | 'description' | 'icon' | 'sort_order' | 'process_enabled' | 'access_config' | 'title_field_id' | 'default_sort_field' | 'default_sort_order'>>) =>
      api.patch<Collection>(`/schema/collections/${id}`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.collections.detail(id) })
      qc.invalidateQueries({ queryKey: queryKeys.collections.list() })
    },
  })
}

/**
 * Delete a collection (DROP TABLE). Two-step pattern:
 * 1. `confirm: false` — returns a preview of affected data (row count, relations).
 * 2. `confirm: true` — executes the deletion and invalidates collection caches.
 */
export function useDeleteCollection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, confirm }: { id: string; confirm: boolean }) =>
      api.del<{ confirmation_required?: boolean; preview?: unknown; status?: string }>(
        `/schema/collections/${id}${confirm ? '?confirm=true' : ''}`,
      ),
    onSuccess: (_data, { confirm }) => {
      if (confirm) {
        qc.invalidateQueries({ queryKey: queryKeys.collections.all })
      }
    },
  })
}

/**
 * Add a field (ALTER TABLE ADD COLUMN). Two-step preview/confirm pattern:
 * first call returns a preview; second call with `confirm: true` commits
 * the DDL change and invalidates detail + list caches.
 */
export function useAddField(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ input, confirm }: { input: CreateFieldIn; confirm: boolean }) =>
      api.post<Field | { confirmation_required: boolean; preview: unknown }>(
        `/schema/collections/${collectionId}/fields${confirm ? '?confirm=true' : ''}`,
        input,
      ),
    onSuccess: (_data, { confirm }) => {
      if (confirm) {
        qc.invalidateQueries({ queryKey: queryKeys.collections.detail(collectionId) })
        qc.invalidateQueries({ queryKey: queryKeys.collections.list() })
      }
    },
  })
}

/**
 * Update a field (ALTER TABLE ALTER COLUMN). Two-step preview/confirm pattern
 * for potentially dangerous changes (e.g. type conversion).
 */
export function useUpdateField() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      fieldId,
      input,
      confirm,
    }: {
      fieldId: string
      input: Partial<Pick<Field, 'label' | 'field_type' | 'is_required' | 'is_unique' | 'is_indexed' | 'default_value' | 'options' | 'width' | 'height'>>
      confirm: boolean
    }) =>
      api.patch<Field | { confirmation_required: boolean; preview: unknown }>(
        `/schema/fields/${fieldId}${confirm ? '?confirm=true' : ''}`,
        input,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

/**
 * Delete a field (ALTER TABLE DROP COLUMN). Same two-step preview/confirm
 * pattern as useDeleteCollection — preview shows affected data before commit.
 */
export function useDeleteField() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ fieldId, confirm }: { fieldId: string; confirm: boolean }) =>
      api.del<{ confirmation_required?: boolean; preview?: unknown }>(
        `/schema/fields/${fieldId}${confirm ? '?confirm=true' : ''}`,
      ),
    onSuccess: (_data, { confirm }) => {
      if (confirm) {
        qc.invalidateQueries({ queryKey: queryKeys.collections.all })
      }
    },
  })
}
