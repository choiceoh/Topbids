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
import { useMemo, useState } from 'react'

import { GripVertical } from 'lucide-react'

import { FIELD_TYPE_LABELS, isLayoutType } from '@/lib/constants'
import type { FieldType } from '@/lib/types'

import ConfirmDialog from '@/components/common/ConfirmDialog'

export interface FieldDraft {
  id: string // client-local id
  slug: string
  label: string
  field_type: FieldType
  is_required: boolean
  is_unique: boolean
  is_indexed: boolean
  default_value?: string
  description?: string
  width: number
  height: number
  options?: Record<string, unknown>
  relation?: {
    target_collection_id: string
    relation_type: 'one_to_one' | 'one_to_many' | 'many_to_many'
    on_delete: string
  }
}

interface Props {
  fields: FieldDraft[]
  selectedId: string | null
  onSelect: (id: string) => void
  onReorder: (fields: FieldDraft[]) => void
  onRemove: (id: string) => void
}

export default function FieldPreview({ fields, selectedId, onSelect, onReorder, onRemove }: Props) {
  const [removeTargetId, setRemoveTargetId] = useState<string | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)

  const fieldIds = useMemo(() => fields.map((f) => f.id), [fields])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  )

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
    updated.splice(newIndex, 0, moved)
    onReorder(updated)
  }

  if (fields.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border-2 border-dashed text-sm text-muted-foreground">
        왼쪽에서 항목을 추가하세요
      </div>
    )
  }

  const colSpanClass: Record<number, string> = {
    1: 'col-span-1',
    2: 'col-span-2',
    3: 'col-span-3',
    6: 'col-span-6',
  }

  const rowSpanClass: Record<number, string> = {
    1: '',
    2: 'row-span-2 min-h-24',
    3: 'row-span-3 min-h-36',
  }

  const activeField = activeId ? fields.find((f) => f.id === activeId) : null

  return (
    <div>
      <h3 className="mb-2 text-sm font-medium text-muted-foreground">항목 목록</h3>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
      >
        <SortableContext items={fieldIds} strategy={verticalListSortingStrategy}>
          <div className="grid grid-cols-6 gap-2">
            {fields.map((field) => (
              <SortableFieldBlock
                key={field.id}
                field={field}
                isSelected={selectedId === field.id}
                colSpanClass={colSpanClass}
                rowSpanClass={rowSpanClass}
                onSelect={onSelect}
                onRemove={setRemoveTargetId}
              />
            ))}
          </div>
        </SortableContext>
        <DragOverlay dropAnimation={{ duration: 200, easing: 'ease' }}>
          {activeField ? (
            <div className="rounded-md border bg-background p-3 shadow-lg opacity-90">
              <span className="text-sm font-medium">{activeField.label || '(제목 없음)'}</span>
              <span className="ml-2 text-xs text-muted-foreground">
                {FIELD_TYPE_LABELS[activeField.field_type]}
              </span>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      <ConfirmDialog
        open={!!removeTargetId}
        onOpenChange={(open) => !open && setRemoveTargetId(null)}
        title="항목을 제거하시겠습니까?"
        description="이 항목이 목록에서 제거됩니다."
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

/* ─── Sortable field block ─── */

interface SortableFieldBlockProps {
  field: FieldDraft
  isSelected: boolean
  colSpanClass: Record<number, string>
  rowSpanClass: Record<number, string>
  onSelect: (id: string) => void
  onRemove: (id: string) => void
}

function SortableFieldBlock({
  field, isSelected, colSpanClass, rowSpanClass, onSelect, onRemove,
}: SortableFieldBlockProps) {
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

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`${colSpanClass[field.width] ?? 'col-span-6'} ${rowSpanClass[field.height] ?? ''} cursor-pointer rounded-md border p-3 transition-colors ${
        isSelected ? 'border-primary bg-accent' : 'hover:bg-accent/50'
      } ${isLayoutType(field.field_type) ? 'border-dashed opacity-75' : ''
      } ${isDragging ? 'z-10 opacity-40' : ''}`}
      onClick={() => onSelect(field.id)}
    >
      <div className="flex items-center justify-between gap-1">
        <div className="flex min-w-0 items-center gap-1">
          <div
            className="flex shrink-0 cursor-grab items-center text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="h-4 w-4" />
          </div>
          <span className="truncate text-sm font-medium">{field.label || '(제목 없음)'}</span>
          {field.is_required && <span className="shrink-0 text-xs text-destructive">*</span>}
          {field.is_unique && <span className="shrink-0 text-xs text-muted-foreground">UNIQUE</span>}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <span className="text-xs text-muted-foreground">
            {FIELD_TYPE_LABELS[field.field_type]}
          </span>
          <button
            onClick={(e) => { e.stopPropagation(); onRemove(field.id) }}
            className="text-xs text-muted-foreground hover:text-destructive"
            type="button"
          >
            ×
          </button>
        </div>
      </div>
      {field.slug && (
        <p className="mt-1 truncate text-xs text-muted-foreground">ID: {field.slug}</p>
      )}
    </div>
  )
}
