import { describe, it, expect } from 'vitest'
import {
  FIELD_TYPE_LABELS,
  isLayoutType,
  isComputedType,
  operatorsForFieldType,
  TERM,
  ROLE_LABELS,
} from './constants'
import type { FieldType } from './types'

const ALL_FIELD_TYPES: FieldType[] = [
  'text', 'textarea', 'number', 'integer', 'boolean',
  'date', 'datetime', 'time', 'select', 'multiselect',
  'relation', 'user', 'file', 'json', 'autonumber',
  'formula', 'lookup', 'rollup', 'table', 'label', 'line', 'spacer',
]

describe('FIELD_TYPE_LABELS', () => {
  it('has a label for every field type', () => {
    for (const ft of ALL_FIELD_TYPES) {
      expect(FIELD_TYPE_LABELS[ft]).toBeTruthy()
    }
  })
})


describe('isLayoutType', () => {
  it('returns true for layout types', () => {
    expect(isLayoutType('label')).toBe(true)
    expect(isLayoutType('line')).toBe(true)
    expect(isLayoutType('spacer')).toBe(true)
  })

  it('returns false for non-layout types', () => {
    expect(isLayoutType('text')).toBe(false)
    expect(isLayoutType('number')).toBe(false)
    expect(isLayoutType('formula')).toBe(false)
  })
})

describe('isComputedType', () => {
  it('returns true for computed types', () => {
    expect(isComputedType('formula')).toBe(true)
    expect(isComputedType('lookup')).toBe(true)
    expect(isComputedType('rollup')).toBe(true)
  })

  it('returns false for non-computed types', () => {
    expect(isComputedType('text')).toBe(false)
    expect(isComputedType('label')).toBe(false)
  })
})

describe('operatorsForFieldType', () => {
  it('number types include comparison operators', () => {
    const ops = operatorsForFieldType('number')
    expect(ops).toContain('gt')
    expect(ops).toContain('gte')
    expect(ops).toContain('lt')
    expect(ops).toContain('lte')
    expect(ops).not.toContain('like')
  })

  it('text types include like operator', () => {
    const ops = operatorsForFieldType('text')
    expect(ops).toContain('like')
    expect(ops).toContain('eq')
    expect(ops).not.toContain('gt')
  })

  it('boolean has limited operators', () => {
    const ops = operatorsForFieldType('boolean')
    expect(ops).toEqual(['eq', 'neq', 'is_null'])
  })

  it('select includes in operator', () => {
    const ops = operatorsForFieldType('select')
    expect(ops).toContain('in')
  })

  it('multiselect uses like', () => {
    const ops = operatorsForFieldType('multiselect')
    expect(ops).toContain('like')
    expect(ops).toContain('is_null')
  })

  it('all types include is_null', () => {
    for (const ft of ALL_FIELD_TYPES) {
      // layout types fall into default case
      if (['label', 'line', 'spacer'].includes(ft)) continue
      const ops = operatorsForFieldType(ft)
      expect(ops).toContain('is_null')
    }
  })
})

describe('TERM', () => {
  it('has Korean translations', () => {
    expect(TERM.collection).toBe('앱')
    expect(TERM.field).toBe('항목')
    expect(TERM.record).toBe('데이터')
  })
})

describe('ROLE_LABELS', () => {
  it('has labels for all roles', () => {
    expect(ROLE_LABELS.director).toBe('관리자')
    expect(ROLE_LABELS.pm).toBe('운영자')
    expect(ROLE_LABELS.engineer).toBe('담당자')
    expect(ROLE_LABELS.viewer).toBe('열람자')
  })
})
