import { useDroppable } from '@dnd-kit/core'
import { useMemo } from 'react'

import type { CalendarWeek } from '@/hooks/useEntries'
import type { Field } from '@/lib/types'

const HOURS = Array.from({ length: 24 }, (_, i) => i)
const HOUR_HEIGHT = 56 // px per hour

interface Props {
  weeks: CalendarWeek[]
  viewDate: Date
  titleField?: Field | undefined
  dateField: Field
  endDateField: Field | null
  onEntryClick: (entry: Record<string, unknown>) => void
  onEntryUpdate?: (entryId: string, updates: Record<string, unknown>) => void
  onCreateEntry?: (prefill: Record<string, unknown>) => void
  getLabel: (entry: Record<string, unknown>) => string
  colorMap: Map<string, string>
  colorField: Field | null
}

function toDateStr(v: unknown): string | null {
  if (!v) return null
  const s = String(v).slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return null
  return s
}

function toTimeStr(v: unknown): { hours: number; minutes: number } | null {
  if (!v) return null
  const s = String(v)
  const match = s.match(/T(\d{2}):(\d{2})/)
  if (match) return { hours: parseInt(match[1], 10), minutes: parseInt(match[2], 10) }
  return null
}

function formatDateStr(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const DAY_NAMES = ['일', '월', '화', '수', '목', '금', '토']

export default function CalendarDayView({
  weeks,
  viewDate,
  dateField,
  endDateField,
  onEntryClick,
  onEntryUpdate,
  onCreateEntry,
  getLabel,
  colorMap,
  colorField,
}: Props) {
  const dayStr = formatDateStr(viewDate)
  const dayOfWeek = viewDate.getDay()
  const today = new Date()
  const isToday = formatDateStr(today) === dayStr

  // Collect entries for this day
  const { allDayEntries, timedEntries } = useMemo(() => {
    const allDay: Record<string, unknown>[] = []
    const timed: { entry: Record<string, unknown>; startHour: number; startMin: number; duration: number }[] = []

    for (const w of weeks) {
      for (const span of w.spans) {
        const entryDate = toDateStr(span.entry[dateField.slug])
        if (entryDate === dayStr) {
          allDay.push(span.entry)
        }
      }
      const entries = w.singles[dayStr]
      if (entries) {
        for (const entry of entries) {
          const time = toTimeStr(entry[dateField.slug])
          if (time) {
            const endTime = endDateField ? toTimeStr(entry[endDateField.slug]) : null
            const duration = endTime
              ? (endTime.hours - time.hours) + (endTime.minutes - time.minutes) / 60
              : 1
            timed.push({
              entry,
              startHour: time.hours,
              startMin: time.minutes,
              duration: Math.max(duration, 0.5),
            })
          } else {
            allDay.push(entry)
          }
        }
      }
    }

    return { allDayEntries: allDay, timedEntries: timed }
  }, [weeks, dayStr, dateField.slug, endDateField])

  function getEntryColor(entry: Record<string, unknown>): string {
    if (!colorField) return 'bg-primary/15 border-l-primary'
    const val = String(entry[colorField.slug] ?? '')
    const color = colorMap.get(val)
    return color ?? 'bg-primary/15 border-l-primary'
  }

  return (
    <div className="overflow-hidden rounded-md border">
      {/* Header */}
      <div className="border-b px-4 py-2 text-center">
        <span className={`text-sm font-semibold ${dayOfWeek === 0 ? 'text-red-500' : dayOfWeek === 6 ? 'text-blue-500' : ''}`}>
          {viewDate.getFullYear()}년 {viewDate.getMonth() + 1}월 {viewDate.getDate()}일 ({DAY_NAMES[dayOfWeek]})
        </span>
        {isToday && (
          <span className="ml-2 rounded-full bg-primary px-2 py-0.5 text-[10px] text-primary-foreground">
            오늘
          </span>
        )}
      </div>

      {/* All-day */}
      {allDayEntries.length > 0 && (
        <div className="border-b px-4 py-1 space-y-0.5">
          <span className="text-[10px] text-muted-foreground">종일</span>
          {allDayEntries.map((entry) => (
            <button
              key={String(entry.id)}
              type="button"
              className={`block w-full max-w-md truncate rounded border-l-2 px-2 py-1 text-left text-xs hover:bg-primary/25 ${getEntryColor(entry)}`}
              onClick={() => onEntryClick(entry)}
            >
              {getLabel(entry)}
            </button>
          ))}
        </div>
      )}

      {/* Time grid */}
      <div className="relative max-h-[600px] overflow-y-auto">
        {HOURS.map((h) => (
          <DayTimeSlot
            key={h}
            id={`${dayStr}T${String(h).padStart(2, '0')}:00`}
            day={dayStr}
            hour={h}
            dateField={dateField}
            onCreateEntry={onCreateEntry}
            draggable={!!onEntryUpdate}
          />
        ))}

        {/* Timed events */}
        {timedEntries.map(({ entry, startHour, startMin, duration }) => {
          const top = (startHour + startMin / 60) * HOUR_HEIGHT
          const height = Math.max(duration * HOUR_HEIGHT, 24)
          return (
            <button
              key={String(entry.id)}
              type="button"
              className={`absolute left-16 right-4 z-10 overflow-hidden rounded border-l-2 px-2 py-1 text-left text-xs leading-tight hover:ring-1 hover:ring-primary/30 ${getEntryColor(entry)}`}
              style={{ top, height }}
              onClick={() => onEntryClick(entry)}
            >
              <span className="font-medium">{getLabel(entry)}</span>
              <span className="ml-2 text-muted-foreground">
                {String(startHour).padStart(2, '0')}:{String(startMin).padStart(2, '0')}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function DayTimeSlot({
  id,
  day,
  hour,
  dateField,
  onCreateEntry,
  draggable,
}: {
  id: string
  day: string
  hour: number
  dateField: Field
  onCreateEntry?: (prefill: Record<string, unknown>) => void
  draggable: boolean
}) {
  const { setNodeRef, isOver } = useDroppable({ id, disabled: !draggable })

  return (
    <div
      ref={setNodeRef}
      className={`flex border-b ${isOver ? 'bg-primary/10' : ''} ${hour % 2 === 0 ? '' : 'border-dashed'}`}
      style={{ height: HOUR_HEIGHT }}
      onClick={() => {
        if (onCreateEntry) {
          onCreateEntry({
            [dateField.slug]: `${day}T${String(hour).padStart(2, '0')}:00`,
          })
        }
      }}
    >
      <div className="w-14 flex-shrink-0 border-r px-1 text-right text-[10px] text-muted-foreground">
        {String(hour).padStart(2, '0')}:00
      </div>
      <div className="flex-1" />
    </div>
  )
}
