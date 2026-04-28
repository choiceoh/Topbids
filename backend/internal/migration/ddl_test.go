package migration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

func TestFieldTypeToPG(t *testing.T) {
	cases := map[schema.FieldType]string{
		schema.FieldText:        "TEXT",
		schema.FieldNumber:      "NUMERIC",
		schema.FieldInteger:     "INTEGER",
		schema.FieldBoolean:     "BOOLEAN",
		schema.FieldDate:        "DATE",
		schema.FieldDatetime:    "TIMESTAMPTZ",
		schema.FieldSelect:      "VARCHAR(255)",
		schema.FieldMultiselect: "TEXT[]",
		schema.FieldRelation:    "UUID",
		schema.FieldFile:        "UUID",
		schema.FieldJSON:        "JSONB",
	}
	for ft, want := range cases {
		if got := FieldTypeToPG(ft); got != want {
			t.Errorf("FieldTypeToPG(%s) = %q, want %q", ft, got, want)
		}
	}
}

func TestSanitizeOnDelete(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"CASCADE", "CASCADE"},
		{"cascade", "CASCADE"},
		{"  set null  ", "SET NULL"},
		{"RESTRICT", "RESTRICT"},
		{"NO ACTION", "NO ACTION"},
		{"SET DEFAULT", "SET DEFAULT"},
		{"", "SET NULL"},
		{"DROP TABLE users", "SET NULL"},
		{"CASCADE; DELETE", "SET NULL"},
	}
	for _, tc := range cases {
		if got := SanitizeOnDelete(tc.in); got != tc.want {
			t.Errorf("SanitizeOnDelete(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGenerateCreateTable(t *testing.T) {
	col := schema.Collection{Slug: "projects", Label: "프로젝트"}
	fields := []schema.Field{
		{Slug: "name", FieldType: schema.FieldText, IsRequired: true},
		{Slug: "capacity", FieldType: schema.FieldNumber, IsIndexed: true},
		{Slug: "code", FieldType: schema.FieldText, IsUnique: true},
	}
	up, down := GenerateCreateTable(col, fields)

	if len(up) < 2 {
		t.Fatalf("expected at least 2 statements (CREATE TABLE + index), got %d", len(up))
	}

	create := up[0]
	for _, want := range []string{
		`"data"."projects"`,
		`"name" TEXT NOT NULL`,
		`"capacity" NUMERIC`,
		`"code" TEXT UNIQUE`,
		`id UUID PRIMARY KEY DEFAULT gen_random_uuid()`,
		`created_at TIMESTAMPTZ`,
		`deleted_at TIMESTAMPTZ`,
	} {
		if !strings.Contains(create, want) {
			t.Errorf("CREATE TABLE missing %q\nfull:\n%s", want, create)
		}
	}

	// At least one CREATE INDEX statement
	foundIdx := false
	for _, stmt := range up[1:] {
		if strings.Contains(stmt, "CREATE INDEX") && strings.Contains(stmt, `"capacity"`) {
			foundIdx = true
			break
		}
	}
	if !foundIdx {
		t.Errorf("missing CREATE INDEX for capacity")
	}

	if len(down) != 1 || !strings.Contains(down[0], "DROP TABLE") {
		t.Errorf("expected DROP TABLE down statement, got %v", down)
	}
}

func TestGenerateAddColumnWithDefault(t *testing.T) {
	f := schema.Field{
		Slug:         "status",
		FieldType:    schema.FieldText,
		IsRequired:   true,
		DefaultValue: json.RawMessage(`"기획"`),
	}
	up, down := GenerateAddColumn("projects", f)
	if len(up) != 1 {
		t.Fatalf("expected 1 up stmt, got %d", len(up))
	}
	stmt := up[0]
	if !strings.Contains(stmt, "ADD COLUMN") || !strings.Contains(stmt, `"status"`) {
		t.Errorf("missing ADD COLUMN: %s", stmt)
	}
	if !strings.Contains(stmt, "NOT NULL") {
		t.Errorf("missing NOT NULL: %s", stmt)
	}
	if !strings.Contains(stmt, "DEFAULT '기획'") {
		t.Errorf("missing DEFAULT: %s", stmt)
	}
	if len(down) != 1 || !strings.Contains(down[0], "DROP COLUMN") {
		t.Errorf("expected DROP COLUMN down, got %v", down)
	}
}

func TestGenerateAddColumnUniqueIndex(t *testing.T) {
	f := schema.Field{Slug: "code", FieldType: schema.FieldText, IsUnique: true, IsIndexed: true}
	up, _ := GenerateAddColumn("projects", f)
	if len(up) != 3 {
		t.Fatalf("expected 3 stmts (add + unique + index), got %d:\n%v", len(up), up)
	}
}

func TestGenerateAlterColumnType(t *testing.T) {
	up, down := GenerateAlterColumnType("projects", "capacity", schema.FieldNumber, schema.FieldInteger)
	if len(up) != 1 || !strings.Contains(up[0], "::INTEGER") {
		t.Errorf("up missing INTEGER cast: %v", up)
	}
	if len(down) != 1 || !strings.Contains(down[0], "NUMERIC") {
		t.Errorf("down missing NUMERIC: %v", down)
	}
}

func TestDefaultClauseTypes(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`"hello"`, "'hello'"},
		{`42`, "42"},
		{`3.14`, "3.14"},
		{`true`, "TRUE"},
		{`false`, "FALSE"},
		{`null`, ""},
		{``, ""},
	}
	for _, tc := range cases {
		f := schema.Field{DefaultValue: json.RawMessage(tc.raw)}
		got := defaultClause(f)
		if got != tc.want {
			t.Errorf("defaultClause(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestEscapeSQLString(t *testing.T) {
	if got := escapeSQLString("O'Brien"); got != "O''Brien" {
		t.Errorf("got %q", got)
	}
}

func TestGenerateAddFKSanitizes(t *testing.T) {
	up, _ := GenerateAddFK("projects", "owner", "users", "DROP TABLE users; --")
	if !strings.Contains(up, "ON DELETE SET NULL") {
		t.Errorf("expected ON DELETE SET NULL fallback, got: %s", up)
	}
	if strings.Contains(up, "DROP TABLE") {
		t.Errorf("injection slipped through: %s", up)
	}
}
