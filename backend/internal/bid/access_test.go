package bid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

// --- Fixtures ---

var deadline = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func sealedField(slug, untilAt string, unlockStatuses ...string) schema.Field {
	opts := map[string]any{}
	if untilAt != "" {
		opts["sealed_until_at"] = untilAt
	}
	if len(unlockStatuses) > 0 {
		opts["unlock_by_status"] = unlockStatuses
	}
	raw, _ := json.Marshal(opts)
	return schema.Field{
		Slug:      slug,
		FieldType: schema.FieldNumber,
		Options:   raw,
	}
}

func plainField(slug string) schema.Field {
	return schema.Field{Slug: slug, FieldType: schema.FieldText}
}

// --- MaskSealedFields ---

func TestMaskSealedFields_AdminAlwaysUnlocked(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "2099-01-01T00:00:00Z", "opened")}
	row := map[string]any{"amount": 1000, "status": "작성중"}
	MaskSealedFields(fields, row, "status", "admin", deadline)
	if row["amount"] != 1000 {
		t.Errorf("admin should bypass masking; got amount=%v", row["amount"])
	}

	row2 := map[string]any{"amount": 2000}
	MaskSealedFields(fields, row2, "status", "director", deadline)
	if row2["amount"] != 2000 {
		t.Errorf("director should bypass masking")
	}
}

func TestMaskSealedFields_StatusUnlock(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "2099-01-01T00:00:00Z", "opened", "awarded")}

	for _, status := range []string{"opened", "awarded"} {
		row := map[string]any{"amount": 1000, "status": status}
		MaskSealedFields(fields, row, "status", "pm", deadline)
		if row["amount"] != 1000 {
			t.Errorf("status %q should unlock; got amount=%v", status, row["amount"])
		}
	}

	for _, status := range []string{"작성중", "공고중"} {
		row := map[string]any{"amount": 1000, "status": status}
		MaskSealedFields(fields, row, "status", "pm", deadline)
		if row["amount"] != nil {
			t.Errorf("status %q should remain sealed; got amount=%v", status, row["amount"])
		}
	}
}

func TestMaskSealedFields_TimeUnlock_Absolute(t *testing.T) {
	pastTime := "2026-04-01T00:00:00Z"
	futureTime := "2027-01-01T00:00:00Z"

	fields := []schema.Field{sealedField("amount", pastTime)}
	row := map[string]any{"amount": 1000}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != 1000 {
		t.Errorf("past sealed_until_at should unlock; got %v", row["amount"])
	}

	fields = []schema.Field{sealedField("amount", futureTime)}
	row = map[string]any{"amount": 2000}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("future sealed_until_at should remain sealed; got %v", row["amount"])
	}
}

func TestMaskSealedFields_TimeUnlock_FieldRef_SameRow(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "field:open_at")}

	// Open time in the past → unlocked.
	row := map[string]any{"amount": 1000, "open_at": "2026-04-01T00:00:00Z"}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != 1000 {
		t.Errorf("past open_at should unlock")
	}

	// Open time in the future → sealed.
	row = map[string]any{"amount": 1000, "open_at": "2027-01-01T00:00:00Z"}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("future open_at should seal")
	}
}

func TestMaskSealedFields_TimeUnlock_FieldRef_CrossCollection(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "field:rfq.open_at")}

	// Relation expanded with past open_at.
	row := map[string]any{
		"amount": 1000,
		"rfq": map[string]any{
			"open_at": "2026-04-01T00:00:00Z",
		},
	}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != 1000 {
		t.Errorf("cross-collection past open_at should unlock")
	}

	// Future.
	row = map[string]any{
		"amount": 1000,
		"rfq": map[string]any{
			"open_at": "2027-01-01T00:00:00Z",
		},
	}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("cross-collection future open_at should seal")
	}
}

func TestMaskSealedFields_UnresolvedRefFailsClosed(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "field:rfq.open_at")}

	// Relation not expanded (no rfq key).
	row := map[string]any{"amount": 1000}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("unresolved field ref should mask (fail closed); got %v", row["amount"])
	}

	// Relation is nil.
	row = map[string]any{"amount": 1000, "rfq": nil}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("nil relation should mask")
	}

	// Malformed until_at (not RFC3339 AND not field: prefix bypasses validator
	// at read time because we're lenient — but resolveUntilAt should fail).
	// Note: such values shouldn't reach here if validation ran, but we fail closed.
}

func TestMaskSealedFields_NonSealedFieldsUntouched(t *testing.T) {
	fields := []schema.Field{
		sealedField("amount", "2099-01-01T00:00:00Z"),
		plainField("title"),
		plainField("note"),
	}
	row := map[string]any{"amount": 1000, "title": "항상 보임", "note": "메모"}
	MaskSealedFields(fields, row, "", "pm", deadline)
	if row["amount"] != nil {
		t.Errorf("sealed field should mask")
	}
	if row["title"] != "항상 보임" {
		t.Errorf("plain text field should not mask")
	}
	if row["note"] != "메모" {
		t.Errorf("plain text field should not mask")
	}
}

func TestMaskSealedFields_StatusPrecedenceOverTime(t *testing.T) {
	// unlock_by_status matches even if sealed_until_at is far in the future.
	fields := []schema.Field{sealedField("amount", "2099-01-01T00:00:00Z", "opened")}
	row := map[string]any{"amount": 1000, "status": "opened"}
	MaskSealedFields(fields, row, "status", "pm", deadline)
	if row["amount"] != 1000 {
		t.Errorf("status unlock should bypass future sealed_until_at")
	}
}

// --- resolveUntilAt ---

func TestResolveUntilAt_AbsoluteTime(t *testing.T) {
	tm, ok := resolveUntilAt("2026-05-01T00:00:00Z", nil)
	if !ok {
		t.Fatal("valid RFC3339 should parse")
	}
	expected := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !tm.Equal(expected) {
		t.Errorf("got %v, want %v", tm, expected)
	}
}

func TestResolveUntilAt_FieldRefTimeValue(t *testing.T) {
	row := map[string]any{"open_at": time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	tm, ok := resolveUntilAt("field:open_at", row)
	if !ok {
		t.Fatal("time.Time value should resolve")
	}
	if tm.Year() != 2026 || tm.Month() != 6 {
		t.Errorf("got %v", tm)
	}
}

func TestResolveUntilAt_MalformedReturnsFalse(t *testing.T) {
	cases := []struct {
		name string
		v    string
		row  map[string]any
	}{
		{"invalid RFC3339", "2026-05-01", nil},
		{"empty", "", nil},
		{"field ref missing key", "field:open_at", map[string]any{}},
		{"field ref nil value", "field:open_at", map[string]any{"open_at": nil}},
		{"cross ref intermediate not map", "field:rfq.open_at",
			map[string]any{"rfq": "not-a-map"}},
		{"deep path missing", "field:a.b.c", map[string]any{"a": map[string]any{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := resolveUntilAt(tc.v, tc.row); ok {
				t.Error("expected resolution failure")
			}
		})
	}
}

// --- toTime ---

// --- SupplierRowFilter ---

func TestSupplierRowFilter(t *testing.T) {
	cases := []struct {
		name       string
		role       string
		supplierID string
		startIdx   int
		wantActive bool
		wantSQL    string
		wantArgs   int
	}{
		{"director skips", "director", "sup-123", 1, false, "", 0},
		{"pm skips", "pm", "sup-123", 1, false, "", 0},
		{"viewer skips", "viewer", "sup-123", 1, false, "", 0},
		{"supplier with id → filter", "supplier", "sup-123", 1, true, "supplier = $1", 1},
		{"supplier with id later idx", "supplier", "sup-456", 5, true, "supplier = $5", 1},
		{"supplier without id fails closed", "supplier", "", 1, true, "1=0", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, active := SupplierRowFilter(tc.role, tc.supplierID, tc.startIdx)
			if active != tc.wantActive {
				t.Errorf("active=%v, want %v", active, tc.wantActive)
			}
			if sql != tc.wantSQL {
				t.Errorf("sql=%q, want %q", sql, tc.wantSQL)
			}
			if len(args) != tc.wantArgs {
				t.Errorf("args len=%d, want %d", len(args), tc.wantArgs)
			}
		})
	}
}

// --- MaskSealedFields bypass for supplier ---

func TestMaskSealedFields_SupplierBypassesMask(t *testing.T) {
	// Supplier viewing own row (upstream filter already applied) — fields
	// should remain visible even with future sealed_until_at.
	fields := []schema.Field{sealedField("amount", "2099-01-01T00:00:00Z")}
	row := map[string]any{"amount": 1000, "status": "submitted"}
	MaskSealedFields(fields, row, "status", "supplier", deadline)
	if row["amount"] != 1000 {
		t.Errorf("supplier should see own sealed values; got %v", row["amount"])
	}
}

func TestToTime(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	if tm, ok := toTime(now); !ok || !tm.Equal(now) {
		t.Errorf("time.Time passthrough failed: ok=%v tm=%v", ok, tm)
	}

	s := "2026-05-01T12:34:56Z"
	tm, ok := toTime(s)
	if !ok {
		t.Fatal("RFC3339 string should parse")
	}
	if tm.Hour() != 12 || tm.Minute() != 34 {
		t.Errorf("parsed wrong: %v", tm)
	}

	if _, ok := toTime(12345); ok {
		t.Error("int should not parse as time")
	}
	if _, ok := toTime(nil); ok {
		t.Error("nil should not parse")
	}
}

// --- IsRowOpened ---

func TestIsRowOpened_StatusInUnlockList(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "", "opened", "awarded")}

	if !IsRowOpened(fields, map[string]any{"status": "opened"}, "status") {
		t.Error("status=opened should report row as opened")
	}
	if !IsRowOpened(fields, map[string]any{"status": "awarded"}, "status") {
		t.Error("status=awarded should report row as opened")
	}
}

func TestIsRowOpened_StatusNotInList(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "", "opened")}
	if IsRowOpened(fields, map[string]any{"status": "submitted"}, "status") {
		t.Error("status=submitted should NOT report row as opened")
	}
}

func TestIsRowOpened_NoSealedFields(t *testing.T) {
	fields := []schema.Field{plainField("name")}
	if IsRowOpened(fields, map[string]any{"status": "opened"}, "status") {
		t.Error("collections without sealed fields should never report opened")
	}
}

func TestIsRowOpened_MissingStatus(t *testing.T) {
	fields := []schema.Field{sealedField("amount", "", "opened")}
	if IsRowOpened(fields, map[string]any{}, "status") {
		t.Error("missing status should NOT report opened")
	}
	if IsRowOpened(fields, map[string]any{"status": nil}, "status") {
		t.Error("nil status should NOT report opened")
	}
}
