package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

// WebhookEvent is the envelope stored for each received webhook.
type WebhookEvent struct {
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	Source     string          `json:"source,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	Processed  bool            `json:"processed"`
	ReceivedAt time.Time       `json:"received_at"`
}

// TopicHandler processes a webhook payload for a specific topic.
type TopicHandler func(payload json.RawMessage) error

// WebhookHandler provides a generic webhook receiver with DB persistence.
type WebhookHandler struct {
	pool     *pgxpool.Pool
	secret   string
	handlers map[string]TopicHandler
}

// NewWebhookHandler creates a handler with optional HMAC-SHA256 verification.
// Set WEBHOOK_SECRET env var to enable signature validation.
func NewWebhookHandler(pool *pgxpool.Pool, secret string) *WebhookHandler {
	return &WebhookHandler{
		pool:     pool,
		secret:   secret,
		handlers: make(map[string]TopicHandler),
	}
}

// Handle registers a handler for a specific topic (URL path segment).
func (h *WebhookHandler) Handle(topic string, fn TopicHandler) {
	h.handlers[topic] = fn
}

// Receive handles POST /api/hooks/{topic}.
func (h *WebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	topic := chi.URLParam(r, "topic")
	if topic == "" {
		apierr.BadRequest("topic is required").Write(w)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	defer r.Body.Close()
	if err != nil {
		apierr.BadRequest("failed to read body").Write(w)
		return
	}

	// Verify HMAC signature if secret is configured.
	if h.secret != "" {
		sig := r.Header.Get("X-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Hub-Signature-256")
		}
		if !verifyHMAC([]byte(h.secret), body, sig) {
			apierr.Unauthorized("invalid signature").Write(w)
			return
		}
	}

	source := r.Header.Get("X-Webhook-Source")
	slog.Info("webhook received",
		"topic", topic,
		"source", source,
		"size", len(body),
	)

	// Persist event.
	var evt WebhookEvent
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO _meta.webhook_events (topic, source, payload)
		 VALUES ($1, $2, $3)
		 RETURNING id, topic, source, payload, processed, received_at`,
		topic, source, json.RawMessage(body),
	).Scan(&evt.ID, &evt.Topic, &evt.Source, &evt.Payload, &evt.Processed, &evt.ReceivedAt)
	if err != nil {
		slog.Error("webhook: failed to store event", "error", err)
		apierr.Internal("failed to store webhook event").Write(w)
		return
	}

	// Dispatch to topic handler if registered.
	fn, ok := h.handlers[topic]
	if !ok {
		slog.Warn("webhook: no handler for topic", "topic", topic)
		writeJSON(w, http.StatusAccepted, evt)
		return
	}

	if err := fn(json.RawMessage(body)); err != nil {
		slog.Error("webhook: handler failed", "topic", topic, "error", err)
		// Mark as processed with error message.
		if _, execErr := h.pool.Exec(r.Context(),
			`UPDATE _meta.webhook_events SET processed = TRUE, error_message = $2 WHERE id = $1`,
			evt.ID, err.Error()); execErr != nil {
			slog.Error("webhook: failed to mark event as processed", "id", evt.ID, "error", execErr)
		}
		apierr.Internal("webhook processing failed").Write(w)
		return
	}

	// Mark as successfully processed.
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE _meta.webhook_events SET processed = TRUE WHERE id = $1`, evt.ID); err != nil {
		slog.Error("webhook: failed to mark event as processed", "id", evt.ID, "error", err)
	}
	evt.Processed = true

	writeJSON(w, http.StatusOK, evt)
}

// List returns paginated webhook events (GET /api/webhooks).
func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	page, limit, offset := ParsePagination(r.URL.Query())

	topicFilter := r.URL.Query().Get("topic")

	// Count.
	var total int64
	countQ := `SELECT COUNT(*) FROM _meta.webhook_events`
	args := []any{}
	if topicFilter != "" {
		countQ += ` WHERE topic = $1`
		args = append(args, topicFilter)
	}
	if err := h.pool.QueryRow(r.Context(), countQ, args...).Scan(&total); err != nil {
		slog.Error("webhook: count failed", "error", err)
		apierr.Internal("failed to list webhooks").Write(w)
		return
	}

	// Fetch.
	listQ := `SELECT id, topic, source, payload, processed, received_at
		FROM _meta.webhook_events`
	listArgs := []any{}
	if topicFilter != "" {
		listQ += ` WHERE topic = $1`
		listArgs = append(listArgs, topicFilter)
		listQ += ` ORDER BY received_at DESC LIMIT $2 OFFSET $3`
		listArgs = append(listArgs, limit, offset)
	} else {
		listQ += ` ORDER BY received_at DESC LIMIT $1 OFFSET $2`
		listArgs = append(listArgs, limit, offset)
	}

	rows, err := h.pool.Query(r.Context(), listQ, listArgs...)
	if err != nil {
		slog.Error("webhook: list query failed", "error", err)
		apierr.Internal("failed to list webhooks").Write(w)
		return
	}
	defer rows.Close()

	events := []WebhookEvent{}
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.Topic, &e.Source, &e.Payload, &e.Processed, &e.ReceivedAt); err != nil {
			slog.Error("webhook: scan failed", "error", err)
			apierr.Internal("failed to list webhooks").Write(w)
			return
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		slog.Error("webhook: rows iteration failed", "error", err)
		apierr.Internal("failed to list webhooks").Write(w)
		return
	}

	writeList(w, events, total, page, limit)
}

// Get returns a single webhook event (GET /api/webhooks/{id}).
func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e WebhookEvent
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, topic, source, payload, processed, received_at
		 FROM _meta.webhook_events WHERE id = $1`, id,
	).Scan(&e.ID, &e.Topic, &e.Source, &e.Payload, &e.Processed, &e.ReceivedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			apierr.NotFound("webhook event not found").Write(w)
			return
		}
		slog.Error("webhook: get failed", "error", err)
		apierr.Internal("failed to get webhook").Write(w)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Delete removes a single webhook event (DELETE /api/webhooks/{id}).
func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM _meta.webhook_events WHERE id = $1`, id)
	if err != nil {
		slog.Error("webhook: delete failed", "error", err)
		apierr.Internal("failed to delete webhook").Write(w)
		return
	}
	if tag.RowsAffected() == 0 {
		apierr.NotFound("webhook event not found").Write(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// verifyHMAC checks the HMAC-SHA256 signature.
// Expected format: "sha256=<hex>".
func verifyHMAC(secret, payload []byte, signature string) bool {
	if len(signature) < 8 || signature[:7] != "sha256=" {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), sig)
}
