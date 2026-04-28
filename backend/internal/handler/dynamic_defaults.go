package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// GetDefaults returns smart default values for a new record based on the
// current user's recent entries. For select fields it returns the most
// frequent choice; for other types it returns the most recent value.
// Text/textarea, file, json, table, and computed/layout fields are skipped.
func (h *DynHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}

	user, ok := middleware.GetUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Filter to defaultable field types.
	var targets []schema.Field
	for _, f := range fields {
		if f.FieldType.NoColumn() {
			continue
		}
		switch f.FieldType {
		case schema.FieldText, schema.FieldTextarea,
			schema.FieldFile, schema.FieldJSON, schema.FieldTable, schema.FieldSpreadsheet,
			schema.FieldAutonumber:
			continue
		}
		targets = append(targets, f)
	}

	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Build a query that fetches mode (most frequent value) for select fields,
	// and the most recent value for other field types.
	qTable := pgutil.QuoteQualified("data", col.Slug)

	var selectParts []string
	for _, f := range targets {
		qCol := pgutil.QuoteIdent(f.Slug)
		switch f.FieldType {
		case schema.FieldSelect, schema.FieldMultiselect:
			// mode() returns the most frequent value
			selectParts = append(selectParts,
				fmt.Sprintf("(SELECT %s FROM %s WHERE created_by = $1 AND %s IS NOT NULL AND deleted_at IS NULL ORDER BY count(*) OVER (PARTITION BY %s) DESC, created_at DESC LIMIT 1) AS %s",
					qCol, qTable, qCol, qCol, qCol))
		default:
			// Most recent non-null value
			selectParts = append(selectParts,
				fmt.Sprintf("(SELECT %s FROM %s WHERE created_by = $1 AND %s IS NOT NULL AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1) AS %s",
					qCol, qTable, qCol, qCol))
		}
	}

	query := "SELECT " + strings.Join(selectParts, ", ")

	rows, err := h.pool.Query(r.Context(), query, user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query defaults")
		return
	}
	defer rows.Close()

	defaults := make(map[string]any)
	if rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan defaults")
			return
		}
		for i, f := range targets {
			if i < len(vals) && vals[i] != nil {
				defaults[f.Slug] = vals[i]
			}
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "defaults: rows iteration failed")
		return
	}

	writeJSON(w, http.StatusOK, defaults)
}
