import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { z } from 'zod'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import { FormField } from '@/components/common/Form'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useCurrentUser } from '@/hooks/useAuth'
import { api, formatError } from '@/lib/api'
import { formatDateTimeKR, formatDeadlineRelative, formatKRW } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

// RHF registers <input type="number"> with valueAsNumber:true so the form
// state holds real numbers — no coerce needed here.
//
// reserve_picks is managed as separate checkbox state rather than via RHF
// because the rule "exactly 2 distinct indices" is easier to enforce in
// imperative code than through zod array constraints.
const schema = z.object({
  total_amount: z.number().positive('0보다 커야 합니다'),
  lead_time: z.number().int().nonnegative('0 이상이어야 합니다').optional(),
  valid_days: z.number().int().positive('1 이상이어야 합니다').optional(),
  note: z.string().max(1000).optional(),
})

type BidForm = z.infer<typeof schema>


/**
 * Bid submission form for a single RFQ. Handles both create (no existing bid)
 * and update (supplier edits their own still-submittable bid).
 *
 * Key invariants:
 * - `supplier` is always forced to the current user's supplier_id
 * - `rfq` is always forced to the URL param
 * - `status` is set to 'submitted' on save
 * - Once the RFQ moves past 'published', the form is read-only — the
 *   scheduler/admin controls lifecycle from there.
 */
export default function PortalBidSubmitPage() {
  const { rfqId } = useParams<{ rfqId: string }>()
  const { data: user } = useCurrentUser()
  const navigate = useNavigate()
  const qc = useQueryClient()

  const rfqQuery = useQuery({
    queryKey: ['portal', 'rfq', rfqId],
    queryFn: () => api.get<EntryRow>(`/data/rfqs/${rfqId}`),
    enabled: !!rfqId,
  })

  // Existing bid for this RFQ (if the supplier already submitted).
  // SupplierRowFilter on the backend restricts this list to the caller's
  // own rows — we only need to filter by rfq here.
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

  // Q&A thread against this RFQ. Everyone authenticated can read (transparency)
  // — suppliers see their own pending questions plus all answered ones.
  // New questions from this supplier are appended below.
  const clarificationsQuery = useQuery({
    queryKey: ['portal', 'rfq-qa', rfqId],
    queryFn: () =>
      api.getList<EntryRow>(
        `/data/rfq_clarifications?sort=-asked_at&limit=100&_filter=` +
          encodeURIComponent(
            JSON.stringify({
              op: 'and',
              conditions: [{ field: 'rfq', op: 'eq', value: rfqId }],
            }),
          ),
      ),
    enabled: !!rfqId,
  })

  const [question, setQuestion] = useState('')
  const askQuestion = useMutation({
    mutationFn: (body: string) =>
      api.post<EntryRow>('/data/rfq_clarifications', {
        rfq: rfqId,
        question: body,
        asked_at: new Date().toISOString(),
        status: 'pending',
      }),
    onSuccess: () => {
      toast.success('질문이 등록되었습니다. 답변이 달리면 이곳에 표시됩니다.')
      setQuestion('')
      qc.invalidateQueries({ queryKey: ['portal', 'rfq-qa', rfqId] })
    },
    onError: (err) => toast.error(formatError(err)),
  })

  const form = useForm<BidForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      total_amount: 0,
      lead_time: undefined,
      valid_days: 30,
      note: '',
    },
  })

  // reserve_picks state for multiple-reserve-price RFQs. Rule: exactly 2
  // distinct indices in [0, reserves.length). The form submit path refuses
  // unless this constraint is met.
  const [reservePicks, setReservePicks] = useState<number[]>([])
  useEffect(() => {
    const raw = existing?.reserve_picks
    if (Array.isArray(raw)) {
      setReservePicks(raw.filter((x): x is number => typeof x === 'number'))
    }
  }, [existing])

  // Hydrate the form when existing bid data arrives.
  useEffect(() => {
    if (existing) {
      form.reset({
        total_amount: Number(existing.total_amount ?? 0),
        lead_time:
          existing.lead_time === null || existing.lead_time === undefined
            ? undefined
            : Number(existing.lead_time),
        valid_days:
          existing.valid_days === null || existing.valid_days === undefined
            ? 30
            : Number(existing.valid_days),
        note: typeof existing.note === 'string' ? existing.note : '',
      })
    }
  }, [existing, form])

  const mutation = useMutation({
    mutationFn: async (values: BidForm) => {
      if (!user?.supplier_id) {
        throw new Error('공급사 정보가 연결되어 있지 않습니다. 관리자에게 문의하세요.')
      }
      const payload: Record<string, unknown> = {
        ...values,
        rfq: rfqId,
        supplier: user.supplier_id,
        status: 'submitted',
        submitted_at: new Date().toISOString(),
      }
      if (rfqQuery.data?.reserve_method === 'multiple') {
        if (reservePicks.length !== 2) {
          throw new Error('복수예가 방식이므로 예비가격 2개를 선택해야 합니다.')
        }
        payload.reserve_picks = reservePicks
      }
      if (existing?.id) {
        return api.patch<EntryRow>(`/data/bids/${String(existing.id)}`, payload)
      }
      return api.post<EntryRow>('/data/bids', payload)
    },
    onSuccess: () => {
      toast.success('입찰서가 제출되었습니다')
      qc.invalidateQueries({ queryKey: ['portal', 'my-bid', rfqId] })
      qc.invalidateQueries({ queryKey: ['portal', 'bids'] })
      navigate('/portal/history')
    },
    onError: (err) => toast.error(formatError(err)),
  })

  // Confirmation state — first click on "제출" opens the dialog so an
  // accidental enter-key or double-click can't submit a bid unsupervised.
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [pendingValues, setPendingValues] = useState<BidForm | null>(null)

  if (rfqQuery.isLoading) return <LoadingState />
  if (rfqQuery.isError)
    return <ErrorState error={rfqQuery.error} onRetry={() => rfqQuery.refetch()} />
  if (!rfqQuery.data) return null

  const rfq = rfqQuery.data
  const rfqStatus = String(rfq.status ?? '')
  const deadlinePassed = rfq.deadline_at
    ? new Date(String(rfq.deadline_at)).getTime() <= Date.now()
    : false
  const editable = rfqStatus === 'published' && !deadlinePassed

  return (
    <div>
      <PageHeader
        title="입찰서 제출"
        description="금액과 납기는 개찰 전까지 본인과 관리자 외에는 열람할 수 없습니다"
        breadcrumb={[
          { label: '공고', href: '/portal' },
          { label: String(rfq.rfq_no ?? '') || '상세' },
        ]}
      />

      <Card className="mb-6 p-5">
        <div className="flex items-start justify-between gap-4">
          <h2 className="text-base font-semibold text-foreground">
            {String(rfq.title ?? '')}
          </h2>
          <span
            className={`shrink-0 text-xs font-medium ${
              deadlinePassed || !editable ? 'text-amber-600' : 'text-foreground'
            }`}
          >
            {formatDeadlineRelative(rfq.deadline_at)}
          </span>
        </div>
        <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-xs text-muted-foreground md:grid-cols-4">
          <div>
            <dt className="text-muted-foreground/70">공고번호</dt>
            <dd className="text-foreground">{String(rfq.rfq_no ?? '-')}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground/70">카테고리</dt>
            <dd className="text-foreground">{String(rfq.category ?? '-')}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground/70">입찰마감</dt>
            <dd className="text-foreground">{formatDateTimeKR(rfq.deadline_at)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground/70">개찰일시</dt>
            <dd className="text-foreground">{formatDateTimeKR(rfq.open_at)}</dd>
          </div>
        </dl>
      </Card>

      {!editable && (
        <Card className="mb-6 border-amber-500/40 bg-amber-50/40 p-4 text-sm">
          이 공고는 현재 입찰을 받지 않습니다
          {deadlinePassed ? ' (마감 시간 경과)' : ` (상태: ${rfqStatus})`}.
          이미 제출한 내용은 참고용으로만 표시됩니다.
        </Card>
      )}

      {typeof rfq.amendment_count === 'number' && rfq.amendment_count > 0 && (
        <Card className="mb-6 border-amber-500/40 bg-amber-50/40 p-4 text-sm">
          <div className="mb-1 font-medium text-amber-800">
            공고가 {rfq.amendment_count}회 변경되었습니다
            {typeof rfq.last_amended_at === 'string' &&
              ` (최근 ${formatDateTimeKR(rfq.last_amended_at)})`}
          </div>
          {typeof rfq.amendment_note === 'string' && rfq.amendment_note && (
            <div className="whitespace-pre-wrap text-foreground/90">{rfq.amendment_note}</div>
          )}
        </Card>
      )}

      <AttachmentsBlock value={rfq.attachments} />

      {typeof rfq.description === 'string' && rfq.description && (
        <Card className="mb-6 p-5 text-sm leading-relaxed whitespace-pre-wrap">
          {rfq.description}
        </Card>
      )}

      <FormProvider {...form}>
        <form
          onSubmit={form.handleSubmit((v) => {
            setPendingValues(v)
            setConfirmOpen(true)
          })}
          className="space-y-5 rounded-lg border border-border bg-white p-6"
        >
          <div className="grid gap-5 md:grid-cols-2">
            <FormField<BidForm> name="total_amount" label="입찰금액 (원)" required>
              <Input
                type="number"
                inputMode="numeric"
                disabled={!editable}
                {...form.register('total_amount', { valueAsNumber: true })}
              />
            </FormField>
            <FormField<BidForm> name="lead_time" label="납기 (일)">
              <Input
                type="number"
                inputMode="numeric"
                disabled={!editable}
                {...form.register('lead_time', { valueAsNumber: true })}
              />
            </FormField>
            <FormField<BidForm> name="valid_days" label="견적 유효기간 (일)">
              <Input
                type="number"
                inputMode="numeric"
                disabled={!editable}
                {...form.register('valid_days', { valueAsNumber: true })}
              />
            </FormField>
          </div>
          <FormField<BidForm> name="note" label="비고">
            <Textarea rows={4} disabled={!editable} {...form.register('note')} />
          </FormField>

          {rfq.reserve_method === 'multiple' && Array.isArray(rfq.reserve_prices) && (
            <ReservePicker
              reserves={rfq.reserve_prices as number[]}
              picks={reservePicks}
              disabled={!editable}
              onChange={setReservePicks}
            />
          )}

          <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
            <Button variant="ghost" type="button" onClick={() => navigate('/portal')}>
              취소
            </Button>
            <Button type="submit" disabled={!editable || mutation.isPending}>
              {mutation.isPending ? '제출 중…' : existing ? '입찰서 수정' : '입찰서 제출'}
            </Button>
          </div>
        </form>

        <ClarificationsBlock
          rfqStatus={rfqStatus}
          rfqEditable={editable}
          clarifications={clarificationsQuery.data?.data ?? []}
          loading={clarificationsQuery.isLoading}
          question={question}
          onQuestionChange={setQuestion}
          onAsk={() => askQuestion.mutate(question)}
          asking={askQuestion.isPending}
        />

        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={existing ? '입찰서를 수정할까요?' : '입찰서를 제출할까요?'}
          description={
            pendingValues
              ? `입찰금액 ${formatKRW(pendingValues.total_amount)}` +
                (pendingValues.lead_time != null
                  ? ` · 납기 ${pendingValues.lead_time}일`
                  : '') +
                '\n제출 후에도 공고가 마감되기 전까지는 수정할 수 있습니다.'
              : ''
          }
          confirmLabel={existing ? '수정' : '제출'}
          loading={mutation.isPending}
          onConfirm={() => {
            if (pendingValues) mutation.mutate(pendingValues)
          }}
        />
      </FormProvider>
    </div>
  )
}

// AttachmentsBlock renders the RFQ's `attachments` file field. The backend
// stores file references as JSONB metadata; we accept either an array of
// {filename, url} objects, a single object, or a plain URL string so the
// display works no matter how the buyer populated the field.
function AttachmentsBlock({ value }: { value: unknown }) {
  const files = normaliseFiles(value)
  if (files.length === 0) return null
  return (
    <Card className="mb-6 p-5">
      <h3 className="mb-3 text-sm font-semibold text-foreground">첨부 파일</h3>
      <ul className="space-y-1 text-sm">
        {files.map((f, i) => (
          <li key={i}>
            <a
              href={f.url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-blue-600 hover:underline"
              download={f.filename}
            >
              {f.filename ?? f.url}
            </a>
          </li>
        ))}
      </ul>
    </Card>
  )
}

// ReservePicker renders the 15 reserve candidates as a two-column grid of
// selectable chips. Exactly 2 must be chosen — the picker enforces this by
// limiting selection to 2 and blocking a third. The chosen indices are
// what the backend's ResolveMultipleReservePrices function later averages.
function ReservePicker(props: {
  reserves: number[]
  picks: number[]
  disabled: boolean
  onChange: (next: number[]) => void
}) {
  const toggle = (idx: number) => {
    if (props.disabled) return
    if (props.picks.includes(idx)) {
      props.onChange(props.picks.filter((p) => p !== idx))
      return
    }
    if (props.picks.length >= 2) return // 2 picks max
    props.onChange([...props.picks, idx].sort((a, b) => a - b))
  }
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4">
      <div className="mb-2 flex items-center justify-between">
        <h4 className="text-sm font-semibold text-foreground">복수예가 선택 (2개 필수)</h4>
        <span className="text-xs text-muted-foreground">
          선택 {props.picks.length}/2
        </span>
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        15개 예비가격 중 2개를 선택하세요. 개찰 시 가장 많이 선택된 4개의 평균이 예정가가 됩니다.
      </p>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
        {props.reserves.map((price, idx) => {
          const selected = props.picks.includes(idx)
          return (
            <button
              key={idx}
              type="button"
              disabled={props.disabled || (!selected && props.picks.length >= 2)}
              onClick={() => toggle(idx)}
              className={`rounded-md border px-3 py-2 text-xs transition-colors ${
                selected
                  ? 'border-foreground bg-foreground text-white'
                  : 'border-border bg-white hover:bg-accent disabled:opacity-40'
              }`}
            >
              <div className="font-mono text-[10px] text-muted-foreground/80">
                #{idx + 1}
              </div>
              <div className={`font-medium ${selected ? 'text-white' : 'text-foreground'}`}>
                {price.toLocaleString('ko-KR')}
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function normaliseFiles(value: unknown): Array<{ url: string; filename?: string }> {
  if (!value) return []
  if (typeof value === 'string') return [{ url: value }]
  if (Array.isArray(value)) {
    return value.flatMap((v) => normaliseFiles(v))
  }
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>
    const url = typeof obj.url === 'string' ? obj.url : typeof obj.path === 'string' ? obj.path : ''
    if (!url) return []
    return [{ url, filename: typeof obj.filename === 'string' ? obj.filename : undefined }]
  }
  return []
}

// ClarificationsBlock renders the Q&A thread for the current RFQ. Everyone
// sees the same answered questions (transparency). Suppliers can post new
// questions only while the RFQ is still accepting bids — locking the input
// mirrors the bid form's editable gate so the supplier isn't misled into
// thinking a late question will be answered.
function ClarificationsBlock(props: {
  rfqStatus: string
  rfqEditable: boolean
  clarifications: EntryRow[]
  loading: boolean
  question: string
  onQuestionChange: (v: string) => void
  onAsk: () => void
  asking: boolean
}) {
  const answered = props.clarifications.filter((c) => c.status === 'answered')
  const pending = props.clarifications.filter((c) => c.status !== 'answered')

  return (
    <section className="mt-8 space-y-4">
      <h3 className="text-sm font-semibold text-foreground">질의응답 (Q&A)</h3>

      {props.loading ? (
        <p className="text-xs text-muted-foreground">불러오는 중…</p>
      ) : props.clarifications.length === 0 ? (
        <p className="text-xs text-muted-foreground">아직 등록된 질문이 없습니다.</p>
      ) : (
        <ul className="space-y-3">
          {[...answered, ...pending].map((c) => (
            <li
              key={String(c.id)}
              className="rounded-lg border border-border bg-white p-4 text-sm"
            >
              <div className="flex items-start justify-between gap-3">
                <p className="whitespace-pre-wrap text-foreground">
                  <span className="mr-2 font-medium text-muted-foreground">Q.</span>
                  {String(c.question ?? '')}
                </p>
                <span
                  className={`shrink-0 rounded px-1.5 py-0.5 text-[11px] ${
                    c.status === 'answered'
                      ? 'bg-foreground text-white'
                      : 'bg-muted text-muted-foreground'
                  }`}
                >
                  {c.status === 'answered' ? '답변됨' : '대기'}
                </span>
              </div>
              {c.status === 'answered' && (
                <p className="mt-2 whitespace-pre-wrap border-l-2 border-border pl-3 text-foreground/90">
                  <span className="mr-2 font-medium text-muted-foreground">A.</span>
                  {String(c.answer ?? '')}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}

      {props.rfqEditable ? (
        <div className="rounded-lg border border-border bg-white p-4">
          <label className="text-xs font-medium text-foreground">
            질문 등록
            <span className="ml-2 text-muted-foreground">
              · 답변은 모든 입찰 참여자에게 공개됩니다
            </span>
          </label>
          <Textarea
            rows={3}
            className="mt-2"
            placeholder="공고 내용 중 궁금한 점을 입력하세요"
            value={props.question}
            onChange={(e) => props.onQuestionChange(e.target.value)}
          />
          <div className="mt-2 flex justify-end">
            <Button
              type="button"
              size="sm"
              disabled={!props.question.trim() || props.asking}
              onClick={props.onAsk}
            >
              {props.asking ? '등록 중…' : '질문 등록'}
            </Button>
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          공고가 마감되어 새 질문은 받지 않습니다
          {props.rfqStatus ? ` (상태: ${props.rfqStatus})` : ''}.
        </p>
      )}
    </section>
  )
}
