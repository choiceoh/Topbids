package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type aiChartRequest struct {
	Description string `json:"description"`
}

const chartSystemPrompt = `You are a chart configurator for Phaeton, a no-code business app platform used by a Korean company.
The user will describe a chart they want to create. You must generate a chart configuration as JSON.

## Tools
ALWAYS call get_collection_fields first to know the exact field slugs and types.

## Output Format
Return a JSON object with these fields:
{
  "name": "차트 이름",
  "chart_type": "bar",
  "config": {
    "group_field": "status",
    "value_field": "amount",
    "aggregation": "sum"
  }
}

## Chart Types
- "bar" — 막대 차트 (default, best for category comparisons)
- "line" — 선 차트 (best for time series)
- "pie" — 원형 차트 (best for proportions)
- "doughnut" — 도넛 차트 (alternative to pie)
- "area" — 영역 차트 (similar to line with filled area)

## Config Fields
- group_field: the field slug to group by (usually a select, user, or date field)
- value_field: the field slug to aggregate (a numeric field, or omit for count)
- aggregation: "count", "sum", "avg", "min", "max" (default: "count")
- filter_field: (optional) field slug to filter by
- filter_value: (optional) value to filter on

## Rules
- Use ONLY exact field slugs from get_collection_fields.
- Choose chart_type based on the data description.
- If the user mentions "비율" or "구성" or "분포", use "pie" or "doughnut".
- If the user mentions "추이" or "시계열" or "변화", use "line".
- Default to "bar" for general comparisons.
- For count-based charts, omit value_field and use aggregation: "count".
- Output ONLY the JSON object, no explanation.
`

// BuildChart generates a chart configuration from a natural language description.
func (h *AIHandler) BuildChart(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionID := chi.URLParam(r, "id")

	var req aiChartRequest
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
	if cols, err := h.store.ListCollections(ctx); err == nil {
		for _, c := range cols {
			if c.ID == collectionID {
				userMsg = "대상 앱: " + c.Label + " (slug: " + c.Slug + ")\n\n" + req.Description
				break
			}
		}
	}

	raw, err := h.client.CompleteWithTools(ctx, chartSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai build-chart failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	jsonStr := extractJSON(raw)
	var result json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Error("ai chart returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
