import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragStartEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Calendar, ChevronsUpDown, GripVertical, Search, Trash2, Upload, User } from 'lucide-react'

import { FIELD_TYPE_LABELS, isLayoutType } from '@/lib/constants'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import type { FieldType, SubColumn } from '@/lib/types'
import { getChoices, getDisplayType, getFieldOptions } from '@/lib/fieldGuards'

import type { FieldDraft } from './FieldPreview'

const GRID_COLS = 6

function computeFieldRows(fields: FieldDraft[]): number[] {
  const rowOf: number[] = []
  let row = 0
  let used = 0
  for (const f of fields) {
    const w = isLayoutType(f.field_type) ? GRID_COLS : (f.width || GRID_COLS)
    if (used + w > GRID_COLS && used > 0) {
      row++
      used = 0
    }
    rowOf.push(row)
    used += w
  }
  return rowOf
}

interface Props {
  fields: FieldDraft[]
  selectedId: string | null
  onSelect: (id: string) => void
  onReorder: (fields: FieldDraft[]) => void
  onRemove: (id: string) => void
  onAdd: (fieldType: FieldType, presetOptions?: Record<string, unknown>, index?: number) => void
  onFieldChange: (updated: FieldDraft) => void
}

export default function FormPreview({ fields, selectedId, onSelect, onReorder, onRemove, onAdd, onFieldChange }: Props) {
  const gridRef = useRef<HTMLDivElement>(null)
  const resizeRef = useRef<{ fieldId: string, startX: number, startWidth: number } | null>(null)
  const [resizingId, setResizingId] = useState<string | null>(null)
  const [previewWidth, setPreviewWidth] = useState<number | null>(null)
  const [removeTargetId, setRemoveTargetId] = useState<string | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)

  const fieldIds = useMemo(() => fields.map((f) => f.id), [fields])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  )

  const handleResizeStart = useCallback((e: React.MouseEvent, field: FieldDraft) => {
    e.preventDefault()
    e.stopPropagation()
    resizeRef.current = { fieldId: field.id, startX: e.clientX, startWidth: field.width || 6 }
    setResizingId(field.id)
    setPreviewWidth(field.width || 6)
  }, [])

  useEffect(() => {
    if (!resizingId) return

    function onMouseMove(e: MouseEvent) {
      if (!resizeRef.current || !gridRef.current) return
      const gridRect = gridRef.current.getBoundingClientRect()
      const colWidth = gridRect.width / 6
      const deltaX = e.clientX - resizeRef.current.startX
      const deltaCols = Math.round(deltaX / colWidth)
      const newWidth = Math.max(1, Math.min(6, resizeRef.current.startWidth + deltaCols))
      setPreviewWidth(newWidth)
    }

    function onMouseUp() {
      if (resizeRef.current && previewWidth !== null) {
        const field = fields.find((f) => f.id === resizeRef.current!.fieldId)
        if (field && previewWidth !== field.width) {
          onFieldChange({ ...field, width: previewWidth })
        }
      }
      resizeRef.current = null
      setResizingId(null)
      setPreviewWidth(null)
    }

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
    return () => {
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
    }
  }, [resizingId, previewWidth, fields, onFieldChange])

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id as string)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveId(null)
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = fields.findIndex((f) => f.id === active.id)
    const newIndex = fields.findIndex((f) => f.id === over.id)
    if (oldIndex === -1 || newIndex === -1) return
    const updated = [...fields]
    const [moved] = updated.splice(oldIndex, 1)

    const restRows = computeFieldRows(updated)
    const neighborIdx = Math.max(0, Math.min(
      oldIndex < newIndex ? newIndex - 1 : newIndex,
      updated.length - 1,
    ))
    const targetRow = updated.length > 0 ? restRows[neighborIdx] : 0

    const restRowMembers: number[] = []
    for (let i = 0; i < restRows.length; i++) {
      if (restRows[i] === targetRow) restRowMembers.push(i)
    }

    updated.splice(newIndex, 0, moved)

    // Map rest indices → final indices (items at ≥ newIndex shifted +1 after insert).
    const finalIndices = restRowMembers.map((i) => (i >= newIndex ? i + 1 : i))
    finalIndices.push(newIndex)
    finalIndices.sort((a, b) => a - b)

    const hasLayout = finalIndices.some((i) => isLayoutType(updated[i].field_type))

    if (!hasLayout && finalIndices.length <= GRID_COLS) {
      const total = finalIndices.reduce((s, i) => s + (updated[i].width || GRID_COLS), 0)
      if (total > GRID_COLS) {
        const count = finalIndices.length
        const base = Math.floor(GRID_COLS / count)
        const extra = GRID_COLS - base * count
        for (let k = 0; k < count; k++) {
          const w = base + (k < extra ? 1 : 0)
          updated[finalIndices[k]] = {
            ...updated[finalIndices[k]],
            width: Math.max(1, Math.min(GRID_COLS, w)),
          }
        }
      }
    }

    onReorder(updated)
  }

  function handlePaletteDrop(e: React.DragEvent) {
    e.preventDefault()
    e.currentTarget.classList.remove('bg-accent/40')
    const paletteData = e.dataTransfer.getData('application/palette-field')
    if (paletteData) {
      const { type, presetOptions } = JSON.parse(paletteData)
      onAdd(type, presetOptions)
    }
  }

  if (fields.length === 0) {
    return (
      <div
        className="flex h-64 items-center justify-center rounded-lg border-2 border-dashed text-sm text-muted-foreground transition-colors"
        onDragOver={(e) => { e.preventDefault(); e.currentTarget.classList.add('bg-accent/40') }}
        onDragLeave={(e) => { e.currentTarget.classList.remove('bg-accent/40') }}
        onDrop={handlePaletteDrop}
      >
        왼쪽에서 항목을 드래그하거나 클릭하여 추가하세요
      </div>
    )
  }

  const smSpan: Record<number, string> = {
    1: 'sm:col-span-1', 2: 'sm:col-span-2', 3: 'sm:col-span-3',
    4: 'sm:col-span-4', 5: 'sm:col-span-5', 6: 'sm:col-span-6',
  }

  const rowSpan: Record<number, string> = {
    1: '',
    2: 'row-span-2 min-h-24',
    3: 'row-span-3 min-h-36',
  }

  const activeField = activeId ? fields.find((f) => f.id === activeId) : null

  return (
    <div>
      <h3 className="mb-2 text-sm font-medium text-foreground/70">입력화면 미리보기</h3>
      <div
        className="rounded-lg border bg-background p-4"
        onDragOver={(e) => {
          if (e.dataTransfer.types.includes('application/palette-field')) {
            e.preventDefault()
            e.currentTarget.classList.add('ring-2', 'ring-primary/30')
          }
        }}
        onDragLeave={(e) => {
          e.currentTarget.classList.remove('ring-2', 'ring-primary/30')
        }}
        onDrop={(e) => {
          e.currentTarget.classList.remove('ring-2', 'ring-primary/30')
          handlePaletteDrop(e)
        }}
      >
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragEnd={handleDragEnd}
        >
          <SortableContext items={fieldIds} strategy={verticalListSortingStrategy}>
            <div ref={gridRef} className="grid grid-cols-1 gap-4 sm:grid-cols-6">
              {fields.map((field) => {
                const span = resizingId === field.id && previewWidth !== null
                  ? previewWidth
                  : (field.width || 6)
                return (
                  <SortableFieldItem
                    key={field.id}
                    field={field}
                    span={isLayoutType(field.field_type) ? 6 : span}
                    isLayout={isLayoutType(field.field_type)}
                    isSelected={selectedId === field.id}
                    isResizing={resizingId === field.id}
                    smSpan={smSpan}
                    rowSpan={rowSpan}
                    onSelect={onSelect}
                    onRemove={setRemoveTargetId}
                    onResizeStart={handleResizeStart}
                  />
                )
              })}
            </div>
          </SortableContext>
          <DragOverlay dropAnimation={{ duration: 200, easing: 'ease' }}>
            {activeField ? (
              <div className="rounded-md border bg-background p-2 shadow-lg opacity-90">
                <Label>{activeField.label || '(제목 없음)'}</Label>
                <div className="pointer-events-none mt-1">
                  <DraftFieldInput field={activeField} />
                </div>
              </div>
            ) : null}
          </DragOverlay>
        </DndContext>
      </div>
      <ConfirmDialog
        open={!!removeTargetId}
        onOpenChange={(open) => !open && setRemoveTargetId(null)}
        title="항목을 제거하시겠습니까?"
        description="이 항목이 입력화면에서 제거됩니다."
        variant="destructive"
        confirmLabel="제거"
        onConfirm={() => {
          if (removeTargetId) onRemove(removeTargetId)
          setRemoveTargetId(null)
        }}
      />
    </div>
  )
}

/* ─── Sortable field item ─── */

interface SortableFieldItemProps {
  field: FieldDraft
  span: number
  isLayout: boolean
  isSelected: boolean
  isResizing: boolean
  smSpan: Record<number, string>
  rowSpan: Record<number, string>
  onSelect: (id: string) => void
  onRemove: (id: string) => void
  onResizeStart: (e: React.MouseEvent, field: FieldDraft) => void
}

function SortableFieldItem({
  field, span, isLayout, isSelected, isResizing, smSpan, rowSpan,
  onSelect, onRemove, onResizeStart,
}: SortableFieldItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: field.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  if (isLayout) {
    return (
      <div
        ref={setNodeRef}
        style={style}
        className={`col-span-full ${isDragging ? 'z-10 opacity-40' : ''}`}
      >
        <div
          className={`relative flex items-center gap-1 cursor-pointer rounded-md px-1 py-0.5 transition-colors ${
            isSelected ? 'ring-2 ring-primary ring-offset-2' : 'hover:bg-accent/30'
          }`}
          onClick={() => onSelect(field.id)}
        >
          <div
            className="flex shrink-0 cursor-grab items-center text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="h-4 w-4" />
          </div>
          <div className="flex-1">
            <LayoutPreview field={field} />
          </div>
          <button
            onClick={(e) => { e.stopPropagation(); onRemove(field.id) }}
            className="absolute top-1 right-1 rounded p-0.5 text-muted-foreground opacity-0 transition-opacity hover:text-destructive [div:hover>&]:opacity-100"
            type="button"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    )
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`col-span-full ${smSpan[span] ?? 'sm:col-span-6'} ${rowSpan[field.height] ?? ''} ${isDragging ? 'z-10 opacity-40' : ''}`}
    >
      <div
        className={`relative cursor-pointer rounded-md p-2 transition-all duration-200 ${
          isSelected ? 'ring-2 ring-primary ring-offset-2' : 'hover:bg-accent/30'
        } ${isResizing ? 'ring-2 ring-primary/50' : ''}`}
        onClick={() => onSelect(field.id)}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            <div
              className="flex shrink-0 cursor-grab items-center text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
              {...attributes}
              {...listeners}
            >
              <GripVertical className="h-4 w-4" />
            </div>
            <Label className="pointer-events-none">
              {field.label || '(제목 없음)'}
              {field.is_required && <span className="ml-1 text-destructive">*</span>}
            </Label>
          </div>
          <div className="flex items-center gap-1">
            {(isSelected || isResizing) && (
              <span className="text-[10px] text-muted-foreground">{span}/6</span>
            )}
            <button
              onClick={(e) => { e.stopPropagation(); onRemove(field.id) }}
              className="rounded p-0.5 text-muted-foreground opacity-0 transition-opacity hover:text-destructive [div:hover>&]:opacity-100"
              type="button"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        {field.description && (
          <p className="mt-0.5 text-xs text-muted-foreground">{field.description}</p>
        )}
        <div className="pointer-events-none mt-1">
          <DraftFieldInput field={field} />
        </div>
        {/* Resize handle */}
        <div
          className="absolute top-0 right-0 h-full w-2 cursor-col-resize rounded-r-md opacity-0 transition-opacity hover:bg-primary/20 hover:opacity-100 [div:hover>&]:opacity-100"
          onMouseDown={(e) => onResizeStart(e, field)}
        />
      </div>
    </div>
  )
}

function LayoutPreview({ field }: { field: FieldDraft }) {
  switch (field.field_type) {
    case 'label':
      return (
        <p className="text-sm text-muted-foreground">
          {getFieldOptions(field, 'label')?.content || field.label}
        </p>
      )
    case 'line':
      return <hr className="my-2" />
    case 'spacer':
      return <div style={{ height: getFieldOptions(field, 'spacer')?.height || 24 }} />
    default:
      return null
  }
}

/** Read-only preview of a field input, matching EntryForm's FieldInput rendering. */
function DraftFieldInput({ field }: { field: FieldDraft }) {
  const h = field.height || 1

  switch (field.field_type) {
    case 'textarea':
      return (
        <Textarea
          readOnly
          tabIndex={-1}
          rows={getFieldOptions(field, 'textarea')?.rows || Math.max(4, h * 2)}
          placeholder={field.label || '텍스트 입력'}
        />
      )
    case 'time':
      return <Input type="time" readOnly tabIndex={-1} />
    case 'text': {
      const textDisplay = getDisplayType(field)
      if (h > 1) {
        return (
          <Textarea readOnly tabIndex={-1} rows={h * 2} placeholder={field.label || '텍스트 입력'} />
        )
      }
      const inputType = textDisplay === 'email' ? 'email'
        : textDisplay === 'url' ? 'url'
        : textDisplay === 'phone' ? 'tel'
        : 'text'
      return <Input type={inputType} readOnly tabIndex={-1} placeholder={field.label || '텍스트 입력'} />
    }
    case 'number':
    case 'integer': {
      const displayType = getDisplayType(field)
      const numOpts = getFieldOptions(field, 'number')
      if (displayType === 'rating') {
        const max = numOpts?.max_rating || 5
        return (
          <div className="flex items-center gap-1 pt-1">
            {Array.from({ length: max }, (_, i) => (
              <span key={i} className="text-lg text-muted-foreground/30">★</span>
            ))}
          </div>
        )
      }
      if (displayType === 'progress') {
        return (
          <div className="space-y-1">
            <Input type="number" readOnly tabIndex={-1} placeholder="0" />
            <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
              <div className="h-full w-0 rounded-full bg-primary" />
            </div>
          </div>
        )
      }
      const prefix = displayType === 'currency'
        ? numOpts?.currency_code === 'USD' ? '$' : '₩'
        : undefined
      const suffix = displayType === 'percent' ? '%' : undefined
      return (
        <div className="relative">
          {prefix && (
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              {prefix}
            </span>
          )}
          <Input
            type="number"
            readOnly
            tabIndex={-1}
            placeholder="0"
            className={prefix ? 'pl-8' : suffix ? 'pr-8' : ''}
          />
          {suffix && (
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              {suffix}
            </span>
          )}
        </div>
      )
    }
    case 'date':
      return (
        <Button
          variant="outline"
          type="button"
          disabled
          className="w-full justify-start text-left font-normal text-muted-foreground"
        >
          <Calendar className="mr-2 h-4 w-4" />
          날짜 선택
        </Button>
      )
    case 'datetime':
      return <Input type="datetime-local" readOnly tabIndex={-1} />
    case 'boolean':
      return (
        <div className="flex items-center gap-2 pt-1">
          <Checkbox disabled />
        </div>
      )
    case 'select': {
      const choices = getChoices(field)
      const display = getFieldOptions(field, 'select')?.display
      if (display === 'radio') {
        return (
          <div className="space-y-1">
            {choices.length > 0 ? choices.map((c) => (
              <label key={c} className="flex items-center gap-2 text-sm">
                <input type="radio" disabled />
                {c}
              </label>
            )) : (
              <span className="text-xs text-muted-foreground">선택지를 추가하세요</span>
            )}
          </div>
        )
      }
      return (
        <Select disabled>
          <SelectTrigger>
            <SelectValue placeholder={choices.length > 0 ? `항목 선택 (${choices.length}개)` : '항목 선택'} />
          </SelectTrigger>
        </Select>
      )
    }
    case 'multiselect': {
      const choices = getChoices(field)
      return (
        <div className="flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-3 py-1.5 text-sm shadow-xs opacity-70">
          {choices.length > 0 ? (
            <>
              {choices.slice(0, 3).map((c) => (
                <Badge key={c} variant="secondary" className="text-xs">
                  {c}
                </Badge>
              ))}
              {choices.length > 3 && (
                <span className="text-xs text-muted-foreground">+{choices.length - 3}</span>
              )}
            </>
          ) : (
            <span className="text-muted-foreground">선택...</span>
          )}
        </div>
      )
    }
    case 'relation': {
      const isMany = field.relation?.relation_type === 'many_to_many'
      return (
        <Button
          variant="outline"
          type="button"
          disabled
          className="w-full justify-between font-normal text-muted-foreground"
        >
          <span className="flex items-center gap-2">
            <Search className="h-4 w-4" />
            {isMany ? '관계 항목 검색 (다중)' : '관계 항목 검색'}
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0" />
        </Button>
      )
    }
    case 'user':
      return (
        <Button
          variant="outline"
          type="button"
          disabled
          className="w-full justify-between font-normal text-muted-foreground"
        >
          <span className="flex items-center gap-2">
            <User className="h-4 w-4" />
            사용자 선택
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0" />
        </Button>
      )
    case 'autonumber':
      return <Input readOnly tabIndex={-1} value="(자동 생성)" className="bg-muted" />
    case 'formula':
    case 'lookup':
    case 'rollup':
      return (
        <div className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">
          ({FIELD_TYPE_LABELS[field.field_type]}) 자동 계산
        </div>
      )
    case 'spreadsheet': {
      const spreadOpts = getFieldOptions(field, 'spreadsheet')
      const cols = spreadOpts?.sub_columns || [
        { key: 'col1', label: 'A' },
        { key: 'col2', label: 'B' },
        { key: 'col3', label: 'C' },
      ]
      const rowCount = Math.min(spreadOpts?.initial_rows || 5, 4)
      return (
        <div className="rounded-md border overflow-hidden text-xs">
          <table className="w-full border-collapse">
            <thead>
              <tr className="bg-muted/70">
                <th className="w-8 border-r border-b px-1 py-0.5 text-center text-[10px] text-muted-foreground" />
                {cols.map((c, i) => (
                  <th key={c.key} className="border-r border-b px-2 py-0.5 text-center text-[10px] font-medium text-muted-foreground min-w-[60px]">
                    <span className="text-muted-foreground/50 mr-0.5">{String.fromCharCode(65 + i)}</span>
                    {c.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Array.from({ length: rowCount }, (_, ri) => (
                <tr key={ri}>
                  <td className="border-r border-b px-1 py-0.5 text-center text-[10px] text-muted-foreground bg-muted/30">
                    {ri + 1}
                  </td>
                  {cols.map((c) => (
                    <td key={c.key} className="border-r border-b px-1 py-0.5 h-6" />
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    }
    case 'file':
      return (
        <div className="flex items-center gap-2 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
          <Upload className="h-4 w-4" />
          파일 업로드
        </div>
      )
    case 'table': {
      const subColumns: SubColumn[] = getFieldOptions(field, 'table')?.sub_columns || [
        { key: 'col1', label: '항목' },
        { key: 'col2', label: '값' },
      ]
      return (
        <div className="space-y-2">
          <div className="overflow-hidden rounded-md border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="w-8 px-2 py-1.5 text-left text-xs font-medium text-muted-foreground">#</th>
                  {subColumns.map((col) => (
                    <th key={col.key} className="px-2 py-1.5 text-left text-xs font-medium text-muted-foreground">
                      {col.label}
                    </th>
                  ))}
                  <th className="w-8 px-2 py-1.5" />
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td colSpan={subColumns.length + 2} className="px-2 py-3 text-center text-xs text-muted-foreground">
                    행을 추가하세요
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <Button type="button" variant="outline" size="sm" disabled className="text-xs">
            + 행 추가
          </Button>
        </div>
      )
    }
    case 'json':
      return <Textarea readOnly tabIndex={-1} rows={Math.max(4, h * 2)} placeholder="{ }" />
    default:
      return <Input readOnly tabIndex={-1} placeholder={field.label || '입력'} />
  }
}
