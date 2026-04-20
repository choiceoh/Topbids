package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	errInvalidUUID  = errors.New("invalid UUID format")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrInvalidInput = errors.New("invalid input")
)

// slugRe enforces the slug format: lowercase letters, digits, underscore; must start with a letter; max 63 chars.
// This constraint ensures slugs are safe for use as PostgreSQL identifiers (table/column names).
var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// PostgreSQL reserved words that cannot be used as table/column names.
var pgReserved = map[string]bool{
	"all": true, "analyse": true, "analyze": true, "and": true, "any": true,
	"array": true, "as": true, "asc": true, "asymmetric": true, "authorization": true,
	"between": true, "binary": true, "both": true, "case": true, "cast": true,
	"check": true, "collate": true, "column": true, "constraint": true, "create": true,
	"cross": true, "current_date": true, "current_role": true, "current_time": true,
	"current_timestamp": true, "current_user": true, "default": true, "deferrable": true,
	"desc": true, "distinct": true, "do": true, "else": true, "end": true, "except": true,
	"false": true, "fetch": true, "for": true, "foreign": true, "freeze": true,
	"from": true, "full": true, "grant": true, "group": true, "having": true, "ilike": true,
	"in": true, "index": true, "initially": true, "inner": true, "intersect": true,
	"into": true, "is": true, "isnull": true, "join": true, "lateral": true, "leading": true,
	"left": true, "like": true, "limit": true, "localtime": true, "localtimestamp": true,
	"natural": true, "not": true, "notnull": true, "null": true, "offset": true, "on": true,
	"only": true, "or": true, "order": true, "outer": true, "overlaps": true, "placing": true,
	"primary": true, "references": true, "returning": true, "right": true, "select": true,
	"session_user": true, "similar": true, "some": true, "symmetric": true, "table": true,
	"then": true, "to": true, "trailing": true, "true": true, "union": true, "unique": true,
	"user": true, "using": true, "variadic": true, "verbose": true, "when": true,
	"where": true, "window": true, "with": true,
}

// autoColumns are system columns automatically added to every dynamic data table by the engine.
// User-defined field slugs must not collide with these names.
var autoColumns = map[string]bool{
	"id": true, "created_at": true, "updated_at": true,
	"created_by": true, "updated_by": true, "deleted_at": true, "_status": true,
}

func ValidateSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("%w: slug must match [a-z][a-z0-9_]{0,62}", ErrInvalidInput)
	}
	if pgReserved[slug] {
		return fmt.Errorf("%w: %q is a PostgreSQL reserved word", ErrInvalidInput, slug)
	}
	if autoColumns[slug] {
		return fmt.Errorf("%w: %q is a reserved auto-column name", ErrInvalidInput, slug)
	}
	return nil
}

func ValidateFieldType(ft FieldType) error {
	if !ft.Valid() {
		return fmt.Errorf("%w: unknown field_type %q", ErrInvalidInput, ft)
	}
	return nil
}

// ValidateCollectionCreate checks the full collection creation request:
//   - Slug format (lowercase identifier, not a reserved word or auto-column)
//   - Label is non-empty
//   - Each inline field passes validateFieldIn (type-specific rules)
//   - No duplicate field slugs within the same collection
func ValidateCollectionCreate(req *CreateCollectionReq) error {
	if err := ValidateSlug(req.Slug); err != nil {
		return err
	}
	if strings.TrimSpace(req.Label) == "" {
		return fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	if req.AccessConfig != nil {
		if err := ValidateBidRole(req.AccessConfig.BidRole); err != nil {
			return err
		}
	}
	slugs := make(map[string]bool, len(req.Fields))
	for i := range req.Fields {
		if err := validateFieldIn(&req.Fields[i]); err != nil {
			return fmt.Errorf("fields[%d]: %w", i, err)
		}
		if slugs[req.Fields[i].Slug] {
			return fmt.Errorf("%w: duplicate field slug %q", ErrInvalidInput, req.Fields[i].Slug)
		}
		slugs[req.Fields[i].Slug] = true
	}
	return nil
}

// ValidateFieldCreate validates a single field creation request.
// Delegates to validateFieldIn for the actual per-type validation logic.
func ValidateFieldCreate(req *CreateFieldIn) error {
	return validateFieldIn(req)
}

// validWidths and validHeights define the allowed layout grid values.
var validWidths = map[int16]bool{1: true, 2: true, 3: true, 6: true}
var validHeights = map[int16]bool{1: true, 2: true, 3: true}

// validateFieldIn performs per-type validation on a field creation input:
//   - Validates slug format and label presence
//   - Layout fields: strips data constraints (is_required, is_unique, etc.) since they have no column
//   - Computed fields: strips constraints and validates type-specific options (formula expression, lookup/rollup config)
//   - Autonumber: normalizes to is_unique=true, no default
//   - Select/multiselect: requires non-empty, unique choices in Options; validates transitions if present
//   - Relation: requires target_collection_id, valid relation_type, and valid on_delete action
//   - Grid layout: validates width (1/2/3/6) and height (1/2/3) values
func validateFieldIn(f *CreateFieldIn) error {
	if err := ValidateSlug(f.Slug); err != nil {
		return err
	}
	if strings.TrimSpace(f.Label) == "" {
		return fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	if err := ValidateFieldType(f.FieldType); err != nil {
		return err
	}
	// Layout fields must not carry data constraints.
	if f.FieldType.IsLayout() {
		f.IsRequired = false
		f.IsUnique = false
		f.IsIndexed = false
		f.DefaultValue = nil
		return nil
	}
	// Computed fields: no DB column, no data constraints.
	if f.FieldType.IsComputed() {
		f.IsRequired = false
		f.IsUnique = false
		f.IsIndexed = false
		f.DefaultValue = nil
		if err := validateComputedOptions(f); err != nil {
			return err
		}
		return nil
	}
	// Autonumber: auto-managed, normalize constraints.
	if f.FieldType == FieldAutonumber {
		f.IsRequired = false
		f.IsUnique = true
		f.DefaultValue = nil
		return nil
	}
	if f.Width != 0 && !validWidths[f.Width] {
		return fmt.Errorf("%w: width must be 1, 2, 3, or 6", ErrInvalidInput)
	}
	if f.Height != 0 && !validHeights[f.Height] {
		return fmt.Errorf("%w: height must be 1, 2, or 3", ErrInvalidInput)
	}
	if f.FieldType == FieldRelation && f.Relation == nil {
		return fmt.Errorf("%w: relation field requires relation config", ErrInvalidInput)
	}
	if f.FieldType == FieldSelect || f.FieldType == FieldMultiselect {
		if err := validateSelectOptions(f.Options); err != nil {
			return fmt.Errorf("field %q: %w", f.Slug, err)
		}
	}
	if f.Relation != nil {
		if f.Relation.TargetCollectionID == "" {
			return fmt.Errorf("%w: relation.target_collection_id is required", ErrInvalidInput)
		}
		switch f.Relation.RelationType {
		case RelOneToOne, RelOneToMany, RelManyToMany:
		default:
			return fmt.Errorf("%w: invalid relation_type %q", ErrInvalidInput, f.Relation.RelationType)
		}
		if f.Relation.OnDelete != "" {
			switch strings.ToUpper(strings.TrimSpace(f.Relation.OnDelete)) {
			case "CASCADE", "SET NULL", "RESTRICT", "NO ACTION", "SET DEFAULT":
			default:
				return fmt.Errorf("%w: invalid on_delete %q", ErrInvalidInput, f.Relation.OnDelete)
			}
		}
	}
	// Sealed bid options (Topbids): structural validation regardless of
	// the parent collection's bid_role. Runtime enforcement happens only
	// when AccessConfig.BidRole == "bid".
	if err := ValidateSealedOptions(f.Options); err != nil {
		return fmt.Errorf("field %q: %w", f.Slug, err)
	}
	return nil
}

// SelectOptions is the expected shape of `options` for select/multiselect fields.
type SelectOptions struct {
	Choices []string `json:"choices"`
}

// NumberRange holds optional min/max constraints from a number field's options.
type NumberRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// ExtractNumberRange parses min/max constraints from a number or integer field's
// Options JSON (e.g. {"min": 0, "max": 100}). Returns nil if the options are
// empty, null, or contain neither min nor max.
func ExtractNumberRange(raw json.RawMessage) *NumberRange {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var opts NumberRange
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil
	}
	if opts.Min == nil && opts.Max == nil {
		return nil
	}
	return &opts
}

// ExtractMaxLength parses the max_length constraint from a text or textarea
// field's Options JSON (e.g. {"max_length": 255}). Returns 0 if not set or unparseable.
func ExtractMaxLength(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var opts struct {
		MaxLength int `json:"max_length"`
	}
	if err := json.Unmarshal(raw, &opts); err != nil {
		return 0
	}
	return opts.MaxLength
}

// ExtractChoices parses the "choices" array from a select/multiselect field's
// Options JSON (e.g. {"choices": ["A", "B", "C"]}). Returns nil if the options
// are empty or null. Used by both validation and the Dynamic API for value checking.
func ExtractChoices(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var opts SelectOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("options must be a JSON object with 'choices' array: %w", err)
	}
	return opts.Choices, nil
}

// ExtractTransitions returns the transitions from a select field's options, if any.
func ExtractTransitions(raw json.RawMessage) ([]Transition, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var opts SelectOptionsWithTransitions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, err
	}
	return opts.Transitions, nil
}

// ValidateTransitions checks that all transitions in a process-enabled select field are valid:
//   - Both From and To must reference existing choices (상태값이 유효한 선택지여야 함)
//   - From and To must differ (self-transitions are not allowed)
//   - AllowedRoles must be non-empty and contain only known roles (director, pm, engineer, viewer)
func ValidateTransitions(transitions []Transition, choices []string) error {
	choiceSet := make(map[string]bool, len(choices))
	for _, c := range choices {
		choiceSet[c] = true
	}
	validRoles := map[string]bool{"director": true, "pm": true, "engineer": true, "viewer": true}

	for i, t := range transitions {
		if !choiceSet[t.From] {
			return fmt.Errorf("%w: transitions[%d].from %q is not a valid choice", ErrInvalidInput, i, t.From)
		}
		if !choiceSet[t.To] {
			return fmt.Errorf("%w: transitions[%d].to %q is not a valid choice", ErrInvalidInput, i, t.To)
		}
		if t.From == t.To {
			return fmt.Errorf("%w: transitions[%d] from and to cannot be the same", ErrInvalidInput, i)
		}
		if len(t.AllowedRoles) == 0 {
			return fmt.Errorf("%w: transitions[%d].allowed_roles cannot be empty", ErrInvalidInput, i)
		}
		for _, role := range t.AllowedRoles {
			if !validRoles[role] {
				return fmt.Errorf("%w: transitions[%d] invalid role %q", ErrInvalidInput, i, role)
			}
		}
	}
	return nil
}

// FormulaOptions is the expected shape of `options` for formula fields.
type FormulaOptions struct {
	Expression string `json:"expression"`          // The formula expression (e.g. "{price} * {quantity}"); parsed by the formula engine.
	ResultType string `json:"result_type"`          // Output type: "number", "integer", "text", "boolean", or "date". Defaults to "number" if empty.
	Precision  *int   `json:"precision,omitempty"`  // Decimal places for number results; nil means no rounding.
}

func validateFormulaOptions(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("%w: formula field requires options.expression", ErrInvalidInput)
	}
	var opts FormulaOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return fmt.Errorf("%w: options must be a JSON object with 'expression': %v", ErrInvalidInput, err)
	}
	expr := strings.TrimSpace(opts.Expression)
	if expr == "" {
		return fmt.Errorf("%w: formula field requires a non-empty expression", ErrInvalidInput)
	}
	switch opts.ResultType {
	case "", "number", "integer", "text", "boolean", "date":
		// valid
	default:
		return fmt.Errorf("%w: invalid result_type %q; must be number, integer, text, boolean, or date", ErrInvalidInput, opts.ResultType)
	}
	return nil
}

// ExtractFormulaOptions parses the formula options from a raw JSON payload.
func ExtractFormulaOptions(raw json.RawMessage) (*FormulaOptions, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var opts FormulaOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// validateComputedOptions checks that computed field options are well-formed:
//   - Formula: requires non-empty expression and valid result_type
//   - Lookup: requires relation_field and target_field in Options
//   - Rollup: requires relation_field, target_field, and a valid aggregation function (SUM/COUNT/AVG/MIN/MAX/COUNTA)
func validateComputedOptions(f *CreateFieldIn) error {
	switch f.FieldType {
	case FieldFormula:
		if err := validateFormulaOptions(f.Options); err != nil {
			return fmt.Errorf("field %q: %w", f.Slug, err)
		}
	case FieldLookup:
		if len(f.Options) == 0 || string(f.Options) == "null" {
			return fmt.Errorf("%w: lookup field %q requires options", ErrInvalidInput, f.Slug)
		}
		var opts map[string]any
		if err := json.Unmarshal(f.Options, &opts); err != nil {
			return fmt.Errorf("%w: invalid options JSON: %v", ErrInvalidInput, err)
		}
		relField, _ := opts["relation_field"].(string)
		targetField, _ := opts["target_field"].(string)
		if relField == "" {
			return fmt.Errorf("%w: lookup field %q requires options.relation_field", ErrInvalidInput, f.Slug)
		}
		if targetField == "" {
			return fmt.Errorf("%w: lookup field %q requires options.target_field", ErrInvalidInput, f.Slug)
		}
	case FieldRollup:
		if len(f.Options) == 0 || string(f.Options) == "null" {
			return fmt.Errorf("%w: rollup field %q requires options", ErrInvalidInput, f.Slug)
		}
		var opts map[string]any
		if err := json.Unmarshal(f.Options, &opts); err != nil {
			return fmt.Errorf("%w: invalid options JSON: %v", ErrInvalidInput, err)
		}
		relField, _ := opts["relation_field"].(string)
		targetField, _ := opts["target_field"].(string)
		fn, _ := opts["function"].(string)
		if relField == "" {
			return fmt.Errorf("%w: rollup field %q requires options.relation_field", ErrInvalidInput, f.Slug)
		}
		if targetField == "" {
			return fmt.Errorf("%w: rollup field %q requires options.target_field", ErrInvalidInput, f.Slug)
		}
		validFns := map[string]bool{"SUM": true, "COUNT": true, "AVG": true, "MIN": true, "MAX": true, "COUNTA": true}
		if !validFns[strings.ToUpper(fn)] {
			return fmt.Errorf("%w: rollup field %q function must be one of SUM/COUNT/AVG/MIN/MAX/COUNTA", ErrInvalidInput, f.Slug)
		}
	}
	return nil
}

func validateSelectOptions(raw json.RawMessage) error {
	choices, err := ExtractChoices(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if len(choices) == 0 {
		return fmt.Errorf("%w: select field requires options.choices with at least one value", ErrInvalidInput)
	}
	seen := make(map[string]bool, len(choices))
	for _, c := range choices {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("%w: choice cannot be empty", ErrInvalidInput)
		}
		if seen[c] {
			return fmt.Errorf("%w: duplicate choice %q", ErrInvalidInput, c)
		}
		seen[c] = true
	}

	// Validate transitions if present.
	transitions, err := ExtractTransitions(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if len(transitions) > 0 {
		if err := ValidateTransitions(transitions, choices); err != nil {
			return err
		}
	}
	return nil
}
