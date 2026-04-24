package bid_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/bid"
	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
	"github.com/choiceoh/phaeton/backend/internal/seed"
	"github.com/choiceoh/phaeton/backend/internal/testutil"
)

// TestBidLifecycle_EndToEnd walks the entire Topbids backend pipeline from
// a published RFQ with submitted bids through to awarded state and per-
// subsidiary PO fan-out, and verifies that each stage writes the expected
// audit entries.
//
// Covers: scheduler transitions → AwardRFQ scoring → DistributePO split →
// _meta.bid_audit_log entries (open, submit, award, distribute).
//
// Skipped when a test Postgres is unreachable — see testutil.SetupDB.
func TestBidLifecycle_EndToEnd(t *testing.T) {
	pool := testutil.SetupDB(t)
	ctx := context.Background()

	cache, _ := setupBidSchema(t, ctx, pool)
	adminID := seedAdminUser(t, ctx, pool)
	subIDs := seedSubsidiaries(t, ctx, pool, "계열사 A", "계열사 B")
	supplierIDs := seedSuppliers(t, ctx, pool, adminID)

	// RFQ: deadline and open_at both in the past so a single scheduler tick
	// moves published → closed → opened.
	rfqID := seedRFQ(t, ctx, pool, adminID, rfqFixture{
		No:         "RFQ-E2E-001",
		Title:      "E2E 테스트 공고",
		Category:   "자재",
		Mode:       "open",
		EvalMethod: bid.EvalMethodLowest,
		Sealed:     true,
		DeadlineAt: time.Now().Add(-2 * time.Hour),
		OpenAt:     time.Now().Add(-1 * time.Hour),
		Status:     bid.RFQStatusPublished,
	})

	// Two bids at different prices — lowest wins under eval_method=lowest.
	// The pricier one must end up 'rejected'. submitted_at ordering matters
	// only for tie-breaks, which don't apply here.
	winnerBidID := seedBid(t, ctx, pool, adminID, rfqID, supplierIDs[0], 10_000_000)
	loserBidID := seedBid(t, ctx, pool, adminID, rfqID, supplierIDs[1], 12_000_000)

	bus := events.NewBus()
	sched := bid.NewScheduler(pool, cache, bus, time.Hour)

	// One tick runs both transitions: deadline→closed, then closed→opened.
	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("scheduler tick: %v", err)
	}

	assertRFQStatus(t, ctx, pool, rfqID, bid.RFQStatusOpened)

	// Award the RFQ. With eval_method=lowest, the 10M bid wins.
	result, err := bid.AwardRFQ(ctx, pool, rfqID, bid.Actor{
		UserID: adminID,
		Name:   "E2E Admin",
		IP:     "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("AwardRFQ: %v", err)
	}

	if result.WinnerBidID != winnerBidID {
		t.Errorf("winner = %s, want %s (cheapest bid)", result.WinnerBidID, winnerBidID)
	}
	if result.WinnerAmount != 10_000_000 {
		t.Errorf("winner amount = %v, want 10000000", result.WinnerAmount)
	}
	if len(result.RejectedBids) != 1 || result.RejectedBids[0] != loserBidID {
		t.Errorf("rejected = %v, want [%s]", result.RejectedBids, loserBidID)
	}
	if result.Distribution == nil {
		t.Fatal("award did not populate Distribution")
	}
	if result.Distribution.SubsidiaryCount != len(subIDs) {
		t.Errorf("PO subsidiary count = %d, want %d", result.Distribution.SubsidiaryCount, len(subIDs))
	}

	// Final DB state assertions.
	assertBidStatus(t, ctx, pool, winnerBidID, bid.BidStatusAwarded)
	assertBidStatus(t, ctx, pool, loserBidID, bid.BidStatusRejected)
	assertRFQStatus(t, ctx, pool, rfqID, bid.RFQStatusAwarded)
	assertPOCount(t, ctx, pool, rfqID, len(subIDs))

	// Audit log assertions. One 'open' (scheduler), one 'award' (AwardRFQ),
	// one 'distribute' (DistributePO). Submit events aren't recorded here
	// because bids were inserted via raw SQL (bypassing DynHandler.Create) —
	// the test for submit logging lives in the handler integration test.
	counts := auditCountsByAction(t, ctx, pool)
	if counts[bid.ActionOpen] != 1 {
		t.Errorf("audit 'open' count = %d, want 1", counts[bid.ActionOpen])
	}
	if counts[bid.ActionAward] != 1 {
		t.Errorf("audit 'award' count = %d, want 1", counts[bid.ActionAward])
	}
	if counts[bid.ActionDistribute] != 1 {
		t.Errorf("audit 'distribute' count = %d, want 1", counts[bid.ActionDistribute])
	}
}

// --- helpers ---

type rfqFixture struct {
	No         string
	Title      string
	Category   string
	Mode       string
	EvalMethod string
	Sealed     bool
	DeadlineAt time.Time
	OpenAt     time.Time
	Status     string
}

// setupBidSchema runs the bid-domain seed.Run() against a freshly truncated DB
// to create rfqs / suppliers / bids / purchase_orders collections along with
// their relations. Returns cache and engine for further use.
func setupBidSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (*schema.Cache, *migration.Engine) {
	t.Helper()
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(ctx); err != nil {
		t.Fatalf("cache load: %v", err)
	}
	engine := migration.NewEngine(pool, store, cache)
	if err := seed.Run(ctx, engine, cache); err != nil {
		t.Fatalf("seed.Run: %v", err)
	}
	return cache, engine
}

// seedAdminUser inserts a director user and returns the generated id.
func seedAdminUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('e2e-admin@example.test', 'E2E Admin', 'unused', 'director')
		RETURNING id`,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return id
}

func seedSubsidiaries(t *testing.T, ctx context.Context, pool *pgxpool.Pool, names ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(names))
	for i, n := range names {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO auth.subsidiaries (name, sort_order, is_active)
			VALUES ($1, $2, true)
			RETURNING id`,
			n, i,
		).Scan(&id)
		if err != nil {
			t.Fatalf("seed subsidiary %s: %v", n, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func seedSuppliers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, adminID string) []string {
	t.Helper()
	fixtures := []struct {
		name  string
		bizNo string
	}{
		{"공급사1", "111-11-11111"},
		{"공급사2", "222-22-22222"},
	}
	ids := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO data.suppliers (name, biz_no, status, created_by)
			VALUES ($1, $2, 'active', $3)
			RETURNING id`,
			f.name, f.bizNo, adminID,
		).Scan(&id)
		if err != nil {
			t.Fatalf("seed supplier %s: %v", f.name, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func seedRFQ(t *testing.T, ctx context.Context, pool *pgxpool.Pool, adminID string, r rfqFixture) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO data.rfqs
		  (rfq_no, title, category, mode, eval_method, sealed,
		   deadline_at, open_at, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		r.No, r.Title, r.Category, r.Mode, r.EvalMethod, r.Sealed,
		r.DeadlineAt, r.OpenAt, r.Status, adminID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	return id
}

func seedBid(t *testing.T, ctx context.Context, pool *pgxpool.Pool, adminID, rfqID, supplierID string, totalAmount float64) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO data.bids
		  (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, $3, $4, now(), $5)
		RETURNING id`,
		rfqID, supplierID, totalAmount, bid.BidStatusSubmitted, adminID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed bid: %v", err)
	}
	return id
}

func assertRFQStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rfqID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM data.rfqs WHERE id = $1`, rfqID).Scan(&got); err != nil {
		t.Fatalf("read rfq status: %v", err)
	}
	if got != want {
		t.Errorf("rfq status = %q, want %q", got, want)
	}
}

func assertBidStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bidID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM data.bids WHERE id = $1`, bidID).Scan(&got); err != nil {
		t.Fatalf("read bid status: %v", err)
	}
	if got != want {
		t.Errorf("bid %s status = %q, want %q", bidID, got, want)
	}
}

func assertPOCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rfqID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM data.purchase_orders WHERE rfq = $1`, rfqID).Scan(&got); err != nil {
		t.Fatalf("count POs: %v", err)
	}
	if got != want {
		t.Errorf("PO count = %d, want %d", got, want)
	}
}

func auditCountsByAction(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT action, COUNT(*) FROM _meta.bid_audit_log GROUP BY action`)
	if err != nil {
		t.Fatalf("audit counts: %v", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var action string
		var n int
		if err := rows.Scan(&action, &n); err != nil {
			t.Fatalf("scan audit: %v", err)
		}
		counts[action] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("audit rows: %v", err)
	}
	return counts
}

// silence "unused import" when helpers grow — kept here so failing
// assertions produce readable output like "status=draft, wanted=closed"
// without stripping Korean label characters.
var _ = fmt.Sprintf
var _ = strings.Contains
