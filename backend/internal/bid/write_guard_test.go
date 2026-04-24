package bid

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// --- fakes ---

// stubRow implements pgx.Row by dispensing a canned list of values (or an
// error) from Scan. One stubRow per queued query.
type stubRow struct {
	values []any
	err    error
}

func (r *stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("stubRow: Scan arity mismatch")
	}
	for i, d := range dest {
		// Support *string (most queries), **string for nullable columns via
		// dereferenced nil, and **float64 for MIN aggregates.
		switch target := d.(type) {
		case *string:
			sv, ok := r.values[i].(string)
			if !ok {
				return errors.New("stubRow: non-string value staged")
			}
			*target = sv
		case **float64:
			// Handle three staging shapes for nullable MIN() results:
			// untyped nil (SQL NULL), a typed *float64 (already-pointer
			// value), or a raw float64 (non-null case).
			if r.values[i] == nil {
				*target = nil
				continue
			}
			if p, ok := r.values[i].(*float64); ok {
				*target = p
				continue
			}
			f, ok := r.values[i].(float64)
			if !ok {
				return errors.New("stubRow: non-float value staged for **float64")
			}
			*target = &f
		default:
			return errors.New("stubRow: unsupported Scan dest type")
		}
	}
	return nil
}

// fakeQuerier returns queued stubRows in FIFO order. Tests push expected
// results in the same order the guard will request them: first bid lookup
// (if update/delete), then rfq status.
type fakeQuerier struct {
	rows []*stubRow
	idx  int
}

func (f *fakeQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if f.idx >= len(f.rows) {
		return &stubRow{err: errors.New("fakeQuerier: no more rows queued")}
	}
	r := f.rows[f.idx]
	f.idx++
	return r
}

// --- tests ---

func TestEnforceBidWriteOwnership_NonSupplierPassesThrough(t *testing.T) {
	// Non-supplier roles must not reach the DB path. Passing nil as the
	// querier proves no query was attempted.
	for _, role := range []string{"director", "pm", "engineer", "viewer", ""} {
		err := EnforceBidWriteOwnership(context.Background(), nil, role, "", OpCreate, "", map[string]any{
			"rfq": "rfq-id",
		})
		if err != nil {
			t.Errorf("role %q should pass through, got %v", role, err)
		}
	}
}

func TestEnforceBidWriteOwnership_SupplierWithoutIDFailsClosed(t *testing.T) {
	err := EnforceBidWriteOwnership(context.Background(), nil, "supplier", "", OpCreate, "", map[string]any{})
	if !errors.Is(err, ErrSupplierNotLinked) {
		t.Errorf("got %v, want ErrSupplierNotLinked", err)
	}
}

func TestEnforceBidWriteOwnership_CreateRequiresRFQ(t *testing.T) {
	err := EnforceBidWriteOwnership(context.Background(), nil, "supplier", "sup-1", OpCreate, "", map[string]any{})
	if !errors.Is(err, ErrRFQMissing) {
		t.Errorf("got %v, want ErrRFQMissing", err)
	}
}

func TestEnforceBidWriteOwnership_CreateForcesSupplierOverride(t *testing.T) {
	// Anti-impersonation: even when the client sends a forged supplier id,
	// the body is overwritten with the caller's real supplier_id BEFORE any
	// DB check. We stub a happy-path RFQ status so the guard returns nil.
	body := map[string]any{
		"rfq":      "rfq-id",
		"supplier": "forged-other-supplier-id",
	}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusPublished, "open"}}, // rfq status query
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-real", OpCreate, "", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["supplier"] != "sup-real" {
		t.Errorf("supplier override missing: body[supplier] = %v, want sup-real", body["supplier"])
	}
}

func TestEnforceBidWriteOwnership_CreateBlockedOnClosedRFQ(t *testing.T) {
	body := map[string]any{"rfq": "rfq-id"}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusClosed, "open"}}, // past deadline — no new submissions
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body)
	if !errors.Is(err, ErrRFQNotAcceptingBids) {
		t.Errorf("got %v, want ErrRFQNotAcceptingBids", err)
	}
}

func TestEnforceBidWriteOwnership_UpdateRejectsForeignRow(t *testing.T) {
	// Supplier sup-1 tries to PATCH a bid owned by sup-2 — 403, before the
	// RFQ status check even runs (no second row queued).
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{"sup-2", "rfq-id"}}, // existing bid: supplier, rfq
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpUpdate, "bid-42", nil)
	if !errors.Is(err, ErrNotBidOwner) {
		t.Errorf("got %v, want ErrNotBidOwner", err)
	}
}

func TestEnforceBidWriteOwnership_UpdateAllowsOwnRowDuringPublished(t *testing.T) {
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{"sup-1", "rfq-id"}},  // own bid
		{values: []any{RFQStatusPublished, "open"}}, // RFQ still accepting
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpUpdate, "bid-1", nil)
	if err != nil {
		t.Errorf("should allow, got %v", err)
	}
}

func TestEnforceBidWriteOwnership_DeleteAfterOpenIsBlocked(t *testing.T) {
	// Even if supplier owns the row, once the RFQ has opened they can't
	// retract — preserves audit integrity.
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{"sup-1", "rfq-id"}},
		{values: []any{RFQStatusOpened, "open"}},
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpDelete, "bid-1", nil)
	if !errors.Is(err, ErrRFQNotAcceptingBids) {
		t.Errorf("got %v, want ErrRFQNotAcceptingBids", err)
	}
}

func TestEnforceBidWriteOwnership_UpdateMissingRowIsNotOwner(t *testing.T) {
	// A row the guard can't find (deleted, fake id, …) returns NotBidOwner
	// rather than NotFound so existence can't be probed by a supplier.
	q := &fakeQuerier{rows: []*stubRow{
		{err: pgx.ErrNoRows},
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpUpdate, "bid-missing", nil)
	if !errors.Is(err, ErrNotBidOwner) {
		t.Errorf("got %v, want ErrNotBidOwner", err)
	}
}

func TestEnforceBidWriteOwnership_CreateStripsForbiddenFields(t *testing.T) {
	// Supplier tries to write tech_score directly. Guard must strip it and
	// any other non-whitelisted key before the handler builds the INSERT.
	body := map[string]any{
		"rfq":          "rfq-id",
		"total_amount": 100.0,
		"tech_score":   99.9, // forbidden
		"price_score":  99.9, // forbidden
		"total_score":  99.9, // forbidden
		"status":       "awarded", // forbidden (status transitions are server-side)
	}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusPublished, "open"}},
	}}
	if err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, forbidden := range []string{"tech_score", "price_score", "total_score", "status"} {
		if _, present := body[forbidden]; present {
			t.Errorf("forbidden field %q should have been stripped; got body=%v", forbidden, body)
		}
	}
	if body["total_amount"] != 100.0 {
		t.Errorf("allowed field total_amount was lost: %v", body["total_amount"])
	}
}

func TestEnforceBidWriteOwnership_UpdateStripsForbiddenFields(t *testing.T) {
	body := map[string]any{
		"note":        "meh",
		"total_score": 1000.0, // forbidden
	}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{"sup-1", "rfq-id"}},
		{values: []any{RFQStatusPublished, "open"}},
	}}
	if err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpUpdate, "bid-1", body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := body["total_score"]; present {
		t.Error("total_score should be stripped on update")
	}
	if body["note"] != "meh" {
		t.Error("note should be preserved")
	}
}

func TestEnforceBidWriteOwnership_AuctionRejectsTieOrAbove(t *testing.T) {
	// Auction: new amount must be strictly lower than current best (900,000).
	body := map[string]any{"rfq": "rfq-id", "total_amount": 900_000.0}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusPublished, "auction"}}, // rfq status+mode
		{values: []any{900_000.0}},                     // MIN(total_amount)
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body)
	if !errors.Is(err, ErrAuctionUndercut) {
		t.Errorf("equal price should be rejected, got %v", err)
	}
}

func TestEnforceBidWriteOwnership_AuctionAcceptsStrictUndercut(t *testing.T) {
	body := map[string]any{"rfq": "rfq-id", "total_amount": 800_000.0}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusPublished, "auction"}},
		{values: []any{900_000.0}},
	}}
	if err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body); err != nil {
		t.Errorf("strict undercut should pass, got %v", err)
	}
}

func TestEnforceBidWriteOwnership_AuctionFirstBidHasNoFloor(t *testing.T) {
	// No prior bids → MIN returns NULL (nil *float64 in our stub). First bid accepts any positive.
	body := map[string]any{"rfq": "rfq-id", "total_amount": 1_000_000.0}
	var nilFloat *float64
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusPublished, "auction"}},
		{values: []any{nilFloat}},
	}}
	if err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body); err != nil {
		t.Errorf("first auction bid should be accepted, got %v", err)
	}
}

func TestEnforceBidWriteOwnership_AuctionClosedStillRejected(t *testing.T) {
	// Auction in closed status rejects like any other mode — undercut rule
	// is only relevant while RFQ is still accepting submissions.
	body := map[string]any{"rfq": "rfq-id", "total_amount": 1.0}
	q := &fakeQuerier{rows: []*stubRow{
		{values: []any{RFQStatusClosed, "auction"}},
	}}
	err := EnforceBidWriteOwnership(context.Background(), q, "supplier", "sup-1", OpCreate, "", body)
	if !errors.Is(err, ErrRFQNotAcceptingBids) {
		t.Errorf("closed auction must reject: got %v", err)
	}
}

func TestEnforceBidWriteOwnership_UnknownOpRejected(t *testing.T) {
	err := EnforceBidWriteOwnership(context.Background(), nil, "supplier", "sup-1", "weird-op", "", map[string]any{})
	if err == nil {
		t.Error("unknown op should error")
	}
}
