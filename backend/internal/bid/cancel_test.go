package bid_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/bid"
	"github.com/choiceoh/phaeton/backend/internal/seed"
	"github.com/choiceoh/phaeton/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// Integration tests for CancelRFQ / WithdrawBid. DB-dependent; skipped if
// test postgres is unreachable (via testutil.SetupDB).

func seedRFQWithStatus(t *testing.T, pool *pgxpool.Pool, adminID, status string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO data.rfqs (rfq_no, title, mode, eval_method, sealed,
		                        deadline_at, open_at, status, created_by)
		VALUES ('RFQ-CAN-'||substr(md5(random()::text),0,6), 'cancel-test',
		        'open', 'lowest', true, $1, $2, $3, $4)
		RETURNING id`,
		time.Now().Add(1*time.Hour), time.Now().Add(2*time.Hour), status, adminID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	return id
}

func setupBidSchemaMin(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(ctx); err != nil {
		t.Fatalf("cache load: %v", err)
	}
	engine := migration.NewEngine(pool, store, cache)
	if err := seed.Run(ctx, engine, cache); err != nil {
		t.Fatalf("seed.Run: %v", err)
	}
}

func TestCancelRFQ_RejectsOutstandingBidsAndLogsAudit(t *testing.T) {
	pool := testutil.SetupDB(t)
	setupBidSchemaMin(t, pool)

	var adminID string
	pool.QueryRow(context.Background(), `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('cancel-admin@test.local', 'admin', 'x', 'director')
		RETURNING id`,
	).Scan(&adminID)

	var supplierID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.suppliers (name, biz_no, status, created_by)
		VALUES ('s', '999-99-99999', 'active', $1) RETURNING id`, adminID,
	).Scan(&supplierID)

	rfqID := seedRFQWithStatus(t, pool, adminID, bid.RFQStatusPublished)

	// Outstanding submitted bid — must become rejected after cancel.
	var bidID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.bids (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, 1000, 'submitted', now(), $3) RETURNING id`,
		rfqID, supplierID, adminID,
	).Scan(&bidID)

	err := bid.CancelRFQ(context.Background(), pool, rfqID, "duplicate submission",
		bid.Actor{UserID: adminID, Name: "admin"})
	if err != nil {
		t.Fatalf("CancelRFQ: %v", err)
	}

	// Assertions.
	var rfqStatus, bidStatus string
	pool.QueryRow(context.Background(), `SELECT status FROM data.rfqs WHERE id = $1`, rfqID).
		Scan(&rfqStatus)
	pool.QueryRow(context.Background(), `SELECT status FROM data.bids WHERE id = $1`, bidID).
		Scan(&bidStatus)
	if rfqStatus != bid.RFQStatusCancelled {
		t.Errorf("rfq status = %q, want cancelled", rfqStatus)
	}
	if bidStatus != bid.BidStatusRejected {
		t.Errorf("bid status = %q, want rejected", bidStatus)
	}

	// Audit row with reason must exist.
	var auditCount int
	pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM _meta.bid_audit_log
		 WHERE action = $1 AND row_id = $2 AND detail->>'reason' = $3`,
		bid.ActionCancel, rfqID, "duplicate submission",
	).Scan(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 cancel audit row with reason, got %d", auditCount)
	}
}

func TestCancelRFQ_RefusesTerminalStatus(t *testing.T) {
	pool := testutil.SetupDB(t)
	setupBidSchemaMin(t, pool)

	var adminID string
	pool.QueryRow(context.Background(), `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('cancel-admin2@test.local', 'admin2', 'x', 'director')
		RETURNING id`,
	).Scan(&adminID)

	for _, terminal := range []string{bid.RFQStatusAwarded, bid.RFQStatusCancelled, bid.RFQStatusFailed} {
		rfqID := seedRFQWithStatus(t, pool, adminID, terminal)
		err := bid.CancelRFQ(context.Background(), pool, rfqID, "", bid.Actor{})
		if !errors.Is(err, bid.ErrRFQNotCancellable) {
			t.Errorf("status=%q: want ErrRFQNotCancellable, got %v", terminal, err)
		}
	}
}

func TestWithdrawBid_BlocksOnClosedRFQ(t *testing.T) {
	pool := testutil.SetupDB(t)
	setupBidSchemaMin(t, pool)

	var adminID string
	pool.QueryRow(context.Background(), `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('wd-admin@test.local', 'admin', 'x', 'director')
		RETURNING id`,
	).Scan(&adminID)
	var supplierID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.suppliers (name, biz_no, status, created_by)
		VALUES ('w', '888-88-88888', 'active', $1) RETURNING id`, adminID,
	).Scan(&supplierID)
	rfqID := seedRFQWithStatus(t, pool, adminID, bid.RFQStatusClosed)

	var bidID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.bids (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, 1000, 'submitted', now(), $3) RETURNING id`,
		rfqID, supplierID, adminID,
	).Scan(&bidID)

	err := bid.WithdrawBid(context.Background(), pool, bidID, bid.Actor{UserID: adminID})
	if !errors.Is(err, bid.ErrBidNotWithdrawable) {
		t.Errorf("want ErrBidNotWithdrawable, got %v", err)
	}
}

func TestWithdrawBid_IdempotentOnAlreadyWithdrawn(t *testing.T) {
	pool := testutil.SetupDB(t)
	setupBidSchemaMin(t, pool)

	var adminID string
	pool.QueryRow(context.Background(), `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('wd-admin2@test.local', 'admin2', 'x', 'director')
		RETURNING id`,
	).Scan(&adminID)
	var supplierID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.suppliers (name, biz_no, status, created_by)
		VALUES ('w2', '777-77-77777', 'active', $1) RETURNING id`, adminID,
	).Scan(&supplierID)
	rfqID := seedRFQWithStatus(t, pool, adminID, bid.RFQStatusPublished)

	var bidID string
	pool.QueryRow(context.Background(), `
		INSERT INTO data.bids (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, 1000, 'withdrawn', now(), $3) RETURNING id`,
		rfqID, supplierID, adminID,
	).Scan(&bidID)

	// Already withdrawn — second call must succeed without touching state.
	if err := bid.WithdrawBid(context.Background(), pool, bidID, bid.Actor{}); err != nil {
		t.Errorf("second withdraw should be idempotent, got %v", err)
	}
}
