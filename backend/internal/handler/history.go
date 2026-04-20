package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/pgutil"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// RecordChange represents a single audit entry for a data record.
type RecordChange struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	RecordID     string          `json:"record_id"`
	UserID       string          `json:"user_id,omitempty"`
	UserName     string          `json:"user_name,omitempty"`
	Operation    string          `json:"operation"`
	Diff         json.RawMessage `json:"diff"`
	CreatedAt    time.Time       `json:"created_at"`
}

// HistoryHandler serves record change history.
type HistoryHandler struct {
	pool  *pgxpool.Pool
	cache *schema.Cache
}

func NewHistoryHandler(pool *pgxpool.Pool, cache *schema.Cache) *HistoryHandler {
	return &HistoryHandler{pool: pool, cache: cache}
}

// ListRecordHistory returns change history for a specific record.
func (h *HistoryHandler) ListRecordHistory(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	recordID := chi.URLParam(r, "id")

	col, ok := h.cache.CollectionBySlug(slug)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("collection %q not found", slug))
		return
	}

	page, limit, offset := ParsePagination(r.URL.Query())

	var total int64
	err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM _history.record_changes WHERE collection_id = $1 AND record_id = $2`,
		col.ID, recordID,
	).Scan(&total)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, collection_id, record_id, user_id, user_name, operation, diff, created_at
		FROM _history.record_changes
		WHERE collection_id = $1 AND record_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		col.ID, recordID, limit, offset,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	var changes []RecordChange
	for rows.Next() {
		var c RecordChange
		var cid, colID, recID, userID pgtype.UUID
		var userName *string
		err := rows.Scan(&cid, &colID, &recID, &userID, &userName, &c.Operation, &c.Diff, &c.CreatedAt)
		if err != nil {
			handleErr(w, r, err)
			return
		}
		c.ID = pgutil.UUIDToString(cid)
		c.CollectionID = pgutil.UUIDToString(colID)
		c.RecordID = pgutil.UUIDToString(recID)
		c.UserID = pgutil.UUIDToString(userID)
		if userName != nil {
			c.UserName = *userName
		}
		changes = append(changes, c)
	}
	if err := rows.Err(); err != nil {
		handleErr(w, r, err)
		return
	}

	writeList(w, changes, total, page, limit)
}

// recordChange inserts a change record into _history.record_changes.
// It is called after successful Create, Update, and Delete operations.
func recordChange(
	ctx context.Context,
	pool *pgxpool.Pool,
	collectionID, recordID, userID, userName, operation string,
	diff map[string]any,
) {
	diffJSON, err := json.Marshal(diff)
	if err != nil {
		slog.Error("recordChange: marshal diff", "error", err, "collection", collectionID, "record", recordID)
		return
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO _history.record_changes (collection_id, record_id, user_id, user_name, operation, diff)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		collectionID, recordID, userID, userName, operation, diffJSON,
	); err != nil {
		slog.Error("recordChange: insert", "error", err, "collection", collectionID, "record", recordID)
	}
}

// computeDiff compares old and new records and returns only changed fields.
func computeDiff(oldRow, newRow map[string]any, fields []schema.Field) map[string]any {
	diff := make(map[string]any)
	for _, f := range fields {
		oldVal := oldRow[f.Slug]
		newVal := newRow[f.Slug]
		if !jsonEqual(oldVal, newVal) {
			diff[f.Slug] = map[string]any{"old": oldVal, "new": newVal}
		}
	}
	return diff
}

// createDiff builds a diff for a newly created record.
func createDiff(row map[string]any, fields []schema.Field) map[string]any {
	diff := make(map[string]any)
	for _, f := range fields {
		if v, ok := row[f.Slug]; ok && v != nil {
			diff[f.Slug] = map[string]any{"new": v}
		}
	}
	return diff
}

func jsonEqual(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return bytes.Equal(aj, bj)
}
