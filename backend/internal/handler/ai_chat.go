package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/choiceoh/topbid/backend/internal/ai"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
)

type aiChatRequest struct {
	Message string           `json:"message"`
	History []ai.ChatMessage `json:"history,omitempty"`
	Images  []string         `json:"images,omitempty"` // image data-URLs for current message
}

type aiChatResponse struct {
	Reply string `json:"reply"`
}

const chatSystemPrompt = `You are a helpful assistant for Topbid, a no-code business app platform used by a Korean company.
Your role is to answer user questions about the platform and help them use it effectively.

` + chatGuide + `

## Tools
You have tools to look up workspace data. Use them when the user asks about specific apps, fields, or data:
- list_collections: 워크스페이스의 앱 목록 조회
- get_collection_fields: 특정 앱의 항목 상세 조회 (slug 필요 → list_collections 먼저 호출)
- query_data: 특정 앱의 데이터를 조회 (slug, limit, filter 지정 가능)

Do NOT guess app names or fields — always use the tools to look up real data.

## Response Guidelines
- Always respond in Korean.
- Be concise and practical. Answer in 2-5 sentences for simple questions.
- If asked how to do something, give numbered step-by-step instructions.
- If asked about a feature that doesn't exist, say so honestly and suggest alternatives.
- Do NOT generate JSON schemas in chat — direct users to "AI로 만들기" for that.
- Use plain text formatting. Bold important terms with ** for emphasis.
- When referring to UI elements, use the Korean labels as they appear in the app.`

// Chat handles conversational Q&A about the platform.
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	var req aiChatRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	ctx := r.Context()
	resolve := h.newChatToolResolver(ctx)

	reply, err := h.client.CompleteWithTools(ctx, chatSystemPrompt, req.History, req.Message, chatTools, resolve, req.Images...)
	if err != nil {
		slog.Error("ai chat failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	writeJSON(w, http.StatusOK, aiChatResponse{Reply: strings.TrimSpace(reply)})
}

// chatTools extends workspaceTools with data querying.
var chatTools = append(append([]ai.Tool{}, workspaceTools...), ai.Tool{
	Type: "function",
	Function: ai.ToolFunction{
		Name:        "query_data",
		Description: "특정 앱의 데이터를 조회합니다. 최근 데이터를 가져오며 필터 조건을 지정할 수 있습니다.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"slug": {"type": "string", "description": "앱의 영문 ID (slug)"},
				"limit": {"type": "integer", "description": "조회할 건수 (기본 5, 최대 20)"},
				"filter": {"type": "string", "description": "필터 조건 (예: status=eq:완료,priority=gt:3). 연산자: eq,neq,gt,gte,lt,lte,like,is_null"}
			},
			"required": ["slug"]
		}`),
	},
})

// newChatToolResolver creates a resolver that handles workspace tools + data queries.
func (h *AIHandler) newChatToolResolver(ctx context.Context) ai.ToolResolver {
	base := h.newToolResolver(ctx)
	return func(name, arguments string) (string, error) {
		if name == "query_data" {
			var args struct {
				Slug   string `json:"slug"`
				Limit  int    `json:"limit"`
				Filter string `json:"filter"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", fmt.Errorf("parse arguments: %w", err)
			}
			return h.queryAppData(ctx, args.Slug, args.Limit, args.Filter)
		}
		return base(name, arguments)
	}
}

// queryAppData fetches recent entries from a dynamic table for AI context.
func (h *AIHandler) queryAppData(ctx context.Context, slug string, limit int, filterExpr string) (string, error) {
	col, ok := h.cache.CollectionBySlug(slug)
	if !ok {
		return "", fmt.Errorf("앱 '%s'을(를) 찾을 수 없습니다", slug)
	}

	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	fields := h.cache.Fields(col.ID)

	// Build SELECT columns (only data fields, skip layout).
	selectCols := []string{"id", "_created_at", "_updated_at", "_created_by"}
	for _, f := range fields {
		if f.IsLayout {
			continue
		}
		selectCols = append(selectCols, pgutil.QuoteIdent(f.Slug))
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	where := "deleted_at IS NULL"
	var args []any

	// Parse simple filters from filterExpr (e.g. "status=eq:active,priority=gt:3").
	if filterExpr != "" {
		parts := strings.Split(filterExpr, ",")
		for _, part := range parts {
			eqIdx := strings.Index(part, "=")
			if eqIdx < 0 {
				continue
			}
			fieldSlug := part[:eqIdx]
			opVal := part[eqIdx+1:]
			colonIdx := strings.Index(opVal, ":")
			if colonIdx < 0 {
				continue
			}
			op := opVal[:colonIdx]
			val := opVal[colonIdx+1:]

			// Validate field exists.
			validField := false
			for _, f := range fields {
				if f.Slug == fieldSlug {
					validField = true
					break
				}
			}
			if !validField {
				continue
			}

			argIdx := len(args) + 1
			sqlOp := ""
			switch op {
			case "eq":
				sqlOp = "="
			case "neq":
				sqlOp = "!="
			case "gt":
				sqlOp = ">"
			case "gte":
				sqlOp = ">="
			case "lt":
				sqlOp = "<"
			case "lte":
				sqlOp = "<="
			case "like":
				sqlOp = "ILIKE"
				val = "%" + val + "%"
			case "is_null":
				if val == "true" {
					where += fmt.Sprintf(" AND %q IS NULL", fieldSlug)
				} else {
					where += fmt.Sprintf(" AND %q IS NOT NULL", fieldSlug)
				}
				continue
			default:
				continue
			}

			where += fmt.Sprintf(" AND %q %s $%d", fieldSlug, sqlOp, argIdx)
			args = append(args, val)
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY _created_at DESC LIMIT %d",
		strings.Join(selectCols, ", "), qTable, where, limit)

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		return "", fmt.Errorf("데이터 조회 실패: %w", err)
	}
	records, err := collectRows(rows)
	rows.Close()
	if err != nil {
		return "", fmt.Errorf("데이터 파싱 실패: %w", err)
	}

	// Count total.
	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", qTable, where)
	if err := h.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		total = int64(len(records))
	}

	// Format results as readable text for AI.
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s (%s) 데이터 조회 결과 — 전체 %d건 중 %d건 표시\n\n",
		col.Label, col.Slug, total, len(records))

	if len(records) == 0 {
		sb.WriteString("(데이터 없음)\n")
		return sb.String(), nil
	}

	// Build field label map for readable output.
	labelMap := map[string]string{
		"id":          "ID",
		"_created_at": "생성일",
		"_updated_at": "수정일",
		"_created_by": "작성자",
	}
	for _, f := range fields {
		labelMap[f.Slug] = f.Label
	}

	for i, rec := range records {
		fmt.Fprintf(&sb, "### 데이터 %d\n", i+1)
		for _, colName := range selectCols {
			label := labelMap[colName]
			if label == "" {
				label = colName
			}
			val := rec[strings.Trim(colName, `"`)]
			if val == nil {
				continue
			}
			var valStr string
			switch v := val.(type) {
			case map[string]any:
				b, _ := json.Marshal(v)
				valStr = string(b)
			case []any:
				b, _ := json.Marshal(v)
				valStr = string(b)
			default:
				valStr = fmt.Sprintf("%v", v)
			}
			fmt.Fprintf(&sb, "- %s: %s\n", label, valStr)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
