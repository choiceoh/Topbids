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

function coerce(value: unknown): Date | null {
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value
  if (typeof value !== 'string' || !value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}
