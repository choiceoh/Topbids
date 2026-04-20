package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type aiCSVMapRequest struct {
	Headers []string `json:"headers"`
}

const csvMapSystemPrompt = `You are a CSV column mapper for Topworks, a no-code business app platform.
Given CSV column headers and a collection's field definitions, map each CSV header to the best matching field slug.

## Tools
ALWAYS call get_collection_fields first to know the exact field slugs and labels.

## Output Format
Return a JSON object mapping CSV headers to field slugs:
{
  "프로젝트명": "project_name",
  "담당": "assignee",
  "시작날짜": "start_date",
  "금액(원)": "amount"
}

## Rules
- Map each CSV header to the most semantically similar field slug.
- If a header clearly doesn't match any field, exclude it from the result.
- Consider Korean synonyms and abbreviations (e.g., "담당" → "assignee", "프로젝트명" → "project_name").
- Header matching should be case-insensitive for English.
- Output ONLY the JSON object, no explanation.
`

// MapCSVColumns maps CSV headers to collection field slugs using AI.
func (h *AIHandler) MapCSVColumns(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionSlug := chi.URLParam(r, "slug")

	var req aiCSVMapRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Headers) == 0 {
		writeError(w, http.StatusBadRequest, "headers is required")
		return
	}

	ctx := r.Context()
	resolve := h.newToolResolver(ctx)

	userMsg := "대상 앱 slug: " + collectionSlug + "\n\nCSV 헤더:\n"
	for _, h := range req.Headers {
		userMsg += "- " + h + "\n"
	}

	raw, err := h.client.CompleteWithTools(ctx, csvMapSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai map-csv-columns failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	jsonStr := extractJSON(raw)
	var result map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Error("ai csv map returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	// Validate: only keep mappings to slugs that actually exist.
	col, err := h.store.GetCollectionBySlug(ctx, collectionSlug)
	if err == nil {
		validSlugs := make(map[string]bool, len(col.Fields))
		for _, f := range col.Fields {
			validSlugs[f.Slug] = true
		}
		for header, slug := range result {
			slug = strings.TrimSpace(slug)
			if !validSlugs[slug] {
				delete(result, header)
			} else {
				result[header] = slug
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}
