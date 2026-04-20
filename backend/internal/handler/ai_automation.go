package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type aiAutomationRequest struct {
	Description string `json:"description"`
}

const automationSystemPrompt = `You are an automation designer for Topworks, a no-code business app platform used by a Korean company.
The user will describe what automation they want. You must generate an automation rule as JSON.

## Tools
You have tools to look up the target collection's fields. ALWAYS call get_collection_fields first
to know the exact field slugs before generating the automation.
- list_collections: 워크스페이스의 앱 목록 조회
- get_collection_fields: 특정 앱의 필드 상세 조회 (slug 필요)

## Automation Concepts
An automation has: trigger → conditions (optional) → actions.

### Trigger Types
- "record_created" — fires when a new record is created
- "record_updated" — fires when any field in a record is updated
- "record_deleted" — fires when a record is deleted
- "status_change" — fires when the status field changes (requires trigger_config with from_status/to_status)

### Condition Operators
- "equals" — field value equals the given value
- "not_equals" — field value does not equal
- "contains" — field value contains substring
- "gt" — field value greater than (for numbers)
- "lt" — field value less than (for numbers)
- "is_empty" — field value is empty/null (no value needed)
- "is_not_empty" — field value is not empty (no value needed)

### Action Types

1. "send_notification" — Send an in-app notification
   action_config: {
     "recipient": "record_creator" | "specific_user" | "field_ref",
     "user_id": "uuid (only when recipient=specific_user)",
     "field_slug": "field_slug (only when recipient=field_ref, must be a user-type field)",
     "title": "notification title in Korean",
     "body": "notification body in Korean"
   }

2. "update_field" — Update a field value on the record
   action_config: {
     "field_slug": "target_field_slug",
     "value": "new value as string"
   }

3. "call_webhook" — Call an external URL
   action_config: {
     "url": "https://...",
     "headers": {"key": "value"} (optional)
   }

## Output Format
Output ONLY valid JSON — no markdown fences, no explanation.
{
  "name": "Korean name describing the automation",
  "is_enabled": true,
  "trigger_type": "one of the trigger types",
  "trigger_config": {},
  "conditions": [
    {
      "field_slug": "field_slug",
      "operator": "one of the operators",
      "value": "comparison value"
    }
  ],
  "actions": [
    {
      "action_type": "one of the action types",
      "action_config": { ... }
    }
  ]
}

## Rules
- "name" should be a concise Korean description of what the automation does
- "conditions" can be an empty array [] if no filtering is needed (applies to all records)
- For "status_change" trigger, set trigger_config: {"from_status": "상태명", "to_status": "상태명"}
  - Use empty string "" for "any status" (from or to)
- For "send_notification", title and body must be in Korean
- For "update_field", field_slug must reference an existing field in the collection
- For "is_empty" and "is_not_empty" operators, "value" should be ""
- If the user wants multiple actions, include them all in the actions array
- Design practical automations that a Korean business would actually use`

// BuildAutomation generates an automation config from a natural-language description.
func (h *AIHandler) BuildAutomation(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 120*time.Second)
	defer cancel()

	collectionID := chi.URLParam(r, "id")

	var req aiAutomationRequest
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

	// Resolve collection ID → slug/label so the AI can call get_collection_fields.
	userMsg := req.Description
	if cols, err := h.store.ListCollections(ctx); err == nil {
		for _, c := range cols {
			if c.ID == collectionID {
				userMsg = "대상 앱: " + c.Label + " (slug: " + c.Slug + ")\n\n" + req.Description
				break
			}
		}
	}

	raw, err := h.client.CompleteWithTools(ctx, automationSystemPrompt, nil, userMsg, workspaceTools, resolve)
	if err != nil {
		slog.Error("ai build-automation failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI 서버 요청에 실패했습니다")
		return
	}

	jsonStr := extractJSON(raw)
	var result json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		slog.Error("ai automation returned invalid JSON", "raw", raw, "error", err)
		writeError(w, http.StatusBadGateway, "AI 응답을 파싱할 수 없습니다")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
