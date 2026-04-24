package bid

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
)

// Audit action values. Each entry on _meta.bid_audit_log carries one of these
// in its action column. Keep values short and lowercase-underscore so they can
// be grouped/filtered with plain SQL.
const (
	ActionSubmit     = "submit"      // supplier inserted a bid row
	ActionReadSealed = "read_sealed" // caller viewed a bid row while sealed
	ActionReadOpened = "read_opened" // caller viewed a bid row after unlock
	ActionOpen       = "open"        // scheduler transitioned rfq → opened
	ActionAward      = "award"       // AwardRFQ picked a winner
	ActionDistribute = "distribute"  // DistributePO fanned out POs
	ActionCancel     = "cancel"      // admin cancelled an RFQ
	ActionWithdraw   = "withdraw"    // supplier retracted their bid
)

// execer is the minimal subset of pgx semantics the audit logger needs.
// Satisfied by *pgxpool.Pool and pgx.Tx — callers pick whichever matches
// the surrounding transaction context.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// AuditEntry is the input to LogEvent. ActorID and IP are optional (nil/"" are
// acceptable for background jobs like the scheduler). Detail is marshalled to
// JSON and stored as-is; callers should keep it compact.
type AuditEntry struct {
	ActorID   string         // auth.users.id; "" for system-triggered events
	ActorName string         // display name — denormalised for fast reads
	Action    string         // one of the Action* constants
	AppSlug   string         // collection slug (e.g. "bids", "rfqs")
	RowID     string         // target record id; "" when not applicable
	IP        string         // remote addr; "" to omit
	Detail    map[string]any // free-form context (eval_method, winner_id, …)
}

// LogEvent appends one row to _meta.bid_audit_log. It never returns an error:
// audit failures are logged via slog.Warn and the caller proceeds. Rationale:
// blocking a legitimate bid action on a secondary audit write is worse than a
// missing audit entry. The same choice is made by most compliance systems
// designed around the "log if you can" pattern.
//
// Pass db=*pgxpool.Pool for standalone calls, or db=pgx.Tx to scope the log
// insert to the same transaction as the caller — handy for ActionSubmit so
// the audit row and the bid row commit or fail together.
func LogEvent(ctx context.Context, db execer, e AuditEntry) {
	var actor any
	if e.ActorID != "" {
		actor = e.ActorID
	}
	var row any
	if e.RowID != "" {
		row = e.RowID
	}
	var ip any
	if e.IP != "" {
		ip = e.IP
	}
	var detail []byte
	if len(e.Detail) > 0 {
		b, err := json.Marshal(e.Detail)
		if err == nil {
			detail = b
		}
	}
	if len(detail) == 0 {
		detail = []byte(`{}`)
	}

	_, err := db.Exec(ctx, `
		INSERT INTO _meta.bid_audit_log
		  (actor_id, actor_name, action, app_slug, row_id, ip, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actor, e.ActorName, e.Action, e.AppSlug, row, ip, detail,
	)
	if err != nil {
		slog.Warn("bid audit: insert failed",
			"action", e.Action,
			"app_slug", e.AppSlug,
			"row_id", e.RowID,
			"error", err,
		)
	}
}
