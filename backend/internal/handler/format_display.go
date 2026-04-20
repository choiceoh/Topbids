package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// resolveAutoExpand replaces the special "auto" value with all relation field slugs.
func resolveAutoExpand(expand string, fields []schema.Field) string {
	if expand != "auto" {
		return expand
	}
	var relSlugs []string
	for _, f := range fields {
		if f.FieldType == schema.FieldRelation {
			relSlugs = append(relSlugs, f.Slug)
		}
	}
	return strings.Join(relSlugs, ",")
}

// applyDisplayFormat adds a "__display" map to each record with pre-formatted
// string values, mirroring the logic in frontend/src/lib/formatCell.ts.
func applyDisplayFormat(records []map[string]any, fields []schema.Field) {
	for _, rec := range records {
		display := make(map[string]string, len(fields))
		for _, f := range fields {
			if f.FieldType.IsLayout() {
				continue
			}
			v, ok := rec[f.Slug]
			if !ok || v == nil {
				display[f.Slug] = "-"
				continue
			}
			display[f.Slug] = formatDisplayValue(v, f)
		}
		rec["__display"] = display
	}
}

// formatDisplayValue converts a field value to a display string.
func formatDisplayValue(value any, f schema.Field) string {
	if value == nil {
		return "-"
	}

	switch f.FieldType {
	case schema.FieldRelation:
		return formatRelationValue(value)

	case schema.FieldUser:
		return formatUserValue(value)

	case schema.FieldBoolean:
		if b, ok := value.(bool); ok && b {
			return "✓"
		}
		return "-"

	case schema.FieldDate, schema.FieldDatetime:
		return formatDateValue(value)

	case schema.FieldTime:
		return fmt.Sprintf("%v", value)

	case schema.FieldMultiselect:
		if arr, ok := value.([]any); ok {
			parts := make([]string, 0, len(arr))
			for _, v := range arr {
				parts = append(parts, fmt.Sprintf("%v", v))
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%v", value)

	case schema.FieldTextarea:
		s := fmt.Sprintf("%v", value)
		if len(s) > 100 {
			return s[:100] + "..."
		}
		return s

	case schema.FieldJSON:
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(b)

	case schema.FieldTable, schema.FieldSpreadsheet:
		if arr, ok := value.([]any); ok {
			return fmt.Sprintf("%d행", len(arr))
		}
		return "-"

	case schema.FieldNumber, schema.FieldInteger:
		return formatNumberValue(value, f)

	case schema.FieldAutonumber:
		return fmt.Sprintf("%v", value)

	case schema.FieldFormula:
		return formatFormulaValue(value, f)

	case schema.FieldRollup:
		if n, ok := toFloat64(value); ok {
			return formatKoreanNumber(n, -1)
		}
		return fmt.Sprintf("%v", value)

	case schema.FieldLookup:
		if arr, ok := value.([]any); ok {
			parts := make([]string, 0, len(arr))
			for _, v := range arr {
				parts = append(parts, fmt.Sprintf("%v", v))
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%v", value)

	default:
		return fmt.Sprintf("%v", value)
	}
}

// formatRelationValue formats a relation field value (object, array, or ID string).
func formatRelationValue(value any) string {
	switch v := value.(type) {
	case []any:
		if len(v) == 0 {
			return "-"
		}
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				parts = append(parts, extractName(m))
			} else {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		return extractName(v)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// formatUserValue formats a user field value.
func formatUserValue(value any) string {
	if m, ok := value.(map[string]any); ok {
		for _, key := range []string{"name", "email", "id"} {
			if v, ok := m[key]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return fmt.Sprintf("%v", value)
}

// formatDateValue formats a date/datetime value in Korean locale style.
func formatDateValue(value any) string {
	s := fmt.Sprintf("%v", value)
	// Try RFC3339 first, then date-only.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return fmt.Sprintf("%d. %d. %d.", t.Year(), int(t.Month()), t.Day())
	}
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return fmt.Sprintf("%d. %d. %d.", t.Year(), int(t.Month()), t.Day())
		}
	}
	return s
}

// formatNumberValue formats a number/integer with optional display_type.
func formatNumberValue(value any, f schema.Field) string {
	num, ok := toFloat64(value)
	if !ok {
		return fmt.Sprintf("%v", value)
	}

	dt := optionString(f.Options, "display_type")
	switch dt {
	case "currency":
		code := optionString(f.Options, "currency_code")
		if code == "" {
			code = "KRW"
		}
		if code == "KRW" || code == "JPY" {
			return fmt.Sprintf("₩%s", formatKoreanNumber(num, 0))
		}
		return fmt.Sprintf("%s %s", code, formatKoreanNumber(num, 2))
	case "percent":
		return fmt.Sprintf("%v%%", value)
	case "rating":
		maxRating := optionFloat(f.Options, "max_rating", 5)
		n := int(math.Min(num, maxRating))
		return strings.Repeat("★", n) + strings.Repeat("☆", int(math.Max(0, maxRating-float64(n))))
	case "progress":
		return fmt.Sprintf("%v%%", value)
	default:
		return formatKoreanNumber(num, -1)
	}
}

// formatFormulaValue formats a formula field based on its result_type.
func formatFormulaValue(value any, f schema.Field) string {
	resultType := optionString(f.Options, "result_type")
	switch resultType {
	case "number", "integer":
		if num, ok := toFloat64(value); ok {
			precision := optionInt(f.Options, "precision", -1)
			return formatKoreanNumber(num, precision)
		}
	case "boolean":
		if b, ok := value.(bool); ok && b {
			return "✓"
		}
		return "-"
	case "date":
		return formatDateValue(value)
	}
	return fmt.Sprintf("%v", value)
}

// formatKoreanNumber formats a number with Korean locale-style thousands separator.
// precision < 0 means auto (no trailing zeros removal).
func formatKoreanNumber(n float64, precision int) string {
	if precision >= 0 {
		format := fmt.Sprintf("%%.%df", precision)
		s := fmt.Sprintf(format, n)
		return addThousandsSep(s)
	}
	// Auto precision.
	if n == float64(int64(n)) {
		return addThousandsSep(fmt.Sprintf("%d", int64(n)))
	}
	return addThousandsSep(fmt.Sprintf("%g", n))
}

// addThousandsSep adds commas to the integer part of a number string.
func addThousandsSep(s string) string {
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	negative := false
	if strings.HasPrefix(intPart, "-") {
		negative = true
		intPart = intPart[1:]
	}

	// Add commas from right to left.
	result := make([]byte, 0, len(intPart)+len(intPart)/3)
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}

	out := string(result)
	if negative {
		out = "-" + out
	}
	if len(parts) == 2 {
		out += "." + parts[1]
	}
	return out
}

// extractName extracts name/title/label/id from a map.
func extractName(m map[string]any) string {
	for _, key := range []string{"name", "title", "label", "id"} {
		if v, ok := m[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return "?"
}

// optionString extracts a string value from field options JSON.
func optionString(opts json.RawMessage, key string) string {
	if len(opts) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(opts, &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// optionFloat extracts a float64 value from field options JSON.
func optionFloat(opts json.RawMessage, key string, defaultVal float64) float64 {
	if len(opts) == 0 {
		return defaultVal
	}
	var m map[string]any
	if err := json.Unmarshal(opts, &m); err != nil {
		return defaultVal
	}
	if n, ok := m[key].(float64); ok {
		return n
	}
	return defaultVal
}

// optionInt extracts an int value from field options JSON.
func optionInt(opts json.RawMessage, key string, defaultVal int) int {
	if len(opts) == 0 {
		return defaultVal
	}
	var m map[string]any
	if err := json.Unmarshal(opts, &m); err != nil {
		return defaultVal
	}
	if n, ok := m[key].(float64); ok {
		return int(n)
	}
	return defaultVal
}
