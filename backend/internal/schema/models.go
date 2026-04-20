// Package schema defines the meta-model for Topworks collections (apps).
//
// Each collection maps to a real PostgreSQL table in the "data" schema.
// The meta-model (stored in _meta.collections, _meta.fields, _meta.relations)
// describes the structure of these dynamic tables.
//
// Field types fall into three categories:
//   - Regular fields: produce a DB column, support INSERT/UPDATE (text, number, date, select, relation, etc.)
//   - Layout fields: schema-only, no DB column (label, line, spacer) — for form layout ordering
//   - Computed fields: no DB column, calculated at query time (formula, lookup, rollup)
//
// Use NoColumn() to check whether a field type produces a DB column.
// Use IsLayout() and IsComputed() for finer-grained checks.
package schema

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/choiceoh/phaeton/backend/internal/pgutil"
)

// FieldType enumerates the supported column types.
type FieldType string

const (
	FieldText        FieldType = "text"        // Single-line text (VARCHAR-like).
	FieldTextarea    FieldType = "textarea"    // Multi-line text (TEXT column).
	FieldNumber      FieldType = "number"      // Decimal number (DOUBLE PRECISION).
	FieldInteger     FieldType = "integer"     // Whole number (BIGINT).
	FieldBoolean     FieldType = "boolean"     // True/false toggle (BOOLEAN).
	FieldDate        FieldType = "date"        // Calendar date without time (DATE).
	FieldDatetime    FieldType = "datetime"    // Timestamp with timezone (TIMESTAMPTZ).
	FieldTime        FieldType = "time"        // Time of day without date (TIME).
	FieldSelect      FieldType = "select"      // Single-choice dropdown; choices stored in Options JSON.
	FieldMultiselect FieldType = "multiselect" // Multi-choice; stored as JSONB array.
	FieldRelation    FieldType = "relation"    // Foreign key to another collection; see Relation struct.
	FieldUser        FieldType = "user"        // Reference to a platform user (UUID FK to _meta.users).
	FieldFile        FieldType = "file"        // File attachment; stores upload metadata as JSONB.
	FieldJSON        FieldType = "json"        // Arbitrary JSON value (JSONB column).

	FieldAutonumber  FieldType = "autonumber"  // Auto-incrementing sequence; managed by the engine.
	FieldTable       FieldType = "table"       // Inline repeating table stored as JSONB.
	FieldSpreadsheet FieldType = "spreadsheet" // Excel-like spreadsheet stored as JSONB.

	// Computed types — stored in _meta.fields but produce no DB column.
	// Values are calculated at read time from other fields/relations.
	FieldFormula FieldType = "formula"
	FieldLookup  FieldType = "lookup"
	FieldRollup  FieldType = "rollup"

	// Layout types — stored in _meta.fields for ordering but produce no DB column.
	FieldLabel  FieldType = "label"
	FieldLine   FieldType = "line"
	FieldSpacer FieldType = "spacer"
)

var validFieldTypes = map[FieldType]bool{
	FieldText: true, FieldTextarea: true, FieldNumber: true, FieldInteger: true,
	FieldBoolean: true, FieldDate: true, FieldDatetime: true, FieldTime: true,
	FieldSelect: true, FieldMultiselect: true, FieldRelation: true,
	FieldFile: true, FieldJSON: true, FieldUser: true, FieldAutonumber: true, FieldTable: true, FieldSpreadsheet: true,
	FieldFormula: true, FieldLookup: true, FieldRollup: true,
	FieldLabel: true, FieldLine: true, FieldSpacer: true,
}

func (ft FieldType) Valid() bool { return validFieldTypes[ft] }

// IsLayout returns true for field types that are purely visual (no DB column).
func (ft FieldType) IsLayout() bool {
	return ft == FieldLabel || ft == FieldLine || ft == FieldSpacer
}

// IsComputed returns true for field types that are calculated at read time (no DB column).
func (ft FieldType) IsComputed() bool {
	return ft == FieldFormula || ft == FieldLookup || ft == FieldRollup
}

// NoColumn returns true for field types that do not produce a DB column.
func (ft FieldType) NoColumn() bool {
	return ft.IsLayout() || ft.IsComputed()
}

// IsManyToMany returns true if this is a M:N relation field (no DB column).
func (f Field) IsManyToMany() bool {
	return f.FieldType == FieldRelation && f.Relation != nil && f.Relation.RelationType == RelManyToMany
}

// RelationType for inter-collection references.
type RelationType string

const (
	RelOneToOne   RelationType = "one_to_one"
	RelOneToMany  RelationType = "one_to_many"
	RelManyToMany RelationType = "many_to_many"
)

// AccessConfig defines per-collection role-based permissions.
// Each key maps an operation to a list of allowed roles.
// RLSMode controls row-level visibility for non-owner roles:
//   - ""/"none": no row filtering (default)
//   - "creator": viewers see only rows they created
//   - "department": viewers see rows created by users in their department
//   - "subsidiary": viewers see rows created by users in their subsidiary
//   - "filter": custom field-based filters (see RLSFilters)
//
// BidRole marks a collection as part of the bidding domain (Topbids).
// Empty means not a bid collection. Valid values are BidRoleRfq, BidRoleBid,
// BidRoleSupplier, BidRoleAward, BidRolePO (see bid_options.go).
// Only BidRoleBid triggers the row-level sealed filter at read time.
type AccessConfig struct {
	EntryView   []string    `json:"entry_view,omitempty"`
	EntryCreate []string    `json:"entry_create,omitempty"`
	EntryEdit   []string    `json:"entry_edit,omitempty"`
	EntryDelete []string    `json:"entry_delete,omitempty"`
	RLSMode     string      `json:"rls_mode,omitempty"`
	RLSFilters  []RLSFilter `json:"rls_filters,omitempty"`
	BidRole     string      `json:"bid_role,omitempty"`
}

// RLSFilter defines a custom field-based row filter for RLS "filter" mode.
// Field is the column slug in the dynamic table.
// Op is the comparison operator: eq, neq, in, contains.
// Value is the literal value to compare against, or a user attribute reference:
//
//	$user.id, $user.department_id, $user.subsidiary_id
type RLSFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// AllowsRole checks whether the given role is allowed the specified operation.
// An empty list means all authenticated roles are allowed.
func (ac AccessConfig) AllowsRole(operation, role string) bool {
	var allowed []string
	switch operation {
	case "entry_view":
		allowed = ac.EntryView
	case "entry_create":
		allowed = ac.EntryCreate
	case "entry_edit":
		allowed = ac.EntryEdit
	case "entry_delete":
		allowed = ac.EntryDelete
	default:
		return false
	}
	if len(allowed) == 0 {
		return true // no restriction
	}
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

// Collection is the top-level schema unit (maps to a PostgreSQL table in the data schema).
// The actual data table lives at wd_<slug> in the public schema.
type Collection struct {
	ID               string       `json:"id"`
	Slug             string       `json:"slug"`               // Immutable identifier; becomes the DB table suffix (wd_<slug>).
	Label            string       `json:"label"`               // Human-readable display name shown in UI.
	Description      string       `json:"description,omitempty"`
	Icon             string       `json:"icon,omitempty"`
	IsSystem         bool         `json:"is_system"`           // System collections (e.g. users, departments) cannot be deleted by end users.
	ProcessEnabled   bool         `json:"process_enabled"`     // When true, entries have a _status column and follow a workflow (프로세스 관리).
	SortOrder        int          `json:"sort_order"`          // Display ordering in the sidebar app list.
	TitleFieldID     string       `json:"title_field_id,omitempty"`     // Field used as the primary display title in views.
	DefaultSortField string       `json:"default_sort_field,omitempty"` // Field slug used as the default sort column.
	DefaultSortOrder string       `json:"default_sort_order,omitempty"` // "asc" or "desc" (default: "desc").
	AccessConfig     AccessConfig `json:"access_config"`       // Role-based + row-level security configuration.
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	CreatedBy        string       `json:"created_by,omitempty"`
	Fields           []Field      `json:"fields,omitempty"`    // Populated by GetCollection / cache; empty for list queries.
}

// Field defines a single column inside a collection.
// For layout and computed fields, no physical DB column exists — see NoColumn().
type Field struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Slug         string          `json:"slug"`
	Label        string          `json:"label"`
	FieldType    FieldType       `json:"field_type"`
	IsRequired   bool            `json:"is_required"`
	IsUnique     bool            `json:"is_unique"`
	IsIndexed    bool            `json:"is_indexed"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	Options      json.RawMessage `json:"options,omitempty"` // Type-specific config: choices for select, expression for formula, min/max for number, etc.
	Width        int16           `json:"width"`             // Form layout grid width (1, 2, 3, or 6 columns out of 6).
	Height       int16           `json:"height"`            // Form layout grid height (1, 2, or 3 rows).
	SortOrder    int             `json:"sort_order"`
	IsLayout     bool            `json:"is_layout"`         // True for label/line/spacer fields; denormalized from FieldType for query convenience.
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Relation     *Relation       `json:"relation,omitempty"` // Non-nil only when FieldType == FieldRelation.
}

// Relation describes how a relation-type field links to another collection.
// For 1:1 and 1:N relations, the source collection holds a UUID FK column.
// For M:N relations, a separate junction table stores the pairs (no FK column on either side).
type Relation struct {
	ID                 string       `json:"id"`
	FieldID            string       `json:"field_id"`
	TargetCollectionID string       `json:"target_collection_id"`
	RelationType       RelationType `json:"relation_type"`
	JunctionTable      string       `json:"junction_table,omitempty"` // Only set for M:N relations (M:N 관계에서만 사용되는 중간 테이블).
	OnDelete           string       `json:"on_delete"`               // PostgreSQL ON DELETE action: CASCADE, SET NULL, RESTRICT, etc.
}

// Transition defines a state transition in a process-enabled select field.
type Transition struct {
	From         string   `json:"from"`
	To           string   `json:"to"`
	AllowedRoles []string `json:"allowed_roles"`
}

// SelectOptionsWithTransitions extends SelectOptions with workflow transitions.
type SelectOptionsWithTransitions struct {
	Choices     []string     `json:"choices"`
	Transitions []Transition `json:"transitions,omitempty"`
}

// --- pgtype UUID helpers (thin wrappers around pgutil) ---

func uuidStr(u pgtype.UUID) string {
	return pgutil.UUIDToString(u)
}

// parseUUID wraps pgutil.ParseUUID but returns an error on malformed input
// so the store layer can distinguish missing values from invalid ones.
func parseUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	u := pgutil.ParseUUID(s)
	if !u.Valid {
		return pgtype.UUID{}, errInvalidUUID
	}
	return u, nil
}

// --- Request DTOs ---
//
// DTOs use pointer fields (*string, *bool) to distinguish "not provided" from zero-value
// in partial-update requests. The CreatedBy field uses `json:"-"` because it is set
// server-side by the handler from the JWT claims, never from client JSON.

// CreateCollectionReq is the input for creating a new collection (app).
type CreateCollectionReq struct {
	Slug         string          `json:"slug"`
	Label        string          `json:"label"`
	Description  string          `json:"description,omitempty"`
	Icon         string          `json:"icon,omitempty"`
	IsSystem     bool            `json:"is_system,omitempty"`
	AccessConfig *AccessConfig   `json:"access_config,omitempty"`
	Fields       []CreateFieldIn `json:"fields,omitempty"`
	CreatedBy    string          `json:"-"` // set by handler, not from JSON
}

type CreateFieldIn struct {
	Slug         string          `json:"slug"`
	Label        string          `json:"label"`
	FieldType    FieldType       `json:"field_type"`
	IsRequired   bool            `json:"is_required"`
	IsUnique     bool            `json:"is_unique"`
	IsIndexed    bool            `json:"is_indexed"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	Options      json.RawMessage `json:"options,omitempty"`
	Width        int16           `json:"width"`
	Height       int16           `json:"height"`
	Relation     *CreateRelIn    `json:"relation,omitempty"`
}

type CreateRelIn struct {
	TargetCollectionID string       `json:"target_collection_id"`
	RelationType       RelationType `json:"relation_type"`
	JunctionTable      string       `json:"junction_table,omitempty"`
	OnDelete           string       `json:"on_delete,omitempty"`
}

type UpdateCollectionReq struct {
	Label            *string       `json:"label,omitempty"`
	Description      *string       `json:"description,omitempty"`
	Icon             *string       `json:"icon,omitempty"`
	SortOrder        *int          `json:"sort_order,omitempty"`
	ProcessEnabled   *bool         `json:"process_enabled,omitempty"`
	AccessConfig     *AccessConfig `json:"access_config,omitempty"`
	TitleFieldID     *string       `json:"title_field_id,omitempty"`
	DefaultSortField *string       `json:"default_sort_field,omitempty"`
	DefaultSortOrder *string       `json:"default_sort_order,omitempty"`
}

type UpdateFieldReq struct {
	Label        *string         `json:"label,omitempty"`
	FieldType    *FieldType      `json:"field_type,omitempty"`
	IsRequired   *bool           `json:"is_required,omitempty"`
	IsUnique     *bool           `json:"is_unique,omitempty"`
	IsIndexed    *bool           `json:"is_indexed,omitempty"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	Options      json.RawMessage `json:"options,omitempty"`
	Width        *int16          `json:"width,omitempty"`
	Height       *int16          `json:"height,omitempty"`
}

// --- View ---

// View represents a saved view configuration for a collection.
type View struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Name         string          `json:"name"`
	ViewType     string          `json:"view_type"` // list, kanban, calendar, gallery
	Config       json.RawMessage `json:"config"`
	SortOrder    int             `json:"sort_order"`
	IsDefault    bool            `json:"is_default"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type CreateViewReq struct {
	Name      string          `json:"name"`
	ViewType  string          `json:"view_type"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder int             `json:"sort_order"`
	IsDefault bool            `json:"is_default"`
}

type UpdateViewReq struct {
	Name      *string         `json:"name,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder *int            `json:"sort_order,omitempty"`
	IsDefault *bool           `json:"is_default,omitempty"`
}

// --- Saved View ---

// SavedView persists a user's filter/sort/visibility configuration for a collection.
type SavedView struct {
	ID            string          `json:"id"`
	CollectionID  string          `json:"collection_id"`
	Name          string          `json:"name"`
	FilterConfig  json.RawMessage `json:"filter_config"`
	SortConfig    string          `json:"sort_config"`
	VisibleFields json.RawMessage `json:"visible_fields,omitempty"`
	IsDefault     bool            `json:"is_default"`
	IsPublic      bool            `json:"is_public"`
	CreatedBy     *string         `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type CreateSavedViewReq struct {
	Name          string          `json:"name"`
	FilterConfig  json.RawMessage `json:"filter_config,omitempty"`
	SortConfig    string          `json:"sort_config,omitempty"`
	VisibleFields json.RawMessage `json:"visible_fields,omitempty"`
	IsDefault     bool            `json:"is_default"`
	IsPublic      bool            `json:"is_public"`
}

type UpdateSavedViewReq struct {
	Name          *string         `json:"name,omitempty"`
	FilterConfig  json.RawMessage `json:"filter_config,omitempty"`
	SortConfig    *string         `json:"sort_config,omitempty"`
	VisibleFields json.RawMessage `json:"visible_fields,omitempty"`
	IsDefault     *bool           `json:"is_default,omitempty"`
	IsPublic      *bool           `json:"is_public,omitempty"`
}
