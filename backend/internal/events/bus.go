// Package events provides an in-process event bus and SSE broker.
//
// The Bus is a synchronous pub/sub: Publish blocks until all handlers return.
// Handlers should dispatch heavy work to goroutines or worker pools.
//
// The Broker fans out events to SSE clients (EventSource in browsers).
// Each client gets a buffered channel (64 messages). If a client falls behind,
// messages are dropped (non-blocking send) to prevent slow clients from
// affecting the event loop.
package events

import (
	"context"
	"sync"
)

// EventType identifies the kind of event.
type EventType string

const (
	EventComment      EventType = "comment"
	EventStateChange  EventType = "state_change"
	EventRecordCreate EventType = "record_created"
	EventRecordUpdate EventType = "record_updated"
	EventRecordDelete EventType = "record_deleted"
	EventFormSubmit   EventType = "form_submit"
)

// Event is published when something notable happens.
type Event struct {
	Type           EventType
	CollectionID   string
	CollectionSlug string
	RecordID       string
	ActorUserID    string
	ActorName      string
	Title          string
	Body           string

	// Record data for automation evaluation.
	OldRecord  map[string]any // nil for create
	NewRecord  map[string]any // nil for delete
	StatusFrom string         // previous _status (for status_change)
	StatusTo   string         // new _status (for status_change)
	FormSlug   string         // form slug (for form_submit)
}

// Handler processes an event.
type Handler func(ctx context.Context, ev Event)

// Bus is a simple synchronous in-process pub/sub.
type Bus struct {
	mu       sync.RWMutex
	handlers []Handler
}

func NewBus() *Bus {
	return &Bus{}
}

// Subscribe registers a handler.
func (b *Bus) Subscribe(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish sends an event to all handlers synchronously.
func (b *Bus) Publish(ctx context.Context, ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, h := range b.handlers {
		h(ctx, ev)
	}
}
