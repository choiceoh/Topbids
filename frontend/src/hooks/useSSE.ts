/**
 * Server-Sent Events hook for real-time collaboration.
 *
 * Connects to GET /api/events (EventSource) and listens for record changes.
 * Key behavior:
 * - Self-event filtering: skips invalidation when actor_user_id === me.id
 *   (optimistic updates already applied, refetching would cause flicker)
 * - Targeted invalidation: only invalidates queries for the affected collection
 * - Exponential backoff reconnection (up to 30s) on connection loss
 * - Toast notifications for other users' changes
 */

import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { queryKeys } from '@/lib/queryKeys'
import { useCurrentUser } from '@/hooks/useAuth'
import { toast } from 'sonner'

interface SSEMessage {
  type: string
  collection_id: string
  collection_slug?: string
  record_id?: string
  actor_user_id?: string
  actor_name?: string
}

const EVENT_LABELS: Record<string, string> = {
  record_created: '새 레코드를 추가했습니다',
  record_updated: '레코드를 수정했습니다',
  record_deleted: '레코드를 삭제했습니다',
  comment: '댓글을 남겼습니다',
}

/**
 * Subscribe to real-time events and keep React Query caches in sync.
 *
 * Event types handled:
 * - `record_created` / `record_updated` / `record_deleted` — invalidate
 *   the affected collection's entry queries (skipped for self-events)
 * - `comment` — additionally invalidate the specific record's comment cache
 *
 * Notifications are always invalidated (even for self-events) because
 * server-side automations may generate notifications as side effects.
 *
 * Reconnection uses exponential backoff: 1s -> 2s -> 4s -> ... -> 30s max.
 * The delay resets to 1s on successful connection.
 */
export function useSSE() {
  const qc = useQueryClient()
  const { data: me } = useCurrentUser()
  const meRef = useRef(me)
  useEffect(() => { meRef.current = me }, [me])

  const retryDelay = useRef(1000)

  useEffect(() => {
    let es: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | null = null
    let unmounted = false

    function connect() {
      if (unmounted) return
      es = new EventSource('/api/events', { withCredentials: true })

      es.addEventListener('connected', () => {
        retryDelay.current = 1000
      })

      es.addEventListener('message', (e) => {
        let msg: SSEMessage
        try {
          msg = JSON.parse(e.data)
        } catch (parseErr) {
          if (import.meta.env.DEV) {
            console.warn('SSE: failed to parse message', e.data, parseErr)
          }
          return
        }

        const isSelf = meRef.current?.id === msg.actor_user_id

        // Always refresh notifications (even for self — server may
        // generate notifications from automations).
        qc.invalidateQueries({ queryKey: queryKeys.notifications.all })

        // Skip data refetch for own actions — optimistic updates already applied.
        if (isSelf) return

        const slug = msg.collection_slug

        if (slug) {
          // Targeted invalidation: only the affected collection's entries.
          qc.invalidateQueries({
            queryKey: [...queryKeys.entries.all, slug],
          })
        }

        if (msg.type === 'comment' && slug && msg.record_id) {
          qc.invalidateQueries({
            queryKey: queryKeys.comments.list(slug, msg.record_id),
          })
        }

        // Toast for other users' changes.
        const label = EVENT_LABELS[msg.type]
        if (label && msg.actor_name) {
          toast(`${msg.actor_name}님이 ${label}`, {
            duration: 3000,
          })
        }
      })

      es.onerror = () => {
        es?.close()
        if (unmounted) return
        // Exponential backoff up to 30s
        timer = setTimeout(connect, retryDelay.current)
        retryDelay.current = Math.min(retryDelay.current * 2, 30000)
      }
    }

    connect()

    return () => {
      unmounted = true
      es?.close()
      if (timer) clearTimeout(timer)
    }
  }, [qc])
}
