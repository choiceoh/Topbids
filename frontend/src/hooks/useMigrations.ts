import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { SafetyLevel } from '@/lib/types'

export interface Migration {
  id: string
  collection_id: string
  operation: string
  payload: Record<string, unknown>
  ddl_up: string
  ddl_down: string
  safety_level: SafetyLevel
  created_at: string
  applied_at?: string
  applied_by?: string
  rolled_back_at?: string
}

export function useMigrationHistory(collectionId?: string) {
  const path = collectionId
    ? `/schema/migrations/history?collection_id=${collectionId}`
    : '/schema/migrations/history'

  return useQuery({
    queryKey: queryKeys.migrations.history(collectionId),
    queryFn: () => api.get<Migration[]>(path),
  })
}

export function useRollbackMigration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.post(`/schema/migrations/rollback/${id}`),
    onSuccess: () => {
      // Rolling back affects everything: collections list, fields, dynamic
      // tables. Wipe the relevant caches and let the UI refetch.
      qc.invalidateQueries({ queryKey: queryKeys.collections.all })
      qc.invalidateQueries({ queryKey: queryKeys.entries.all })
      qc.invalidateQueries({ queryKey: queryKeys.migrations.all })
    },
  })
}
