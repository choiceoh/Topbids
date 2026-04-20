/**
 * Cell value formatter for the data grid and export.
 *
 * Converts raw PostgreSQL values into human-readable display strings.
 * Handles every {@link FieldType} including computed fields (formula, lookup,
 * rollup) and display sub-variants (currency, percent, progress, rating).
 * Dates are localized to Korean format via `toLocaleDateString('ko')`.
 */

import type { Field } from '@/lib/types'
import { isExpandedRecord, getDisplayLabel, getFieldOptions, getDisplayType } from '@/lib/fieldGuards'

/**
 * Format a single cell value for display.
 *
 * @param value - Raw value from the entry row (may be null, string, number,
 *   array, or expanded record object).
 * @param field - Field metadata that determines formatting rules.
 * @returns Display string. Returns `'-'` for null/empty values.
 *
 * Per-field-type rules:
 * - `relation`    — expands to display label(s); M:N returns comma-separated list.
 * - `user`        — shows name, falling back to email, then ID.
 * - `boolean`     — check mark or dash.
 * - `date/datetime` — Korean locale date string.
 * - `number/integer` — applies display_type sub-variant:
 *     - `currency`  — Intl.NumberFormat with currency code (default KRW).
 *     - `percent`   — appends `%`.
 *     - `rating`    — star characters up to max_rating.
 *     - `progress`  — appends `%`.
 * - `formula`     — formats based on `result_type` in options.
 * - `rollup`      — Korean locale number formatting.
 * - `lookup`      — comma-separated if array (from M:N).
 * - `table/spreadsheet` — shows row count (e.g. "3행").
 * - `textarea`    — truncated to 100 chars with ellipsis.
 */
export function formatCell(value: unknown, field: Field): string {
  if (value == null) return '-'
  if (field.field_type === 'relation') {
    // M:N: array of UUIDs or expanded objects.
    if (Array.isArray(value)) {
      if (value.length === 0) return '-'
      return value.map((v) => {
        if (isExpandedRecord(v)) return getDisplayLabel(v)
        return String(v)
      }).join(', ')
    }
    // 1:1/1:N: single expanded object.
    if (isExpandedRecord(value)) return getDisplayLabel(value)
  }
  if (field.field_type === 'user' && isExpandedRecord(value)) {
    return String(value.name ?? value.email ?? value.id ?? '?')
  }
  if (field.field_type === 'boolean') return value ? '✓' : '-'
  if (field.field_type === 'date' || field.field_type === 'datetime') {
    return new Date(value as string).toLocaleDateString('ko')
  }
  if (field.field_type === 'time') return String(value)
  if (field.field_type === 'multiselect' && Array.isArray(value)) {
    return value.join(', ')
  }
  if (field.field_type === 'textarea') {
    const s = String(value)
    return s.length > 100 ? s.slice(0, 100) + '...' : s
  }
  if (field.field_type === 'json') return JSON.stringify(value)
  if (field.field_type === 'table' || field.field_type === 'spreadsheet') {
    if (Array.isArray(value)) return `${value.length}행`
    return '-'
  }

  const displayType = getDisplayType(field)

  // Number display subtypes
  if ((field.field_type === 'number' || field.field_type === 'integer') && displayType) {
    const numOpts = getFieldOptions(field, 'number')
    const num = Number(value)
    if (displayType === 'currency') {
      const code = numOpts?.currency_code || 'KRW'
      try {
        return num.toLocaleString('ko-KR', { style: 'currency', currency: code })
      } catch {
        return `${code} ${num.toLocaleString('ko-KR')}`
      }
    }
    if (displayType === 'percent') return `${num}%`
    if (displayType === 'rating') {
      const max = numOpts?.max_rating || 5
      return '★'.repeat(Math.min(num, max)) + '☆'.repeat(Math.max(0, max - num))
    }
    if (displayType === 'progress') return `${num}%`
  }

  // Text display subtypes
  if (field.field_type === 'text' && displayType) {
    const s = String(value)
    if (displayType === 'url') return s
    if (displayType === 'email') return s
    if (displayType === 'phone') return s
  }

  if (field.field_type === 'autonumber') return String(value)

  // Formula fields: format based on result_type in options.
  if (field.field_type === 'formula') {
    const formulaOpts = getFieldOptions(field, 'formula')
    const resultType = formulaOpts?.result_type
    if (resultType === 'number' || resultType === 'integer') {
      const num = Number(value)
      if (!isNaN(num)) {
        const precision = formulaOpts?.precision
        return precision !== undefined
          ? num.toLocaleString('ko-KR', { minimumFractionDigits: precision, maximumFractionDigits: precision })
          : num.toLocaleString('ko-KR')
      }
    }
    if (resultType === 'boolean') return value ? '✓' : '-'
    if (resultType === 'date' && value) {
      return new Date(value as string).toLocaleDateString('ko')
    }
    return String(value)
  }

  // Rollup fields: format numbers nicely.
  if (field.field_type === 'rollup') {
    if (typeof value === 'number') return value.toLocaleString('ko-KR')
    return String(value)
  }

  // Lookup fields: may be a single value or array (from M:N).
  if (field.field_type === 'lookup') {
    if (Array.isArray(value)) return value.map(String).join(', ')
    return String(value)
  }

  return String(value)
}
