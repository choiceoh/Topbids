package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey string

const (
	ctxKeyRequestID ctxKey = "request_id"
	ctxKeyLogger    ctxKey = "logger"
)

// QueryTimeout is the per-request deadline applied to all DB operations
// initiated from a request handler.
const QueryTimeout = 15 * time.Second

// withTimeout wraps every request context with a deadline so that no DB query
// can outlive the request itself.
func WithTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), QueryTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withRequestID assigns a short hex token to every request and stores it
// in the context + a "X-Request-ID" response header.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withLogger attaches a per-request slog.Logger pre-tagged with request_id,
// method, and path. Handlers retrieve it via api.Log(r).
func WithLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid, _ := r.Context().Value(ctxKeyRequestID).(string)
			lg := base.With(
				slog.String("request_id", rid),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			ctx := context.WithValue(r.Context(), ctxKeyLogger, lg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Log returns the request-scoped logger, falling back to the default if missing.
func Log(r *http.Request) *slog.Logger {
	if lg, ok := r.Context().Value(ctxKeyLogger).(*slog.Logger); ok && lg != nil {
		return lg
	}
	return slog.Default()
}

// RequestID returns the current request's correlation ID, or "" if absent.
func RequestID(r *http.Request) string {
	if rid, ok := r.Context().Value(ctxKeyRequestID).(string); ok {
		return rid
	}
	return ""
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
