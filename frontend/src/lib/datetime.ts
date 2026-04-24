// Topbids is deployed for a single Korean operator; dates in the UI must
// always render in Asia/Seoul regardless of the viewer's local timezone.
// A supplier viewing from overseas should still see the deadline exactly
// as the buyer set it.

const KR_TZ = 'Asia/Seoul'

const FULL_FMT = new Intl.DateTimeFormat('ko-KR', {
  timeZone: KR_TZ,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

const DATE_FMT = new Intl.DateTimeFormat('ko-KR', {
  timeZone: KR_TZ,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
})

/** Render an ISO string (or Date) as "2026-04-24 14:30" in Asia/Seoul. */
export function formatDateTimeKR(value: unknown): string {
  const d = coerce(value)
  if (!d) return '-'
  return FULL_FMT.format(d)
}

/** Render just the date portion in Asia/Seoul. */
export function formatDateKR(value: unknown): string {
  const d = coerce(value)
  if (!d) return '-'
  return DATE_FMT.format(d)
}

/**
 * Relative countdown shown next to deadline fields. Returns a Korean phrase
 * like "마감 3시간 남음" or "마감 5분 지남". Past deadlines are labelled
 * explicitly so suppliers immediately see they can't submit.
 */
export function formatDeadlineRelative(value: unknown, now: Date = new Date()): string {
  const d = coerce(value)
  if (!d) return ''
  const ms = d.getTime() - now.getTime()
  const past = ms < 0
  const abs = Math.abs(ms)

  const minutes = Math.round(abs / 60_000)
  if (minutes < 1) return past ? '방금 마감' : '잠시 후 마감'
  if (minutes < 60) return `${past ? '마감' : '마감까지'} ${minutes}분${past ? ' 지남' : ' 남음'}`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${past ? '마감' : '마감까지'} ${hours}시간${past ? ' 지남' : ' 남음'}`
  const days = Math.round(hours / 24)
  return `${past ? '마감' : '마감까지'} ${days}일${past ? ' 지남' : ' 남음'}`
}

/**
 * Korean-friendly money formatter for Topbids.
 *
 * Procurement numbers reach 10억+ routinely and grouping commas alone become
 * hard to scan. This renders "1억 2,000만 원", falling back to plain
 * comma-separated won for small amounts. Null/NaN/zero render as "—" so
 * missing-data rows look empty rather than "0원".
 */
export function formatKRW(value: unknown): string {
  const n = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(n) || n === 0) return '—'

  const sign = n < 0 ? '-' : ''
  const abs = Math.abs(n)
  const eok = Math.floor(abs / 100_000_000)
  const man = Math.floor((abs % 100_000_000) / 10_000)
  const rest = Math.round(abs % 10_000)

  const parts: string[] = []
  if (eok > 0) parts.push(`${eok.toLocaleString('ko-KR')}억`)
  if (man > 0) parts.push(`${man.toLocaleString('ko-KR')}만`)
  // Drop sub-10,000 remainder for large figures — procurement rounding rarely
  // cares about won-level precision above 1억.
  if (eok === 0 && rest > 0) parts.push(rest.toLocaleString('ko-KR'))
  if (parts.length === 0) return `${sign}0 원`
  return `${sign}${parts.join(' ')} 원`
}

function coerce(value: unknown): Date | null {
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value
  if (typeof value !== 'string' || !value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}
