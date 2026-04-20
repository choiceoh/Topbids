package handler

import (
	"fmt"
	"net/http"

	"github.com/choiceoh/phaeton/backend/internal/events"
)

// SSEHandler serves Server-Sent Events for real-time updates.
type SSEHandler struct {
	broker *events.Broker
}

func NewSSEHandler(broker *events.Broker) *SSEHandler {
	return &SSEHandler{broker: broker}
}

// Stream handles GET /api/events — an SSE endpoint.
func (h *SSEHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := h.broker.Subscribe()
	defer unsub()

	// Send initial connection event.
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
