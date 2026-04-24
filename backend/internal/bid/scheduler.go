package bid

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// RFQ status values. Kept in sync with the "rfqs" preset (see
// backend/internal/seed/bid_apps.go). The scheduler only transitions between
// these four; other values (awarded/failed/cancelled) are terminal and set
// by user action.
const (
	RFQStatusPublished = "published"  // accepting bids
	RFQStatusClosed    = "closed"     // deadline passed, no new submissions
	RFQStatusOpened    = "opened"     // open_at passed, bids unsealed
)

// Scheduler polls the rfqs data table and transitions each RFQ through its
// time-driven states:
//
//	published → closed  (when deadline_at has passed)
//	closed    → opened  (when open_at has passed)
//
// The polling interval should be short relative to a typical RFQ lifetime
// (hours to days). Default 30s. The scheduler is safe to run alongside
// user-initiated status updates because the UPDATE predicates match only
// the exact (old-status, time-reached) pair.
//
// On each transition the scheduler publishes a record_update event to the
// provided event bus so SSE subscribers refresh. Audit logging and further
// side effects (notifications to bidders) live elsewhere.
type Scheduler struct {
	pool     *pgxpool.Pool
	cache    *schema.Cache
	bus      *events.Bus
	interval time.Duration

	stop chan struct{}
	done chan struct{}
}

// NewScheduler constructs a Scheduler. interval defaults to 30s if zero.
func NewScheduler(pool *pgxpool.Pool, cache *schema.Cache, bus *events.Bus, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		pool:     pool,
		cache:    cache,
		bus:      bus,
		interval: interval,
	}
}

// Start launches the polling loop on a background goroutine. Returns
// immediately. Call Stop to cancel. Stopping also happens automatically
// when ctx is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	if s.stop != nil {
		return // already running
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.loop(ctx)
}

// Stop signals the scheduler to exit and waits for the loop goroutine to
// finish one in-flight tick. Idempotent.
func (s *Scheduler) Stop() {
	if s.stop == nil {
		return
	}
	select {
	case <-s.stop:
		// already stopped
	default:
		close(s.stop)
	}
	<-s.done
	s.stop = nil
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)

	// Immediate first tick so startup catches up.
	if err := s.Tick(ctx); err != nil {
		slog.Warn("bid scheduler: initial tick failed", "error", err)
	}

	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-t.C:
			if err := s.Tick(ctx); err != nil {
				slog.Warn("bid scheduler: tick failed", "error", err)
			}
		}
	}
}

// Tick runs one transition pass. Exposed for testing; production callers
// should use Start/Stop.
func (s *Scheduler) Tick(ctx context.Context) error {
	rfqs, ok := s.cache.CollectionBySlug("rfqs")
	if !ok {
		return nil // seed hasn't run yet
	}

	closed, err := s.transition(ctx, "deadline_at", RFQStatusPublished, RFQStatusClosed)
	if err != nil {
		return fmt.Errorf("published→closed: %w", err)
	}
	opened, err := s.transition(ctx, "open_at", RFQStatusClosed, RFQStatusOpened)
	if err != nil {
		return fmt.Errorf("closed→opened: %w", err)
	}

	if n := len(closed) + len(opened); n > 0 {
		slog.Info("bid scheduler: transitioned", "closed", len(closed), "opened", len(opened))
	}

	// Broadcast events so SSE subscribers (frontend) refresh.
	for _, id := range closed {
		s.publish(ctx, rfqs, id, RFQStatusClosed)
	}
	for _, id := range opened {
		s.publish(ctx, rfqs, id, RFQStatusOpened)
		// Audit: record scheduler-initiated open of each RFQ. No actor — this
		// is a system event. Detail carries the transition for future analysis.
		LogEvent(ctx, s.pool, AuditEntry{
			ActorName: "bid-scheduler",
			Action:    ActionOpen,
			AppSlug:   rfqs.Slug,
			RowID:     id,
			Detail:    map[string]any{"from": RFQStatusClosed, "to": RFQStatusOpened},
		})
	}

	return nil
}

// transition runs an UPDATE that matches (fromStatus AND timeColumn <= now())
// and returns the IDs of the rows it flipped.
func (s *Scheduler) transition(ctx context.Context, timeColumn, fromStatus, toStatus string) ([]string, error) {
	const q = `
		UPDATE data.rfqs
		   SET status = $1, updated_at = now()
		 WHERE status = $2
		   AND %s <= now()
		   AND deleted_at IS NULL
		RETURNING id
	`
	rows, err := s.pool.Query(ctx, fmt.Sprintf(q, timeColumn), toStatus, fromStatus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// publish broadcasts a record_update event for the RFQ. Best-effort; log
// warnings but don't fail the tick.
func (s *Scheduler) publish(ctx context.Context, rfqs schema.Collection, recordID, newStatus string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, events.Event{
		Type:           events.EventRecordUpdate,
		CollectionID:   rfqs.ID,
		CollectionSlug: rfqs.Slug,
		RecordID:       recordID,
		ActorName:      "bid-scheduler",
	})
	_ = newStatus // reserved for future event payloads
}
