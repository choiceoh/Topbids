package pgutil

import "github.com/jackc/pgx/v5"

// QuoteIdent safely quotes a single SQL identifier using pgx.Identifier.
// Example: QuoteIdent("my_col") → `"my_col"`
func QuoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// QuoteQualified safely quotes a schema-qualified SQL identifier.
// Example: QuoteQualified("data", "my_table") → `"data"."my_table"`
func QuoteQualified(schema, name string) string {
	return pgx.Identifier{schema, name}.Sanitize()
}
