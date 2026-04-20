package handler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

func TestValidateFieldValueText(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldText}
	if err := validateFieldValue(f, "hello"); err != nil {
		t.Errorf("string value: %v", err)
	}
	if err := validateFieldValue(f, 123.0); err == nil {
		t.Error("number for text should fail")
	}
}

func TestValidateFieldValueNumber(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldNumber}
	if err := validateFieldValue(f, 3.14); err != nil {
		t.Errorf("float value: %v", err)
	}
	if err := validateFieldValue(f, "abc"); err == nil {
		t.Error("string for number should fail")
	}
}

func TestValidateFieldValueInteger(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldInteger}
	if err := validateFieldValue(f, 42.0); err != nil {
		t.Errorf("whole number: %v", err)
	}
	if err := validateFieldValue(f, 3.5); err == nil {
		t.Error("fractional for integer should fail")
	}
	if err := validateFieldValue(f, "abc"); err == nil {
		t.Error("string for integer should fail")
	}
}

func TestValidateFieldValueBoolean(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldBoolean}
	if err := validateFieldValue(f, true); err != nil {
		t.Errorf("bool value: %v", err)
	}
	if err := validateFieldValue(f, "true"); err == nil {
		t.Error("string for boolean should fail")
	}
}

func TestValidateFieldValueDate(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldDate}
	if err := validateFieldValue(f, "2024-01-15"); err != nil {
		t.Errorf("valid date: %v", err)
	}
	if err := validateFieldValue(f, "01-15-2024"); err == nil {
		t.Error("wrong format should fail")
	}
	if err := validateFieldValue(f, 12345.0); err == nil {
		t.Error("number for date should fail")
	}
}

func TestValidateFieldValueDatetime(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldDatetime}
	if err := validateFieldValue(f, "2024-01-15T10:30:00Z"); err != nil {
		t.Errorf("valid datetime: %v", err)
	}
	if err := validateFieldValue(f, "2024-01-15"); err == nil {
		t.Error("date-only for datetime should fail")
	}
}

func TestValidateFieldValueTime(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldTime}
	cases := []struct {
		val string
		ok  bool
	}{
		{"09:30", true},
		{"23:59:59", true},
		{"00:00", true},
		{"24:00", false},
		{"9:30", false},
		{"abc", false},
	}
	for _, tc := range cases {
		err := validateFieldValue(f, tc.val)
		if tc.ok && err != nil {
			t.Errorf("time %q should be valid: %v", tc.val, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("time %q should be invalid", tc.val)
		}
	}
}

func TestValidateFieldValueSelect(t *testing.T) {
	opts, _ := json.Marshal(schema.SelectOptions{Choices: []string{"a", "b", "c"}})
	f := schema.Field{FieldType: schema.FieldSelect, Options: opts}

	if err := validateFieldValue(f, "a"); err != nil {
		t.Errorf("valid choice: %v", err)
	}
	if err := validateFieldValue(f, "d"); err == nil {
		t.Error("invalid choice should fail")
	}
	if err := validateFieldValue(f, 123.0); err == nil {
		t.Error("number for select should fail")
	}
}

func TestValidateFieldValueMultiselect(t *testing.T) {
	opts, _ := json.Marshal(schema.SelectOptions{Choices: []string{"x", "y", "z"}})
	f := schema.Field{FieldType: schema.FieldMultiselect, Options: opts}

	if err := validateFieldValue(f, []any{"x", "z"}); err != nil {
		t.Errorf("valid multiselect: %v", err)
	}
	if err := validateFieldValue(f, []any{"x", "invalid"}); err == nil {
		t.Error("invalid choice in multiselect should fail")
	}
	if err := validateFieldValue(f, "x"); err == nil {
		t.Error("string for multiselect should fail")
	}
	if err := validateFieldValue(f, []any{123}); err == nil {
		t.Error("non-string item should fail")
	}
}

func TestValidateFieldValueRelation(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldRelation}
	if err := validateFieldValue(f, "550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Errorf("valid UUID: %v", err)
	}
	if err := validateFieldValue(f, "not-a-uuid"); err == nil {
		t.Error("invalid UUID should fail")
	}
	if err := validateFieldValue(f, 123.0); err == nil {
		t.Error("number for relation should fail")
	}
}

func TestValidateFieldValueJSON(t *testing.T) {
	f := schema.Field{FieldType: schema.FieldJSON}
	if err := validateFieldValue(f, map[string]any{"key": "val"}); err != nil {
		t.Errorf("JSON object: %v", err)
	}
	if err := validateFieldValue(f, "string"); err != nil {
		t.Errorf("JSON string: %v", err)
	}
}

func TestValidateFieldValueUnknownType(t *testing.T) {
	f := schema.Field{FieldType: "unknown"}
	err := validateFieldValue(f, "val")
	if err == nil {
		t.Error("unknown type should fail")
	}
	if !errors.Is(err, schema.ErrInvalidInput) {
		t.Errorf("error should wrap ErrInvalidInput, got: %v", err)
	}
}
