package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
)

// User represents an auth.users row.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Password     string    `json:"-"`
	Role         string    `json:"role"`
	IsActive     bool      `json:"is_active"`
	ExternalID   *string   `json:"external_id,omitempty"`
	DepartmentID *string   `json:"department_id,omitempty"`
	Position     *string   `json:"position,omitempty"`
	Title        *string   `json:"title,omitempty"`
	Phone        *string   `json:"phone,omitempty"`
	Avatar       *string   `json:"avatar,omitempty"`
	JoinedAt     *string   `json:"joined_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	SubsidiaryID *string `json:"subsidiary_id,omitempty"`
	// SupplierID links a role="supplier" user to a row in data.suppliers.
	// Populated for supplier-portal users only; nil for internal staff.
	SupplierID *string `json:"supplier_id,omitempty"`

	// Joined fields (populated by detail/list queries).
	DepartmentName *string `json:"department_name,omitempty"`
	SubsidiaryName *string `json:"subsidiary_name,omitempty"`
}

// Role constants.
const (
	RoleDirector = "director"
	RolePM       = "pm"
	RoleEngineer = "engineer"
	RoleViewer   = "viewer"
	// RoleSupplier is the external-bidder role. Users with this role must
	// have SupplierID set and are restricted to their own bid rows via the
	// row-level filter in handler/dynamic.go.
	RoleSupplier = "supplier"
)

// LoginInput is the request body for /api/auth/login.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResponse is returned on successful login.
type TokenResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// normalizeEmail lowercases and trims whitespace.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Login handles POST /api/auth/login.
func Login(pool *pgxpool.Pool, limiter *middleware.RateLimiter, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input LoginInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}
		defer r.Body.Close()

		ip := clientIP(r)

		if limiter != nil {
			if allowed, retryMs := limiter.Check(ip); !allowed {
				apierr.New(http.StatusTooManyRequests, "RATE_LIMITED",
					"too many failed attempts, try again later").
					With("retryAfterMs", retryMs).
					Write(w)
				return
			}
		}

		email := normalizeEmail(input.Email)

		var user User
		err := pool.QueryRow(r.Context(),
			`SELECT id, email, name, password, role, is_active,
			        external_id, department_id, subsidiary_id, supplier_id, position, title, phone, avatar,
			        joined_at::text
			 FROM auth.users WHERE LOWER(email) = $1`,
			email,
		).Scan(&user.ID, &user.Email, &user.Name, &user.Password, &user.Role, &user.IsActive,
			&user.ExternalID, &user.DepartmentID, &user.SubsidiaryID, &user.SupplierID, &user.Position, &user.Title, &user.Phone, &user.Avatar,
			&user.JoinedAt)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if limiter != nil {
					limiter.RecordFailure(ip)
				}
				apierr.Unauthorized("invalid credentials").Write(w)
				return
			}
			slog.Error("login: db query failed", "error", err, "email", email)
			apierr.WrapInternal("query user", err).Write(w)
			return
		}

		if !user.IsActive {
			apierr.Forbidden("account deactivated").Write(w)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
			if limiter != nil {
				limiter.RecordFailure(ip)
			}
			apierr.Unauthorized("invalid credentials").Write(w)
			return
		}

		if limiter != nil {
			limiter.Reset(ip)
		}

		tokenStr, err := generateToken(user, jwtSecret)
		if err != nil {
			slog.Error("login: token generation failed", "error", err)
			apierr.Internal("failed to generate token").Write(w)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    tokenStr,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7,
		})

		user.Password = ""
		writeJSON(w, http.StatusOK, TokenResponse{Token: tokenStr, User: user})
	}
}

// Logout handles POST /api/auth/logout.
// Clears the httpOnly cookie server-side so the client doesn't have to.
func Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}

// Me handles GET /api/auth/me.
func Me(pool *pgxpool.Pool, authDisabled ...bool) http.HandlerFunc {
	disabled := len(authDisabled) > 0 && authDisabled[0]
	return func(w http.ResponseWriter, r *http.Request) {
		if disabled {
			dev := middleware.DevUser
			now := time.Now()
			writeJSON(w, http.StatusOK, User{
				ID:        dev.UserID,
				Email:     dev.Email,
				Name:      dev.Name,
				Role:      dev.Role,
				IsActive:  true,
				CreatedAt: now,
				UpdatedAt: now,
			})
			return
		}

		claims, ok := middleware.GetUser(r.Context())
		if !ok {
			apierr.Unauthorized("not authenticated").Write(w)
			return
		}
		var user User
		err := pool.QueryRow(r.Context(),
			`SELECT id, email, name, role, is_active,
			        external_id, department_id, subsidiary_id, position, title, phone, avatar,
			        joined_at::text, created_at, updated_at
			 FROM auth.users WHERE id = $1`, claims.UserID,
		).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.IsActive,
			&user.ExternalID, &user.DepartmentID, &user.SubsidiaryID, &user.Position, &user.Title, &user.Phone, &user.Avatar,
			&user.JoinedAt, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			slog.Error("me: query failed", "error", err)
			apierr.WrapInternal("query current user", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

// CreateUser handles POST /api/users (director only).
func CreateUser(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := middleware.GetUser(r.Context())
		if !ok {
			apierr.Unauthorized("not authenticated").Write(w)
			return
		}
		if caller.Role != RoleDirector {
			apierr.Forbidden("directors only").Write(w)
			return
		}

		var input struct {
			Email        string  `json:"email"`
			Name         string  `json:"name"`
			Password     string  `json:"password"`
			Role         string  `json:"role"`
			DepartmentID *string `json:"department_id"`
			SubsidiaryID *string `json:"subsidiary_id"`
			// SupplierID is required when Role == "supplier" and must reference
			// a row in data.suppliers. Ignored for other roles.
			SupplierID *string `json:"supplier_id"`
			Position   *string `json:"position"`
			Title      *string `json:"title"`
			Phone      *string `json:"phone"`
			JoinedAt   *string `json:"joined_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}
		defer r.Body.Close()

		email := normalizeEmail(input.Email)
		if email == "" || input.Password == "" || input.Name == "" {
			apierr.BadRequest("email, name, and password are required").Write(w)
			return
		}

		switch input.Role {
		case RoleDirector, RolePM, RoleEngineer, RoleViewer:
		case RoleSupplier:
			// Suppliers must link to a company row; otherwise SupplierRowFilter
			// locks them out and the account is unusable. Validate the linkage
			// exists rather than trusting a raw id — saves a confusing 403
			// later when the user first tries to list bids.
			if input.SupplierID == nil || *input.SupplierID == "" {
				apierr.BadRequest("supplier_id is required for role=supplier").Write(w)
				return
			}
			var exists bool
			if err := pool.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM data.suppliers WHERE id = $1 AND deleted_at IS NULL)`,
				*input.SupplierID,
			).Scan(&exists); err != nil {
				slog.Error("create user: verify supplier", "error", err)
				apierr.WrapInternal("verify supplier", err).Write(w)
				return
			}
			if !exists {
				apierr.BadRequest("supplier_id does not reference an active supplier").Write(w)
				return
			}
		default:
			apierr.BadRequest("invalid role").Write(w)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("create user: bcrypt failed", "error", err)
			apierr.Internal("failed to hash password").Write(w)
			return
		}

		// Normalize empty IDs to nil. Suppliers don't belong to a department
		// or subsidiary — clear those even if the client sent them.
		if input.DepartmentID != nil && *input.DepartmentID == "" {
			input.DepartmentID = nil
		}
		if input.SubsidiaryID != nil && *input.SubsidiaryID == "" {
			input.SubsidiaryID = nil
		}
		if input.Role == RoleSupplier {
			input.DepartmentID = nil
			input.SubsidiaryID = nil
		} else {
			input.SupplierID = nil
		}

		var id string
		err = pool.QueryRow(r.Context(),
			`INSERT INTO auth.users
			   (email, name, password, role, department_id, subsidiary_id,
			    supplier_id, position, title, phone, joined_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 RETURNING id`,
			email, input.Name, string(hash), input.Role,
			input.DepartmentID, input.SubsidiaryID, input.SupplierID,
			input.Position, input.Title, input.Phone, input.JoinedAt,
		).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
				apierr.Conflict("email already exists").Write(w)
				return
			}
			slog.Error("create user: insert failed", "error", err)
			apierr.WrapInternal("create user", err).Write(w)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// ListUsers handles GET /api/users.
// Optional query params: ?department_id=... to filter by department.
func ListUsers(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := `SELECT u.id, u.email, u.name, u.role, u.is_active,
		             u.external_id, u.department_id, u.subsidiary_id, u.supplier_id,
		             u.position, u.title, u.phone, u.avatar, u.joined_at,
		             u.created_at, u.updated_at,
		             d.name AS department_name,
		             s.name AS subsidiary_name
		      FROM auth.users u
		      LEFT JOIN auth.departments d ON d.id = u.department_id
		      LEFT JOIN auth.subsidiaries s ON s.id = u.subsidiary_id`

		var args []any
		var wheres []string
		argN := 1
		if deptID := r.URL.Query().Get("department_id"); deptID != "" {
			wheres = append(wheres, fmt.Sprintf("u.department_id = $%d", argN))
			args = append(args, deptID)
			argN++
		}
		if subID := r.URL.Query().Get("subsidiary_id"); subID != "" {
			wheres = append(wheres, fmt.Sprintf("u.subsidiary_id = $%d", argN))
			args = append(args, subID)
		}
		if len(wheres) > 0 {
			q += " WHERE " + strings.Join(wheres, " AND ")
		}
		q += ` ORDER BY u.name`

		rows, err := pool.Query(r.Context(), q, args...)
		if err != nil {
			slog.Error("list users: query failed", "error", err)
			apierr.WrapInternal("query users", err).Write(w)
			return
		}
		defer rows.Close()

		users := make([]User, 0)
		for rows.Next() {
			var u User
			if err := rows.Scan(
				&u.ID, &u.Email, &u.Name, &u.Role, &u.IsActive,
				&u.ExternalID, &u.DepartmentID, &u.SubsidiaryID, &u.SupplierID,
				&u.Position, &u.Title, &u.Phone, &u.Avatar, &u.JoinedAt,
				&u.CreatedAt, &u.UpdatedAt,
				&u.DepartmentName, &u.SubsidiaryName,
			); err != nil {
				slog.Warn("list users: scan row failed", "error", err)
				continue
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			slog.Error("list users: rows iteration failed", "error", err)
			apierr.Internal("list users failed").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

// GetUser handles GET /api/users/{id}.
func GetUser(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var u User
		err := pool.QueryRow(r.Context(),
			`SELECT u.id, u.email, u.name, u.role, u.is_active,
			        u.external_id, u.department_id, u.subsidiary_id,
			        u.position, u.title, u.phone, u.avatar, u.joined_at,
			        u.created_at, u.updated_at,
			        d.name AS department_name,
			        s.name AS subsidiary_name
			 FROM auth.users u
			 LEFT JOIN auth.departments d ON d.id = u.department_id
			 LEFT JOIN auth.subsidiaries s ON s.id = u.subsidiary_id
			 WHERE u.id = $1`, id,
		).Scan(
			&u.ID, &u.Email, &u.Name, &u.Role, &u.IsActive,
			&u.ExternalID, &u.DepartmentID, &u.SubsidiaryID,
			&u.Position, &u.Title, &u.Phone, &u.Avatar, &u.JoinedAt,
			&u.CreatedAt, &u.UpdatedAt,
			&u.DepartmentName, &u.SubsidiaryName,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierr.NotFound("user").Write(w)
				return
			}
			apierr.WrapInternal("get user", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

// UpdateUser handles PATCH /api/users/{id} (director only).
// Allows updating name, email, role, is_active, department_id, position, title, phone, avatar, joined_at, password.
func UpdateUser(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := middleware.GetUser(r.Context())
		if !ok {
			apierr.Unauthorized("not authenticated").Write(w)
			return
		}
		if caller.Role != RoleDirector {
			apierr.Forbidden("directors only").Write(w)
			return
		}

		userID := chi.URLParam(r, "id")

		var input struct {
			Name         *string `json:"name"`
			Email        *string `json:"email"`
			Role         *string `json:"role"`
			IsActive     *bool   `json:"is_active"`
			DepartmentID *string `json:"department_id"`
			SubsidiaryID *string `json:"subsidiary_id"`
			SupplierID   *string `json:"supplier_id"`
			Position     *string `json:"position"`
			Title        *string `json:"title"`
			Phone        *string `json:"phone"`
			Avatar       *string `json:"avatar"`
			JoinedAt     *string `json:"joined_at"`
			Password     *string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}
		defer r.Body.Close()

		sets := []string{}
		args := []any{}
		n := 1

		addSet := func(col string, val any) {
			sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
			args = append(args, val)
			n++
		}

		if input.Name != nil {
			addSet("name", *input.Name)
		}
		if input.Email != nil {
			addSet("email", normalizeEmail(*input.Email))
		}
		if input.Role != nil {
			switch *input.Role {
			case RoleDirector, RolePM, RoleEngineer, RoleViewer, RoleSupplier:
			default:
				apierr.BadRequest("invalid role").Write(w)
				return
			}
			addSet("role", *input.Role)
			// Switching to supplier: supplier_id must be set — either in this
			// same request body or already on the row. Reject the update
			// rather than let the account become unusable.
			if *input.Role == RoleSupplier {
				hasSupplier := input.SupplierID != nil && *input.SupplierID != ""
				if !hasSupplier {
					var existing *string
					err := pool.QueryRow(r.Context(),
						`SELECT supplier_id::text FROM auth.users WHERE id = $1`, userID,
					).Scan(&existing)
					if err != nil || existing == nil || *existing == "" {
						apierr.BadRequest("switching to role=supplier requires supplier_id").Write(w)
						return
					}
				}
			}
		}
		if input.SupplierID != nil {
			if *input.SupplierID == "" {
				addSet("supplier_id", nil)
			} else {
				var exists bool
				if err := pool.QueryRow(r.Context(),
					`SELECT EXISTS(SELECT 1 FROM data.suppliers WHERE id = $1 AND deleted_at IS NULL)`,
					*input.SupplierID,
				).Scan(&exists); err != nil {
					slog.Error("update user: verify supplier", "error", err)
					apierr.WrapInternal("verify supplier", err).Write(w)
					return
				}
				if !exists {
					apierr.BadRequest("supplier_id does not reference an active supplier").Write(w)
					return
				}
				addSet("supplier_id", *input.SupplierID)
			}
		}
		if input.IsActive != nil {
			addSet("is_active", *input.IsActive)
		}
		if input.DepartmentID != nil {
			if *input.DepartmentID == "" {
				addSet("department_id", nil)
			} else {
				addSet("department_id", *input.DepartmentID)
			}
		}
		if input.SubsidiaryID != nil {
			if *input.SubsidiaryID == "" {
				addSet("subsidiary_id", nil)
			} else {
				addSet("subsidiary_id", *input.SubsidiaryID)
			}
		}
		if input.Position != nil {
			addSet("position", *input.Position)
		}
		if input.Title != nil {
			addSet("title", *input.Title)
		}
		if input.Phone != nil {
			addSet("phone", *input.Phone)
		}
		if input.Avatar != nil {
			addSet("avatar", *input.Avatar)
		}
		if input.JoinedAt != nil {
			if *input.JoinedAt == "" {
				addSet("joined_at", nil)
			} else {
				addSet("joined_at", *input.JoinedAt)
			}
		}
		if input.Password != nil && *input.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
			if err != nil {
				slog.Error("update user: bcrypt failed", "error", err)
				apierr.Internal("failed to hash password").Write(w)
				return
			}
			addSet("password", string(hash))
		}

		if len(sets) == 0 {
			apierr.BadRequest("no fields to update").Write(w)
			return
		}

		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf("UPDATE auth.users SET %s WHERE id = $%d",
			strings.Join(sets, ", "), n)
		args = append(args, userID)

		tag, err := pool.Exec(r.Context(), query, args...)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
				apierr.Conflict("email already exists").Write(w)
				return
			}
			slog.Error("update user: exec failed", "error", err)
			apierr.WrapInternal("update user", err).Write(w)
			return
		}
		if tag.RowsAffected() == 0 {
			apierr.NotFound("user not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// UpdateMe handles PATCH /api/auth/me (profile edit by current user).
func UpdateMe(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.GetUser(r.Context())
		if !ok {
			apierr.Unauthorized("not authenticated").Write(w)
			return
		}

		var input struct {
			Name     *string `json:"name"`
			Phone    *string `json:"phone"`
			Avatar   *string `json:"avatar"`
			Position *string `json:"position"`
			Title    *string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}
		defer r.Body.Close()

		sets := []string{}
		args := []any{}
		n := 1

		if input.Name != nil {
			if *input.Name == "" {
				apierr.BadRequest("name cannot be empty").Write(w)
				return
			}
			sets = append(sets, fmt.Sprintf("name = $%d", n))
			args = append(args, *input.Name)
			n++
		}
		if input.Phone != nil {
			sets = append(sets, fmt.Sprintf("phone = $%d", n))
			args = append(args, *input.Phone)
			n++
		}
		if input.Avatar != nil {
			sets = append(sets, fmt.Sprintf("avatar = $%d", n))
			args = append(args, *input.Avatar)
			n++
		}
		if input.Position != nil {
			sets = append(sets, fmt.Sprintf("position = $%d", n))
			args = append(args, *input.Position)
			n++
		}
		if input.Title != nil {
			sets = append(sets, fmt.Sprintf("title = $%d", n))
			args = append(args, *input.Title)
			n++
		}

		if len(sets) == 0 {
			apierr.BadRequest("no fields to update").Write(w)
			return
		}

		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf("UPDATE auth.users SET %s WHERE id = $%d",
			strings.Join(sets, ", "), n)
		args = append(args, claims.UserID)

		if _, err := pool.Exec(r.Context(), query, args...); err != nil {
			slog.Error("update me: exec failed", "error", err)
			apierr.WrapInternal("update profile", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// ChangePassword handles POST /api/auth/password (current user).
func ChangePassword(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.GetUser(r.Context())
		if !ok {
			apierr.Unauthorized("not authenticated").Write(w)
			return
		}

		var input struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}
		defer r.Body.Close()

		if input.CurrentPassword == "" || input.NewPassword == "" {
			apierr.BadRequest("current_password and new_password are required").Write(w)
			return
		}
		if len(input.NewPassword) < 6 {
			apierr.BadRequest("new password must be at least 6 characters").Write(w)
			return
		}

		var currentHash string
		err := pool.QueryRow(r.Context(),
			`SELECT password FROM auth.users WHERE id = $1`, claims.UserID,
		).Scan(&currentHash)
		if err != nil {
			slog.Error("change password: query failed", "error", err)
			apierr.WrapInternal("query password", err).Write(w)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(input.CurrentPassword)); err != nil {
			apierr.BadRequest("current password is incorrect").Write(w)
			return
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("change password: bcrypt failed", "error", err)
			apierr.Internal("failed to hash password").Write(w)
			return
		}

		if _, err := pool.Exec(r.Context(),
			`UPDATE auth.users SET password = $1, updated_at = now() WHERE id = $2`,
			string(newHash), claims.UserID); err != nil {
			slog.Error("change password: update failed", "error", err)
			apierr.WrapInternal("change password", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "changed"})
	}
}

func generateToken(user User, jwtSecret string) (string, error) {
	claims := jwt.MapClaims{
		"userId": user.ID,
		"email":  user.Email,
		"name":   user.Name,
		"role":   user.Role,
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	if user.DepartmentID != nil {
		claims["departmentId"] = *user.DepartmentID
	}
	if user.SubsidiaryID != nil {
		claims["subsidiaryId"] = *user.SubsidiaryID
	}
	if user.SupplierID != nil {
		claims["supplierId"] = *user.SupplierID
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// clientIP extracts the client IP from the request.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

// SeedDirector creates an initial director user if none exist.
func SeedDirector(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth.users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("135792ch"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO auth.users (email, name, password, role) VALUES ($1, $2, $3, $4)`,
		"choiceoh@topsolar.kr", "관리자", string(hash), RoleDirector,
	)
	return err
}
