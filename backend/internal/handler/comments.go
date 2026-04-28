package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/topbid/backend/internal/events"
	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// Comment represents a user comment on a data record.
type Comment struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collection_id"`
	RecordID     string    `json:"record_id"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CommentHandler serves comment CRUD nested under data records.
type CommentHandler struct {
	pool  *pgxpool.Pool
	cache *schema.Cache
	bus   *events.Bus
}

func NewCommentHandler(pool *pgxpool.Pool, cache *schema.Cache, bus *events.Bus) *CommentHandler {
	return &CommentHandler{pool: pool, cache: cache, bus: bus}
}

// List returns comments for a record.
func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	recordID := chi.URLParam(r, "id")

	col, ok := h.cache.CollectionBySlug(slug)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("collection %q not found", slug))
		return
	}

	page, limit, offset := ParsePagination(r.URL.Query())

	var total int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM _meta.comments WHERE collection_id = $1 AND record_id = $2`,
		col.ID, recordID,
	).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "count comments: "+err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, collection_id, record_id, user_id, user_name, body, created_at, updated_at
		FROM _meta.comments
		WHERE collection_id = $1 AND record_id = $2
		ORDER BY created_at ASC
		LIMIT $3 OFFSET $4`,
		col.ID, recordID, limit, offset,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	comments, err := scanComments(rows)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeList(w, comments, total, page, limit)
}

// Create adds a comment.
func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	recordID := chi.URLParam(r, "id")

	col, ok := h.cache.CollectionBySlug(slug)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("collection %q not found", slug))
		return
	}

	user, _ := middleware.GetUser(r.Context())

	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	var c Comment
	var id, colID, recID, userID pgtype.UUID
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO _meta.comments (collection_id, record_id, user_id, user_name, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, collection_id, record_id, user_id, user_name, body, created_at, updated_at`,
		col.ID, recordID, user.UserID, user.Name, body.Body,
	).Scan(&id, &colID, &recID, &userID, &c.UserName, &c.Body, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	c.ID = pgutil.UUIDToString(id)
	c.CollectionID = pgutil.UUIDToString(colID)
	c.RecordID = pgutil.UUIDToString(recID)
	c.UserID = pgutil.UUIDToString(userID)

	// Publish event for notifications.
	if h.bus != nil {
		h.bus.Publish(r.Context(), events.Event{
			Type:           events.EventComment,
			CollectionID:   col.ID,
			CollectionSlug: col.Slug,
			RecordID:       recordID,
			ActorUserID:    user.UserID,
			ActorName:      user.Name,
			Title:          fmt.Sprintf("%s님이 댓글을 남겼습니다", user.Name),
			Body:           body.Body,
		})
	}

	writeJSON(w, http.StatusCreated, c)
}

// Update modifies own comment.
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "commentId")
	user, _ := middleware.GetUser(r.Context())

	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	// Only own comments (or director).
	whereUser := ""
	args := []any{body.Body, commentID}
	if user.Role != "director" {
		whereUser = " AND user_id = $3"
		args = append(args, user.UserID)
	}

	tag, err := h.pool.Exec(r.Context(),
		fmt.Sprintf(`UPDATE _meta.comments SET body = $1, updated_at = now() WHERE id = $2%s`, whereUser),
		args...,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "comment not found or not yours")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete removes own comment (or any if director).
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "commentId")
	user, _ := middleware.GetUser(r.Context())

	whereUser := ""
	args := []any{commentID}
	if user.Role != "director" {
		whereUser = " AND user_id = $2"
		args = append(args, user.UserID)
	}

	tag, err := h.pool.Exec(r.Context(),
		fmt.Sprintf(`DELETE FROM _meta.comments WHERE id = $1%s`, whereUser),
		args...,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "comment not found or not yours")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func scanComments(rows pgx.Rows) ([]Comment, error) {
	var out []Comment
	for rows.Next() {
		var c Comment
		var id, colID, recID, userID pgtype.UUID
		err := rows.Scan(&id, &colID, &recID, &userID, &c.UserName, &c.Body, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		c.ID = pgutil.UUIDToString(id)
		c.CollectionID = pgutil.UUIDToString(colID)
		c.RecordID = pgutil.UUIDToString(recID)
		c.UserID = pgutil.UUIDToString(userID)
		out = append(out, c)
	}
	return out, rows.Err()
}
