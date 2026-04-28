package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/topbid/backend/internal/infra/apierr"
)

// Subsidiary represents an auth.subsidiaries row.
type Subsidiary struct {
	ID           string    `json:"id"`
	ExternalCode *string   `json:"external_code,omitempty"`
	Name         string    `json:"name"`
	SortOrder    int       `json:"sort_order"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListSubsidiaries handles GET /api/subsidiaries.
func ListSubsidiaries(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(),
			`SELECT id, external_code, name, sort_order, is_active, created_at, updated_at
			 FROM auth.subsidiaries ORDER BY sort_order, name`)
		if err != nil {
			slog.Error("list subsidiaries: query failed", "error", err)
			apierr.WrapInternal("query subsidiaries", err).Write(w)
			return
		}
		defer rows.Close()

		subs := make([]Subsidiary, 0)
		for rows.Next() {
			var s Subsidiary
			if err := rows.Scan(&s.ID, &s.ExternalCode, &s.Name, &s.SortOrder, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
				slog.Warn("list subsidiaries: scan failed", "error", err)
				continue
			}
			subs = append(subs, s)
		}
		if err := rows.Err(); err != nil {
			slog.Error("list subsidiaries: rows iteration failed", "error", err)
		}

		writeJSON(w, http.StatusOK, subs)
	}
}

// GetSubsidiary handles GET /api/subsidiaries/{id}.
func GetSubsidiary(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var s Subsidiary
		err := pool.QueryRow(r.Context(),
			`SELECT id, external_code, name, sort_order, is_active, created_at, updated_at
			 FROM auth.subsidiaries WHERE id = $1`, id,
		).Scan(&s.ID, &s.ExternalCode, &s.Name, &s.SortOrder, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierr.NotFound("subsidiary not found").Write(w)
				return
			}
			slog.Error("get subsidiary: query failed", "error", err)
			apierr.WrapInternal("query subsidiary", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, s)
	}
}

// CreateSubsidiary handles POST /api/subsidiaries (director only).
func CreateSubsidiary(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ExternalCode *string `json:"external_code"`
			Name         string  `json:"name"`
			SortOrder    int     `json:"sort_order"`
			IsActive     *bool   `json:"is_active"`
		}
		if err := readJSON(r, &input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}

		if input.Name == "" {
			apierr.BadRequest("name is required").Write(w)
			return
		}

		isActive := true
		if input.IsActive != nil {
			isActive = *input.IsActive
		}

		var s Subsidiary
		err := pool.QueryRow(r.Context(),
			`INSERT INTO auth.subsidiaries (external_code, name, sort_order, is_active)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, external_code, name, sort_order, is_active, created_at, updated_at`,
			input.ExternalCode, input.Name, input.SortOrder, isActive,
		).Scan(&s.ID, &s.ExternalCode, &s.Name, &s.SortOrder, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
				apierr.Conflict("external_code already exists").Write(w)
				return
			}
			apierr.WrapInternal("create subsidiary", err).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, s)
	}
}

// UpdateSubsidiary handles PATCH /api/subsidiaries/{id} (director only).
func UpdateSubsidiary(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			ExternalCode *string `json:"external_code"`
			Name         *string `json:"name"`
			SortOrder    *int    `json:"sort_order"`
			IsActive     *bool   `json:"is_active"`
		}
		if err := readJSON(r, &input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}

		sets := []string{}
		args := []any{}
		argN := 1

		if input.ExternalCode != nil {
			sets = append(sets, fmt.Sprintf("external_code = $%d", argN))
			args = append(args, *input.ExternalCode)
			argN++
		}
		if input.Name != nil {
			if *input.Name == "" {
				apierr.BadRequest("name cannot be empty").Write(w)
				return
			}
			sets = append(sets, fmt.Sprintf("name = $%d", argN))
			args = append(args, *input.Name)
			argN++
		}
		if input.SortOrder != nil {
			sets = append(sets, fmt.Sprintf("sort_order = $%d", argN))
			args = append(args, *input.SortOrder)
			argN++
		}
		if input.IsActive != nil {
			sets = append(sets, fmt.Sprintf("is_active = $%d", argN))
			args = append(args, *input.IsActive)
			argN++
		}

		if len(sets) == 0 {
			apierr.BadRequest("no fields to update").Write(w)
			return
		}

		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf("UPDATE auth.subsidiaries SET %s WHERE id = $%d",
			strings.Join(sets, ", "), argN)
		args = append(args, id)

		tag, err := pool.Exec(r.Context(), query, args...)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
				apierr.Conflict("external_code already exists").Write(w)
				return
			}
			slog.Error("update subsidiary: exec failed", "error", err)
			apierr.WrapInternal("update subsidiary", err).Write(w)
			return
		}
		if tag.RowsAffected() == 0 {
			apierr.NotFound("subsidiary not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// DeleteSubsidiary handles DELETE /api/subsidiaries/{id} (director only).
func DeleteSubsidiary(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		tag, err := pool.Exec(r.Context(),
			`DELETE FROM auth.subsidiaries WHERE id = $1`, id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
				apierr.BadRequest("subsidiary has assigned departments or users").Write(w)
				return
			}
			slog.Error("delete subsidiary: exec failed", "error", err)
			apierr.WrapInternal("delete subsidiary", err).Write(w)
			return
		}
		if tag.RowsAffected() == 0 {
			apierr.NotFound("subsidiary not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}
