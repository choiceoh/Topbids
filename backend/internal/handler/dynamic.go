package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/bid"
	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/formula"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/pgutil"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, allowing
// helpers like syncM2MLinks to run inside or outside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DynHandler serves the Dynamic API (/api/data/...).
// It builds SQL queries at runtime based on the meta-table cache.
type DynHandler struct {
	pool  *pgxpool.Pool
	cache *schema.Cache
	bus   *events.Bus
}

func NewDynHandler(pool *pgxpool.Pool, cache *schema.Cache, bus *events.Bus) *DynHandler {
	return &DynHandler{pool: pool, cache: cache, bus: bus}
}

// --- List ---

// List returns a paginated, filtered, sorted list of records from a dynamic data
// table. The full query pipeline is:
//
//  1. Resolve the collection from the schema cache by URL slug
//  2. Check access: verify the authenticated user's role is allowed entry_view
//  3. Parse pagination (page/limit), sort spec (with relation LEFT JOINs for
//     dot-notation like "-subsidiary.name"), filters (JSON _filter or legacy
//     query params), and text search (?q= across all text/textarea fields via
//     GIN-indexed _tsv column)
//  4. Build an RLS (row-level security) clause for viewer-role users based on
//     the collection's rls_mode (creator/department/subsidiary/filter)
//  5. Execute COUNT(*) for total, then a paginated SELECT with all clauses
//  6. Post-processing: expand relations (?expand=), auto-expand user fields,
//     resolve computed fields (formula/lookup/rollup), load M:N links, and
//     optionally apply display formatting (?format=display)
func (h *DynHandler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()
	page, limit, offset := ParsePagination(params)
	qTable := pgutil.QuoteQualified("data", col.Slug)

	// Resolve relation targets for dot-notation sorts (e.g. "-subsidiary.name").
	resolveRel := func(f schema.Field) (string, bool) {
		if f.Relation == nil {
			return "", false
		}
		target, ok := h.cache.CollectionByID(f.Relation.TargetCollectionID)
		if !ok {
			return "", false
		}
		return pgutil.QuoteQualified("data", target.Slug), true
	}
	sortParam := params.Get("sort")
	if sortParam == "" && col.DefaultSortField != "" {
		prefix := ""
		if col.DefaultSortOrder == "desc" || col.DefaultSortOrder == "" {
			prefix = "-"
		}
		sortParam = prefix + col.DefaultSortField
	}
	orderBy, sortJoins := ParseSortWithRelations(sortParam, fields, resolveRel)

	// Filters: support both legacy query params and JSON _filter param.
	var (
		where string
		args  []any
		err   error
	)
	if jsonFilter := params.Get("_filter"); jsonFilter != "" {
		prefix := ""
		if len(sortJoins) > 0 {
			prefix = qTable
		}
		where, args, err = ParseJSONFilter(jsonFilter, fields, prefix)
	} else if len(sortJoins) > 0 {
		where, args, err = ParseFiltersWithPrefix(params, fields, qTable)
	} else {
		where, args, err = ParseFilters(params, fields)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Text search: ?q=term searches across all text/textarea fields.
	searchClause, searchArgs := BuildSearchClause(
		params.Get("q"), fields,
		func() string {
			if len(sortJoins) > 0 {
				return qTable
			}
			return ""
		}(),
		len(args)+1,
	)
	where += " " + searchClause
	args = append(args, searchArgs...)

	// Row-level security: restrict row visibility based on collection's rls_mode.
	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, func() string {
			if len(sortJoins) > 0 {
				return qTable
			}
			return ""
		}())
	}

	// Count total. Sort joins are not needed for COUNT, but we use the same
	// WHERE prefix to keep parameter ordering consistent.
	deletedClause := "deleted_at IS NULL"
	if len(sortJoins) > 0 {
		deletedClause = fmt.Sprintf("%s.deleted_at IS NULL", qTable)
	}
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s %s%s", qTable, deletedClause, where, rlsClause)
	var total int64
	if err := h.pool.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		handleErr(w, r, err)
		return
	}

	// Build optional LEFT JOINs for relation sorting.
	joinClause := ""
	for _, j := range sortJoins {
		joinClause += fmt.Sprintf(" LEFT JOIN %s AS %s ON %s.%q = %s.id",
			j.TargetTable, j.Alias, qTable, j.OwnerColumn, j.Alias,
		)
	}

	// Fetch page. Qualify SELECT columns with the table name when joins are present.
	procEnabled := h.hasProcessEnabled(col.ID)
	var selectCols string
	if joinClause != "" {
		selectCols = qualifySelectCols(fields, qTable, procEnabled, &selectColOpts{cache: h.cache})
	} else {
		selectCols = buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})
	}
	dataSQL := fmt.Sprintf("SELECT %s FROM %s%s WHERE %s %s%s %s LIMIT %d OFFSET %d",
		selectCols, qTable, joinClause, deletedClause, where, rlsClause, orderBy, limit, offset)

	rows, err := h.pool.Query(r.Context(), dataSQL, args...)
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

	// Optional relation expansion.
	if expand := params.Get("expand"); expand != "" {
		expand = resolveAutoExpand(expand, fields)
		if expand != "" {
			if err := h.expandRelations(r.Context(), records, fields, expand); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	// Auto-expand user fields.
	h.expandUserFields(r.Context(), records, fields)

	// Resolve computed fields (formula, lookup, rollup).
	h.resolveComputedFields(r.Context(), records, fields)

	// Load M:N links.
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Topbids: mask sealed fields on bid-role collections (after relation
	// expansion so cross-collection sealed_until_at refs resolve).
	if col.AccessConfig.BidRole == schema.BidRoleBid {
		user, _ := middleware.GetUser(r.Context())
		now := time.Now()
		for _, rec := range records {
			bid.MaskSealedFields(fields, rec, "status", user.Role, now)
		}
	}

	// Optional display formatting.
	if params.Get("format") == "display" {
		applyDisplayFormat(records, fields)
	}

	writeList(w, records, total, page, limit)
}

// --- Get ---

func (h *DynHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	procEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	// RLS: restrict row visibility based on collection's rls_mode.
	getArgs := []any{id}
	rlsGet := ""
	if colRole := middleware.GetCollectionRole(r.Context()); colRole == "viewer" {
		rlsGet = buildRLSClause(r, col, &getArgs, "")
	}

	getSQL := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1 AND deleted_at IS NULL%s", selectCols, qTable, rlsGet)

	rows, err := h.pool.Query(r.Context(), getSQL, getArgs...)
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
	if len(records) == 0 {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	// Optional relation expansion.
	if expand := r.URL.Query().Get("expand"); expand != "" {
		expand = resolveAutoExpand(expand, fields)
		if expand != "" {
			if err := h.expandRelations(r.Context(), records, fields, expand); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	// Auto-expand user fields.
	h.expandUserFields(r.Context(), records, fields)

	// Resolve computed fields (formula, lookup, rollup).
	h.resolveComputedFields(r.Context(), records, fields)

	// Load M:N links.
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Topbids: mask sealed fields on bid-role collections (see List).
	if col.AccessConfig.BidRole == schema.BidRoleBid {
		user, _ := middleware.GetUser(r.Context())
		bid.MaskSealedFields(fields, records[0], "status", user.Role, time.Now())
	}

	// Optional display formatting.
	if r.URL.Query().Get("format") == "display" {
		applyDisplayFormat(records, fields)
	}

	writeJSON(w, http.StatusOK, records[0])
}

// --- Create ---

// Create inserts a new record into a dynamic data table. After validating the
// payload against the collection's field definitions, it builds an INSERT
// statement, auto-sets created_by from the authenticated user, and injects the
// initial process status if the collection has an active workflow. M:N relation
// values are separated and synced to their junction tables after the main INSERT.
//
// On success, the handler records a change history entry, resolves computed fields,
// loads M:N links, and publishes an EventRecordCreate event to the automation bus
// so that automation rules (e.g., notifications, webhooks) can fire.
func (h *DynHandler) Create(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_create") {
		return
	}

	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validatePayload(r.Context(), h.pool, h.cache, body, fields, true); err != nil {
		handleErr(w, r, err)
		return
	}

	// Separate M:N field values — they go to junction tables, not the main INSERT.
	m2mValues := make(map[string][]string)
	for _, f := range fields {
		if !f.IsManyToMany() {
			continue
		}
		v, exists := body[f.Slug]
		if !exists {
			continue
		}
		m2mValues[f.Slug] = toStringSlice(v)
		delete(body, f.Slug)
	}

	// Build INSERT.
	colNames := []string{}
	placeholders := []string{}
	args := []any{}
	idx := 1
	for _, f := range fields {
		if f.FieldType.NoColumn() || f.IsManyToMany() {
			continue
		}
		v, exists := body[f.Slug]
		if !exists {
			continue
		}
		colNames = append(colNames, pgutil.QuoteIdent(f.Slug))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, coerceValue(v, f.FieldType))
		idx++
	}

	// Auto-set created_by from authenticated user.
	user, _ := middleware.GetUser(r.Context())
	colNames = append(colNames, `"created_by"`)
	placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
	args = append(args, user.UserID)
	idx++

	// Process: inject initial status for new entries.
	procEnabled := h.hasProcessEnabled(col.ID)
	if procEnabled {
		if initStatus := h.initialStatusName(col.ID); initStatus != "" {
			colNames = append(colNames, `"_status"`)
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			args = append(args, initStatus)
		}
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	var sql string
	if len(colNames) == 0 {
		// No recognized fields in body — insert a row with all auto-defaults.
		// PostgreSQL requires `DEFAULT VALUES` syntax for this; `() VALUES ()` is invalid.
		sql = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES RETURNING %s", qTable, selectCols)
	} else {
		sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
			qTable,
			strings.Join(colNames, ", "),
			strings.Join(placeholders, ", "),
			selectCols,
		)
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	rows, err := tx.Query(r.Context(), sql, args...)
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
	if len(records) == 0 {
		writeError(w, http.StatusInternalServerError, "insert returned no rows")
		return
	}

	// Sync M:N junction tables.
	if recID, ok := records[0]["id"].(string); ok {
		for _, f := range fields {
			if !f.IsManyToMany() {
				continue
			}
			ids, exists := m2mValues[f.Slug]
			if !exists {
				continue
			}
			if err := h.syncM2MLinks(r.Context(), tx, col.Slug, f, recID, ids); err != nil {
				handleErr(w, r, err)
				return
			}
			records[0][f.Slug] = ids
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		handleErr(w, r, err)
		return
	}

	// Record change history.
	if recID, ok := records[0]["id"].(string); ok {
		diff := createDiff(records[0], fields)
		recordChange(r.Context(), h.pool, col.ID, recID, user.UserID, user.Name, "create", diff)
	}

	// Resolve computed fields for the created record.
	h.resolveComputedFields(r.Context(), records, fields)

	// Load M:N links for response.
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Publish automation event.
	if recID, ok := records[0]["id"].(string); ok {
		h.bus.Publish(r.Context(), events.Event{
			Type:           events.EventRecordCreate,
			CollectionID:   col.ID,
			CollectionSlug: col.Slug,
			RecordID:       recID,
			ActorUserID:    user.UserID,
			ActorName:      user.Name,
			NewRecord:      records[0],
		})
	}

	writeJSON(w, http.StatusCreated, records[0])
}

// --- Update ---

// Update performs a partial update on an existing record. It fetches the current
// row for change history diffing and process transition validation, then builds
// a dynamic UPDATE SET clause from only the provided fields. Key behaviors:
//
//   - Optimistic locking: if the request body includes _version, the WHERE clause
//     checks it matches the current version; a mismatch returns 409 Conflict.
//   - Process transitions: if the collection has process_enabled and _status is
//     being changed, the transition is validated against allowed roles/users.
//   - RLS: viewers can only update rows they created.
//   - After commit: records a change history diff, resolves computed fields,
//     syncs M:N junction tables, and publishes EventRecordUpdate (plus a
//     separate EventStateChange if the status changed) for automation triggers.
func (h *DynHandler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_edit") {
		return
	}

	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validatePayload(r.Context(), h.pool, h.cache, body, fields, false); err != nil {
		handleErr(w, r, err)
		return
	}

	// Separate M:N field values — they go to junction tables, not the main UPDATE.
	m2mValues := make(map[string][]string)
	for _, f := range fields {
		if !f.IsManyToMany() {
			continue
		}
		v, exists := body[f.Slug]
		if !exists {
			continue
		}
		m2mValues[f.Slug] = toStringSlice(v)
		delete(body, f.Slug)
	}

	// Fetch current row for transition check and change history.
	oldRow, err := h.fetchRow(r.Context(), col, fields, id)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	user, _ := middleware.GetUser(r.Context())

	// Process transition check: if collection has process_enabled, enforce transition rules.
	if col.ProcessEnabled {
		if err := checkTransitions(oldRow, body, fields, user.Role); err != nil {
			handleErr(w, r, err)
			return
		}
	}

	// Optimistic locking: extract _version from request body.
	var reqVersion *int
	if v, ok := body["_version"]; ok {
		switch n := v.(type) {
		case float64:
			iv := int(n)
			reqVersion = &iv
		case int:
			reqVersion = &n
		}
		delete(body, "_version")
	}

	sets := []string{`"updated_at" = now()`, `"_version" = "_version" + 1`}
	args := []any{}
	idx := 1

	// Auto-set updated_by from authenticated user.
	sets = append(sets, fmt.Sprintf(`"updated_by" = $%d`, idx))
	args = append(args, user.UserID)
	idx++

	for _, f := range fields {
		if f.FieldType.NoColumn() || f.FieldType == schema.FieldAutonumber || f.IsManyToMany() {
			continue
		}
		v, exists := body[f.Slug]
		if !exists {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", pgutil.QuoteIdent(f.Slug), idx))
		args = append(args, coerceValue(v, f.FieldType))
		idx++
	}

	// Process: validate and apply status transition.
	procEnabled := h.hasProcessEnabled(col.ID)
	if newStatus, ok := body["_status"]; ok && newStatus != nil {
		if !procEnabled {
			writeError(w, http.StatusBadRequest, "이 앱에는 프로세스가 활성화되지 않았습니다")
			return
		}
		newStatusStr, ok := newStatus.(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "_status는 문자열이어야 합니다")
			return
		}
		// Fetch current status.
		qTable := pgutil.QuoteQualified("data", col.Slug)
		var currentStatus *string
		err := h.pool.QueryRow(r.Context(),
			fmt.Sprintf(`SELECT "_status" FROM %s WHERE id = $1 AND deleted_at IS NULL`, qTable), id,
		).Scan(&currentStatus)
		if err != nil {
			handleErr(w, r, err)
			return
		}
		fromStatus := ""
		if currentStatus != nil {
			fromStatus = *currentStatus
		}
		if err := h.validateStatusTransition(col.ID, fromStatus, newStatusStr, user.Role, user.UserID); err != nil {
			handleErr(w, r, err)
			return
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", pgutil.QuoteIdent("_status"), idx))
		args = append(args, newStatusStr)
		idx++
	}

	args = append(args, id)
	idIdx := idx
	idx++
	qTable := pgutil.QuoteQualified("data", col.Slug)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})

	// Optimistic locking: add version check to WHERE clause.
	versionClause := ""
	if reqVersion != nil {
		args = append(args, *reqVersion)
		versionClause = fmt.Sprintf(` AND "_version" = $%d`, idx)
		idx++
	}

	// RLS: viewers can only update their own rows.
	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		args = append(args, user.UserID)
		rlsClause = fmt.Sprintf(" AND created_by = $%d", idx)
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d AND deleted_at IS NULL%s%s RETURNING %s",
		qTable, strings.Join(sets, ", "), idIdx, versionClause, rlsClause, selectCols)

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
	if len(records) == 0 {
		// Distinguish version conflict from genuine not-found.
		if reqVersion != nil {
			writeError(w, http.StatusConflict, "다른 사용자가 이미 이 레코드를 수정했습니다. 새로고침 후 다시 시도해 주세요.")
			return
		}
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	// Sync M:N junction tables.
	for _, f := range fields {
		if !f.IsManyToMany() {
			continue
		}
		ids, exists := m2mValues[f.Slug]
		if !exists {
			continue
		}
		if err := h.syncM2MLinks(r.Context(), h.pool, col.Slug, f, id, ids); err != nil {
			handleErr(w, r, err)
			return
		}
	}

	// Record change history.
	diff := computeDiff(oldRow, records[0], fields)

	// Detect status change early (used for both history enrichment and events).
	oldStatus, _ := oldRow["_status"].(string)
	newStatus, _ := records[0]["_status"].(string)
	statusChanged := oldStatus != "" && newStatus != "" && newStatus != oldStatus

	// Enrich diff with status change metadata for prominent history display.
	if statusChanged {
		sc := map[string]any{
			"from":  oldStatus,
			"to":    newStatus,
			"actor": user.Name,
		}
		if proc, ok := h.cache.ProcessByCollectionID(col.ID); ok {
			for _, s := range proc.Statuses {
				if s.Name == oldStatus {
					sc["from_color"] = s.Color
				}
				if s.Name == newStatus {
					sc["to_color"] = s.Color
				}
			}
		}
		diff["_status_change"] = sc
	}

	if len(diff) > 0 {
		recordChange(r.Context(), h.pool, col.ID, id, user.UserID, user.Name, "update", diff)
	}

	// Resolve computed fields for the updated record.
	h.resolveComputedFields(r.Context(), records, fields)

	// Load M:N links for response.
	h.loadM2MFields(r.Context(), records, fields, col.Slug)

	// Publish automation event.
	ev := events.Event{
		Type:           events.EventRecordUpdate,
		CollectionID:   col.ID,
		CollectionSlug: col.Slug,
		RecordID:       id,
		ActorUserID:    user.UserID,
		ActorName:      user.Name,
		OldRecord:      oldRow,
		NewRecord:      records[0],
	}
	if statusChanged {
		ev.StatusFrom = oldStatus
		ev.StatusTo = newStatus
	}
	h.bus.Publish(r.Context(), ev)
	// Publish a separate EventStateChange so notification/automation subscribers fire.
	if ev.StatusFrom != "" && ev.StatusTo != "" {
		stateEv := ev
		stateEv.Type = events.EventStateChange
		h.bus.Publish(r.Context(), stateEv)
	}

	writeJSON(w, http.StatusOK, records[0])
}

// --- Aggregate ---

// Totals returns aggregate values (sum, avg, count, min, max) for all numeric
// fields across the full (filtered + RLS-restricted) dataset — no GROUP BY.
//
// GET /api/data/{slug}/totals
//
// Response: { "_count": N, "field_slug": { "sum": N, "avg": N, "min": N, "max": N }, ... }
func (h *DynHandler) Totals(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	// Collect numeric fields.
	var numFields []schema.Field
	for _, f := range fields {
		if f.FieldType == schema.FieldNumber || f.FieldType == schema.FieldInteger || f.FieldType == schema.FieldAutonumber {
			numFields = append(numFields, f)
		}
	}
	if len(numFields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	params := r.URL.Query()
	qTable := pgutil.QuoteQualified("data", col.Slug)

	var (
		where string
		args  []any
		err   error
	)
	if jsonFilter := params.Get("_filter"); jsonFilter != "" {
		where, args, err = ParseJSONFilter(jsonFilter, fields, "")
	} else {
		where, args, err = ParseFilters(params, fields)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Text search.
	searchClause, searchArgs := BuildSearchClause(params.Get("q"), fields, "", len(args)+1)
	where += " " + searchClause
	args = append(args, searchArgs...)

	// RLS.
	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		rlsClause = buildRLSClause(r, col, &args, "")
	}

	// Build SELECT with all five aggregation functions for each numeric field.
	var selectParts []string
	selectParts = append(selectParts, "COUNT(*) AS _count")
	for _, f := range numFields {
		q := pgutil.QuoteIdent(f.Slug)
		selectParts = append(selectParts,
			fmt.Sprintf("SUM(%s) AS %q", q, "sum_"+f.Slug),
			fmt.Sprintf("AVG(%s) AS %q", q, "avg_"+f.Slug),
			fmt.Sprintf("MIN(%s) AS %q", q, "min_"+f.Slug),
			fmt.Sprintf("MAX(%s) AS %q", q, "max_"+f.Slug),
		)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL %s%s",
		strings.Join(selectParts, ", "), qTable, where, rlsClause)

	row := h.pool.QueryRow(r.Context(), sql, args...)

	// Scan all columns: 1 (count) + 4 per numeric field.
	nCols := 1 + len(numFields)*4
	vals := make([]any, nCols)
	ptrs := make([]any, nCols)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		handleErr(w, r, err)
		return
	}

	totalCount := normalizeValue(vals[0])
	result := make(map[string]any, len(numFields)+1)
	result["_count"] = totalCount
	for i, f := range numFields {
		base := 1 + i*4
		result[f.Slug] = map[string]any{
			"sum": normalizeValue(vals[base]),
			"avg": normalizeValue(vals[base+1]),
			"min": normalizeValue(vals[base+2]),
			"max": normalizeValue(vals[base+3]),
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// Aggregate runs simple GROUP BY queries for dashboard widgets.
// Query params:
//
//	group=field_slug   — required, must be a non-relation column on this collection
//	fn=count|sum|avg|min|max — default: count
//	field=field_slug   — required for sum/avg/min/max; ignored for count
//	filter passthrough — same WHERE syntax as List
//
// Supports:
//   - Multiple groups: ?group=status&group=department → GROUP BY status, department
//   - Date interval:   ?group=created_at&interval=month → DATE_TRUNC('month', created_at)
//   - Multiple series: ?fn=count&fn=sum&field=amount → multiple aggregation columns
//
// Response: [{ "groups": [...], "values": { "count": N, "sum_amount": N } }, ...]
// Legacy single-group response: [{ "group": <value>, "value": <number> }, ...]
func (h *DynHandler) Aggregate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	params := r.URL.Query()
	groupSlugs := params["group"]
	if len(groupSlugs) == 0 {
		writeError(w, http.StatusBadRequest, "group parameter is required")
		return
	}

	result, err := runAggregate(r.Context(), h.pool, col, fields, aggregateParams{
		Groups:   groupSlugs,
		Fns:      params["fn"],
		Fields:   params["field"],
		Interval: params.Get("interval"),
		Filters:  params,
	})
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// BatchAggregate runs multiple aggregate queries in a single request.
func (h *DynHandler) BatchAggregate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	var req struct {
		Queries []batchAggQuery `json:"queries"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Queries) == 0 {
		writeError(w, http.StatusBadRequest, "queries array is required")
		return
	}
	if len(req.Queries) > 10 {
		writeError(w, http.StatusBadRequest, "maximum 10 queries per batch")
		return
	}

	results := make([]any, 0, len(req.Queries))
	for _, q := range req.Queries {
		if len(q.Groups) == 0 {
			writeError(w, http.StatusBadRequest, "each query requires at least one group")
			return
		}
		result, err := runAggregate(r.Context(), h.pool, col, fields, aggregateParams{
			Groups:   q.Groups,
			Fns:      q.Fns,
			Fields:   q.Fields,
			Interval: q.Interval,
			Filters:  r.URL.Query(), // shared filters from query string
		})
		if err != nil {
			handleErr(w, r, err)
			return
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, results)
}

type batchAggQuery struct {
	Groups   []string `json:"group"`
	Fns      []string `json:"fn"`
	Fields   []string `json:"field"`
	Interval string   `json:"interval"`
}

// aggregateParams holds the parameters for a single aggregate query.
type aggregateParams struct {
	Groups   []string
	Fns      []string
	Fields   []string
	Interval string
	Filters  url.Values
}

// runAggregate executes a single aggregate query and returns the result.
func runAggregate(ctx context.Context, pool *pgxpool.Pool, col schema.Collection, fields []schema.Field, p aggregateParams) (any, error) {
	bySlug := make(map[string]schema.Field, len(fields))
	for _, f := range fields {
		bySlug[f.Slug] = f
	}

	// Valid date intervals for DATE_TRUNC.
	validIntervals := map[string]bool{
		"year": true, "quarter": true, "month": true, "week": true, "day": true, "hour": true,
	}
	interval := strings.ToLower(p.Interval)
	if interval != "" && !validIntervals[interval] {
		return nil, fmt.Errorf("%w: invalid interval %q; must be year/quarter/month/week/day/hour", schema.ErrInvalidInput, interval)
	}

	// Auto columns allowed for grouping.
	autoColumns := map[string]bool{"created_at": true, "updated_at": true, "deleted_at": true}

	// Build GROUP BY expressions.
	var groupExprs []string
	var groupAliases []string
	for i, gs := range p.Groups {
		_, isField := bySlug[gs]
		if !isField && !autoColumns[gs] {
			return nil, fmt.Errorf("%w: group field %q not found", schema.ErrInvalidInput, gs)
		}
		alias := fmt.Sprintf("g%d", i)
		qCol := pgutil.QuoteIdent(gs)
		if interval != "" && (gs == "created_at" || gs == "updated_at" || gs == "deleted_at" ||
			(isField && (bySlug[gs].FieldType == schema.FieldDate || bySlug[gs].FieldType == schema.FieldDatetime))) {
			groupExprs = append(groupExprs, fmt.Sprintf("DATE_TRUNC('%s', %s) AS %s", interval, qCol, alias))
		} else {
			groupExprs = append(groupExprs, fmt.Sprintf("%s AS %s", qCol, alias))
		}
		groupAliases = append(groupAliases, alias)
	}

	// Build aggregation expressions.
	fns := p.Fns
	aggFields := p.Fields
	if len(fns) == 0 {
		fns = []string{"count"}
	}

	type aggDef struct {
		expr string
		key  string
	}
	var aggs []aggDef
	for i, fn := range fns {
		fn = strings.ToLower(fn)
		switch fn {
		case "count":
			aggs = append(aggs, aggDef{"COUNT(*)", "count"})
		case "sum", "avg", "min", "max":
			fieldSlug := ""
			if i < len(aggFields) {
				fieldSlug = aggFields[i]
			} else if len(aggFields) > 0 {
				fieldSlug = aggFields[0]
			}
			if fieldSlug == "" {
				return nil, fmt.Errorf("%w: %s requires field parameter", schema.ErrInvalidInput, fn)
			}
			f, exists := bySlug[fieldSlug]
			if !exists {
				return nil, fmt.Errorf("%w: field %q not found", schema.ErrInvalidInput, fieldSlug)
			}
			if f.FieldType != schema.FieldNumber && f.FieldType != schema.FieldInteger && f.FieldType != schema.FieldAutonumber {
				return nil, fmt.Errorf("%w: %s requires numeric field, %s is %s",
					schema.ErrInvalidInput, fn, fieldSlug, f.FieldType)
			}
			aggs = append(aggs, aggDef{
				expr: fmt.Sprintf("%s(%q)", strings.ToUpper(fn), fieldSlug),
				key:  fmt.Sprintf("%s_%s", fn, fieldSlug),
			})
		default:
			return nil, fmt.Errorf("%w: unknown aggregation function %q", schema.ErrInvalidInput, fn)
		}
	}

	where, args, err := ParseFilters(p.Filters, fields)
	if err != nil {
		return nil, err
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)

	var selectParts []string
	selectParts = append(selectParts, groupExprs...)
	for _, a := range aggs {
		selectParts = append(selectParts, a.expr)
	}
	groupByStr := strings.Join(groupAliases, ", ")

	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE deleted_at IS NULL %s GROUP BY %s ORDER BY %s",
		strings.Join(selectParts, ", "), qTable, where, groupByStr, groupByStr,
	)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Single group + single agg: legacy format { group, value }.
	isLegacy := len(p.Groups) == 1 && len(aggs) == 1

	if isLegacy {
		type bucket struct {
			Group any `json:"group"`
			Value any `json:"value"`
		}
		var result []bucket
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return nil, err
			}
			result = append(result, bucket{
				Group: normalizeValue(vals[0]),
				Value: normalizeValue(vals[1]),
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Multi-group or multi-series: { groups: [...], values: { key: val } }.
	type multiBucket struct {
		Groups []any          `json:"groups"`
		Values map[string]any `json:"values"`
	}
	var result []multiBucket
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		groups := make([]any, len(p.Groups))
		for i := range p.Groups {
			groups[i] = normalizeValue(vals[i])
		}
		values := make(map[string]any, len(aggs))
		for i, a := range aggs {
			values[a.key] = normalizeValue(vals[len(p.Groups)+i])
		}
		result = append(result, multiBucket{Groups: groups, Values: values})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// --- Bulk Create ---

// BulkCreate inserts an array of records in a single transaction.
// Returns 201 with the created rows, or 400/500 if any row fails (entire batch rolls back).
func (h *DynHandler) BulkCreate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_create") {
		return
	}

	var bodies []map[string]any
	if err := readJSON(r, &bodies); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(bodies) == 0 {
		writeError(w, http.StatusBadRequest, "empty bulk payload")
		return
	}
	if len(bodies) > 1000 {
		writeError(w, http.StatusBadRequest, "bulk payload too large (max 1000)")
		return
	}

	// Validate every record up front before opening a tx.
	for i, body := range bodies {
		if err := validatePayload(r.Context(), h.pool, h.cache, body, fields, true); err != nil {
			handleErr(w, r, fmt.Errorf("record[%d]: %w", i, err))
			return
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	qTable := pgutil.QuoteQualified("data", col.Slug)
	bulkProcEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, bulkProcEnabled, &selectColOpts{cache: h.cache})

	// Inject initial status for bulk create if process is enabled.
	if bulkProcEnabled {
		if initStatus := h.initialStatusName(col.ID); initStatus != "" {
			for i := range bodies {
				bodies[i]["_status"] = initStatus
			}
		}
	}

	user, _ := middleware.GetUser(r.Context())
	created := make([]map[string]any, 0, len(bodies))
	for i, body := range bodies {
		colNames, placeholders, args := buildInsertColumns(body, fields, user.UserID)
		var sql string
		if len(colNames) == 0 {
			sql = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES RETURNING %s", qTable, selectCols)
		} else {
			sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
				qTable,
				strings.Join(colNames, ", "),
				strings.Join(placeholders, ", "),
				selectCols,
			)
		}
		rows, err := tx.Query(r.Context(), sql, args...)
		if err != nil {
			handleErr(w, r, fmt.Errorf("record[%d]: %w", i, err))
			return
		}
		recs, err := collectRows(rows)
		rows.Close()
		if err != nil {
			handleErr(w, r, fmt.Errorf("record[%d]: %w", i, err))
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
	writeJSON(w, http.StatusCreated, created)
}

// BulkDelete soft-deletes records by ID array.
// Body: { "ids": ["uuid1", "uuid2", ...] }
func (h *DynHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, _, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_delete") {
		return
	}

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "empty ids array")
		return
	}
	if len(body.IDs) > 1000 {
		writeError(w, http.StatusBadRequest, "too many ids (max 1000)")
		return
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	args := make([]any, len(body.IDs))
	placeholders := make([]string, len(body.IDs))
	for i, id := range body.IDs {
		args[i] = id
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// RLS: viewers can only delete their own rows.
	rlsClause := ""
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		user, _ := middleware.GetUser(r.Context())
		args = append(args, user.UserID)
		rlsClause = fmt.Sprintf(" AND created_by = $%d", len(args))
	}

	sql := fmt.Sprintf(
		"UPDATE %s SET deleted_at = now() WHERE id IN (%s) AND deleted_at IS NULL%s",
		qTable, strings.Join(placeholders, ","), rlsClause,
	)
	tag, err := h.pool.Exec(r.Context(), sql, args...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": tag.RowsAffected(),
	})
}

// buildInsertColumns extracts the column list for an INSERT from a body map,
// returning quoted column names, $-placeholders, and the matching arg values.
// userID is injected as created_by for all inserts.
func buildInsertColumns(body map[string]any, fields []schema.Field, userID string) (cols []string, placeholders []string, args []any) {
	idx := 1
	for _, f := range fields {
		if f.FieldType.NoColumn() || f.FieldType == schema.FieldAutonumber || f.IsManyToMany() {
			continue
		}
		v, exists := body[f.Slug]
		if !exists {
			continue
		}
		cols = append(cols, pgutil.QuoteIdent(f.Slug))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, coerceValue(v, f.FieldType))
		idx++
	}
	// Auto-set created_by from authenticated user.
	cols = append(cols, `"created_by"`)
	placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
	args = append(args, userID)
	idx++
	if st, ok := body["_status"]; ok {
		cols = append(cols, `"_status"`)
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, st)
	}
	return cols, placeholders, args
}

// --- FormulaPreview ---

// FormulaPreview validates a formula expression and returns sample results.
// POST /api/data/{slug}/formula-preview
// Body: { "expression": "price * quantity", "result_type": "number" }
func (h *DynHandler) FormulaPreview(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_view") {
		return
	}

	var body struct {
		Expression string `json:"expression"`
		ResultType string `json:"result_type"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	slugMap := buildSlugMap(fields)
	resolver := buildRelationResolver(fields, h.cache)
	result, err := formula.ParseWithResolver(body.Expression, slugMap, resolver)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":      false,
			"error":      err.Error(),
			"sql":        "",
			"refs":       []string{},
			"cross_refs": []string{},
			"samples":    []any{},
		})
		return
	}
	sqlExpr := result.SQL
	refs := result.Refs
	crossRefs := result.CrossRefs

	// Fetch up to 5 sample rows to show computed values.
	qTable := pgutil.QuoteQualified("data", col.Slug)
	sampleSQL := fmt.Sprintf("SELECT (%s) AS result FROM %s WHERE deleted_at IS NULL LIMIT 5", sqlExpr, qTable)

	rows, err := h.pool.Query(r.Context(), sampleSQL)
	var samples []any
	if err == nil {
		for rows.Next() {
			vals, e := rows.Values()
			if e != nil {
				break
			}
			if len(vals) > 0 {
				samples = append(samples, normalizeValue(vals[0]))
			}
		}
		if e := rows.Err(); e != nil {
			slog.Warn("validateFormula: row iteration error", "error", e)
		}
		rows.Close()
	} else {
		slog.Warn("validateFormula: sample query failed", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      true,
		"error":      "",
		"sql":        sqlExpr,
		"refs":       refs,
		"cross_refs": crossRefs,
		"samples":    samples,
	})
}

// --- BatchUpdate ---

// BatchUpdate applies partial updates to multiple rows in a single transaction.
// Body: { "updates": [{ "id": "uuid", "fields": { "col": value, ... } }, ...] }
func (h *DynHandler) BatchUpdate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	col, fields, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_edit") {
		return
	}

	var body struct {
		Updates []struct {
			ID      string         `json:"id"`
			Version *int           `json:"_version"`
			Fields  map[string]any `json:"fields"`
		} `json:"updates"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Updates) == 0 {
		writeError(w, http.StatusBadRequest, "empty batch payload")
		return
	}
	if len(body.Updates) > 1000 {
		writeError(w, http.StatusBadRequest, "batch payload too large (max 1000)")
		return
	}

	// Validate all payloads up front.
	for i, u := range body.Updates {
		if u.ID == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("updates[%d]: id is required", i))
			return
		}
		if err := validatePayload(r.Context(), h.pool, h.cache, u.Fields, fields, false); err != nil {
			handleErr(w, r, fmt.Errorf("updates[%d]: %w", i, err))
			return
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	qTable := pgutil.QuoteQualified("data", col.Slug)
	procEnabled := h.hasProcessEnabled(col.ID)
	selectCols := buildSelectCols(fields, procEnabled, &selectColOpts{cache: h.cache})
	user, _ := middleware.GetUser(r.Context())

	type changeMeta struct {
		recordID string
		diff     map[string]any
	}

	updated := make([]map[string]any, 0, len(body.Updates))
	changes := make([]changeMeta, 0, len(body.Updates))

	for i, u := range body.Updates {
		sets := []string{`"updated_at" = now()`, `"_version" = "_version" + 1`}
		args := []any{}
		idx := 1
		for _, f := range fields {
			if f.FieldType.IsLayout() || f.FieldType == schema.FieldAutonumber {
				continue
			}
			v, exists := u.Fields[f.Slug]
			if !exists {
				continue
			}
			sets = append(sets, fmt.Sprintf("%s = $%d", pgutil.QuoteIdent(f.Slug), idx))
			args = append(args, coerceValue(v, f.FieldType))
			idx++
		}

		if len(sets) == 2 { // only updated_at + _version increment
			continue
		}

		args = append(args, u.ID)
		idIdx := idx
		idx++

		// Optimistic locking.
		versionClause := ""
		if u.Version != nil {
			args = append(args, *u.Version)
			versionClause = fmt.Sprintf(` AND "_version" = $%d`, idx)
		}

		sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d AND deleted_at IS NULL%s RETURNING %s",
			qTable, strings.Join(sets, ", "), idIdx, versionClause, selectCols)

		rows, err := tx.Query(r.Context(), sql, args...)
		if err != nil {
			handleErr(w, r, fmt.Errorf("updates[%d]: %w", i, err))
			return
		}
		recs, err := collectRows(rows)
		rows.Close()
		if err != nil {
			handleErr(w, r, fmt.Errorf("updates[%d]: %w", i, err))
			return
		}
		if len(recs) == 0 && u.Version != nil {
			writeError(w, http.StatusConflict,
				fmt.Sprintf("updates[%d]: 다른 사용자가 이미 이 레코드를 수정했습니다. 새로고침 후 다시 시도해 주세요.", i))
			return
		}
		if len(recs) > 0 {
			updated = append(updated, recs[0])
			diff := make(map[string]any)
			for k, v := range u.Fields {
				diff[k] = map[string]any{"new": v}
			}
			changes = append(changes, changeMeta{recordID: u.ID, diff: diff})
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		handleErr(w, r, err)
		return
	}

	// Record change history after successful commit.
	for _, c := range changes {
		recordChange(r.Context(), h.pool, col.ID, c.recordID, user.UserID, user.Name, "update", c.diff)
	}

	writeJSON(w, http.StatusOK, updated)
}

// --- Delete (soft) ---

// Delete performs a soft delete on a record by setting its deleted_at timestamp
// to now(). The record remains in the table but is excluded from all queries via
// the standard "WHERE deleted_at IS NULL" filter. Viewers can only soft-delete
// rows they created (RLS enforcement). A change history entry is recorded and an
// EventRecordDelete event is published for automation triggers.
func (h *DynHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	col, _, ok := h.resolveCollection(w, slug)
	if !ok {
		return
	}
	if !h.checkAccess(w, r, col, "entry_delete") {
		return
	}

	qTable := pgutil.QuoteQualified("data", col.Slug)
	user, _ := middleware.GetUser(r.Context())

	// RLS: viewers can only delete their own rows.
	var sqlDel string
	var delArgs []any
	colRole := middleware.GetCollectionRole(r.Context())
	if colRole == "viewer" {
		sqlDel = fmt.Sprintf("UPDATE %s SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL AND created_by = $2", qTable)
		delArgs = []any{id, user.UserID}
	} else {
		sqlDel = fmt.Sprintf("UPDATE %s SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", qTable)
		delArgs = []any{id}
	}

	tag, err := h.pool.Exec(r.Context(), sqlDel, delArgs...)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}

	// Record change history.
	recordChange(r.Context(), h.pool, col.ID, id, user.UserID, user.Name, "delete", map[string]any{"_deleted": true})

	// Publish automation event.
	h.bus.Publish(r.Context(), events.Event{
		Type:           events.EventRecordDelete,
		CollectionID:   col.ID,
		CollectionSlug: col.Slug,
		RecordID:       id,
		ActorUserID:    user.UserID,
		ActorName:      user.Name,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- helpers ---

// checkAccess verifies the current user is allowed the given operation (e.g.,
// "entry_view", "entry_create", "entry_edit", "entry_delete") on the collection's
// access_config. Directors always have full access. For other roles, it checks
// the collection's AccessConfig.AllowsRole. Returns false and writes a 403
// response if denied, so the caller can short-circuit.
func (h *DynHandler) checkAccess(w http.ResponseWriter, r *http.Request, col schema.Collection, operation string) bool {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return false
	}
	// Directors always have full access.
	if user.Role == "director" {
		return true
	}
	if !col.AccessConfig.AllowsRole(operation, user.Role) {
		writeError(w, http.StatusForbidden, "access denied for this collection")
		return false
	}
	return true
}

// buildRLSClause appends an AND clause that restricts rows based on the
// collection's rls_mode. prefix is the optional table qualifier (e.g. "data"."my_table").
//
// Supported modes:
//   - ""/"creator": created_by = $userID
//   - "department": created_by IN (SELECT id FROM auth.users WHERE department_id = $deptID)
//   - "subsidiary": created_by IN (SELECT id FROM auth.users WHERE subsidiary_id = $subID)
//   - "filter": custom field-based filters from AccessConfig.RLSFilters
func buildRLSClause(r *http.Request, col schema.Collection, args *[]any, prefix string) string {
	user, _ := middleware.GetUser(r.Context())
	colRef := "created_by"
	if prefix != "" {
		colRef = prefix + ".created_by"
	}

	mode := col.AccessConfig.RLSMode

	switch mode {
	case "department":
		if user.DepartmentID != "" {
			*args = append(*args, user.DepartmentID)
			return fmt.Sprintf(" AND %s IN (SELECT id FROM auth.users WHERE department_id = $%d)", colRef, len(*args))
		}
		// Fall back to creator if no department.

	case "subsidiary":
		if user.SubsidiaryID != "" {
			*args = append(*args, user.SubsidiaryID)
			return fmt.Sprintf(" AND %s IN (SELECT id FROM auth.users WHERE subsidiary_id = $%d)", colRef, len(*args))
		}
		// Fall back to creator if no subsidiary.

	case "filter":
		if len(col.AccessConfig.RLSFilters) > 0 {
			return buildCustomRLSFilters(col.AccessConfig.RLSFilters, user, args, prefix)
		}
		// Fall back to creator if no filters configured.
	}

	// Default: creator-only.
	*args = append(*args, user.UserID)
	return fmt.Sprintf(" AND %s = $%d", colRef, len(*args))
}

// buildCustomRLSFilters generates AND clauses from custom RLS filter rules.
// User attribute references ($user.id, $user.department_id, $user.subsidiary_id)
// are resolved at query time.
func buildCustomRLSFilters(filters []schema.RLSFilter, user middleware.UserClaims, args *[]any, prefix string) string {
	var clauses []string
	for _, f := range filters {
		col := pgutil.QuoteIdent(f.Field)
		if prefix != "" {
			col = prefix + "." + col
		}

		// Resolve user attribute references in value.
		val := resolveRLSValue(f.Value, user)

		switch f.Op {
		case "eq":
			*args = append(*args, val)
			clauses = append(clauses, fmt.Sprintf("%s = $%d", col, len(*args)))
		case "neq":
			*args = append(*args, val)
			clauses = append(clauses, fmt.Sprintf("%s != $%d", col, len(*args)))
		case "in":
			// Value is comma-separated list.
			*args = append(*args, val)
			clauses = append(clauses, fmt.Sprintf("%s = ANY(string_to_array($%d, ','))", col, len(*args)))
		case "contains":
			*args = append(*args, "%"+val+"%")
			clauses = append(clauses, fmt.Sprintf("CAST(%s AS TEXT) ILIKE $%d", col, len(*args)))
		default:
			// Unknown op — treat as eq for safety.
			*args = append(*args, val)
			clauses = append(clauses, fmt.Sprintf("%s = $%d", col, len(*args)))
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(clauses, " AND ")
}

// resolveRLSValue replaces $user.* references with actual user attribute values.
func resolveRLSValue(value string, user middleware.UserClaims) string {
	switch value {
	case "$user.id":
		return user.UserID
	case "$user.department_id":
		return user.DepartmentID
	case "$user.subsidiary_id":
		return user.SubsidiaryID
	case "$user.email":
		return user.Email
	case "$user.name":
		return user.Name
	case "$user.role":
		return user.Role
	default:
		return value
	}
}

// resolveCollection looks up a collection by its URL slug in the in-memory schema
// cache and loads its field definitions. If the collection is not found, it writes
// a 404 response and returns false so the caller can short-circuit.
func (h *DynHandler) resolveCollection(w http.ResponseWriter, slug string) (schema.Collection, []schema.Field, bool) {
	col, ok := h.cache.CollectionBySlug(slug)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("collection %q not found", slug))
		return schema.Collection{}, nil, false
	}
	fields := h.cache.Fields(col.ID)
	return col, fields, true
}

// hasProcessEnabled checks if the collection has an active process workflow.
func (h *DynHandler) hasProcessEnabled(collectionID string) bool {
	p, ok := h.cache.ProcessByCollectionID(collectionID)
	return ok && p.IsEnabled
}

// initialStatusName returns the name of the initial status for a collection's process.
func (h *DynHandler) initialStatusName(collectionID string) string {
	p, ok := h.cache.ProcessByCollectionID(collectionID)
	if !ok {
		return ""
	}
	for _, s := range p.Statuses {
		if s.IsInitial {
			return s.Name
		}
	}
	return ""
}

// validateStatusTransition checks if a status transition is allowed for the given user role and user ID.
func (h *DynHandler) validateStatusTransition(collectionID, fromStatus, toStatus, userRole, userID string) error {
	p, ok := h.cache.ProcessByCollectionID(collectionID)
	if !ok || !p.IsEnabled {
		return fmt.Errorf("%w: 이 앱에는 프로세스가 활성화되지 않았습니다", schema.ErrInvalidInput)
	}

	// Build status ID lookup.
	nameToID := make(map[string]string, len(p.Statuses))
	for _, s := range p.Statuses {
		nameToID[s.Name] = s.ID
	}

	fromID, ok := nameToID[fromStatus]
	if !ok {
		return fmt.Errorf("%w: 현재 상태 %q를 찾을 수 없습니다", schema.ErrInvalidInput, fromStatus)
	}
	toID, ok := nameToID[toStatus]
	if !ok {
		return fmt.Errorf("%w: 대상 상태 %q를 찾을 수 없습니다", schema.ErrInvalidInput, toStatus)
	}

	for _, t := range p.Transitions {
		if t.FromStatusID == fromID && t.ToStatusID == toID {
			// If both allowed_roles and allowed_user_ids are empty, any user can perform the transition.
			if len(t.AllowedRoles) == 0 && len(t.AllowedUserIDs) == 0 {
				return nil
			}
			// Check if user matches either role OR user ID.
			for _, r := range t.AllowedRoles {
				if r == userRole {
					return nil
				}
			}
			for _, uid := range t.AllowedUserIDs {
				if uid == userID {
					return nil
				}
			}
			return fmt.Errorf("%w: %q → %q 상태 이동 권한이 없습니다",
				schema.ErrInvalidInput, fromStatus, toStatus)
		}
	}

	// Build list of allowed transitions for the error message.
	var allowed []string
	for _, t := range p.Transitions {
		if t.FromStatusID == fromID {
			for _, s := range p.Statuses {
				if s.ID == t.ToStatusID {
					allowed = append(allowed, fmt.Sprintf("%s (%s)", s.Name, t.Label))
				}
			}
		}
	}
	return fmt.Errorf("%w: %q → %q 상태 이동이 허용되지 않습니다. 허용: %v",
		schema.ErrInvalidInput, fromStatus, toStatus, allowed)
}

// selectColOpts holds optional parameters for column generation.
type selectColOpts struct {
	cache *schema.Cache
}

func buildSelectCols(fields []schema.Field, hasStatus bool, opts *selectColOpts) string {
	cols := []string{`"id"`}
	slugMap := buildSlugMap(fields)
	var cache *schema.Cache
	if opts != nil {
		cache = opts.cache
	}
	for _, f := range fields {
		if f.FieldType.IsLayout() {
			continue
		}
		// Formula: computed as SQL expression in SELECT.
		// Lookup/Rollup: skipped here, resolved post-fetch.
		if f.FieldType == schema.FieldFormula {
			if expr := formulaExpr(f, slugMap, cache, fields); expr != "" {
				cols = append(cols, fmt.Sprintf(`(%s) AS %q`, expr, f.Slug))
			}
			continue
		}
		if f.FieldType == schema.FieldLookup || f.FieldType == schema.FieldRollup {
			continue
		}
		if f.IsManyToMany() {
			continue
		}
		cols = append(cols, pgutil.QuoteIdent(f.Slug))
	}
	cols = append(cols, `"created_at"`, `"updated_at"`, `"created_by"`, `"updated_by"`, `"deleted_at"`, `"_version"`)
	if hasStatus {
		cols = append(cols, `"_status"`)
	}
	return strings.Join(cols, ", ")
}

// qualifySelectCols returns the same column list but each column qualified
// with the given table prefix and aliased back to its bare name so the
// row scanner sees the same field names.
func qualifySelectCols(fields []schema.Field, prefix string, hasStatus bool, opts *selectColOpts) string {
	cols := []string{fmt.Sprintf(`%s.%q AS %q`, prefix, "id", "id")}
	slugMap := buildSlugMap(fields)
	var cache *schema.Cache
	if opts != nil {
		cache = opts.cache
	}
	for _, f := range fields {
		if f.FieldType.IsLayout() {
			continue
		}
		if f.FieldType == schema.FieldFormula {
			if expr := formulaExpr(f, slugMap, cache, fields); expr != "" {
				cols = append(cols, fmt.Sprintf(`(%s) AS %q`, expr, f.Slug))
			}
			continue
		}
		if f.FieldType == schema.FieldLookup || f.FieldType == schema.FieldRollup {
			continue
		}
		if f.IsManyToMany() {
			continue
		}
		cols = append(cols, fmt.Sprintf(`%s.%q AS %q`, prefix, f.Slug, f.Slug))
	}
	for _, sysCol := range []string{"created_at", "updated_at", "created_by", "updated_by", "deleted_at", "_version"} {
		cols = append(cols, fmt.Sprintf(`%s.%q AS %q`, prefix, sysCol, sysCol))
	}
	if hasStatus {
		cols = append(cols, fmt.Sprintf(`%s.%q AS %q`, prefix, "_status", "_status"))
	}
	return strings.Join(cols, ", ")
}

// buildSlugMap creates a set of valid field slugs for formula parsing.
func buildSlugMap(fields []schema.Field) map[string]bool {
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !f.FieldType.IsLayout() && !f.FieldType.IsComputed() {
			m[f.Slug] = true
		}
	}
	return m
}

// formulaExpr parses a formula field's expression and returns a safe SQL fragment.
// Returns empty string on parse error (field is silently omitted from SELECT).
func formulaExpr(f schema.Field, slugMap map[string]bool, cache *schema.Cache, fields []schema.Field) string {
	opts, err := schema.ExtractFormulaOptions(f.Options)
	if err != nil || opts == nil || opts.Expression == "" {
		return ""
	}

	// Build a relation resolver from the cache.
	var resolver formula.RelationResolver
	if cache != nil {
		resolver = buildRelationResolver(fields, cache)
	}

	result, err := formula.ParseWithResolver(opts.Expression, slugMap, resolver)
	if err != nil {
		return ""
	}
	return result.SQL
}

// buildRelationResolver creates a RelationResolver callback that uses the schema
// cache to resolve relation fields to their target tables.
func buildRelationResolver(fields []schema.Field, cache *schema.Cache) formula.RelationResolver {
	// Index relation fields by slug.
	relBySlug := make(map[string]schema.Field)
	for _, f := range fields {
		if f.FieldType == schema.FieldRelation && f.Relation != nil {
			relBySlug[f.Slug] = f
		}
	}

	return func(relSlug string) (*formula.RelationInfo, error) {
		f, ok := relBySlug[relSlug]
		if !ok || f.Relation == nil {
			return nil, fmt.Errorf("%q is not a relation field", relSlug)
		}

		targetCol, ok := cache.CollectionByID(f.Relation.TargetCollectionID)
		if !ok {
			return nil, fmt.Errorf("target collection not found for relation %q", relSlug)
		}

		targetTable := pgutil.QuoteQualified("data", targetCol.Slug)

		// For reverse relations (SUMREL etc.), find the FK column on the target
		// table that points back to this collection.
		reverseCol := ""
		targetFields := cache.Fields(targetCol.ID)
		for _, tf := range targetFields {
			if tf.FieldType == schema.FieldRelation && tf.Relation != nil &&
				tf.Relation.TargetCollectionID == f.CollectionID {
				reverseCol = tf.Slug
				break
			}
		}

		return &formula.RelationInfo{
			TargetTable:   targetTable,
			OwnerColumn:   f.Slug,
			ReverseColumn: reverseCol,
		}, nil
	}
}

// collectRows uses pgx.RowToMap and normalizes types.
func collectRows(rows pgx.Rows) ([]map[string]any, error) {
	var result []map[string]any
	descs := rows.FieldDescriptions()

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(vals))
		for i, v := range vals {
			name := descs[i].Name
			row[name] = normalizeValue(v)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case [16]byte:
		return pgutil.FormatUUID(val)
	case pgtype.UUID:
		return pgutil.UUIDToString(val)
	case pgtype.Numeric:
		f, err := val.Float64Value()
		if err != nil || !f.Valid {
			return nil
		}
		return f.Float64
	case time.Time:
		if val.IsZero() {
			return nil
		}
		return val
	default:
		return val
	}
}

// expandRelations replaces UUID values in relation fields with the full target
// record. Each expand field triggers exactly one batched SELECT — no N+1 queries.
//
// Format: ?expand=field1,field2
// Only relation fields (non-M:N) are expandable; other field types and unknown
// slugs return an error so the client discovers the problem immediately.
func (h *DynHandler) expandRelations(ctx context.Context, records []map[string]any, fields []schema.Field, expandParam string) error {
	if len(records) == 0 {
		return nil
	}

	// Index the collection's fields for fast lookup.
	fieldBySlug := make(map[string]schema.Field, len(fields))
	for _, f := range fields {
		fieldBySlug[f.Slug] = f
	}

	for _, raw := range strings.Split(expandParam, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		f, ok := fieldBySlug[name]
		if !ok {
			return fmt.Errorf("expand: unknown field %q", name)
		}
		if f.FieldType != schema.FieldRelation {
			return fmt.Errorf("expand: field %q is not a relation", name)
		}
		if f.Relation == nil {
			return fmt.Errorf("expand: field %q has no relation config", name)
		}

		targetCol, ok := h.cache.CollectionByID(f.Relation.TargetCollectionID)
		if !ok {
			return fmt.Errorf("expand: target collection for %q not found", name)
		}

		if f.IsManyToMany() {
			if err := h.expandM2M(ctx, records, f, targetCol); err != nil {
				return err
			}
			continue
		}

		// Collect distinct non-null UUIDs from the current result set.
		seen := make(map[string]struct{})
		ids := make([]string, 0, len(records))
		for _, row := range records {
			v := row[name]
			s, ok := v.(string)
			if !ok || s == "" {
				continue
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			ids = append(ids, s)
		}
		if len(ids) == 0 {
			continue
		}

		// Batch fetch targets in a single query.
		targetFields := h.cache.Fields(targetCol.ID)
		targetSelectCols := buildSelectCols(targetFields, false, nil)

		placeholders := make([]string, len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		qTargetTable := pgutil.QuoteQualified("data", targetCol.Slug)
		sql := fmt.Sprintf(
			"SELECT %s FROM %s WHERE id IN (%s) AND deleted_at IS NULL",
			targetSelectCols, qTargetTable, strings.Join(placeholders, ","),
		)
		targetRows, err := h.pool.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("expand %s: %w", name, err)
		}
		targetRecords, err := collectRows(targetRows)
		targetRows.Close()
		if err != nil {
			return fmt.Errorf("expand %s scan: %w", name, err)
		}

		// Build id → target row map.
		byID := make(map[string]map[string]any, len(targetRecords))
		for _, tr := range targetRecords {
			if id, ok := tr["id"].(string); ok {
				byID[id] = tr
			}
		}

		// Replace UUIDs with the nested record.
		for _, row := range records {
			s, ok := row[name].(string)
			if !ok || s == "" {
				continue
			}
			if target, found := byID[s]; found {
				row[name] = target
			}
		}
	}
	return nil
}

// expandUserFields batch-fetches auth.users for all user-type fields and replaces
// UUID values with {id, name, email} objects. This runs automatically, unlike
// relation expand which is opt-in.
func (h *DynHandler) expandUserFields(ctx context.Context, records []map[string]any, fields []schema.Field) {
	var userFields []schema.Field
	for _, f := range fields {
		if f.FieldType == schema.FieldUser {
			userFields = append(userFields, f)
		}
	}
	if len(userFields) == 0 {
		return
	}

	// Collect all distinct user UUIDs across all user fields.
	seen := make(map[string]struct{})
	var ids []string
	for _, row := range records {
		for _, f := range userFields {
			s, ok := row[f.Slug].(string)
			if !ok || s == "" {
				continue
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			ids = append(ids, s)
		}
	}
	if len(ids) == 0 {
		return
	}

	// Batch fetch users.
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	sql := fmt.Sprintf(
		`SELECT id, name, email FROM auth.users WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return // best effort — don't fail the request
	}
	defer rows.Close()

	type userInfo struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	byID := make(map[string]userInfo)
	for rows.Next() {
		var u userInfo
		var uid pgtype.UUID
		if err := rows.Scan(&uid, &u.Name, &u.Email); err != nil {
			continue
		}
		u.ID = pgutil.UUIDToString(uid)
		byID[u.ID] = u
	}

	// Replace UUIDs with user objects.
	for _, row := range records {
		for _, f := range userFields {
			s, ok := row[f.Slug].(string)
			if !ok || s == "" {
				continue
			}
			if u, found := byID[s]; found {
				row[f.Slug] = u
			}
		}
	}
}

// fetchRow loads a single record by ID. Used for pre-update comparisons.
func (h *DynHandler) fetchRow(ctx context.Context, col schema.Collection, fields []schema.Field, id string) (map[string]any, error) {
	qTable := pgutil.QuoteQualified("data", col.Slug)
	selectCols := buildSelectCols(fields, false, &selectColOpts{cache: h.cache})
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1 AND deleted_at IS NULL", selectCols, qTable)
	rows, err := h.pool.Query(ctx, sql, id)
	if err != nil {
		return nil, err
	}
	records, err := collectRows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("record %s: %w", id, schema.ErrNotFound)
	}
	return records[0], nil
}

// coerceValue ensures the Go value matches what pgx expects for the column type.
func coerceValue(v any, ft schema.FieldType) any {
	if v == nil {
		return nil
	}
	switch ft {
	case schema.FieldMultiselect:
		// JSON array → []string
		switch arr := v.(type) {
		case []any:
			strs := make([]string, len(arr))
			for i, el := range arr {
				strs[i] = fmt.Sprint(el)
			}
			return strs
		}
	case schema.FieldJSON:
		// Keep as JSONB.
		b, _ := json.Marshal(v)
		return b
	case schema.FieldInteger:
		if f, ok := v.(float64); ok {
			return int64(f)
		}
	}
	return v
}

// ---------- M:N helpers ----------

// toStringSlice converts a JSON-decoded value ([]any or []string) to []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []any:
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			if s, ok := el.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return arr
	}
	return nil
}

// m2mJunctionInfo resolves junction table and column names for an M:N field.
func m2mJunctionInfo(ownerSlug string, f schema.Field, cache *schema.Cache) (junctionTable, ownerCol, targetCol string, ok bool) {
	if f.Relation == nil {
		return "", "", "", false
	}
	target, found := cache.CollectionByID(f.Relation.TargetCollectionID)
	if !found {
		return "", "", "", false
	}
	junc := f.Relation.JunctionTable
	if junc == "" {
		junc = ownerSlug + "_" + target.Slug + "_rel"
	}
	return junc, ownerSlug + "_id", target.Slug + "_id", true
}

// syncM2MLinks replaces all links for a record in a junction table.
// It deletes existing links and inserts the new set.
func (h *DynHandler) syncM2MLinks(ctx context.Context, db querier, ownerSlug string, f schema.Field, recordID string, targetIDs []string) error {
	junc, ownerCol, targetCol, ok := m2mJunctionInfo(ownerSlug, f, h.cache)
	if !ok {
		return fmt.Errorf("m2m: cannot resolve junction for field %q", f.Slug)
	}

	qTable := pgutil.QuoteQualified("data", junc)

	// Delete existing links.
	delSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", qTable, pgutil.QuoteIdent(ownerCol))
	if _, err := db.Exec(ctx, delSQL, recordID); err != nil {
		return fmt.Errorf("m2m delete: %w", err)
	}

	if len(targetIDs) == 0 {
		return nil
	}

	// Batch insert new links.
	var vals []string
	args := []any{recordID}
	for i, tid := range targetIDs {
		args = append(args, tid)
		vals = append(vals, fmt.Sprintf("($1, $%d)", i+2))
	}
	insSQL := fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES %s ON CONFLICT DO NOTHING",
		qTable, pgutil.QuoteIdent(ownerCol), pgutil.QuoteIdent(targetCol), strings.Join(vals, ", "))
	if _, err := db.Exec(ctx, insSQL, args...); err != nil {
		return fmt.Errorf("m2m insert: %w", err)
	}
	return nil
}

// loadM2MFields populates M:N relation fields on records by querying junction tables.
// Each M:N field gets an array of target UUIDs.
func (h *DynHandler) loadM2MFields(ctx context.Context, records []map[string]any, fields []schema.Field, ownerSlug string) {
	// Find M:N fields.
	var m2mFields []schema.Field
	for _, f := range fields {
		if f.IsManyToMany() {
			m2mFields = append(m2mFields, f)
		}
	}
	if len(m2mFields) == 0 || len(records) == 0 {
		return
	}

	// Collect all record IDs.
	var recordIDs []string
	for _, row := range records {
		if id, ok := row["id"].(string); ok {
			recordIDs = append(recordIDs, id)
		}
	}
	if len(recordIDs) == 0 {
		return
	}

	for _, f := range m2mFields {
		junc, ownerCol, targetCol, ok := m2mJunctionInfo(ownerSlug, f, h.cache)
		if !ok {
			continue
		}

		// Build query to fetch all links for all records at once.
		placeholders := make([]string, len(recordIDs))
		args := make([]any, len(recordIDs))
		for i, id := range recordIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		qTable := pgutil.QuoteQualified("data", junc)
		sql := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s IN (%s)",
			pgutil.QuoteIdent(ownerCol), pgutil.QuoteIdent(targetCol), qTable, pgutil.QuoteIdent(ownerCol), strings.Join(placeholders, ","))

		rows, err := h.pool.Query(ctx, sql, args...)
		if err != nil {
			// Non-fatal: set empty arrays.
			for _, row := range records {
				row[f.Slug] = []string{}
			}
			continue
		}

		// Group target IDs by owner ID.
		linkMap := make(map[string][]string)
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				slog.Warn("loadM2MFields: skipping row", "field", f.Slug, "error", err)
				continue
			}
			if len(vals) < 2 {
				continue
			}
			oID := fmt.Sprint(normalizeValue(vals[0]))
			tID := fmt.Sprint(normalizeValue(vals[1]))
			linkMap[oID] = append(linkMap[oID], tID)
		}
		if err := rows.Err(); err != nil {
			slog.Warn("loadM2MFields: row iteration error", "field", f.Slug, "error", err)
		}
		rows.Close()

		// Set values on records.
		for _, row := range records {
			id, ok := row["id"].(string)
			if !ok {
				id = fmt.Sprint(row["id"])
			}
			if links, ok := linkMap[id]; ok {
				row[f.Slug] = links
			} else {
				row[f.Slug] = []string{}
			}
		}
	}
}

// expandM2M expands M:N relation fields by replacing UUID arrays with full target records.
func (h *DynHandler) expandM2M(ctx context.Context, records []map[string]any, f schema.Field, targetCol schema.Collection) error {
	// Collect all distinct target IDs across all records.
	seen := make(map[string]struct{})
	var allIDs []string
	for _, row := range records {
		ids := toStringSliceFromRow(row[f.Slug])
		for _, id := range ids {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				allIDs = append(allIDs, id)
			}
		}
	}
	if len(allIDs) == 0 {
		return nil
	}

	// Batch fetch target records.
	targetFields := h.cache.Fields(targetCol.ID)
	targetSelectCols := buildSelectCols(targetFields, false, nil)

	placeholders := make([]string, len(allIDs))
	args := make([]any, len(allIDs))
	for i, id := range allIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	qTargetTable := pgutil.QuoteQualified("data", targetCol.Slug)
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE id IN (%s) AND deleted_at IS NULL",
		targetSelectCols, qTargetTable, strings.Join(placeholders, ","))

	targetRows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("expand m2m %s: %w", f.Slug, err)
	}
	targetRecords, err := collectRows(targetRows)
	targetRows.Close()
	if err != nil {
		return fmt.Errorf("expand m2m %s scan: %w", f.Slug, err)
	}

	byID := make(map[string]map[string]any, len(targetRecords))
	for _, tr := range targetRecords {
		if id, ok := tr["id"].(string); ok {
			byID[id] = tr
		}
	}

	// Replace UUID arrays with expanded objects.
	for _, row := range records {
		ids := toStringSliceFromRow(row[f.Slug])
		expanded := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			if target, ok := byID[id]; ok {
				expanded = append(expanded, target)
			}
		}
		row[f.Slug] = expanded
	}
	return nil
}

// toStringSliceFromRow extracts a []string from a record value that may be
// []string, []any, or []map[string]any (already expanded).
func toStringSliceFromRow(v any) []string {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			if s, ok := el.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
