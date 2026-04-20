/**
 * GanttView — Timeline chart for entries with start/end date fields.
 *
 * Data is fetched server-side via useGanttView, which returns GanttRow[]
 * containing pre-computed date ranges and display metadata.
 *
 * Layout:
 * - Left panel: collapsible entry list with title and date summary.
 * - Right panel: SVG-based timeline with day-width columns and horizontal bars.
 * - Synchronized scrolling: vertical scroll is linked between the list and
 *   the timeline; horizontal scroll controls the visible date range.
 *
 * Key behaviors:
 * - Bar coloring: deterministic hue assignment by assignee or status field
 *   using a hash function over the field value string.
 * - Drag-to-resize: grab bar edges to adjust start/end dates, firing
 *   onEntryUpdate with the new date range.
 * - Responsive: left panel auto-collapses on small screens (<640px).
 * - Today marker: vertical red line indicating the current date.
 */
import { GanttChart, Loader2, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import EmptyState from '@/components/common/EmptyState'
import { useGanttView, type GanttRow } from '@/hooks/useEntries'
import type { Field } from '@/lib/types'

const DAY_WIDTH = 36
const ROW_HEIGHT = 36
const HEADER_HEIGHT = 52
const BAR_HEIGHT = 22
const BAR_Y_OFFSET = (ROW_HEIGHT - BAR_HEIGHT) / 2

function useResponsivePanel() {
  const [width, setWidth] = useState(
    typeof window !== 'undefined' ? window.innerWidth : 1280,
  )
  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  if (width < 640) return { panelWidth: 0, defaultCollapsed: true }
  if (width < 1024) return { panelWidth: 220, defaultCollapsed: false }
  return { panelWidth: 420, defaultCollapsed: false }
}

// Distinct hue palette for assignee/status color coding
const BAR_PALETTE = [
  'hsl(221 83% 53%)',  // blue
  'hsl(142 71% 45%)',  // green
  'hsl(262 83% 58%)',  // violet
  'hsl(25 95% 53%)',   // orange
  'hsl(339 90% 51%)',  // rose
  'hsl(174 72% 40%)',  // teal
  'hsl(47 96% 53%)',   // amber
  'hsl(199 89% 48%)',  // sky
  'hsl(280 67% 44%)',  // purple
  'hsl(12 76% 61%)',   // coral
]

function hashColor(key: string): string {
  let h = 0
  for (let i = 0; i < key.length; i++) h = ((h << 5) - h + key.charCodeAt(i)) | 0
  return BAR_PALETTE[Math.abs(h) % BAR_PALETTE.length]
}

interface Props {
  slug: string
  fields: Field[]
  filters?: Record<string, string>
  onEntryClick: (entryId: string) => void
  onEntryUpdate?: (entryId: string, updates: Record<string, unknown>) => void
}

function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(y, m - 1, d)
}

function formatDateStr(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}

function diffDays(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / (1000 * 60 * 60 * 24))
}

export default function GanttView({ slug, fields, filters, onEntryClick, onEntryUpdate }: Props) {
  const { panelWidth, defaultCollapsed } = useResponsivePanel()
  const [panelCollapsed, setPanelCollapsed] = useState(defaultCollapsed)
  const effectivePanelWidth = panelCollapsed ? 0 : panelWidth
  const isCompact = panelWidth <= 220

  // Detect date fields for drag updates
  const dateFields = useMemo(
    () => fields.filter((f) => f.field_type === 'date' || f.field_type === 'datetime'),
    [fields],
  )
  const startDateField = dateFields[0]
  const endDateField = dateFields.length >= 2 ? dateFields[1] : dateFields[0]

  const userField = useMemo(
    () => fields.find((f) => f.field_type === 'user'),
    [fields],
  )

  const progressField = useMemo(
    () =>
      fields.find(
        (f) =>
          (f.field_type === 'number' || f.field_type === 'integer') &&
          (f.options?.display_type === 'progress' ||
            f.slug.includes('progress') ||
            f.label.includes('진행')),
      ),
    [fields],
  )

  // Fetch gantt data from server
  const { data, isLoading } = useGanttView(slug, {
    startField: startDateField?.slug ?? '',
    endField: endDateField?.slug,
    filters,
  })

  const rows = useMemo(() => data?.rows ?? [], [data?.rows])
  const rangeStr = data?.range?.start
  const rangeStart = useMemo(
    () => (rangeStr ? parseDate(rangeStr) : addDays(new Date(), -7)),
    [rangeStr],
  )
  const totalDays = data?.range?.totalDays ?? 37
  const monthHeaders = useMemo(() => data?.months ?? [], [data?.months])

  // Generate day columns
  const days = useMemo(() => {
    const result: Date[] = []
    for (let i = 0; i < totalDays; i++) {
      result.push(addDays(rangeStart, i))
    }
    return result
  }, [rangeStart, totalDays])

  // Synchronized scrolling
  const leftBodyRef = useRef<HTMLDivElement>(null)
  const rightPanelRef = useRef<HTMLDivElement>(null)
  const scrollingRef = useRef<'left' | 'right' | null>(null)

  const handleRightScroll = useCallback(() => {
    if (scrollingRef.current === 'left') return
    scrollingRef.current = 'right'
    if (rightPanelRef.current && leftBodyRef.current) {
      leftBodyRef.current.scrollTop = rightPanelRef.current.scrollTop
    }
    requestAnimationFrame(() => {
      scrollingRef.current = null
    })
  }, [])

  const handleLeftScroll = useCallback(() => {
    if (scrollingRef.current === 'right') return
    scrollingRef.current = 'left'
    if (leftBodyRef.current && rightPanelRef.current) {
      rightPanelRef.current.scrollTop = leftBodyRef.current.scrollTop
    }
    requestAnimationFrame(() => {
      scrollingRef.current = null
    })
  }, [])

  // Drag state
  const [dragState, setDragState] = useState<{
    rowId: string
    type: 'move' | 'resize-start' | 'resize-end'
    startX: number
    origStart: string
    origEnd: string
    dayDelta: number
  } | null>(null)

  const handleMouseDown = useCallback(
    (
      e: React.MouseEvent,
      rowId: string,
      type: 'move' | 'resize-start' | 'resize-end',
      startDate: string,
      endDate: string,
    ) => {
      e.preventDefault()
      e.stopPropagation()
      setDragState({
        rowId,
        type,
        startX: e.clientX,
        origStart: startDate,
        origEnd: endDate,
        dayDelta: 0,
      })
    },
    [],
  )

  useEffect(() => {
    if (!dragState) return

    function handleMouseMove(e: MouseEvent) {
      if (!dragState) return
      const dx = e.clientX - dragState.startX
      const dayDelta = Math.round(dx / DAY_WIDTH)
      setDragState((prev) => (prev ? { ...prev, dayDelta } : null))
    }

    function handleMouseUp(e: MouseEvent) {
      if (!dragState) return
      const dx = e.clientX - dragState.startX
      const dayDelta = Math.round(dx / DAY_WIDTH)

      if (dayDelta !== 0 && onEntryUpdate && startDateField) {
        const origStart = parseDate(dragState.origStart)
        const origEnd = parseDate(dragState.origEnd)
        let newStart: Date
        let newEnd: Date

        switch (dragState.type) {
          case 'move':
            newStart = addDays(origStart, dayDelta)
            newEnd = addDays(origEnd, dayDelta)
            break
          case 'resize-start':
            newStart = addDays(origStart, dayDelta)
            newEnd = origEnd
            if (newStart > newEnd) newStart = newEnd
            break
          case 'resize-end':
            newStart = origStart
            newEnd = addDays(origEnd, dayDelta)
            if (newEnd < newStart) newEnd = newStart
            break
        }

        const updates: Record<string, unknown> = {
          [startDateField.slug]: formatDateStr(newStart!),
        }
        if (endDateField && endDateField.id !== startDateField.id) {
          updates[endDateField.slug] = formatDateStr(newEnd!)
        }
        onEntryUpdate(dragState.rowId, updates)
      }
      setDragState(null)
    }

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [dragState, onEntryUpdate, startDateField, endDateField])

  // Today indicator
  const todayStr = formatDateStr(new Date())
  const todayIdx = days.findIndex((d) => formatDateStr(d) === todayStr)

  // Build entry position map for dependencies
  const entryPositions = useMemo(() => {
    const map = new Map<string, { rowIdx: number; startIdx: number; endIdx: number }>()
    rows.forEach((row, rowIdx) => {
      if (!row.startDate || !row.endDate) return
      const startIdx = diffDays(rangeStart, parseDate(row.startDate))
      const endIdx = diffDays(rangeStart, parseDate(row.endDate))
      map.set(row.id, { rowIdx, startIdx, endIdx })
    })
    return map
  }, [rows, rangeStart])

  // Dependency lines
  const dependencies = useMemo(() => {
    const lines: { fromX: number; fromY: number; toX: number; toY: number }[] = []
    for (const row of rows) {
      if (row.dependencies.length === 0) continue
      const fromPos = entryPositions.get(row.id)
      if (!fromPos) continue
      for (const depId of row.dependencies) {
        const toPos = entryPositions.get(depId)
        if (!toPos) continue
        lines.push({
          fromX: (fromPos.endIdx + 1) * DAY_WIDTH,
          fromY: fromPos.rowIdx * ROW_HEIGHT + ROW_HEIGHT / 2,
          toX: toPos.startIdx * DAY_WIDTH,
          toY: toPos.rowIdx * ROW_HEIGHT + ROW_HEIGHT / 2,
        })
      }
    }
    return lines
  }, [rows, entryPositions])

  // Scroll to today on mount
  useEffect(() => {
    if (todayIdx >= 0 && rightPanelRef.current) {
      const scrollLeft = Math.max(0, todayIdx * DAY_WIDTH - rightPanelRef.current.clientWidth / 3)
      rightPanelRef.current.scrollLeft = scrollLeft
    }
  }, [todayIdx])

  const timelineWidth = totalDays * DAY_WIDTH
  const bodyHeight = rows.length * ROW_HEIGHT

  // Compute bar position with drag delta applied
  function getBarPosition(row: GanttRow) {
    if (!row.startDate || !row.endDate) return null
    let sDate = parseDate(row.startDate)
    let eDate = parseDate(row.endDate)

    if (dragState && dragState.rowId === row.id && dragState.dayDelta !== 0) {
      switch (dragState.type) {
        case 'move':
          sDate = addDays(sDate, dragState.dayDelta)
          eDate = addDays(eDate, dragState.dayDelta)
          break
        case 'resize-start':
          sDate = addDays(sDate, dragState.dayDelta)
          if (sDate > eDate) sDate = eDate
          break
        case 'resize-end':
          eDate = addDays(eDate, dragState.dayDelta)
          if (eDate < sDate) eDate = sDate
          break
      }
    }

    const startIdx = diffDays(rangeStart, sDate)
    const endIdx = diffDays(rangeStart, eDate)
    return {
      left: startIdx * DAY_WIDTH + 2,
      width: Math.max((endIdx - startIdx + 1) * DAY_WIDTH - 4, 8),
      startLabel: formatDateStr(sDate),
      endLabel: formatDateStr(eDate),
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center" style={{ height: 'calc(100vh - 320px)', minHeight: 400 }}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (rows.length === 0) {
    return (
      <EmptyState
        icon={<GanttChart className="h-10 w-10" />}
        title="간트 차트에 표시할 데이터가 없습니다"
        description="날짜 필드가 있는 데이터를 추가하면 타임라인에 표시됩니다."
      />
    )
  }

  return (
    <div
      className="flex rounded-md border overflow-hidden relative"
      style={{ height: 'calc(100vh - 320px)', minHeight: 400 }}
    >
      {/* Panel toggle button */}
      <Button
        variant="ghost"
        size="sm"
        className="absolute top-1 left-1 z-20 h-7 w-7 p-0"
        onClick={() => setPanelCollapsed((v) => !v)}
        title={panelCollapsed ? '목록 패널 열기' : '목록 패널 닫기'}
      >
        {panelCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
      </Button>

      {/* Left panel */}
      {!panelCollapsed && effectivePanelWidth > 0 && (
      <div
        className="flex flex-col flex-shrink-0 border-r transition-[width] duration-200"
        style={{ width: effectivePanelWidth }}
      >
        {/* Left header */}
        <div
          className="flex border-b bg-muted/30"
          style={{ height: HEADER_HEIGHT, minHeight: HEADER_HEIGHT }}
        >
          <div className="flex-1 px-3 pl-9 flex items-center text-xs font-medium">제목</div>
          {!isCompact && (
            <>
              <div className="w-[80px] px-2 flex items-center text-xs font-medium border-l">
                시작일
              </div>
              <div className="w-[80px] px-2 flex items-center text-xs font-medium border-l">
                종료일
              </div>
              {userField && (
                <div className="w-[64px] px-2 flex items-center text-xs font-medium border-l">
                  담당자
                </div>
              )}
              {progressField && (
                <div className="w-[56px] px-2 flex items-center text-xs font-medium border-l">
                  진행률
                </div>
              )}
            </>
          )}
        </div>
        {/* Left body */}
        <div
          ref={leftBodyRef}
          className="overflow-y-auto overflow-x-hidden flex-1"
          onScroll={handleLeftScroll}
          style={{ scrollbarWidth: 'none' }}
        >
          {rows.map((row) => (
            <div
              key={row.id}
              className="flex border-b hover:bg-muted/20 cursor-pointer"
              style={{ height: ROW_HEIGHT }}
              onClick={() => onEntryClick(row.id)}
            >
              <div className="flex-1 px-3 flex items-center text-sm truncate">
                {row.title}
              </div>
              {!isCompact && (
                <>
                  <div className="w-[80px] px-2 flex items-center text-xs text-muted-foreground border-l">
                    {row.startDate
                      ? `${parseDate(row.startDate).getMonth() + 1}/${parseDate(row.startDate).getDate()}`
                      : '-'}
                  </div>
                  <div className="w-[80px] px-2 flex items-center text-xs text-muted-foreground border-l">
                    {row.endDate
                      ? `${parseDate(row.endDate).getMonth() + 1}/${parseDate(row.endDate).getDate()}`
                      : '-'}
                  </div>
                  {userField && (
                    <div className="w-[64px] px-2 flex items-center text-xs text-muted-foreground border-l truncate">
                      {row.user || '-'}
                    </div>
                  )}
                  {progressField && (
                    <div className="w-[56px] px-2 flex items-center text-xs border-l">
                      {row.progress !== null ? (
                        <div className="flex items-center gap-1 w-full">
                          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-primary"
                              style={{
                                width: `${Math.min(100, Math.max(0, row.progress))}%`,
                              }}
                            />
                          </div>
                          <span className="text-[10px] text-muted-foreground">
                            {row.progress}%
                          </span>
                        </div>
                      ) : (
                        '-'
                      )}
                    </div>
                  )}
                </>
              )}
            </div>
          ))}
        </div>
      </div>
      )}

      {/* Right panel - timeline */}
      <div
        ref={rightPanelRef}
        className="flex-1 overflow-auto"
        onScroll={handleRightScroll}
      >
        <div style={{ width: timelineWidth, minWidth: '100%' }}>
          {/* Timeline header */}
          <div
            className="sticky top-0 z-10 bg-background border-b"
            style={{ height: HEADER_HEIGHT }}
          >
            {/* Month row */}
            <div className="flex" style={{ height: HEADER_HEIGHT / 2 }}>
              {monthHeaders.map((mh) => (
                <div
                  key={`${mh.label}-${mh.startIndex}`}
                  className="border-r border-b flex items-center justify-center text-xs font-medium"
                  style={{ width: mh.span * DAY_WIDTH }}
                >
                  {mh.label}
                </div>
              ))}
            </div>
            {/* Day row */}
            <div className="flex" style={{ height: HEADER_HEIGHT / 2 }}>
              {days.map((d, i) => {
                const dow = d.getDay()
                const isWeekend = dow === 0 || dow === 6
                const isT = formatDateStr(d) === todayStr
                return (
                  <div
                    key={i}
                    className={`border-r flex items-center justify-center text-[10px] ${
                      isT
                        ? 'bg-primary text-primary-foreground font-bold'
                        : isWeekend
                          ? 'text-muted-foreground bg-muted/30'
                          : ''
                    }`}
                    style={{ width: DAY_WIDTH }}
                  >
                    {d.getDate()}
                  </div>
                )
              })}
            </div>
          </div>

          {/* Timeline body */}
          <div className="relative" style={{ height: bodyHeight }}>
            {/* Weekend/today background columns */}
            {days.map((d, i) => {
              const dow = d.getDay()
              const isWeekend = dow === 0 || dow === 6
              const isT = formatDateStr(d) === todayStr
              if (!isWeekend && !isT) return null
              return (
                <div
                  key={i}
                  className={`absolute top-0 ${isT ? 'bg-primary/5' : 'bg-muted/20'}`}
                  style={{
                    left: i * DAY_WIDTH,
                    width: DAY_WIDTH,
                    height: bodyHeight,
                  }}
                />
              )
            })}

            {/* Row gridlines */}
            {rows.map((_, i) => (
              <div
                key={i}
                className="absolute w-full border-b"
                style={{ top: (i + 1) * ROW_HEIGHT - 1 }}
              />
            ))}

            {/* Today line */}
            {todayIdx >= 0 && (
              <div
                className="absolute top-0 w-[2px] bg-primary z-10"
                style={{
                  left: todayIdx * DAY_WIDTH + DAY_WIDTH / 2,
                  height: bodyHeight,
                }}
              />
            )}

            {/* Bars */}
            {rows.map((row, rowIdx) => {
              const pos = getBarPosition(row)
              if (!pos) return null
              const barTop = rowIdx * ROW_HEIGHT + BAR_Y_OFFSET
              const isDragging = dragState?.rowId === row.id
              const barColor = row.colorKey ? hashColor(row.colorKey) : BAR_PALETTE[0]

              return (
                <div
                  key={row.id}
                  className="absolute rounded group"
                  role="button"
                  tabIndex={0}
                  aria-label={`${row.title} ${pos.startLabel} ~ ${pos.endLabel}`}
                  style={{
                    left: pos.left,
                    top: barTop,
                    width: pos.width,
                    height: BAR_HEIGHT,
                    opacity: isDragging ? 0.8 : 1,
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      onEntryClick(row.id)
                    }
                    if (!onEntryUpdate || !row.startDate || !row.endDate) return
                    const shift = e.shiftKey ? 7 : 1
                    if (e.key === 'ArrowLeft') {
                      e.preventDefault()
                      const ns = formatDateStr(addDays(parseDate(row.startDate), -shift))
                      const ne = formatDateStr(addDays(parseDate(row.endDate), -shift))
                      onEntryUpdate(row.id, {
                        [startDateField!.slug]: ns,
                        ...(endDateField && endDateField.id !== startDateField!.id ? { [endDateField.slug]: ne } : {}),
                      })
                    } else if (e.key === 'ArrowRight') {
                      e.preventDefault()
                      const ns = formatDateStr(addDays(parseDate(row.startDate), shift))
                      const ne = formatDateStr(addDays(parseDate(row.endDate), shift))
                      onEntryUpdate(row.id, {
                        [startDateField!.slug]: ns,
                        ...(endDateField && endDateField.id !== startDateField!.id ? { [endDateField.slug]: ne } : {}),
                      })
                    }
                  }}
                >
                  {/* Bar background */}
                  <div
                    className="absolute inset-0 rounded cursor-grab active:cursor-grabbing"
                    style={{ backgroundColor: `color-mix(in srgb, ${barColor} 25%, transparent)` }}
                    onMouseDown={(e) =>
                      onEntryUpdate &&
                      row.startDate &&
                      row.endDate &&
                      handleMouseDown(e, row.id, 'move', row.startDate, row.endDate)
                    }
                    onClick={(e) => {
                      e.stopPropagation()
                      if (!dragState) onEntryClick(row.id)
                    }}
                  >
                    {/* Progress fill */}
                    {row.progress !== null && row.progress > 0 && (
                      <div
                        className={`absolute inset-y-0 left-0 ${
                          row.progress >= 100 ? 'rounded' : 'rounded-l'
                        }`}
                        style={{
                          width: `${Math.min(100, row.progress)}%`,
                          backgroundColor: `color-mix(in srgb, ${barColor} 55%, transparent)`,
                        }}
                      />
                    )}
                    {/* Bar label */}
                    <span className="absolute inset-0 flex items-center px-2 text-xs truncate font-medium">
                      {row.title}
                    </span>
                  </div>

                  {/* Drag date tooltip */}
                  {isDragging && dragState.dayDelta !== 0 && (
                    <div className="absolute -top-7 left-1/2 -translate-x-1/2 whitespace-nowrap rounded bg-foreground px-2 py-0.5 text-[10px] text-background shadow z-20">
                      {pos.startLabel} ~ {pos.endLabel}
                    </div>
                  )}

                  {/* Resize handles */}
                  {onEntryUpdate && row.startDate && row.endDate && (
                    <>
                      <div
                        className="absolute left-0 top-0 w-2 h-full cursor-col-resize opacity-0 group-hover:opacity-100 rounded-l"
                        style={{ backgroundColor: `color-mix(in srgb, ${barColor} 45%, transparent)` }}
                        onMouseDown={(e) =>
                          handleMouseDown(
                            e,
                            row.id,
                            'resize-start',
                            row.startDate!,
                            row.endDate!,
                          )
                        }
                      />
                      <div
                        className="absolute right-0 top-0 w-2 h-full cursor-col-resize opacity-0 group-hover:opacity-100 rounded-r"
                        style={{ backgroundColor: `color-mix(in srgb, ${barColor} 45%, transparent)` }}
                        onMouseDown={(e) =>
                          handleMouseDown(
                            e,
                            row.id,
                            'resize-end',
                            row.startDate!,
                            row.endDate!,
                          )
                        }
                      />
                    </>
                  )}
                </div>
              )
            })}

            {/* Dependency arrows (SVG overlay) */}
            {dependencies.length > 0 && (
              <svg
                className="absolute top-0 left-0 pointer-events-none"
                style={{ width: timelineWidth, height: bodyHeight }}
              >
                <defs>
                  <marker
                    id="gantt-arrow"
                    markerWidth="6"
                    markerHeight="6"
                    refX="6"
                    refY="3"
                    orient="auto"
                  >
                    <path
                      d="M0,0 L6,3 L0,6 Z"
                      fill="currentColor"
                      className="text-foreground/60"
                    />
                  </marker>
                </defs>
                {dependencies.map((dep, i) => {
                  const midX = (dep.fromX + dep.toX) / 2
                  return (
                    <path
                      key={i}
                      d={`M${dep.fromX},${dep.fromY} C${midX},${dep.fromY} ${midX},${dep.toY} ${dep.toX},${dep.toY}`}
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.5"
                      className="text-foreground/60"
                      markerEnd="url(#gantt-arrow)"
                    />
                  )
                })}
              </svg>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
