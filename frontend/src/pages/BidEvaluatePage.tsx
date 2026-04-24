import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useParams } from 'react-router'
import { toast } from 'sonner'

import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useCurrentUser } from '@/hooks/useAuth'
import { api, formatError } from '@/lib/api'
import { formatKRW } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

/**
 * Evaluator entry page at /admin/rfqs/{rfqId}/evaluate.
 *
 * Each buyer staffer (director/pm/engineer) enters their own tech_score and
 * optional comment per bid. Upsert by (bid, evaluator) — if a row already
 * exists for this user we PATCH, otherwise POST. AwardRFQ averages all
 * evaluators at award time, so multiple scorers get blended automatically.
 *
 * Intentionally shows only the row this user owns, not other evaluators'
 * scores — blind review prevents anchoring.
 */
export default function BidEvaluatePage() {
  const { rfqId } = useParams<{ rfqId: string }>()
  const { data: user } = useCurrentUser()
  const qc = useQueryClient()

  const rfqQuery = useQuery({
    queryKey: ['admin', 'rfq', rfqId],
    queryFn: () => api.get<EntryRow>(`/data/rfqs/${rfqId}`),
    enabled: !!rfqId,
  })

  const bidsQuery = useQuery({
    queryKey: ['admin', 'rfq', rfqId, 'bids'],
    queryFn: () =>
      api.getList<EntryRow>(
        `/data/bids?sort=total_amount&limit=200&expand=supplier&_filter=` +
          encodeURIComponent(
            JSON.stringify({
              op: 'and',
              conditions: [
                { field: 'rfq', op: 'eq', value: rfqId },
                {
                  op: 'or',
                  conditions: [
                    { field: 'status', op: 'eq', value: 'submitted' },
                    { field: 'status', op: 'eq', value: 'opened' },
                    { field: 'status', op: 'eq', value: 'evaluated' },
                  ],
                },
              ],
            }),
          ),
      ),
    enabled: !!rfqId,
  })

  // Load my own evaluation rows for all bids on this RFQ — one per bid if I
  // scored it earlier. The bid_evaluations collection has a supplier-free
  // access_config (supplier role can't read), so this is buyer-side only.
  const myEvalsQuery = useQuery({
    queryKey: ['admin', 'my-evals', rfqId, user?.id],
    queryFn: () =>
      api.getList<EntryRow>(
        `/data/bid_evaluations?limit=500&_filter=` +
          encodeURIComponent(
            JSON.stringify({
              op: 'and',
              conditions: [{ field: 'evaluator', op: 'eq', value: user?.id ?? '' }],
            }),
          ),
      ),
    enabled: !!rfqId && !!user?.id,
  })

  const byBid = new Map<string, EntryRow>()
  for (const e of myEvalsQuery.data?.data ?? []) {
    byBid.set(String(e.bid), e)
  }

  const upsert = useMutation({
    mutationFn: async (vars: { bidID: string; techScore: number; comment: string }) => {
      const existing = byBid.get(vars.bidID)
      const payload = {
        bid: vars.bidID,
        evaluator: user?.id,
        tech_score: vars.techScore,
        comment: vars.comment,
        scored_at: new Date().toISOString(),
      }
      if (existing) {
        return api.patch<EntryRow>(`/data/bid_evaluations/${String(existing.id)}`, payload)
      }
      return api.post<EntryRow>('/data/bid_evaluations', payload)
    },
    onSuccess: () => {
      toast.success('평가 점수가 저장되었습니다')
      qc.invalidateQueries({ queryKey: ['admin', 'my-evals'] })
    },
    onError: (err) => toast.error(formatError(err)),
  })

  if (!rfqId) return null
  if (rfqQuery.isLoading || bidsQuery.isLoading) return <LoadingState />
  if (rfqQuery.isError)
    return <ErrorState error={rfqQuery.error} onRetry={() => rfqQuery.refetch()} />

  const rfq = rfqQuery.data
  const bids = bidsQuery.data?.data ?? []

  return (
    <div>
      <PageHeader
        title="입찰 기술 평가"
        description="본인 점수만 표시됩니다 (블라인드). 낙찰 시 모든 평가자의 평균이 사용됩니다"
        breadcrumb={[
          { label: '앱', href: '/apps' },
          { label: '입찰', href: `/apps/rfqs` },
          { label: String(rfq?.rfq_no ?? rfqId) },
        ]}
      />

      {bids.length === 0 ? (
        <EmptyState
          title="평가 대상 입찰서가 없습니다"
          description="공급사가 제출 완료한 입찰서가 이곳에 나타납니다."
        />
      ) : (
        <ul className="space-y-3">
          {bids.map((bid) => (
            <EvaluateRow
              key={String(bid.id)}
              bid={bid}
              existing={byBid.get(String(bid.id))}
              onSave={(techScore, comment) =>
                upsert.mutate({ bidID: String(bid.id), techScore, comment })
              }
              pending={upsert.isPending}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function EvaluateRow({
  bid,
  existing,
  onSave,
  pending,
}: {
  bid: EntryRow
  existing?: EntryRow
  onSave: (techScore: number, comment: string) => void
  pending: boolean
}) {
  const [score, setScore] = useState<string>(
    existing?.tech_score != null ? String(existing.tech_score) : '',
  )
  const [comment, setComment] = useState<string>(
    typeof existing?.comment === 'string' ? existing.comment : '',
  )
  const supplier = bid.supplier as Record<string, unknown> | null | undefined
  const supplierObj = typeof supplier === 'object' && supplier !== null ? supplier : null
  const supplierName = supplierObj ? String(supplierObj.name ?? '—') : '—'

  const parsed = Number(score)
  const invalid = score !== '' && (!Number.isFinite(parsed) || parsed < 0 || parsed > 100)

  return (
    <li className="rounded-lg border border-border bg-white p-5">
      <div className="mb-3 flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-foreground">{supplierName}</p>
          <p className="text-xs text-muted-foreground">
            입찰금액 {formatKRW(bid.total_amount)}
            {bid.lead_time != null ? ` · 납기 ${String(bid.lead_time)}일` : ''}
          </p>
        </div>
        {existing && (
          <span className="text-[11px] text-muted-foreground">
            최근 저장: {String(existing.scored_at ?? '-').slice(0, 16)}
          </span>
        )}
      </div>
      <div className="grid gap-3 md:grid-cols-[120px_1fr_auto] md:items-start">
        <div>
          <Label className="text-xs">기술점수 (0-100)</Label>
          <Input
            type="number"
            inputMode="numeric"
            value={score}
            onChange={(e) => setScore(e.target.value)}
            aria-invalid={invalid || undefined}
          />
        </div>
        <div>
          <Label className="text-xs">평가 의견 (선택)</Label>
          <Textarea
            rows={2}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="예: 기술 제안서는 충실하나 납기 일정이 다소 빠듯함"
          />
        </div>
        <div className="self-end">
          <Button
            disabled={invalid || score === '' || pending}
            onClick={() => onSave(parsed, comment)}
          >
            {existing ? '수정' : '저장'}
          </Button>
        </div>
      </div>
    </li>
  )
}
