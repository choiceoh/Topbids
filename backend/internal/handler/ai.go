package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/ai"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// AIHandler serves AI-powered endpoints.
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

type aiBuildRequest struct {
	Description string            `json:"description"`
	Answers     map[string]string `json:"answers,omitempty"` // question_id → answer (second round)
}

type aiBuildField struct {
	Slug       string                 `json:"slug"`
	Label      string                 `json:"label"`
	FieldType  string                 `json:"field_type"`
	IsRequired bool                   `json:"is_required"`
	Width      int                    `json:"width"`
	Height     int                    `json:"height"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// aiBuildQuestion is a clarifying question the AI wants to ask.
type aiBuildQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Placeholder string   `json:"placeholder,omitempty"`
	Choices     []string `json:"choices,omitempty"` // optional: predefined choices
}

// aiBuildEnvelope wraps both question and schema responses.
// If Questions is non-empty the frontend should show them; otherwise Schema is the result.
type aiBuildEnvelope struct {
	Mode      string            `json:"mode"` // "questions" | "schema"
	Questions []aiBuildQuestion `json:"questions,omitempty"`
	Schema    *aiBuildSchema    `json:"schema,omitempty"`
}

type aiBuildSchema struct {
	Slug        string         `json:"slug"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Icon        string         `json:"icon,omitempty"`
	Fields      []aiBuildField `json:"fields"`
}

// legacy flat response used internally for AI JSON parsing
type aiBuildResponse = aiBuildSchema

const systemPromptBase = `You are a schema designer for Topworks, a no-code business app platform used by a Korean company.
The user will describe a business process or data they want to manage. You must generate a collection (app) schema as JSON.

## Output Format
Output ONLY valid JSON — no markdown fences, no explanation, no extra text.
{
  "slug": "snake_case_english",
  "label": "Korean display name",
  "description": "Korean description (1-2 sentences explaining purpose)",
  "fields": [
    {
      "slug": "snake_case_english",
      "label": "Korean field name",
      "field_type": "one of the valid types",
      "is_required": true/false,
      "width": 6,
      "height": 1,
      "options": {}
    }
  ]
}

## Field Types & Options
Data field types: text, textarea, number, integer, boolean, date, datetime, time, select, multiselect, user, file, autonumber, relation, json, formula, lookup, rollup.
Layout types (visual separator only, no data stored): label, line, spacer.

Option rules per type:
- select / multiselect → "options": {"choices": ["선택1", "선택2", ...]}. Always provide 3-6 realistic Korean choices.
- number (currency) → "options": {"display_type": "currency", "currency_code": "KRW"}
- number (percent) → "options": {"display_type": "percent"}
- text (URL) → "options": {"display_type": "url"}
- text (email) → "options": {"display_type": "email"}
- text (phone) → "options": {"display_type": "phone"}
- autonumber → "options": {"prefix": "XXX-", "start": 1}. Prefix should be a short uppercase abbreviation of the app (e.g. "BT-" for business trip, "PO-" for purchase order).
- label → "options": {"text": "섹션 제목 텍스트"} (section header, bold text)
- formula → "options": {"expression": "field_slug * other_slug"}. Reference other field slugs in the same schema.
- lookup → "options": {"relation_field": "relation_slug", "target_field": "target_slug"}
- rollup → "options": {"relation_field": "relation_slug", "target_field": "target_slug", "function": "SUM"} (functions: SUM/COUNT/AVG/MIN/MAX/COUNTA)

## Slug Rules
- Start with lowercase letter, only [a-z0-9_], max 63 chars.
- Reserved (do NOT use as field slug): id, created_at, updated_at, created_by, updated_by, deleted_at, _status.
- Slugs must be unique within the schema. Use descriptive names (e.g. start_date, end_date — not date1, date2).

## Layout Rules
- width values: 2 (1/3), 3 (1/2), 4 (2/3), 6 (full width). Default 6.
- Row grouping: consecutive fields whose widths sum to 6 appear on the same row.
  Example: [width=3, width=3] → one row with two half-width fields.
  Example: [width=2, width=2, width=2] → one row with three 1/3-width fields.
- height: 1, 2, or 3 rows. Default 1. Use 2 for textarea, 3 for long descriptions.
- Use "label" type as section headers and "line" type as separators to organize forms with 8+ fields.

## Design Guidelines
1. **Field count**: 6-12 fields for simple apps, 12-20 for complex workflows. Avoid under-designing or over-designing.
2. **Language**: All labels and descriptions MUST be in Korean. Slugs are in English.
3. **Structure pattern for workflow apps**:
   - Start with an autonumber field for unique ID (e.g. "접수번호", "신청번호")
   - Core identification fields (제목/이름/건명) near the top, required
   - Related details grouped together (use label sections for 8+ field forms)
   - Status field (select) for workflow state tracking
   - User field for 담당자/신청자 assignment
   - Date fields for 일정/마감일/시작일-종료일
   - Amount/number fields with proper display_type (currency for 금액)
   - Textarea for 비고/메모/특이사항 at the bottom
4. **Smart defaults**:
   - Status choices should reflect the actual workflow: ["신청", "검토중", "승인", "반려"] or ["대기", "진행중", "완료", "보류"]
   - Priority choices: ["긴급", "높음", "보통", "낮음"]
   - Boolean for yes/no toggles (e.g. 완료 여부, 승인 여부)
   - Date pairs (시작일 + 종료일) on the same row with width=3 each
5. **Korean business context**:
   - 품의/결재 workflows need 기안자, 결재자, 결재일, 결재상태
   - 거래처/고객 management needs 업체명, 사업자번호, 대표자, 연락처, 주소
   - 재고/자산 tracking needs 품목명, 수량, 단가, 금액(formula), 위치
   - 일정/프로젝트 management needs 제목, 시작일, 종료일, 진행률(number percent), 담당자
6. **태양광 발전사업 도메인** (이 플랫폼의 주요 사용 분야):
   - 인허가 관리: 발전사업허가→개발행위허가→농지/산지전용→환경영향평가→공사계획인가→사용전검사→계통연계→COD
   - 인허가 앱에는 허가종류(select), 신청일/접수일/허가일(date), 관할기관(text), 처리상태(select: 준비중/신청/보완요청/허가/반려), 담당자(user), 비고(textarea)
   - 프로젝트 관리 앱에는 사업명, 용량(number, kW), 사업단계(select: 기획/인허가/시공/시운전/운영), 위치, EPC업체, 착공일/준공예정일, 사업비(currency)
   - 주요 용어: RPS, REC, SMP, PPA, COD, EPC, 계통연계, 출력제어, 영농형 태양광
   - 규모별 차이를 인지: 1MW 미만(소규모), 1~100MW(중규모), 100MW 이상(대규모)

## Example — "출장 신청서"
{"slug":"business_trip_request","label":"출장 신청서","description":"임직원 출장 신청 및 승인 관리","fields":[{"slug":"request_no","label":"신청번호","field_type":"autonumber","is_required":false,"width":3,"height":1,"options":{"prefix":"BT-","start":1}},{"slug":"status","label":"처리상태","field_type":"select","is_required":true,"width":3,"height":1,"options":{"choices":["신청","검토중","승인","반려"]}},{"slug":"requester","label":"신청자","field_type":"user","is_required":true,"width":3,"height":1,"options":{}},{"slug":"department","label":"부서","field_type":"text","is_required":true,"width":3,"height":1,"options":{}},{"slug":"destination","label":"출장지","field_type":"text","is_required":true,"width":6,"height":1,"options":{}},{"slug":"start_date","label":"출장 시작일","field_type":"date","is_required":true,"width":3,"height":1,"options":{}},{"slug":"end_date","label":"출장 종료일","field_type":"date","is_required":true,"width":3,"height":1,"options":{}},{"slug":"purpose","label":"출장 목적","field_type":"textarea","is_required":true,"width":6,"height":2,"options":{}},{"slug":"estimated_cost","label":"예상 경비","field_type":"number","is_required":false,"width":3,"height":1,"options":{"display_type":"currency","currency_code":"KRW"}},{"slug":"transport","label":"교통수단","field_type":"select","is_required":false,"width":3,"height":1,"options":{"choices":["자가용","KTX","항공","버스","기타"]}},{"slug":"remarks","label":"비고","field_type":"textarea","is_required":false,"width":6,"height":2,"options":{}}]}`

const triagePrompt = `You are a requirements analyst for Topworks, a no-code business app platform.
The user wants to create a new app. Your job is to decide whether their description is clear enough to generate a good schema, or if you need to ask clarifying questions first.

## Decision Rule: Default to PROCEED
Most descriptions are clear enough. Only ask questions when ambiguity would lead to a fundamentally different schema.

### → Proceed (output {"mode": "proceed"}) when:
- The description names a specific form/document (e.g. "출장 신청서", "견적서", "휴가 신청")
- The description names a specific management target (e.g. "거래처 관리", "재고 관리", "프로젝트 관리")
- The description is a common business process with well-known fields
- Minor details are missing but can be reasonably assumed from Korean business convention
- Short descriptions with clear intent (e.g. "회의록" → obvious structure)

### → Ask questions (max 2-3) ONLY when:
- The description is a single vague word like "관리" or "목록" without a subject
- The business process could mean very different schemas (e.g. "인사 관리" could be 채용/급여/인사기록)
- Industry-specific terms where the wrong assumption would produce useless fields

## Output Format
Output ONLY valid JSON — no markdown, no explanation.

If clear enough:
{"mode": "proceed"}

If you need clarification:
{"mode": "questions", "questions": [{"id": "q1", "question": "Korean question", "placeholder": "예: 힌트", "choices": ["선택1", "선택2"]}]}

## Examples of proceed vs questions:
- "출장 신청서" → {"mode": "proceed"} (well-known form)
- "고객 관리" → {"mode": "proceed"} (standard CRM fields)
- "재고 현황판" → {"mode": "proceed"} (inventory tracking is clear)
- "회의실 예약" → {"mode": "proceed"} (booking system fields are obvious)
- "인허가 체크리스트" → {"mode": "proceed"} (solar permit tracking — well-known domain)
- "태양광 프로젝트 관리" → {"mode": "proceed"} (solar project management is clear)
- "발전사업허가 신청 관리" → {"mode": "proceed"} (specific permit type)
- "관리" → questions (what are you managing?)
- "우리팀 앱" → questions (too vague, need scope)
- "인사" → questions (recruitment? payroll? records?)

## Rules for questions:
- Max 2-3 questions. Be concise and specific.
- Questions MUST be in Korean.
- Always include "choices" array with 2-4 common options to help the user pick quickly.
- Include "placeholder" as a hint for free-text answers.
- Each question has a unique "id" (q1, q2, q3).
- Focus questions on what would change the field structure, not cosmetic details.`

const slugPrompt = `You are a slug generator for a no-code business app platform.
The user will provide a Korean name (label) for a collection or field.
You must generate an English snake_case slug that represents the meaning.

Rules:
1. Output ONLY the slug string — no quotes, no explanation, no extra text.
2. The slug must start with a lowercase letter, only contain [a-z0-9_], and be at most 63 chars.
3. Do NOT use these reserved slugs: id, created_at, updated_at, created_by, updated_by, deleted_at, _status.
4. Translate the Korean meaning to concise English, then convert to snake_case.
5. Examples:
   - "인허가 체크리스트" → "permit_checklist"
   - "프로젝트 관리" → "project_management"
   - "출장 신청서" → "business_trip_request"
   - "재고 현황" → "inventory_status"
   - "거래처 목록" → "vendor_list"`

type aiSlugRequest struct {
	Label string `json:"label"`
}

type aiSlugResponse struct {
	Slug string `json:"slug"`
}

// GenerateSlug generates an English snake_case slug from a Korean label.
func (h *AIHandler) GenerateSlug(w http.ResponseWriter, r *http.Request) {
	var req aiSlugRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	raw, err := h.client.Complete(r.Context(), slugPrompt, req.Label)
	if err != nil {
		slog.Error("ai generate-slug failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	slug := sanitizeSlug(strings.TrimSpace(raw))
	writeJSON(w, http.StatusOK, aiSlugResponse{Slug: slug})
}

// BuildCollection generates a collection schema from a natural-language description.
// Flow: triage (questions?) → generate → text self-critique.
func (h *AIHandler) BuildCollection(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	var req aiBuildRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}

	ctx := r.Context()

	// ── Step 0: Triage — ask clarifying questions if needed ──
	// Skip triage if user already provided answers (second round).
	if len(req.Answers) == 0 {
		slog.Info("ai build: step 0 - triage")

		raw, err := h.client.Complete(ctx, triagePrompt, req.Description)
		if err != nil {
			slog.Warn("ai triage failed, proceeding to generation", "error", err)
		} else if questions, ok := parseTriageResponse(raw); ok && len(questions) > 0 {
			writeJSON(w, http.StatusOK, aiBuildEnvelope{
				Mode:      "questions",
				Questions: questions,
			})
			return
		}
	}

	// Build the full description including answers if present.
	fullDescription := req.Description
	if len(req.Answers) > 0 {
		var sb strings.Builder
		sb.WriteString(req.Description)
		sb.WriteString("\n\n추가 정보:\n")
		for qID, answer := range req.Answers {
			fmt.Fprintf(&sb, "- %s: %s\n", qID, answer)
		}
		fullDescription = sb.String()
	}

	// ── Step 1: Generate schema ──
	slog.Info("ai build: step 1 - generating schema")
	raw, err := h.client.Complete(ctx, systemPromptBase, fullDescription)
	if err != nil {
		slog.Error("ai build step 1 failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	result, err := parseAndSanitize(raw)
	if err != nil {
		slog.Error("ai step 1 returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	writeJSON(w, http.StatusOK, aiBuildEnvelope{Mode: "schema", Schema: &result})
}

// triageResult is the shape returned by the triage prompt.
type triageResult struct {
	Mode      string            `json:"mode"`
	Questions []aiBuildQuestion `json:"questions,omitempty"`
}

// parseTriageResponse attempts to parse a triage JSON response.
// Returns questions and true if the AI wants to ask, or nil and false to proceed.
func parseTriageResponse(raw string) ([]aiBuildQuestion, bool) {
	jsonStr := extractJSON(raw)
	var tr triageResult
	if err := json.Unmarshal([]byte(jsonStr), &tr); err != nil {
		return nil, false
	}
	if tr.Mode == "questions" && len(tr.Questions) > 0 {
		return tr.Questions, true
	}
	return nil, false
}

// parseAndSanitize extracts JSON from AI output, parses it, and sanitizes slugs.
func parseAndSanitize(raw string) (aiBuildResponse, error) {
	jsonStr := extractJSON(raw)
	var result aiBuildResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return aiBuildResponse{}, err
	}
	result.Slug = sanitizeSlug(result.Slug)
	for i := range result.Fields {
		result.Fields[i].Slug = sanitizeSlug(result.Fields[i].Slug)
		if result.Fields[i].Width == 0 {
			result.Fields[i].Width = 6
		}
		if result.Fields[i].Height == 0 {
			result.Fields[i].Height = 1
		}
	}
	return result, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9_]`)

func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "_")
	// Ensure starts with letter.
	if len(s) > 0 && (s[0] < 'a' || s[0] > 'z') {
		s = "x_" + s
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// extractJSON tries to pull a JSON object from AI output that might be wrapped
// in markdown code fences or have extra text around it.
func extractJSON(s string) string {
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

	// Find the outermost { ... }.
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				goto done
			}
		}
	}
done:
	if end > start {
		return s[start : end+1]
	}
	return s[start:]
}
