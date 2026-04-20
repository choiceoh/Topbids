import { useCallback, useMemo, useState } from 'react'

import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

type FrequencyType = 'daily' | 'weekdays' | 'weekly' | 'monthly' | 'custom'

const FREQUENCY_LABELS: Record<FrequencyType, string> = {
  daily: '매일',
  weekdays: '평일 (월~금)',
  weekly: '매주',
  monthly: '매월',
  custom: '직접 입력',
}

const WEEKDAY_LABELS = [
  { value: '1', label: '월' },
  { value: '2', label: '화' },
  { value: '3', label: '수' },
  { value: '4', label: '목' },
  { value: '5', label: '금' },
  { value: '6', label: '토' },
  { value: '0', label: '일' },
]

const HOURS = Array.from({ length: 24 }, (_, i) => i)
const MINUTES = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]
const DAYS_OF_MONTH = Array.from({ length: 31 }, (_, i) => i + 1)

interface Props {
  value: string
  onChange: (cron: string) => void
}

/** Validate a 5-field cron expression (minute hour dom month dow). */
export function isValidCron(cron: string): boolean {
  const parts = cron.trim().split(/\s+/)
  if (parts.length !== 5) return false
  const patterns = [
    /^(\*|(\d{1,2})(,\d{1,2})*|(\d{1,2}-\d{1,2})(\/\d{1,2})?)$/, // minute 0-59
    /^(\*|(\d{1,2})(,\d{1,2})*|(\d{1,2}-\d{1,2})(\/\d{1,2})?)$/, // hour 0-23
    /^(\*|(\d{1,2})(,\d{1,2})*|(\d{1,2}-\d{1,2})(\/\d{1,2})?)$/, // dom 1-31
    /^(\*|(\d{1,2})(,\d{1,2})*|(\d{1,2}-\d{1,2})(\/\d{1,2})?)$/, // month 1-12
    /^(\*|(\d)(,\d)*|(\d-\d)(\/\d)?)$/,                           // dow 0-7
  ]
  const ranges = [
    [0, 59], [0, 23], [1, 31], [1, 12], [0, 7],
  ]
  for (let i = 0; i < 5; i++) {
    if (!patterns[i].test(parts[i])) return false
    // Check numeric values are within range
    const nums = parts[i].match(/\d+/g)
    if (nums) {
      for (const n of nums) {
        const v = parseInt(n, 10)
        if (v < ranges[i][0] || v > ranges[i][1]) return false
      }
    }
  }
  return true
}

/** Parse a cron expression into our friendly UI state */
function parseCron(cron: string): {
  frequency: FrequencyType
  hour: number
  minute: number
  weekdays: string[]
  monthDay: number
} {
  const defaults = { frequency: 'daily' as FrequencyType, hour: 9, minute: 0, weekdays: ['1'], monthDay: 1 }
  if (!cron.trim()) return defaults

  const parts = cron.trim().split(/\s+/)
  if (parts.length !== 5) return { ...defaults, frequency: 'custom' }

  const [minStr, hourStr, domStr, , dowStr] = parts
  const minute = parseInt(minStr, 10)
  const hour = parseInt(hourStr, 10)

  if (isNaN(minute) || isNaN(hour)) return { ...defaults, frequency: 'custom' }

  // "0 9 * * *" → daily
  if (domStr === '*' && dowStr === '*') {
    return { frequency: 'daily', hour, minute, weekdays: ['1'], monthDay: 1 }
  }
  // "0 9 * * 1-5" → weekdays
  if (domStr === '*' && dowStr === '1-5') {
    return { frequency: 'weekdays', hour, minute, weekdays: ['1', '2', '3', '4', '5'], monthDay: 1 }
  }
  // "0 9 * * 1,3,5" → weekly with specific days
  if (domStr === '*' && /^[\d,]+$/.test(dowStr)) {
    const weekdays = dowStr.split(',')
    return { frequency: 'weekly', hour, minute, weekdays, monthDay: 1 }
  }
  // "0 9 15 * *" → monthly
  if (/^\d+$/.test(domStr) && dowStr === '*') {
    return { frequency: 'monthly', hour, minute, weekdays: ['1'], monthDay: parseInt(domStr, 10) }
  }

  return { ...defaults, frequency: 'custom' }
}

export default function SchedulePicker({ value, onChange }: Props) {
  const parsed = useMemo(() => parseCron(value), [value])

  const [frequency, setFrequency] = useState<FrequencyType>(parsed.frequency)
  const [hour, setHour] = useState(parsed.hour)
  const [minute, setMinute] = useState(parsed.minute)
  const [weekdays, setWeekdays] = useState<string[]>(parsed.weekdays)
  const [monthDay, setMonthDay] = useState(parsed.monthDay)
  const [customCron, setCustomCron] = useState(parsed.frequency === 'custom' ? value : '')
  const cronError = frequency === 'custom' && customCron.trim() !== '' && !isValidCron(customCron)
    ? '유효하지 않은 크론 표현식입니다 (예: 0 9 * * 1-5)'
    : ''

  // Sync from external value changes (e.g. edit mode)
  const [syncedValue, setSyncedValue] = useState(value)
  if (value !== syncedValue) {
    setSyncedValue(value)
    const p = parseCron(value)
    setFrequency(p.frequency)
    setHour(p.hour)
    setMinute(p.minute)
    setWeekdays(p.weekdays)
    setMonthDay(p.monthDay)
    if (p.frequency === 'custom') setCustomCron(value)
  }

  const buildCron = useCallback(
    (freq: FrequencyType, h: number, m: number, wd: string[], md: number, custom: string) => {
      switch (freq) {
        case 'daily':
          return `${m} ${h} * * *`
        case 'weekdays':
          return `${m} ${h} * * 1-5`
        case 'weekly':
          return `${m} ${h} * * ${wd.length > 0 ? wd.join(',') : '1'}`
        case 'monthly':
          return `${m} ${h} ${md} * *`
        case 'custom':
          return custom
      }
    },
    [],
  )

  function emitChange(
    freq: FrequencyType,
    h: number,
    m: number,
    wd: string[],
    md: number,
    custom: string,
  ) {
    onChange(buildCron(freq, h, m, wd, md, custom))
  }

  function handleFrequencyChange(f: FrequencyType) {
    setFrequency(f)
    emitChange(f, hour, minute, weekdays, monthDay, customCron)
  }

  function handleHourChange(h: string | null) {
    if (!h) return
    const n = parseInt(h, 10)
    setHour(n)
    emitChange(frequency, n, minute, weekdays, monthDay, customCron)
  }

  function handleMinuteChange(m: string | null) {
    if (!m) return
    const n = parseInt(m, 10)
    setMinute(n)
    emitChange(frequency, hour, n, weekdays, monthDay, customCron)
  }

  function handleWeekdaysChange(wd: string[]) {
    if (wd.length === 0) return // at least one day
    setWeekdays(wd)
    emitChange(frequency, hour, minute, wd, monthDay, customCron)
  }

  function handleMonthDayChange(d: string | null) {
    if (!d) return
    const n = parseInt(d, 10)
    setMonthDay(n)
    emitChange(frequency, hour, minute, weekdays, n, customCron)
  }

  const summary = useMemo(() => {
    const timeStr = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
    switch (frequency) {
      case 'daily':
        return `매일 ${timeStr}에 실행`
      case 'weekdays':
        return `평일(월~금) ${timeStr}에 실행`
      case 'weekly': {
        const dayNames = weekdays
          .sort((a, b) => Number(a) - Number(b))
          .map((d) => WEEKDAY_LABELS.find((w) => w.value === d)?.label ?? d)
          .join(', ')
        return `매주 ${dayNames}요일 ${timeStr}에 실행`
      }
      case 'monthly':
        return `매월 ${monthDay}일 ${timeStr}에 실행`
      case 'custom':
        return customCron ? `크론: ${customCron}` : ''
    }
  }, [frequency, hour, minute, weekdays, monthDay, customCron])

  return (
    <div className="space-y-3">
      {/* Frequency selector */}
      <div>
        <Label>반복 주기</Label>
        <Select value={frequency} onValueChange={(v: string | null) => v && handleFrequencyChange(v as FrequencyType)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(FREQUENCY_LABELS) as FrequencyType[]).map((f) => (
              <SelectItem key={f} value={f}>{FREQUENCY_LABELS[f]}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Custom cron input */}
      {frequency === 'custom' && (
        <div>
          <Label>크론 표현식</Label>
          <input
            className={`flex h-9 w-full rounded-md border bg-background px-3 py-1 text-sm ${cronError ? 'border-destructive' : 'border-input'}`}
            value={customCron}
            onChange={(e) => {
              setCustomCron(e.target.value)
              if (isValidCron(e.target.value) || e.target.value.trim() === '') {
                emitChange('custom', hour, minute, weekdays, monthDay, e.target.value)
              }
            }}
            placeholder="분 시 일 월 요일 (예: 0 9 * * 1-5)"
          />
          {cronError && (
            <p className="text-sm text-destructive mt-1">{cronError}</p>
          )}
        </div>
      )}

      {/* Time picker */}
      {frequency !== 'custom' && (
        <div className="flex gap-3">
          <div className="w-28">
            <Label>시</Label>
            <Select value={String(hour)} onValueChange={handleHourChange}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {HOURS.map((h) => (
                  <SelectItem key={h} value={String(h)}>
                    {h === 0 ? '오전 12시' : h < 12 ? `오전 ${h}시` : h === 12 ? '오후 12시' : `오후 ${h - 12}시`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="w-24">
            <Label>분</Label>
            <Select value={String(minute)} onValueChange={handleMinuteChange}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {MINUTES.map((m) => (
                  <SelectItem key={m} value={String(m)}>
                    {String(m).padStart(2, '0')}분
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      )}

      {/* Weekday picker */}
      {frequency === 'weekly' && (
        <div>
          <Label>요일 선택</Label>
          <ToggleGroup
            multiple
            value={weekdays}
            onValueChange={handleWeekdaysChange}
            className="justify-start mt-1"
          >
            {WEEKDAY_LABELS.map((d) => (
              <ToggleGroupItem
                key={d.value}
                value={d.value}
                className="w-10 h-9 text-sm"
              >
                {d.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>
      )}

      {/* Month day picker */}
      {frequency === 'monthly' && (
        <div className="w-28">
          <Label>날짜</Label>
          <Select value={String(monthDay)} onValueChange={handleMonthDayChange}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DAYS_OF_MONTH.map((d) => (
                <SelectItem key={d} value={String(d)}>{d}일</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Summary */}
      {summary && (
        <p className="text-sm text-muted-foreground">{summary}</p>
      )}
    </div>
  )
}
