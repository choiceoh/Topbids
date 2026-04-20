import { describe, it, expect } from 'vitest'
import { extractSelectionData, toTSV } from './clipboard'

describe('extractSelectionData', () => {
  const data = [
    { a: 1, b: 'x', c: true },
    { a: 2, b: 'y', c: false },
    { a: 3, b: 'z', c: true },
  ]
  const cols = ['a', 'b', 'c']

  it('extracts a single cell', () => {
    const result = extractSelectionData(data, cols, { startRow: 0, endRow: 0, startCol: 0, endCol: 0 })
    expect(result).toEqual([[1]])
  })

  it('extracts a range', () => {
    const result = extractSelectionData(data, cols, { startRow: 0, endRow: 1, startCol: 1, endCol: 2 })
    expect(result).toEqual([['x', true], ['y', false]])
  })

  it('normalizes reversed range', () => {
    const result = extractSelectionData(data, cols, { startRow: 1, endRow: 0, startCol: 2, endCol: 1 })
    expect(result).toEqual([['x', true], ['y', false]])
  })

  it('handles missing values as empty string', () => {
    const sparse = [{ a: 1 }]
    const result = extractSelectionData(sparse, ['a', 'b'], { startRow: 0, endRow: 0, startCol: 0, endCol: 1 })
    expect(result).toEqual([[1, '']])
  })
})

describe('toTSV', () => {
  it('converts simple matrix', () => {
    expect(toTSV([[1, 2], [3, 4]])).toBe('1\t2\n3\t4')
  })

  it('handles null/undefined', () => {
    expect(toTSV([[null, undefined]])).toBe('\t')
  })

  it('escapes tabs in values', () => {
    expect(toTSV([['a\tb']])).toBe('"a\tb"')
  })

  it('escapes newlines in values', () => {
    expect(toTSV([['a\nb']])).toBe('"a\nb"')
  })

  it('escapes quotes', () => {
    expect(toTSV([['say "hello"']])).toBe('"say ""hello"""')
  })
})
