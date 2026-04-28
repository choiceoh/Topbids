package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/notify"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// ReportHandler handles PDF/email report requests.
type ReportHandler struct {
	dynH  *DynHandler
	email *notify.EmailNotifier // nil if SMTP not configured
}

// NewReportHandler creates a ReportHandler.
func NewReportHandler(dyn *DynHandler, email *notify.EmailNotifier) *ReportHandler {
	return &ReportHandler{dynH: dyn, email: email}
}

// EmailReport generates a PDF and emails it to the specified address.
//
// POST /api/data/{slug}/email-report
// Body: { "to": "user@example.com", "subject": "Report", "message": "See attached." }
func (h *ReportHandler) EmailReport(w http.ResponseWriter, r *http.Request) {
	if h.email == nil {
		writeError(w, http.StatusServiceUnavailable, "email not configured (set SMTP_HOST)")
		return
	}

	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.dynH.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.dynH.checkAccess(w, r, col, "entry_view") {
		return
	}

	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Message string `json:"message"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.To == "" {
		writeError(w, http.StatusBadRequest, "to is required")
		return
	}
	if req.Subject == "" {
		req.Subject = fmt.Sprintf("[Topworks] %s Report", col.Label)
	}

	// Fetch data (same logic as ExportPDF).
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

	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	orderBy := ParseSort(params.Get("sort"), fields)
	selectCols := buildSelectCols(fields, false, &selectColOpts{cache: h.dynH.cache})

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s%s %s LIMIT 5000",
		selectCols, qTable, where, rlsClause, orderBy)

	rows, err := h.dynH.pool.Query(r.Context(), sql, args...)
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

	var visibleFields []schema.Field
	for _, f := range fields {
		if !isLayoutField(f.FieldType) {
			visibleFields = append(visibleFields, f)
		}
	}

	// Generate PDF.
	pdf := buildPDF(col, visibleFields, records)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		handleErr(w, r, err)
		return
	}

	// Build HTML body.
	htmlBody := fmt.Sprintf(`<div style="font-family:sans-serif;color:#333;">
<h2>%s</h2>
<p>%s</p>
<p style="color:#888;font-size:12px;">%d건의 데이터가 첨부된 PDF에 포함되어 있습니다.</p>
</div>`, col.Label, req.Message, len(records))

	filename := fmt.Sprintf("%s_%s.pdf", col.Slug, time.Now().Format("20060102"))
	if err := h.email.SendWithAttachment(req.To, req.Subject, htmlBody, filename, buf.Bytes()); err != nil {
		writeError(w, http.StatusInternalServerError, "email send failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "sent",
		"to":     req.To,
		"rows":   len(records),
	})
}
