package automation

import (
	"encoding/json"
	"time"
)

// TriggerType constants define when an automation fires.
const (
	TriggerRecordCreated = "record_created"  // Fires after a new record is inserted.
	TriggerRecordUpdated = "record_updated"  // Fires after any field on an existing record changes.
	TriggerRecordDeleted = "record_deleted"  // Fires after a record is deleted.
	TriggerStatusChange  = "status_change"   // Fires when _status transitions (optionally filtered by from/to).
	TriggerSchedule      = "schedule"        // Fires on a cron schedule (evaluated by the Scheduler).
	TriggerFormSubmit    = "form_submit"     // Fires when a public form is submitted (optionally filtered by form slug).
)

// ActionType constants define what an automation does when triggered.
const (
	ActionSendNotification = "send_notification" // Creates an in-app notification for the target user(s).
	ActionUpdateField      = "update_field"      // Sets a field on the triggering record to a fixed value.
	ActionCallWebhook      = "call_webhook"      // POSTs the record data as JSON to an external URL.
)

// Operator constants for conditions.
const (
	OpEquals     = "equals"
	OpNotEquals  = "not_equals"
	OpContains   = "contains"
	OpGT         = "gt"
	OpLT         = "lt"
	OpIsEmpty    = "is_empty"
	OpIsNotEmpty = "is_not_empty"
)

// Automation is a trigger->condition->action rule scoped to a single collection.
// When IsEnabled is false the engine skips it entirely. TriggerConfig holds
// trigger-specific JSON (e.g. from/to status for status_change, cron for schedule).
// Conditions and Actions are loaded eagerly by loadAutomations and executed in
// SortOrder sequence.
type Automation struct {
	ID            string          `json:"id"`
	CollectionID  string          `json:"collection_id"`
	Name          string          `json:"name"`
	IsEnabled     bool            `json:"is_enabled"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Conditions    []Condition     `json:"conditions"`
	Actions       []Action        `json:"actions"`
	CreatedBy     string          `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Condition is a field-value comparison. All conditions within an automation
// are AND-joined: every condition must match for the actions to execute.
// SortOrder determines the evaluation order (lower values first).
type Condition struct {
	ID        string `json:"id"`
	FieldSlug string `json:"field_slug"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

// Action is what to do when the trigger fires and conditions pass.
// Actions are executed sequentially in SortOrder. If any action fails,
// the remaining actions are skipped and the run is logged as "error".
type Action struct {
	ID           string          `json:"id"`
	ActionType   string          `json:"action_type"`
	ActionConfig json.RawMessage `json:"action_config"`
	SortOrder    int             `json:"sort_order"`
}

// Run is an execution log entry stored in _meta.automation_runs.
// Status is one of "success" (all actions completed), "error" (an action failed),
// or "skipped" (trigger config or conditions did not match).
type Run struct {
	ID           string    `json:"id"`
	AutomationID string    `json:"automation_id"`
	CollectionID string    `json:"collection_id"`
	RecordID     string    `json:"record_id"`
	TriggerType  string    `json:"trigger_type"`
	Status       string    `json:"status"` // success, error, skipped
	ErrorMessage string    `json:"error_message,omitempty"`
	DurationMs   int       `json:"duration_ms"`
	CreatedAt    time.Time `json:"created_at"`
}

// TriggerStatusConfig holds from/to status for status_change triggers.
type TriggerStatusConfig struct {
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
}

// ScheduleConfig holds cron expression for schedule triggers.
type ScheduleConfig struct {
	Cron     string `json:"cron"`     // cron expression e.g. "0 9 * * *" (every day at 9am)
	Timezone string `json:"timezone"` // e.g. "Asia/Seoul"
}

// FormSubmitConfig holds optional form slug filter for form_submit triggers.
type FormSubmitConfig struct {
	FormSlug string `json:"form_slug,omitempty"` // if set, only triggers for this form
}

// NotificationConfig holds config for send_notification actions.
// Recipient determines who receives the notification:
//   - "record_creator": the user who created the triggering record
//   - "specific_user": a fixed user identified by UserID
//   - "field_ref": the user ID stored in the field identified by FieldSlug
type NotificationConfig struct {
	Recipient string `json:"recipient"` // record_creator, specific_user, field_ref
	UserID    string `json:"user_id,omitempty"`
	FieldSlug string `json:"field_slug,omitempty"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// UpdateFieldConfig holds config for update_field actions.
type UpdateFieldConfig struct {
	FieldSlug string `json:"field_slug"`
	Value     string `json:"value"`
}

// WebhookConfig holds config for call_webhook actions.
// The engine POSTs the full record as JSON to URL with optional custom Headers.
// The request has a 30-second timeout and retries are not attempted.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}
