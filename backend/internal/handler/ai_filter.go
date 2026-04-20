package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type aiFilterRequest struct {
	Query string `json:"query"`
}

var filterSystemPrompt = fmt.Sprintf(`You are a filter builder for Topworks, a no-code business app platform used by a Korean company.
The user will describe what data they want to see in natural language (usually Korean).
You must generate filter conditions as a JSON array.

## Tools
ALWAYS call get_collection_fields first to know the exact field slugs and types.

## Today's Date
%s

## Output Format
Return a JSON array of filter conditions:
[
  { "field": "slug_name", "operator": "eq", "value": "some_value" }
]

## Supported Operators
- eq: equals
- neq: not equals
- gt: greater than
- gte: greater than or equal
- lt: less than
- lte: less than or equal
- like: contains (partial text match)
- in: one of (comma-separated values)
- is_null: field is empty (value is ignored)

## Rules
- Use ONLY exact field slugs from get_collection_fields.
- For date fields, use ISO format: "2026-04-01"
- For relative dates, compute the actual date. Today is shown above.
  - "이번 주" → compute Monday to Sunday of this week
  - "이번 달" → first and last day of current month
  - "지난주" → previous week range
- For date ranges, use two conditions (gte + lte)
- For select fields, use exact option values from the choices list
- Output ONLY the JSON array, no explanation

## Examples
User: "이번 달 완료된 건"
→ [{"field":"status","operator":"eq","value":"완료"},{"field":"created_at","operator":"gte","value":"2026-04-01"},{"field":"created_at","operator":"lte","value":"2026-04-30"}]

User: "김영수가 등록한 대기 건"
→ [{"field":"_created_by_name","operator":"like","value":"김영수"},{"field":"status","operator":"eq","value":"대기"}]
`, time.Now().Format("2006-01-02"))

// BuildFilter generates filter conditions from a natural language query.
func (h *AIHandler) BuildFilter(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionSlug := chi.URLParam(r, "slug")

	var req aiFilterRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	ctx := r.Context()
	resolve := h.newToolResolver(ctx)

	userMsg := req.Query
	if col, err := h.store.GetCollectionBySlug(ctx, collectionSlug); err == nil {
		userMsg = "대상 앱: " + col.Label + " (slug: " + col.Slug + ")\n\n" + req.Query
	}

	raw, err := h.client.CompleteWithTools(ctx, filterSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai build-filter failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	// Extract and validate JSON array.
	jsonStr := extractJSONArray(raw)
	var result json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Error("ai filter returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// extractJSONArray finds the outermost [...] in a string.
func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown fences.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)

	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
