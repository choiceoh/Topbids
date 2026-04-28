package migration

import (
	"encoding/json"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

func TestClassifyAddField(t *testing.T) {
	cases := []struct {
		name string
		req  schema.CreateFieldIn
		want SafetyLevel
	}{
		{
			name: "nullable text",
			req:  schema.CreateFieldIn{FieldType: schema.FieldText},
			want: Safe,
		},
		{
			name: "required with default",
			req: schema.CreateFieldIn{
				FieldType:    schema.FieldText,
				IsRequired:   true,
				DefaultValue: json.RawMessage(`"hello"`),
			},
			want: Safe,
		},
		{
			name: "required without default",
			req: schema.CreateFieldIn{
				FieldType:  schema.FieldText,
				IsRequired: true,
			},
			want: Cautious,
		},
	}
	for _, tc := range cases {
		got := ClassifyAddField(&tc.req)
		if got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestClassifyAlterField(t *testing.T) {
	old := schema.Field{
		Slug:       "name",
		FieldType:  schema.FieldText,
		IsRequired: false,
	}

	// type change is always dangerous
	newType := schema.FieldNumber
	got := ClassifyAlterField(old, &schema.UpdateFieldReq{FieldType: &newType})
	if got != Dangerous {
		t.Errorf("type change should be Dangerous, got %s", got)
	}

	// tightening to required is cautious
	required := true
	got = ClassifyAlterField(old, &schema.UpdateFieldReq{IsRequired: &required})
	if got != Cautious {
		t.Errorf("tightening required should be Cautious, got %s", got)
	}

	// loosening from required is safe
	oldReq := old
	oldReq.IsRequired = true
	notRequired := false
	got = ClassifyAlterField(oldReq, &schema.UpdateFieldReq{IsRequired: &notRequired})
	if got != Safe {
		t.Errorf("loosening required should be Safe, got %s", got)
	}

	// label-only change is safe
	label := "New Label"
	got = ClassifyAlterField(old, &schema.UpdateFieldReq{Label: &label})
	if got != Safe {
		t.Errorf("label change should be Safe, got %s", got)
	}
}
