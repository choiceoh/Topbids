/**
 * Core domain types for the Topworks no-code app platform.
 *
 * Hierarchy: Collection → Field[] → Entry (row data)
 * Each Collection maps to a real PostgreSQL table in the "data" schema.
 *
 * Field types fall into 3 categories:
 * - Regular: produce a DB column (text, number, select, relation, etc.)
 * - Layout: no DB column, form ordering only (label, line, spacer)
 * - Computed: no DB column, calculated at query time (formula, lookup, rollup)
 */

/**
 * All field types supported by the schema engine.
 *
 * Regular (produce a DB column): text, textarea, number, integer, boolean,
 * date, datetime, time, select, multiselect, relation, user, file, json,
 * autonumber, table, spreadsheet.
 *
 * Layout (form-only, no DB column): label, line, spacer.
 *
 * Computed (evaluated at query time, no DB column): formula, lookup, rollup.
 */
export type FieldType =
  | 'text'
  | 'textarea'
  | 'number'
  | 'integer'
  | 'boolean'
  | 'date'
  | 'datetime'
  | 'time'
  | 'select'
  | 'multiselect'
  | 'relation'
  | 'user'
  | 'file'
  | 'json'
  | 'autonumber'
  | 'formula'
  | 'lookup'
  | 'rollup'
  | 'table'
  | 'spreadsheet'
  | 'label'
  | 'line'
  | 'spacer'

/** Cardinality of a relation between two collections. */
export type RelationType = 'one_to_one' | 'one_to_many' | 'many_to_many'

/** FK metadata linking one collection's field to another collection's rows. */
export interface Relation {
  id: string
  field_id: string
  target_collection_id: string
  relation_type: RelationType
  junction_table?: string
  on_delete: string
}

/**
 * A single field (column) within a collection.
 *
 * - `slug` is the actual PostgreSQL column name (snake_case).
 * - `options` is a JSON bag whose shape varies by `field_type`
 *   (see {@link FieldOptionsMap} for per-type shapes).
 * - `relation` is only populated when `field_type === 'relation'`.
 * - `is_layout` is true for layout-only fields (label, line, spacer)
 *   that do not produce a DB column.
 */
export interface Field {
  id: string
  collection_id: string
  slug: string
  label: string
  field_type: FieldType
  is_required: boolean
  is_unique: boolean
  is_indexed: boolean
  default_value?: unknown
  options?: Record<string, unknown>
  width: number
  height: number
  sort_order: number
  is_layout?: boolean
  created_at: string
  updated_at: string
  relation?: Relation
}

// --- Per-field-type option interfaces ---

/** Base options shared by all field types (e.g. conditional visibility). */
export interface CommonFieldOptions {
  visibility_rules?: { field_slug: string; operator: string; value?: string }[]
}

export interface TextFieldOptions extends CommonFieldOptions {
  display_type?: 'url' | 'email' | 'phone'
}

export interface TextareaFieldOptions extends CommonFieldOptions {
  rows?: number
}

export interface NumberFieldOptions extends CommonFieldOptions {
  min?: number
  max?: number
  display_type?: 'currency' | 'percent' | 'rating' | 'progress'
  currency_code?: string
  max_rating?: number
}

export type IntegerFieldOptions = NumberFieldOptions

export interface SelectFieldOptions extends CommonFieldOptions {
  choices?: string[]
  display?: 'dropdown' | 'radio'
}

export interface MultiselectFieldOptions extends CommonFieldOptions {
  choices?: string[]
}

export interface FormulaFieldOptions extends CommonFieldOptions {
  expression?: string
  result_type?: 'text' | 'number' | 'integer' | 'boolean' | 'date'
  precision?: number
}

export interface LookupFieldOptions extends CommonFieldOptions {
  relation_field?: string
  lookup_field?: string
}

export interface RollupFieldOptions extends CommonFieldOptions {
  relation_field?: string
  rollup_field?: string
  rollup_fn?: string
}

export interface LabelFieldOptions extends CommonFieldOptions {
  content?: string
}

export interface SpacerFieldOptions extends CommonFieldOptions {
  height?: number
}

export interface SubColumn {
  key: string
  label: string
  type?: string
  formula?: string
  choices?: string[]
}

export interface TableFieldOptions extends CommonFieldOptions {
  sub_columns?: SubColumn[]
  initial_rows?: number
}

export interface ConditionalRule {
  id: string
  colIdx: number
  operator: string
  value: string
  style: Record<string, string>
}

export interface MergedCell {
  startRow: number
  startCol: number
  endRow: number
  endCol: number
}

export interface SpreadsheetFieldOptions extends CommonFieldOptions {
  sub_columns?: (SubColumn & { formula?: string })[]
  initial_rows?: number
  conditional_rules?: ConditionalRule[]
  merged_cells?: MergedCell[]
}

/** Maps each {@link FieldType} to its strongly-typed options interface. */
export interface FieldOptionsMap {
  text: TextFieldOptions
  textarea: TextareaFieldOptions
  number: NumberFieldOptions
  integer: IntegerFieldOptions
  select: SelectFieldOptions
  multiselect: MultiselectFieldOptions
  formula: FormulaFieldOptions
  lookup: LookupFieldOptions
  rollup: RollupFieldOptions
  label: LabelFieldOptions
  spacer: SpacerFieldOptions
  table: TableFieldOptions
  spreadsheet: SpreadsheetFieldOptions
  // These field types don't have specific options
  boolean: CommonFieldOptions
  date: CommonFieldOptions
  datetime: CommonFieldOptions
  time: CommonFieldOptions
  relation: CommonFieldOptions
  user: CommonFieldOptions
  file: CommonFieldOptions
  json: CommonFieldOptions
  autonumber: CommonFieldOptions
  line: CommonFieldOptions
}

// --- Entry row (dynamic table row with known system columns) ---

/** A related record expanded inline (e.g. relation or user fields). */
export interface ExpandedRecord {
  id: string
  name?: string
  title?: string
  label?: string
  email?: string
  [key: string]: unknown
}

/**
 * A single row from a dynamic table. System columns (_created_by, _status,
 * etc.) are optional; all user-defined columns are accessed via string index.
 * `_version` supports optimistic concurrency; `_optimistic` flags client-side
 * pending rows.
 */
export interface EntryRow extends Record<string, unknown> {
  id: string
  _version?: number
  _optimistic?: boolean
  _created_by?: string | ExpandedRecord
  _status?: string
  created_at?: string
  updated_at?: string
  _created_at?: string
  _updated_at?: string
}

/** A single row-level security filter rule. `value` may be a literal or a
 *  variable like `$user.id`, `$user.department_id`, `$user.subsidiary_id`. */
export interface RLSFilter {
  field: string
  op: 'eq' | 'neq' | 'in' | 'contains'
  value: string // literal or $user.id, $user.department_id, $user.subsidiary_id, etc.
}

/**
 * Per-collection access control configuration.
 *
 * `entry_*` arrays list role names allowed to perform each CRUD operation.
 *
 * `rls_mode` controls row-level security:
 * - `''` / `'none'` — no row filtering, all rows visible.
 * - `'creator'`     — users see only rows they created.
 * - `'department'`  — users see rows created by anyone in their department.
 * - `'subsidiary'`  — users see rows created by anyone in their subsidiary.
 * - `'filter'`      — custom filter rules defined in `rls_filters`.
 */
export interface AccessConfig {
  entry_view?: string[]
  entry_create?: string[]
  entry_edit?: string[]
  entry_delete?: string[]
  rls_mode?: '' | 'none' | 'creator' | 'department' | 'subsidiary' | 'filter'
  rls_filters?: RLSFilter[]
}

/**
 * A collection (app) in the platform. Each collection maps 1:1 to a
 * PostgreSQL table named `wd_{slug}` in the data schema.
 *
 * - `slug` — table identifier (immutable after creation).
 * - `access_config` — role-based + row-level security settings.
 * - `process_enabled` — when true, rows carry a `_status` column governed
 *   by the associated {@link Process} state machine.
 * - `fields` — eagerly loaded when fetching a single collection detail.
 */
export interface Collection {
  id: string
  slug: string
  label: string
  description?: string
  icon?: string
  is_system: boolean
  process_enabled: boolean
  sort_order: number
  title_field_id?: string
  default_sort_field?: string
  default_sort_order?: 'asc' | 'desc'
  access_config: AccessConfig
  created_at: string
  updated_at: string
  created_by?: string
  fields?: Field[]
}

/**
 * Platform user. `role` determines global permission level:
 * - `director` — full admin: manage users, collections, settings.
 * - `pm`       — project manager: create/edit collections, manage members.
 * - `engineer` — standard user: CRUD entries per collection access_config.
 * - `viewer`   — read-only access to permitted collections.
 * - `supplier` — external bidder; restricted to /portal/* and row-filtered
 *                to their own bids via `supplier_id`.
 */
export interface User {
  id: string
  email: string
  name: string
  role: 'director' | 'pm' | 'engineer' | 'viewer' | 'supplier'
  is_active: boolean
  department_id?: string | null
  subsidiary_id?: string | null
  /** Present only for role='supplier'. Points at data.suppliers.id. */
  supplier_id?: string | null
  position: string
  title: string
  phone: string
  avatar: string
  joined_at?: string | null
  created_at?: string
  updated_at?: string
  department_name?: string | null
  subsidiary_name?: string | null
}

/** Organizational department, may belong to a subsidiary. Supports parent_id for tree nesting. */
export interface Department {
  id: string
  name: string
  parent_id: string | null
  subsidiary_id?: string | null
  subsidiary_name?: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** Top-level legal entity (subsidiary / affiliate company). */
export interface Subsidiary {
  id: string
  external_code?: string | null
  name: string
  sort_order: number
  is_active: boolean
  created_at: string
  updated_at: string
}

// --- Request payloads ---

/** Payload for adding a new field to a collection (triggers ALTER TABLE ADD COLUMN). */
export interface CreateFieldIn {
  slug: string
  label: string
  field_type: FieldType
  is_required?: boolean
  is_unique?: boolean
  is_indexed?: boolean
  default_value?: unknown
  options?: Record<string, unknown>
  width?: number
  height?: number
  relation?: {
    target_collection_id: string
    relation_type: RelationType
    junction_table?: string
    on_delete?: string
  }
}

/** Payload for creating a new collection (triggers CREATE TABLE). */
export interface CreateCollectionReq {
  slug: string
  label: string
  description?: string
  icon?: string
  fields?: CreateFieldIn[]
}

// --- Schema migration preview ---

/** Risk level for a proposed DDL migration. */
export type SafetyLevel = 'SAFE' | 'CAUTIOUS' | 'DANGEROUS'

/**
 * Server-generated preview of a schema migration (field type change, rename,
 * etc.). Shows the DDL, affected row count, and any data-loss warnings so the
 * user can confirm before execution.
 */
export interface Preview {
  safety_level: SafetyLevel
  description: string
  affected_rows: number
  incompatible_rows?: number
  incompatible_sample?: Record<string, unknown>[]
  ddl_up: string
  ddl_down: string
  warnings?: string[]
}

// --- Process (workflow) ---

/**
 * A named state within a {@link Process} workflow (e.g. "접수", "검토중", "완료").
 * Exactly one status must have `is_initial: true` — new entries start there.
 */
export interface ProcessStatus {
  id: string
  process_id: string
  name: string
  color: string
  sort_order: number
  is_initial: boolean
}

/**
 * A directed edge in the process state machine: allows moving a record
 * from `from_status_id` to `to_status_id`. Gated by `allowed_roles` and/or
 * `allowed_user_ids` — only those users may trigger the transition.
 */
export interface ProcessTransition {
  id: string
  process_id: string
  from_status_id: string
  to_status_id: string
  label: string
  allowed_roles: string[]
  allowed_user_ids: string[]
}

/**
 * Workflow state machine attached to a collection.
 * When `is_enabled`, every entry carries a `_status` column whose value
 * must be one of the defined {@link ProcessStatus} items, and transitions
 * between statuses are constrained by {@link ProcessTransition} rules.
 */
export interface Process {
  id: string
  collection_id: string
  is_enabled: boolean
  statuses: ProcessStatus[]
  transitions: ProcessTransition[]
}

export interface SaveProcessReq {
  is_enabled: boolean
  statuses: { name: string; color: string; sort_order: number; is_initial: boolean }[]
  transitions: { from_index: number; to_index: number; label: string; allowed_roles: string[]; allowed_user_ids: string[] }[]
}

// --- Filter condition (frontend-only) ---

/**
 * A single filter predicate used in list queries.
 * `field` is the field slug, `operator` is one of:
 * eq, neq, gt, gte, lt, lte, like, in, is_null.
 * Serialized into the `_filter` query parameter via {@link serializeFilterGroup}.
 */
export interface FilterCondition {
  id: string
  /** Field slug to filter on. */
  field: string
  /** Comparison operator: eq, neq, gt, gte, lt, lte, like, in, is_null. */
  operator: string
  value: string
}

export type FilterLogic = 'and' | 'or'

/** Recursive filter tree: conditions joined by `logic`, with nested sub-groups. */
export interface FilterGroup {
  id: string
  logic: FilterLogic
  conditions: FilterCondition[]
  groups: FilterGroup[]
}

/** Create an empty root filter group */
export function emptyFilterGroup(): FilterGroup {
  return { id: 'root', logic: 'and', conditions: [], groups: [] }
}

/** Check if a filter group has any conditions (flat or nested) */
export function isFilterGroupEmpty(group: FilterGroup): boolean {
  return group.conditions.length === 0 && group.groups.every(isFilterGroupEmpty)
}

/** Flatten a FilterGroup into a flat list of conditions (for display in chips) */
export function flattenFilterGroup(group: FilterGroup): FilterCondition[] {
  const result: FilterCondition[] = [...group.conditions]
  for (const sub of group.groups) {
    result.push(...flattenFilterGroup(sub))
  }
  return result
}

/** Serialize a FilterGroup to JSON for the _filter query param */
export function serializeFilterGroup(group: FilterGroup): string | undefined {
  if (isFilterGroupEmpty(group)) return undefined
  function strip(g: FilterGroup): unknown {
    return {
      logic: g.logic,
      conditions: g.conditions.map((c) => ({
        field: c.field,
        operator: c.operator,
        value: c.value,
      })),
      groups: g.groups.filter((sg) => !isFilterGroupEmpty(sg)).map(strip),
    }
  }
  return JSON.stringify(strip(group))
}

// --- Aggregate response ---

export interface AggregateResult {
  group: string
  value: number
}

// Totals response from GET /api/data/{slug}/totals
export interface TotalsResult {
  _count: number
  [fieldSlug: string]: number | { sum: number; avg: number; min: number; max: number }
}

// --- Views ---

/**
 * Supported view layouts for a collection:
 * - `list`     — data grid (default).
 * - `kanban`   — card columns grouped by a select/status field.
 * - `calendar` — entries plotted on a date/datetime field.
 * - `gantt`    — timeline bar chart for date-range fields.
 * - `form`     — public or internal data-entry form.
 */
export type ViewType = 'list' | 'kanban' | 'calendar' | 'gantt' | 'form'

/** A saved view configuration for a collection. */
export interface View {
  id: string
  collection_id: string
  name: string
  view_type: ViewType
  config: Record<string, unknown>
  sort_order: number
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface CreateViewReq {
  name: string
  view_type: ViewType
  config?: Record<string, unknown>
  sort_order?: number
  is_default?: boolean
}

export interface UpdateViewReq {
  name?: string
  config?: Record<string, unknown>
  sort_order?: number
  is_default?: boolean
}

// --- Comments ---

/** A comment on a specific record within a collection. */
export interface Comment {
  id: string
  collection_id: string
  record_id: string
  user_id: string
  user_name: string
  body: string
  created_at: string
  updated_at: string
}

// --- Notifications ---

/** In-app notification delivered via SSE. `type` determines the icon and routing. */
export interface Notification {
  id: string
  user_id: string
  type: 'comment' | 'state_change' | 'record_update'
  title: string
  body?: string
  ref_collection_id?: string
  ref_record_id?: string
  is_read: boolean
  created_at: string
}

// --- Collection members ---

/** A user's membership and role within a specific collection. */
export interface CollectionMember {
  id: string
  collection_id: string
  user_id: string
  user_name?: string
  user_email?: string
  role: 'owner' | 'editor' | 'viewer'
  created_at: string
}

// --- Process transitions ---

export interface Transition {
  from: string
  to: string
  allowed_roles: string[]
}

// --- Change history ---

/** Audit log entry for a single record mutation. `diff` maps field slugs to old/new values. */
export interface RecordChange {
  id: string
  collection_id: string
  record_id: string
  user_id?: string
  user_name?: string
  operation: 'create' | 'update' | 'delete'
  diff: Record<string, { old?: unknown, new?: unknown }>
  created_at: string
}

// --- Saved views ---

/** A user-saved combination of filters, sort order, and visible fields for quick recall. */
export interface SavedView {
  id: string
  collection_id: string
  name: string
  filter_config: Record<string, unknown>
  sort_config: string
  visible_fields?: string[]
  is_default: boolean
  is_public: boolean
  created_by?: string
  created_at: string
  updated_at: string
}

export interface CreateSavedViewReq {
  name: string
  filter_config?: Record<string, unknown>
  sort_config?: string
  visible_fields?: string[]
  is_default?: boolean
  is_public?: boolean
}

export interface UpdateSavedViewReq {
  name?: string
  filter_config?: Record<string, unknown>
  sort_config?: string
  visible_fields?: string[]
  is_default?: boolean
  is_public?: boolean
}

// --- Automations ---

/** Event that fires an automation rule. */
export type TriggerType = 'record_created' | 'record_updated' | 'record_deleted' | 'status_change' | 'schedule' | 'form_submit'
/** Side-effect an automation can perform when its conditions pass. */
export type ActionType = 'send_notification' | 'update_field' | 'call_webhook'
/** Comparison operators for automation condition predicates. */
export type ConditionOperator = 'equals' | 'not_equals' | 'contains' | 'gt' | 'lt' | 'is_empty' | 'is_not_empty'

/**
 * A guard condition in an automation rule. All conditions must pass (AND logic)
 * for the automation's actions to execute.
 */
export interface AutomationCondition {
  id: string
  field_slug: string
  operator: ConditionOperator
  value: string
  sort_order: number
}

/** A side-effect to perform when an automation fires. `action_config` shape depends on `action_type`. */
export interface AutomationAction {
  id: string
  action_type: ActionType
  action_config: Record<string, unknown>
  sort_order: number
}

/**
 * An automation rule: trigger → conditions → actions.
 * When `trigger_type` fires and all {@link AutomationCondition}s pass,
 * each {@link AutomationAction} executes in `sort_order`.
 */
export interface Automation {
  id: string
  collection_id: string
  name: string
  is_enabled: boolean
  trigger_type: TriggerType
  trigger_config: Record<string, unknown>
  conditions: AutomationCondition[]
  actions: AutomationAction[]
  created_by?: string
  created_at: string
  updated_at: string
  action_count?: number
}

/** Execution log for a single automation invocation. */
export interface AutomationRun {
  id: string
  automation_id: string
  collection_id: string
  record_id: string
  trigger_type: string
  status: 'success' | 'error' | 'skipped'
  error_message?: string
  duration_ms: number
  created_at: string
}

export interface CreateAutomationReq {
  name: string
  is_enabled: boolean
  trigger_type: TriggerType
  trigger_config?: Record<string, unknown>
  conditions: { field_slug: string, operator: ConditionOperator, value: string }[]
  actions: { action_type: ActionType, action_config: Record<string, unknown> }[]
}

// --- Webhook events ---

/** Inbound webhook event received from an external system. */
export interface WebhookEvent {
  id: string
  topic: string
  source: string
  payload: Record<string, unknown>
  processed: boolean
  received_at: string
}

// --- Envelope responses ---

/** Standard API response wrapper for single-item endpoints. */
export interface DataEnvelope<T> {
  data: T
  error?: string
}

/** Paginated API response wrapper for list endpoints. */
export interface ListEnvelope<T> {
  data: T[]
  total: number
  page: number
  limit: number
  total_pages: number
}
