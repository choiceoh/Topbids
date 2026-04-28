// Package notify defines the notification sending interface.
//
// Messenger-specific adapters (Slack, Teams, Kakao Work, etc.) are
// implemented separately once API specs are received. The core
// application only depends on the Notifier interface.
package notify

import "context"

// Message holds a notification payload.
type Message struct {
	// Title is the notification title/subject (optional for some channels).
	Title string `json:"title,omitempty"`
	// Body is the main message content (required).
	Body string `json:"body"`
	// URL is an optional deep-link into the Topbid UI.
	URL string `json:"url,omitempty"`
	// Metadata holds channel-specific extra data (e.g. button configs).
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Notifier is the interface that notification adapters must implement.
type Notifier interface {
	// Name returns the adapter name for logging (e.g. "slack", "email").
	Name() string

	// Send delivers a message to the specified user.
	// userID is the Topbid user UUID; the adapter resolves the external
	// address (email, Slack member ID, etc.) internally.
	Send(ctx context.Context, userID string, msg Message) error

	// SendBulk delivers a message to multiple users.
	// Implementations may batch API calls for efficiency.
	SendBulk(ctx context.Context, userIDs []string, msg Message) error
}

// Dispatcher fans out notifications to all registered adapters.
type Dispatcher struct {
	adapters []Notifier
}

// NewDispatcher creates a dispatcher with the given adapters.
func NewDispatcher(adapters ...Notifier) *Dispatcher {
	return &Dispatcher{adapters: adapters}
}

// Send delivers a message via all registered adapters.
// Errors from individual adapters are logged but do not block others.
func (d *Dispatcher) Send(ctx context.Context, userID string, msg Message) error {
	var firstErr error
	for _, a := range d.adapters {
		if err := a.Send(ctx, userID, msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// SendBulk delivers a message to multiple users via all adapters.
func (d *Dispatcher) SendBulk(ctx context.Context, userIDs []string, msg Message) error {
	var firstErr error
	for _, a := range d.adapters {
		if err := a.SendBulk(ctx, userIDs, msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// NoopNotifier is a no-op implementation for development/testing.
type NoopNotifier struct{}

func (NoopNotifier) Name() string                                            { return "noop" }
func (NoopNotifier) Send(_ context.Context, _ string, _ Message) error       { return nil }
func (NoopNotifier) SendBulk(_ context.Context, _ []string, _ Message) error { return nil }
