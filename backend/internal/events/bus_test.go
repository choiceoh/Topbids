package events

import (
	"context"
	"sync"
	"testing"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := NewBus()
	var got Event

	bus.Subscribe(func(_ context.Context, ev Event) {
		got = ev
	})

	ev := Event{
		Type:         EventRecordCreate,
		CollectionID: "col-1",
		RecordID:     "rec-1",
		ActorUserID:  "user-1",
	}
	bus.Publish(context.Background(), ev)

	if got.Type != EventRecordCreate {
		t.Errorf("got type %q, want %q", got.Type, EventRecordCreate)
	}
	if got.CollectionID != "col-1" {
		t.Errorf("got collection_id %q, want %q", got.CollectionID, "col-1")
	}
}

func TestBus_MultipleHandlers(t *testing.T) {
	bus := NewBus()
	var count int

	for i := 0; i < 3; i++ {
		bus.Subscribe(func(_ context.Context, _ Event) {
			count++
		})
	}

	bus.Publish(context.Background(), Event{Type: EventComment})

	if count != 3 {
		t.Errorf("got %d handler calls, want 3", count)
	}
}

func TestBus_NoHandlers(t *testing.T) {
	bus := NewBus()
	// Should not panic with no handlers.
	bus.Publish(context.Background(), Event{Type: EventRecordDelete})
}

func TestBus_ConcurrentSubscribePublish(t *testing.T) {
	bus := NewBus()
	var mu sync.Mutex
	var count int

	var wg sync.WaitGroup
	// Subscribe concurrently.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Subscribe(func(_ context.Context, _ Event) {
				mu.Lock()
				count++
				mu.Unlock()
			})
		}()
	}
	wg.Wait()

	// Publish and verify all handlers fire.
	bus.Publish(context.Background(), Event{Type: EventStateChange})

	mu.Lock()
	defer mu.Unlock()
	if count != 10 {
		t.Errorf("got %d handler calls, want 10", count)
	}
}

func TestEventTypeConstants(t *testing.T) {
	types := []EventType{
		EventComment, EventStateChange,
		EventRecordCreate, EventRecordUpdate, EventRecordDelete,
	}
	seen := make(map[EventType]bool, len(types))
	for _, et := range types {
		if et == "" {
			t.Error("event type should not be empty")
		}
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}
