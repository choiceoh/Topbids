package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// SimilarRecords returns records with similar text content based on pg_trgm.
// It searches the first text/textarea field (or the field specified by ?field=slug).
// Usage: GET /api/data/{slug}/similar?q=검색어&field=title&limit=3
func (h *DynHandler) SimilarRecords(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Find the target text field.
	fieldSlug := r.URL.Query().Get("field")
	var targetField *schema.Field
	if fieldSlug != "" {
		for _, f := range fields {
			if f.Slug == fieldSlug && (f.FieldType == schema.FieldText || f.FieldType == schema.FieldTextarea) {
				targetField = &f
				break
			}
		}
	}
	// Default: pick the first text field.
	if targetField == nil {
		for _, f := range fields {
			if f.FieldType == schema.FieldText {
				targetField = &f
				break
			}
		}
	}
	if targetField == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	qCol := pgutil.QuoteIdent(targetField.Slug)

	// Use pg_trgm similarity. Falls back to ILIKE if pg_trgm is not available.
	query := fmt.Sprintf(
		`SELECT id, %s, created_at FROM %s
		 WHERE deleted_at IS NULL AND %s IS NOT NULL
		   AND similarity(%s, $1) > 0.2
		 ORDER BY similarity(%s, $1) DESC
		 LIMIT 5`,
		qCol, qTable, qCol, qCol, qCol,
	)

	rows, err := h.pool.Query(r.Context(), query, q)
	if err != nil {
		// pg_trgm might not be enabled; fall back to ILIKE.
		query = fmt.Sprintf(
			`SELECT id, %s, created_at FROM %s
			 WHERE deleted_at IS NULL AND %s ILIKE '%%' || $1 || '%%'
			 ORDER BY created_at DESC
			 LIMIT 5`,
			qCol, qTable, qCol,
		)
		rows, err = h.pool.Query(r.Context(), query, q)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
	}
	defer rows.Close()

	type similar struct {
		ID        string    `json:"id"`
		Value     string    `json:"value"`
		CreatedAt time.Time `json:"created_at"`
	}
	var results []similar
	for rows.Next() {
		var s similar
		if err := rows.Scan(&s.ID, &s.Value, &s.CreatedAt); err != nil {
			continue
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("similar records: rows iteration failed", "error", err)
	}
	if results == nil {
		results = []similar{}
	}

	writeJSON(w, http.StatusOK, results)
}
