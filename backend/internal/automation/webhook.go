package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/infra/httpretry"
	"github.com/choiceoh/phaeton/backend/pkg/httputil"
)

// WebhookSender sends outbound HTTP webhook calls.
type WebhookSender struct {
	client *http.Client
}

// NewWebhookSender creates a sender with a 10-second timeout.
func NewWebhookSender() *WebhookSender {
	return &WebhookSender{
		client: httputil.NewClient(10 * time.Second),
	}
}

// WebhookPayload is the JSON body sent to webhook URLs.
type WebhookPayload struct {
	CollectionID string         `json:"collection_id"`
	RecordID     string         `json:"record_id"`
	TriggerType  string         `json:"trigger_type"`
	Record       map[string]any `json:"record,omitempty"`
}

// Send posts a JSON payload to the webhook URL with optional headers.
// Retries up to 3 times on transient failures.
func (s *WebhookSender) Send(ctx context.Context, cfg WebhookConfig, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	backoff := httpretry.Backoff{
		Base: 500 * time.Millisecond,
		Max:  5 * time.Second,
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("webhook call failed", "url", cfg.URL, "attempt", attempt, "error", err)
			time.Sleep(backoff.Delay(attempt))
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode < 400 {
			return nil
		}
		if !httpretry.IsRetryable(resp.StatusCode) {
			return fmt.Errorf("webhook returned %d", resp.StatusCode)
		}
		lastErr = fmt.Errorf("webhook returned %d", resp.StatusCode)
		slog.Warn("webhook retryable failure", "url", cfg.URL, "status", resp.StatusCode, "attempt", attempt)
		time.Sleep(backoff.Delay(attempt))
	}
	return fmt.Errorf("webhook failed after 3 attempts: %w", lastErr)
}
