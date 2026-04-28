package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// ExportCSV streams the entire (filtered) result set as CSV.
// It reuses the same filter/sort params as List but removes pagination.
func (h *DynHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()
	qTable := pgutil.QuoteQualified("data", col.Slug)

	where, args, err := ParseFilters(params, fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Text search.
	searchClause, searchArgs := BuildSearchClause(params.Get("q"), fields, "", len(args)+1)
	where += " " + searchClause
	args = append(args, searchArgs...)

	orderBy := ParseSort(params.Get("sort"), fields)
	selectCols := buildSelectCols(fields, false, &selectColOpts{cache: h.cache})

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s %s",
		selectCols, qTable, where, orderBy)

	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	records, err := collectRows(rows)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Build CSV headers: id + field labels + system columns.
	headers := []string{"id"}
	slugs := []string{"id"}
	for _, f := range fields {
		headers = append(headers, f.Label)
		slugs = append(slugs, f.Slug)
	}
	headers = append(headers, "작성일", "수정일")
	slugs = append(slugs, "created_at", "updated_at")

	filename := fmt.Sprintf("%s_%s.csv", col.Slug, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	// BOM for Excel UTF-8 compatibility.
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.Write(headers)

	for _, rec := range records {
		row := make([]string, len(slugs))
		for i, s := range slugs {
			row[i] = formatCSVValue(rec[s])
		}
		cw.Write(row)
	}
	cw.Flush()
}

func formatCSVValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339)
	case []any:
		parts := make([]string, len(val))
		for i, el := range val {
			parts[i] = fmt.Sprint(el)
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(val, ", ")
	default:
		return fmt.Sprint(val)
	}
}

// ImportCSV parses an uploaded CSV file and bulk-creates records.
// The first row must be a header row containing field slugs.
func (h *DynHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 60*time.Second)
	defer cancel()

	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_create") {
		return
	}

	// Limit upload size to 10 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing or invalid file upload")
		return
	}
	defer file.Close()

	cr := csv.NewReader(file)
	cr.TrimLeadingSpace = true

	// Read header row.
	headerRow, err := cr.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read CSV header")
		return
	}

	// Map header indices to field slugs. Accept both slug and label.
	bySlug := make(map[string]schema.Field, len(fields))
	byLabel := make(map[string]schema.Field, len(fields))
	for _, f := range fields {
		bySlug[f.Slug] = f
		byLabel[f.Label] = f
	}

	// Optional: AI-provided column mapping (header → slug).
	var aiMap map[string]string
	if cm := r.FormValue("column_map"); cm != "" {
		_ = json.Unmarshal([]byte(cm), &aiMap)
	}

	type colMapping struct {
		idx   int
		field schema.Field
	}
	var mappings []colMapping
	for i, h := range headerRow {
		h = strings.TrimSpace(h)
		// Strip BOM from first header.
		if i == 0 {
			h = strings.TrimPrefix(h, "\xef\xbb\xbf")
		}
		if f, ok := bySlug[h]; ok {
			mappings = append(mappings, colMapping{idx: i, field: f})
		} else if f, ok := byLabel[h]; ok {
			mappings = append(mappings, colMapping{idx: i, field: f})
		} else if aiMap != nil {
			// Fall back to AI-provided mapping.
			if slug, ok := aiMap[h]; ok {
				if f, ok := bySlug[slug]; ok {
					mappings = append(mappings, colMapping{idx: i, field: f})
				}
			}
		}
		// Skip unrecognized columns silently.
	}

	if len(mappings) == 0 {
		writeError(w, http.StatusBadRequest, "no CSV columns matched any field slug or label")
		return
	}

	// Read data rows.
	var bodies []map[string]any
	lineNum := 1
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("CSV parse error at line %d: %v", lineNum+1, err))
			return
		}
		lineNum++

		body := make(map[string]any)
		for _, m := range mappings {
			if m.idx >= len(record) {
				continue
			}
			val := strings.TrimSpace(record[m.idx])
			if val == "" {
				continue
			}
			coerced, err := coerceCSVValue(val, m.field)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("line %d, field %q: %v", lineNum, m.field.Slug, err))
				return
			}
			body[m.field.Slug] = coerced
		}
		if len(body) > 0 {
			bodies = append(bodies, body)
		}
	}

	if len(bodies) == 0 {
		writeError(w, http.StatusBadRequest, "CSV contains no data rows")
		return
	}
	if len(bodies) > 1000 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("CSV too large: %d rows (max 1000)", len(bodies)))
		return
	}

	// Validate all records.
	for i, body := range bodies {
		if err := validatePayload(r.Context(), h.pool, h.cache, body, fields, true); err != nil {
			handleErr(w, r, fmt.Errorf("row %d: %w", i+2, err))
			return
		}
	}

	// Bulk insert in a single transaction.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	qTable := pgutil.QuoteQualified("data", col.Slug)
	selectCols := buildSelectCols(fields, false, &selectColOpts{cache: h.cache})

	user, _ := middleware.GetUser(r.Context())
	created := make([]map[string]any, 0, len(bodies))
	for i, body := range bodies {
		colNames, placeholders, args := buildInsertColumns(body, fields, user.UserID)
		var sql string
		if len(colNames) == 0 {
			sql = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES RETURNING %s", qTable, selectCols)
		} else {
			sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
				qTable, strings.Join(colNames, ", "), strings.Join(placeholders, ", "), selectCols)
		}
		rows, err := tx.Query(r.Context(), sql, args...)
		if err != nil {
			handleErr(w, r, fmt.Errorf("row %d: %w", i+2, err))
			return
		}
		recs, err := collectRows(rows)
		rows.Close()
		if err != nil {
			handleErr(w, r, fmt.Errorf("row %d: %w", i+2, err))
			return
		}
		if len(recs) > 0 {
			created = append(created, recs[0])
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"imported": len(created),
	})
}

// coerceCSVValue converts a raw CSV string into the appropriate Go type for a field.
func coerceCSVValue(val string, f schema.Field) (any, error) {
	switch f.FieldType {
	case schema.FieldText, schema.FieldTextarea:
		return val, nil
	case schema.FieldNumber:
		var n float64
		if _, err := fmt.Sscanf(val, "%f", &n); err != nil {
			return nil, fmt.Errorf("expected number, got %q", val)
		}
		return n, nil
	case schema.FieldInteger:
		var n int64
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
			return nil, fmt.Errorf("expected integer, got %q", val)
		}
		return n, nil
	case schema.FieldBoolean:
		switch strings.ToLower(val) {
		case "true", "1", "yes", "y":
			return true, nil
		case "false", "0", "no", "n":
			return false, nil
		default:
			return nil, fmt.Errorf("expected boolean, got %q", val)
		}
	case schema.FieldDate:
		if _, err := time.Parse("2006-01-02", val); err != nil {
			return nil, fmt.Errorf("expected YYYY-MM-DD date, got %q", val)
		}
		return val, nil
	case schema.FieldDatetime:
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return nil, fmt.Errorf("expected RFC3339 datetime, got %q", val)
		}
		return val, nil
	case schema.FieldTime:
		if _, err := time.Parse("15:04", val); err != nil {
			if _, err2 := time.Parse("15:04:05", val); err2 != nil {
				return nil, fmt.Errorf("expected HH:MM or HH:MM:SS, got %q", val)
			}
		}
		return val, nil
	case schema.FieldSelect:
		choices, err := schema.ExtractChoices(f.Options)
		if err == nil && len(choices) > 0 {
			found := false
			for _, c := range choices {
				if c == val {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("value %q is not in allowed choices %v", val, choices)
			}
		}
		return val, nil
	case schema.FieldMultiselect:
		parts := strings.Split(val, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		choices, err := schema.ExtractChoices(f.Options)
		if err == nil && len(choices) > 0 {
			choiceSet := make(map[string]bool, len(choices))
			for _, c := range choices {
				choiceSet[c] = true
			}
			for _, p := range parts {
				if !choiceSet[p] {
					return nil, fmt.Errorf("value %q is not in allowed choices %v", p, choices)
				}
			}
		}
		return parts, nil
	case schema.FieldRelation, schema.FieldUser, schema.FieldFile:
		return val, nil // UUID string
	case schema.FieldJSON:
		return val, nil // stored as-is; validatePayload will check
	default:
		return val, nil
	}
}
