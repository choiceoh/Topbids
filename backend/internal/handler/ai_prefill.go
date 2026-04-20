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

type aiPrefillRequest struct {
	Description string `json:"description"`
}

var prefillSystemPrompt = fmt.Sprintf(`You are a data entry assistant for Topworks, a no-code business app platform used by a Korean company.
The user will describe a record in one sentence (usually Korean).
You must extract field values and return them as a JSON object.

## Tools
ALWAYS call get_collection_fields first to know the exact field slugs and types.
If you need to resolve a person's name to a user ID, call list_users.

## Today's Date
%s

## Output Format
Return a JSON object where keys are field slugs and values are the extracted data:
{
  "title": "보고서 제출 요청",
  "assignee": "uuid-of-user",
  "deadline": "2026-04-11",
  "status": "대기"
}

## Rules
- Use ONLY exact field slugs from get_collection_fields.
- For date fields, use ISO format: "2026-04-11"
- For relative dates ("내일", "다음주 월요일"), compute the actual date from today.
- For user fields, resolve the person's name to their user ID using list_users.
- For select fields, use exact option values from the choices list.
- For boolean fields, use true or false.
- ONLY include fields you are confident about. Leave uncertain fields out.
- Output ONLY the JSON object, no explanation.
- Do NOT include system fields (id, created_at, updated_at, created_by).
`, time.Now().Format("2006-01-02"))

// Prefill generates field values from a natural language description of a record.
func (h *AIHandler) Prefill(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionSlug := chi.URLParam(r, "slug")

	var req aiPrefillRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}

	ctx := r.Context()
	resolve := h.newToolResolver(ctx)

	userMsg := req.Description
	if col, err := h.store.GetCollectionBySlug(ctx, collectionSlug); err == nil {
		userMsg = "대상 앱: " + col.Label + " (slug: " + col.Slug + ")\n\n" + req.Description
	}

	raw, err := h.client.CompleteWithTools(ctx, prefillSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai prefill failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	jsonStr := extractJSON(raw)
	var result json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Error("ai prefill returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
