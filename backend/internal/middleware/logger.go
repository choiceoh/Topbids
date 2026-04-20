package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Logger returns HTTP request logging middleware using slog.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	log := logger.With("pkg", "http")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			elapsed := time.Since(start)

			level := slog.LevelInfo
			if ww.Status() >= 500 {
				level = slog.LevelError
			} else if ww.Status() >= 400 {
				level = slog.LevelWarn
			}

			log.Log(r.Context(), level, r.Method+" "+r.URL.Path,
				"status", ww.Status(),
				"latency", elapsed,
				"bytes", ww.BytesWritten(),
			)
		})
	}
}
