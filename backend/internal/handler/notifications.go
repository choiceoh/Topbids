package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/pgutil"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// Notification represents a user notification.
type Notification struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	Body            string    `json:"body,omitempty"`
	RefCollectionID string    `json:"ref_collection_id,omitempty"`
	RefRecordID     string    `json:"ref_record_id,omitempty"`
	IsRead          bool      `json:"is_read"`
	CreatedAt       time.Time `json:"created_at"`
}

// NotificationHandler serves notification CRUD.
type NotificationHandler struct {
	pool *pgxpool.Pool
}

func NewNotificationHandler(pool *pgxpool.Pool) *NotificationHandler {
	return &NotificationHandler{pool: pool}
}

// List returns the current user's notifications.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUser(r.Context())
	page, limit, offset := ParsePagination(r.URL.Query())

	var total int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM _meta.notifications WHERE user_id = $1`, user.UserID,
	).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "count notifications: "+err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, user_id, type, title, body, ref_collection_id, ref_record_id, is_read, created_at
		FROM _meta.notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		user.UserID, limit, offset,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	notifs, err := scanNotifications(rows)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeList(w, notifs, total, page, limit)
}

// UnreadCount returns the count of unread notifications for the badge.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUser(r.Context())
	var count int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM _meta.notifications WHERE user_id = $1 AND is_read = FALSE`, user.UserID,
	).Scan(&count); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}

// MarkRead marks a single notification as read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	notifID := chi.URLParam(r, "id")
	user, _ := middleware.GetUser(r.Context())
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE _meta.notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2`,
		notifID, user.UserID,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// MarkAllRead marks all notifications as read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUser(r.Context())
	_, err := h.pool.Exec(r.Context(),
		`UPDATE _meta.notifications SET is_read = TRUE WHERE user_id = $1 AND is_read = FALSE`,
		user.UserID,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "all_read"})
}

func scanNotifications(rows pgx.Rows) ([]Notification, error) {
	var out []Notification
	for rows.Next() {
		var n Notification
		var id, userID, refColID, refRecID pgtype.UUID
		var body *string
		err := rows.Scan(&id, &userID, &n.Type, &n.Title, &body, &refColID, &refRecID, &n.IsRead, &n.CreatedAt)
		if err != nil {
			return nil, err
		}
		n.ID = pgutil.UUIDToString(id)
		n.UserID = pgutil.UUIDToString(userID)
		n.RefCollectionID = pgutil.UUIDToString(refColID)
		n.RefRecordID = pgutil.UUIDToString(refRecID)
		if body != nil {
			n.Body = *body
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// --- Notification Subscriber ---

// SubscribeNotifications registers a handler on the event bus that creates
// notifications for relevant users when events are published.
func SubscribeNotifications(pool *pgxpool.Pool, bus *events.Bus, cache *schema.Cache) {
	bus.Subscribe(func(ctx context.Context, ev events.Event) {
		switch ev.Type {
		case events.EventComment:
			notifyCommentRecipients(ctx, pool, ev)
		case events.EventStateChange:
			notifyStateChangeRecipients(ctx, pool, cache, ev)
		}
	})
}

// notifyCommentRecipients sends notifications to the record creator and other
// commenters (excluding the actor).
func notifyCommentRecipients(ctx context.Context, pool *pgxpool.Pool, ev events.Event) {
	// Look up the collection slug so we can query the data table for the record creator.
	var slug string
	if err := pool.QueryRow(ctx,
		`SELECT slug FROM _meta.collections WHERE id = $1`, ev.CollectionID,
	).Scan(&slug); err != nil {
		slog.Warn("notify comment recipients: collection lookup failed", "error", err)
		slug = "" // proceed without record creator
	}

	var rows pgx.Rows
	var err error
	if slug != "" {
		qTable := pgutil.QuoteQualified("data", slug)
		rows, err = pool.Query(ctx, fmt.Sprintf(`
			SELECT DISTINCT user_id::text FROM (
				SELECT _created_by AS user_id FROM %s WHERE id = $1 AND deleted_at IS NULL
				UNION
				SELECT user_id FROM _meta.comments WHERE collection_id = $2 AND record_id = $3
			) sub
			WHERE user_id IS NOT NULL AND user_id::text != $4
			LIMIT 50`, qTable),
			ev.RecordID, ev.CollectionID, ev.RecordID, ev.ActorUserID,
		)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT DISTINCT user_id::text FROM _meta.comments
			WHERE collection_id = $1 AND record_id = $2 AND user_id::text != $3
			LIMIT 50`,
			ev.CollectionID, ev.RecordID, ev.ActorUserID,
		)
	}
	if err != nil {
		slog.Error("notify comment recipients query", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var recipientID string
		if err := rows.Scan(&recipientID); err != nil {
			continue
		}
		insertNotification(ctx, pool, recipientID, string(ev.Type), ev.Title, ev.Body, ev.CollectionID, ev.RecordID)
	}
}

// notifyStateChangeRecipients sends notifications for status changes:
// 1. Record creator is notified about the status change.
// 2. Designated approvers (allowed_user_ids on outgoing transitions from the new status) are notified.
func notifyStateChangeRecipients(ctx context.Context, pool *pgxpool.Pool, cache *schema.Cache, ev events.Event) {
	title := fmt.Sprintf("상태 변경: %s → %s", ev.StatusFrom, ev.StatusTo)
	body := fmt.Sprintf("항목이 '%s' 단계로 이동했습니다.", ev.StatusTo)

	// 1. Notify record creator.
	var slug string
	err := pool.QueryRow(ctx,
		`SELECT slug FROM _meta.collections WHERE id = $1`, ev.CollectionID,
	).Scan(&slug)
	if err == nil {
		var creatorID string
		qTable := pgutil.QuoteQualified("data", slug)
		err = pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT _created_by::text FROM %s WHERE id = $1 AND deleted_at IS NULL`, qTable),
			ev.RecordID,
		).Scan(&creatorID)
		if err == nil && creatorID != "" && creatorID != ev.ActorUserID {
			insertNotification(ctx, pool, creatorID, "state_change", title, body, ev.CollectionID, ev.RecordID)
		}
	}

	// 2. Notify designated approvers for the next stage.
	proc, ok := cache.ProcessByCollectionID(ev.CollectionID)
	if !ok || !proc.IsEnabled {
		return
	}

	// Find the status ID for the new status.
	var toStatusID string
	for _, s := range proc.Statuses {
		if s.Name == ev.StatusTo {
			toStatusID = s.ID
			break
		}
	}
	if toStatusID == "" {
		return
	}

	// Collect user IDs from outgoing transitions of the new status.
	notified := make(map[string]bool)
	notified[ev.ActorUserID] = true // Don't notify the actor.
	approverBody := fmt.Sprintf("항목이 '%s' 단계로 이동했습니다. 다음 단계 전환을 처리해주세요.", ev.StatusTo)
	for _, t := range proc.Transitions {
		if t.FromStatusID != toStatusID {
			continue
		}
		for _, uid := range t.AllowedUserIDs {
			if notified[uid] {
				continue
			}
			notified[uid] = true
			insertNotification(ctx, pool, uid, "state_change", title, approverBody, ev.CollectionID, ev.RecordID)
		}
	}
}

func insertNotification(ctx context.Context, pool *pgxpool.Pool, userID, ntype, title, body, refCollectionID, refRecordID string) {
	_, _ = pool.Exec(ctx, `
		INSERT INTO _meta.notifications (user_id, type, title, body, ref_collection_id, ref_record_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, ntype, title, body, refCollectionID, refRecordID,
	)
}
