package migration

import (
	"strings"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

func TestGenerateCreateTable_Basic(t *testing.T) {
	col := schema.Collection{Slug: "tasks"}
	fields := []schema.Field{
		{Slug: "title", FieldType: schema.FieldText, IsRequired: true},
		{Slug: "done", FieldType: schema.FieldBoolean},
	}

	up, down := GenerateCreateTable(col, fields)
	if len(up) == 0 {
		t.Fatal("expected up statements")
	}
	if len(down) == 0 {
		t.Fatal("expected down statements")
	}
	// Should contain CREATE TABLE.
	found := false
	for _, s := range up {
		if strings.Contains(s, "CREATE TABLE") {
			found = true
			if !strings.Contains(s, `"title"`) {
				t.Error("missing title column")
			}
			if !strings.Contains(s, `"done"`) {
				t.Error("missing done column")
			}
		}
	}
	if !found {
		t.Fatal("no CREATE TABLE statement found")
	}
	if !strings.Contains(down[0], "DROP TABLE") {
		t.Fatal("down should contain DROP TABLE")
	}
}

func TestGenerateCreateTable_WithTSVector(t *testing.T) {
	col := schema.Collection{Slug: "articles"}
	fields := []schema.Field{
		{Slug: "title", FieldType: schema.FieldText},
		{Slug: "body", FieldType: schema.FieldTextarea},
		{Slug: "count", FieldType: schema.FieldInteger},
	}

	up, _ := GenerateCreateTable(col, fields)

	// Should have _tsv column, GIN index, trigger function, and trigger.
	var hasTSV, hasGIN, hasFunc, hasTrigger bool
	for _, s := range up {
		if strings.Contains(s, "_tsv TSVECTOR") {
			hasTSV = true
		}
		if strings.Contains(s, "USING GIN (_tsv)") {
			hasGIN = true
		}
		if strings.Contains(s, "to_tsvector") {
			hasFunc = true
		}
		if strings.Contains(s, "CREATE TRIGGER") {
			hasTrigger = true
		}
	}
	if !hasTSV {
		t.Error("missing _tsv column")
	}
	if !hasGIN {
		t.Error("missing GIN index")
	}
	if !hasFunc {
		t.Error("missing tsvector function")
	}
	if !hasTrigger {
		t.Error("missing trigger")
	}
}

func TestGenerateCreateTable_NoTSVWithoutTextFields(t *testing.T) {
	col := schema.Collection{Slug: "numbers"}
	fields := []schema.Field{
		{Slug: "amount", FieldType: schema.FieldNumber},
		{Slug: "active", FieldType: schema.FieldBoolean},
	}

	up, _ := GenerateCreateTable(col, fields)
	for _, s := range up {
		if strings.Contains(s, "_tsv") {
			t.Error("should not add _tsv without text fields")
		}
	}
}

func TestGenerateCreateTable_LayoutFieldsSkipped(t *testing.T) {
	col := schema.Collection{Slug: "form"}
	fields := []schema.Field{
		{Slug: "name", FieldType: schema.FieldText},
		{Slug: "sep", FieldType: schema.FieldLine},
		{Slug: "spacer1", FieldType: schema.FieldSpacer},
		{Slug: "label1", FieldType: schema.FieldLabel},
	}

	up, _ := GenerateCreateTable(col, fields)
	for _, s := range up {
		if strings.Contains(s, "CREATE TABLE") {
			if strings.Contains(s, `"sep"`) || strings.Contains(s, `"spacer1"`) || strings.Contains(s, `"label1"`) {
				t.Error("layout fields should not appear in CREATE TABLE")
			}
		}
	}
}

func TestGenerateCreateTable_ComputedFieldsSkipped(t *testing.T) {
	col := schema.Collection{Slug: "calc"}
	fields := []schema.Field{
		{Slug: "value", FieldType: schema.FieldNumber},
		{Slug: "total", FieldType: schema.FieldFormula},
		{Slug: "ref_name", FieldType: schema.FieldLookup},
		{Slug: "sum_val", FieldType: schema.FieldRollup},
	}

	up, _ := GenerateCreateTable(col, fields)
	for _, s := range up {
		if strings.Contains(s, "CREATE TABLE") {
			if strings.Contains(s, `"total"`) || strings.Contains(s, `"ref_name"`) || strings.Contains(s, `"sum_val"`) {
				t.Error("computed fields should not appear in CREATE TABLE")
			}
		}
	}
}

func TestGenerateAddColumn_WithIndex(t *testing.T) {
	f := schema.Field{Slug: "email", FieldType: schema.FieldText, IsIndexed: true}
	up, down := GenerateAddColumn("users", f)
	if len(up) != 2 {
		t.Fatalf("expected 2 up stmts (ALTER + INDEX), got %d", len(up))
	}
	if !strings.Contains(up[1], "CREATE INDEX") {
		t.Error("second statement should be CREATE INDEX")
	}
	if len(down) != 1 {
		t.Fatalf("expected 1 down stmt, got %d", len(down))
	}
}

func TestGenerateDropColumn(t *testing.T) {
	f := schema.Field{Slug: "old_col", FieldType: schema.FieldText}
	up, down := GenerateDropColumn("projects", f)
	if len(up) != 1 || !strings.Contains(up[0], "DROP COLUMN") {
		t.Error("expected DROP COLUMN")
	}
	if len(down) != 1 || !strings.Contains(down[0], "ADD COLUMN") {
		t.Error("expected ADD COLUMN in down")
	}
}

func TestGenerateStatusColumn(t *testing.T) {
	up, down := GenerateAddStatusColumn("orders")
	if len(up) != 1 || !strings.Contains(up[0], "_status") {
		t.Error("expected _status column in up")
	}
	if len(down) != 1 || !strings.Contains(down[0], "_status") {
		t.Error("expected _status column in down")
	}
}

func TestGenerateAlterColumnType_NumberToInteger(t *testing.T) {
	up, down := GenerateAlterColumnType("t", "c", schema.FieldNumber, schema.FieldInteger)
	if len(up) != 1 || !strings.Contains(up[0], "TYPE INTEGER") {
		t.Errorf("expected TYPE INTEGER, got %s", up[0])
	}
	if len(down) != 1 || !strings.Contains(down[0], "TYPE NUMERIC") {
		t.Errorf("expected TYPE NUMERIC, got %s", down[0])
	}
}

func TestGenerateJunctionTable(t *testing.T) {
	up, down := GenerateJunctionTable("users", "roles", "users_roles")
	if !strings.Contains(up, "CREATE TABLE") {
		t.Error("expected CREATE TABLE")
	}
	if !strings.Contains(up, "users_id") {
		t.Error("expected users_id column")
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Error("expected DROP TABLE in down")
	}
}

func TestGenerateCreateTable_Autonumber(t *testing.T) {
	col := schema.Collection{Slug: "invoices"}
	fields := []schema.Field{
		{Slug: "invoice_no", FieldType: schema.FieldAutonumber},
	}

	up, _ := GenerateCreateTable(col, fields)
	var hasSeq, hasNextval bool
	for _, s := range up {
		if strings.Contains(s, "CREATE SEQUENCE") {
			hasSeq = true
		}
		if strings.Contains(s, "nextval") {
			hasNextval = true
		}
	}
	if !hasSeq {
		t.Error("missing CREATE SEQUENCE for autonumber")
	}
	if !hasNextval {
		t.Error("missing nextval default for autonumber")
	}
}
