package handler

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-pdf/fpdf"

	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// ExportPDF generates a PDF report of the (filtered) dataset and streams it
// as a downloadable file.
//
// GET /api/data/{slug}/export.pdf
func (h *DynHandler) ExportPDF(w http.ResponseWriter, r *http.Request) {
	r, cancel := withDeadline(r, 60*time.Second)
	defer cancel()

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

	searchClause, searchArgs := BuildSearchClause(params.Get("q"), fields, "", len(args)+1)
	where += " " + searchClause
	args = append(args, searchArgs...)

	// RLS.
	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	orderBy := ParseSort(params.Get("sort"), fields)
	selectCols := buildSelectCols(fields, false, &selectColOpts{cache: h.cache})

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s%s %s LIMIT 5000",
		selectCols, qTable, where, rlsClause, orderBy)

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

	// Filter to visible (non-layout) fields.
	var visibleFields []schema.Field
	for _, f := range fields {
		if !isLayoutField(f.FieldType) {
			visibleFields = append(visibleFields, f)
		}
	}

	pdf := buildPDF(col, visibleFields, records)

	filename := fmt.Sprintf("%s_%s.pdf", col.Slug, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := pdf.Output(w); err != nil {
		handleErr(w, r, err)
	}
}

// buildPDF creates a table-style PDF from records.
func buildPDF(col schema.Collection, fields []schema.Field, records []map[string]any) *fpdf.Fpdf {
	// Landscape A4 for wider tables.
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)

	// Try to load a CJK-capable font; fall back to Helvetica if not found.
	fontPath := findFontPath()
	fontFamily := "Helvetica"
	if fontPath != "" {
		pdf.AddUTF8Font("NotoSans", "", fontPath)
		pdf.AddUTF8Font("NotoSans", "B", fontPath)
		fontFamily = "NotoSans"
	}

	pdf.AddPage()

	// Title.
	pdf.SetFont(fontFamily, "B", 14)
	pdf.CellFormat(0, 10, col.Label, "", 1, "L", false, 0, "")

	// Subtitle with date.
	pdf.SetFont(fontFamily, "", 9)
	pdf.SetTextColor(128, 128, 128)
	subtitle := fmt.Sprintf("exported: %s  |  %d rows", time.Now().Format("2006-01-02 15:04"), len(records))
	pdf.CellFormat(0, 6, subtitle, "", 1, "L", false, 0, "")
	pdf.Ln(4)
	pdf.SetTextColor(0, 0, 0)

	if len(fields) == 0 || len(records) == 0 {
		pdf.SetFont(fontFamily, "", 10)
		pdf.CellFormat(0, 10, "No data.", "", 1, "C", false, 0, "")
		return pdf
	}

	// Calculate column widths (proportional, capped).
	pageWidth := 277.0 // A4 landscape - margins
	maxCols := 12
	if len(fields) > maxCols {
		fields = fields[:maxCols]
	}
	numCol := len(fields) + 1 // +1 for row number
	colWidth := pageWidth / float64(numCol)
	rowNumWidth := colWidth * 0.5
	dataWidth := (pageWidth - rowNumWidth) / float64(len(fields))

	// Header row.
	pdf.SetFont(fontFamily, "B", 8)
	pdf.SetFillColor(240, 240, 240)
	pdf.CellFormat(rowNumWidth, 7, "#", "1", 0, "C", true, 0, "")
	for _, f := range fields {
		pdf.CellFormat(dataWidth, 7, truncateStr(f.Label, 20), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Data rows.
	pdf.SetFont(fontFamily, "", 7)
	for i, rec := range records {
		// Alternate row colors.
		if i%2 == 0 {
			pdf.SetFillColor(255, 255, 255)
		} else {
			pdf.SetFillColor(248, 248, 248)
		}

		pdf.CellFormat(rowNumWidth, 6, fmt.Sprintf("%d", i+1), "1", 0, "C", true, 0, "")
		for _, f := range fields {
			val := formatPDFValue(rec[f.Slug], f)
			pdf.CellFormat(dataWidth, 6, truncateStr(val, 35), "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
	}

	// Footer.
	pdf.Ln(4)
	pdf.SetFont(fontFamily, "", 7)
	pdf.SetTextColor(128, 128, 128)
	pdf.CellFormat(0, 5, fmt.Sprintf("Topbid - %s  |  %d rows", col.Label, len(records)), "", 1, "R", false, 0, "")

	return pdf
}

func formatPDFValue(v any, f schema.Field) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case time.Time:
		if f.FieldType == schema.FieldDate {
			return val.Format("2006-01-02")
		}
		return val.Format("2006-01-02 15:04")
	case bool:
		if val {
			return "Y"
		}
		return "N"
	case []any:
		parts := make([]string, len(val))
		for i, el := range val {
			parts[i] = fmt.Sprint(el)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(val)
	}
}

func truncateStr(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "..."
}

func isLayoutField(ft schema.FieldType) bool {
	switch ft {
	case "section", "divider", "spacer":
		return true
	default:
		return false
	}
}

// findFontPath returns the path to a bundled CJK-capable font, or "" if none found.
func findFontPath() string {
	candidates := []string{
		"fonts/NotoSansKR-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansKR-Regular.ttf",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
