// Package sync defines the interface for external API → Topbid DB
// synchronization batch jobs.
//
// Actual API calls are deferred until external API specs are received.
// Implementations should be registered and scheduled via the Runner.
package sync

import (
	"context"
	"log/slog"
	"time"
)

// Source is the interface that each external data source must implement.
// Examples: HR system (departments/users), ERP (inventory), etc.
type Source interface {
	// Name returns a human-readable identifier for logging/metrics.
	Name() string

	// Sync performs a single synchronization cycle.
	// It should fetch data from the external system and upsert into Topbid.
	// The context carries a deadline; implementations must respect cancellation.
	Sync(ctx context.Context) (*Result, error)
}

// Result holds the outcome of a single sync cycle.
type Result struct {
	Created  int           `json:"created"`
	Updated  int           `json:"updated"`
	Deleted  int           `json:"deleted"`
	Skipped  int           `json:"skipped"`
	Errors   int           `json:"errors"`
	Duration time.Duration `json:"duration_ms"`
}

// Runner orchestrates periodic sync jobs.
type Runner struct {
	sources  []Source
	interval time.Duration
	logger   *slog.Logger
}

// NewRunner creates a runner that ticks at the given interval.
func NewRunner(interval time.Duration, logger *slog.Logger) *Runner {
	return &Runner{
		interval: interval,
		logger:   logger,
	}
}

// Register adds a Source to the runner.
func (r *Runner) Register(s Source) {
	r.sources = append(r.sources, s)
}

// Start begins the sync loop in a goroutine. It blocks until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	if len(r.sources) == 0 {
		r.logger.Info("sync: no sources registered, skipping")
		return
	}

	r.logger.Info("sync: starting", "sources", len(r.sources), "interval", r.interval)
	r.runAll(ctx) // immediate first run

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("sync: shutting down")
			return
		case <-ticker.C:
			r.runAll(ctx)
		}
	}
}

func (r *Runner) runAll(ctx context.Context) {
	for _, src := range r.sources {
		if ctx.Err() != nil {
			return
		}
		r.logger.Info("sync: running", "source", src.Name())
		result, err := src.Sync(ctx)
		if err != nil {
			r.logger.Error("sync: failed", "source", src.Name(), "error", err)
			continue
		}
		r.logger.Info("sync: completed",
			"source", src.Name(),
			"created", result.Created,
			"updated", result.Updated,
			"deleted", result.Deleted,
			"skipped", result.Skipped,
			"errors", result.Errors,
			"duration_ms", result.Duration.Milliseconds(),
		)
	}
}
