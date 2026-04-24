import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCurrentUser } from '@/hooks/useAuth'
import { api, formatError } from '@/lib/api'
import { formatDateTimeKR, formatDeadlineRelative, formatKRW } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

interface AuctionBidRow {
  id: string
  supplier_name: string
  total_amount: number
  submitted_at: string
  is_mine?: boolean
}

/**
 * Supplier-facing live view for mode='auction' RFQs.
 *
 * Polls /api/bid/rfqs/{id}/auction-bids every 3 seconds to keep the leaderboard
 * fresh without wiring up SSE. Competitor company names are server-side
 * anonymised to "공급사 #N" — suppliers see the price ranking but not who
 * they're competing with, which prevents collusion while preserving the
 * competitive pressure auction depends on.
 *
 * Submission form submits via the standard /data/bids POST/PATCH. The write
 * guard enforces the undercut rule (new < current min), so the server
 * rejects non-improving bids with a 409.
 */
export default function PortalAuctionPage() {
  const { rfqId } = useParams<{ rfqId: string }>()
  const { data: user } = useCurrentUser()
  const navigate = useNavigate()
  const qc = useQueryClient()

  const rfqQuery = useQuery({
    queryKey: ['portal', 'auction-rfq', rfqId],
    queryFn: () => api.get<EntryRow>(`/data/rfqs/${rfqId}`),
    enabled: !!rfqId,
  })

  const leaderboardQuery = useQuery({
    queryKey: ['portal', 'auction-bids', rfqId],
    queryFn: () => api.get<{ data: AuctionBidRow[] }>(`/bid/rfqs/${rfqId}/auction-bids`),
    enabled: !!rfqId,
    // 3s polling — short enough to feel real-time, long enough to avoid
    // hammering the DB for a screen that 5-20 suppliers watch at once.
    refetchInterval: 3000,
  })

  const existingBidQuery = useQuery({
    queryKey: ['portal', 'my-bid', rfqId],
    queryFn: () =>
      api.getList<EntryRow>(
        `/data/bids?limit=1&_filter=` +
          encodeURIComponent(
            JSON.stringify({
              op: 'and',
              conditions: [{ field: 'rfq', op: 'eq', value: rfqId }],
            }),
          ),
      ),
    enabled: !!rfqId,
  })
  const existing = existingBidQuery.data?.data[0]

  const [nextAmount, setNextAmount] = useState('')

  const submit = useMutation({
    mutationFn: async () => {
      if (!user?.supplier_id) throw new Error('공급사 정보가 연결되어 있지 않습니다.')
      const n = Number(nextAmount)
      if (!Number.isFinite(n) || n <= 0) throw new Error('유효한 금액을 입력하세요.')
      const payload = {
        rfq: rfqId,
        supplier: user.supplier_id,
        total_amount: n,
        status: 'submitted',
        submitted_at: new Date().toISOString(),
      }
      if (existing?.id) {
        return api.patch<EntryRow>(`/data/bids/${String(existing.id)}`, payload)
      }
      return api.post<EntryRow>('/data/bids', payload)
    },
    onSuccess: () => {
      toast.success('입찰가가 갱신되었습니다')
      setNextAmount('')
      qc.invalidateQueries({ queryKey: ['portal', 'auction-bids', rfqId] })
      qc.invalidateQueries({ queryKey: ['portal', 'my-bid', rfqId] })
    },
    onError: (err) => toast.error(formatError(err)),
  })

  if (rfqQuery.isLoading) return <LoadingState />
  if (rfqQuery.isError)
    return <ErrorState error={rfqQuery.error} onRetry={() => rfqQuery.refetch()} />
  if (!rfqQuery.data) return null

  const rfq = rfqQuery.data
  if (rfq.mode !== 'auction') {
    // Wrong RFQ type for this page — bounce to the normal submit flow.
    return (
      <Card className="p-6 text-sm">
        이 공고는 역경매 모드가 아닙니다.
        <Button variant="link" onClick={() => navigate(`/portal/rfqs/${rfqId}/bid`)}>
          일반 입찰서 제출 페이지로 이동
        </Button>
      </Card>
    )
  }

  const deadlinePassed = rfq.deadline_at
    ? new Date(String(rfq.deadline_at)).getTime() <= Date.now()
    : false
  const editable = rfq.status === 'published' && !deadlinePassed

  const leaderboard = leaderboardQuery.data?.data ?? []
  const currentBest = leaderboard[0]?.total_amount
  const myEntry = leaderboard.find((b) => b.is_mine)

  return (
    <div>
      <PageHeader
        title="실시간 역경매"
        description="경쟁 입찰가가 3초마다 갱신됩니다. 새 입찰은 현재 최저가보다 낮아야 합니다"
        breadcrumb={[
          { label: '공고', href: '/portal' },
          { label: String(rfq.rfq_no ?? '') },
        ]}
      />

      <Card className="mb-6 p-5">
        <div className="flex items-start justify-between gap-4">
          <h2 className="text-base font-semibold">{String(rfq.title ?? '')}</h2>
          <span className={`text-xs font-medium ${editable ? 'text-foreground' : 'text-amber-600'}`}>
            {formatDeadlineRelative(rfq.deadline_at)}
          </span>
        </div>
        <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1 text-xs text-muted-foreground md:grid-cols-4">
          <Info label="공고번호" value={String(rfq.rfq_no ?? '-')} />
          <Info label="입찰마감" value={formatDateTimeKR(rfq.deadline_at)} />
          <Info label="현재 최저가" value={currentBest != null ? formatKRW(currentBest) : '—'} />
          <Info label="내 현재 입찰가" value={myEntry ? formatKRW(myEntry.total_amount) : '미제출'} />
        </dl>
      </Card>

      <Card className="mb-6 p-5">
        <h3 className="mb-3 text-sm font-semibold">리더보드</h3>
        {leaderboard.length === 0 ? (
          <p className="text-xs text-muted-foreground">아직 제출된 입찰가가 없습니다.</p>
        ) : (
          <ol className="space-y-2">
            {leaderboard.map((b, i) => (
              <li
                key={b.id}
                className={`flex items-center justify-between rounded-md border px-3 py-2 text-sm ${
                  b.is_mine
                    ? 'border-foreground bg-foreground/5'
                    : 'border-border bg-white'
                }`}
              >
                <div className="flex items-center gap-2">
                  <Badge variant={i === 0 ? 'default' : 'outline'}>
                    {i === 0 ? '1위' : `${i + 1}위`}
                  </Badge>
                  <span className="font-medium">{b.supplier_name}</span>
                  {b.is_mine && <Badge variant="secondary">내 입찰</Badge>}
                </div>
                <div className="flex items-center gap-3 text-xs">
                  <span className="font-medium text-foreground">{formatKRW(b.total_amount)}</span>
                  <span className="text-muted-foreground">
                    {formatDateTimeKR(b.submitted_at)}
                  </span>
                </div>
              </li>
            ))}
          </ol>
        )}
      </Card>

      {editable && (
        <Card className="p-5">
          <h3 className="mb-3 text-sm font-semibold">
            {existing ? '입찰가 갱신' : '첫 입찰가 제출'}
          </h3>
          <p className="mb-3 text-xs text-muted-foreground">
            현재 최저가:{' '}
            <span className="font-medium text-foreground">
              {currentBest != null ? formatKRW(currentBest) : '아직 없음'}
            </span>
            {currentBest != null && ' — 이보다 낮은 금액만 제출할 수 있습니다.'}
          </p>
          <div className="flex gap-3 md:max-w-md">
            <div className="flex-1">
              <Label className="text-xs">새 입찰금액 (원)</Label>
              <Input
                type="number"
                inputMode="numeric"
                value={nextAmount}
                onChange={(e) => setNextAmount(e.target.value)}
                placeholder={currentBest != null ? String(currentBest - 10_000) : '금액 입력'}
              />
            </div>
            <div className="self-end">
              <Button
                disabled={submit.isPending || !nextAmount}
                onClick={() => submit.mutate()}
              >
                {submit.isPending ? '전송 중…' : '입찰'}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {!editable && (
        <Card className="border-amber-500/40 bg-amber-50/40 p-4 text-sm">
          역경매가 마감되었습니다{deadlinePassed ? ' (마감 시간 경과)' : ''}. 관리자가 개찰·낙찰을 처리하면 결과가 입찰 내역에 반영됩니다.
        </Card>
      )}
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground/70">{label}</dt>
      <dd className="text-foreground">{value}</dd>
    </div>
  )
}
