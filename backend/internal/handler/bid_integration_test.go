package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/events"
	"github.com/choiceoh/topbid/backend/internal/handler"
	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/migration"
	"github.com/choiceoh/topbid/backend/internal/schema"
	"github.com/choiceoh/topbid/backend/internal/seed"
	"github.com/choiceoh/topbid/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupBidHandlerRouter spins up a minimal HTTP surface backed by the real
// schema engine + seeded bid collections. Tests that need this skip when no
// test DB is available (via testutil.SetupDB).
//
// Returns the router, the pool (for direct SQL setup), an admin user id for
// seeding rows, and the supplier row id / supplier user id pair for the
// test's "own" supplier.
type bidEnv struct {
	router   *chi.Mux
	pool     *pgxpool.Pool
	adminID  string
	supplier string // data.suppliers.id belonging to the test's primary supplier
	other    string // a second supplier id used to simulate impersonation attempts
	rfqID    string // a published RFQ
}

func setupBidEnv(t *testing.T) bidEnv {
	t.Helper()
	pool := testutil.SetupDB(t)
	ctx := context.Background()

	// Schema: create the bid presets (rfqs/bids/suppliers/purchase_orders).
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(ctx); err != nil {
		t.Fatalf("cache load: %v", err)
	}
	engine := migration.NewEngine(pool, store, cache)
	if err := seed.Run(ctx, engine, cache); err != nil {
		t.Fatalf("seed.Run: %v", err)
	}

	// Admin user for seed bookkeeping.
	var adminID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO auth.users (email, name, password, role)
		VALUES ('bid-e2e-admin@test.local', 'admin', 'x', 'director')
		RETURNING id`,
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// Two supplier rows (master data).
	seedSupplier := func(name, bizNo string) string {
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO data.suppliers (name, biz_no, status, created_by)
			VALUES ($1, $2, 'active', $3) RETURNING id`,
			name, bizNo, adminID,
		).Scan(&id); err != nil {
			t.Fatalf("seed supplier: %v", err)
		}
		return id
	}
	mine := seedSupplier("MineCo", "111-11-11111")
	other := seedSupplier("OtherCo", "222-22-22222")

	// A published RFQ, deadline in the future so submissions are allowed.
	var rfqID string
	err := pool.QueryRow(ctx, `
		INSERT INTO data.rfqs (rfq_no, title, mode, eval_method, sealed,
		                       deadline_at, open_at, status, created_by)
		VALUES ('RFQ-WG-001', 'write-guard test', 'open', 'lowest', true,
		        $1, $2, 'published', $3)
		RETURNING id`,
		time.Now().Add(3*time.Hour), time.Now().Add(4*time.Hour), adminID,
	).Scan(&rfqID)
	if err != nil {
		t.Fatalf("seed rfq: %v", err)
	}

	// Router — only the endpoints the write guard touches.
	bus := events.NewBus()
	dyn := handler.NewDynHandler(pool, cache, bus)
	r := chi.NewRouter()
	r.Use(handler.WithRequestID)
	r.Get("/api/data/{slug}", dyn.List)
	r.Post("/api/data/{slug}", dyn.Create)
	r.Get("/api/data/{slug}/{id}", dyn.Get)
	r.Patch("/api/data/{slug}/{id}", dyn.Update)
	r.Delete("/api/data/{slug}/{id}", dyn.Delete)

	return bidEnv{
		router:   r,
		pool:     pool,
		adminID:  adminID,
		supplier: mine,
		other:    other,
		rfqID:    rfqID,
	}
}

func supplierClaims(userID, supplierID string) middleware.UserClaims {
	return middleware.UserClaims{
		UserID:     userID,
		Email:      "supplier@test.local",
		Role:       "supplier",
		Name:       "supplier",
		SupplierID: supplierID,
	}
}

// --- tests ---

func TestBidGuard_CreateForcesSupplierOverride(t *testing.T) {
	env := setupBidEnv(t)

	// A supplier posts a bid that tries to impersonate "other" AND sneak in
	// a tech_score (an internal evaluation field). The guard must force the
	// supplier column to the caller's real id and strip tech_score before
	// the INSERT fires.
	payload := map[string]any{
		"rfq":          env.rfqID,
		"supplier":     env.other, // attempted impersonation
		"total_amount": 1_000_000,
		"lead_time":    10,
		"tech_score":   99.9, // forbidden internal field
	}
	b, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/data/bids", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", w.Code, w.Body.String())
	}

	// Verify directly against the DB — can't trust whatever the response echos.
	var rowSupplier string
	var rowTech *float64
	if err := env.pool.QueryRow(context.Background(), `
		SELECT supplier::text, tech_score
		  FROM data.bids
		 WHERE rfq = $1 AND deleted_at IS NULL`,
		env.rfqID,
	).Scan(&rowSupplier, &rowTech); err != nil {
		t.Fatalf("read bid: %v", err)
	}
	if rowSupplier != env.supplier {
		t.Errorf("supplier override missed: got %s, want %s", rowSupplier, env.supplier)
	}
	if rowTech != nil {
		t.Errorf("tech_score should have been stripped, got %v", *rowTech)
	}
}

func TestBidGuard_UpdateForeignBidIs403(t *testing.T) {
	env := setupBidEnv(t)

	// Seed a bid owned by 'other' directly in the DB.
	var otherBidID string
	err := env.pool.QueryRow(context.Background(), `
		INSERT INTO data.bids (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, 5_000_000, 'submitted', now(), $3)
		RETURNING id`,
		env.rfqID, env.other, env.adminID,
	).Scan(&otherBidID)
	if err != nil {
		t.Fatalf("seed bid: %v", err)
	}

	// 'mine' tries to patch 'other's bid.
	body := `{"total_amount": 1}`
	req := httptest.NewRequest("PATCH", "/api/data/bids/"+otherBidID,
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestBidGuard_WriteBlockedWhenRFQClosed(t *testing.T) {
	env := setupBidEnv(t)

	// Move the RFQ to 'closed' — no new submissions allowed.
	if _, err := env.pool.Exec(context.Background(),
		`UPDATE data.rfqs SET status = 'closed' WHERE id = $1`, env.rfqID,
	); err != nil {
		t.Fatalf("close rfq: %v", err)
	}

	payload := map[string]any{
		"rfq":          env.rfqID,
		"supplier":     env.supplier,
		"total_amount": 1_000_000,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/data/bids", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 on closed RFQ, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestBidGuard_SupplierListOfSuppliersSeesSelfOnly(t *testing.T) {
	env := setupBidEnv(t)

	// Supplier lists /api/data/suppliers. Must return only their own row,
	// never competitor records — that would leak biz_no / email / phone.
	req := httptest.NewRequest("GET", "/api/data/suppliers", nil)
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list suppliers: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("supplier should see exactly 1 row (self), got %d", resp.Total)
	}
	if len(resp.Data) != 1 || resp.Data[0]["id"] != env.supplier {
		t.Errorf("expected only own id %s, got %v", env.supplier, resp.Data)
	}
}

func TestBidGuard_RfqListHidesInvitedFromSupplier(t *testing.T) {
	env := setupBidEnv(t)

	// Seed an invited-mode RFQ alongside the existing open-mode one.
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO data.rfqs (rfq_no, title, mode, eval_method, sealed,
		                        deadline_at, open_at, status, created_by)
		VALUES ('RFQ-INV-001', 'invited-only', 'invited', 'lowest', true,
		        now() + interval '3 hours', now() + interval '4 hours',
		        'published', $1)`,
		env.adminID,
	); err != nil {
		t.Fatalf("seed invited rfq: %v", err)
	}

	// Supplier lists RFQs — must see the open one but NOT the invited one.
	req := httptest.NewRequest("GET", "/api/data/rfqs", nil)
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list rfqs: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	sawOpen := false
	sawInvited := false
	for _, row := range resp.Data {
		switch row["mode"] {
		case "open":
			sawOpen = true
		case "invited", "private":
			sawInvited = true
		}
	}
	if !sawOpen {
		t.Error("supplier should see open-mode RFQs")
	}
	if sawInvited {
		t.Error("supplier must NOT see invited/private RFQs in discovery list")
	}

	// Same list for a director — should include both.
	dirReq := httptest.NewRequest("GET", "/api/data/rfqs", nil)
	dirReq = injectUser(dirReq, middleware.UserClaims{
		UserID: env.adminID, Role: "director", Name: "admin",
	})
	dirW := httptest.NewRecorder()
	env.router.ServeHTTP(dirW, dirReq)
	var dirResp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(dirW.Body.Bytes(), &dirResp)
	if len(dirResp.Data) < 2 {
		t.Errorf("director should see all RFQs, got %d rows", len(dirResp.Data))
	}
}

func TestBidGuard_SupplierCantSeeForeignBidViaGet(t *testing.T) {
	env := setupBidEnv(t)

	var otherBidID string
	err := env.pool.QueryRow(context.Background(), `
		INSERT INTO data.bids (rfq, supplier, total_amount, status, submitted_at, created_by)
		VALUES ($1, $2, 5_000_000, 'submitted', now(), $3)
		RETURNING id`,
		env.rfqID, env.other, env.adminID,
	).Scan(&otherBidID)
	if err != nil {
		t.Fatalf("seed bid: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/data/bids/"+otherBidID, nil)
	req = injectUser(req, supplierClaims(env.adminID, env.supplier))
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	// SupplierRowFilter applies a WHERE supplier = $me predicate; a query
	// for someone else's bid legitimately returns 404. (Returning 403 would
	// leak existence — the SELECT not matching any row is indistinguishable
	// from the row not existing, which is the desired property.)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body=%s)", w.Code, w.Body.String())
	}
}
