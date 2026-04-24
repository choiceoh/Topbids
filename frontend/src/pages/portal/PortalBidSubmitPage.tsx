import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { FormProvider, useForm } from 'react-hook-form'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { z } from 'zod'

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
import type { EntryRow } from '@/lib/types'

// RHF registers <input type="number"> with valueAsNumber:true so the form
// state holds real numbers — no coerce needed here.
const schema = z.object({
  total_amount: z.number().positive('0보다 커야 합니다'),
  lead_time: z.number().int().nonnegative('0 이상이어야 합니다').optional(),
  valid_days: z.number().int().positive('1 이상이어야 합니다').optional(),
  note: z.string().max(1000).optional(),
})

type BidForm = z.infer<typeof schema>

function formatDate(iso: unknown): string {
  if (typeof iso !== 'string' || !iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString('ko-KR', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

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

  const form = useForm<BidForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      total_amount: 0,
      lead_time: undefined,
      valid_days: 30,
      note: '',
    },
  })

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
      const payload = {
        ...values,
        rfq: rfqId,
        supplier: user.supplier_id,
        status: 'submitted',
        submitted_at: new Date().toISOString(),
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

  if (rfqQuery.isLoading) return <LoadingState />
  if (rfqQuery.isError)
    return <ErrorState error={rfqQuery.error} onRetry={() => rfqQuery.refetch()} />
  if (!rfqQuery.data) return null

  const rfq = rfqQuery.data
  const rfqStatus = String(rfq.status ?? '')
  const editable = rfqStatus === 'published'

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
        <h2 className="text-base font-semibold text-foreground">
          {String(rfq.title ?? '')}
        </h2>
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
            <dd className="text-foreground">{formatDate(rfq.deadline_at)}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground/70">개찰일시</dt>
            <dd className="text-foreground">{formatDate(rfq.open_at)}</dd>
          </div>
        </dl>
      </Card>

      {!editable && (
        <Card className="mb-6 border-amber-500/40 bg-amber-50/40 p-4 text-sm">
          이 공고는 현재 입찰을 받지 않습니다 (상태: {rfqStatus}). 제출된 내용은 참고용으로만
          표시됩니다.
        </Card>
      )}

      <FormProvider {...form}>
        <form
          onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
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

          <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
            <Button variant="ghost" type="button" onClick={() => navigate('/portal')}>
              취소
            </Button>
            <Button type="submit" disabled={!editable || mutation.isPending}>
              {mutation.isPending ? '제출 중…' : existing ? '입찰서 수정' : '입찰서 제출'}
            </Button>
          </div>
        </form>
      </FormProvider>
    </div>
  )
}
