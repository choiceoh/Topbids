import { useDroppable } from '@dnd-kit/core'
import { useMemo } from 'react'

import type { CalendarWeek } from '@/hooks/useEntries'
import type { Field } from '@/lib/types'

const HOURS = Array.from({ length: 24 }, (_, i) => i)
const DAY_NAMES = ['일', '월', '화', '수', '목', '금', '토']
const HOUR_HEIGHT = 48 // px per hour

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
  // Try to extract time from datetime string
  const match = s.match(/T(\d{2}):(\d{2})/)
  if (match) return { hours: parseInt(match[1], 10), minutes: parseInt(match[2], 10) }
  return null
}

function getWeekDays(viewDate: Date): string[] {
  const d = new Date(viewDate)
  const day = d.getDay()
  d.setDate(d.getDate() - day)
  const days: string[] = []
  for (let i = 0; i < 7; i++) {
    const y = d.getFullYear()
    const m = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    days.push(`${y}-${m}-${dd}`)
    d.setDate(d.getDate() + 1)
  }
  return days
}

export default function CalendarWeekView({
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
  const weekDays = useMemo(() => getWeekDays(viewDate), [viewDate])
  const today = new Date()
  const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`

  // Collect all entries from weeks data that fall in the current week
  const { allDayEntries, timedEntries } = useMemo(() => {
    const allDay: Map<string, Record<string, unknown>[]> = new Map()
    const timed: Map<string, { entry: Record<string, unknown>; startHour: number; startMin: number; duration: number }[]> = new Map()

    for (const day of weekDays) {
      allDay.set(day, [])
      timed.set(day, [])
    }

    for (const w of weeks) {
      // Spans (multi-day events) go to allDay
      for (const span of w.spans) {
        const entryDate = toDateStr(span.entry[dateField.slug])
        if (entryDate && weekDays.includes(entryDate)) {
          const bucket = allDay.get(entryDate)
          if (bucket) bucket.push(span.entry)
        }
      }
      // Singles
      for (const [dateStr, entries] of Object.entries(w.singles)) {
        if (!weekDays.includes(dateStr)) continue
        for (const entry of entries) {
          const time = toTimeStr(entry[dateField.slug])
          if (time) {
            const endTime = endDateField ? toTimeStr(entry[endDateField.slug]) : null
            const duration = endTime
              ? (endTime.hours - time.hours) + (endTime.minutes - time.minutes) / 60
              : 1
            const timedBucket = timed.get(dateStr)
            if (timedBucket) timedBucket.push({
              entry,
              startHour: time.hours,
              startMin: time.minutes,
              duration: Math.max(duration, 0.5),
            })
          } else {
            const allDayBucket = allDay.get(dateStr)
            if (allDayBucket) allDayBucket.push(entry)
          }
        }
      }
    }

    return { allDayEntries: allDay, timedEntries: timed }
  }, [weeks, weekDays, dateField.slug, endDateField])

  const maxAllDay = useMemo(() => {
    let max = 0
    for (const entries of allDayEntries.values()) {
      max = Math.max(max, entries.length)
    }
    return max
  }, [allDayEntries])

  const allDayHeight = Math.max(maxAllDay * 24 + 8, 32)

  function getEntryColor(entry: Record<string, unknown>): string {
    if (!colorField) return 'bg-primary/15 border-l-primary'
    const val = String(entry[colorField.slug] ?? '')
    const color = colorMap.get(val)
    return color ?? 'bg-primary/15 border-l-primary'
  }

  return (
    <div className="overflow-hidden rounded-md border">
      {/* Header */}
      <div className="grid grid-cols-[60px_repeat(7,1fr)]">
        <div className="border-b border-r px-1 py-1.5 text-center text-xs text-muted-foreground">
          시간
        </div>
        {weekDays.map((day, i) => {
          const d = new Date(day + 'T00:00:00')
          const isToday = day === todayStr
          return (
            <div
              key={day}
              className={`border-b border-r px-2 py-1.5 text-center text-xs font-medium ${
                i === 0 ? 'text-red-500' : i === 6 ? 'text-blue-500' : 'text-muted-foreground'
              } ${isToday ? 'bg-primary/5' : ''}`}
            >
              {DAY_NAMES[i]} {d.getDate()}
            </div>
          )
        })}
      </div>

      {/* All-day row */}
      {maxAllDay > 0 && (
        <div className="grid grid-cols-[60px_repeat(7,1fr)] border-b">
          <div className="border-r px-1 py-1 text-center text-[10px] text-muted-foreground">
            종일
          </div>
          {weekDays.map((day) => {
            const entries = allDayEntries.get(day) ?? []
            return (
              <div key={day} className="border-r p-0.5 space-y-0.5" style={{ minHeight: allDayHeight }}>
                {entries.map((entry) => (
                  <button
                    key={String(entry.id)}
                    type="button"
                    className={`block w-full truncate rounded border-l-2 px-1 py-0.5 text-left text-[10px] hover:bg-primary/25 ${getEntryColor(entry)}`}
                    onClick={() => onEntryClick(entry)}
                  >
                    {getLabel(entry)}
                  </button>
                ))}
              </div>
            )
          })}
        </div>
      )}

      {/* Time grid */}
      <div className="grid grid-cols-[60px_repeat(7,1fr)] max-h-[600px] overflow-y-auto">
        {/* Time labels column */}
        <div className="border-r">
          {HOURS.map((h) => (
            <div
              key={h}
              className="border-b px-1 text-right text-[10px] text-muted-foreground"
              style={{ height: HOUR_HEIGHT }}
            >
              {String(h).padStart(2, '0')}:00
            </div>
          ))}
        </div>

        {/* Day columns */}
        {weekDays.map((day) => {
          const entries = timedEntries.get(day) ?? []
          const isToday = day === todayStr
          return (
            <div key={day} className={`relative border-r ${isToday ? 'bg-primary/5' : ''}`}>
              {/* Hour lines */}
              {HOURS.map((h) => (
                <WeekTimeSlot
                  key={h}
                  id={`${day}T${String(h).padStart(2, '0')}:00`}
                  day={day}
                  hour={h}
                  dateField={dateField}
                  onCreateEntry={onCreateEntry}
                  draggable={!!onEntryUpdate}
                />
              ))}
              {/* Timed events */}
              {entries.map(({ entry, startHour, startMin, duration }) => {
                const top = (startHour + startMin / 60) * HOUR_HEIGHT
                const height = Math.max(duration * HOUR_HEIGHT, 20)
                return (
                  <button
                    key={String(entry.id)}
                    type="button"
                    className={`absolute left-0.5 right-0.5 z-10 overflow-hidden rounded border-l-2 px-1 py-0.5 text-left text-[10px] leading-tight hover:ring-1 hover:ring-primary/30 ${getEntryColor(entry)}`}
                    style={{ top, height }}
                    onClick={() => onEntryClick(entry)}
                  >
                    <span className="font-medium">{getLabel(entry)}</span>
                    <br />
                    <span className="text-muted-foreground">
                      {String(startHour).padStart(2, '0')}:{String(startMin).padStart(2, '0')}
                    </span>
                  </button>
                )
              })}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function WeekTimeSlot({
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
      className={`border-b ${isOver ? 'bg-primary/10' : ''} ${hour % 2 === 0 ? '' : 'border-dashed'}`}
      style={{ height: HOUR_HEIGHT }}
      onClick={() => {
        if (onCreateEntry) {
          onCreateEntry({
            [dateField.slug]: `${day}T${String(hour).padStart(2, '0')}:00`,
          })
        }
      }}
    />
  )
}
