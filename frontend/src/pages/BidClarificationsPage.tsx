import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'

import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { api, formatError } from '@/lib/api'
import { formatDateTimeKR } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

// Admin page at /admin/rfq-qa. Lists every clarification thread across the
// whole platform with the rfq expanded, so buyer staff can triage at a
// glance. Answering uses the dedicated /api/bid/clarifications/{id}/answer
// endpoint which also broadcasts notifications to every bidder.

type StatusFilter = 'pending' | 'answered' | 'all'

export default function BidClarificationsPage() {
  const qc = useQueryClient()
  const [filter, setFilter] = useState<StatusFilter>('pending')
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  const query = useQuery({
    queryKey: ['bid', 'clarifications', filter],
    queryFn: () => {
      const params = new URLSearchParams()
      params.set('sort', '-asked_at')
      params.set('limit', '200')
      params.set('expand', 'rfq')
      if (filter !== 'all') {
        params.set(
          '_filter',
          JSON.stringify({
            op: 'and',
            conditions: [{ field: 'status', op: 'eq', value: filter }],
          }),
        )
      }
      return api.getList<EntryRow>(`/data/rfq_clarifications?${params}`)
    },
  })

  const answer = useMutation({
    mutationFn: ({ id, text }: { id: string; text: string }) =>
      api.post<{ id: string; status: string }>(`/bid/clarifications/${id}/answer`, {
        answer: text,
      }),
    onSuccess: (_, vars) => {
      toast.success('답변이 등록되고 입찰자에게 알림이 전송되었습니다')
      setDrafts((prev) => {
        const next = { ...prev }
        delete next[vars.id]
        return next
      })
      qc.invalidateQueries({ queryKey: ['bid', 'clarifications'] })
    },
    onError: (err) => toast.error(formatError(err)),
  })

  return (
    <div>
      <PageHeader
        title="입찰 Q&A 관리"
        description="공급사 질문에 답변하면 해당 공고에 참여한 모든 입찰자에게 전파됩니다"
      />

      <div className="mb-4 flex items-center gap-2">
        {(['pending', 'answered', 'all'] as StatusFilter[]).map((f) => (
          <Button
            key={f}
            variant={filter === f ? 'default' : 'outline'}
            size="sm"
            onClick={() => setFilter(f)}
          >
            {f === 'pending' ? '답변 대기' : f === 'answered' ? '답변 완료' : '전체'}
          </Button>
        ))}
      </div>

      {query.isLoading && <LoadingState />}
      {query.isError && <ErrorState error={query.error} onRetry={() => query.refetch()} />}

      {query.data && query.data.data.length === 0 && (
        <EmptyState
          title={filter === 'pending' ? '답변 대기 중인 질문이 없습니다' : '표시할 질문이 없습니다'}
          description="공급사가 공고 상세에서 질문을 등록하면 이곳에 표시됩니다."
        />
      )}

      {query.data && query.data.data.length > 0 && (
        <ul className="space-y-3">
          {query.data.data.map((c) => {
            const id = String(c.id)
            const rfq = c.rfq as Record<string, unknown> | null | undefined
            const rfqObj = typeof rfq === 'object' && rfq !== null ? rfq : null
            const rfqTitle = rfqObj ? String(rfqObj.title ?? '') : '-'
            const rfqNo = rfqObj ? String(rfqObj.rfq_no ?? '') : '-'
            const status = String(c.status ?? 'pending')
            const draft = drafts[id] ?? (typeof c.answer === 'string' ? c.answer : '')
            return (
              <li key={id} className="rounded-lg border border-border bg-white p-5">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={status === 'answered' ? 'default' : 'secondary'}>
                    {status === 'answered' ? '답변됨' : '대기'}
                  </Badge>
                  <span className="font-mono">{rfqNo}</span>
                  <span className="truncate">{rfqTitle}</span>
                  <span className="ml-auto">{formatDateTimeKR(c.asked_at)}</span>
                </div>
                <p className="whitespace-pre-wrap text-sm text-foreground">
                  <span className="mr-2 font-medium text-muted-foreground">Q.</span>
                  {String(c.question ?? '')}
                </p>
                {status === 'answered' ? (
                  <div className="mt-3 whitespace-pre-wrap border-l-2 border-border pl-3 text-sm text-foreground/90">
                    <span className="mr-2 font-medium text-muted-foreground">A.</span>
                    {String(c.answer ?? '')}
                    <div className="mt-1 text-[11px] text-muted-foreground">
                      {formatDateTimeKR(c.answered_at)}
                    </div>
                  </div>
                ) : (
                  <div className="mt-3 space-y-2">
                    <Textarea
                      rows={3}
                      placeholder="답변을 입력하세요. 등록 시 모든 입찰자에게 알림이 전파됩니다."
                      value={draft}
                      onChange={(e) =>
                        setDrafts((prev) => ({ ...prev, [id]: e.target.value }))
                      }
                    />
                    <div className="flex justify-end">
                      <Button
                        size="sm"
                        disabled={!draft.trim() || answer.isPending}
                        onClick={() => answer.mutate({ id, text: draft })}
                      >
                        {answer.isPending ? '등록 중…' : '답변 등록'}
                      </Button>
                    </div>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
