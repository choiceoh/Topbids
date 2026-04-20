import { ChevronLeft, ChevronRight, FileText, Plus } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import EmptyState from '@/components/common/EmptyState'
import EntryForm from '@/components/works/EntryForm'
import { Button } from '@/components/ui/button'
import { isLayoutType } from '@/lib/constants'
import { formatCell } from '@/lib/formatCell'
import type { Field, Process } from '@/lib/types'
import { getDisplayType, getFieldOptions, isExpandedRecord } from '@/lib/fieldGuards'

interface Props {
  fields: Field[]
  entries: Record<string, unknown>[]
  onEntryClick?: (entry: Record<string, unknown>) => void
  onEntrySubmit: (data: Record<string, unknown>, entryId?: string) => void
  submitting?: boolean
  process?: Process
  slug?: string
  collectionId?: string
  total?: number
}

export default function FormView({
  fields,
  entries,
  onEntrySubmit,
  submitting,
  process,
  slug,
  collectionId,
  total,
}: Props) {
  const [currentIndex, setCurrentIndex] = useState(0)
  const [mode, setMode] = useState<'view' | 'edit' | 'create'>('view')

  // Clamp index when entries change
  if (entries.length > 0 && currentIndex >= entries.length) {
    setCurrentIndex(entries.length - 1)
  }

  // Keyboard navigation
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (mode !== 'view') return
      if (e.key === 'ArrowLeft' && currentIndex > 0) {
        setCurrentIndex((i) => i - 1)
      } else if (e.key === 'ArrowRight' && currentIndex < entries.length - 1) {
        setCurrentIndex((i) => i + 1)
      }
    },
    [mode, currentIndex, entries.length],
  )

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  const currentEntry = entries[currentIndex]
  const dataFields = fields.filter((f) => !isLayoutType(f.field_type))
  const displayIndex = entries.length > 0 ? currentIndex + 1 : 0

  function handleFormSubmit(data: Record<string, unknown>) {
    if (mode === 'create') {
      onEntrySubmit(data)
    } else if (mode === 'edit' && currentEntry) {
      onEntrySubmit(data, String(currentEntry.id))
    }
    setMode('view')
  }

  function handleCreate() {
    setMode('create')
  }

  if (entries.length === 0 && mode !== 'create') {
    return (
      <EmptyState
        icon={<FileText className="h-10 w-10" />}
        title="데이터가 없습니다"
        description="폼 뷰에서 첫 번째 레코드를 추가하세요."
        action={
          <Button onClick={handleCreate}>
            <Plus className="mr-1 h-4 w-4" />
            새 레코드
          </Button>
        }
      />
    )
  }

  if (mode === 'create') {
    return (
      <div className="mx-auto max-w-2xl rounded-lg border bg-card p-6">
        <h3 className="mb-4 text-lg font-medium">새 레코드</h3>
        <EntryForm
          fields={fields}
          onSubmit={handleFormSubmit}
          onCancel={() => setMode('view')}
          submitting={submitting}
          process={process}
          slug={slug}
          collectionId={collectionId}
        />
      </div>
    )
  }

  if (mode === 'edit' && currentEntry) {
    return (
      <div className="mx-auto max-w-2xl rounded-lg border bg-card p-6">
        <h3 className="mb-4 text-lg font-medium">레코드 편집</h3>
        <EntryForm
          fields={fields}
          initialData={currentEntry}
          onSubmit={handleFormSubmit}
          onCancel={() => setMode('view')}
          submitting={submitting}
          process={process}
          slug={slug}
          collectionId={collectionId}
        />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl">
      {/* Navigation bar */}
      <div className="mb-4 flex items-center justify-between rounded-lg border bg-card px-4 py-2">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            disabled={currentIndex === 0}
            onClick={() => setCurrentIndex((i) => i - 1)}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm tabular-nums text-muted-foreground">
            {displayIndex} / {total ?? entries.length}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            disabled={currentIndex >= entries.length - 1}
            onClick={() => setCurrentIndex((i) => i + 1)}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleCreate}>
            <Plus className="mr-1 h-3.5 w-3.5" />
            새 레코드
          </Button>
          <Button variant="outline" size="sm" onClick={() => setMode('edit')}>
            편집
          </Button>
        </div>
      </div>

      {/* Read-only record display */}
      {currentEntry && (
        <div className="rounded-lg border bg-card">
          {/* Process status badge */}
          {process?.is_enabled && !!currentEntry._status && (
            <div className="border-b px-6 py-3">
              <span className="text-sm text-muted-foreground">상태: </span>
              <span
                className="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
                style={{
                  backgroundColor:
                    process.statuses?.find((s) => s.name === currentEntry._status)?.color ?? '#6b7280',
                }}
              >
                {String(currentEntry._status)}
              </span>
            </div>
          )}

          <div className="divide-y">
            {dataFields.map((field) => {
              const value = currentEntry[field.slug]
              return (
                <div
                  key={field.id}
                  className="grid grid-cols-3 gap-4 px-6 py-3"
                >
                  <div className="text-sm font-medium text-muted-foreground">
                    {field.label}
                  </div>
                  <div className="col-span-2 text-sm">
                    <FieldDisplay field={field} value={value} />
                  </div>
                </div>
              )
            })}
          </div>

          {/* Metadata */}
          <div className="border-t bg-muted/30 px-6 py-3">
            <div className="flex gap-6 text-xs text-muted-foreground">
              {!!currentEntry._created_at && (
                <span>
                  생성: {new Date(String(currentEntry._created_at)).toLocaleString('ko')}
                </span>
              )}
              {!!currentEntry._updated_at && (
                <span>
                  수정: {new Date(String(currentEntry._updated_at)).toLocaleString('ko')}
                </span>
              )}
              {!!currentEntry._created_by && (
                <span>
                  작성자: {isExpandedRecord(currentEntry._created_by)
                    ? String(currentEntry._created_by.name ?? '')
                    : String(currentEntry._created_by)}
                </span>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function FieldDisplay({ field, value }: { field: Field; value: unknown }) {
  if (value == null || value === '') {
    return <span className="text-muted-foreground">-</span>
  }

  // Boolean
  if (field.field_type === 'boolean') {
    return <span>{value ? 'Yes' : 'No'}</span>
  }

  // Multiselect
  if (field.field_type === 'multiselect' && Array.isArray(value)) {
    return (
      <div className="flex flex-wrap gap-1">
        {(value as string[]).map((v) => (
          <span key={v} className="rounded bg-muted px-2 py-0.5 text-xs">
            {v}
          </span>
        ))}
      </div>
    )
  }

  // Select with color
  if (field.field_type === 'select') {
    return (
      <span className="rounded bg-muted px-2 py-0.5 text-xs">
        {String(value)}
      </span>
    )
  }

  // File
  if (field.field_type === 'file' && typeof value === 'string') {
    return (
      <a
        href={value}
        target="_blank"
        rel="noopener noreferrer"
        className="text-primary underline"
      >
        {value.split('/').pop()}
      </a>
    )
  }

  // Progress
  if (
    (field.field_type === 'number' || field.field_type === 'integer') &&
    getDisplayType(field) === 'progress'
  ) {
    const num = Number(value)
    return (
      <div className="flex items-center gap-2">
        <div className="h-2 w-24 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary"
            style={{ width: `${Math.min(100, Math.max(0, num))}%` }}
          />
        </div>
        <span>{num}%</span>
      </div>
    )
  }

  // Rating
  if (
    (field.field_type === 'number' || field.field_type === 'integer') &&
    getDisplayType(field) === 'rating'
  ) {
    const max = getFieldOptions(field, 'number')?.max_rating || 5
    const current = Number(value) || 0
    return (
      <span>
        {Array.from({ length: max }, (_, i) => (
          <span key={i} className={i < current ? 'text-yellow-500' : 'text-muted-foreground/30'}>
            ★
          </span>
        ))}
      </span>
    )
  }

  // Table (sub-rows)
  if (field.field_type === 'table' && Array.isArray(value)) {
    const rows = value as Record<string, unknown>[]
    if (rows.length === 0) return <span className="text-muted-foreground">-</span>
    const cols = Object.keys(rows[0])
    return (
      <div className="overflow-auto rounded border text-xs">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              {cols.map((c) => (
                <th key={c} className="px-2 py-1 text-left font-medium">{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={i} className="border-b last:border-0">
                {cols.map((c) => (
                  <td key={c} className="px-2 py-1">{row[c] != null ? String(row[c]) : ''}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  // Default: use formatCell
  return <span>{formatCell(value, field)}</span>
}
