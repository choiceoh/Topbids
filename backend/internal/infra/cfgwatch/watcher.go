// Package cfgwatch provides config hot-reload via polling.
// No external dependency (no fsnotify).
// Ported from Deneb gateway config/watcher.go (deleted in 96ccbac5).
package cfgwatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	DefaultInterval = 5 * time.Second
	DefaultDebounce = 2 * time.Second
)

// ReloadCallback is called when the watched file changes.
type ReloadCallback func(oldHash, newHash string, data []byte) error

// Watcher monitors a file for changes via polling + hash comparison.
type Watcher struct {
	mu           sync.Mutex
	path         string
	interval     time.Duration
	debounce     time.Duration
	lastHash     string
	lastReloadAt time.Time
	callbacks    []ReloadCallback
	logger       *slog.Logger
}

func New(path string, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		path:     path,
		interval: DefaultInterval,
		debounce: DefaultDebounce,
		logger:   logger.With("pkg", "cfgwatch"),
	}
}

func (w *Watcher) OnReload(cb ReloadCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

func (w *Watcher) SetInitialHash(hash string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastHash = hash
}

// Start begins polling. Blocks until ctx is canceled.
func (w *Watcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.logger.Info("config watcher started", "path", w.path, "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("config watcher stopped")
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *Watcher) check() {
	w.mu.Lock()
	lastHash := w.lastHash
	lastReloadAt := w.lastReloadAt
	w.mu.Unlock()

	if !lastReloadAt.IsZero() && time.Since(lastReloadAt) < w.debounce {
		return
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		w.logger.Warn("config watcher: read failed", "error", err)
		return
	}

	hash := hashBytes(data)
	if hash == lastHash {
		return
	}

	w.logger.Info("config change detected",
		"old_hash", truncHash(lastHash),
		"new_hash", truncHash(hash),
	)

	w.mu.Lock()
	callbacks := make([]ReloadCallback, len(w.callbacks))
	copy(callbacks, w.callbacks)
	oldHash := w.lastHash
	w.lastHash = hash
	w.lastReloadAt = time.Now()
	w.mu.Unlock()

	for _, cb := range callbacks {
		if err := cb(oldHash, hash, data); err != nil {
			w.logger.Warn("reload callback failed", "error", err)
		}
	}
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func truncHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
