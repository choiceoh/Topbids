package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/ai"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// AIHandler serves AI-powered endpoints.
// The no-code app-builder AI features (BuildCollection, GenerateSlug, build-formula,
// build-chart, build-automation, prefill, map-csv-columns, build-filter) were removed
// in the Topbids refactor. What remains is the site-usage chat (see ai_chat.go).
type AIHandler struct {
	client *ai.Client
	store  *schema.Store
	pool   *pgxpool.Pool
	cache  *schema.Cache
}

func NewAIHandler(client *ai.Client, store *schema.Store, pool *pgxpool.Pool, cache *schema.Cache) *AIHandler {
	return &AIHandler{client: client, store: store, pool: pool, cache: cache}
}

// HealthCheck returns whether the vLLM backend is reachable.
func (h *AIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ok := h.client.Healthy(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if ok {
		w.Write([]byte(`{"available":true}`))
	} else {
		w.Write([]byte(`{"available":false}`))
	}
}
