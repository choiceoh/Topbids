package schema

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug    string
		wantErr bool
	}{
		{"projects", false},
		{"my_collection", false},
		{"a", false},
		{"a1", false},
		{"a_1_b_2", false},

		{"", true},                       // empty
		{"1abc", true},                   // starts with digit
		{"_abc", true},                   // starts with underscore
		{"ABC", true},                    // uppercase
		{"abc-def", true},                // dash not allowed
		{"abc.def", true},                // dot not allowed
		{"select", true},                 // PG reserved
		{"id", true},                     // auto column
		{"created_at", true},             // auto column
		{"deleted_at", true},             // auto column
		{string(make([]byte, 64)), true}, // > 63 chars
	}
	for _, tc := range cases {
		err := ValidateSlug(tc.slug)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateSlug(%q) err=%v, wantErr=%v", tc.slug, err, tc.wantErr)
		}
	}
}

func TestValidateFieldType(t *testing.T) {
	good := []FieldType{FieldText, FieldNumber, FieldInteger, FieldBoolean,
		FieldDate, FieldDatetime, FieldSelect, FieldMultiselect,
		FieldRelation, FieldFile, FieldJSON}
	for _, ft := range good {
		if err := ValidateFieldType(ft); err != nil {
			t.Errorf("ValidateFieldType(%q) unexpectedly failed: %v", ft, err)
		}
	}
	if err := ValidateFieldType("nonsense"); err == nil {
		t.Error("ValidateFieldType should reject unknown type")
	}
}

func TestValidateCollectionCreate_DuplicateFieldSlug(t *testing.T) {
	req := &CreateCollectionReq{
		Slug:  "projects",
		Label: "Projects",
		Fields: []CreateFieldIn{
			{Slug: "name", Label: "Name", FieldType: FieldText},
			{Slug: "name", Label: "Name 2", FieldType: FieldText},
		},
	}
	err := ValidateCollectionCreate(req)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateCollectionCreate_MissingLabel(t *testing.T) {
	req := &CreateCollectionReq{Slug: "projects"}
	err := ValidateCollectionCreate(req)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateCollectionCreate_Valid(t *testing.T) {
	req := &CreateCollectionReq{
		Slug:  "projects",
		Label: "프로젝트",
		Fields: []CreateFieldIn{
			{Slug: "name", Label: "이름", FieldType: FieldText, IsRequired: true},
			{Slug: "capacity", Label: "용량", FieldType: FieldNumber},
			{Slug: "status", Label: "상태", FieldType: FieldSelect,
				Options: json.RawMessage(`{"choices": ["기획","시공","운영"]}`)},
		},
	}
	if err := ValidateCollectionCreate(req); err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidateRelationOnDelete(t *testing.T) {
	good := []string{"", "CASCADE", "set null", "RESTRICT", "no action", "SET DEFAULT"}
	for _, od := range good {
		f := &CreateFieldIn{
			Slug:      "owner",
			Label:     "소유자",
			FieldType: FieldRelation,
			Relation: &CreateRelIn{
				TargetCollectionID: "00000000-0000-0000-0000-000000000001",
				RelationType:       RelOneToMany,
				OnDelete:           od,
			},
		}
		if err := validateFieldIn(f); err != nil {
			t.Errorf("on_delete %q should be valid: %v", od, err)
		}
	}

	bad := []string{"DROP TABLE", "CASCADE; DELETE", "INVALID"}
	for _, od := range bad {
		f := &CreateFieldIn{
			Slug:      "owner",
			Label:     "소유자",
			FieldType: FieldRelation,
			Relation: &CreateRelIn{
				TargetCollectionID: "00000000-0000-0000-0000-000000000001",
				RelationType:       RelOneToMany,
				OnDelete:           od,
			},
		}
		if err := validateFieldIn(f); err == nil {
			t.Errorf("on_delete %q should be rejected", od)
		}
	}
}

func TestValidateSelectOptions(t *testing.T) {
	cases := []struct {
		opts    string
		wantErr bool
	}{
		{`{"choices":["a","b","c"]}`, false},
		{`{"choices":["one"]}`, false},

		{`{"choices":[]}`, true},              // empty
		{`{}`, true},                          // missing choices
		{`null`, true},                        // null
		{``, true},                            // empty
		{`{"choices":["a","a"]}`, true},       // duplicate
		{`{"choices":["a",""]}`, true},        // empty value
		{`{"choices": "not an array"}`, true}, // wrong type
		{`not json`, true},                    // invalid JSON
	}
	for _, tc := range cases {
		err := validateSelectOptions(json.RawMessage(tc.opts))
		if (err != nil) != tc.wantErr {
			t.Errorf("validateSelectOptions(%q): err=%v, wantErr=%v", tc.opts, err, tc.wantErr)
		}
	}
}

func TestExtractChoices(t *testing.T) {
	choices, err := ExtractChoices(json.RawMessage(`{"choices":["a","b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 2 || choices[0] != "a" || choices[1] != "b" {
		t.Errorf("got %v", choices)
	}

	choices, err = ExtractChoices(json.RawMessage(``))
	if err != nil || choices != nil {
		t.Errorf("empty input should return (nil, nil)")
	}
}
