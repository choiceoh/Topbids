package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/topbid/backend/internal/infra/apierr"
	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
)

// CollectionMember represents a user's role within a collection.
type CollectionMember struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collection_id"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name,omitempty"`
	UserEmail    string    `json:"user_email,omitempty"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// MemberHandler serves collection membership CRUD.
type MemberHandler struct {
	pool *pgxpool.Pool
}

func NewMemberHandler(pool *pgxpool.Pool) *MemberHandler {
	return &MemberHandler{pool: pool}
}

// List returns members of a collection with pagination.
func (h *MemberHandler) List(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	page, limit, offset := ParsePagination(r.URL.Query())

	var total int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM _meta.collection_members WHERE collection_id = $1`,
		collectionID,
	).Scan(&total); err != nil {
		handleErr(w, r, err)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT m.id, m.collection_id, m.user_id, u.name, u.email, m.role, m.created_at
		FROM _meta.collection_members m
		JOIN auth.users u ON u.id = m.user_id
		WHERE m.collection_id = $1
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`, collectionID, limit, offset)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	var members []CollectionMember
	for rows.Next() {
		var m CollectionMember
		var id, colID, userID pgtype.UUID
		err := rows.Scan(&id, &colID, &userID, &m.UserName, &m.UserEmail, &m.Role, &m.CreatedAt)
		if err != nil {
			handleErr(w, r, err)
			return
		}
		m.ID = pgutil.UUIDToString(id)
		m.CollectionID = pgutil.UUIDToString(colID)
		m.UserID = pgutil.UUIDToString(userID)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		handleErr(w, r, err)
		return
	}
	writeList(w, members, total, page, limit)
}

// Add adds a user as a member of a collection.
func (h *MemberHandler) Add(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")

	// Only director or collection owner can add members.
	user, _ := middleware.GetUser(r.Context())
	if user.Role != "director" {
		apierr.Forbidden("only directors can manage members").Write(w)
		return
	}

	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if body.Role == "" {
		body.Role = "viewer"
	}
	if body.Role != "owner" && body.Role != "editor" && body.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be owner, editor, or viewer")
		return
	}

	// Verify user exists before adding as member.
	var userExists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM auth.users WHERE id = $1)`, body.UserID,
	).Scan(&userExists); err != nil {
		handleErr(w, r, err)
		return
	}
	if !userExists {
		writeError(w, http.StatusBadRequest, "user not found")
		return
	}

	var m CollectionMember
	var id, colID, userID pgtype.UUID
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO _meta.collection_members (collection_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id, collection_id, user_id, role, created_at`,
		collectionID, body.UserID, body.Role,
	).Scan(&id, &colID, &userID, &m.Role, &m.CreatedAt)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	m.ID = pgutil.UUIDToString(id)
	m.CollectionID = pgutil.UUIDToString(colID)
	m.UserID = pgutil.UUIDToString(userID)
	writeJSON(w, http.StatusCreated, m)
}

// Update changes a member's role.
func (h *MemberHandler) Update(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	user, _ := middleware.GetUser(r.Context())
	if user.Role != "director" {
		apierr.Forbidden("only directors can manage members").Write(w)
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Role != "owner" && body.Role != "editor" && body.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be owner, editor, or viewer")
		return
	}

	tag, err := h.pool.Exec(r.Context(), `
		UPDATE _meta.collection_members SET role = $1
		WHERE collection_id = $2 AND user_id = $3`,
		body.Role, collectionID, targetUserID,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Remove deletes a member from a collection.
func (h *MemberHandler) Remove(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	user, _ := middleware.GetUser(r.Context())
	if user.Role != "director" {
		apierr.Forbidden("only directors can manage members").Write(w)
		return
	}

	tag, err := h.pool.Exec(r.Context(), `
		DELETE FROM _meta.collection_members
		WHERE collection_id = $1 AND user_id = $2`,
		collectionID, targetUserID,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
