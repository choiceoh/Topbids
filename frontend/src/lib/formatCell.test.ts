import { describe, it, expect } from 'vitest'
import { formatCell } from './formatCell'
import type { Field } from './types'

function makeField(overrides: Partial<Field>): Field {
  return {
    id: 'f1',
    collection_id: 'c1',
    slug: 'test',
    label: 'Test',
    field_type: 'text',
    is_required: false,
    is_unique: false,
    is_indexed: false,
    width: 3,
    height: 1,
    sort_order: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

describe('formatCell', () => {
  it('returns "-" for null/undefined', () => {
    expect(formatCell(null, makeField({}))).toBe('-')
    expect(formatCell(undefined, makeField({}))).toBe('-')
  })

  it('formats relation objects', () => {
    const f = makeField({ field_type: 'relation' })
    expect(formatCell({ name: 'Task A' }, f)).toBe('Task A')
    expect(formatCell({ title: 'Item B' }, f)).toBe('Item B')
    expect(formatCell({ label: 'Label C' }, f)).toBe('Label C')
    expect(formatCell({ id: 'id-1' }, f)).toBe('id-1')
    expect(formatCell({}, f)).toBe('?')
  })

  it('formats user objects', () => {
    const f = makeField({ field_type: 'user' })
    expect(formatCell({ name: 'Alice' }, f)).toBe('Alice')
    expect(formatCell({ email: 'a@b.com' }, f)).toBe('a@b.com')
  })

  it('formats boolean', () => {
    const f = makeField({ field_type: 'boolean' })
    expect(formatCell(true, f)).toBe('✓')
    expect(formatCell(false, f)).toBe('-')
  })

  it('formats date', () => {
    const f = makeField({ field_type: 'date' })
    const result = formatCell('2025-01-15', f)
    expect(result).toContain('2025')
  })

  it('formats time', () => {
    const f = makeField({ field_type: 'time' })
    expect(formatCell('14:30:00', f)).toBe('14:30:00')
  })

  it('formats multiselect', () => {
    const f = makeField({ field_type: 'multiselect' })
    expect(formatCell(['A', 'B', 'C'], f)).toBe('A, B, C')
  })

  it('truncates long textarea', () => {
    const f = makeField({ field_type: 'textarea' })
    const long = 'x'.repeat(150)
    const result = formatCell(long, f)
    expect(result.length).toBeLessThanOrEqual(103) // 100 + "..."
    expect(result).toContain('...')
  })

  it('formats json', () => {
    const f = makeField({ field_type: 'json' })
    expect(formatCell({ a: 1 }, f)).toBe('{"a":1}')
  })

  it('formats currency number', () => {
    const f = makeField({
      field_type: 'number',
      options: { display_type: 'currency', currency_code: 'KRW' },
    })
    const result = formatCell(10000, f)
    expect(result).toContain('10,000')
  })

  it('formats percent number', () => {
    const f = makeField({
      field_type: 'number',
      options: { display_type: 'percent' },
    })
    expect(formatCell(75, f)).toBe('75%')
  })

  it('formats rating number', () => {
    const f = makeField({
      field_type: 'integer',
      options: { display_type: 'rating', max_rating: 5 },
    })
    expect(formatCell(3, f)).toBe('★★★☆☆')
  })

  it('formats progress number', () => {
    const f = makeField({
      field_type: 'number',
      options: { display_type: 'progress' },
    })
    expect(formatCell(80, f)).toBe('80%')
  })

  it('formats autonumber', () => {
    const f = makeField({ field_type: 'autonumber' })
    expect(formatCell(42, f)).toBe('42')
  })

  it('formats formula with number result', () => {
    const f = makeField({
      field_type: 'formula',
      options: { result_type: 'number', precision: 2 },
    })
    const result = formatCell(1234.5, f)
    expect(result).toContain('1,234.50')
  })

  it('formats formula with boolean result', () => {
    const f = makeField({
      field_type: 'formula',
      options: { result_type: 'boolean' },
    })
    expect(formatCell(true, f)).toBe('✓')
    expect(formatCell(false, f)).toBe('-')
  })

  it('formats rollup as localized number', () => {
    const f = makeField({ field_type: 'rollup' })
    const result = formatCell(1234, f)
    expect(result).toContain('1,234')
  })

  it('formats lookup as string', () => {
    const f = makeField({ field_type: 'lookup' })
    expect(formatCell('hello', f)).toBe('hello')
  })

  it('falls back to String() for unknown types', () => {
    const f = makeField({ field_type: 'text' })
    expect(formatCell(42, f)).toBe('42')
  })
})
