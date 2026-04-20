package middleware

import (
	"net/http"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

// RequireRole returns middleware that restricts access to users with the given roles.
// Must be used after RequireAuth.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUser(r.Context())
			if !ok {
				apierr.Unauthorized("not authenticated").Write(w)
				return
			}
			if _, allowed := set[user.Role]; !allowed {
				apierr.Forbidden("insufficient role").Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
