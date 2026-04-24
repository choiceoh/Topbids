package bid

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeExecer captures a single Exec call for assertion. The execErr field
// lets individual tests simulate DB failures without touching a real pool.
type fakeExecer struct {
	called  bool
	sql     string
	args    []any
	execErr error
}

func (f *fakeExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.called = true
	f.sql = sql
	f.args = args
	return pgconn.CommandTag{}, f.execErr
}

func TestLogEvent_InsertsExpectedColumns(t *testing.T) {
	fx := &fakeExecer{}
	LogEvent(context.Background(), fx, AuditEntry{
		ActorID:   "00000000-0000-0000-0000-000000000001",
		ActorName: "alice",
		Action:    ActionSubmit,
		AppSlug:   "bids",
		RowID:     "00000000-0000-0000-0000-000000000042",
		IP:        "10.0.0.1",
		Detail:    map[string]any{"note": "first"},
	})

	if !fx.called {
		t.Fatal("Exec not called")
	}
	if len(fx.args) != 7 {
		t.Fatalf("expected 7 args (actor, name, action, slug, row, ip, detail), got %d", len(fx.args))
	}
	if fx.args[0] != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("actor_id arg = %v", fx.args[0])
	}
	if fx.args[2] != ActionSubmit {
		t.Errorf("action arg = %v, want %q", fx.args[2], ActionSubmit)
	}
	if fx.args[3] != "bids" {
		t.Errorf("app_slug arg = %v", fx.args[3])
	}
	// Detail is marshalled to JSON bytes; verify round-trip.
	raw, ok := fx.args[6].([]byte)
	if !ok {
		t.Fatalf("detail arg type = %T, want []byte", fx.args[6])
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("detail not valid json: %v", err)
	}
	if back["note"] != "first" {
		t.Errorf("detail round-trip lost value: %v", back)
	}
}

func TestLogEvent_OmitsOptionalFields(t *testing.T) {
	// Background job style: no actor, no row, no IP.
	fx := &fakeExecer{}
	LogEvent(context.Background(), fx, AuditEntry{
		ActorName: "bid-scheduler",
		Action:    ActionOpen,
		AppSlug:   "rfqs",
	})
	if fx.args[0] != nil {
		t.Errorf("actor_id should be nil when empty, got %v", fx.args[0])
	}
	if fx.args[4] != nil {
		t.Errorf("row_id should be nil when empty, got %v", fx.args[4])
	}
	if fx.args[5] != nil {
		t.Errorf("ip should be nil when empty, got %v", fx.args[5])
	}
	// Detail defaults to "{}" so the JSONB column stays non-null.
	if string(fx.args[6].([]byte)) != "{}" {
		t.Errorf("detail default = %q, want {}", fx.args[6])
	}
}

func TestLogEvent_SwallowsExecError(t *testing.T) {
	// Audit failure must not panic or propagate — business action already committed.
	fx := &fakeExecer{execErr: errors.New("pool closed")}
	LogEvent(context.Background(), fx, AuditEntry{
		Action:  ActionAward,
		AppSlug: "rfqs",
	})
	if !fx.called {
		t.Error("LogEvent should still attempt the insert on failure")
	}
}

func TestActionConstants_AreStable(t *testing.T) {
	// Guards against a careless rename — action strings are written to a DB
	// table that outlives code refactors. Changing these requires a migration.
	cases := map[string]string{
		ActionSubmit:     "submit",
		ActionReadSealed: "read_sealed",
		ActionReadOpened: "read_opened",
		ActionOpen:       "open",
		ActionAward:      "award",
		ActionDistribute: "distribute",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("action %q drifted from %q", got, want)
		}
	}
}
