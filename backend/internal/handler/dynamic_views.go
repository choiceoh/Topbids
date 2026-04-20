package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/pgutil"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// ---------------------------------------------------------------------------
// Calendar View  GET /api/data/{slug}/calendar
// ---------------------------------------------------------------------------

// calendarSpan is a multi-day event span placed on a specific week row.
type calendarSpan struct {
	Entry    map[string]any `json:"entry"`
	Label    string         `json:"label"`
	StartCol int            `json:"startCol"`
	ColSpan  int            `json:"colSpan"`
	Track    int            `json:"track"`
	IsStart  bool           `json:"isStart"`
	IsEnd    bool           `json:"isEnd"`
}

type calendarWeek struct {
	Start   string                      `json:"start"`
	End     string                      `json:"end"`
	Days    []string                    `json:"days"`
	Spans   []calendarSpan              `json:"spans"`
	Singles map[string][]map[string]any `json:"singles"`
}

type calendarResponse struct {
	Year  int            `json:"year"`
	Month int            `json:"month"`
	Weeks []calendarWeek `json:"weeks"`
}

func (h *DynHandler) CalendarView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()

	yearStr := params.Get("year")
	monthStr := params.Get("month")
	dateFieldSlug := params.Get("date_field")
	endDateFieldSlug := params.Get("end_date_field")

	if dateFieldSlug == "" {
		writeError(w, http.StatusBadRequest, "date_field is required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1970 || year > 2100 {
		writeError(w, http.StatusBadRequest, "invalid year")
		return
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		writeError(w, http.StatusBadRequest, "invalid month (1-12)")
		return
	}

	// Validate date_field exists and is a date/datetime type.
	dateField := findField(fields, dateFieldSlug)
	if dateField == nil || (dateField.FieldType != schema.FieldDate && dateField.FieldType != schema.FieldDatetime) {
		writeError(w, http.StatusBadRequest, "date_field must be a date or datetime field")
		return
	}
	var endDateField *schema.Field
	if endDateFieldSlug != "" {
		endDateField = findField(fields, endDateFieldSlug)
		if endDateField == nil || (endDateField.FieldType != schema.FieldDate && endDateField.FieldType != schema.FieldDatetime) {
			writeError(w, http.StatusBadRequest, "end_date_field must be a date or datetime field")
			return
		}
	}

	// Compute calendar grid boundaries.
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	startDay := int(firstOfMonth.Weekday()) // 0=Sunday
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()

	// Build calendar days: padding nulls + real days + trailing nulls.
	totalCells := startDay + daysInMonth
	for totalCells%7 != 0 {
		totalCells++
	}

	// Build weeks with date strings.
	type weekInfo struct {
		days []string // len=7, "" for padding cells
	}
	weeks := make([]weekInfo, totalCells/7)
	for wi := range weeks {
		weeks[wi].days = make([]string, 7)
	}
	for i := 0; i < totalCells; i++ {
		wi := i / 7
		di := i % 7
		dayNum := i - startDay + 1
		if dayNum >= 1 && dayNum <= daysInMonth {
			weeks[wi].days[di] = fmt.Sprintf("%04d-%02d-%02d", year, month, dayNum)
		}
	}

	// Grid date range for SQL.
	gridStart := weeks[0].days[0]
	if gridStart == "" {
		// First cell is padding → find actual first date.
		for _, d := range weeks[0].days {
			if d != "" {
				gridStart = d
				break
			}
		}
	}
	lastWeek := weeks[len(weeks)-1]
	gridEnd := ""
	for i := 6; i >= 0; i-- {
		if lastWeek.days[i] != "" {
			gridEnd = lastWeek.days[i]
			break
		}
	}

	// Build SQL: fetch entries where date range overlaps the grid.
	qTable := pgutil.QuoteQualified("data", col.Slug)
	endCol := pgutil.QuoteIdent(dateFieldSlug)
	if endDateField != nil {
		endCol = fmt.Sprintf("COALESCE(%q, %q)", endDateFieldSlug, dateFieldSlug)
	}

	where, args, err := parseCalendarFilters(params, fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	dateArg1 := len(args) + 1
	dateArg2 := len(args) + 2
	args = append(args, gridEnd, gridStart)

	procEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	sql := fmt.Sprintf(
		"SELECT %s FROM %s WHERE deleted_at IS NULL %s%s AND %q <= $%d AND %s >= $%d ORDER BY %q ASC LIMIT 1000",
		selectCols, qTable, where, rlsClause,
		dateFieldSlug, dateArg1,
		endCol, dateArg2,
		dateFieldSlug,
	)

	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	records, err := collectRows(rows)
	rows.Close()
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Expand relations/users/computed.
	if expand := params.Get("expand"); expand != "" {
		if err := h.expandRelations(r.Context(), records, fields, expand); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.expandUserFields(r.Context(), records, fields)
	h.resolveComputedFields(r.Context(), records, fields)
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Find title field for labels.
	titleFieldSlug := params.Get("title_field")
	if titleFieldSlug == "" && col.TitleFieldID != "" {
		for _, f := range fields {
			if f.ID == col.TitleFieldID {
				titleFieldSlug = f.Slug
				break
			}
		}
	}
	if titleFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldText {
				titleFieldSlug = f.Slug
				break
			}
		}
	}

	// Build calendar response.
	result := calendarResponse{Year: year, Month: month}
	for _, wi := range weeks {
		cw := calendarWeek{
			Days:    wi.days,
			Spans:   []calendarSpan{},
			Singles: make(map[string][]map[string]any),
		}
		// Find week start/end (non-empty dates).
		for _, d := range wi.days {
			if d != "" {
				cw.Start = d
				break
			}
		}
		for i := 6; i >= 0; i-- {
			if wi.days[i] != "" {
				cw.End = wi.days[i]
				break
			}
		}
		result.Weeks = append(result.Weeks, cw)
	}

	// Assign entries to weeks.
	for _, entry := range records {
		rawStart := toDateStrGo(entry[dateFieldSlug])
		if rawStart == "" {
			continue
		}
		rawEnd := rawStart
		if endDateField != nil {
			if e := toDateStrGo(entry[endDateFieldSlug]); e != "" && e > rawStart {
				rawEnd = e
			}
		}
		isMultiDay := rawEnd != rawStart
		label := "(무제)"
		if titleFieldSlug != "" {
			if v, ok := entry[titleFieldSlug]; ok && v != nil {
				label = fmt.Sprintf("%v", v)
			}
		}

		if !isMultiDay {
			// Single-day event.
			for wi := range result.Weeks {
				for _, d := range result.Weeks[wi].Days {
					if d == rawStart {
						result.Weeks[wi].Singles[d] = append(result.Weeks[wi].Singles[d], entry)
						break
					}
				}
			}
			continue
		}

		// Multi-day: split across weeks.
		for wi := range result.Weeks {
			weekStart := result.Weeks[wi].Start
			weekEnd := result.Weeks[wi].End
			if weekStart == "" || weekEnd == "" {
				continue
			}
			if rawStart > weekEnd || rawEnd < weekStart {
				continue
			}

			clampedStart := rawStart
			if clampedStart < weekStart {
				clampedStart = weekStart
			}
			clampedEnd := rawEnd
			if clampedEnd > weekEnd {
				clampedEnd = weekEnd
			}

			startCol := 0
			endCol := 6
			for di, d := range result.Weeks[wi].Days {
				if d == clampedStart {
					startCol = di
				}
				if d == clampedEnd {
					endCol = di
				}
			}

			result.Weeks[wi].Spans = append(result.Weeks[wi].Spans, calendarSpan{
				Entry:    entry,
				Label:    label,
				StartCol: startCol,
				ColSpan:  endCol - startCol + 1,
				Track:    0,
				IsStart:  rawStart >= weekStart,
				IsEnd:    rawEnd <= weekEnd,
			})
		}
	}

	// Assign tracks per week (greedy algorithm).
	for wi := range result.Weeks {
		spans := result.Weeks[wi].Spans
		sort.Slice(spans, func(i, j int) bool {
			if spans[i].StartCol != spans[j].StartCol {
				return spans[i].StartCol < spans[j].StartCol
			}
			return spans[i].ColSpan > spans[j].ColSpan
		})
		trackEnds := []int{}
		for si := range spans {
			assigned := -1
			for t := 0; t < len(trackEnds); t++ {
				if trackEnds[t] < spans[si].StartCol {
					assigned = t
					break
				}
			}
			if assigned == -1 {
				assigned = len(trackEnds)
				trackEnds = append(trackEnds, 0)
			}
			spans[si].Track = assigned
			trackEnds[assigned] = spans[si].StartCol + spans[si].ColSpan - 1
		}
		result.Weeks[wi].Spans = spans
	}

	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Gantt View  GET /api/data/{slug}/gantt
// ---------------------------------------------------------------------------

type ganttRow struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	StartDate    string   `json:"startDate"`
	EndDate      string   `json:"endDate"`
	Progress     *float64 `json:"progress"`
	ColorKey     string   `json:"colorKey"`
	Dependencies []string `json:"dependencies"`
	User         string   `json:"user,omitempty"`
	Status       string   `json:"status,omitempty"`
}

type ganttRange struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	TotalDays int    `json:"totalDays"`
}

type ganttMonth struct {
	Label      string `json:"label"`
	StartIndex int    `json:"startIndex"`
	Span       int    `json:"span"`
}

type ganttResponse struct {
	Rows   []ganttRow   `json:"rows"`
	Range  ganttRange   `json:"range"`
	Months []ganttMonth `json:"months"`
}

func (h *DynHandler) GanttView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()

	startFieldSlug := params.Get("start_field")
	endFieldSlug := params.Get("end_field")
	titleFieldSlug := params.Get("title_field")
	progressFieldSlug := params.Get("progress_field")
	dependencyFieldSlug := params.Get("dependency_field")
	userFieldSlug := params.Get("user_field")
	statusFieldSlug := params.Get("status_field")

	if startFieldSlug == "" {
		writeError(w, http.StatusBadRequest, "start_field is required")
		return
	}

	startField := findField(fields, startFieldSlug)
	if startField == nil || (startField.FieldType != schema.FieldDate && startField.FieldType != schema.FieldDatetime) {
		writeError(w, http.StatusBadRequest, "start_field must be a date or datetime field")
		return
	}

	// end_field defaults to start_field.
	if endFieldSlug == "" {
		endFieldSlug = startFieldSlug
	}
	endField := findField(fields, endFieldSlug)
	if endField == nil || (endField.FieldType != schema.FieldDate && endField.FieldType != schema.FieldDatetime) {
		writeError(w, http.StatusBadRequest, "end_field must be a date or datetime field")
		return
	}

	// Auto-detect title field.
	if titleFieldSlug == "" && col.TitleFieldID != "" {
		for _, f := range fields {
			if f.ID == col.TitleFieldID {
				titleFieldSlug = f.Slug
				break
			}
		}
	}
	if titleFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldText {
				titleFieldSlug = f.Slug
				break
			}
		}
	}

	// Auto-detect progress field.
	if progressFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldNumber || f.FieldType == schema.FieldInteger {
				if hasDisplayType(f.Options, "progress") {
					progressFieldSlug = f.Slug
					break
				}
				if strings.Contains(f.Slug, "progress") || strings.Contains(f.Label, "진행") {
					progressFieldSlug = f.Slug
					break
				}
			}
		}
	}

	// Auto-detect user field.
	if userFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldUser {
				userFieldSlug = f.Slug
				break
			}
		}
	}

	// Auto-detect status field.
	if statusFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldSelect {
				statusFieldSlug = f.Slug
				break
			}
		}
	}

	// Auto-detect dependency field.
	if dependencyFieldSlug == "" {
		for _, f := range fields {
			if f.FieldType == schema.FieldRelation {
				dependencyFieldSlug = f.Slug
				break
			}
		}
	}

	// Fetch entries (no pagination, with safety cap).
	qTable := pgutil.QuoteQualified("data", col.Slug)
	where, args, err := parseCalendarFilters(params, fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	procEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s%s ORDER BY %q ASC NULLS LAST LIMIT 1000",
		selectCols, qTable, where, rlsClause, startFieldSlug)

	dbRows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	records, err := collectRows(dbRows)
	dbRows.Close()
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Expand relations.
	expandParam := params.Get("expand")
	if expandParam == "" && dependencyFieldSlug != "" {
		expandParam = dependencyFieldSlug
	}
	if expandParam != "" {
		if err := h.expandRelations(r.Context(), records, fields, expandParam); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.expandUserFields(r.Context(), records, fields)
	h.resolveComputedFields(r.Context(), records, fields)
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Build gantt rows.
	var gRows []ganttRow
	var allDates []time.Time

	for _, rec := range records {
		startStr := toDateStrGo(rec[startFieldSlug])
		endStr := toDateStrGo(rec[endFieldSlug])
		if startStr == "" {
			startStr = endStr
		}
		if endStr == "" || endStr < startStr {
			endStr = startStr
		}
		if startStr == "" {
			continue
		}

		title := "(무제)"
		if titleFieldSlug != "" {
			if v := rec[titleFieldSlug]; v != nil {
				title = fmt.Sprintf("%v", v)
			}
		}

		var progress *float64
		if progressFieldSlug != "" {
			if v := rec[progressFieldSlug]; v != nil {
				if n, ok := v.(float64); ok {
					progress = &n
				}
			}
		}

		userName := ""
		if userFieldSlug != "" {
			userName = extractDisplayName(rec[userFieldSlug])
		}

		status := ""
		if statusFieldSlug != "" {
			if v := rec[statusFieldSlug]; v != nil {
				status = fmt.Sprintf("%v", v)
			}
		}

		colorKey := userName
		if colorKey == "" {
			colorKey = status
		}

		deps := extractDependencyIDs(rec[dependencyFieldSlug])

		gRows = append(gRows, ganttRow{
			ID:           fmt.Sprintf("%v", rec["id"]),
			Title:        title,
			StartDate:    startStr,
			EndDate:      endStr,
			Progress:     progress,
			ColorKey:     colorKey,
			Dependencies: deps,
			User:         userName,
			Status:       status,
		})

		if t, err := time.Parse("2006-01-02", startStr); err == nil {
			allDates = append(allDates, t)
		}
		if t, err := time.Parse("2006-01-02", endStr); err == nil {
			allDates = append(allDates, t)
		}
	}

	// Compute date range.
	var rangeStart, rangeEnd time.Time
	if len(allDates) == 0 {
		now := time.Now()
		rangeStart = now.AddDate(0, 0, -7)
		rangeEnd = now.AddDate(0, 0, 30)
	} else {
		rangeStart = allDates[0]
		rangeEnd = allDates[0]
		for _, d := range allDates[1:] {
			if d.Before(rangeStart) {
				rangeStart = d
			}
			if d.After(rangeEnd) {
				rangeEnd = d
			}
		}
		rangeStart = rangeStart.AddDate(0, 0, -7)
		rangeEnd = rangeEnd.AddDate(0, 0, 14)
	}
	totalDays := int(rangeEnd.Sub(rangeStart).Hours()/24) + 1

	// Generate month headers.
	var months []ganttMonth
	currentMonth := -1
	currentYear := -1
	for i := 0; i < totalDays; i++ {
		d := rangeStart.AddDate(0, 0, i)
		m := int(d.Month())
		y := d.Year()
		if m != currentMonth || y != currentYear {
			currentMonth = m
			currentYear = y
			months = append(months, ganttMonth{
				Label:      fmt.Sprintf("%d년 %d월", y, m),
				StartIndex: i,
				Span:       1,
			})
		} else {
			months[len(months)-1].Span++
		}
	}

	resp := ganttResponse{
		Rows: gRows,
		Range: ganttRange{
			Start:     rangeStart.Format("2006-01-02"),
			End:       rangeEnd.Format("2006-01-02"),
			TotalDays: totalDays,
		},
		Months: months,
	}
	if resp.Rows == nil {
		resp.Rows = []ganttRow{}
	}
	if resp.Months == nil {
		resp.Months = []ganttMonth{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Kanban View  GET /api/data/{slug}/kanban
// ---------------------------------------------------------------------------

type kanbanColumn struct {
	Value   string           `json:"value"`
	Label   string           `json:"label"`
	Color   string           `json:"color,omitempty"`
	Entries []map[string]any `json:"entries"`
}

type kanbanResponse struct {
	Columns      []kanbanColumn      `json:"columns"`
	AllowedMoves map[string][]string `json:"allowed_moves,omitempty"`
}

func (h *DynHandler) KanbanView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()
	groupFieldSlug := params.Get("group_field")
	if groupFieldSlug == "" {
		writeError(w, http.StatusBadRequest, "group_field is required")
		return
	}

	groupField := findField(fields, groupFieldSlug)
	if groupField == nil || groupField.FieldType != schema.FieldSelect {
		writeError(w, http.StatusBadRequest, "group_field must be a select field")
		return
	}

	choices, _ := schema.ExtractChoices(groupField.Options)
	if choices == nil {
		choices = []string{}
	}

	// Fetch entries (no pagination).
	qTable := pgutil.QuoteQualified("data", col.Slug)
	where, args, err := parseCalendarFilters(params, fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	procEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s%s ORDER BY _created_at DESC LIMIT 500",
		selectCols, qTable, where, rlsClause)

	dbRows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	records, err := collectRows(dbRows)
	dbRows.Close()
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Expand relations/users/computed.
	if expand := params.Get("expand"); expand != "" {
		if err := h.expandRelations(r.Context(), records, fields, expand); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.expandUserFields(r.Context(), records, fields)
	h.resolveComputedFields(r.Context(), records, fields)
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Get choice colors from options.
	choiceColors := extractChoiceColors(groupField.Options)

	// Group entries by column value.
	columnMap := make(map[string][]map[string]any)
	choiceSet := make(map[string]bool, len(choices))
	for _, c := range choices {
		choiceSet[c] = true
		columnMap[c] = nil // ensure key exists
	}

	for _, rec := range records {
		val := ""
		if v := rec[groupFieldSlug]; v != nil {
			val = fmt.Sprintf("%v", v)
		}
		if val == "" || !choiceSet[val] {
			columnMap["__none__"] = append(columnMap["__none__"], rec)
		} else {
			columnMap[val] = append(columnMap[val], rec)
		}
	}

	// Build columns in choice order.
	var columns []kanbanColumn
	for _, c := range choices {
		entries := columnMap[c]
		if entries == nil {
			entries = []map[string]any{}
		}
		columns = append(columns, kanbanColumn{
			Value:   c,
			Label:   c,
			Color:   choiceColors[c],
			Entries: entries,
		})
	}
	if uncategorized, ok := columnMap["__none__"]; ok && len(uncategorized) > 0 {
		columns = append(columns, kanbanColumn{
			Value:   "__none__",
			Label:   "미분류",
			Entries: uncategorized,
		})
	}

	resp := kanbanResponse{Columns: columns}

	// Build allowed_moves from process transitions if applicable.
	if groupFieldSlug == "_status" {
		proc, hasProc := h.cache.ProcessByCollectionID(col.ID)
		if hasProc && proc.IsEnabled {
			user, _ := middleware.GetUser(r.Context())
			resp.AllowedMoves = buildAllowedMoves(proc, user.Role, user.UserID)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hasDisplayType checks if a field's options JSON has a specific display_type.
func hasDisplayType(opts json.RawMessage, dt string) bool {
	if len(opts) == 0 {
		return false
	}
	var parsed map[string]any
	if err := json.Unmarshal(opts, &parsed); err != nil {
		return false
	}
	v, _ := parsed["display_type"].(string)
	return v == dt
}

// findField finds a field by slug in the given slice.
func findField(fields []schema.Field, slug string) *schema.Field {
	for i := range fields {
		if fields[i].Slug == slug {
			return &fields[i]
		}
	}
	return nil
}

// toDateStrGo extracts YYYY-MM-DD from various date representations.
func toDateStrGo(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case time.Time:
		return val.Format("2006-01-02")
	case string:
		if len(val) >= 10 {
			s := val[:10]
			if _, err := time.Parse("2006-01-02", s); err == nil {
				return s
			}
		}
		return ""
	default:
		s := fmt.Sprintf("%v", val)
		if len(s) >= 10 {
			candidate := s[:10]
			if _, err := time.Parse("2006-01-02", candidate); err == nil {
				return candidate
			}
		}
		return ""
	}
}

// extractDisplayName extracts a display name from a user field value.
func extractDisplayName(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case map[string]any:
		for _, key := range []string{"name", "email", "id"} {
			if n, ok := val[key]; ok && n != nil {
				return fmt.Sprintf("%v", n)
			}
		}
	case string:
		return val
	}
	return ""
}

// extractDependencyIDs extracts relation IDs from a field value.
func extractDependencyIDs(v any) []string {
	if v == nil {
		return []string{}
	}
	switch val := v.(type) {
	case []any:
		ids := make([]string, 0, len(val))
		for _, item := range val {
			switch it := item.(type) {
			case map[string]any:
				if id, ok := it["id"]; ok {
					ids = append(ids, fmt.Sprintf("%v", id))
				}
			case string:
				ids = append(ids, it)
			default:
				ids = append(ids, fmt.Sprintf("%v", it))
			}
		}
		return ids
	case map[string]any:
		if id, ok := val["id"]; ok {
			return []string{fmt.Sprintf("%v", id)}
		}
	case string:
		if val != "" {
			return []string{val}
		}
	}
	return []string{}
}

// extractChoiceColors parses select field options for color associations.
func extractChoiceColors(opts []byte) map[string]string {
	colors := make(map[string]string)
	if len(opts) == 0 {
		return colors
	}
	// Options may have a "choice_colors" map.
	var parsed map[string]any
	if err := json.Unmarshal(opts, &parsed); err != nil {
		return colors
	}
	if cc, ok := parsed["choice_colors"].(map[string]any); ok {
		for k, v := range cc {
			colors[k] = fmt.Sprintf("%v", v)
		}
	}
	return colors
}

// buildAllowedMoves constructs the allowed_moves map from process transitions.
func buildAllowedMoves(proc schema.Process, userRole, userID string) map[string][]string {
	// Build ID → name lookup.
	idToName := make(map[string]string, len(proc.Statuses))
	for _, s := range proc.Statuses {
		idToName[s.ID] = s.Name
	}

	moves := make(map[string][]string)
	for _, s := range proc.Statuses {
		moves[s.Name] = []string{}
	}

	for _, t := range proc.Transitions {
		if !isTransitionAllowed(t, userRole, userID) {
			continue
		}
		fromName := idToName[t.FromStatusID]
		toName := idToName[t.ToStatusID]
		if fromName != "" && toName != "" {
			moves[fromName] = append(moves[fromName], toName)
		}
	}

	return moves
}

// isTransitionAllowed checks if a user (by role and/or ID) is permitted to perform a transition.
func isTransitionAllowed(t schema.ProcessTransition, userRole, userID string) bool {
	if len(t.AllowedRoles) == 0 && len(t.AllowedUserIDs) == 0 {
		return true
	}
	for _, r := range t.AllowedRoles {
		if r == userRole {
			return true
		}
	}
	for _, uid := range t.AllowedUserIDs {
		if uid == userID {
			return true
		}
	}
	return false
}

// parseCalendarFilters is like ParseFilters but excludes view-specific params.
// Also supports the _filter JSON param for AND/OR group filtering.
func parseCalendarFilters(params url.Values, fields []schema.Field) (string, []any, error) {
	// If _filter is present, use JSON filter parsing directly.
	if jsonFilter := params.Get("_filter"); jsonFilter != "" {
		return ParseJSONFilter(jsonFilter, fields, "")
	}

	// Strip view-specific params before passing to ParseFilters.
	cleaned := make(url.Values)
	viewParams := map[string]bool{
		"year": true, "month": true, "date_field": true, "end_date_field": true,
		"title_field": true, "start_field": true, "end_field": true,
		"progress_field": true, "dependency_field": true, "user_field": true,
		"status_field": true, "group_field": true, "format": true,
	}
	for k, v := range params {
		if !viewParams[k] {
			cleaned[k] = v
		}
	}
	return ParseFilters(cleaned, fields)
}
