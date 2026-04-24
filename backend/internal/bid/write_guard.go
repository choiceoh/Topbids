package bid

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// rowQuerier is the minimum pgx surface EnforceBidWriteOwnership needs. Both
// *pgxpool.Pool and pgx.Tx satisfy it; so does a test fake. Kept narrow so
// unit tests can supply a simple stub without importing pgx pool internals.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Write operations recognised by EnforceBidWriteOwnership.
const (
	OpCreate = "create"
	OpUpdate = "update"
	OpDelete = "delete"
)

// supplierWritableBidFields lists every column a supplier may set on a
// `bids` row. Fields not in this set are stripped from the request body
// before the dynamic handler assembles its INSERT/UPDATE — critical to
// prevent a supplier from writing their own tech_score / price_score /
// total_score (buyer-only evaluation fields) and from overriding system
// bookkeeping columns.
//
// Deliberately does NOT include `status` — status transitions go through
// status mapping in the handler (always forced to 'submitted' on create),
// not user input.
var supplierWritableBidFields = map[string]struct{}{
	"rfq":          {}, // required relation; ownership check reads it
	"supplier":     {}, // overwritten to caller's supplier_id below
	"total_amount": {},
	"lead_time":    {},
	"valid_days":   {},
	"note":         {},
	"submitted_at": {}, // client may set; handler can also stamp
}

// ErrNotBidOwner is returned when a supplier-role caller tries to write to
// a bid row that doesn't belong to their company. We deliberately collapse
// "row not found" into this same error — returning 404 for missing rows
// would leak existence and let a supplier enumerate other companies' bid ids.
var ErrNotBidOwner = errors.New("forbidden: not the owner of this bid")

// ErrRFQNotAcceptingBids is returned when a supplier attempts to create,
// update, or delete a bid against an RFQ that has left the `published` state.
// After closing, opening, or awarding, bid values are locked — otherwise a
// supplier could revise their amount after seeing competitors unmask, which
// defeats the sealed-bid mechanism.
var ErrRFQNotAcceptingBids = errors.New("forbidden: RFQ is not accepting bid submissions")

// ErrSupplierNotLinked is returned when a role='supplier' user whose
// supplier_id is empty tries to write. A misconfigured account this way
// would otherwise bypass row filtering entirely, so we fail closed.
var ErrSupplierNotLinked = errors.New("forbidden: supplier account has no supplier_id")

// ErrRFQMissing is returned when the referenced RFQ id is absent from the
// request body (create) or is unresolvable from an existing row.
var ErrRFQMissing = errors.New("invalid: rfq is required")

// EnforceBidWriteOwnership applies the Topbids write invariants for
// supplier-role callers against the `bids` collection:
//
//  1. The caller's supplier_id must be non-empty (account integrity).
//  2. On create, the request's `supplier` field is force-set to the caller's
//     supplier_id — a supplier cannot impersonate another company even if
//     the client sends a forged value.
//  3. On update/delete, the existing row's `supplier` column must match the
//     caller's supplier_id.
//  4. The parent RFQ must be in `published` status. Any later state (closed,
//     opened, evaluating, awarded, failed, cancelled) locks the bid — the
//     deadline has passed and the sealed mechanism assumes immutable values.
//
// Non-supplier callers return nil with no side effects; role-level gating
// is handled by AccessConfig before this function runs.
//
// For op = OpCreate, body must contain a string `rfq` key. For update/delete,
// body is ignored and the existing row is loaded by bidRowID.
func EnforceBidWriteOwnership(
	ctx context.Context,
	db rowQuerier,
	callerRole, callerSupplierID, op, bidRowID string,
	body map[string]any,
) error {
	if callerRole != supplierRole {
		return nil
	}
	if callerSupplierID == "" {
		return ErrSupplierNotLinked
	}

	var rfqID string

	switch op {
	case OpCreate:
		// Strip any field outside the supplier-writable allowlist BEFORE
		// validating rfq presence, so a client that sneaks in tech_score
		// has the forbidden field silently dropped.
		for k := range body {
			if _, ok := supplierWritableBidFields[k]; !ok {
				delete(body, k)
			}
		}
		rfqID, _ = body["rfq"].(string)
		if rfqID == "" {
			return ErrRFQMissing
		}
		// Override supplier regardless of what the client sent. The handler
		// must use the returned body (same map, mutated in place) for the
		// actual INSERT.
		body["supplier"] = callerSupplierID

	case OpUpdate, OpDelete:
		// For update: same strip as create. Delete has no body.
		if op == OpUpdate && body != nil {
			for k := range body {
				if _, ok := supplierWritableBidFields[k]; !ok {
					delete(body, k)
				}
			}
		}
		var rowSupplier string
		err := db.QueryRow(ctx, `
			SELECT supplier::text, rfq::text
			  FROM data.bids
			 WHERE id = $1
			   AND deleted_at IS NULL`,
			bidRowID,
		).Scan(&rowSupplier, &rfqID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotBidOwner
			}
			return fmt.Errorf("load bid for ownership check: %w", err)
		}
		if rowSupplier != callerSupplierID {
			return ErrNotBidOwner
		}

	default:
		return fmt.Errorf("unknown op %q", op)
	}

	// RFQ must still be accepting submissions.
	var rfqStatus string
	err := db.QueryRow(ctx, `
		SELECT status
		  FROM data.rfqs
		 WHERE id = $1
		   AND deleted_at IS NULL`,
		rfqID,
	).Scan(&rfqStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRFQNotAcceptingBids
		}
		return fmt.Errorf("load rfq status: %w", err)
	}
	if rfqStatus != RFQStatusPublished {
		return ErrRFQNotAcceptingBids
	}

	return nil
}
