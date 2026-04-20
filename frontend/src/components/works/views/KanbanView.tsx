/**
 * KanbanView — Drag-and-drop board view grouped by a select field.
 *
 * Built on @dnd-kit for accessible drag-and-drop between columns.
 * Each column corresponds to one of the groupField's select choices,
 * plus an "(empty)" column for entries with no value set.
 *
 * Key behaviors:
 * - Dragging a card between columns fires onCardMove, which updates
 *   the group field value on the entry via the parent's mutation.
 * - Drop animations: scale-bounce on successful drop, spring-back on cancel.
 * - Cards show the title field prominently and up to 3 additional fields.
 * - Collapsed columns can be toggled to save horizontal space.
 * - Keyboard navigation: arrow keys move focus between cards within a column.
 */
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  closestCorners,
  type DragStartEvent,
  type DragEndEvent,
  type DragOverEvent,
} from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Ban, ChevronRight, ChevronDown, LayoutGrid, Loader2, Plus } from 'lucide-react'

import EmptyState from '@/components/common/EmptyState'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useKanbanView, type KanbanColumn } from '@/hooks/useEntries'
import { formatCell } from '@/lib/formatCell'
import { isLayoutType } from '@/lib/constants'
import type { EntryRow, Field } from '@/lib/types'

interface Props {
  slug: string
  groupField: Field
  fields: Field[]
  filters?: Record<string, string>
  onCardClick: (entry: Record<string, unknown>) => void
  onCardMove?: (entryId: string, newValue: string, oldValue: string) => void
  onAddEntry?: () => void
}

function SortableCard({
  entry,
  titleField,
  cardFields,
  onClick,
  justDropped,
  justCancelled,
  isFocused,
}: {
  entry: Record<string, unknown>
  titleField: Field | undefined
  cardFields: Field[]
  onClick: () => void
  justDropped?: boolean
  justCancelled?: boolean
  isFocused?: boolean
}) {
  const id = String(entry.id)
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    data: { entry },
  })
  const cardRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (isFocused) cardRef.current?.scrollIntoView({ block: 'nearest' })
  }, [isFocused])

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
  }

  return (
    <Card
      ref={(node) => {
        setNodeRef(node)
        ;(cardRef as React.MutableRefObject<HTMLDivElement | null>).current = node
      }}
      style={style}
      {...attributes}
      {...listeners}
      className={`cursor-grab p-3 transition-colors hover:bg-accent active:cursor-grabbing ${justDropped ? 'animate-scale-bounce' : justCancelled ? 'animate-spring-back' : ''} ${isFocused ? 'ring-2 ring-primary ring-offset-1' : ''}`}
      onClick={(e) => {
        if (!isDragging) {
          e.stopPropagation()
          onClick()
        }
      }}
    >
      <p className="truncate text-sm font-medium">
        {titleField
          ? String(entry[titleField.slug] || '제목 없음')
          : `#${String(entry.id).slice(0, 8)}`}
      </p>
      {/* Additional field summaries */}
      {cardFields.map((f) => {
        const val = entry[f.slug]
        if (val == null || val === '') return null
        return (
          <p key={f.slug} className="mt-0.5 truncate text-xs text-muted-foreground">
            <span className="font-medium">{f.label}:</span>{' '}
            {formatCell(val, f)}
          </p>
        )
      })}
      <p className="mt-1 text-xs text-muted-foreground">
        {entry.created_at
          ? new Date(entry.created_at as string).toLocaleDateString('ko')
          : ''}
      </p>
    </Card>
  )
}

function DroppableColumn({
  column,
  titleField,
  cardFields,
  onCardClick,
  droppedId,
  cancelledId,
  dropState,
  isDropTarget,
  collapsed,
  onToggleCollapse,
  focusedCardId,
}: {
  column: KanbanColumn
  titleField: Field | undefined
  cardFields: Field[]
  onCardClick: (entry: Record<string, unknown>) => void
  droppedId?: string | null
  cancelledId?: string | null
  dropState: 'idle' | 'allowed' | 'blocked'
  isDropTarget?: boolean
  collapsed: boolean
  onToggleCollapse: () => void
  focusedCardId?: string | null
}) {
  const ids = column.entries.map((e) => String(e.id))
  const count = column.entries.length

  if (collapsed) {
    return (
      <div className="min-w-[48px] flex-shrink-0 snap-center">
        <div className="flex flex-col items-center gap-2">
          <button
            type="button"
            className="rounded p-0.5 hover:bg-accent"
            aria-label="컬럼 펼치기"
            onClick={onToggleCollapse}
          >
            <ChevronRight className="h-4 w-4" />
          </button>
          <Badge variant="secondary" className="py-2" style={{ writingMode: 'vertical-lr', transform: 'rotate(180deg)' }}>
            {column.label}
          </Badge>
          <span className="text-xs text-muted-foreground">{count}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="min-w-[240px] flex-shrink-0 snap-center">
      <div className="mb-2 flex items-center gap-2">
        <button
          type="button"
          className="rounded p-0.5 hover:bg-accent"
          aria-label="컬럼 접기"
          onClick={onToggleCollapse}
        >
          <ChevronDown className="h-4 w-4" />
        </button>
        <Badge variant="secondary">{column.label}</Badge>
        <span className="text-xs text-muted-foreground">{count}</span>
        {dropState === 'blocked' && (
          <Ban className="h-3.5 w-3.5 text-muted-foreground/50" />
        )}
      </div>
      <SortableContext items={ids} strategy={verticalListSortingStrategy} id={column.value}>
        <div
          className={`min-h-[60px] space-y-2 rounded-lg border-2 p-1 transition-colors ${
            dropState === 'blocked'
              ? 'border-dashed border-muted-foreground/20 bg-muted/30 opacity-50'
              : dropState === 'allowed' || isDropTarget
                ? 'border-primary bg-primary/5'
                : 'border-transparent'
          }`}
        >
          {column.entries.map((entry) => (
            <SortableCard
              key={String(entry.id)}
              entry={entry}
              titleField={titleField}
              cardFields={cardFields}
              onClick={() => onCardClick(entry)}
              justDropped={droppedId === String(entry.id)}
              justCancelled={cancelledId === String(entry.id)}
              isFocused={focusedCardId === String(entry.id)}
            />
          ))}
          {column.entries.length === 0 && (
            <div className="rounded border-2 border-dashed p-4 text-center text-xs text-muted-foreground">
              비어 있음
            </div>
          )}
        </div>
      </SortableContext>
    </div>
  )
}

export default function KanbanView({
  slug,
  groupField,
  fields,
  filters,
  onCardClick,
  onCardMove,
  onAddEntry,
}: Props) {
  const { data: kanbanData, isLoading } = useKanbanView(slug, {
    groupField: groupField.slug,
    filters,
  })

  const columns: KanbanColumn[] = useMemo(() => kanbanData?.columns ?? [], [kanbanData?.columns])
  const serverAllowedMoves = kanbanData?.allowed_moves

  const allowedMoves = useMemo(() => {
    if (!serverAllowedMoves) return undefined
    const map = new Map<string, Set<string>>()
    for (const [from, tos] of Object.entries(serverAllowedMoves)) {
      map.set(from, new Set(tos))
    }
    return map
  }, [serverAllowedMoves])

  const choices = columns.map((c) => c.value)

  // Card fields: first 3 non-layout, non-title, non-groupField visible fields
  const titleField = fields.find((f) => f.field_type === 'text')
  const cardFields = useMemo(() => {
    return fields
      .filter((f) =>
        !isLayoutType(f.field_type) &&
        f.slug !== titleField?.slug &&
        f.slug !== groupField.slug &&
        f.field_type !== 'file' &&
        f.field_type !== 'json',
      )
      .slice(0, 3)
  }, [fields, titleField?.slug, groupField.slug])

  const [activeEntry, setActiveEntry] = useState<Record<string, unknown> | null>(null)
  const [droppedId, setDroppedId] = useState<string | null>(null)
  const [cancelledId, setCancelledId] = useState<string | null>(null)
  const [fromColumn, setFromColumn] = useState<string | null>(null)
  const [overColumnValue, setOverColumnValue] = useState<string | null>(null)
  const [collapsedColumns, setCollapsedColumns] = useState<Set<string>>(new Set())

  // Keyboard navigation state
  const [focusedCol, setFocusedCol] = useState<number>(-1)
  const [focusedCard, setFocusedCard] = useState<number>(-1)
  const containerRef = useRef<HTMLDivElement>(null)

  const focusedCardId = useMemo(() => {
    if (focusedCol < 0 || focusedCard < 0) return null
    const col = columns[focusedCol]
    if (!col) return null
    const entry = col.entries[focusedCard]
    return entry ? String(entry.id) : null
  }, [focusedCol, focusedCard, columns])

  useEffect(() => {
    if (!droppedId) return
    const t = setTimeout(() => setDroppedId(null), 300)
    return () => clearTimeout(t)
  }, [droppedId])

  useEffect(() => {
    if (!cancelledId) return
    const t = setTimeout(() => setCancelledId(null), 350)
    return () => clearTimeout(t)
  }, [cancelledId])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  )

  const columnDropStates = useMemo(() => {
    const states = new Map<string, 'idle' | 'allowed' | 'blocked'>()
    if (!fromColumn || !allowedMoves) {
      columns.forEach((c) => states.set(c.value, 'idle'))
      return states
    }
    const allowed = allowedMoves.get(fromColumn)
    columns.forEach((c) => {
      if (c.value === fromColumn) {
        states.set(c.value, 'idle')
      } else if (allowed && allowed.has(c.value)) {
        states.set(c.value, 'allowed')
      } else if (allowed) {
        states.set(c.value, 'blocked')
      } else {
        states.set(c.value, 'allowed')
      }
    })
    return states
  }, [fromColumn, allowedMoves, columns])

  function findColumnValue(entryId: string): string | undefined {
    for (const col of columns) {
      if (col.entries.some((e) => String(e.id) === entryId)) return col.value
      if (col.value === entryId) return col.value
    }
    return undefined
  }

  function handleDragStart(event: DragStartEvent) {
    const entry = event.active.data.current?.entry as EntryRow | undefined
    setActiveEntry(entry ?? null)
    if (entry) {
      const col = findColumnValue(String(entry.id))
      setFromColumn(col ?? null)
    }
  }

  function handleDragOver(event: DragOverEvent) {
    const { over } = event
    if (!over) { setOverColumnValue(null); return }
    const overId = String(over.id)
    let col = findColumnValue(overId)
    if (choices.includes(overId) || overId === '__none__') col = overId
    setOverColumnValue(col ?? null)
  }

  function handleDragEnd(event: DragEndEvent) {
    const draggedId = activeEntry ? String(activeEntry.id) : null
    setActiveEntry(null)
    setFromColumn(null)
    setOverColumnValue(null)
    const { active, over } = event

    if (!over || !onCardMove) {
      if (draggedId) setCancelledId(draggedId)
      return
    }

    const activeId = String(active.id)
    const overId = String(over.id)

    const fromCol = findColumnValue(activeId)
    let toCol = findColumnValue(overId)
    if (choices.includes(overId) || overId === '__none__') {
      toCol = overId
    }

    if (!toCol || fromCol === toCol) {
      if (draggedId) setCancelledId(draggedId)
      return
    }

    // Check permission
    if (allowedMoves && fromCol) {
      const allowed = allowedMoves.get(fromCol)
      if (allowed && !allowed.has(toCol)) {
        setCancelledId(activeId)
        return
      }
    }

    setDroppedId(activeId)
    onCardMove(activeId, toCol === '__none__' ? '' : toCol, fromCol ?? '')
  }

  // Keyboard navigation handler
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (columns.length === 0) return
    const { key } = e

    if (key === 'ArrowRight') {
      e.preventDefault()
      setFocusedCol((prev) => {
        const next = Math.min(prev + 1, columns.length - 1)
        setFocusedCard(0)
        return next
      })
    } else if (key === 'ArrowLeft') {
      e.preventDefault()
      setFocusedCol((prev) => {
        const next = Math.max(prev - 1, 0)
        setFocusedCard(0)
        return next
      })
    } else if (key === 'ArrowDown') {
      e.preventDefault()
      setFocusedCard((prev) => {
        const col = columns[focusedCol]
        if (!col) return prev
        return Math.min(prev + 1, col.entries.length - 1)
      })
    } else if (key === 'ArrowUp') {
      e.preventDefault()
      setFocusedCard((prev) => Math.max(prev - 1, 0))
    } else if (key === 'Enter') {
      e.preventDefault()
      const col = columns[focusedCol]
      if (col) {
        const entry = col.entries[focusedCard]
        if (entry) onCardClick(entry)
      }
    }
  }, [columns, focusedCol, focusedCard, onCardClick])

  const [hideEmpty, setHideEmpty] = useState(false)
  const emptyCount = columns.filter((c) => c.entries.length === 0).length
  const visibleColumns = hideEmpty ? columns.filter((c) => c.entries.length > 0) : columns

  const toggleCollapse = useCallback((value: string) => {
    setCollapsedColumns((prev) => {
      const next = new Set(prev)
      if (next.has(value)) next.delete(value)
      else next.add(value)
      return next
    })
  }, [])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const totalEntries = columns.reduce((sum, c) => sum + c.entries.length, 0)
  if (totalEntries === 0) {
    return (
      <EmptyState
        icon={<LayoutGrid className="h-10 w-10" />}
        title="칸반 보드에 표시할 데이터가 없습니다"
        description="데이터를 추가하면 칸반 보드에 카드가 표시됩니다."
        action={onAddEntry && (
          <Button size="sm" onClick={onAddEntry}>
            <Plus className="mr-1 h-4 w-4" />
            첫 데이터 추가
          </Button>
        )}
      />
    )
  }

  return (
    <>
    {emptyCount > 0 && (
      <div className="mb-2 flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 text-xs"
          onClick={() => setHideEmpty(!hideEmpty)}
        >
          <LayoutGrid className="h-3.5 w-3.5" />
          {hideEmpty ? `빈 컬럼 ${emptyCount}개 표시` : '빈 컬럼 숨기기'}
        </Button>
      </div>
    )}
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
    >
      <div
        ref={containerRef}
        className="flex snap-x snap-mandatory gap-4 overflow-x-auto scroll-smooth pb-4 sm:snap-none animate-fade-in focus:outline-none"
        tabIndex={0}
        onKeyDown={handleKeyDown}
      >
        {visibleColumns.map((col, ci) => (
          <DroppableColumn
            key={col.value}
            column={col}
            titleField={titleField}
            cardFields={cardFields}
            onCardClick={onCardClick}
            droppedId={droppedId}
            cancelledId={cancelledId}
            dropState={columnDropStates.get(col.value) ?? 'idle'}
            isDropTarget={overColumnValue === col.value}
            collapsed={collapsedColumns.has(col.value)}
            onToggleCollapse={() => toggleCollapse(col.value)}
            focusedCardId={focusedCol === ci ? focusedCardId : null}
          />
        ))}
      </div>

      <DragOverlay>
        {activeEntry && (
          <Card className="w-[220px] p-3 shadow-lg">
            <p className="text-sm font-medium">
              {titleField
                ? String(activeEntry[titleField.slug] || '제목 없음')
                : `#${String(activeEntry.id).slice(0, 8)}`}
            </p>
          </Card>
        )}
      </DragOverlay>
    </DndContext>
    </>
  )
}
