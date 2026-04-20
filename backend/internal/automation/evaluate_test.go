package automation

import (
	"encoding/json"
	"testing"

	"github.com/choiceoh/phaeton/backend/internal/events"
)

func TestEvaluateConditions_Empty(t *testing.T) {
	if !evaluateConditions(nil, map[string]any{"x": 1}) {
		t.Error("empty conditions should return true")
	}
}

func TestEvaluateConditions_Equals(t *testing.T) {
	conds := []Condition{{FieldSlug: "status", Operator: OpEquals, Value: "open"}}
	if !evaluateConditions(conds, map[string]any{"status": "open"}) {
		t.Error("should match equals")
	}
	if evaluateConditions(conds, map[string]any{"status": "closed"}) {
		t.Error("should not match different value")
	}
}

func TestEvaluateConditions_NotEquals(t *testing.T) {
	conds := []Condition{{FieldSlug: "status", Operator: OpNotEquals, Value: "closed"}}
	if !evaluateConditions(conds, map[string]any{"status": "open"}) {
		t.Error("should match not_equals")
	}
	if evaluateConditions(conds, map[string]any{"status": "closed"}) {
		t.Error("should not match when equal")
	}
}

func TestEvaluateConditions_Contains(t *testing.T) {
	conds := []Condition{{FieldSlug: "name", Operator: OpContains, Value: "hello"}}
	if !evaluateConditions(conds, map[string]any{"name": "say hello world"}) {
		t.Error("should match contains")
	}
	if evaluateConditions(conds, map[string]any{"name": "goodbye"}) {
		t.Error("should not match when not contained")
	}
}

func TestEvaluateConditions_GT_LT(t *testing.T) {
	gtCond := []Condition{{FieldSlug: "amount", Operator: OpGT, Value: "100"}}
	ltCond := []Condition{{FieldSlug: "amount", Operator: OpLT, Value: "100"}}

	if !evaluateConditions(gtCond, map[string]any{"amount": 200}) {
		t.Error("200 should be > 100")
	}
	if evaluateConditions(gtCond, map[string]any{"amount": 50}) {
		t.Error("50 should not be > 100")
	}
	if !evaluateConditions(ltCond, map[string]any{"amount": 50}) {
		t.Error("50 should be < 100")
	}
	if evaluateConditions(ltCond, map[string]any{"amount": 200}) {
		t.Error("200 should not be < 100")
	}
}

func TestEvaluateConditions_IsEmpty(t *testing.T) {
	conds := []Condition{{FieldSlug: "note", Operator: OpIsEmpty}}

	if !evaluateConditions(conds, map[string]any{}) {
		t.Error("missing field should be empty")
	}
	if !evaluateConditions(conds, map[string]any{"note": nil}) {
		t.Error("nil field should be empty")
	}
	if !evaluateConditions(conds, map[string]any{"note": ""}) {
		t.Error("empty string should be empty")
	}
	if evaluateConditions(conds, map[string]any{"note": "something"}) {
		t.Error("non-empty string should not be empty")
	}
}

func TestEvaluateConditions_IsNotEmpty(t *testing.T) {
	conds := []Condition{{FieldSlug: "note", Operator: OpIsNotEmpty}}

	if evaluateConditions(conds, map[string]any{}) {
		t.Error("missing field should not be not-empty")
	}
	if !evaluateConditions(conds, map[string]any{"note": "hi"}) {
		t.Error("non-empty string should be not-empty")
	}
}

func TestEvaluateConditions_AND(t *testing.T) {
	conds := []Condition{
		{FieldSlug: "status", Operator: OpEquals, Value: "open"},
		{FieldSlug: "priority", Operator: OpEquals, Value: "high"},
	}

	if !evaluateConditions(conds, map[string]any{"status": "open", "priority": "high"}) {
		t.Error("both conditions met, should return true")
	}
	if evaluateConditions(conds, map[string]any{"status": "open", "priority": "low"}) {
		t.Error("second condition not met, should return false")
	}
}

func TestMatchTriggerConfig_NonStatusChange(t *testing.T) {
	a := Automation{TriggerType: TriggerRecordCreated}
	if !matchTriggerConfig(a, "", "", "") {
		t.Error("non-status_change trigger should always match")
	}
}

func TestMatchTriggerConfig_StatusChange(t *testing.T) {
	tests := []struct {
		name      string
		cfg       TriggerStatusConfig
		from, to  string
		wantMatch bool
	}{
		{
			name:      "exact match",
			cfg:       TriggerStatusConfig{FromStatus: "open", ToStatus: "closed"},
			from:      "open",
			to:        "closed",
			wantMatch: true,
		},
		{
			name:      "from mismatch",
			cfg:       TriggerStatusConfig{FromStatus: "open", ToStatus: "closed"},
			from:      "in_progress",
			to:        "closed",
			wantMatch: false,
		},
		{
			name:      "to mismatch",
			cfg:       TriggerStatusConfig{FromStatus: "open", ToStatus: "closed"},
			from:      "open",
			to:        "in_progress",
			wantMatch: false,
		},
		{
			name:      "wildcard from",
			cfg:       TriggerStatusConfig{FromStatus: "", ToStatus: "closed"},
			from:      "anything",
			to:        "closed",
			wantMatch: true,
		},
		{
			name:      "wildcard both",
			cfg:       TriggerStatusConfig{FromStatus: "", ToStatus: ""},
			from:      "x",
			to:        "y",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgJSON, _ := json.Marshal(tt.cfg)
			a := Automation{
				TriggerType:   TriggerStatusChange,
				TriggerConfig: cfgJSON,
			}
			got := matchTriggerConfig(a, tt.from, tt.to, "")
			if got != tt.wantMatch {
				t.Errorf("matchTriggerConfig() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestMatchTriggerConfig_InvalidJSON(t *testing.T) {
	a := Automation{
		TriggerType:   TriggerStatusChange,
		TriggerConfig: json.RawMessage(`invalid`),
	}
	if matchTriggerConfig(a, "a", "b", "") {
		t.Error("invalid JSON config should not match")
	}
}

func TestStringify(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{json.Number("42"), "42"},
		{123, "123"},
		{true, "true"},
	}
	for _, tt := range tests {
		got := stringify(tt.input)
		if got != tt.want {
			t.Errorf("stringify(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"10", "5", 1},
		{"5", "10", -1},
		{"5", "5", 0},
		{"abc", "def", -1}, // falls back to string compare
	}
	for _, tt := range tests {
		got := compareNumeric(tt.a, tt.b)
		if (tt.want < 0 && got >= 0) || (tt.want > 0 && got <= 0) || (tt.want == 0 && got != 0) {
			t.Errorf("compareNumeric(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMatchTriggerConfig_FormSubmit(t *testing.T) {
	tests := []struct {
		name      string
		cfg       FormSubmitConfig
		formSlug  string
		wantMatch bool
	}{
		{
			name:      "exact form match",
			cfg:       FormSubmitConfig{FormSlug: "contact"},
			formSlug:  "contact",
			wantMatch: true,
		},
		{
			name:      "form mismatch",
			cfg:       FormSubmitConfig{FormSlug: "contact"},
			formSlug:  "signup",
			wantMatch: false,
		},
		{
			name:      "empty config matches all",
			cfg:       FormSubmitConfig{FormSlug: ""},
			formSlug:  "anything",
			wantMatch: true,
		},
		{
			name:      "empty form slug with config",
			cfg:       FormSubmitConfig{FormSlug: "contact"},
			formSlug:  "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgJSON, _ := json.Marshal(tt.cfg)
			a := Automation{
				TriggerType:   TriggerFormSubmit,
				TriggerConfig: cfgJSON,
			}
			got := matchTriggerConfig(a, "", "", tt.formSlug)
			if got != tt.wantMatch {
				t.Errorf("matchTriggerConfig() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestMatchTriggerConfig_FormSubmit_InvalidJSON(t *testing.T) {
	a := Automation{
		TriggerType:   TriggerFormSubmit,
		TriggerConfig: json.RawMessage(`invalid`),
	}
	// Invalid JSON in form_submit defaults to matching (see evaluate.go).
	if !matchTriggerConfig(a, "", "", "any") {
		t.Error("invalid JSON form_submit config should match (fallback)")
	}
}

func TestMatchTriggerConfig_FormSubmit_NullConfig(t *testing.T) {
	a := Automation{
		TriggerType:   TriggerFormSubmit,
		TriggerConfig: json.RawMessage(`{}`),
	}
	if !matchTriggerConfig(a, "", "", "some-form") {
		t.Error("empty config object should match all forms")
	}
}

func TestMapEventType_FormSubmit(t *testing.T) {
	got := mapEventType(events.EventFormSubmit)
	if got != TriggerFormSubmit {
		t.Errorf("mapEventType(EventFormSubmit) = %q, want %q", got, TriggerFormSubmit)
	}
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		input events.EventType
		want  string
	}{
		{events.EventRecordCreate, TriggerRecordCreated},
		{events.EventRecordUpdate, TriggerRecordUpdated},
		{events.EventRecordDelete, TriggerRecordDeleted},
		{events.EventStateChange, TriggerStatusChange},
		{events.EventComment, ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := mapEventType(tt.input)
		if got != tt.want {
			t.Errorf("mapEventType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
