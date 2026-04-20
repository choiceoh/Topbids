package migration

import (
	"testing"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

func TestCheckCompat(t *testing.T) {
	cases := []struct {
		from, to        schema.FieldType
		wantAllowed     bool
		wantConditional bool
	}{
		// Always-safe widening / serialization conversions
		{schema.FieldNumber, schema.FieldText, true, false},
		{schema.FieldInteger, schema.FieldNumber, true, false},
		{schema.FieldInteger, schema.FieldText, true, false},
		{schema.FieldBoolean, schema.FieldText, true, false},
		{schema.FieldBoolean, schema.FieldInteger, true, false},
		{schema.FieldDate, schema.FieldText, true, false},
		{schema.FieldDate, schema.FieldDatetime, true, false},
		{schema.FieldDatetime, schema.FieldDate, true, false},
		{schema.FieldSelect, schema.FieldText, true, false},
		{schema.FieldSelect, schema.FieldMultiselect, true, false},

		// Conditional conversions need data validation
		{schema.FieldText, schema.FieldNumber, true, true},
		{schema.FieldText, schema.FieldInteger, true, true},
		{schema.FieldText, schema.FieldDate, true, true},
		{schema.FieldText, schema.FieldBoolean, true, true},
		{schema.FieldNumber, schema.FieldInteger, true, true},
		{schema.FieldMultiselect, schema.FieldSelect, true, true},

		// Forbidden / unsupported
		{schema.FieldJSON, schema.FieldText, false, false},
		{schema.FieldRelation, schema.FieldText, false, false},
		{schema.FieldFile, schema.FieldText, false, false},
		{schema.FieldText, schema.FieldRelation, false, false},
	}

	for _, tc := range cases {
		allowed, conditional := CheckCompat(tc.from, tc.to)
		if allowed != tc.wantAllowed || conditional != tc.wantConditional {
			t.Errorf("CheckCompat(%s→%s) = (allowed=%v, cond=%v), want (allowed=%v, cond=%v)",
				tc.from, tc.to, allowed, conditional, tc.wantAllowed, tc.wantConditional)
		}
	}
}

func TestSameTypeIsForbidden(t *testing.T) {
	// Same-type "conversion" should not be in the matrix.
	allowed, _ := CheckCompat(schema.FieldText, schema.FieldText)
	if allowed {
		t.Error("same-type conversion should not appear in matrix (caller skips it)")
	}
}
