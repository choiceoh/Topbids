// Package bid contains the Topbids-specific runtime logic that sits on top of
// the Topworks no-code schema engine: sealed field access control, auction
// scheduling, award/PO distribution actions.
//
// The schema-level declarations live in backend/internal/schema (SealedOptions,
// AccessConfig.BidRole). This package reads those declarations at runtime to
// enforce the bidding domain's invariants.
package bid

import (
	"fmt"
	"strings"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// Role buckets. Separate because sealing and row-filtering apply differently:
//
//   - adminRoles: bypass all sealing (bid field values always visible) and
//     see all rows. Policy choice for internal-regulation systems.
//   - buyerRoles: see all rows but sealed values are masked until unlock.
//   - supplierRole: see only their own bid rows (WHERE supplier = user.SupplierID).
//     Sealed values on their own rows are always visible — they submitted them.
var (
	buyerRoles   = map[string]bool{"pm": true, "engineer": true}
	adminRoles   = map[string]bool{"admin": true, "director": true}
	supplierRole = "supplier"
)

// SupplierRowFilter returns a pgx-compatible SQL WHERE fragment restricting
// bid rows to those owned by the caller's supplier.
//
// Returns ("", nil, false) for non-supplier users — callers should skip the
// filter entirely. Returns (sql, args, true) when the user is a supplier
// with a SupplierID; the caller appends "AND <sql>" to its WHERE clause
// and extends its args with the returned slice.
//
// A supplier role user WITHOUT a SupplierID gets a contradictory filter
// (1=0) — fail closed. This shouldn't happen in practice (SeedSupplierUser
// enforces the pairing) but guards against misconfiguration.
func SupplierRowFilter(userRole, supplierID string, startIdx int) (sql string, args []any, active bool) {
	return supplierColumnFilter("supplier", userRole, supplierID, startIdx)
}

// RfqListModeFilter restricts the `rfqs` list endpoint for supplier-role
// callers to public ("open") RFQs. Invited/private RFQs are reached by
// direct link (the buyer shares /portal/rfqs/{id}/bid with chosen suppliers);
// they must NOT appear in the discovery list, otherwise any supplier could
// enumerate private tenders.
//
// Applies to List only. Get by ID still works so a supplier with the direct
// link can view the RFQ and submit — the write guard separately enforces
// published status, so invite-leak at Get is low-risk.
func RfqListModeFilter(userRole string) (sql string, active bool) {
	if userRole != supplierRole {
		return "", false
	}
	return "mode = 'open'", true
}

// SupplierSelfRowFilter restricts reads on the `suppliers` master collection
// so a supplier-role caller can only see their own company row. Without this,
// listing /api/data/suppliers would leak every competitor's business number,
// email, phone — a serious privacy regression.
//
// Same fail-closed semantics as SupplierRowFilter; wires against `id` rather
// than a `supplier` column because the suppliers collection IS the target.
func SupplierSelfRowFilter(userRole, supplierID string, startIdx int) (sql string, args []any, active bool) {
	return supplierColumnFilter("id", userRole, supplierID, startIdx)
}

func supplierColumnFilter(column, userRole, supplierID string, startIdx int) (string, []any, bool) {
	if userRole != supplierRole {
		return "", nil, false
	}
	if supplierID == "" {
		return "1=0", nil, true
	}
	return fmt.Sprintf("%s = $%d", column, startIdx), []any{supplierID}, true
}

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
	// Admins always see sealed values. Suppliers viewing their own rows
	// (filtered by SupplierRowFilter upstream) also see unmasked because
	// they submitted the values themselves.
	if adminRoles[userRole] || userRole == supplierRole {
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

// IsRowOpened reports whether the row's status value matches the unlock list
// of any sealed field on this collection. Used by audit logging to distinguish
// reads against still-sealed rows ('read_sealed') from reads after the RFQ
// was opened ('read_opened'), independent of the caller's role.
//
// Returns false if no sealed fields exist or if no UnlockByStatus is configured
// — i.e. audit callers should default to 'read_sealed' in ambiguous cases.
func IsRowOpened(fields []schema.Field, row map[string]any, statusField string) bool {
	status := statusAsString(row, statusField)
	if status == "" {
		return false
	}
	for i := range fields {
		opts, err := schema.ExtractSealedOptions(fields[i].Options)
		if err != nil || opts == nil {
			continue
		}
		for _, s := range opts.UnlockByStatus {
			if s == status {
				return true
			}
		}
	}
	return false
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
