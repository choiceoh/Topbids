package migration

import (
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// ClassifyAddField determines the safety level for adding a new field.
func ClassifyAddField(f *schema.CreateFieldIn) SafetyLevel {
	if f.IsRequired && len(f.DefaultValue) == 0 {
		return Cautious // existing rows need a value
	}
	return Safe
}

// ClassifyAlterField determines the safety level when changing field properties.
func ClassifyAlterField(old schema.Field, req *schema.UpdateFieldReq) SafetyLevel {
	level := Safe

	// Type change is always dangerous.
	if req.FieldType != nil && *req.FieldType != old.FieldType {
		return Dangerous
	}

	// Tightening NOT NULL on an existing column.
	if req.IsRequired != nil && *req.IsRequired && !old.IsRequired {
		level = Cautious
	}

	return level
}

// Note: drop_field and drop_collection are hardcoded as Dangerous at the call site
// (see engine.DropField / engine.DropCollection); no classifier helper is needed.
