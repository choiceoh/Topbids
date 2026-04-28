package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/infra/apierr"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// RequireCollectionOwnerOrRole allows access if the user has one of the given
// system roles (e.g. director, pm) OR is the creator of the collection
// identified by the {id} URL parameter.
func RequireCollectionOwnerOrRole(cache *schema.Cache, roles ...string) func(http.Handler) http.Handler {
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUser(r.Context())
			if !ok {
				apierr.Unauthorized("not authenticated").Write(w)
				return
			}

			// Allow if user has a privileged system role.
			if _, allowed := roleSet[user.Role]; allowed {
				next.ServeHTTP(w, r)
				return
			}

			// Allow if user is the creator of this collection.
			collectionID := chi.URLParam(r, "id")
			if collectionID != "" {
				if col, found := cache.CollectionByID(collectionID); found {
					if col.CreatedBy != "" && col.CreatedBy == user.UserID {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			apierr.Forbidden("insufficient role").Write(w)
		})
	}
}

// RequireFieldOwnerOrRole is like RequireCollectionOwnerOrRole but resolves
// the collection from a {fieldId} URL parameter via the cache.
func RequireFieldOwnerOrRole(cache *schema.Cache, roles ...string) func(http.Handler) http.Handler {
	roleSet := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		roleSet[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUser(r.Context())
			if !ok {
				apierr.Unauthorized("not authenticated").Write(w)
				return
			}

			// Allow if user has a privileged system role.
			if _, allowed := roleSet[user.Role]; allowed {
				next.ServeHTTP(w, r)
				return
			}

			// Resolve field → collection, then check ownership.
			fieldID := chi.URLParam(r, "fieldId")
			if fieldID != "" {
				if col, found := cache.CollectionByFieldID(fieldID); found {
					if col.CreatedBy != "" && col.CreatedBy == user.UserID {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			apierr.Forbidden("insufficient role").Write(w)
		})
	}
}
