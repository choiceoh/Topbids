package handler

import (
	"net/http"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/ai"
)

// newTestAIClient creates an ai.Client pointing to a test server URL.
// This is used in tests to point the AI client at a mock vLLM server.
func newTestAIClient(baseURL string) *ai.Client {
	return ai.NewClientWith(baseURL, "test-model", &http.Client{Timeout: 5 * time.Second})
}
