package migration

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// FieldTypeToPG maps a schema field type to its PostgreSQL column type.
// Computed types (formula, lookup, rollup) and layout types (label, line, spacer)
// return an empty string because they have no backing database column. The mapping
// covers the full set of Topworks field types:
//
//   - text, textarea     -> TEXT
//   - number             -> NUMERIC
//   - integer            -> INTEGER
//   - boolean            -> BOOLEAN
//   - date               -> DATE
//   - datetime           -> TIMESTAMPTZ
//   - time               -> TIME
//   - select             -> VARCHAR(255)
//   - multiselect        -> TEXT[]
//   - relation, user, file -> UUID
//   - json, table, spreadsheet -> JSONB
//   - autonumber         -> BIGINT
//   - (unknown)          -> TEXT (safe fallback)
func FieldTypeToPG(ft schema.FieldType) string {
	switch ft {
	case schema.FieldText, schema.FieldTextarea:
		return "TEXT"
	case schema.FieldNumber:
		return "NUMERIC"
	case schema.FieldInteger:
		return "INTEGER"
	case schema.FieldBoolean:
		return "BOOLEAN"
	case schema.FieldDate:
		return "DATE"
	case schema.FieldDatetime:
		return "TIMESTAMPTZ"
	case schema.FieldTime:
		return "TIME"
	case schema.FieldSelect:
		return "VARCHAR(255)"
	case schema.FieldMultiselect:
		return "TEXT[]"
	case schema.FieldRelation:
		return "UUID"
	case schema.FieldUser:
		return "UUID"
	case schema.FieldFile:
		return "UUID"
	case schema.FieldJSON, schema.FieldTable, schema.FieldSpreadsheet:
		return "JSONB"
	case schema.FieldAutonumber:
		return "BIGINT"
	case schema.FieldFormula, schema.FieldLookup, schema.FieldRollup:
		return "" // computed types have no DB column
	case schema.FieldLabel, schema.FieldLine, schema.FieldSpacer:
		return "" // layout types have no DB column
	default:
		return "TEXT"
	}
}

// All generators return slices of individual SQL statements.
// Each element is a complete, standalone statement with no trailing semicolon.
// This avoids any `;\n` splitting that breaks on statements with embedded
// semicolons (function bodies, DO blocks, triggers).

// GenerateCreateTable produces the DDL statements for a new data table in the
// "data" schema. The up DDL includes:
//
//   - CREATE TABLE with an auto-generated UUID primary key (id)
//   - User-defined columns derived from the field list (computed/M:N fields are skipped)
//   - System columns: created_at, updated_at, created_by, updated_by, deleted_at, _version
//   - Sequences for any autonumber fields (created before the table)
//   - A partial index on id WHERE deleted_at IS NULL (accelerates all active-row queries)
//   - Individual indexes for fields marked as indexed
//   - Full-text search infrastructure: _tsv TSVECTOR column, GIN index, and an
//     auto-update trigger that concatenates all text/textarea fields
//
// The down DDL is a single DROP TABLE IF EXISTS statement for rollback.
func GenerateCreateTable(col schema.Collection, fields []schema.Field) (up, down []string) {
	qTable := quoteIdent("data", col.Slug)

	var colDefs []string
	colDefs = append(colDefs, "id UUID PRIMARY KEY DEFAULT gen_random_uuid()")
	for _, f := range fields {
		if f.FieldType.NoColumn() || f.IsManyToMany() {
			continue
		}
		colDefs = append(colDefs, columnDefWithTable(col.Slug, f))
	}
	colDefs = append(colDefs,
		"created_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"created_by UUID",
		"updated_by UUID",
		"deleted_at TIMESTAMPTZ",
		"_version INTEGER NOT NULL DEFAULT 1",
	)

	// Create sequences for autonumber fields before the table.
	for _, f := range fields {
		if f.FieldType == schema.FieldAutonumber {
			seqName := autonumberSeqName(col.Slug, f.Slug)
			up = append(up, fmt.Sprintf("CREATE SEQUENCE %s", quoteIdent("data", seqName)))
		}
	}

	up = append(up, fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", qTable, strings.Join(colDefs, ",\n  ")))

	// Partial index on deleted_at — every query filters WHERE deleted_at IS NULL.
	up = append(up, fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s (id) WHERE deleted_at IS NULL",
		quoteIdentSingle(fmt.Sprintf("idx_%s_active", col.Slug)), qTable))

	for _, f := range fields {
		if f.IsIndexed {
			up = append(up, generateCreateIndex(col.Slug, f.Slug))
		}
	}

	// Full-text search: add _tsv column + GIN index + auto-update trigger
	// for all text/textarea fields.
	var tsvFields []string
	for _, f := range fields {
		if f.FieldType == schema.FieldText || f.FieldType == schema.FieldTextarea {
			tsvFields = append(tsvFields, f.Slug)
		}
	}
	if len(tsvFields) > 0 {
		up = append(up,
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN _tsv TSVECTOR", qTable),
		)
		up = append(up,
			fmt.Sprintf("CREATE INDEX %s ON %s USING GIN (_tsv)",
				quoteIdentSingle(fmt.Sprintf("idx_%s_tsv", col.Slug)), qTable),
		)
		// Build coalesce expressions for the trigger function.
		var coalesces []string
		for _, slug := range tsvFields {
			coalesces = append(coalesces, fmt.Sprintf("coalesce(NEW.%s, '')", quoteIdentSingle(slug)))
		}
		trigFunc := fmt.Sprintf("tsvupd_%s", col.Slug)
		trigName := fmt.Sprintf("trg_%s_tsv", col.Slug)

		up = append(up,
			fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$ BEGIN NEW._tsv := to_tsvector('simple', %s); RETURN NEW; END $$ LANGUAGE plpgsql`,
				quoteIdent("data", trigFunc),
				strings.Join(coalesces, " || ' ' || "),
			),
		)
		up = append(up,
			fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT OR UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION %s()",
				quoteIdentSingle(trigName), qTable, quoteIdent("data", trigFunc)),
		)
	}

	down = []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", qTable)}
	return up, down
}

// GenerateDropTable produces DDL to drop a data table.
// The returned `down` is nil because reconstructing the original schema
// requires the full Collection+Fields (see engine.DropCollection which stores
// the reconstruction DDL in the migration record by calling GenerateCreateTable
// at drop time).
func GenerateDropTable(slug string) (up, down []string) {
	qTable := quoteIdent("data", slug)
	up = []string{fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", qTable)}
	return up, nil
}

// GenerateAddColumn produces DDL to add a column to an existing data table.
// The field type is mapped to a PostgreSQL type via FieldTypeToPG. The up DDL
// includes ALTER TABLE ADD COLUMN with optional NOT NULL and DEFAULT clauses,
// plus separate statements for UNIQUE constraints and indexes if requested.
// Autonumber fields additionally create a sequence with a DEFAULT nextval().
//
// Layout and computed fields (NoColumn=true) produce no DDL and return nil.
//
// The down DDL contains the matching ALTER TABLE DROP COLUMN for rollback.
func GenerateAddColumn(tableSlug string, f schema.Field) (up, down []string) {
	if f.FieldType.NoColumn() {
		return nil, nil
	}
	qTable := quoteIdent("data", tableSlug)
	qCol := quoteIdentSingle(f.Slug)
	pgType := FieldTypeToPG(f.FieldType)

	// Autonumber: create sequence + column with DEFAULT nextval
	if f.FieldType == schema.FieldAutonumber {
		seqName := autonumberSeqName(tableSlug, f.Slug)
		up = append(up,
			fmt.Sprintf("CREATE SEQUENCE %s", quoteIdent("data", seqName)),
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NOT NULL DEFAULT nextval('%s')",
				qTable, qCol, pgType, "data."+seqName),
		)
		down = []string{
			fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", qTable, qCol),
			fmt.Sprintf("DROP SEQUENCE IF EXISTS %s", quoteIdent("data", seqName)),
		}
		return up, down
	}

	clause := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", qTable, qCol, pgType)
	if f.IsRequired {
		clause += " NOT NULL"
	}
	if def := defaultClause(f); def != "" {
		clause += " DEFAULT " + def
	}
	up = append(up, clause)

	if f.IsUnique {
		up = append(up, generateAddUnique(tableSlug, f.Slug))
	}
	if f.IsIndexed {
		up = append(up, generateCreateIndex(tableSlug, f.Slug))
	}

	down = []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", qTable, qCol)}
	return up, down
}

// GenerateDropColumn produces DDL to drop a column from a data table.
// The up DDL is ALTER TABLE DROP COLUMN IF EXISTS. For autonumber fields, the
// associated sequence is also dropped. The down DDL is the inverse ALTER TABLE
// ADD COLUMN with the original type, enabling rollback to restore the column
// structure (though data in the column is not recoverable).
func GenerateDropColumn(tableSlug string, f schema.Field) (up, down []string) {
	qTable := quoteIdent("data", tableSlug)
	qCol := quoteIdentSingle(f.Slug)
	pgType := FieldTypeToPG(f.FieldType)

	up = []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", qTable, qCol)}
	if f.FieldType == schema.FieldAutonumber {
		seqName := autonumberSeqName(tableSlug, f.Slug)
		up = append(up, fmt.Sprintf("DROP SEQUENCE IF EXISTS %s", quoteIdent("data", seqName)))
	}
	down = []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", qTable, qCol, pgType)}
	return up, down
}

// GenerateAlterColumnType produces DDL for a type change with the appropriate USING cast.
func GenerateAlterColumnType(tableSlug, colSlug string, from, to schema.FieldType) (up, down []string) {
	qTable := quoteIdent("data", tableSlug)
	qCol := quoteIdentSingle(colSlug)
	toPG := FieldTypeToPG(to)
	fromPG := FieldTypeToPG(from)
	casting := castExpr(qCol, from, to)

	up = []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s",
		qTable, qCol, toPG, casting)}
	down = []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
		qTable, qCol, fromPG)}
	return up, down
}

// GenerateSetNotNull / GenerateDropNotNull — single-statement helpers.
func GenerateSetNotNull(tableSlug, colSlug string) string {
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
		quoteIdent("data", tableSlug), quoteIdentSingle(colSlug))
}

func GenerateDropNotNull(tableSlug, colSlug string) string {
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
		quoteIdent("data", tableSlug), quoteIdentSingle(colSlug))
}

// validOnDelete is the whitelist of allowed ON DELETE actions.
var validOnDelete = map[string]bool{
	"CASCADE":     true,
	"SET NULL":    true,
	"RESTRICT":    true,
	"NO ACTION":   true,
	"SET DEFAULT": true,
}

// SanitizeOnDelete returns a safe ON DELETE clause, defaulting to SET NULL.
func SanitizeOnDelete(s string) string {
	up := strings.ToUpper(strings.TrimSpace(s))
	if validOnDelete[up] {
		return up
	}
	return "SET NULL"
}

// GenerateAddFK produces a foreign-key constraint for a relation field.
// The FK is named fk_{tableSlug}_{colSlug} and references data."{targetSlug}"(id).
// The ON DELETE action is sanitized via SanitizeOnDelete (defaults to SET NULL
// if the provided action is invalid). The down DDL drops the constraint by name.
func GenerateAddFK(tableSlug, colSlug, targetSlug, onDelete string) (up, down string) {
	constraintName := fmt.Sprintf("fk_%s_%s", tableSlug, colSlug)
	action := SanitizeOnDelete(onDelete)
	up = fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE %s",
		quoteIdent("data", tableSlug),
		quoteIdentSingle(constraintName),
		quoteIdentSingle(colSlug),
		quoteIdent("data", targetSlug),
		action,
	)
	down = fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s",
		quoteIdent("data", tableSlug), quoteIdentSingle(constraintName))
	return up, down
}

// GenerateJunctionTable produces DDL for a many-to-many (M:N) junction table.
// The junction table is created in the "data" schema with the structure:
//
//   - id: UUID primary key
//   - {slugA}_id: UUID NOT NULL REFERENCES data."{slugA}"(id) ON DELETE CASCADE
//   - {slugB}_id: UUID NOT NULL REFERENCES data."{slugB}"(id) ON DELETE CASCADE
//   - created_at: TIMESTAMPTZ
//   - UNIQUE({slugA}_id, {slugB}_id): prevents duplicate links
//
// Both FK columns cascade on delete so removing either side automatically cleans
// up the junction rows. The down DDL is DROP TABLE IF EXISTS.
func GenerateJunctionTable(slugA, slugB, junctionName string) (up, down string) {
	qTable := quoteIdent("data", junctionName)
	colA := slugA + "_id"
	colB := slugB + "_id"
	up = fmt.Sprintf(`CREATE TABLE %s (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  %s UUID NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
  %s UUID NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(%s, %s)
)`,
		qTable,
		quoteIdentSingle(colA), quoteIdent("data", slugA),
		quoteIdentSingle(colB), quoteIdent("data", slugB),
		quoteIdentSingle(colA), quoteIdentSingle(colB),
	)
	down = fmt.Sprintf("DROP TABLE IF EXISTS %s", qTable)
	return up, down
}

// GenerateAddStatusColumn produces DDL to add the _status column to a data table.
func GenerateAddStatusColumn(tableSlug string) (up, down []string) {
	qTable := quoteIdent("data", tableSlug)
	up = []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %q VARCHAR(255)", qTable, "_status")}
	down = []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %q", qTable, "_status")}
	return up, down
}

// GenerateDropStatusColumn produces DDL to drop the _status column.
func GenerateDropStatusColumn(tableSlug string) (up, down []string) {
	qTable := quoteIdent("data", tableSlug)
	up = []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %q", qTable, "_status")}
	down = []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %q VARCHAR(255)", qTable, "_status")}
	return up, down
}

// ---------- internal helpers ----------

func columnDefWithTable(tableSlug string, f schema.Field) string {
	qCol := quoteIdentSingle(f.Slug)
	pgType := FieldTypeToPG(f.FieldType)
	def := qCol + " " + pgType
	if f.FieldType == schema.FieldAutonumber && tableSlug != "" {
		seqRef := fmt.Sprintf("'data.%s'", autonumberSeqName(tableSlug, f.Slug))
		def += " NOT NULL DEFAULT nextval(" + seqRef + ")"
		return def
	}
	if f.IsRequired {
		def += " NOT NULL"
	}
	if d := defaultClause(f); d != "" {
		def += " DEFAULT " + d
	}
	if f.IsUnique {
		def += " UNIQUE"
	}
	return def
}

func defaultClause(f schema.Field) string {
	if len(f.DefaultValue) == 0 || string(f.DefaultValue) == "null" {
		return ""
	}
	var raw any
	if err := json.Unmarshal(f.DefaultValue, &raw); err != nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return "'" + escapeSQLString(v) + "'"
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return ""
	}
}

func castExpr(qCol string, from, to schema.FieldType) string {
	toPG := FieldTypeToPG(to)
	switch {
	case from == schema.FieldSelect && to == schema.FieldMultiselect:
		return fmt.Sprintf("ARRAY[%s]::TEXT[]", qCol)
	case from == schema.FieldNumber && to == schema.FieldInteger:
		return fmt.Sprintf("%s::INTEGER", qCol)
	case from == schema.FieldInteger && to == schema.FieldNumber:
		return fmt.Sprintf("%s::NUMERIC", qCol)
	default:
		return fmt.Sprintf("%s::%s", qCol, toPG)
	}
}

func generateCreateIndex(tableSlug, colSlug string) string {
	idxName := fmt.Sprintf("idx_%s_%s", tableSlug, colSlug)
	return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
		quoteIdentSingle(idxName), quoteIdent("data", tableSlug), quoteIdentSingle(colSlug))
}

func generateAddUnique(tableSlug, colSlug string) string {
	cName := fmt.Sprintf("uq_%s_%s", tableSlug, colSlug)
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s)",
		quoteIdent("data", tableSlug), quoteIdentSingle(cName), quoteIdentSingle(colSlug))
}

func quoteIdent(schemaName, name string) string {
	return pgutil.QuoteQualified(schemaName, name)
}

func quoteIdentSingle(name string) string {
	return pgutil.QuoteIdent(name)
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// autonumberSeqName returns the sequence name for an autonumber column.
func autonumberSeqName(tableSlug, colSlug string) string {
	return fmt.Sprintf("%s_%s_seq", tableSlug, colSlug)
}
