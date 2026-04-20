package automation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// evaluateConditions returns true if all conditions match the record (AND logic).
// An empty condition list always returns true.
func evaluateConditions(conditions []Condition, record map[string]any) bool {
	for _, c := range conditions {
		if !evaluateOne(c, record) {
			return false
		}
	}
	return true
}

// evaluateOne tests a single condition against a record. It handles each operator:
//   - is_empty / is_not_empty: check field existence and empty string
//   - equals / not_equals: string comparison after stringify
//   - contains: substring match
//   - gt / lt: numeric comparison with fallback to lexicographic order
func evaluateOne(c Condition, record map[string]any) bool {
	val, exists := record[c.FieldSlug]

	switch c.Operator {
	case OpIsEmpty:
		return !exists || val == nil || fmt.Sprintf("%v", val) == ""
	case OpIsNotEmpty:
		return exists && val != nil && fmt.Sprintf("%v", val) != ""
	}

	// For remaining operators, stringify both sides for comparison.
	actual := stringify(val)
	expected := c.Value

	switch c.Operator {
	case OpEquals:
		return actual == expected
	case OpNotEquals:
		return actual != expected
	case OpContains:
		return strings.Contains(actual, expected)
	case OpGT:
		return compareNumeric(actual, expected) > 0
	case OpLT:
		return compareNumeric(actual, expected) < 0
	default:
		return false
	}
}

// matchTriggerConfig checks trigger-specific config (e.g. from/to status, form slug).
func matchTriggerConfig(a Automation, statusFrom, statusTo, formSlug string) bool {
	switch a.TriggerType {
	case TriggerStatusChange:
		var cfg TriggerStatusConfig
		if err := json.Unmarshal(a.TriggerConfig, &cfg); err != nil {
			return false
		}
		if cfg.FromStatus != "" && cfg.FromStatus != statusFrom {
			return false
		}
		if cfg.ToStatus != "" && cfg.ToStatus != statusTo {
			return false
		}
		return true
	case TriggerFormSubmit:
		var cfg FormSubmitConfig
		if err := json.Unmarshal(a.TriggerConfig, &cfg); err != nil {
			return true // no config = match all forms
		}
		if cfg.FormSlug != "" && cfg.FormSlug != formSlug {
			return false
		}
		return true
	default:
		return true
	}
}

// stringify converts any value to its string representation for comparison.
// nil becomes "", json.Number uses its string form, and everything else uses fmt.Sprintf.
func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch tv := v.(type) {
	case string:
		return tv
	case json.Number:
		return tv.String()
	default:
		return fmt.Sprintf("%v", tv)
	}
}

// compareNumeric attempts to parse both strings as float64 and returns -1, 0, or 1.
// If either value cannot be parsed as a number, it falls back to lexicographic
// string comparison via strings.Compare.
func compareNumeric(a, b string) int {
	var fa, fb float64
	if _, err := fmt.Sscanf(a, "%f", &fa); err != nil {
		return strings.Compare(a, b)
	}
	if _, err := fmt.Sscanf(b, "%f", &fb); err != nil {
		return strings.Compare(a, b)
	}
	switch {
	case fa < fb:
		return -1
	case fa > fb:
		return 1
	default:
		return 0
	}
}
