package bid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Audit actions introduced by the A/B feature fills.
const (
	ActionAmend     = "amend"      // admin edited a published RFQ
	ActionNotify    = "notify"     // system broadcast to bidders
	ActionCloneRFQ  = "clone_rfq"  // admin duplicated a template or past RFQ
	ActionEvaluate  = "evaluate"   // moved to evaluating / qualification review
	ActionResolve   = "resolve"    // resolved reserve_price (opened multiple reserve)
)

// ErrAmendOnTerminalRFQ blocks amendments once an RFQ has reached a terminal
// state. Amending a cancelled/awarded RFQ would contradict the audit trail
// (POs already generated, suppliers already notified of the outcome).
var ErrAmendOnTerminalRFQ = errors.New("rfq is in a terminal state and cannot be amended")

// AmendRFQ records a buyer-initiated change to a published RFQ and notifies
// every supplier who has already submitted a bid. Meant for typos, deadline
// extensions, spec corrections — not major scope rewrites.
//
// The note is stored verbatim on the RFQ (rolling log — most recent wins
// in amendment_note, count bumps, last_amended_at stamps) so suppliers can
// see what changed without trawling audit logs.
func AmendRFQ(ctx context.Context, pool *pgxpool.Pool, rfqID, note string, actor Actor) error {
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM data.rfqs WHERE id = $1 AND deleted_at IS NULL`, rfqID,
	).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRFQNotFound
		}
		return fmt.Errorf("load rfq: %w", err)
	}
	switch status {
	case RFQStatusAwarded, RFQStatusCancelled, RFQStatusFailed:
		return fmt.Errorf("%w: status=%q", ErrAmendOnTerminalRFQ, status)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE data.rfqs
		   SET amendment_count = COALESCE(amendment_count, 0) + 1,
		       last_amended_at = now(),
		       amendment_note = $2,
		       updated_at = now()
		 WHERE id = $1`,
		rfqID, strings.TrimSpace(note),
	); err != nil {
		return fmt.Errorf("amend rfq: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "amend-rfq"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionAmend,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		IP:        actor.IP,
		Detail:    map[string]any{"note": note},
	})

	// Best-effort notification; swallow errors so the amend itself is durable.
	if err := NotifyBidders(ctx, pool, rfqID, "rfq_amended",
		"공고 변경 알림", fmt.Sprintf("입찰 공고가 수정되었습니다: %s", note),
	); err != nil {
		return nil //nolint:nilerr // notify failure shouldn't roll back the amendment
	}
	return nil
}

// NotifyBidders inserts _meta.notifications rows for every user whose supplier
// has an active bid (any non-terminal status) on the RFQ. Used by AmendRFQ,
// scheduler open transitions, and award to let suppliers react without
// polling.
//
// Returns early without error when the notifications table is missing — the
// feature was seeded in an earlier release and should always exist, but the
// guard keeps scheduler-driven code resilient to partial deploys.
func NotifyBidders(ctx context.Context, pool *pgxpool.Pool, rfqID, notifType, title, body string) error {
	// Resolve the collection id for rfqs so ref_collection_id matches the
	// usual notification shape. Nil is acceptable — the bell renders fine
	// without the ref_ fields.
	var rfqColID *string
	_ = pool.QueryRow(ctx,
		`SELECT id::text FROM _meta.collections WHERE slug='rfqs'`,
	).Scan(&rfqColID)

	// One row per user tied to a supplier that submitted any bid on this
	// RFQ. We don't filter by bid.status here — suppliers whose bids are
	// already 'rejected' or 'awarded' still need the notification (that's
	// often exactly when the outcome is communicated). Soft-deleted bids
	// are excluded so retracted paper trails don't generate noise.
	_, err := pool.Exec(ctx, `
		INSERT INTO _meta.notifications
		  (user_id, type, title, body, ref_collection_id, ref_record_id)
		SELECT DISTINCT u.id, $1, $2, $3, $4::uuid, $5::uuid
		  FROM auth.users u
		  JOIN data.bids b ON b.supplier = u.supplier_id
		 WHERE u.role = 'supplier'
		   AND u.is_active = true
		   AND b.rfq = $5
		   AND b.deleted_at IS NULL`,
		notifType, title, body, rfqColID, rfqID,
	)
	if err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}

	LogEvent(ctx, pool, AuditEntry{
		ActorName: "notify-bidders",
		Action:    ActionNotify,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		Detail:    map[string]any{"type": notifType, "title": title},
	})
	return nil
}

// CloneRFQ duplicates a source RFQ into a fresh draft. Template flag is
// cleared on the copy (templates stay templates; copies are real drafts),
// submission-state fields (status/published_at/amendment_*) reset, and a
// new rfq_no is generated. Returns the new RFQ id.
//
// Used both by "duplicate this RFQ" buttons on past RFQs and by template
// libraries (is_template=true shells).
func CloneRFQ(ctx context.Context, pool *pgxpool.Pool, sourceID string, actor Actor) (string, error) {
	// Check source exists first for a clean 404.
	var srcExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM data.rfqs WHERE id=$1 AND deleted_at IS NULL)`,
		sourceID,
	).Scan(&srcExists); err != nil {
		return "", fmt.Errorf("check source: %w", err)
	}
	if !srcExists {
		return "", ErrRFQNotFound
	}

	// Deep copy: bring every reusable field across. Deliberately excluded —
	//   status (always draft), published_at, planned_price, reserve_prices,
	//   amendment_*, deadline_at, open_at (fresh dates required).
	// is_template is forced to false on the copy so a supplier-visible draft
	// emerges from a template shell.
	newNo := fmt.Sprintf("RFQ-CLONE-%s", time.Now().UTC().Format("20060102-150405"))
	var newID string
	err := pool.QueryRow(ctx, `
		INSERT INTO data.rfqs
		  (rfq_no, title, description, category, mode, eval_method, sealed,
		   base_amount, estimated_price, min_win_rate,
		   reserve_method, rfx_type,
		   attachments, is_template,
		   created_by)
		SELECT $2, title || ' (사본)', description, category, mode, eval_method, sealed,
		       base_amount, estimated_price, min_win_rate,
		       reserve_method, rfx_type,
		       attachments, false,
		       $3
		  FROM data.rfqs WHERE id = $1
		RETURNING id::text`,
		sourceID, newNo, nilIfEmptyString(actor.UserID),
	).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("clone rfq: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "clone-rfq"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionCloneRFQ,
		AppSlug:   "rfqs",
		RowID:     newID,
		IP:        actor.IP,
		Detail:    map[string]any{"source_id": sourceID},
	})
	return newID, nil
}

// PublishRFQ moves an RFQ from draft to published, stamping 15 reserve_prices
// candidates into the row when reserve_method='multiple'. This is the single
// entry point for "making an RFQ live" — it replaces a raw PATCH so we can
// enforce the reserves invariant and write a clean audit row.
//
// Fails closed when:
//   - RFQ not in 'draft' status (idempotent publish would re-stamp reserves
//     and shift the math for bids already submitted)
//   - reserve_method='multiple' but base_amount <= 0 (no basis for candidates)
func PublishRFQ(ctx context.Context, pool *pgxpool.Pool, rfqID string, actor Actor) error {
	var status, reserveMethod string
	var baseAmount *float64
	err := pool.QueryRow(ctx, `
		SELECT status, reserve_method, base_amount
		  FROM data.rfqs
		 WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&status, &reserveMethod, &baseAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRFQNotFound
		}
		return fmt.Errorf("load rfq: %w", err)
	}
	if status != "draft" {
		return fmt.Errorf("%w: only draft RFQs may be published (got %q)", ErrRFQIneligible, status)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if reserveMethod == "multiple" {
		if baseAmount == nil || *baseAmount <= 0 {
			return fmt.Errorf("%w: reserve_method=multiple requires base_amount > 0", ErrRFQIneligible)
		}
		reserves := GenerateReservePrices(*baseAmount)
		reservesJSON, _ := json.Marshal(reserves)
		if _, err := tx.Exec(ctx, `
			UPDATE data.rfqs
			   SET reserve_prices = $2::jsonb,
			       status = 'published',
			       published_at = COALESCE(published_at, now()),
			       updated_at = now()
			 WHERE id = $1`,
			rfqID, reservesJSON,
		); err != nil {
			return fmt.Errorf("publish with reserves: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE data.rfqs
			   SET status = 'published',
			       published_at = COALESCE(published_at, now()),
			       updated_at = now()
			 WHERE id = $1`,
			rfqID,
		); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "publish-rfq"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID: actor.UserID, ActorName: actorName,
		Action: "publish", AppSlug: "rfqs", RowID: rfqID, IP: actor.IP,
		Detail: map[string]any{"reserve_method": reserveMethod},
	})
	return nil
}

// ResolveMultipleReservePrices loads every bid's reserve_picks for the RFQ,
// computes the 4-most-picked average, and stamps it as planned_price.
// Called by the scheduler at open_at transition when reserve_method='multiple'.
//
// No-op (returns nil) when reserve_method != 'multiple' or no bids recorded
// picks — this lets the scheduler call it unconditionally for every RFQ
// going opened without branching.
func ResolveMultipleReservePrices(ctx context.Context, pool *pgxpool.Pool, rfqID string) error {
	var reserveMethod string
	var reservesRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT reserve_method, reserve_prices
		  FROM data.rfqs WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&reserveMethod, &reservesRaw)
	if err != nil || reserveMethod != "multiple" || len(reservesRaw) == 0 {
		return nil //nolint:nilerr // feature off for this RFQ
	}

	var reserves []float64
	if err := json.Unmarshal(reservesRaw, &reserves); err != nil {
		return fmt.Errorf("parse reserve_prices: %w", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT reserve_picks
		  FROM data.bids
		 WHERE rfq = $1
		   AND status IN ('submitted','opened')
		   AND reserve_picks IS NOT NULL
		   AND deleted_at IS NULL`,
		rfqID,
	)
	if err != nil {
		return fmt.Errorf("load picks: %w", err)
	}
	defer rows.Close()

	var picks [][]int
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var row []int
		if err := json.Unmarshal(raw, &row); err == nil {
			picks = append(picks, row)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	planned := ResolvePlannedPrice(reserves, picks)
	if planned <= 0 {
		return nil
	}
	if _, err := pool.Exec(ctx, `
		UPDATE data.rfqs SET planned_price = $2, updated_at = now()
		 WHERE id = $1`,
		rfqID, planned,
	); err != nil {
		return fmt.Errorf("stamp planned_price: %w", err)
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorName: "resolve-planned-price",
		Action:    ActionResolve,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		Detail: map[string]any{
			"planned_price": planned,
			"bidder_count":  len(picks),
		},
	})
	return nil
}

// AnswerClarification posts an answer to a pending Q&A row, flips the status
// to 'answered', and notifies every bidder on the parent RFQ. Replacing a
// raw PATCH ensures the notification fires exactly once (PATCH-based flows
// are easy to double-fire from eager UI retries) and that the audit trail
// records who answered.
func AnswerClarification(ctx context.Context, pool *pgxpool.Pool, clarID, answer string, actor Actor) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return errors.New("answer is required")
	}
	var rfqID string
	err := pool.QueryRow(ctx, `
		UPDATE data.rfq_clarifications
		   SET answer = $2, answered_at = now(), status = 'answered',
		       updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL
		RETURNING rfq::text`,
		clarID, answer,
	).Scan(&rfqID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("clarification not found")
		}
		return fmt.Errorf("answer: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "answer-clarification"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID: actor.UserID, ActorName: actorName,
		Action: "answer_qa", AppSlug: "rfq_clarifications",
		RowID: clarID, IP: actor.IP,
		Detail: map[string]any{"rfq_id": rfqID},
	})

	// Broadcast so every bidder sees the clarification, not just the asker —
	// Q&A answers bind the whole procurement process equally.
	if err := NotifyBidders(ctx, pool, rfqID, "rfq_clarification_answered",
		"입찰 Q&A 답변 등록", "참여하신 공고의 질의응답에 새 답변이 등록되었습니다.",
	); err != nil {
		return nil //nolint:nilerr // best-effort
	}
	return nil
}

// MoveToEvaluating transitions an RFQ from `opened` to `evaluating`. This is
// the explicit "적격심사 시작" signal — Korean procurement practice is to
// separate price-open from winner-selection so buyers can verify paperwork
// without time pressure.
//
// AwardRFQ already accepts both `opened` and `evaluating`, so going through
// this extra step is optional. Recording it gives a clean audit trail of
// when evaluation started.
func MoveToEvaluating(ctx context.Context, pool *pgxpool.Pool, rfqID string, actor Actor) error {
	tag, err := pool.Exec(ctx, `
		UPDATE data.rfqs SET status=$1, updated_at=now()
		 WHERE id=$2 AND status=$3 AND deleted_at IS NULL`,
		RFQStatusEvaluating, rfqID, RFQStatusOpened,
	)
	if err != nil {
		return fmt.Errorf("move to evaluating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either id unknown or current status !=opened; reuse the ineligible
		// error so the handler returns a coherent 400.
		return fmt.Errorf("%w: must currently be in 'opened' state", ErrRFQIneligible)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "move-to-evaluating"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionEvaluate,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		IP:        actor.IP,
	})
	return nil
}

// GenerateReservePrices computes 15 random multi-reserve-price candidates
// distributed in [base*0.97, base*1.03] — the conventional Korean procurement
// spread. Returned values are rounded to the nearest 원.
//
// Used when an admin publishes a multiple-reserve-method RFQ: the buyer sets
// base_amount, we stamp 15 candidates, suppliers pick 2 indices at bid time,
// and the 4 most-picked average into the 예정가 at open time.
//
// Deterministic per call via math/rand/v2 seeded from the caller's
// pseudo-random state — tests use ResolvePlannedPrice separately.
func GenerateReservePrices(base float64) []float64 {
	out := make([]float64, 15)
	for i := range out {
		// Spread: ±3% of base. 0.97 + rand*0.06 → [0.97, 1.03).
		factor := 0.97 + rand.Float64()*0.06
		out[i] = float64(int64(base * factor))
	}
	return out
}

// ResolvePlannedPrice picks the 4 most-frequent indices across all bids'
// reserve_picks arrays and averages the corresponding reserve_prices into
// the 예정가 (planned_price). Called at open_at by the scheduler or on-
// demand by the handler.
//
// `picks` is one entry per bid, each an array of 2 ints in [0, len(reserves)).
// Reserves must be the full 15-element array stamped on the RFQ.
//
// Returns 0 when no picks were recorded — deployments without the multiple-
// reserve feature active never reach this.
func ResolvePlannedPrice(reserves []float64, picks [][]int) float64 {
	if len(reserves) == 0 || len(picks) == 0 {
		return 0
	}
	counts := make(map[int]int, len(reserves))
	for _, row := range picks {
		for _, idx := range row {
			if idx >= 0 && idx < len(reserves) {
				counts[idx]++
			}
		}
	}
	if len(counts) == 0 {
		return 0
	}
	// Rank indices by frequency. Ties broken by lower index.
	ranked := make([]freqEntry, 0, len(counts))
	for k, v := range counts {
		ranked = append(ranked, freqEntry{idx: k, count: v})
	}
	sortByFreqThenIdx(ranked)

	// Average the top 4 (or fewer if not enough unique picks).
	n := 4
	if len(ranked) < n {
		n = len(ranked)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += reserves[ranked[i].idx]
	}
	return sum / float64(n)
}

// freqEntry carries a reserve_prices index and how many bids picked it.
type freqEntry struct{ idx, count int }

// sortByFreqThenIdx is extracted for testability. Descending by count, then
// ascending by index. Hand-rolled insertion sort avoids importing "sort" for
// a max-15-element slice.
func sortByFreqThenIdx(a []freqEntry) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0; j-- {
			if a[j].count > a[j-1].count ||
				(a[j].count == a[j-1].count && a[j].idx < a[j-1].idx) {
				a[j], a[j-1] = a[j-1], a[j]
				continue
			}
			break
		}
	}
}

// nilIfEmptyString returns nil for empty strings so pgx encodes SQL NULL
// rather than the zero UUID.
func nilIfEmptyString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// used to silence "encoding/json imported and not used" in environments
// where future helpers below may be commented out during refactors.
var _ = json.Marshal
