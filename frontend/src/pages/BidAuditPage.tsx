import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCurrentUser } from '@/hooks/useAuth'
import { api } from '@/lib/api'
import { formatDateTimeKR } from '@/lib/datetime'

// Wire shape matches handler.auditLogRow in backend/internal/handler/bid.go.
interface AuditRow {
  id: string
  actor_id?: string
  actor_name: string
  action: string
  app_slug: string
  row_id?: string
  ip?: string
  detail: Record<string, unknown> | null
  created_at: string
}

interface AuditListResponse {
  data: AuditRow[]
  total: number
  page: number
  limit: number
  total_pages: number
}

const ACTION_LABEL: Record<string, string> = {
  submit: '제출',
  read_sealed: '밀봉 열람',
  read_opened: '개찰 후 열람',
  open: '개찰',
  award: '낙찰',
  distribute: 'PO 분배',
  cancel: 'RFQ 취소',
  withdraw: '입찰 철회',
}

const APP_SLUG_LABEL: Record<string, string> = {
  rfqs: '입찰 공고',
  bids: '입찰서',
  suppliers: '공급사',
  purchase_orders: '발주서',
}

const ACTION_VARIANT: Record<string, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  submit: 'default',
  read_sealed: 'secondary',
  read_opened: 'outline',
  open: 'outline',
  award: 'default',
  distribute: 'outline',
  cancel: 'destructive',
  withdraw: 'destructive',
}

const ANY = '__any__'

/**
 * Director-only audit log viewer for the Topbids compliance trail.
 *
 * Surfaces the raw _meta.bid_audit_log rows with filtering by action, app,
 * and date range. Intentionally read-only — the table is append-only at the
 * DB layer and the UI mirrors that (no delete/edit affordances).
 */
export default function BidAuditPage() {
  const { data: user } = useCurrentUser()

  const [action, setAction] = useState<string>(ANY)
  const [appSlug, setAppSlug] = useState<string>(ANY)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [page, setPage] = useState(1)

  const qs = new URLSearchParams()
  if (action !== ANY) qs.set('action', action)
  if (appSlug !== ANY) qs.set('app_slug', appSlug)
  if (from) qs.set('from', new Date(from).toISOString())
  if (to) qs.set('to', new Date(to).toISOString())
  qs.set('page', String(page))
  qs.set('limit', '50')

  const query = useQuery({
    queryKey: ['bid', 'audit', qs.toString()],
    queryFn: () => api.get<AuditListResponse>(`/bid/audit?${qs}`),
    enabled: user?.role === 'director',
  })

  if (user && user.role !== 'director') {
    return (
      <div className="p-8 text-sm text-muted-foreground">
        감사 로그는 관리자(director)만 열람할 수 있습니다.
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="입찰 감사 로그"
        description="밀봉 열람·개찰·낙찰·PO 분배 등 입찰 도메인의 모든 주요 이벤트를 확인합니다"
      />

      <div className="mb-4 grid gap-3 rounded-lg border border-border bg-white p-4 md:grid-cols-5">
        <div>
          <Label htmlFor="filter-action" className="text-xs">
            액션
          </Label>
          <Select value={action} onValueChange={(v) => setAction(v ?? ANY)}>
            <SelectTrigger id="filter-action">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>전체</SelectItem>
              {Object.entries(ACTION_LABEL).map(([k, v]) => (
                <SelectItem key={k} value={k}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="filter-app" className="text-xs">
            대상 앱
          </Label>
          <Select value={appSlug} onValueChange={(v) => setAppSlug(v ?? ANY)}>
            <SelectTrigger id="filter-app">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>전체</SelectItem>
              {Object.entries(APP_SLUG_LABEL).map(([k, v]) => (
                <SelectItem key={k} value={k}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="filter-from" className="text-xs">
            시작 (포함)
          </Label>
          <Input
            id="filter-from"
            type="datetime-local"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="filter-to" className="text-xs">
            종료 (포함)
          </Label>
          <Input
            id="filter-to"
            type="datetime-local"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </div>
        <div className="flex items-end">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setAction(ANY)
              setAppSlug(ANY)
              setFrom('')
              setTo('')
              setPage(1)
            }}
          >
            필터 초기화
          </Button>
        </div>
      </div>

      {query.isLoading && <LoadingState />}
      {query.isError && <ErrorState error={query.error} onRetry={() => query.refetch()} />}

      {query.data && (
        <>
          <div className="overflow-hidden rounded-lg border border-border bg-white">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[160px]">시각</TableHead>
                  <TableHead className="w-[120px]">작업자</TableHead>
                  <TableHead className="w-[120px]">액션</TableHead>
                  <TableHead className="w-[100px]">앱</TableHead>
                  <TableHead className="w-[220px]">대상 ID</TableHead>
                  <TableHead>상세</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {query.data.data.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="py-8 text-center text-sm text-muted-foreground">
                      표시할 감사 로그가 없습니다
                    </TableCell>
                  </TableRow>
                ) : (
                  query.data.data.map((row) => (
                    <TableRow key={row.id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {formatDateTimeKR(row.created_at)}
                      </TableCell>
                      <TableCell className="text-sm">{row.actor_name || '-'}</TableCell>
                      <TableCell>
                        <Badge variant={ACTION_VARIANT[row.action] ?? 'outline'}>
                          {ACTION_LABEL[row.action] ?? row.action}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {APP_SLUG_LABEL[row.app_slug] ?? row.app_slug}
                      </TableCell>
                      <TableCell className="font-mono text-[11px] text-muted-foreground">
                        {row.row_id ?? '-'}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {row.detail && Object.keys(row.detail).length > 0
                          ? JSON.stringify(row.detail)
                          : '-'}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          <div className="mt-4 flex items-center justify-between text-xs text-muted-foreground">
            <span>전체 {query.data.total.toLocaleString('ko-KR')}건</span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                이전
              </Button>
              <span>
                {page} / {Math.max(1, query.data.total_pages)}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= query.data.total_pages}
                onClick={() => setPage((p) => p + 1)}
              >
                다음
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
