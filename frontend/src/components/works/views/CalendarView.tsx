/**
 * CalendarView — Month/week/day calendar displaying entries by a date field.
 *
 * Data is fetched server-side via useCalendarView, which returns entries
 * grouped by the visible date range. Supports three view modes:
 * - Month: grid of weeks; multi-day events span across cells (colSpan).
 * - Week: delegated to CalendarWeekView sub-component.
 * - Day: delegated to CalendarDayView sub-component.
 *
 * Key behaviors:
 * - Drag-and-drop (@dnd-kit) to move entries between dates, updating
 *   the date field value via onEntryUpdate callback.
 * - Month navigation with direction-based slide animations (left/right).
 * - Click on an empty date cell to create a new entry pre-filled with that date.
 * - Color coding: entries are colored by a select field value using a
 *   rotating palette of 8 distinct colors.
 */
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { Calendar, ChevronLeft, ChevronRight } from 'lucide-react'
import { useCallback, useMemo, useRef, useState } from 'react'

import EmptyState from '@/components/common/EmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { useCalendarView } from '@/hooks/useEntries'
import type { Field } from '@/lib/types'

import CalendarWeekView from './CalendarWeekView'
import CalendarDayView from './CalendarDayView'

interface Props {
  slug: string
  dateField: Field
  fields: Field[]
  filters?: Record<string, string>
  onEntryClick: (entry: Record<string, unknown>) => void
  onEntryUpdate?: (entryId: string, updates: Record<string, unknown>) => void
  onCreateEntry?: (prefill: Record<string, unknown>) => void
}

type ViewMode = 'month' | 'week' | 'day'
type Direction = 'left' | 'right' | null

/** YYYY-MM-DD from any date-ish value */
function toDateStr(v: unknown): string | null {
  if (!v) return null
  const s = String(v).slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return null
  return s
}

function makeDateStr(year: number, month: number, day: number) {
  return `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`
}

function addDays(dateStr: string, n: number): string {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + n)
  return makeDateStr(d.getFullYear(), d.getMonth(), d.getDate())
}

function diffDays(a: string, b: string): number {
  const da = new Date(a + 'T00:00:00')
  const db = new Date(b + 'T00:00:00')
  return Math.round((db.getTime() - da.getTime()) / (1000 * 60 * 60 * 24))
}

const DAY_NAMES = ['일', '월', '화', '수', '목', '금', '토']
const MAX_VISIBLE_SPANS = 3

// Color palette for select field value color coding
const COLOR_CLASSES = [
  'bg-blue-100 border-l-blue-500',
  'bg-green-100 border-l-green-500',
  'bg-amber-100 border-l-amber-500',
  'bg-red-100 border-l-red-500',
  'bg-purple-100 border-l-purple-500',
  'bg-teal-100 border-l-teal-500',
  'bg-pink-100 border-l-pink-500',
  'bg-indigo-100 border-l-indigo-500',
]

export default function CalendarView({
  slug,
  dateField,
  fields,
  filters,
  onEntryClick,
  onEntryUpdate,
  onCreateEntry,
}: Props) {
  const [viewDate, setViewDate] = useState(() => new Date())
  const [viewMode, setViewMode] = useState<ViewMode>('month')
  const [direction, setDirection] = useState<Direction>(null)
  const [animating, setAnimating] = useState(false)
  const gridRef = useRef<HTMLDivElement>(null)

  const year = viewDate.getFullYear()
  const month = viewDate.getMonth()

  // Drag state
  const [dragEntry, setDragEntry] = useState<Record<string, unknown> | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  )

  const titleField = useMemo(
    () => fields.find((f) => f.field_type === 'text'),
    [fields],
  )

  const endDateField = useMemo(() => {
    const dateFields = fields.filter(
      (f) => (f.field_type === 'date' || f.field_type === 'datetime') && f.id !== dateField.id,
    )
    return dateFields[0] ?? null
  }, [fields, dateField.id])

  // Color coding: use first select field
  const colorField = useMemo(
    () => fields.find((f) => f.field_type === 'select' && f.slug !== '_status') ?? null,
    [fields],
  )

  const getLabel = useCallback(
    (entry: Record<string, unknown>) => {
      if (titleField) {
        const v = entry[titleField.slug]
        return v ? String(v) : '(무제)'
      }
      return String(entry.id ?? '').slice(0, 8) || '(무제)'
    },
    [titleField],
  )

  // Fetch server-computed calendar data
  const { data: calendarData, isLoading } = useCalendarView(slug, {
    year,
    month: month + 1,
    dateField: dateField.slug,
    endDateField: endDateField?.slug,
    filters,
  })

  const weeks = calendarData?.weeks ?? []
  const monthHasEvents = weeks.some(
    (w) => (w.spans?.length ?? 0) > 0 || Object.keys(w.singles ?? {}).length > 0,
  )

  const today = new Date()
  const isToday = (d: number) =>
    d === today.getDate() && month === today.getMonth() && year === today.getFullYear()

  // Build color map from entries
  const colorMap = useMemo(() => {
    if (!colorField) return new Map<string, string>()
    const values = new Set<string>()
    for (const w of weeks) {
      for (const span of w.spans ?? []) {
        const v = span.entry[colorField.slug]
        if (v) values.add(String(v))
      }
      for (const entries of Object.values(w.singles ?? {})) {
        for (const e of entries) {
          const v = e[colorField.slug]
          if (v) values.add(String(v))
        }
      }
    }
    const map = new Map<string, string>()
    let i = 0
    for (const v of values) {
      map.set(v, COLOR_CLASSES[i % COLOR_CLASSES.length])
      i++
    }
    return map
  }, [colorField, weeks])

  function getEntryColor(entry: Record<string, unknown>): string {
    if (!colorField) return ''
    const val = String(entry[colorField.slug] ?? '')
    return colorMap.get(val) ?? ''
  }

  function navigate(dir: 'left' | 'right') {
    if (animating && viewMode === 'month') return
    if (viewMode === 'month') {
      setDirection(dir)
      setAnimating(true)
    } else if (viewMode === 'week') {
      setViewDate((prev) => {
        const d = new Date(prev)
        d.setDate(d.getDate() + (dir === 'left' ? -7 : 7))
        return d
      })
    } else {
      setViewDate((prev) => {
        const d = new Date(prev)
        d.setDate(d.getDate() + (dir === 'left' ? -1 : 1))
        return d
      })
    }
  }

  function handleAnimationEnd() {
    if (!direction) return
    setViewDate((prev) => {
      const y = prev.getFullYear()
      const m = prev.getMonth()
      return direction === 'left' ? new Date(y, m - 1, 1) : new Date(y, m + 1, 1)
    })
    setDirection(null)
    setAnimating(false)
  }

  function goToday() {
    if (animating) return
    setViewDate(new Date())
    setDirection(null)
  }

  // Build a flat lookup of all entries from server data for drag handling
  const allEntries = useMemo(() => {
    if (!calendarData) return new Map<string, Record<string, unknown>>()
    const map = new Map<string, Record<string, unknown>>()
    for (const w of calendarData.weeks) {
      for (const span of w.spans ?? []) {
        map.set(String(span.entry.id), span.entry)
      }
      for (const entries of Object.values(w.singles ?? {})) {
        for (const e of entries) {
          map.set(String(e.id), e)
        }
      }
    }
    return map
  }, [calendarData])

  // Drag handlers
  function handleDragStart(event: DragStartEvent) {
    const id = String(event.active.id)
    const entry = allEntries.get(id)
    if (entry) setDragEntry(entry)
  }

  function handleDragEnd(event: DragEndEvent) {
    setDragEntry(null)
    if (!onEntryUpdate || !event.over) return

    const entryId = String(event.active.id)
    const targetDate = String(event.over.id)
    const entry = allEntries.get(entryId)
    if (!entry) return

    const currentDate = toDateStr(entry[dateField.slug])
    if (!currentDate || currentDate === targetDate.slice(0, 10)) return

    const dayDelta = diffDays(currentDate, targetDate.slice(0, 10))
    const updates: Record<string, unknown> = { [dateField.slug]: targetDate.includes('T') ? targetDate : targetDate }

    if (endDateField) {
      const currentEnd = toDateStr(entry[endDateField.slug])
      if (currentEnd) {
        updates[endDateField.slug] = addDays(currentEnd, dayDelta)
      }
    }

    onEntryUpdate(entryId, updates)
  }

  // Header label
  const headerLabel = useMemo(() => {
    if (viewMode === 'month') return `${year}년 ${month + 1}월`
    if (viewMode === 'week') {
      const d = new Date(viewDate)
      const day = d.getDay()
      d.setDate(d.getDate() - day)
      const weekStart = new Date(d)
      d.setDate(d.getDate() + 6)
      const weekEnd = d
      if (weekStart.getMonth() === weekEnd.getMonth()) {
        return `${weekStart.getFullYear()}년 ${weekStart.getMonth() + 1}월 ${weekStart.getDate()}일 - ${weekEnd.getDate()}일`
      }
      return `${weekStart.getMonth() + 1}/${weekStart.getDate()} - ${weekEnd.getMonth() + 1}/${weekEnd.getDate()}`
    }
    return `${year}년 ${month + 1}월 ${viewDate.getDate()}일`
  }, [viewMode, year, month, viewDate])

  if (!isLoading && calendarData && !monthHasEvents && weeks.length === 0) {
    return (
      <EmptyState
        icon={<Calendar className="h-10 w-10" />}
        title="캘린더에 표시할 항목이 없습니다"
        description="날짜 필드가 있는 항목을 추가하면 캘린더에 표시됩니다."
      />
    )
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="space-y-3">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => navigate('left')}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <h3 className="text-lg font-semibold min-w-[180px] text-center">
              {headerLabel}
            </h3>
            <Button variant="outline" size="sm" onClick={() => navigate('right')}>
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex rounded-lg border p-0.5 gap-0.5">
              {(['month', 'week', 'day'] as const).map((mode) => (
                <button
                  key={mode}
                  type="button"
                  className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
                    viewMode === mode
                      ? 'bg-primary text-primary-foreground'
                      : 'hover:bg-muted'
                  }`}
                  onClick={() => setViewMode(mode)}
                >
                  {mode === 'month' ? '월' : mode === 'week' ? '주' : '일'}
                </button>
              ))}
            </div>
            <Button variant="outline" size="sm" onClick={goToday}>
              오늘
            </Button>
          </div>
        </div>

        {/* Loading */}
        {isLoading && (
          <div className="space-y-1 p-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-[100px] w-full" />
            ))}
          </div>
        )}

        {/* View content */}
        {!isLoading && viewMode === 'week' && (
          <CalendarWeekView
            weeks={weeks}
            viewDate={viewDate}
            titleField={titleField}
            dateField={dateField}
            endDateField={endDateField}
            onEntryClick={onEntryClick}
            onEntryUpdate={onEntryUpdate}
            onCreateEntry={onCreateEntry}
            getLabel={getLabel}
            colorMap={colorMap}
            colorField={colorField}
          />
        )}

        {!isLoading && viewMode === 'day' && (
          <CalendarDayView
            weeks={weeks}
            viewDate={viewDate}
            titleField={titleField}
            dateField={dateField}
            endDateField={endDateField}
            onEntryClick={onEntryClick}
            onEntryUpdate={onEntryUpdate}
            onCreateEntry={onCreateEntry}
            getLabel={getLabel}
            colorMap={colorMap}
            colorField={colorField}
          />
        )}

        {!isLoading && viewMode === 'month' && (
          <div className="overflow-hidden rounded-md border">
            {/* Day names header */}
            <div className="grid grid-cols-7">
              {DAY_NAMES.map((name, i) => (
                <div
                  key={name}
                  className={`border-b px-2 py-1.5 text-center text-xs font-medium ${
                    i === 0 ? 'text-red-500' : i === 6 ? 'text-blue-500' : 'text-muted-foreground'
                  }`}
                >
                  {name}
                </div>
              ))}
            </div>

            {/* Animated grid */}
            <div
              ref={gridRef}
              className={
                direction === 'left'
                  ? 'animate-slide-right'
                  : direction === 'right'
                    ? 'animate-slide-left'
                    : ''
              }
              onAnimationEnd={handleAnimationEnd}
            >
              {weeks.map((week, wi) => {
                const weekSpans = week.spans ?? []
                const maxTrack = weekSpans.length > 0
                  ? Math.min(Math.max(...weekSpans.map((s) => s.track)) + 1, MAX_VISIBLE_SPANS)
                  : 0
                const hiddenSpanCount = weekSpans.filter((s) => s.track >= MAX_VISIBLE_SPANS).length

                return (
                  <div key={wi} className="grid grid-cols-7 relative">
                    {/* Spanning bars layer */}
                    {weekSpans
                      .filter((s) => s.track < MAX_VISIBLE_SPANS)
                      .map((span) => {
                        const leftPct = (span.startCol / 7) * 100
                        const widthPct = (span.colSpan / 7) * 100
                        const top = 24 + span.track * 22
                        const colorCls = getEntryColor(span.entry)

                        return (
                          <button
                            key={`${String(span.entry.id)}-${wi}`}
                            type="button"
                            className={`absolute z-10 truncate text-xs px-1.5 py-0.5 hover:bg-primary/25 cursor-pointer border-l-2 ${
                              colorCls || 'bg-primary/15 border-primary'
                            } ${span.isStart ? 'rounded-l' : ''} ${span.isEnd ? 'rounded-r' : ''}`}
                            style={{
                              left: `calc(${leftPct}% + 2px)`,
                              width: `calc(${widthPct}% - 4px)`,
                              top: `${top}px`,
                              height: '20px',
                              lineHeight: '18px',
                            }}
                            onClick={() => onEntryClick(span.entry)}
                          >
                            {span.isStart ? span.label : `… ${span.label}`}
                          </button>
                        )
                      })}

                    {/* Day cells */}
                    {week.days.map((dateStr, di) => {
                      if (!dateStr) {
                        return (
                          <DroppableCell key={di} id={`empty-${wi}-${di}`} disabled>
                            <div className="min-h-[100px] bg-muted/20" />
                          </DroppableCell>
                        )
                      }

                      const dayNum = parseInt(dateStr.slice(8, 10), 10)
                      const dayEntries = week.singles?.[dateStr] ?? []
                      const spanOffset = maxTrack * 22

                      return (
                        <DroppableCell key={di} id={dateStr}>
                          <div
                            className={`min-h-[100px] border-b border-r p-1 ${
                              isToday(dayNum) ? 'bg-primary/5' : ''
                            }`}
                            onClick={(e) => {
                              if (onCreateEntry && e.target === e.currentTarget) {
                                onCreateEntry({ [dateField.slug]: dateStr })
                              }
                            }}
                          >
                            <div
                              className={`mb-1 text-xs font-medium ${
                                di === 0 ? 'text-red-500' : di === 6 ? 'text-blue-500' : ''
                              } ${
                                isToday(dayNum)
                                  ? 'rounded-full bg-primary text-primary-foreground w-5 h-5 flex items-center justify-center'
                                  : ''
                              }`}
                            >
                              {dayNum}
                            </div>
                            <div className="space-y-0.5" style={{ marginTop: `${spanOffset}px` }}>
                              {dayEntries.slice(0, 3).map((entry) => {
                                const colorCls = getEntryColor(entry)
                                return (
                                  <DraggableEntry
                                    key={String(entry.id)}
                                    entry={entry}
                                    label={getLabel(entry)}
                                    onClick={() => onEntryClick(entry)}
                                    draggable={!!onEntryUpdate}
                                    colorClass={colorCls}
                                  />
                                )
                              })}
                              {dayEntries.length > 3 && (
                                <span className="text-[10px] text-muted-foreground pl-1">
                                  +{dayEntries.length - 3} 더
                                </span>
                              )}
                              {di === 6 && hiddenSpanCount > 0 && (
                                <span className="text-[10px] text-muted-foreground pl-1">
                                  +{hiddenSpanCount} 더
                                </span>
                              )}
                            </div>
                          </div>
                        </DroppableCell>
                      )
                    })}
                  </div>
                )
              })}
            </div>

            {/* Empty month message */}
            {!monthHasEvents && weeks.length > 0 && (
              <div className="flex flex-col items-center justify-center py-10 text-muted-foreground text-sm">
                <Calendar className="h-8 w-8 mb-2 opacity-40" />
                <p>이번 달에 해당하는 일정이 없습니다</p>
                <Button variant="link" size="sm" className="mt-1" onClick={goToday}>
                  오늘로 이동
                </Button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Drag overlay */}
      <DragOverlay>
        {dragEntry && (
          <div className="rounded bg-primary/20 px-2 py-1 text-xs shadow-md border max-w-[160px] truncate">
            {getLabel(dragEntry)}
          </div>
        )}
      </DragOverlay>
    </DndContext>
  )
}

// --- Sub-components ---

import { useDroppable } from '@dnd-kit/core'
import { useDraggable } from '@dnd-kit/core'

function DroppableCell({
  id,
  disabled,
  children,
}: {
  id: string
  disabled?: boolean
  children: React.ReactNode
}) {
  const { setNodeRef, isOver } = useDroppable({ id, disabled })

  return (
    <div
      ref={setNodeRef}
      className={isOver ? 'ring-2 ring-primary/30 ring-inset' : ''}
    >
      {children}
    </div>
  )
}

function DraggableEntry({
  entry,
  label,
  onClick,
  draggable,
  colorClass,
}: {
  entry: Record<string, unknown>
  label: string
  onClick: () => void
  draggable: boolean
  colorClass?: string
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: String(entry.id),
    disabled: !draggable,
  })

  return (
    <button
      ref={setNodeRef}
      type="button"
      className={`block w-full truncate rounded px-1 py-0.5 text-left text-xs hover:bg-primary/20 ${
        colorClass || 'bg-primary/10'
      } ${colorClass ? 'border-l-2' : ''} ${
        isDragging ? 'opacity-30' : ''
      } ${draggable ? 'cursor-grab active:cursor-grabbing' : ''}`}
      onClick={onClick}
      {...(draggable ? { ...listeners, ...attributes } : {})}
    >
      {label}
    </button>
  )
}
