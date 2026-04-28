package handler

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$`)

// validatePayload checks user-supplied data against field definitions before
// it reaches the database. It catches problems that PG would otherwise raise
// as opaque errors (FK violations, type mismatches, check constraints) and
// returns a structured ErrInvalidInput so handleErr maps it to a 400.
//
// `isCreate` controls whether required fields must be present (only on POST).
func validatePayload(
	ctx context.Context,
	pool *pgxpool.Pool,
	cache *schema.Cache,
	body map[string]any,
	fields []schema.Field,
	isCreate bool,
) error {
	// Index fields for faster lookup.
	bySlug := make(map[string]schema.Field, len(fields))
	for _, f := range fields {
		bySlug[f.Slug] = f
	}

	// Reject unknown user-supplied fields (the API surface should match the schema).
	for k := range body {
		f, ok := bySlug[k]
		if ok {
			if f.FieldType.IsLayout() {
				return fmt.Errorf("%w: layout field %q cannot hold data", schema.ErrInvalidInput, k)
			}
			if f.FieldType.IsComputed() {
				return fmt.Errorf("%w: computed field %q cannot hold data", schema.ErrInvalidInput, k)
			}
			continue
		}
		// System columns the client may send (auto-managed by the server).
		switch k {
		case "id", "created_at", "updated_at", "deleted_at",
			"created_by", "updated_by", "_status", "_version", "_optimistic":
			continue
		}
		return fmt.Errorf("%w: unknown field %q", schema.ErrInvalidInput, k)
	}

	for _, f := range fields {
		if f.FieldType.IsLayout() || f.FieldType.IsComputed() {
			continue
		}
		v, present := body[f.Slug]

		if !present {
			if isCreate && f.IsRequired {
				label := fieldLabel(f)
				return fmt.Errorf("%w: field %s is required", schema.ErrInvalidInput, label)
			}
			continue
		}
		if v == nil {
			if f.IsRequired {
				label := fieldLabel(f)
				return fmt.Errorf("%w: field %s cannot be null", schema.ErrInvalidInput, label)
			}
			continue
		}

		if err := validateFieldValue(f, v); err != nil {
			label := fieldLabel(f)
			return fmt.Errorf("field %s: %w", label, err)
		}

		// Relation: confirm the target row(s) exist (and aren't soft-deleted).
		if f.FieldType == schema.FieldRelation && f.Relation != nil {
			if f.IsManyToMany() {
				// M:N: value is an array of UUID strings.
				arr, ok := v.([]any)
				if !ok {
					return fmt.Errorf("%w: field %q must be an array of UUID strings", schema.ErrInvalidInput, f.Slug)
				}
				for _, el := range arr {
					id, ok := el.(string)
					if !ok {
						return fmt.Errorf("%w: field %q array elements must be UUID strings", schema.ErrInvalidInput, f.Slug)
					}
					if err := checkRelationTarget(ctx, pool, cache, f.Relation.TargetCollectionID, id); err != nil {
						return fmt.Errorf("field %q: %w", f.Slug, err)
					}
				}
			} else {
				id, ok := v.(string)
				if !ok {
					return fmt.Errorf("%w: field %q must be a UUID string", schema.ErrInvalidInput, f.Slug)
				}
				if err := checkRelationTarget(ctx, pool, cache, f.Relation.TargetCollectionID, id); err != nil {
					return fmt.Errorf("field %q: %w", f.Slug, err)
				}
			}
		}

		// User: confirm the referenced user exists.
		if f.FieldType == schema.FieldUser {
			id, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: field %q must be a UUID string", schema.ErrInvalidInput, f.Slug)
			}
			if err := checkUserExists(ctx, pool, id); err != nil {
				return fmt.Errorf("field %q: %w", f.Slug, err)
			}
		}
	}
	return nil
}

func validateFieldValue(f schema.Field, v any) error {
	switch f.FieldType {
	case schema.FieldText, schema.FieldTextarea:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected string", schema.ErrInvalidInput)
		}
		if maxLen := schema.ExtractMaxLength(f.Options); maxLen > 0 && len([]rune(s)) > maxLen {
			return fmt.Errorf("%w: value exceeds maximum length of %d characters", schema.ErrInvalidInput, maxLen)
		}
	case schema.FieldNumber:
		num, ok := v.(float64)
		if !ok {
			return fmt.Errorf("%w: expected number", schema.ErrInvalidInput)
		}
		if rng := schema.ExtractNumberRange(f.Options); rng != nil {
			if rng.Min != nil && num < *rng.Min {
				return fmt.Errorf("%w: value %v is below minimum %v", schema.ErrInvalidInput, num, *rng.Min)
			}
			if rng.Max != nil && num > *rng.Max {
				return fmt.Errorf("%w: value %v exceeds maximum %v", schema.ErrInvalidInput, num, *rng.Max)
			}
		}
	case schema.FieldInteger:
		f64, ok := v.(float64)
		if !ok {
			return fmt.Errorf("%w: expected integer", schema.ErrInvalidInput)
		}
		if f64 != float64(int64(f64)) {
			return fmt.Errorf("%w: expected whole number, got %v", schema.ErrInvalidInput, f64)
		}
		if rng := schema.ExtractNumberRange(f.Options); rng != nil {
			if rng.Min != nil && f64 < *rng.Min {
				return fmt.Errorf("%w: value %v is below minimum %v", schema.ErrInvalidInput, f64, *rng.Min)
			}
			if rng.Max != nil && f64 > *rng.Max {
				return fmt.Errorf("%w: value %v exceeds maximum %v", schema.ErrInvalidInput, f64, *rng.Max)
			}
		}
	case schema.FieldBoolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%w: expected boolean", schema.ErrInvalidInput)
		}
	case schema.FieldDate:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected date string YYYY-MM-DD", schema.ErrInvalidInput)
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("%w: invalid date %q (expected YYYY-MM-DD)", schema.ErrInvalidInput, s)
		}
	case schema.FieldDatetime:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected RFC3339 datetime string", schema.ErrInvalidInput)
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return fmt.Errorf("%w: invalid datetime %q", schema.ErrInvalidInput, s)
		}
	case schema.FieldSelect:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected string for select", schema.ErrInvalidInput)
		}
		choices, err := schema.ExtractChoices(f.Options)
		if err != nil {
			return fmt.Errorf("%w: malformed options: %v", schema.ErrInvalidInput, err)
		}
		if !contains(choices, s) {
			return fmt.Errorf("%w: %q is not in allowed choices %v", schema.ErrInvalidInput, s, choices)
		}
	case schema.FieldMultiselect:
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%w: expected array for multiselect", schema.ErrInvalidInput)
		}
		choices, err := schema.ExtractChoices(f.Options)
		if err != nil {
			return fmt.Errorf("%w: malformed options: %v", schema.ErrInvalidInput, err)
		}
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("%w: multiselect items must be strings", schema.ErrInvalidInput)
			}
			if !contains(choices, s) {
				return fmt.Errorf("%w: %q is not in allowed choices %v", schema.ErrInvalidInput, s, choices)
			}
		}
	case schema.FieldTime:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected time string HH:MM or HH:MM:SS", schema.ErrInvalidInput)
		}
		if !timeRe.MatchString(s) {
			return fmt.Errorf("%w: invalid time %q (expected HH:MM or HH:MM:SS)", schema.ErrInvalidInput, s)
		}
	case schema.FieldRelation:
		if f.IsManyToMany() {
			arr, ok := v.([]any)
			if !ok {
				return fmt.Errorf("%w: expected array of UUID strings for M:N relation", schema.ErrInvalidInput)
			}
			for _, el := range arr {
				s, ok := el.(string)
				if !ok || !pgutil.ParseUUID(s).Valid {
					return fmt.Errorf("%w: invalid UUID in M:N relation array", schema.ErrInvalidInput)
				}
			}
		} else {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: expected UUID string", schema.ErrInvalidInput)
			}
			if !pgutil.ParseUUID(s).Valid {
				return fmt.Errorf("%w: invalid UUID %q", schema.ErrInvalidInput, s)
			}
		}
	case schema.FieldFile, schema.FieldUser:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%w: expected UUID string", schema.ErrInvalidInput)
		}
		if !pgutil.ParseUUID(s).Valid {
			return fmt.Errorf("%w: invalid UUID %q", schema.ErrInvalidInput, s)
		}
	case schema.FieldJSON, schema.FieldTable, schema.FieldSpreadsheet:
		// Any JSON value is acceptable; we just need to round-trip through encoding.
		_ = v
	default:
		return fmt.Errorf("%w: unhandled field type %q", schema.ErrInvalidInput, f.FieldType)
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// checkUserExists confirms the referenced user exists in auth.users.
func checkUserExists(ctx context.Context, pool *pgxpool.Pool, id string) error {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM auth.users WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify user: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: user %s does not exist", schema.ErrInvalidInput, id)
	}
	return nil
}

// checkTransitions verifies that any select field changes on a process-enabled
// collection comply with the defined transition rules.
func checkTransitions(
	oldRow map[string]any,
	body map[string]any,
	fields []schema.Field,
	userRole string,
) error {
	for _, f := range fields {
		if f.FieldType != schema.FieldSelect {
			continue
		}
		newVal, changing := body[f.Slug]
		if !changing {
			continue
		}

		transitions, err := schema.ExtractTransitions(f.Options)
		if err != nil || len(transitions) == 0 {
			continue
		}

		newStr, _ := newVal.(string)
		oldStr, _ := oldRow[f.Slug].(string)
		if newStr == oldStr {
			continue
		}

		// Find a matching transition rule.
		allowed := false
		for _, t := range transitions {
			if t.From == oldStr && t.To == newStr {
				for _, r := range t.AllowedRoles {
					if r == userRole {
						allowed = true
						break
					}
				}
				if !allowed {
					return fmt.Errorf("%w: role %q cannot transition %q from %q to %q",
						schema.ErrInvalidInput, userRole, f.Slug, oldStr, newStr)
				}
				break
			}
		}
		// If transitions are defined but no rule matches this from→to, block it.
		if !allowed {
			return fmt.Errorf("%w: transition %q → %q is not defined for field %q",
				schema.ErrInvalidInput, oldStr, newStr, f.Slug)
		}
	}
	return nil
}

// fieldLabel returns a human-readable identifier for a field (label if available, slug otherwise).
func fieldLabel(f schema.Field) string {
	if f.Label != "" {
		return fmt.Sprintf("%q (%s)", f.Label, f.Slug)
	}
	return fmt.Sprintf("%q", f.Slug)
}

// checkRelationTarget confirms the referenced row exists and is not soft-deleted.
func checkRelationTarget(ctx context.Context, pool *pgxpool.Pool, cache *schema.Cache, targetCollectionID, id string) error {
	target, ok := cache.CollectionByID(targetCollectionID)
	if !ok {
		return fmt.Errorf("%w: relation target collection %s not found", schema.ErrInvalidInput, targetCollectionID)
	}
	qTable := pgutil.QuoteQualified("data", target.Slug)
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+qTable+" WHERE id = $1 AND deleted_at IS NULL)",
		id,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify relation target: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: relation target %s does not exist in %s", schema.ErrInvalidInput, id, target.Slug)
	}
	return nil
}
