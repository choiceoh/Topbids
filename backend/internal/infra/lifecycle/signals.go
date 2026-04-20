// Package lifecycle provides graceful shutdown with signal handling.
// Adapted from Deneb gateway bootstrap/lifecycle.
package lifecycle

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// RunWithSignals runs fn with a context cancelled on SIGINT or SIGTERM.
// Returns 1 on error, 0 on clean shutdown.
func RunWithSignals(fn func(ctx context.Context) error, logger *slog.Logger) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	if err := fn(ctx); err != nil {
		logger.Error("server error", "error", err)
		return 1
	}
	return 0
}
