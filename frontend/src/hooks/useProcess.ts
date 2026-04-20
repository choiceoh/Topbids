import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Process, SaveProcessReq } from '@/lib/types'

/**
 * Fetch the workflow (process) configuration for a collection.
 * Returns statuses, transitions, and assignment rules.
 * Disabled when collectionId is undefined.
 */
export function useProcess(collectionId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.process.detail(collectionId ?? ''),
    queryFn: () => api.get<Process>(`/schema/collections/${collectionId}/process`),
    enabled: !!collectionId,
  })
}

/**
 * Save (create or update) a workflow process configuration.
 *
 * Invalidates three cache families on success:
 * - process detail — the config itself changed
 * - collection detail — process_enabled flag may have toggled
 * - all entries — workflow changes affect status display, allowed
 *   transitions, and kanban column ordering
 */
export function useSaveProcess(collectionId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SaveProcessReq) =>
      api.put<Process>(`/schema/collections/${collectionId}/process`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.process.detail(collectionId) })
      qc.invalidateQueries({ queryKey: queryKeys.collections.detail(collectionId) })
      qc.invalidateQueries({ queryKey: queryKeys.entries.all })
    },
  })
}
