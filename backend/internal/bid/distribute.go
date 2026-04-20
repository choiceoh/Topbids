package bid

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PO status values. Must stay in sync with the purchase_orders preset.
const (
	POStatusDraft     = "draft"
	POStatusConfirmed = "confirmed"
	POStatusShipped   = "shipped"
	POStatusReceived  = "received"
	POStatusCompleted = "completed"
	POStatusCancelled = "cancelled"
)

// ErrAlreadyDistributed is returned when DistributePO is invoked for an RFQ
// that already has purchase_orders rows. Safe to ignore in auto-chain from
// AwardRFQ so award is idempotent.
var ErrAlreadyDistributed = errors.New("purchase orders already distributed for this rfq")

// DistributeResult describes what DistributePO created.
type DistributeResult struct {
	RFQID            string   `json:"rfq_id"`
	WinnerBidID      string   `json:"winner_bid_id"`
	TotalAmount      float64  `json:"total_amount"`
	SubsidiaryCount  int      `json:"subsidiary_count"`
	PurchaseOrderIDs []string `json:"purchase_order_ids"`
}

// DistributePO fans out a winning bid into per-subsidiary purchase orders.
//
// Current strategy: **equal split across active subsidiaries**. Every
// subsidiary in auth.subsidiaries with is_active=true receives one PO
// whose allocated_amount = winner.total_amount / N. When no subsidiaries
// exist, a single unallocated PO is created as a placeholder.
//
// A future iteration will replace equal split with proportional allocation
// once purchase_requests are seeded and linked to RFQs via
// rfqs.source_requests. The surface (this function) stays the same.
//
// Idempotent: returns ErrAlreadyDistributed if any PO row already exists
// for the RFQ. Callers in auto-chain should treat that as success.
func DistributePO(ctx context.Context, pool *pgxpool.Pool, rfqID string) (*DistributeResult, error) {
	// 1. Load the awarded bid for the RFQ.
	var winnerID, supplierID string
	var totalAmount float64
	err := pool.QueryRow(ctx, `
		SELECT id, supplier, total_amount
		  FROM data.bids
		 WHERE rfq = $1
		   AND status = $2
		   AND deleted_at IS NULL
		 LIMIT 1`,
		rfqID, BidStatusAwarded,
	).Scan(&winnerID, &supplierID, &totalAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: no awarded bid found", ErrNoBids)
		}
		return nil, fmt.Errorf("load winner bid: %w", err)
	}

	// 2. Idempotency guard.
	var existingCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM data.purchase_orders WHERE rfq = $1 AND deleted_at IS NULL`,
		rfqID,
	).Scan(&existingCount); err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if existingCount > 0 {
		return nil, ErrAlreadyDistributed
	}

	// 3. Load active subsidiaries (auth schema, outside the dynamic engine).
	subs, err := loadActiveSubsidiaries(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load subsidiaries: %w", err)
	}

	// 4. Load RFQ's po_no seed.
	var rfqNo string
	if err := pool.QueryRow(ctx, `SELECT rfq_no FROM data.rfqs WHERE id = $1`, rfqID).Scan(&rfqNo); err != nil {
		return nil, fmt.Errorf("load rfq_no: %w", err)
	}

	// 5. Insert POs in one transaction.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback err is noise if commit succeeds

	var createdIDs []string
	now := time.Now().UTC()

	if len(subs) == 0 {
		// Placeholder: single unallocated PO the admin can split manually.
		id, err := insertPO(ctx, tx, poInsert{
			PONo:        fmt.Sprintf("%s-PO-001", rfqNo),
			RFQID:       rfqID,
			BidID:       winnerID,
			SupplierID:  supplierID,
			SubID:       "",
			SubName:     "",
			Amount:      totalAmount,
			Ratio:       1.0,
			PODate:      now,
		})
		if err != nil {
			return nil, err
		}
		createdIDs = append(createdIDs, id)
	} else {
		share := totalAmount / float64(len(subs))
		ratio := 1.0 / float64(len(subs))
		for i, s := range subs {
			id, err := insertPO(ctx, tx, poInsert{
				PONo:        fmt.Sprintf("%s-PO-%03d", rfqNo, i+1),
				RFQID:       rfqID,
				BidID:       winnerID,
				SupplierID:  supplierID,
				SubID:       s.id,
				SubName:     s.name,
				Amount:      share,
				Ratio:       ratio,
				PODate:      now,
			})
			if err != nil {
				return nil, err
			}
			createdIDs = append(createdIDs, id)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &DistributeResult{
		RFQID:            rfqID,
		WinnerBidID:      winnerID,
		TotalAmount:      totalAmount,
		SubsidiaryCount:  len(subs),
		PurchaseOrderIDs: createdIDs,
	}, nil
}

// subsidiary is the projection of auth.subsidiaries we need for distribution.
type subsidiary struct {
	id   string
	name string
}

func loadActiveSubsidiaries(ctx context.Context, pool *pgxpool.Pool) ([]subsidiary, error) {
	rows, err := pool.Query(ctx, `
		SELECT id::text, name
		  FROM auth.subsidiaries
		 WHERE is_active = true
		 ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []subsidiary
	for rows.Next() {
		var s subsidiary
		if err := rows.Scan(&s.id, &s.name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// poInsert carries the fields for a single purchase_orders row.
type poInsert struct {
	PONo       string
	RFQID      string
	BidID      string
	SupplierID string
	SubID      string
	SubName    string
	Amount     float64
	Ratio      float64
	PODate     time.Time
}

func insertPO(ctx context.Context, tx pgx.Tx, p poInsert) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO data.purchase_orders
		  (po_no, rfq, bid, supplier, subsidiary, subsidiary_name,
		   allocated_amount, allocation_ratio, status, po_date)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
		        $7, $8, $9, $10)
		RETURNING id`,
		p.PONo, p.RFQID, p.BidID, p.SupplierID,
		p.SubID, p.SubName,
		p.Amount, p.Ratio, POStatusDraft, p.PODate,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert PO %s: %w", p.PONo, err)
	}
	return id, nil
}
