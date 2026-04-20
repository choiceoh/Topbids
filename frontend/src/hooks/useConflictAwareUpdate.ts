import { useCallback } from 'react'
import { toast } from 'sonner'
import { ApiError, formatError } from '@/lib/api'

/**
 * Returns an onError handler that auto-refetches on 409 conflicts.
 * For non-conflict errors it calls `fallback` if provided, otherwise shows a generic toast.
 *
 * Usage:
 *   const onConflictError = useConflictAwareUpdate(refetch)
 *   updateEntry.mutate(payload, { onError: onConflictError })
 *   updateEntry.mutate(payload, { onError: (err) => onConflictError(err, () => retryToast(err, retry)) })
 */
export function useConflictAwareUpdate(refetch: () => void) {
  return useCallback(
    (err: Error, fallback?: (err: Error) => void) => {
      if (err instanceof ApiError && err.isConflict()) {
        toast.error('다른 사용자가 이미 수정했습니다. 최신 데이터를 불러옵니다.')
        refetch()
      } else if (fallback) {
        fallback(err)
      } else {
        toast.error(formatError(err))
      }
    },
    [refetch],
  )
}
