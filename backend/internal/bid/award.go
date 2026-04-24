package bid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Bid status values used by AwardRFQ. Must stay in sync with the bids
// preset (see backend/internal/seed/bid_apps.go).
const (
	BidStatusDraft     = "draft"
	BidStatusSubmitted = "submitted"
	BidStatusOpened    = "opened"
	BidStatusEvaluated = "evaluated"
	BidStatusAwarded   = "awarded"
	BidStatusRejected  = "rejected"

	RFQStatusEvaluating = "evaluating"
	RFQStatusAwarded    = "awarded"
	RFQStatusFailed     = "failed"

	EvalMethodLowest   = "lowest"
	EvalMethodWeighted = "weighted"

	// defaultTechWeight is used for weighted evaluation when the RFQ does not
	// specify a tech_weight. Standard procurement practice: technical weight
	// 20%, price weight 80%.
	defaultTechWeight = 20.0
)

var (
	ErrRFQNotFound      = errors.New("rfq not found")
	ErrRFQIneligible    = errors.New("rfq is not in an eligible status for award")
	ErrNoBids           = errors.New("no eligible bids to award")
	ErrTechScoreMissing = errors.New("weighted evaluation requires all bids to have a tech_score")
)

// AwardResult describes the outcome of an award computation.
// When distributePO chains successfully, Distribution is populated.
//
// Idempotent=true signals the RFQ was already awarded before this call —
// no scores were recomputed and no audit row was written; the handler
// should treat it as a 200 reply with the stored winner info. This lets
// the UI safely retry (network blip, double-click) without triggering a
// 400 or creating duplicate PO rows.
type AwardResult struct {
	RFQID        string            `json:"rfq_id"`
	EvalMethod   string            `json:"eval_method"`
	WinnerBidID  string            `json:"winner_bid_id"`
	WinnerAmount float64           `json:"winner_amount"`
	TotalBids    int               `json:"total_bids"`
	RejectedBids []string          `json:"rejected_bids"`
	Distribution *DistributeResult `json:"distribution,omitempty"`
	Idempotent   bool              `json:"idempotent,omitempty"`
}

// bidRow is the minimal shape we load for scoring.
type bidRow struct {
	id          string
	totalAmount float64
	techScore   *float64
}

// Actor carries the who-did-it metadata for audit-logged actions. Zero value
// is valid: an empty ID produces a "system" entry in _meta.bid_audit_log.
type Actor struct {
	UserID string
	Name   string
	IP     string
}

// ErrSupplierNotQualified indicates the winning bid's supplier has no
// valid (status='approved', valid_until >= today) PQ record for the RFQ's
// category. Award is blocked until the buyer approves a qualification.
var ErrSupplierNotQualified = errors.New("winning supplier lacks a valid PQ for this category")

// RFQStatusCancelled is a terminal status set by admins to abandon an RFQ.
// Separate from 'failed' (which is for no-eligible-bid outcomes).
const RFQStatusCancelled = "cancelled"

// ErrRFQNotCancellable is returned when CancelRFQ is invoked on a terminal
// RFQ (already cancelled/failed/awarded). Admins must not reverse those
// because downstream POs may already depend on them.
var ErrRFQNotCancellable = errors.New("rfq is in a terminal state and cannot be cancelled")

// CancelRFQ marks an RFQ as cancelled and rejects any outstanding bids in a
// single transaction, then logs an audit trail entry.
//
// Eligible states: draft, published, closed, opened, evaluating. Once an RFQ
// is awarded/cancelled/failed, it cannot be cancelled — terminal states are
// final (awarded already has POs, others already recorded that outcome).
//
// Outstanding submitted/opened bids on the RFQ are moved to BidStatusRejected
// so suppliers see a consistent "탈락" status and no bid remains "in limbo".
// Drafts and already-rejected bids are left alone.
func CancelRFQ(ctx context.Context, pool *pgxpool.Pool, rfqID, reason string, actor Actor) error {
	var status string
	err := pool.QueryRow(ctx,
		`SELECT status FROM data.rfqs WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRFQNotFound
		}
		return fmt.Errorf("load rfq: %w", err)
	}
	switch status {
	case RFQStatusAwarded, RFQStatusCancelled, RFQStatusFailed:
		return fmt.Errorf("%w: status=%q", ErrRFQNotCancellable, status)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		UPDATE data.rfqs SET status = $1, updated_at = now()
		 WHERE id = $2`,
		RFQStatusCancelled, rfqID,
	); err != nil {
		return fmt.Errorf("cancel rfq: %w", err)
	}

	// Reject live bids so suppliers see a definitive outcome rather than a
	// stale "제출됨" badge forever.
	if _, err := tx.Exec(ctx, `
		UPDATE data.bids SET status = $1, updated_at = now()
		 WHERE rfq = $2 AND status IN ($3, $4) AND deleted_at IS NULL`,
		BidStatusRejected, rfqID, BidStatusSubmitted, BidStatusOpened,
	); err != nil {
		return fmt.Errorf("reject bids: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "cancel-rfq"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionCancel,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		IP:        actor.IP,
		Detail: map[string]any{
			"reason": reason,
			"from":   status,
		},
	})

	return nil
}

// BidStatusWithdrawn is what a supplier-initiated retract transitions to.
// Keeping it separate from 'rejected' preserves the distinction between
// "evaluator ruled out" and "supplier pulled their own offer".
const BidStatusWithdrawn = "withdrawn"

// ErrBidNotWithdrawable is returned when WithdrawBid is invoked on a bid
// whose RFQ has moved past 'published' or whose status is terminal.
var ErrBidNotWithdrawable = errors.New("bid cannot be withdrawn in its current state")

// ErrBidNotFound is returned when the bid row doesn't exist or is
// soft-deleted. Callers that expose this to suppliers should collapse to a
// generic 404 to avoid existence enumeration.
var ErrBidNotFound = errors.New("bid not found")

// WithdrawBid lets a supplier (or admin) retract a submitted bid while the
// parent RFQ is still accepting changes. Ownership is enforced at the
// handler layer via EnforceBidWriteOwnership; here we only transition the
// status and write an audit row.
//
// The operation is idempotent on already-withdrawn bids — a second call
// returns nil without touching the DB.
func WithdrawBid(ctx context.Context, pool *pgxpool.Pool, bidID string, actor Actor) error {
	var bidStatus, rfqStatus string
	err := pool.QueryRow(ctx, `
		SELECT b.status, r.status
		  FROM data.bids b
		  JOIN data.rfqs r ON r.id = b.rfq
		 WHERE b.id = $1 AND b.deleted_at IS NULL AND r.deleted_at IS NULL`,
		bidID,
	).Scan(&bidStatus, &rfqStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBidNotFound
		}
		return fmt.Errorf("load bid: %w", err)
	}
	if bidStatus == BidStatusWithdrawn {
		return nil // idempotent
	}
	if bidStatus != BidStatusDraft && bidStatus != BidStatusSubmitted {
		return fmt.Errorf("%w: status=%q", ErrBidNotWithdrawable, bidStatus)
	}
	if rfqStatus != RFQStatusPublished {
		return fmt.Errorf("%w: rfq is %q", ErrBidNotWithdrawable, rfqStatus)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE data.bids SET status = $1, updated_at = now()
		 WHERE id = $2`,
		BidStatusWithdrawn, bidID,
	); err != nil {
		return fmt.Errorf("withdraw: %w", err)
	}

	actorName := actor.Name
	if actorName == "" {
		actorName = "withdraw-bid"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionWithdraw,
		AppSlug:   "bids",
		RowID:     bidID,
		IP:        actor.IP,
		Detail:    map[string]any{"from_status": bidStatus},
	})
	return nil
}

// AwardRFQ selects a winner among the eligible bids for the given RFQ,
// then updates statuses in a single transaction:
//   - winning bid: BidStatusAwarded
//   - losing bids: BidStatusRejected
//   - RFQ:         RFQStatusAwarded
//
// Eligible RFQ statuses: opened or evaluating. Eligible bids: submitted or
// opened (not draft/rejected/awarded already).
//
// Scoring depends on the RFQ's eval_method:
//   - "lowest":   pick the bid with the smallest total_amount (ties broken
//                 by earliest submitted_at, then id)
//   - "weighted": price_score = (min_amount / this_amount) * 100;
//                 total = tech * w + price * (1-w) where w = tech_weight/100
//                 Requires every bid to have a tech_score. Ties broken as above.
//
// actor is recorded on the audit trail. Pass a zero Actor for scripted/system
// invocations.
func AwardRFQ(ctx context.Context, pool *pgxpool.Pool, rfqID string, actor Actor) (*AwardResult, error) {
	// 1. Load RFQ + validate eligibility.
	var rfqStatus, evalMethod string
	var rfqCategory *string
	var estimatedPrice, minWinRate *float64
	err := pool.QueryRow(ctx, `
		SELECT status, eval_method, category, estimated_price, min_win_rate
		  FROM data.rfqs
		 WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&rfqStatus, &evalMethod, &rfqCategory, &estimatedPrice, &minWinRate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRFQNotFound
		}
		return nil, fmt.Errorf("load rfq: %w", err)
	}
	// Already awarded → return the prior result idempotently. Double-submits
	// from the UI are common (network retries, impatient clicks) and must
	// not create duplicate audit rows or re-run PO distribution.
	if rfqStatus == RFQStatusAwarded {
		return loadExistingAward(ctx, pool, rfqID, evalMethod)
	}
	if rfqStatus != RFQStatusOpened && rfqStatus != RFQStatusEvaluating {
		return nil, fmt.Errorf("%w: status=%q (must be opened or evaluating)", ErrRFQIneligible, rfqStatus)
	}

	// 2. Load eligible bids.
	bids, err := loadEligibleBids(ctx, pool, rfqID)
	if err != nil {
		return nil, fmt.Errorf("load bids: %w", err)
	}
	if len(bids) == 0 {
		return nil, ErrNoBids
	}

	// 2a. Apply minimum winning price floor if configured. This is the
	// "낙찰하한가" in Korean procurement — bids below (estimated * min_rate)
	// are disqualified before scoring to prevent lowball wins from
	// unable-to-deliver vendors. Both fields must be > 0 to activate.
	bids = applyMinWinFloor(bids, estimatedPrice, minWinRate)
	if len(bids) == 0 {
		return nil, ErrNoBids
	}

	// 2b. Hydrate tech_score from bid_evaluations when multiple evaluators
	// scored a bid. Falls back to the bid.tech_score column so collections
	// that don't use multi-eval still work.
	if evalMethod == EvalMethodWeighted {
		if err := hydrateEvaluatorAverages(ctx, pool, bids); err != nil {
			return nil, fmt.Errorf("hydrate evaluations: %w", err)
		}
	}

	// 3. Pick winner.
	winnerID, winnerAmount, err := pickWinner(bids, evalMethod)
	if err != nil {
		return nil, err
	}

	// 3a. Qualification check (적격심사): the winning supplier must have a
	// currently-valid PQ row for the RFQ's category. Empty category or
	// missing PQ collection both bypass — the feature is opt-in per deployment.
	if rfqCategory != nil && *rfqCategory != "" {
		if err := requireWinnerQualified(ctx, pool, winnerID, *rfqCategory); err != nil {
			return nil, err
		}
	}

	// 4. Apply status transitions in a single transaction.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback err ignored if commit succeeds

	if _, err := tx.Exec(ctx, `
		UPDATE data.bids
		   SET status = $1, updated_at = now()
		 WHERE id = $2`,
		BidStatusAwarded, winnerID,
	); err != nil {
		return nil, fmt.Errorf("mark winner: %w", err)
	}

	rejectedIDs := make([]string, 0, len(bids)-1)
	for _, b := range bids {
		if b.id == winnerID {
			continue
		}
		rejectedIDs = append(rejectedIDs, b.id)
	}
	if len(rejectedIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE data.bids
			   SET status = $1, updated_at = now()
			 WHERE id = ANY($2::uuid[])`,
			BidStatusRejected, rejectedIDs,
		); err != nil {
			return nil, fmt.Errorf("mark rejected: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE data.rfqs
		   SET status = $1, updated_at = now()
		 WHERE id = $2`,
		RFQStatusAwarded, rfqID,
	); err != nil {
		return nil, fmt.Errorf("mark rfq awarded: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	result := &AwardResult{
		RFQID:        rfqID,
		EvalMethod:   evalMethod,
		WinnerBidID:  winnerID,
		WinnerAmount: winnerAmount,
		TotalBids:    len(bids),
		RejectedBids: rejectedIDs,
	}

	// Audit: log the award before chaining into PO distribution so we have a
	// record even if distribution fails downstream.
	actorName := actor.Name
	if actorName == "" {
		actorName = "award-rfq"
	}
	LogEvent(ctx, pool, AuditEntry{
		ActorID:   actor.UserID,
		ActorName: actorName,
		Action:    ActionAward,
		AppSlug:   "rfqs",
		RowID:     rfqID,
		IP:        actor.IP,
		Detail: map[string]any{
			"winner_bid_id": winnerID,
			"winner_amount": winnerAmount,
			"eval_method":   evalMethod,
			"total_bids":    len(bids),
		},
	})

	// Chain: automatically fan out into per-subsidiary POs. Failures here
	// don't invalidate the award — the admin can retry distribution later.
	dist, distErr := DistributePO(ctx, pool, rfqID)
	switch {
	case distErr == nil:
		result.Distribution = dist
	case errors.Is(distErr, ErrAlreadyDistributed):
		// Idempotent — treat as success without populating Distribution.
		slog.Info("award: distribution already done", "rfq_id", rfqID)
	default:
		slog.Warn("award: distribution failed, awarding anyway", "rfq_id", rfqID, "error", distErr)
	}

	// Notify all bidders (winner + rejected) so the suppliers' portal
	// history page refreshes with final status. Best-effort.
	if err := NotifyBidders(ctx, pool, rfqID, "rfq_awarded",
		"낙찰 결과 발표", "참여하신 입찰의 낙찰 결과가 발표되었습니다.",
	); err != nil {
		slog.Warn("award: notify failed", "rfq_id", rfqID, "error", err)
	}

	return result, nil
}

// loadExistingAward rebuilds an AwardResult from the durable state of an
// already-awarded RFQ. Used for the idempotent path of AwardRFQ so a second
// identical call returns the same shape as the first without touching the DB.
//
// Returns a result with Idempotent=true and Distribution=nil (callers that
// need distribution info can hit the purchase_orders collection directly —
// the award itself is durable and distribution is derivable).
func loadExistingAward(ctx context.Context, pool *pgxpool.Pool, rfqID, evalMethod string) (*AwardResult, error) {
	// Locate the single winner. There must be exactly one — if the data is
	// corrupted (0 or >1 awarded bids), we surface that as an error rather
	// than silently picking one.
	var winnerID string
	var winnerAmount float64
	err := pool.QueryRow(ctx, `
		SELECT id, total_amount
		  FROM data.bids
		 WHERE rfq = $1 AND status = $2 AND deleted_at IS NULL`,
		rfqID, BidStatusAwarded,
	).Scan(&winnerID, &winnerAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: rfq marked awarded but no winning bid row", ErrRFQIneligible)
		}
		return nil, fmt.Errorf("load existing winner: %w", err)
	}

	// Bucket the rejected bids for the response summary.
	rows, err := pool.Query(ctx, `
		SELECT id
		  FROM data.bids
		 WHERE rfq = $1 AND status = $2 AND deleted_at IS NULL`,
		rfqID, BidStatusRejected,
	)
	if err != nil {
		return nil, fmt.Errorf("load rejected: %w", err)
	}
	defer rows.Close()
	var rejected []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		rejected = append(rejected, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &AwardResult{
		RFQID:        rfqID,
		EvalMethod:   evalMethod,
		WinnerBidID:  winnerID,
		WinnerAmount: winnerAmount,
		TotalBids:    1 + len(rejected),
		RejectedBids: rejected,
		Idempotent:   true,
	}, nil
}

// hydrateEvaluatorAverages overwrites each bid's techScore with the average
// of its rows in data.bid_evaluations. Bids with zero evaluation rows keep
// their existing techScore (backward compat — single-evaluator deployments
// continue to populate bid.tech_score directly).
//
// The query joins by bid id and counts rows with non-null tech_score only.
func hydrateEvaluatorAverages(ctx context.Context, pool *pgxpool.Pool, bids []bidRow) error {
	if len(bids) == 0 {
		return nil
	}
	// Skip if the evaluations collection hasn't been seeded yet. Table-
	// missing errors would otherwise break award for deployments that
	// haven't run the new seed.
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.tables
		               WHERE table_schema='data' AND table_name='bid_evaluations')`,
	).Scan(&exists); err != nil || !exists {
		return nil //nolint:nilerr // table absent = feature off, not an error
	}

	ids := make([]string, len(bids))
	for i, b := range bids {
		ids[i] = b.id
	}
	rows, err := pool.Query(ctx, `
		SELECT bid::text, AVG(tech_score)::float8
		  FROM data.bid_evaluations
		 WHERE bid = ANY($1::uuid[])
		   AND tech_score IS NOT NULL
		   AND deleted_at IS NULL
		 GROUP BY bid`,
		ids,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	avgs := make(map[string]float64)
	for rows.Next() {
		var id string
		var avg float64
		if err := rows.Scan(&id, &avg); err != nil {
			return err
		}
		avgs[id] = avg
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range bids {
		if avg, ok := avgs[bids[i].id]; ok {
			bids[i].techScore = &avg
		}
	}
	return nil
}

// requireWinnerQualified returns ErrSupplierNotQualified unless the winning
// bid's supplier holds an approved, unexpired PQ row for the RFQ category.
// Bypasses silently when the supplier_qualifications collection is absent —
// PQ is opt-in per deployment.
func requireWinnerQualified(ctx context.Context, pool *pgxpool.Pool, bidID, category string) error {
	var tableExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.tables
		               WHERE table_schema='data' AND table_name='supplier_qualifications')`,
	).Scan(&tableExists); err != nil || !tableExists {
		return nil //nolint:nilerr // PQ collection not seeded — feature off
	}

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM data.supplier_qualifications pq
		  JOIN data.bids b ON b.supplier = pq.supplier
		 WHERE b.id = $1
		   AND pq.category = $2
		   AND pq.status = 'approved'
		   AND (pq.valid_until IS NULL OR pq.valid_until >= CURRENT_DATE)
		   AND pq.deleted_at IS NULL`,
		bidID, category,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check PQ: %w", err)
	}
	if count == 0 {
		return ErrSupplierNotQualified
	}
	return nil
}

// applyMinWinFloor drops bids whose total_amount is below the floor
// (estimated_price * min_win_rate). Returns the input unchanged when either
// configuration value is missing or non-positive — the feature is opt-in
// per RFQ, not mandatory.
//
// Korean public-procurement convention is 80-87% of 예정가; private buyers
// set their own rate. The intent is to disqualify bids so cheap the vendor
// can't realistically deliver, before scoring runs.
func applyMinWinFloor(bids []bidRow, estimated, rate *float64) []bidRow {
	if estimated == nil || rate == nil || *estimated <= 0 || *rate <= 0 {
		return bids
	}
	floor := *estimated * *rate
	out := make([]bidRow, 0, len(bids))
	for _, b := range bids {
		if b.totalAmount >= floor {
			out = append(out, b)
		}
	}
	return out
}

// loadEligibleBids fetches bids that are still in play for the given RFQ.
func loadEligibleBids(ctx context.Context, pool *pgxpool.Pool, rfqID string) ([]bidRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, total_amount, tech_score
		  FROM data.bids
		 WHERE rfq = $1
		   AND status IN ($2, $3)
		   AND deleted_at IS NULL
		 ORDER BY submitted_at NULLS LAST, id`,
		rfqID, BidStatusSubmitted, BidStatusOpened,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []bidRow
	for rows.Next() {
		var b bidRow
		if err := rows.Scan(&b.id, &b.totalAmount, &b.techScore); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// pickWinner dispatches to the lowest or weighted scoring strategy.
// Returns (winnerBidID, winnerAmount, error). bids is assumed non-empty.
func pickWinner(bids []bidRow, evalMethod string) (string, float64, error) {
	switch evalMethod {
	case EvalMethodLowest:
		return pickLowest(bids)
	case EvalMethodWeighted:
		return pickWeighted(bids, defaultTechWeight)
	default:
		// Unknown method → fall back to lowest for safety.
		return pickLowest(bids)
	}
}

// pickLowest returns the bid with the smallest total_amount. Ties are
// broken by the bid row's pre-sorted order (earliest submitted_at).
func pickLowest(bids []bidRow) (string, float64, error) {
	best := bids[0]
	for _, b := range bids[1:] {
		if b.totalAmount < best.totalAmount {
			best = b
		}
	}
	return best.id, best.totalAmount, nil
}

// pickWeighted computes total_score = tech*w + price*(1-w) where
// price_score = (minAmount / thisAmount) * 100 and w = techWeight/100.
// Returns the bid with the highest total_score.
func pickWeighted(bids []bidRow, techWeight float64) (string, float64, error) {
	// Validate every bid has a tech_score.
	for _, b := range bids {
		if b.techScore == nil {
			return "", 0, ErrTechScoreMissing
		}
	}

	// Find the minimum amount for price-score normalization.
	minAmount := bids[0].totalAmount
	for _, b := range bids[1:] {
		if b.totalAmount < minAmount {
			minAmount = b.totalAmount
		}
	}

	tw := techWeight / 100
	pw := 1 - tw

	best := bids[0]
	bestScore := scoreOf(bids[0], minAmount, tw, pw)
	for _, b := range bids[1:] {
		s := scoreOf(b, minAmount, tw, pw)
		if s > bestScore {
			best = b
			bestScore = s
		}
	}
	return best.id, best.totalAmount, nil
}

func scoreOf(b bidRow, minAmount, tw, pw float64) float64 {
	var priceScore float64
	if b.totalAmount > 0 {
		priceScore = (minAmount / b.totalAmount) * 100
	}
	return *b.techScore*tw + priceScore*pw
}
