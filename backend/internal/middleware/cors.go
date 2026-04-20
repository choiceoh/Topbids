package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing.
//
// CORS_ORIGIN env can be a comma-separated list of allowed origins.
// If unset, development defaults to http://localhost:5173 with a warning.
// In production (GO_ENV=production), unset CORS_ORIGIN is an error and the
// middleware refuses all cross-origin requests.
func CORS(corsOrigin string, isProd bool) func(http.Handler) http.Handler {
	raw := corsOrigin

	var allowed []string
	if raw == "" {
		if isProd {
			slog.Error("CORS_ORIGIN is required in production — cross-origin requests will be rejected")
		} else {
			slog.Warn("CORS_ORIGIN not set, defaulting to Vite dev server", "origin", "http://localhost:5173")
			allowed = []string{"http://localhost:5173"}
		}
	} else {
		for _, o := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(o); s != "" {
				allowed = append(allowed, s)
			}
		}
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		allowedSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowedSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
