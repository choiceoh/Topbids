import { describe, expect, it } from 'vitest'

import { formatDateTimeKR, formatDeadlineRelative, formatKRW } from './datetime'

describe('formatKRW', () => {
  it('renders under-만 amounts with comma grouping', () => {
    expect(formatKRW(1234)).toBe('1,234 원')
    expect(formatKRW(9999)).toBe('9,999 원')
  })

  it('composes 억 and 만 units for large figures', () => {
    // 1억 2,000만 원 — this is the standard procurement readout
    expect(formatKRW(120_000_000)).toBe('1억 2,000만 원')
    // 10억 — 억 alone, no 만 section
    expect(formatKRW(1_000_000_000)).toBe('10억 원')
    // Exactly 1억
    expect(formatKRW(100_000_000)).toBe('1억 원')
    // Under 1억 but over 만 — only 만 unit used
    expect(formatKRW(50_000_000)).toBe('5,000만 원')
  })

  it('drops sub-10,000 remainder above 1억', () => {
    // 1억 + 999원 → just "1억 원" (won-level precision rarely matters here)
    expect(formatKRW(100_000_999)).toBe('1억 원')
  })

  it('handles zero, negatives, and bad input', () => {
    expect(formatKRW(0)).toBe('—')
    expect(formatKRW(null)).toBe('—')
    expect(formatKRW('not a number')).toBe('—')
    expect(formatKRW(-50_000_000)).toBe('-5,000만 원')
  })
})

describe('formatDateTimeKR', () => {
  it('renders a valid ISO string in Asia/Seoul', () => {
    // 2026-04-24 14:30 UTC → 23:30 in Seoul (UTC+9)
    const out = formatDateTimeKR('2026-04-24T14:30:00Z')
    // Intl output differs slightly across Node versions; just assert the
    // key parts are present (year, month, day, hour, minute).
    expect(out).toMatch(/2026/)
    expect(out).toMatch(/23:30/)
  })

  it('returns "-" for bad input', () => {
    expect(formatDateTimeKR(null)).toBe('-')
    expect(formatDateTimeKR('')).toBe('-')
    expect(formatDateTimeKR('not-a-date')).toBe('-')
  })
})

describe('formatDeadlineRelative', () => {
  const now = new Date('2026-04-24T12:00:00Z')

  it('renders future deadlines as 남음', () => {
    expect(formatDeadlineRelative('2026-04-24T12:30:00Z', now)).toContain('남음')
    expect(formatDeadlineRelative('2026-04-24T15:00:00Z', now)).toContain('3시간')
  })

  it('renders past deadlines as 지남', () => {
    expect(formatDeadlineRelative('2026-04-24T11:30:00Z', now)).toContain('지남')
    // 3 days ago — well into the "일" bucket (48h+ boundary)
    expect(formatDeadlineRelative('2026-04-21T12:00:00Z', now)).toContain('3일')
  })

  it('returns empty string for invalid input', () => {
    expect(formatDeadlineRelative(null, now)).toBe('')
    expect(formatDeadlineRelative('nope', now)).toBe('')
  })
})
