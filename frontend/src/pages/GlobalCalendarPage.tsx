import { Calendar, ChevronLeft, ChevronRight } from 'lucide-react'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'

import LoadingState from '@/components/common/LoadingState'
import ErrorState from '@/components/common/ErrorState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useGlobalCalendarEvents, type GlobalCalendarEvent } from '@/hooks/useEntries'

// Palette for distinguishing collections on the calendar.
const COLORS = [
  { bg: 'bg-blue-500/15', border: 'border-blue-500', text: 'text-blue-700', badge: 'bg-blue-100 text-blue-700' },
  { bg: 'bg-emerald-500/15', border: 'border-emerald-500', text: 'text-emerald-700', badge: 'bg-emerald-100 text-emerald-700' },
  { bg: 'bg-amber-500/15', border: 'border-amber-500', text: 'text-amber-700', badge: 'bg-amber-100 text-amber-700' },
  { bg: 'bg-rose-500/15', border: 'border-rose-500', text: 'text-rose-700', badge: 'bg-rose-100 text-rose-700' },
  { bg: 'bg-violet-500/15', border: 'border-violet-500', text: 'text-violet-700', badge: 'bg-violet-100 text-violet-700' },
  { bg: 'bg-cyan-500/15', border: 'border-cyan-500', text: 'text-cyan-700', badge: 'bg-cyan-100 text-cyan-700' },
  { bg: 'bg-orange-500/15', border: 'border-orange-500', text: 'text-orange-700', badge: 'bg-orange-100 text-orange-700' },
  { bg: 'bg-pink-500/15', border: 'border-pink-500', text: 'text-pink-700', badge: 'bg-pink-100 text-pink-700' },
]

interface CalendarEvent extends GlobalCalendarEvent {
  colorIndex: number
}

const DAY_NAMES = ['일', '월', '화', '수', '목', '금', '토']

function makeDateStr(year: number, month: number, day: number) {
  return `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`
}

type Direction = 'left' | 'right' | null

export default function GlobalCalendarPage() {
  const navigate = useNavigate()
  const [viewDate, setViewDate] = useState(() => new Date())
  const [direction, setDirection] = useState<Direction>(null)
  const [animating, setAnimating] = useState(false)
  const gridRef = useRef<HTMLDivElement>(null)

  const year = viewDate.getFullYear()
  const month = viewDate.getMonth()

  const { data: rawEvents, isLoading, isError, error } = useGlobalCalendarEvents(year, month + 1)

  // Assign a stable color index per collection.
  const events: CalendarEvent[] = useMemo(() => {
    if (!rawEvents) return []
    const colorMap = new Map<string, number>()
    let nextColor = 0
    return rawEvents.map((ev) => {
      let colorIndex = colorMap.get(ev.collectionId)
      if (colorIndex === undefined) {
        colorIndex = nextColor++ % COLORS.length
        colorMap.set(ev.collectionId, colorIndex)
      }
      return { ...ev, colorIndex }
    })
  }, [rawEvents])

  // Calendar grid.
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const firstDayOfMonth = new Date(year, month, 1)
  const startDay = firstDayOfMonth.getDay()

  const calendarDays: (number | null)[] = useMemo(() => {
    const days: (number | null)[] = []
    for (let i = 0; i < startDay; i++) days.push(null)
    for (let d = 1; d <= daysInMonth; d++) days.push(d)
    while (days.length % 7 !== 0) days.push(null)
    return days
  }, [startDay, daysInMonth])

  const weeks = useMemo(() => {
    const w: (number | null)[][] = []
    for (let i = 0; i < calendarDays.length; i += 7) {
      w.push(calendarDays.slice(i, i + 7))
    }
    return w
  }, [calendarDays])

  // Events grouped by date for single-day events.
  const eventsByDate = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>()
    for (const ev of events) {
      if (ev.endDate) continue // multi-day handled separately
      const existing = map.get(ev.date) ?? []
      existing.push(ev)
      map.set(ev.date, existing)
    }
    return map
  }, [events])

  // Multi-day spanning events per week.
  const spansByWeek = useMemo(() => {
    const multiDay = events.filter((ev) => ev.endDate)
    const result: { event: CalendarEvent; startCol: number; colSpan: number; track: number }[][] =
      weeks.map(() => [])

    for (const ev of multiDay) {
      for (let wi = 0; wi < weeks.length; wi++) {
        const week = weeks[wi]
        const weekDates: (string | null)[] = week.map((day) =>
          day !== null ? makeDateStr(year, month, day) : null,
        )

        let weekStart: string | null = null
        let weekEnd: string | null = null
        let weekStartCol = 0
        let weekEndCol = 6
        for (let di = 0; di < 7; di++) {
          if (weekDates[di]) {
            if (!weekStart) { weekStart = weekDates[di]!; weekStartCol = di }
            weekEnd = weekDates[di]!; weekEndCol = di
          }
        }
        if (!weekStart || !weekEnd) continue
        if (ev.date > weekEnd || (ev.endDate ?? ev.date) < weekStart) continue

        const clampedStart = ev.date > weekStart ? ev.date : weekStart
        const clampedEnd = (ev.endDate ?? ev.date) < weekEnd ? (ev.endDate ?? ev.date) : weekEnd

        let startCol = weekStartCol
        let endCol = weekEndCol
        for (let di = 0; di < 7; di++) {
          if (weekDates[di] === clampedStart) startCol = di
          if (weekDates[di] === clampedEnd) endCol = di
        }

        result[wi].push({
          event: ev,
          startCol,
          colSpan: endCol - startCol + 1,
          track: 0,
        })
      }
    }

    // Assign tracks.
    for (const spans of result) {
      spans.sort((a, b) => a.startCol - b.startCol || b.colSpan - a.colSpan)
      const trackEnds: number[] = []
      for (const span of spans) {
        let assigned = -1
        for (let t = 0; t < trackEnds.length; t++) {
          if (trackEnds[t] < span.startCol) { assigned = t; break }
        }
        if (assigned === -1) { assigned = trackEnds.length; trackEnds.push(0) }
        span.track = assigned
        trackEnds[assigned] = span.startCol + span.colSpan - 1
      }
    }

    return result
  }, [events, weeks, year, month])

  const today = new Date()
  const isToday = (d: number) =>
    d === today.getDate() && month === today.getMonth() && year === today.getFullYear()

  const handleEventClick = useCallback(
    (ev: CalendarEvent) => {
      navigate(`/apps/${ev.collectionId}`)
    },
    [navigate],
  )

  const goMonth = useCallback((dir: -1 | 1) => {
    if (animating) return
    setDirection(dir === -1 ? 'left' : 'right')
    setAnimating(true)
  }, [animating])

  const handleAnimationEnd = useCallback(() => {
    if (!direction) return
    setViewDate((prev) => {
      const y = prev.getFullYear()
      const m = prev.getMonth()
      return direction === 'left' ? new Date(y, m - 1, 1) : new Date(y, m + 1, 1)
    })
    setDirection(null)
    setAnimating(false)
  }, [direction])

  const goToday = useCallback(() => {
    if (animating) return
    setViewDate(new Date())
    setDirection(null)
  }, [animating])

  // Active collection legend (only those with events).
  const activeCollections = useMemo(() => {
    const seen = new Map<string, { id: string; label: string; icon?: string; colorIndex: number }>()
    for (const ev of events) {
      if (seen.has(ev.collectionId)) continue
      seen.set(ev.collectionId, {
        id: ev.collectionId,
        label: ev.collectionLabel,
        icon: ev.collectionIcon,
        colorIndex: ev.colorIndex,
      })
    }
    return [...seen.values()]
  }, [events])

  if (isLoading) return <LoadingState variant="summary" />
  if (isError) return <ErrorState error={error} />

  if (events.length === 0 && !isLoading) {
    return (
      <div>
        <PageHeader title="캘린더" description="전체 앱의 일정을 한눈에 확인합니다" />
        <div className="py-16 text-center text-muted-foreground">
          <Calendar className="mx-auto mb-3 h-10 w-10" />
          <p>날짜 필드가 있는 앱이 없습니다</p>
          <p className="text-sm">앱에 날짜 필드를 추가하면 여기에 일정이 표시됩니다.</p>
        </div>
      </div>
    )
  }

  const MAX_VISIBLE_SPANS = 3

  return (
    <div>
      <PageHeader title="캘린더" description="전체 앱의 일정을 한눈에 확인합니다" />

      {/* Legend */}
      {activeCollections.length > 1 && (
        <div className="mb-4 flex flex-wrap gap-2">
          {activeCollections.map(({ id, label, colorIndex }) => (
            <Badge key={id} variant="secondary" className={COLORS[colorIndex].badge}>
              {label}
            </Badge>
          ))}
        </div>
      )}

      <div className="space-y-3">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => goMonth(-1)}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <h3 className="text-lg font-semibold min-w-[120px] text-center">
              {year}년 {month + 1}월
            </h3>
            <Button variant="outline" size="sm" onClick={() => goMonth(1)}>
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={goToday}>
              오늘
            </Button>
          </div>
        </div>

        {/* Calendar grid */}
        <div className="overflow-hidden rounded-md border">
          {/* Day names */}
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
            const weekSpans = spansByWeek[wi]
            const maxTrack = weekSpans.length > 0
              ? Math.min(Math.max(...weekSpans.map((s) => s.track)) + 1, MAX_VISIBLE_SPANS)
              : 0
            const hiddenSpanCount = weekSpans.filter((s) => s.track >= MAX_VISIBLE_SPANS).length

            return (
              <div key={wi} className="grid grid-cols-7 relative">
                {/* Spanning bars */}
                {weekSpans
                  .filter((s) => s.track < MAX_VISIBLE_SPANS)
                  .map((span) => {
                    const leftPct = (span.startCol / 7) * 100
                    const widthPct = (span.colSpan / 7) * 100
                    const top = 24 + span.track * 22
                    const color = COLORS[span.event.colorIndex]

                    const isStart = span.event.date >= makeDateStr(year, month, week[span.startCol] ?? 1)
                    const isEnd =
                      (span.event.endDate ?? span.event.date) <=
                      makeDateStr(year, month, week[span.startCol + span.colSpan - 1] ?? daysInMonth)

                    return (
                      <button
                        key={`${span.event.id}-${wi}`}
                        type="button"
                        className={`absolute z-10 truncate ${color.bg} text-xs px-1.5 py-0.5 hover:opacity-80 cursor-pointer border-l-2 ${color.border} ${
                          isStart ? 'rounded-l' : ''
                        } ${isEnd ? 'rounded-r' : ''}`}
                        style={{
                          left: `calc(${leftPct}% + 2px)`,
                          width: `calc(${widthPct}% - 4px)`,
                          top: `${top}px`,
                          height: '20px',
                          lineHeight: '18px',
                        }}
                        onClick={() => handleEventClick(span.event)}
                        title={`${span.event.collectionLabel}: ${span.event.label}`}
                      >
                        {isStart ? span.event.label : `… ${span.event.label}`}
                      </button>
                    )
                  })}

                {/* Day cells */}
                {week.map((day, di) => {
                  if (day === null) {
                    return <div key={di} className="min-h-[100px] bg-muted/20" />
                  }

                  const dateStr = makeDateStr(year, month, day)
                  const dayEvents = eventsByDate.get(dateStr) ?? []
                  const spanOffset = maxTrack * 22

                  return (
                    <div
                      key={di}
                      className={`min-h-[100px] border-b border-r p-1 ${
                        isToday(day) ? 'bg-primary/5' : ''
                      }`}
                    >
                      <div
                        className={`mb-1 text-xs font-medium ${
                          di === 0 ? 'text-red-500' : di === 6 ? 'text-blue-500' : ''
                        } ${
                          isToday(day)
                            ? 'rounded-full bg-primary text-primary-foreground w-5 h-5 flex items-center justify-center'
                            : ''
                        }`}
                      >
                        {day}
                      </div>
                      <div className="space-y-0.5" style={{ marginTop: `${spanOffset}px` }}>
                        {dayEvents.slice(0, 3).map((ev) => {
                          const color = COLORS[ev.colorIndex]
                          return (
                            <button
                              key={ev.id}
                              type="button"
                              className={`block w-full truncate rounded ${color.bg} px-1 py-0.5 text-left text-xs hover:opacity-80 border-l-2 ${color.border}`}
                              onClick={() => handleEventClick(ev)}
                              title={`${ev.collectionLabel}: ${ev.label}`}
                            >
                              {ev.label}
                            </button>
                          )
                        })}
                        {dayEvents.length > 3 && (
                          <span className="text-[10px] text-muted-foreground pl-1">
                            +{dayEvents.length - 3} 더
                          </span>
                        )}
                        {di === 6 && hiddenSpanCount > 0 && (
                          <span className="text-[10px] text-muted-foreground pl-1">
                            +{hiddenSpanCount} 더
                          </span>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )
          })}

          </div>

          {/* Empty month */}
          {events.length === 0 && !isLoading && (
            <div className="flex flex-col items-center justify-center py-10 text-muted-foreground text-sm">
              <Calendar className="h-8 w-8 mb-2 opacity-40" />
              <p>이번 달에 해당하는 일정이 없습니다</p>
              <Button variant="link" size="sm" className="mt-1" onClick={goToday}>
                오늘로 이동
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
