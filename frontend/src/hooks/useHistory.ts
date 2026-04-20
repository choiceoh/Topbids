import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { RecordChange } from '@/lib/types'

export function useRecordHistory(slug: string | undefined, recordId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.history.record(slug ?? '', recordId ?? ''),
    queryFn: () =>
      api.getList<RecordChange>(`/data/${slug}/${recordId}/history?limit=50`),
    enabled: !!slug && !!recordId,
  })
}
