import { useCallback } from 'react'
import { toast } from 'sonner'
import { formatError } from '@/lib/api'

/**
 * Shows an error toast with a "재시도" action button.
 * Usage: retryToast(error, () => mutate(...))
 */
export function useRetryToast() {
  const show = useCallback((error: unknown, retryFn: () => void) => {
    toast.error(formatError(error), {
      action: {
        label: '재시도',
        onClick: retryFn,
      },
    })
  }, [])

  return show
}
