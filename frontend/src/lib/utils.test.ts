import { describe, it, expect } from 'vitest'
import { cn } from './utils'

describe('cn', () => {
  it('merges class names', () => {
    expect(cn('foo', 'bar')).toBe('foo bar')
  })

  it('handles undefined classes', () => {
    expect(cn('foo', undefined, 'baz')).toBe('foo baz')
  })
})
