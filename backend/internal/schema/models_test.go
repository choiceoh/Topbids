package schema

import "testing"

func TestFieldTypeValid(t *testing.T) {
	valid := []FieldType{
		FieldText, FieldTextarea, FieldNumber, FieldInteger,
		FieldBoolean, FieldDate, FieldDatetime, FieldTime,
		FieldSelect, FieldMultiselect, FieldRelation,
		FieldFile, FieldJSON, FieldUser, FieldAutonumber,
		FieldLabel, FieldLine, FieldSpacer,
	}
	for _, ft := range valid {
		if !ft.Valid() {
			t.Errorf("%q should be valid", ft)
		}
	}
	if FieldType("unknown").Valid() {
		t.Error("unknown field type should be invalid")
	}
}

func TestFieldTypeIsLayout(t *testing.T) {
	layout := []FieldType{FieldLabel, FieldLine, FieldSpacer}
	for _, ft := range layout {
		if !ft.IsLayout() {
			t.Errorf("%q should be layout", ft)
		}
	}
	nonLayout := []FieldType{FieldText, FieldNumber, FieldBoolean, FieldRelation}
	for _, ft := range nonLayout {
		if ft.IsLayout() {
			t.Errorf("%q should not be layout", ft)
		}
	}
}

func TestAccessConfigAllowsRole(t *testing.T) {
	ac := AccessConfig{
		EntryView:   []string{"admin", "editor"},
		EntryCreate: []string{"admin"},
		EntryEdit:   nil, // empty = all allowed
	}

	cases := []struct {
		op   string
		role string
		want bool
	}{
		{"entry_view", "admin", true},
		{"entry_view", "editor", true},
		{"entry_view", "viewer", false},
		{"entry_create", "admin", true},
		{"entry_create", "editor", false},
		{"entry_edit", "anyone", true},  // empty = all allowed
		{"entry_delete", "admin", true}, // empty = all allowed
		{"invalid_op", "admin", false},
	}
	for _, tc := range cases {
		got := ac.AllowsRole(tc.op, tc.role)
		if got != tc.want {
			t.Errorf("AllowsRole(%q, %q) = %v, want %v", tc.op, tc.role, got, tc.want)
		}
	}
}
