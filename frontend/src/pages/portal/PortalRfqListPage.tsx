import { useQuery } from '@tanstack/react-query'
import { FileText } from 'lucide-react'
import { Link } from 'react-router'

import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { api } from '@/lib/api'
import { formatDateTimeKR, formatDeadlineRelative } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

// RFQ lifecycle phases that are actionable from the supplier's perspective.
// Drafts and past-deadline RFQs are filtered out server-side.
type VisibleStatus = 'published' | 'closed' | 'opened' | 'evaluating' | 'awarded'

const STATUS_LABEL: Record<VisibleStatus, string> = {
  published: '공고중',
  closed: '마감',
  opened: '개찰',
  evaluating: '심사중',
  awarded: '낙찰',
}

const STATUS_VARIANT: Record<VisibleStatus, 'default' | 'secondary' | 'outline'> = {
  published: 'default',
  closed: 'secondary',
  opened: 'outline',
  evaluating: 'outline',
  awarded: 'outline',
}


export default function PortalRfqListPage() {
  // Supplier sees RFQs that are live or recently concluded. We skip drafts
  // (not yet broadcast) and cancelled/failed (irrelevant) via a status filter.
  // Sort newest-first by deadline so imminent deadlines rise to the top.
  const query = useQuery({
    queryKey: ['portal', 'rfqs'],
    queryFn: () =>
      api.getList<EntryRow>(
        '/data/rfqs?sort=-deadline_at&limit=50&_filter=' +
          encodeURIComponent(
            JSON.stringify({
              op: 'or',
              conditions: [
                { field: 'status', op: 'eq', value: 'published' },
                { field: 'status', op: 'eq', value: 'closed' },
                { field: 'status', op: 'eq', value: 'opened' },
                { field: 'status', op: 'eq', value: 'evaluating' },
                { field: 'status', op: 'eq', value: 'awarded' },
              ],
            }),
          ),
      ),
  })

  return (
    <div>
      <PageHeader
        title="입찰 공고"
        description="참여 가능한 공고를 확인하고 입찰서를 제출하세요"
      />

      {query.isLoading && <LoadingState variant="card-grid" />}
      {query.isError && <ErrorState error={query.error} onRetry={() => query.refetch()} />}

      {query.data && query.data.data.length === 0 && (
        <div className="mx-auto mt-8 max-w-lg">
          <EmptyState
            title="진행 중인 공고가 없습니다"
            description="새 공고가 등록되면 이곳에 표시됩니다."
            icon={<FileText className="h-10 w-10" />}
          />
        </div>
      )}

      {query.data && query.data.data.length > 0 && (
        <div className="grid gap-3">
          {query.data.data.map((rfq) => {
            const status = String(rfq.status ?? '') as VisibleStatus
            const variant = STATUS_VARIANT[status] ?? 'outline'
            const label = STATUS_LABEL[status] ?? status
            const submittable = status === 'published'

            return (
              <Card key={String(rfq.id)} className="p-5 transition-shadow hover:shadow-sm">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <Badge variant={variant}>{label}</Badge>
                      <span className="text-xs text-muted-foreground">
                        {String(rfq.rfq_no ?? '')}
                      </span>
                    </div>
                    <h3 className="mt-2 truncate text-base font-semibold text-foreground">
                      {String(rfq.title ?? '(제목 없음)')}
                    </h3>
                    <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1 text-xs text-muted-foreground md:grid-cols-4">
                      <div>
                        <dt className="text-muted-foreground/70">카테고리</dt>
                        <dd className="text-foreground">{String(rfq.category ?? '-')}</dd>
                      </div>
                      <div>
                        <dt className="text-muted-foreground/70">평가방식</dt>
                        <dd className="text-foreground">
                          {rfq.eval_method === 'weighted' ? '종합평가' : '최저가'}
                        </dd>
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
                  </div>
                  <div className="flex shrink-0 flex-col items-end gap-2">
                    {submittable ? (
                      <>
                        <Link
                          to={`/portal/rfqs/${String(rfq.id)}/bid`}
                          className="inline-flex items-center rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-white hover:bg-foreground/90"
                        >
                          입찰서 제출
                        </Link>
                        <span className="text-[11px] text-muted-foreground">
                          {formatDeadlineRelative(rfq.deadline_at)}
                        </span>
                      </>
                    ) : (
                      <span className="text-xs text-muted-foreground">마감됨</span>
                    )}
                  </div>
                </div>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
