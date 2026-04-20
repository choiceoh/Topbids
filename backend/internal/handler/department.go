package handler

import (
	"context"
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

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

// Department represents an auth.departments row.
type Department struct {
	ID             string        `json:"id"`
	ExternalCode   *string       `json:"external_code,omitempty"`
	Name           string        `json:"name"`
	ParentID       *string       `json:"parent_id,omitempty"`
	SubsidiaryID   *string       `json:"subsidiary_id,omitempty"`
	SortOrder      int           `json:"sort_order"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	SubsidiaryName *string       `json:"subsidiary_name,omitempty"`
	Children       []*Department `json:"children,omitempty"`
}

// ListDepartments handles GET /api/departments.
// Returns a flat list by default; ?tree=true returns a nested tree.
func ListDepartments(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := `SELECT d.id, d.external_code, d.name, d.parent_id, d.subsidiary_id,
		                 d.sort_order, d.created_at, d.updated_at, s.name
		          FROM auth.departments d
		          LEFT JOIN auth.subsidiaries s ON s.id = d.subsidiary_id`

		args := []any{}
		if subID := r.URL.Query().Get("subsidiary_id"); subID != "" {
			query += ` WHERE d.subsidiary_id = $1`
			args = append(args, subID)
		}
		query += ` ORDER BY d.sort_order, d.name`

		rows, err := pool.Query(r.Context(), query, args...)
		if err != nil {
			slog.Error("list departments: query failed", "error", err)
			apierr.WrapInternal("query departments", err).Write(w)
			return
		}
		defer rows.Close()

		depts := make([]Department, 0)
		for rows.Next() {
			var d Department
			if err := rows.Scan(&d.ID, &d.ExternalCode, &d.Name, &d.ParentID, &d.SubsidiaryID,
				&d.SortOrder, &d.CreatedAt, &d.UpdatedAt, &d.SubsidiaryName); err != nil {
				slog.Warn("list departments: scan failed", "error", err)
				continue
			}
			depts = append(depts, d)
		}
		if err := rows.Err(); err != nil {
			slog.Error("list departments: rows iteration failed", "error", err)
		}

		if r.URL.Query().Get("tree") == "true" {
			writeJSON(w, http.StatusOK, buildTree(depts))
			return
		}
		writeJSON(w, http.StatusOK, depts)
	}
}

// buildTree converts a flat list to a nested tree rooted at parent_id IS NULL.
func buildTree(depts []Department) []*Department {
	byID := make(map[string]*Department, len(depts))
	for i := range depts {
		d := &depts[i]
		d.Children = []*Department{}
		byID[d.ID] = d
	}

	var roots []*Department
	for _, d := range byID {
		if d.ParentID == nil {
			roots = append(roots, d)
		} else if parent, ok := byID[*d.ParentID]; ok {
			parent.Children = append(parent.Children, d)
		} else {
			roots = append(roots, d)
		}
	}
	return roots
}

// GetDepartment handles GET /api/departments/{id}.
func GetDepartment(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var d Department
		err := pool.QueryRow(r.Context(),
			`SELECT d.id, d.external_code, d.name, d.parent_id, d.subsidiary_id,
			        d.sort_order, d.created_at, d.updated_at, s.name
			 FROM auth.departments d
			 LEFT JOIN auth.subsidiaries s ON s.id = d.subsidiary_id
			 WHERE d.id = $1`, id,
		).Scan(&d.ID, &d.ExternalCode, &d.Name, &d.ParentID, &d.SubsidiaryID,
			&d.SortOrder, &d.CreatedAt, &d.UpdatedAt, &d.SubsidiaryName)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierr.NotFound("department not found").Write(w)
				return
			}
			slog.Error("get department: query failed", "error", err)
			apierr.WrapInternal("query department", err).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

// CreateDepartment handles POST /api/departments (director only).
func CreateDepartment(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ExternalCode *string `json:"external_code"`
			Name         string  `json:"name"`
			ParentID     *string `json:"parent_id"`
			SubsidiaryID *string `json:"subsidiary_id"`
			SortOrder    int     `json:"sort_order"`
		}
		if err := readJSON(r, &input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}

		if input.Name == "" {
			apierr.BadRequest("name is required").Write(w)
			return
		}

		var d Department
		err := pool.QueryRow(r.Context(),
			`INSERT INTO auth.departments (external_code, name, parent_id, subsidiary_id, sort_order)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, external_code, name, parent_id, subsidiary_id, sort_order, created_at, updated_at`,
			input.ExternalCode, input.Name, input.ParentID, input.SubsidiaryID, input.SortOrder,
		).Scan(&d.ID, &d.ExternalCode, &d.Name, &d.ParentID, &d.SubsidiaryID, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case pgerrcode.UniqueViolation:
					apierr.Conflict("external_code already exists").Write(w)
					return
				case pgerrcode.ForeignKeyViolation:
					apierr.BadRequest("invalid parent_id").Write(w)
					return
				}
			}
			apierr.WrapInternal("create department", err).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

// UpdateDepartment handles PATCH /api/departments/{id} (director only).
func UpdateDepartment(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			ExternalCode *string `json:"external_code"`
			Name         *string `json:"name"`
			ParentID     *string `json:"parent_id"`
			SubsidiaryID *string `json:"subsidiary_id"`
			SortOrder    *int    `json:"sort_order"`
		}
		if err := readJSON(r, &input); err != nil {
			apierr.BadRequest("invalid request body").Write(w)
			return
		}

		// Prevent self-reference.
		if input.ParentID != nil && *input.ParentID == id {
			apierr.BadRequest("department cannot be its own parent").Write(w)
			return
		}

		// Prevent circular parent chain (A→B→C→A).
		if input.ParentID != nil && *input.ParentID != "" {
			if err := detectCircularDepartment(r.Context(), pool, id, *input.ParentID); err != nil {
				apierr.BadRequest(err.Error()).Write(w)
				return
			}
		}

		// Build dynamic SET clause.
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
		if input.ParentID != nil {
			if *input.ParentID == "" {
				sets = append(sets, fmt.Sprintf("parent_id = $%d", argN))
				args = append(args, nil)
			} else {
				sets = append(sets, fmt.Sprintf("parent_id = $%d", argN))
				args = append(args, *input.ParentID)
			}
			argN++
		}
		if input.SubsidiaryID != nil {
			if *input.SubsidiaryID == "" {
				sets = append(sets, fmt.Sprintf("subsidiary_id = $%d", argN))
				args = append(args, nil)
			} else {
				sets = append(sets, fmt.Sprintf("subsidiary_id = $%d", argN))
				args = append(args, *input.SubsidiaryID)
			}
			argN++
		}
		if input.SortOrder != nil {
			sets = append(sets, fmt.Sprintf("sort_order = $%d", argN))
			args = append(args, *input.SortOrder)
			argN++
		}

		if len(sets) == 0 {
			apierr.BadRequest("no fields to update").Write(w)
			return
		}

		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf("UPDATE auth.departments SET %s WHERE id = $%d",
			strings.Join(sets, ", "), argN)
		args = append(args, id)

		tag, err := pool.Exec(r.Context(), query, args...)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case pgerrcode.ForeignKeyViolation:
					apierr.BadRequest("invalid parent department").Write(w)
					return
				case pgerrcode.UniqueViolation:
					apierr.Conflict("external_code already exists").Write(w)
					return
				}
			}
			slog.Error("update department: exec failed", "error", err)
			apierr.WrapInternal("update department", err).Write(w)
			return
		}
		if tag.RowsAffected() == 0 {
			apierr.NotFound("department not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// detectCircularDepartment walks up the parent chain from newParentID and
// returns an error if it reaches targetID, indicating a circular reference.
func detectCircularDepartment(ctx context.Context, pool *pgxpool.Pool, targetID, newParentID string) error {
	visited := map[string]bool{targetID: true}
	current := newParentID
	for i := 0; i < 100; i++ { // depth limit
		if visited[current] {
			return fmt.Errorf("circular department reference detected")
		}
		visited[current] = true
		var parentID *string
		err := pool.QueryRow(ctx,
			`SELECT parent_id FROM auth.departments WHERE id = $1`, current,
		).Scan(&parentID)
		if err != nil {
			return nil // parent not found — chain ends
		}
		if parentID == nil || *parentID == "" {
			return nil // reached root
		}
		current = *parentID
	}
	return fmt.Errorf("department hierarchy too deep")
}

// DeleteDepartment handles DELETE /api/departments/{id} (director only).
func DeleteDepartment(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		tag, err := pool.Exec(r.Context(),
			`DELETE FROM auth.departments WHERE id = $1`, id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
				apierr.BadRequest("department has child departments or assigned users").Write(w)
				return
			}
			slog.Error("delete department: exec failed", "error", err)
			apierr.WrapInternal("delete department", err).Write(w)
			return
		}
		if tag.RowsAffected() == 0 {
			apierr.NotFound("department not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}
