// Package bid contains the Topbids-specific runtime logic that sits on top of
// the Topworks no-code schema engine: sealed field access control, auction
// scheduling, award/PO distribution actions.
//
// The schema-level declarations live in backend/internal/schema (SealedOptions,
// AccessConfig.BidRole). This package reads those declarations at runtime to
// enforce the bidding domain's invariants.
package bid

import (
	"strings"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// Roles that are considered "buyer-side" — procurement staff who manage RFQs
// and evaluate bids. They may view all bid rows, but cannot view sealed field
// values until the row's sealed condition is met.
//
// Administrators (director) additionally bypass sealing; they can always view
// sealed values for auditing. This is a deliberate policy: for internal-regulation
// bid systems (not public procurement), an administrator override is acceptable
// and all reads are audit-logged separately.
var (
	buyerRoles = map[string]bool{"pm": true, "engineer": true}
	adminRoles = map[string]bool{"admin": true, "director": true}
)

// MaskSealedFields walks the fields of a row and zeroes out any sealed-field
// values whose unlock condition is not yet met, mutating the row in place.
//
// The sealed condition unlocks (field becomes visible) when ANY of:
//   - the caller's role is in adminRoles (audit override)
//   - the row's status field value is in the field's unlock_by_status list
//   - sealed_until_at has been reached (absolute RFC3339 time or resolved
//     field reference). Field refs of the form "field:<slug>" read the
//     same row; "field:<rel>.<slug>" walks an already-expanded relation
//     (row[rel] must be a map)
//
// If sealed_until_at cannot be resolved (missing relation expansion, malformed
// value, non-time field), the field is masked — fail closed.
//
// statusField is the slug of the status column used for unlock_by_status
// comparison; pass "" to skip status-based unlocking.
func MaskSealedFields(
	fields []schema.Field,
	row map[string]any,
	statusField string,
	userRole string,
	now time.Time,
) {
	if adminRoles[userRole] {
		return
	}
	statusVal := statusAsString(row, statusField)
	for i := range fields {
		f := &fields[i]
		if masked, _ := shouldMaskField(f, statusVal, row, userRole, now); masked {
			row[f.Slug] = nil
		}
	}
}

// shouldMaskField reports whether a single field should be masked for the
// given caller/row context. Returns (masked, reason) where reason describes
// the unlock-miss for audit logs (not shown to end users).
//
// Non-sealed fields and error cases behave as documented on MaskSealedFields.
func shouldMaskField(
	f *schema.Field,
	status string,
	row map[string]any,
	userRole string,
	now time.Time,
) (bool, string) {
	opts, err := schema.ExtractSealedOptions(f.Options)
	if err != nil || opts == nil {
		return false, ""
	}
	if adminRoles[userRole] {
		return false, ""
	}
	// Status unlock: any match opens the field.
	for _, s := range opts.UnlockByStatus {
		if s == status {
			return false, ""
		}
	}
	// Time unlock: parse and compare.
	if opts.UntilAt != "" {
		t, ok := resolveUntilAt(opts.UntilAt, row)
		if !ok {
			return true, "sealed_until_at unresolved"
		}
		if !now.Before(t) {
			return false, ""
		}
		return true, "before sealed_until_at"
	}
	// Sealed without a time anchor: only status-based unlocking possible,
	// and it didn't match above.
	_ = buyerRoles // reserved for future role-based nuance
	return true, "status not in unlock list"
}

// resolveUntilAt parses sealed_until_at into a concrete time.
//
// Formats:
//   - "2026-05-01T00:00:00Z" — RFC3339 absolute
//   - "field:<slug>"         — read row[<slug>] as datetime
//   - "field:<rel>.<slug>"   — read row[<rel>][<slug>] as datetime
//     (requires the relation to have been expanded into an object)
//
// Multi-segment paths beyond two are supported (walks further nested objects)
// but rarely needed in practice.
func resolveUntilAt(v string, row map[string]any) (time.Time, bool) {
	if strings.HasPrefix(v, "field:") {
		path := strings.Split(strings.TrimPrefix(v, "field:"), ".")
		if len(path) == 0 {
			return time.Time{}, false
		}
		current := row
		for i, seg := range path {
			val, exists := current[seg]
			if !exists || val == nil {
				return time.Time{}, false
			}
			if i == len(path)-1 {
				return toTime(val)
			}
			next, ok := val.(map[string]any)
			if !ok {
				return time.Time{}, false
			}
			current = next
		}
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	return t, err == nil
}

// toTime coerces a JSON-decoded value to time.Time. Accepts time.Time
// directly (from pgx) or RFC3339 strings (from JSON).
func toTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		t, err := time.Parse(time.RFC3339, x)
		return t, err == nil
	}
	return time.Time{}, false
}

// statusAsString extracts the status field value as a string, with the usual
// JSON-decoded any-shapes handled. Returns "" for missing/null/non-string.
func statusAsString(row map[string]any, slug string) string {
	if slug == "" {
		return ""
	}
	v, ok := row[slug]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
