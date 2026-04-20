import type {
  FieldOptionsMap,
  FieldType,
  ExpandedRecord,
  EntryRow,
  CommonFieldOptions,
} from '@/lib/types'

/** Minimal field shape accepted by all guard functions. */
interface FieldLike {
  field_type: FieldType
  options?: Record<string, unknown>
}

/**
 * Narrow field.options to a typed interface based on field type.
 * Returns undefined if the field type doesn't match or options is absent.
 */
export function getFieldOptions<T extends keyof FieldOptionsMap>(
  field: FieldLike,
  expectedType: T,
): FieldOptionsMap[T] | undefined {
  if (field.field_type !== expectedType || !field.options) return undefined
  return field.options as FieldOptionsMap[T]
}

/**
 * Get choices for select/multiselect fields.
 */
export function getChoices(field: FieldLike): string[] {
  const opts = field.options as { choices?: unknown } | undefined
  return Array.isArray(opts?.choices) ? opts.choices as string[] : []
}

/**
 * Get display_type for text/number/integer fields.
 */
export function getDisplayType(field: FieldLike): string | undefined {
  const opts = field.options as { display_type?: unknown } | undefined
  return typeof opts?.display_type === 'string' ? opts.display_type : undefined
}

/**
 * Get visibility rules from any field's options.
 */
export function getVisibilityRules(field: FieldLike): CommonFieldOptions['visibility_rules'] {
  const opts = field.options as CommonFieldOptions | undefined
  return opts?.visibility_rules
}

// --- Relation / expanded record guards ---

/**
 * Runtime check: is the value an expanded record (object with string id)?
 */
export function isExpandedRecord(v: unknown): v is ExpandedRecord {
  return typeof v === 'object' && v !== null && 'id' in v && typeof (v as ExpandedRecord).id === 'string'
}

/**
 * Extract display label from an expanded record: name → title → label → id → '?'
 */
export function getDisplayLabel(v: ExpandedRecord): string {
  return String(v.name ?? v.title ?? v.label ?? v.id ?? '?')
}

/**
 * Extract the id from a value that may be an expanded record or a raw id string.
 */
export function extractRelationId(v: unknown): string | undefined {
  if (typeof v === 'string') return v
  if (isExpandedRecord(v)) return v.id
  return undefined
}

/**
 * Extract ids from an array of expanded records or raw id strings (M:N).
 */
export function extractRelationIds(v: unknown): string[] {
  if (!Array.isArray(v)) return []
  return v.map(extractRelationId).filter((id): id is string => id !== undefined)
}

// --- Entry row helper ---

/**
 * Cast a generic record to EntryRow. The server always provides at least `id`.
 */
export function asEntryRow(row: Record<string, unknown>): EntryRow {
  return row as EntryRow
}
