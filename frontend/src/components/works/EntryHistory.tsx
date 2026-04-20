import { History } from 'lucide-react'

import EmptyState from '@/components/common/EmptyState'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useRecordHistory } from '@/hooks/useHistory'
import type { Field } from '@/lib/types'
import { isExpandedRecord } from '@/lib/fieldGuards'

const OP_LABELS: Record<string, string> = {
  create: '생성',
  update: '수정',
  delete: '삭제',
}

interface Props {
  slug: string
  recordId: string
  fields: Field[]
}

export default function EntryHistory({ slug, recordId, fields }: Props) {
  const { data: historyData, isLoading } = useRecordHistory(slug, recordId)

  if (isLoading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="space-y-2 rounded-md border p-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Skeleton className="h-5 w-12 rounded-full" />
                <Skeleton className="h-4 w-16" />
              </div>
              <Skeleton className="h-3 w-24" />
            </div>
            <Skeleton className="h-4 w-3/4" />
          </div>
        ))}
      </div>
    )
  }

  if (!historyData?.data?.length) {
    return (
      <EmptyState
        compact
        icon={<History className="h-8 w-8" />}
        title="변경 이력이 없습니다"
        description="데이터가 수정되면 이력이 기록됩니다."
      />
    )
  }

  return (
    <div className="space-y-3">
      {historyData.data.map((change) => (
        <div key={change.id} className="rounded-md border p-3 text-sm">
          <div className="mb-1 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Badge variant="secondary">{OP_LABELS[change.operation] ?? change.operation}</Badge>
              <span className="font-medium">{change.user_name || '시스템'}</span>
            </div>
            <span className="text-xs text-muted-foreground">
              {new Date(change.created_at).toLocaleString('ko')}
            </span>
          </div>
          {change.operation === 'update' && (
            <div className="mt-2 space-y-1">
              {/* Status change highlight */}
              {change.diff._status_change && (
                <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-1.5">
                  <span className="text-xs font-medium">상태 변경:</span>
                  <span
                    className="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
                    style={{ backgroundColor: (change.diff._status_change as Record<string, unknown>).from_color as string ?? '#6b7280' }}
                  >
                    {(change.diff._status_change as Record<string, unknown>).from as string}
                  </span>
                  <span className="text-xs">→</span>
                  <span
                    className="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
                    style={{ backgroundColor: (change.diff._status_change as Record<string, unknown>).to_color as string ?? '#6b7280' }}
                  >
                    {(change.diff._status_change as Record<string, unknown>).to as string}
                  </span>
                </div>
              )}
              {Object.entries(change.diff)
                .filter(([key]) => key !== '_status_change' && key !== '_status')
                .map(([key, val]) => (
                <div key={key} className="text-xs">
                  <span className="font-medium">{fieldLabel(fields, key)}</span>:{' '}
                  <span className="text-muted-foreground">{formatValue(val.old)}</span>
                  {' → '}
                  <span>{formatValue(val.new)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

function fieldLabel(fields: Field[], key: string): string {
  const field = fields.find((f) => f.slug === key)
  return field?.label ?? key
}

function formatValue(v: unknown): string {
  if (v == null) return '-'
  if (Array.isArray(v)) {
    if (v.length === 0) return '-'
    return v.map((item) => formatSingleValue(item)).join(', ')
  }
  return formatSingleValue(v)
}

function formatSingleValue(v: unknown): string {
  if (v == null) return '-'
  if (typeof v === 'object') {
    if (isExpandedRecord(v)) {
      const obj = v
      if ('display_value' in obj && obj.display_value != null) return String(obj.display_value)
      if (obj.name != null) return String(obj.name)
      if (obj.label != null) return String(obj.label)
      if (obj.title != null) return String(obj.title)
    }
    return JSON.stringify(v)
  }
  return String(v)
}
