import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { Automation, AutomationRun } from '@/lib/types'

/**
 * Polls automation runs for a collection and shows a toast
 * when a new run is detected.
 */
export function useAutomationRunToasts(collectionId: string | undefined) {
  const lastSeenRef = useRef<Map<string, string>>(new Map())
  const initializedRef = useRef(false)

  const { data: automations } = useQuery({
    queryKey: queryKeys.automations.list(collectionId ?? ''),
    queryFn: () => api.get<Automation[]>(`/schema/collections/${collectionId}/automations`),
    enabled: !!collectionId,
  })

  const enabledAutomations = automations?.filter((a) => a.is_enabled) ?? []
  const firstId = enabledAutomations[0]?.id

  // Poll the first enabled automation's runs as a signal.
  // Most collections have few automations so this is lightweight.
  const { data: runs } = useQuery({
    queryKey: [...queryKeys.automations.runs(firstId ?? ''), 'toast-poll'],
    queryFn: () => api.getList<AutomationRun>(`/schema/automations/${firstId}/runs`),
    enabled: !!firstId,
    refetchInterval: (query) => (query.state.error ? false : 15000),
  })

  useEffect(() => {
    if (!runs?.data?.length || !enabledAutomations.length) return

    // Build automation name map.
    const nameMap = new Map(enabledAutomations.map((a) => [a.id, a.name]))

    for (const run of runs.data.slice(0, 5)) {
      const prevSeen = lastSeenRef.current.get(run.automation_id)

      if (!initializedRef.current) {
        // First load — record current state without toasting.
        lastSeenRef.current.set(run.automation_id, run.id)
        continue
      }

      if (prevSeen === run.id) continue

      // New run detected.
      lastSeenRef.current.set(run.automation_id, run.id)
      const name = nameMap.get(run.automation_id) ?? '자동화'

      if (run.status === 'success') {
        toast.info(`방금 자동화 "${name}"가 실행됨`)
      } else if (run.status === 'error') {
        toast.error(`자동화 "${name}" 실행 실패: ${run.error_message ?? '알 수 없는 오류'}`)
      }
    }

    if (!initializedRef.current) {
      initializedRef.current = true
    }
  }, [runs, enabledAutomations])
}
