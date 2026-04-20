package bid

import (
	"context"
	"errors"
	"fmt"

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
type AwardResult struct {
	RFQID        string   `json:"rfq_id"`
	EvalMethod   string   `json:"eval_method"`
	WinnerBidID  string   `json:"winner_bid_id"`
	WinnerAmount float64  `json:"winner_amount"`
	TotalBids    int      `json:"total_bids"`
	RejectedBids []string `json:"rejected_bids"`
}

// bidRow is the minimal shape we load for scoring.
type bidRow struct {
	id          string
	totalAmount float64
	techScore   *float64
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
func AwardRFQ(ctx context.Context, pool *pgxpool.Pool, rfqID string) (*AwardResult, error) {
	// 1. Load RFQ + validate eligibility.
	var rfqStatus, evalMethod string
	err := pool.QueryRow(ctx, `
		SELECT status, eval_method
		  FROM data.rfqs
		 WHERE id = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&rfqStatus, &evalMethod)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRFQNotFound
		}
		return nil, fmt.Errorf("load rfq: %w", err)
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

	// 3. Pick winner.
	winnerID, winnerAmount, err := pickWinner(bids, evalMethod)
	if err != nil {
		return nil, err
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

	return &AwardResult{
		RFQID:        rfqID,
		EvalMethod:   evalMethod,
		WinnerBidID:  winnerID,
		WinnerAmount: winnerAmount,
		TotalBids:    len(bids),
		RejectedBids: rejectedIDs,
	}, nil
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
