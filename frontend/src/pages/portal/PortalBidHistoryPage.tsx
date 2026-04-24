import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Inbox } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api, formatError } from '@/lib/api'
import { formatDateTimeKR, formatKRW } from '@/lib/datetime'
import type { EntryRow } from '@/lib/types'

const BID_STATUS_LABEL: Record<string, string> = {
  draft: '작성중',
  submitted: '제출됨',
  opened: '개찰',
  evaluated: '평가완료',
  awarded: '낙찰',
  rejected: '탈락',
  withdrawn: '철회',
}

const BID_STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  draft: 'secondary',
  submitted: 'default',
  opened: 'outline',
  evaluated: 'outline',
  awarded: 'default',
  rejected: 'destructive',
  withdrawn: 'secondary',
}

/**
 * Supplier's bid history. Backend's SupplierRowFilter already restricts the
 * response to the caller's own bids, and MaskSealedFields blanks sealed
 * values on rows where status hasn't reached the unlock threshold. Suppliers
 * do see their own sealed values (they submitted them) — the mask only
 * applies to buyer-role readers.
 */
export default function PortalBidHistoryPage() {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: ['portal', 'bids'],
    queryFn: () =>
      api.getList<EntryRow>('/data/bids?sort=-submitted_at&limit=100&expand=rfq'),
  })

  const [withdrawTarget, setWithdrawTarget] = useState<EntryRow | null>(null)
  const withdraw = useMutation({
    mutationFn: (bidID: string) =>
      api.post<{ bid_id: string; status: string }>(`/bid/bids/${bidID}/withdraw`, {}),
    onSuccess: () => {
      toast.success('입찰서가 철회되었습니다')
      qc.invalidateQueries({ queryKey: ['portal', 'bids'] })
    },
    onError: (err) => toast.error(formatError(err)),
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
                <TableHead className="w-[180px] text-right">입찰금액</TableHead>
                <TableHead className="w-[80px] text-center">납기(일)</TableHead>
                <TableHead className="w-[160px]">제출일시</TableHead>
                <TableHead className="w-[90px]">상태</TableHead>
                <TableHead className="w-[90px] text-right">작업</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.data.map((bid) => {
                const rfq = bid.rfq as Record<string, unknown> | string | null | undefined
                const rfqObj = typeof rfq === 'object' && rfq !== null ? rfq : null
                const rfqNo = rfqObj ? String(rfqObj.rfq_no ?? '') : '-'
                const rfqTitle = rfqObj ? String(rfqObj.title ?? '') : '-'
                const rfqStatus = rfqObj ? String(rfqObj.status ?? '') : ''
                const status = String(bid.status ?? '')
                // Withdraw only while the RFQ is still accepting and the bid
                // is in an actionable state — mirrors backend WithdrawBid.
                const canWithdraw = rfqStatus === 'published' && (status === 'submitted' || status === 'draft')
                return (
                  <TableRow key={String(bid.id)}>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {rfqNo}
                    </TableCell>
                    <TableCell className="max-w-[320px] truncate">{rfqTitle}</TableCell>
                    <TableCell className="text-right font-medium">
                      {formatKRW(bid.total_amount)}
                    </TableCell>
                    <TableCell className="text-center">
                      {bid.lead_time === null || bid.lead_time === undefined
                        ? '—'
                        : String(bid.lead_time)}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatDateTimeKR(bid.submitted_at)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={BID_STATUS_VARIANT[status] ?? 'outline'}>
                        {BID_STATUS_LABEL[status] ?? status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      {canWithdraw ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setWithdrawTarget(bid)}
                        >
                          철회
                        </Button>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={!!withdrawTarget}
        onOpenChange={(open) => !open && setWithdrawTarget(null)}
        title="입찰서를 철회할까요?"
        description="철회 후에는 상태가 '철회'로 바뀌고, 공고가 마감되기 전이라도 같은 내용으로 다시 제출해야 합니다."
        confirmLabel="철회"
        variant="destructive"
        loading={withdraw.isPending}
        onConfirm={async () => {
          if (!withdrawTarget) return
          await withdraw.mutateAsync(String(withdrawTarget.id))
          setWithdrawTarget(null)
        }}
      />
    </div>
  )
}
