package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/pgutil"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// computedOpts extracts common options from a computed field.
type computedOpts struct {
	Expression    string `json:"expression"`     // formula only
	RelationField string `json:"relation_field"` // lookup & rollup
	TargetField   string `json:"target_field"`   // lookup & rollup
	Function      string `json:"function"`       // rollup only
}

func parseComputedOpts(raw json.RawMessage) computedOpts {
	var o computedOpts
	_ = json.Unmarshal(raw, &o)
	return o
}

// resolveComputedFields fills in computed field values for each record.
// Must be called after records are fetched and relations are expanded.
// It handles three computation types:
//
//   - Formula: evaluated at the SQL level via a computed expression in the SELECT
//     clause (see formulaExpr in dynamic.go). The formula parser translates user
//     expressions (e.g., "price * quantity") into safe SQL fragments. This method
//     skips formula fields because their values are already populated by the query.
//
//   - Lookup: fetches a single field value from a related record. For 1:1/1:N
//     relations, returns the target field value. For M:N relations, returns an
//     array of values from all linked records. Uses batch IN queries to avoid N+1.
//
//   - Rollup: aggregates a field across related records using functions like SUM,
//     AVG, MIN, MAX, COUNT, COUNTA. Supports both forward relations (this table
//     has FK to target) and reverse relations (target table has FK back to this
//     table), as well as M:N relations via junction tables.
func (h *DynHandler) resolveComputedFields(
	ctx context.Context,
	records []map[string]any,
	fields []schema.Field,
) {
	if len(records) == 0 {
		return
	}

	// Index fields by slug.
	bySlug := make(map[string]schema.Field, len(fields))
	for _, f := range fields {
		bySlug[f.Slug] = f
	}

	for _, f := range fields {
		if !f.FieldType.IsComputed() {
			continue
		}
		opts := parseComputedOpts(f.Options)

		switch f.FieldType {
		case schema.FieldFormula:
			// Formula fields are computed at the SQL level (see formulaExpr in dynamic.go).
			// Only fall back to client-side evaluation if no SQL value was set.
			continue
		case schema.FieldLookup:
			h.resolveLookup(ctx, records, fields, bySlug, f, opts)
		case schema.FieldRollup:
			h.resolveRollup(ctx, records, fields, bySlug, f, opts)
		}
	}
}

// --- Lookup ---

// resolveLookup fetches a single field value from the related record.
// For 1:1/1:N: returns a single value.
// For M:N: returns an array of values from all linked records.
func (h *DynHandler) resolveLookup(
	ctx context.Context,
	records []map[string]any,
	fields []schema.Field,
	bySlug map[string]schema.Field,
	f schema.Field,
	opts computedOpts,
) {
	relField, ok := bySlug[opts.RelationField]
	if !ok || relField.FieldType != schema.FieldRelation || relField.Relation == nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	targetCol, ok := h.cache.CollectionByID(relField.Relation.TargetCollectionID)
	if !ok {
		return
	}

	// M:N lookup: collect IDs from the array field populated by loadM2MFields.
	if relField.IsManyToMany() {
		h.resolveLookupM2M(ctx, records, f, opts, relField, targetCol)
		return
	}

	// Collect distinct relation IDs.
	seen := make(map[string]struct{})
	var ids []string
	for _, row := range records {
		s := extractRelID(row, opts.RelationField)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			ids = append(ids, s)
		}
	}
	if len(ids) == 0 {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	// Batch fetch the target field values.
	targetFieldSlug := opts.TargetField
	values, err := batchFetchField(ctx, h.pool, targetCol.Slug, targetFieldSlug, ids)
	if err != nil {
		slog.Warn("resolveRelation: batch fetch failed",
			"field", f.Slug, "target", targetCol.Slug, "error", err)
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	for _, row := range records {
		relID := extractRelID(row, opts.RelationField)
		if v, ok := values[relID]; ok {
			row[f.Slug] = v
		} else {
			row[f.Slug] = nil
		}
	}
}

// resolveLookupM2M handles lookup for M:N relations. Returns an array of values.
func (h *DynHandler) resolveLookupM2M(
	ctx context.Context,
	records []map[string]any,
	f schema.Field,
	opts computedOpts,
	relField schema.Field,
	targetCol schema.Collection,
) {
	// Collect all distinct target IDs from M:N arrays.
	seen := make(map[string]struct{})
	var allIDs []string
	for _, row := range records {
		ids := toStringSliceFromRow(row[opts.RelationField])
		for _, id := range ids {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				allIDs = append(allIDs, id)
			}
		}
	}
	if len(allIDs) == 0 {
		for _, row := range records {
			row[f.Slug] = []any{}
		}
		return
	}

	values, err := batchFetchField(ctx, h.pool, targetCol.Slug, opts.TargetField, allIDs)
	if err != nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	for _, row := range records {
		ids := toStringSliceFromRow(row[opts.RelationField])
		result := make([]any, 0, len(ids))
		for _, id := range ids {
			if v, ok := values[id]; ok {
				result = append(result, v)
			}
		}
		row[f.Slug] = result
	}
}

// --- Rollup ---

// resolveRollup aggregates values from a reverse relation.
// The relation_field points to a relation field on THIS collection.
// We find all records in the target collection that reference
// each record's ID and aggregate the target_field.
func (h *DynHandler) resolveRollup(
	ctx context.Context,
	records []map[string]any,
	fields []schema.Field,
	bySlug map[string]schema.Field,
	f schema.Field,
	opts computedOpts,
) {
	relField, ok := bySlug[opts.RelationField]
	if !ok || relField.FieldType != schema.FieldRelation || relField.Relation == nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	targetCol, ok := h.cache.CollectionByID(relField.Relation.TargetCollectionID)
	if !ok {
		return
	}

	// Collect all record IDs from the current result set.
	var recordIDs []string
	for _, row := range records {
		if id, ok := row["id"].(string); ok {
			recordIDs = append(recordIDs, id)
		}
	}
	if len(recordIDs) == 0 {
		return
	}

	fn := strings.ToUpper(opts.Function)
	targetFieldSlug := opts.TargetField

	// M:N rollup: aggregate values from linked target records via junction table.
	if relField.IsManyToMany() {
		h.resolveRollupM2M(ctx, records, f, opts, relField, targetCol)
		return
	}

	// Two strategies:
	// 1. Forward relation (this collection has FK to target): use lookup approach
	// 2. Reverse relation: find records in target that reference this collection's records

	// For rollup we look for a reverse relation: target collection has a relation field
	// pointing back to our collection. We find which field on the target table references
	// the source collection.
	sourceCollectionID := relField.CollectionID

	// Find the field on the target collection that references back to the source.
	targetFields := h.cache.Fields(targetCol.ID)
	var reverseRelSlug string
	for _, tf := range targetFields {
		if tf.FieldType == schema.FieldRelation && tf.Relation != nil &&
			tf.Relation.TargetCollectionID == sourceCollectionID {
			reverseRelSlug = tf.Slug
			break
		}
	}

	// If no reverse relation found, try forward lookup (the relation field on this
	// collection points to target, so we aggregate values from target that are
	// referenced by records in this collection).
	if reverseRelSlug == "" {
		// Forward: collect relation IDs from current records, aggregate target field values.
		h.resolveRollupForward(ctx, records, f, opts, relField, targetCol)
		return
	}

	// Reverse: aggregate target records that reference our records.
	results, err := batchRollup(ctx, h.pool, targetCol.Slug, reverseRelSlug, targetFieldSlug, fn, recordIDs)
	if err != nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	for _, row := range records {
		id, _ := row["id"].(string)
		if v, ok := results[id]; ok {
			row[f.Slug] = v
		} else {
			if fn == "COUNT" || fn == "COUNTA" {
				row[f.Slug] = float64(0)
			} else {
				row[f.Slug] = nil
			}
		}
	}
}

// resolveRollupForward handles rollup when the relation is forward (this table → target).
// Groups the current records by their relation FK value and fetches target field values.
func (h *DynHandler) resolveRollupForward(
	ctx context.Context,
	records []map[string]any,
	f schema.Field,
	opts computedOpts,
	relField schema.Field,
	targetCol schema.Collection,
) {
	fn := strings.ToUpper(opts.Function)

	// For forward relation rollup, each record points to ONE target record,
	// so aggregation doesn't make as much sense. But we support it by grouping
	// source records by target ID and computing per-source-record values.
	// In this case, each record's rollup value is just the target field value.
	seen := make(map[string]struct{})
	var ids []string
	for _, row := range records {
		s := extractRelID(row, opts.RelationField)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			ids = append(ids, s)
		}
	}
	if len(ids) == 0 {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	values, err := batchFetchField(ctx, h.pool, targetCol.Slug, opts.TargetField, ids)
	if err != nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	for _, row := range records {
		relID := extractRelID(row, opts.RelationField)
		if v, ok := values[relID]; ok {
			row[f.Slug] = v
		} else {
			if fn == "COUNT" || fn == "COUNTA" {
				row[f.Slug] = float64(0)
			} else {
				row[f.Slug] = nil
			}
		}
	}
}

// --- Helpers ---

// extractRelID gets the relation UUID from a record. The value may be a string (UUID)
// or an expanded object with an "id" field.
func extractRelID(row map[string]any, fieldSlug string) string {
	v := row[fieldSlug]
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}
	return ""
}

// batchFetchField fetches a single column value for multiple records by their IDs
// in a single SELECT ... WHERE id IN (...) query. Returns a map from record ID to
// the field value. This is the core N+1 prevention mechanism for lookup and rollup
// computations: instead of querying once per parent record, all target IDs are
// collected up front and fetched in one batch.
func batchFetchField(ctx context.Context, pool *pgxpool.Pool, tableSlug, fieldSlug string, ids []string) (map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	qTable := pgutil.QuoteQualified("data", tableSlug)
	qCol := pgutil.QuoteIdent(fieldSlug)
	sql := fmt.Sprintf(
		"SELECT id, %s FROM %s WHERE id IN (%s) AND deleted_at IS NULL",
		qCol, qTable, strings.Join(placeholders, ","),
	)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]any, len(ids))
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			slog.Warn("batchFetchField: skipping row", "error", err)
			continue
		}
		if len(vals) >= 2 {
			id := normalizeValue(vals[0])
			val := normalizeValue(vals[1])
			if idStr, ok := id.(string); ok {
				result[idStr] = val
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// batchRollup runs an aggregate query (SUM, AVG, MIN, MAX, COUNT, COUNTA) on the
// target table, grouped by a relation FK column. It collects results for all source
// record IDs in a single query. Returns a map from source record ID to the
// aggregated float64 value.
func batchRollup(
	ctx context.Context,
	pool *pgxpool.Pool,
	targetTableSlug, relationColSlug, valueColSlug, fn string,
	sourceIDs []string,
) (map[string]float64, error) {
	if len(sourceIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(sourceIDs))
	args := make([]any, len(sourceIDs))
	for i, id := range sourceIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	qTable := pgutil.QuoteQualified("data", targetTableSlug)
	qRelCol := pgutil.QuoteIdent(relationColSlug)
	qValCol := pgutil.QuoteIdent(valueColSlug)

	var aggExpr string
	switch fn {
	case "SUM":
		aggExpr = fmt.Sprintf("COALESCE(SUM(%s::NUMERIC), 0)", qValCol)
	case "AVG":
		aggExpr = fmt.Sprintf("AVG(%s::NUMERIC)", qValCol)
	case "MIN":
		aggExpr = fmt.Sprintf("MIN(%s::NUMERIC)", qValCol)
	case "MAX":
		aggExpr = fmt.Sprintf("MAX(%s::NUMERIC)", qValCol)
	case "COUNT":
		aggExpr = "COUNT(*)"
	case "COUNTA":
		aggExpr = fmt.Sprintf("COUNT(%s)", qValCol)
	default:
		aggExpr = "COUNT(*)"
	}

	sql := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s IN (%s) AND deleted_at IS NULL GROUP BY %s",
		qRelCol, aggExpr, qTable, qRelCol,
		strings.Join(placeholders, ","), qRelCol,
	)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64, len(sourceIDs))
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			slog.Warn("batchRollup: skipping row", "error", err)
			continue
		}
		if len(vals) >= 2 {
			id := normalizeValue(vals[0])
			val := normalizeValue(vals[1])
			if idStr, ok := id.(string); ok {
				if num, ok := toFloat64(val); ok {
					result[idStr] = num
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// resolveRollupM2M aggregates values from M:N linked records via junction table.
func (h *DynHandler) resolveRollupM2M(
	ctx context.Context,
	records []map[string]any,
	f schema.Field,
	opts computedOpts,
	relField schema.Field,
	targetCol schema.Collection,
) {
	fn := strings.ToUpper(opts.Function)

	// Collect all distinct target IDs from the M:N arrays.
	seen := make(map[string]struct{})
	var allIDs []string
	for _, row := range records {
		ids := toStringSliceFromRow(row[opts.RelationField])
		for _, id := range ids {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				allIDs = append(allIDs, id)
			}
		}
	}
	if len(allIDs) == 0 {
		for _, row := range records {
			if fn == "COUNT" || fn == "COUNTA" {
				row[f.Slug] = float64(0)
			} else {
				row[f.Slug] = nil
			}
		}
		return
	}

	// Fetch target field values.
	values, err := batchFetchField(ctx, h.pool, targetCol.Slug, opts.TargetField, allIDs)
	if err != nil {
		for _, row := range records {
			row[f.Slug] = nil
		}
		return
	}

	// Aggregate per record.
	for _, row := range records {
		ids := toStringSliceFromRow(row[opts.RelationField])
		var nums []float64
		nonNullCount := 0
		for _, id := range ids {
			v, ok := values[id]
			if !ok {
				continue
			}
			if v != nil {
				nonNullCount++
			}
			if n, ok := toFloat64(v); ok {
				nums = append(nums, n)
			}
		}

		switch fn {
		case "COUNT":
			row[f.Slug] = float64(len(ids))
		case "COUNTA":
			row[f.Slug] = float64(nonNullCount)
		case "SUM":
			sum := 0.0
			for _, n := range nums {
				sum += n
			}
			row[f.Slug] = sum
		case "AVG":
			if len(nums) == 0 {
				row[f.Slug] = nil
			} else {
				sum := 0.0
				for _, n := range nums {
					sum += n
				}
				row[f.Slug] = sum / float64(len(nums))
			}
		case "MIN":
			if len(nums) == 0 {
				row[f.Slug] = nil
			} else {
				min := nums[0]
				for _, n := range nums[1:] {
					if n < min {
						min = n
					}
				}
				row[f.Slug] = min
			}
		case "MAX":
			if len(nums) == 0 {
				row[f.Slug] = nil
			} else {
				max := nums[0]
				for _, n := range nums[1:] {
					if n > max {
						max = n
					}
				}
				row[f.Slug] = max
			}
		default:
			row[f.Slug] = float64(len(ids))
		}
	}
}

// toFloat64 attempts to convert any value to float64.
func toFloat64(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
