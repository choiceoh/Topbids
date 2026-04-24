import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { api, formatError } from '@/lib/api'
import { formatDateTimeKR, formatKRW } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

/**
 * Admin RFQ lifecycle dashboard at /admin/rfqs/{rfqId}/manage. Single page
 * that shows current status and offers every valid transition as a button:
 *
 *   draft         → publish (stamps reserves if 복수예가)
 *   published     → amend (notify bidders) | cancel
 *   closed/opened → evaluate (→ 적격심사) | award | cancel
 *   evaluating    → award | cancel
 *   any           → clone (spawn a fresh draft)
 *
 * Buttons only render when the backend will accept them — reduces accidental
 * clicks that 400. The underlying bid.* endpoints are the source of truth;
 * UI hints are advisory.
 */
export default function BidManagePage() {
  const { rfqId } = useParams<{ rfqId: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()

  const q = useQuery({
    queryKey: ['admin', 'rfq-manage', rfqId],
    queryFn: () => api.get<EntryRow>(`/data/rfqs/${rfqId}`),
    enabled: !!rfqId,
  })

  // Keep the status/count snapshot local so buttons can re-fetch after each action.
  const [amendNote, setAmendNote] = useState('')
  const [amendOpen, setAmendOpen] = useState(false)
  const [confirmKind, setConfirmKind] = useState<'cancel' | 'publish' | 'award' | null>(null)

  const call = useMutation({
    mutationFn: async (vars: { action: string; body?: unknown }) => {
      return api.post(`/bid/rfqs/${rfqId}/${vars.action}`, vars.body ?? {})
    },
    onSuccess: (_, vars) => {
      const msg: Record<string, string> = {
        publish: '공고가 게시되고 공급사에 알림이 전파됩니다',
        amend: '변경공고가 등록되었습니다',
        clone: 'RFQ가 복제되었습니다',
        cancel: 'RFQ가 취소되었습니다',
        evaluate: '적격심사 단계로 전환되었습니다',
        award: '낙찰이 확정되고 PO가 자동 분배됩니다',
      }
      toast.success(msg[vars.action] ?? '처리되었습니다')
      qc.invalidateQueries({ queryKey: ['admin', 'rfq-manage'] })
    },
    onError: (err) => toast.error(formatError(err)),
  })

  const cloneCall = useMutation({
    mutationFn: () => api.post<{ rfq_id: string }>(`/bid/rfqs/${rfqId}/clone`, {}),
    onSuccess: (data) => {
      toast.success('RFQ가 복제되었습니다. 새 draft로 이동합니다.')
      navigate(`/admin/rfqs/${data.rfq_id}/manage`)
    },
    onError: (err) => toast.error(formatError(err)),
  })

  if (!rfqId) return null
  if (q.isLoading) return <LoadingState />
  if (q.isError) return <ErrorState error={q.error} onRetry={() => q.refetch()} />
  if (!q.data) return null

  const rfq = q.data
  const status = String(rfq.status ?? 'draft')
  const isDraft = status === 'draft'
  const isLive = ['published', 'closed', 'opened', 'evaluating'].includes(status)
  const canAward = ['opened', 'evaluating'].includes(status)
  const canEvaluate = status === 'opened'
  const isTerminal = ['awarded', 'cancelled', 'failed'].includes(status)

  return (
    <div>
      <PageHeader
        title={String(rfq.title ?? rfq.rfq_no ?? 'RFQ 관리')}
        description="공고의 현재 상태에 따라 사용 가능한 작업만 활성화됩니다"
        breadcrumb={[
          { label: '앱', href: '/apps' },
          { label: String(rfq.rfq_no ?? '') },
        ]}
      />

      <Card className="mb-6 p-5">
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={isTerminal ? 'secondary' : 'default'}>{status}</Badge>
          {typeof rfq.rfx_type === 'string' && (
            <Badge variant="outline">{String(rfq.rfx_type).toUpperCase()}</Badge>
          )}
          {typeof rfq.mode === 'string' && <Badge variant="outline">{String(rfq.mode)}</Badge>}
          {rfq.is_template === true && <Badge variant="outline">템플릿</Badge>}
          {typeof rfq.amendment_count === 'number' && rfq.amendment_count > 0 && (
            <Badge variant="outline" className="border-amber-500 text-amber-700">
              변경 {rfq.amendment_count}회
            </Badge>
          )}
        </div>

        <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-xs md:grid-cols-4">
          <Info label="공고번호" value={String(rfq.rfq_no ?? '-')} />
          <Info label="카테고리" value={String(rfq.category ?? '-')} />
          <Info label="평가방식" value={rfq.eval_method === 'weighted' ? '종합평가' : '최저가'}/>
          <Info label="예가방식" value={String(rfq.reserve_method ?? 'single')} />
          <Info label="기초금액" value={formatKRW(rfq.base_amount)} />
          <Info label="추정가" value={formatKRW(rfq.estimated_price)} />
          <Info label="예정가" value={formatKRW(rfq.planned_price)} />
          <Info label="낙찰하한율" value={rfq.min_win_rate != null ? `${(Number(rfq.min_win_rate) * 100).toFixed(0)}%` : '-'} />
          <Info label="입찰마감" value={formatDateTimeKR(rfq.deadline_at)} />
          <Info label="개찰일시" value={formatDateTimeKR(rfq.open_at)} />
          <Info label="공고일시" value={formatDateTimeKR(rfq.published_at)} />
          <Info label="최종 변경" value={formatDateTimeKR(rfq.last_amended_at)} />
        </dl>
      </Card>

      <Card className="mb-6 p-5">
        <h3 className="mb-3 text-sm font-semibold">작업</h3>
        <div className="flex flex-wrap gap-2">
          {isDraft && (
            <Button onClick={() => setConfirmKind('publish')} disabled={call.isPending}>
              공고 게시
            </Button>
          )}
          {isLive && !isTerminal && (
            <Button variant="outline" onClick={() => setAmendOpen(true)}>
              변경공고 등록
            </Button>
          )}
          {canEvaluate && (
            <Button
              variant="outline"
              onClick={() => call.mutate({ action: 'evaluate' })}
              disabled={call.isPending}
            >
              적격심사 시작
            </Button>
          )}
          {canAward && (
            <Button
              onClick={() => setConfirmKind('award')}
              disabled={call.isPending}
            >
              낙찰 확정
            </Button>
          )}
          {isLive && !isTerminal && (
            <Button
              variant="destructive"
              onClick={() => setConfirmKind('cancel')}
              disabled={call.isPending}
            >
              RFQ 취소
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={() => cloneCall.mutate()}
            disabled={cloneCall.isPending}
          >
            RFQ 복제
          </Button>
          <Button variant="ghost" onClick={() => navigate(`/admin/rfqs/${rfqId}/evaluate`)}>
            평가 점수 입력
          </Button>
        </div>

        {rfq.mode === 'auction' && isLive && (
          <div className="mt-4 border-t border-border pt-4">
            <p className="mb-2 text-xs text-muted-foreground">
              역경매 모드입니다. 실시간 리더보드는 포털 측 /portal/rfqs/{rfqId}/auction에서 확인할 수 있습니다.
            </p>
          </div>
        )}
      </Card>

      {/* Amend dialog — uses body instead of the generic ConfirmDialog because
          it needs a textarea for the note */}
      {amendOpen && (
        <Card className="mb-6 p-5">
          <h3 className="mb-2 text-sm font-semibold">변경공고 등록</h3>
          <Label className="text-xs text-muted-foreground">변경 내용 (공급사에게 전파됩니다)</Label>
          <Textarea
            rows={4}
            value={amendNote}
            onChange={(e) => setAmendNote(e.target.value)}
            placeholder="예: 납품 사양 변경 (도면 v2 첨부) · 마감 연장"
          />
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setAmendOpen(false)}>
              취소
            </Button>
            <Button
              disabled={!amendNote.trim() || call.isPending}
              onClick={() =>
                call.mutate(
                  { action: 'amend', body: { note: amendNote } },
                  {
                    onSuccess: () => {
                      setAmendOpen(false)
                      setAmendNote('')
                    },
                  },
                )
              }
            >
              등록 + 알림 발송
            </Button>
          </div>
        </Card>
      )}

      <ConfirmDialog
        open={confirmKind !== null}
        onOpenChange={(open) => !open && setConfirmKind(null)}
        title={
          confirmKind === 'publish'
            ? '공고를 게시할까요?'
            : confirmKind === 'award'
              ? '낙찰을 확정할까요?'
              : 'RFQ를 취소할까요?'
        }
        description={
          confirmKind === 'publish'
            ? '게시 후에는 draft로 되돌릴 수 없습니다. 복수예가 방식이라면 15개 예비가격이 자동 생성됩니다.'
            : confirmKind === 'award'
              ? '낙찰자가 확정되고 PO가 계열사별로 자동 분배됩니다. 낙찰 이후에는 되돌릴 수 없습니다.'
              : '진행 중인 입찰서는 탈락 처리되고 공급사에게 알림이 전달됩니다.'
        }
        variant={confirmKind === 'cancel' ? 'destructive' : 'default'}
        loading={call.isPending}
        onConfirm={() => {
          if (!confirmKind) return
          const body = confirmKind === 'cancel' ? { reason: '관리자 요청' } : {}
          call.mutate(
            { action: confirmKind, body },
            { onSuccess: () => setConfirmKind(null) },
          )
        }}
      />
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
