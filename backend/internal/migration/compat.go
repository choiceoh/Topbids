package migration

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

type conversionKind int

const (
	convAlways      conversionKind = iota // always safe
	convConditional                       // requires data validation
	convForbidden                         // never allowed
)

type compatEntry struct {
	kind conversionKind
	// validationQuery: if conditional, SQL predicate that matches INCOMPATIBLE rows.
	// The placeholder %s is replaced with the quoted column name.
	validationQuery string
}

// compatMatrix defines whether a type A → type B conversion is allowed
// and, if conditional, which rows are incompatible.
var compatMatrix = map[schema.FieldType]map[schema.FieldType]compatEntry{
	schema.FieldText: {
		schema.FieldNumber:   {convConditional, `%s IS NOT NULL AND %s::TEXT !~ '^-?[0-9]+(\\.[0-9]+)?$'`},
		schema.FieldInteger:  {convConditional, `%s IS NOT NULL AND %s::TEXT !~ '^-?[0-9]+$'`},
		schema.FieldDate:     {convConditional, `%s IS NOT NULL AND %s::TEXT !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'`},
		schema.FieldBoolean:  {convConditional, `%s IS NOT NULL AND LOWER(%s::TEXT) NOT IN ('true','false','t','f','1','0')`},
		schema.FieldTextarea: {convAlways, ""},
		schema.FieldTime:     {convConditional, `%s IS NOT NULL AND %s::TEXT !~ '^([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?$'`},
	},
	schema.FieldNumber: {
		schema.FieldText:    {convAlways, ""},
		schema.FieldInteger: {convConditional, `%s IS NOT NULL AND %s != TRUNC(%s)`},
	},
	schema.FieldInteger: {
		schema.FieldText:   {convAlways, ""},
		schema.FieldNumber: {convAlways, ""},
	},
	schema.FieldSelect: {
		schema.FieldMultiselect: {convAlways, ""},
		schema.FieldText:        {convAlways, ""},
	},
	schema.FieldMultiselect: {
		schema.FieldSelect: {convConditional, `%s IS NOT NULL AND array_length(%s, 1) > 1`},
	},
	schema.FieldBoolean: {
		schema.FieldText:    {convAlways, ""},
		schema.FieldInteger: {convAlways, ""},
	},
	schema.FieldDate: {
		schema.FieldText:     {convAlways, ""},
		schema.FieldDatetime: {convAlways, ""},
	},
	schema.FieldDatetime: {
		schema.FieldText: {convAlways, ""},
		schema.FieldDate: {convAlways, ""}, // truncates time portion
	},
	schema.FieldTextarea: {
		schema.FieldText: {convAlways, ""},
	},
	schema.FieldTime: {
		schema.FieldText: {convAlways, ""},
	},
	// Autonumber cannot be converted to/from any other type.
	schema.FieldAutonumber: {},
}

// CheckCompat returns whether a type change from→to is possible.
func CheckCompat(from, to schema.FieldType) (allowed bool, conditional bool) {
	m, ok := compatMatrix[from]
	if !ok {
		return false, false
	}
	e, ok := m[to]
	if !ok {
		return false, false
	}
	if e.kind == convForbidden {
		return false, false
	}
	return true, e.kind == convConditional
}

// ValidateConversion counts rows incompatible with the proposed type change
// and returns a sample of up to 5 offending rows.
func ValidateConversion(
	ctx context.Context,
	pool *pgxpool.Pool,
	tableSlug, colSlug string,
	from, to schema.FieldType,
) (total int64, incompatible int64, sample []map[string]any, err error) {
	m, ok := compatMatrix[from]
	if !ok {
		return 0, 0, nil, fmt.Errorf("no conversion path from %s to %s", from, to)
	}
	entry, ok := m[to]
	if !ok || entry.kind == convForbidden {
		return 0, 0, nil, fmt.Errorf("conversion from %s to %s is forbidden", from, to)
	}

	qTable := quoteIdent("data", tableSlug)
	qCol := quoteIdentSingle(colSlug)

	// Total rows.
	err = pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL", qTable)).Scan(&total)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("count total rows: %w", err)
	}

	if entry.kind == convAlways {
		return total, 0, nil, nil
	}

	// Build the incompatibility predicate.
	// The validation query template uses %s for the column reference.
	pred := buildPredicate(entry.validationQuery, qCol)

	// Count incompatible rows.
	err = pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL AND (%s)", qTable, pred),
	).Scan(&incompatible)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("count incompatible rows: %w", err)
	}

	if incompatible == 0 {
		return total, 0, nil, nil
	}

	// Fetch a sample of incompatible rows.
	rows, err := pool.Query(ctx,
		fmt.Sprintf("SELECT id, %s FROM %s WHERE deleted_at IS NULL AND (%s) LIMIT 5",
			qCol, qTable, pred))
	if err != nil {
		return total, incompatible, nil, fmt.Errorf("sample incompatible rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return total, incompatible, nil, err
		}
		row := map[string]any{
			"id":    vals[0],
			colSlug: vals[1],
		}
		sample = append(sample, row)
	}
	return total, incompatible, sample, rows.Err()
}

// buildPredicate replaces all %s in the template with the quoted column name.
func buildPredicate(template, qCol string) string {
	// Count %s occurrences and replace them.
	result := template
	for {
		idx := indexOf(result, "%s")
		if idx < 0 {
			break
		}
		result = result[:idx] + qCol + result[idx+2:]
	}
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
