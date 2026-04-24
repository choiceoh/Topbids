import { useQuery } from '@tanstack/react-query'
import { Inbox } from 'lucide-react'

import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import type { EntryRow } from '@/lib/types'

const BID_STATUS_LABEL: Record<string, string> = {
  draft: '작성중',
  submitted: '제출됨',
  opened: '개찰',
  evaluated: '평가완료',
  awarded: '낙찰',
  rejected: '탈락',
}

const BID_STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  draft: 'secondary',
  submitted: 'default',
  opened: 'outline',
  evaluated: 'outline',
  awarded: 'default',
  rejected: 'destructive',
}

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

function formatMoney(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  const n = typeof v === 'number' ? v : Number(v)
  if (!Number.isFinite(n)) return '—'
  return n.toLocaleString('ko-KR') + ' 원'
}

/**
 * Supplier's bid history. Backend's SupplierRowFilter already restricts the
 * response to the caller's own bids, and MaskSealedFields blanks sealed
 * values on rows where status hasn't reached the unlock threshold. Suppliers
 * do see their own sealed values (they submitted them) — the mask only
 * applies to buyer-role readers.
 */
export default function PortalBidHistoryPage() {
  const query = useQuery({
    queryKey: ['portal', 'bids'],
    queryFn: () =>
      api.getList<EntryRow>('/data/bids?sort=-submitted_at&limit=100&expand=rfq'),
  })

  return (
    <div>
      <PageHeader
        title="내 입찰 내역"
        description="제출한 입찰서와 진행 상태를 한 눈에 확인하세요"
      />

      {query.isLoading && <LoadingState />}
      {query.isError && <ErrorState error={query.error} onRetry={() => query.refetch()} />}

      {query.data && query.data.data.length === 0 && (
        <div className="mx-auto mt-8 max-w-lg">
          <EmptyState
            title="제출한 입찰서가 없습니다"
            description="공고 탭에서 참여 가능한 입찰을 확인하세요."
            icon={<Inbox className="h-10 w-10" />}
          />
        </div>
      )}

      {query.data && query.data.data.length > 0 && (
        <div className="rounded-lg border border-border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[120px]">공고번호</TableHead>
                <TableHead>공고명</TableHead>
                <TableHead className="w-[140px] text-right">입찰금액</TableHead>
                <TableHead className="w-[80px] text-center">납기(일)</TableHead>
                <TableHead className="w-[160px]">제출일시</TableHead>
                <TableHead className="w-[90px]">상태</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.data.map((bid) => {
                const rfq = bid.rfq as Record<string, unknown> | string | null | undefined
                const rfqObj = typeof rfq === 'object' && rfq !== null ? rfq : null
                const rfqNo = rfqObj ? String(rfqObj.rfq_no ?? '') : '-'
                const rfqTitle = rfqObj ? String(rfqObj.title ?? '') : '-'
                const status = String(bid.status ?? '')
                return (
                  <TableRow key={String(bid.id)}>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {rfqNo}
                    </TableCell>
                    <TableCell className="max-w-[320px] truncate">{rfqTitle}</TableCell>
                    <TableCell className="text-right font-medium">
                      {formatMoney(bid.total_amount)}
                    </TableCell>
                    <TableCell className="text-center">
                      {bid.lead_time === null || bid.lead_time === undefined
                        ? '—'
                        : String(bid.lead_time)}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatDate(bid.submitted_at)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={BID_STATUS_VARIANT[status] ?? 'outline'}>
                        {BID_STATUS_LABEL[status] ?? status}
                      </Badge>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
