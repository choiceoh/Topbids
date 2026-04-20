import { useMutation } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { CreateAutomationReq } from '@/lib/types'

export function useAIBuildAutomation(collectionId: string) {
  return useMutation({
    mutationFn: (description: string) =>
      api.post<CreateAutomationReq>(
        `/ai/build-automation/${collectionId}`,
        { description },
      ),
  })
}
