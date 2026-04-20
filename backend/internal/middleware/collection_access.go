package middleware

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

// CollectionAccess checks that the authenticated user has access to the collection
// identified by the {slug} URL parameter. It sets the collection-level role in context.
//
// Rules:
//   - director system role always passes (full access)
//   - If collection has zero members → open to all (backwards compatible)
//   - If collection has members → user must be a member
func CollectionAccess(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUser(r.Context())
			if !ok {
				apierr.Unauthorized("authentication required").Write(w)
				return
			}

			// Director bypasses all collection access checks.
			if user.Role == "director" {
				ctx := SetCollectionRole(r.Context(), "owner")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Extract slug from URL path: /api/data/{slug}[/...]
			slug := extractSlug(r.URL.Path)
			if slug == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Look up the collection ID by slug.
			var collectionID string
			err := pool.QueryRow(r.Context(),
				`SELECT id::text FROM _meta.collections WHERE slug = $1`, slug,
			).Scan(&collectionID)
			if err == pgx.ErrNoRows {
				// Collection not found — let the handler return 404.
				next.ServeHTTP(w, r)
				return
			}
			if err != nil {
				apierr.WrapInternal("collection lookup", err).Write(w)
				return
			}

			// Check if collection has any members.
			var memberCount int
			err = pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM _meta.collection_members WHERE collection_id = $1`, collectionID,
			).Scan(&memberCount)
			if err != nil {
				apierr.WrapInternal("member count", err).Write(w)
				return
			}

			if memberCount == 0 {
				// Open collection — use system role as fallback.
				colRole := systemRoleToCollectionRole(user.Role)
				ctx := SetCollectionRole(r.Context(), colRole)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Collection has members — user must be one.
			var memberRole string
			err = pool.QueryRow(r.Context(),
				`SELECT role FROM _meta.collection_members WHERE collection_id = $1 AND user_id = $2`,
				collectionID, user.UserID,
			).Scan(&memberRole)
			if err == pgx.ErrNoRows {
				apierr.Forbidden("you are not a member of this collection").Write(w)
				return
			}
			if err != nil {
				apierr.WrapInternal("member lookup", err).Write(w)
				return
			}

			ctx := SetCollectionRole(r.Context(), memberRole)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractSlug gets the collection slug from a data API path.
// Path format: /api/data/{slug}[/{id}[/...]]
func extractSlug(path string) string {
	const prefix = "/api/data/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.IndexByte(rest, '/'); idx > 0 {
		return rest[:idx]
	}
	return rest
}

// systemRoleToCollectionRole maps system roles to collection-level roles for open collections.
func systemRoleToCollectionRole(sysRole string) string {
	switch sysRole {
	case "director", "pm":
		return "editor"
	case "engineer":
		return "editor"
	case "viewer":
		return "viewer"
	default:
		return "viewer"
	}
}
