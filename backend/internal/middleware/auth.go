package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

type contextKey string

const userContextKey contextKey = "user"

// ExportedUserContextKey is the context key for tests that need to inject a user.
var ExportedUserContextKey = userContextKey

// UserClaims holds JWT claims for the authenticated user.
// SupplierID is set for role="supplier" users and drives bid row filtering.
type UserClaims struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	DepartmentID string `json:"departmentId,omitempty"`
	SubsidiaryID string `json:"subsidiaryId,omitempty"`
	SupplierID   string `json:"supplierId,omitempty"`
}

// DevUser is the hard-coded user injected when auth is disabled.
var DevUser = UserClaims{
	UserID: "ce3e41d2-0abb-4c98-955c-adf1ce26717c",
	Email:  "choiceoh@topsolar.kr",
	Name:   "개발자",
	Role:   "director",
}

// RequireAuth returns middleware that validates JWT from Authorization header or cookie.
// When authDisabled is true, it skips validation and injects DevUser.
func RequireAuth(jwtSecret string, authDisabled bool) func(http.Handler) http.Handler {
	if authDisabled {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), userContextKey, DevUser)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
	}

	secret := []byte(jwtSecret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				apierr.Unauthorized("missing token").Write(w)
				return
			}

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			})
			if err != nil || !token.Valid {
				apierr.Unauthorized("invalid token").Write(w)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				apierr.Unauthorized("invalid claims").Write(w)
				return
			}

			user, err := claimsToUser(claims)
			if err != nil {
				apierr.Unauthorized("invalid claims: " + err.Error()).Write(w)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// claimsToUser safely extracts UserClaims from JWT map claims.
// Returns an error instead of panicking on type mismatch.
func claimsToUser(claims jwt.MapClaims) (UserClaims, error) {
	user := UserClaims{}

	if userID, ok := claims["userId"].(string); ok {
		user.UserID = userID
	} else {
		return user, errInvalidClaim("userId")
	}
	if email, ok := claims["email"].(string); ok {
		user.Email = email
	} else {
		return user, errInvalidClaim("email")
	}
	if name, ok := claims["name"].(string); ok {
		user.Name = name
	} else {
		return user, errInvalidClaim("name")
	}
	if role, ok := claims["role"].(string); ok {
		user.Role = role
	} else {
		return user, errInvalidClaim("role")
	}
	// Optional: departmentId (may be absent for users without a department).
	if deptID, ok := claims["departmentId"].(string); ok {
		user.DepartmentID = deptID
	}
	// Optional: subsidiaryId (may be absent for users without a subsidiary).
	if subID, ok := claims["subsidiaryId"].(string); ok {
		user.SubsidiaryID = subID
	}
	// Optional: supplierId (only set for role="supplier" users).
	if supID, ok := claims["supplierId"].(string); ok {
		user.SupplierID = supID
	}

	return user, nil
}

func errInvalidClaim(key string) error { return &claimError{kind: "invalid", key: key} }

type claimError struct{ kind, key string }

func (e *claimError) Error() string { return e.kind + " " + e.key }

// GetUser extracts the authenticated user from request context.
func GetUser(ctx context.Context) (UserClaims, bool) {
	u, ok := ctx.Value(userContextKey).(UserClaims)
	return u, ok
}

// extractToken retrieves the JWT from the request. It checks the Authorization
// header first ("Bearer <token>"), then falls back to the "token" httpOnly cookie.
// Returns "" if no token is found in either location.
func extractToken(r *http.Request) string {
	// Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// httpOnly cookie
	if c, err := r.Cookie("token"); err == nil {
		return c.Value
	}
	return ""
}

// --- Collection-level role (set by CollectionAccess middleware) ---

const collectionRoleKey contextKey = "collectionRole"

// SetCollectionRole stores the collection-level role in the request context.
func SetCollectionRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, collectionRoleKey, role)
}

// GetCollectionRole returns the collection-level role from context ("owner", "editor", "viewer", or "").
func GetCollectionRole(ctx context.Context) string {
	r, _ := ctx.Value(collectionRoleKey).(string)
	return r
}

