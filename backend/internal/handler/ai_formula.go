package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type aiFormulaRequest struct {
	Description string `json:"description"`
}

const formulaSystemPrompt = `You are a formula assistant for Phaeton, a no-code business app platform used by a Korean company.
The user will describe a formula in natural language (usually in Korean). You must generate a valid formula expression.

## Tools
You have tools to look up the target collection's fields. ALWAYS call get_collection_fields first
to know the exact field slugs before generating the formula.
- get_collection_fields: 특정 앱의 필드 상세 조회 (slug 필요)

## Supported Syntax
- Arithmetic: + - * / % and parentheses ()
- Functions: SUM, AVG, MIN, MAX, COUNT, ROUND, ABS, COALESCE, CEIL, FLOOR
- Conditions: IF(condition, true_value, false_value)
- Comparisons: = != < > <= >=
- Logical: AND, OR, NOT
- String concatenation (for text result): field1 || ' ' || field2
- Cross-collection references: LOOKUP(relation_field, target_field)
- Cross-collection aggregation: SUMREL(relation_field, target_field), AVGREL, COUNTREL, MINREL, MAXREL

## Rules
- Use ONLY the exact field slugs from the collection. Do NOT invent slug names.
- Output ONLY the formula expression string, nothing else. No explanation, no markdown.
- Field slugs are used directly without quotes: price * quantity
- String literals use single quotes: IF(status = '완료', total, 0)
- For cross-collection lookups, the relation_field is the slug of the relation-type field in this collection.

## Examples
- "단가 곱하기 수량" → unit_price * quantity
- "총액에서 할인금액 빼기" → total_amount - discount
- "상태가 완료이면 금액, 아니면 0" → IF(status = '완료', amount, 0)
- "수량 합계를 100으로 나누고 소수점 2자리 반올림" → ROUND(quantity / 100, 2)
`

// BuildFormula generates a formula expression from a natural language description.
// The {slug} URL param is the collection slug (consistent with the formula-preview endpoint).
func (h *AIHandler) BuildFormula(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionSlug := chi.URLParam(r, "slug")

	var req aiFormulaRequest
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

	// Resolve collection slug → label for context.
	userMsg := req.Description
	if col, err := h.store.GetCollectionBySlug(ctx, collectionSlug); err == nil {
		userMsg = "대상 앱: " + col.Label + " (slug: " + col.Slug + ")\n\n" + req.Description
	}

	raw, err := h.client.CompleteWithTools(ctx, formulaSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai build-formula failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	// The AI should return just the expression string. Clean it up.
	expression := strings.TrimSpace(raw)
	// Strip markdown code fences if present.
	expression = strings.TrimPrefix(expression, "```")
	expression = strings.TrimSuffix(expression, "```")
	expression = strings.TrimSpace(expression)

	result := map[string]string{"expression": expression}
	b, _ := json.Marshal(result)
	writeJSON(w, http.StatusOK, json.RawMessage(b))
}
