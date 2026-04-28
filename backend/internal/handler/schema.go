package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/migration"
	"github.com/choiceoh/topbid/backend/internal/pgutil"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// SchemaHandler serves the Schema API (/api/schema/...).
type SchemaHandler struct {
	pool   *pgxpool.Pool
	store  *schema.Store
	cache  *schema.Cache
	engine *migration.Engine
}

func NewSchemaHandler(pool *pgxpool.Pool, store *schema.Store, cache *schema.Cache, engine *migration.Engine) *SchemaHandler {
	return &SchemaHandler{pool: pool, store: store, cache: cache, engine: engine}
}

// --- Collections ---

func (h *SchemaHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	cols := h.cache.Collections()
	writeJSON(w, http.StatusOK, cols)
}

// CollectionCounts returns a map of collection slug → row count for all
// collections in a single query, avoiding the N+1 per-card fetches.
func (h *SchemaHandler) CollectionCounts(w http.ResponseWriter, r *http.Request) {
	cols := h.cache.Collections()
	if len(cols) == 0 {
		writeJSON(w, http.StatusOK, map[string]int64{})
		return
	}

	// Build a UNION ALL query: SELECT 'slug' AS slug, COUNT(*) FROM "data"."slug" UNION ALL ...
	var sql string
	for i, col := range cols {
		if i > 0 {
			sql += " UNION ALL "
		}
		quoted := fmt.Sprintf(`SELECT '%s' AS slug, COUNT(*) AS cnt FROM "data".%q`, col.Slug, col.Slug)
		sql += quoted
	}

	rows, err := h.pool.Query(r.Context(), sql)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := make(map[string]int64, len(cols))
	for rows.Next() {
		var slug string
		var cnt int64
		if err := rows.Scan(&slug, &cnt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result[slug] = cnt
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *SchemaHandler) GetCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	col, err := h.store.GetCollection(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, col)
}

func (h *SchemaHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var req schema.CreateCollectionReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if user, ok := middleware.GetUser(r.Context()); ok {
		req.CreatedBy = user.UserID
	}

	col, err := h.engine.CreateCollection(r.Context(), &req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (h *SchemaHandler) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req schema.UpdateCollectionReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	col, err := h.engine.UpdateCollection(r.Context(), id, &req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, col)
}

func (h *SchemaHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	confirmed := r.URL.Query().Get("confirm") == "true"

	if !confirmed {
		preview, err := h.engine.PreviewDropCollection(r.Context(), id)
		if err != nil {
			handleErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"confirmation_required": true,
			"preview":               preview,
		})
		return
	}

	if err := h.engine.DropCollection(r.Context(), id); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Fields ---

func (h *SchemaHandler) AddField(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	var req schema.CreateFieldIn
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	confirmed := r.URL.Query().Get("confirm") == "true"
	field, preview, err := h.engine.AddField(r.Context(), collectionID, &req, confirmed)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if preview != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"confirmation_required": true,
			"preview":               preview,
		})
		return
	}
	writeJSON(w, http.StatusCreated, field)
}

func (h *SchemaHandler) UpdateField(w http.ResponseWriter, r *http.Request) {
	fieldID := chi.URLParam(r, "fieldId")
	var req schema.UpdateFieldReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	confirmed := r.URL.Query().Get("confirm") == "true"
	preview, err := h.engine.AlterField(r.Context(), fieldID, &req, confirmed)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if preview != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"confirmation_required": true,
			"preview":               preview,
		})
		return
	}

	field, err := h.store.GetField(r.Context(), fieldID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, field)
}

func (h *SchemaHandler) DeleteField(w http.ResponseWriter, r *http.Request) {
	fieldID := chi.URLParam(r, "fieldId")
	confirmed := r.URL.Query().Get("confirm") == "true"

	if !confirmed {
		preview, err := h.engine.PreviewDropField(r.Context(), fieldID)
		if err != nil {
			handleErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"confirmation_required": true,
			"preview":               preview,
		})
		return
	}

	if err := h.engine.DropField(r.Context(), fieldID); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Process ---

func (h *SchemaHandler) GetProcess(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	proc, err := h.engine.GetProcess(r.Context(), collectionID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proc)
}

func (h *SchemaHandler) SaveProcess(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	var req schema.SaveProcessReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	proc, err := h.engine.SaveProcess(r.Context(), collectionID, &req)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proc)
}

// --- Migrations ---

func (h *SchemaHandler) MigrationHistory(w http.ResponseWriter, r *http.Request) {
	collectionID := r.URL.Query().Get("collection_id")
	var (
		migs []migration.Migration
		err  error
	)
	if collectionID != "" {
		migs, err = h.engine.History(r.Context(), collectionID)
	} else {
		migs, err = h.engine.FullHistory(r.Context())
	}
	if err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, migs)
}

func (h *SchemaHandler) RollbackMigration(w http.ResponseWriter, r *http.Request) {
	migrationID := chi.URLParam(r, "migrationId")
	if err := h.engine.Rollback(r.Context(), migrationID); err != nil {
		handleErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rolled_back"})
}

// ---------------------------------------------------------------------------
// My Tasks  GET /api/my-tasks
// ---------------------------------------------------------------------------

type myTaskItem struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
	CollectionID    string `json:"collectionId"`
	CollectionLabel string `json:"collectionLabel"`
	CollectionSlug  string `json:"collectionSlug"`
	CollectionIcon  string `json:"collectionIcon,omitempty"`
}

// MyTasks returns entries across all process-enabled collections where the
// current user is allowed to perform a transition from the entry's current status.
func (h *SchemaHandler) MyTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUser(r.Context())

	cols := h.cache.Collections()
	var items []myTaskItem

	for _, col := range cols {
		if !col.AccessConfig.AllowsRole("entry_view", user.Role) {
			continue
		}

		proc, ok := h.cache.ProcessByCollectionID(col.ID)
		if !ok || !proc.IsEnabled {
			continue
		}

		// Build set of status names where current user can act.
		idToName := make(map[string]string, len(proc.Statuses))
		for _, s := range proc.Statuses {
			idToName[s.ID] = s.Name
		}

		actionableStatuses := make(map[string]struct{})
		for _, t := range proc.Transitions {
			if isTransitionAllowed(t, user.Role, user.UserID) {
				if name, ok := idToName[t.FromStatusID]; ok {
					actionableStatuses[name] = struct{}{}
				}
			}
		}
		if len(actionableStatuses) == 0 {
			continue
		}

		// Build IN clause for status names.
		statusNames := make([]string, 0, len(actionableStatuses))
		for name := range actionableStatuses {
			statusNames = append(statusNames, name)
		}

		// Find first text field for label.
		fields := h.cache.Fields(col.ID)
		var titleField *schema.Field
		for i := range fields {
			if fields[i].FieldType == schema.FieldText && titleField == nil {
				titleField = &fields[i]
			}
		}

		qTable := pgutil.QuoteQualified("data", col.Slug)
		selCols := []string{"id", `"_status"`, `"_created_at"`}
		if titleField != nil {
			selCols = append(selCols, pgutil.QuoteIdent(titleField.Slug))
		}

		args := make([]any, len(statusNames))
		placeholders := make([]string, len(statusNames))
		for i, name := range statusNames {
			args[i] = name
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}

		sql := fmt.Sprintf(
			"SELECT %s FROM %s WHERE deleted_at IS NULL AND \"_status\" IN (%s) ORDER BY \"_created_at\" DESC LIMIT 200",
			strings.Join(selCols, ", "), qTable, strings.Join(placeholders, ", "),
		)

		rows, err := h.pool.Query(r.Context(), sql, args...)
		if err != nil {
			continue
		}
		records, err := collectRows(rows)
		rows.Close()
		if err != nil {
			continue
		}

		for _, rec := range records {
			label := "(무제)"
			if titleField != nil {
				if v := rec[titleField.Slug]; v != nil {
					label = fmt.Sprintf("%v", v)
				}
			}

			status := ""
			if v, ok := rec["_status"].(string); ok {
				status = v
			}

			createdAt := ""
			if v, ok := rec["_created_at"]; v != nil && ok {
				createdAt = fmt.Sprintf("%v", v)
			}

			items = append(items, myTaskItem{
				ID:              fmt.Sprintf("%v", rec["id"]),
				Label:           label,
				Status:          status,
				CreatedAt:       createdAt,
				CollectionID:    col.ID,
				CollectionLabel: col.Label,
				CollectionSlug:  col.Slug,
				CollectionIcon:  col.Icon,
			})
		}
	}

	if items == nil {
		items = []myTaskItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// ---------------------------------------------------------------------------
// Global Calendar  GET /api/calendar/events
// ---------------------------------------------------------------------------

type globalCalendarEvent struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Date            string `json:"date"`
	EndDate         string `json:"endDate,omitempty"`
	CollectionID    string `json:"collectionId"`
	CollectionLabel string `json:"collectionLabel"`
	CollectionSlug  string `json:"collectionSlug"`
	CollectionIcon  string `json:"collectionIcon,omitempty"`
}

func (h *SchemaHandler) GlobalCalendarEvents(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	yearStr := params.Get("year")
	monthStr := params.Get("month")

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

	user, _ := middleware.GetUser(r.Context())

	monthStart := fmt.Sprintf("%04d-%02d-01", year, month)
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	monthEnd := fmt.Sprintf("%04d-%02d-%02d", year, month, lastDay)

	cols := h.cache.Collections()
	var events []globalCalendarEvent

	// Query each eligible collection (using the cache to avoid N+1 on schema,
	// but still one query per collection for data — a UNION ALL would require
	// dynamic column selection which is impractical with varying schemas).
	for _, col := range cols {
		if !col.AccessConfig.AllowsRole("entry_view", user.Role) {
			continue
		}

		fields := h.cache.Fields(col.ID)
		var dateField *schema.Field
		var endDateField *schema.Field
		var titleField *schema.Field

		for i := range fields {
			f := &fields[i]
			if f.FieldType == schema.FieldDate || f.FieldType == schema.FieldDatetime {
				if dateField == nil {
					dateField = f
				} else if endDateField == nil {
					endDateField = f
				}
			}
			if f.FieldType == schema.FieldText && titleField == nil {
				titleField = f
			}
		}
		if dateField == nil {
			continue
		}

		qTable := pgutil.QuoteQualified("data", col.Slug)

		// Build select columns.
		selCols := []string{"id", pgutil.QuoteIdent(dateField.Slug)}
		if endDateField != nil {
			selCols = append(selCols, pgutil.QuoteIdent(endDateField.Slug))
		}
		if titleField != nil {
			selCols = append(selCols, pgutil.QuoteIdent(titleField.Slug))
		}

		sql := fmt.Sprintf(
			"SELECT %s FROM %s WHERE deleted_at IS NULL AND %q >= $1 AND %q <= $2 ORDER BY %q ASC LIMIT 500",
			strings.Join(selCols, ", "), qTable,
			dateField.Slug, dateField.Slug,
			dateField.Slug,
		)

		rows, err := h.pool.Query(r.Context(), sql, monthStart, monthEnd)
		if err != nil {
			// Skip collections we can't query (permission, deleted table, etc.)
			continue
		}
		records, err := collectRows(rows)
		rows.Close()
		if err != nil {
			continue
		}

		for _, rec := range records {
			dateStr := toDateStrGo(rec[dateField.Slug])
			if dateStr == "" {
				continue
			}

			endStr := ""
			if endDateField != nil {
				if e := toDateStrGo(rec[endDateField.Slug]); e != "" && e > dateStr {
					endStr = e
				}
			}

			label := "(무제)"
			if titleField != nil {
				if v := rec[titleField.Slug]; v != nil {
					label = fmt.Sprintf("%v", v)
				}
			}

			events = append(events, globalCalendarEvent{
				ID:              fmt.Sprintf("%v", rec["id"]),
				Label:           label,
				Date:            dateStr,
				EndDate:         endStr,
				CollectionID:    col.ID,
				CollectionLabel: col.Label,
				CollectionSlug:  col.Slug,
				CollectionIcon:  col.Icon,
			})
		}
	}

	if events == nil {
		events = []globalCalendarEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// ---------------------------------------------------------------------------
// Relationship Graph  GET /api/schema/relationship-graph
// ---------------------------------------------------------------------------

type graphNode struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Icon       string `json:"icon,omitempty"`
	FieldCount int    `json:"fieldCount"`
}

type graphEdge struct {
	ID           string `json:"id"`
	SourceID     string `json:"sourceId"`
	TargetID     string `json:"targetId"`
	Label        string `json:"label"`
	RelationType string `json:"relationType"`
}

type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func (h *SchemaHandler) RelationshipGraph(w http.ResponseWriter, r *http.Request) {
	cols := h.cache.Collections()

	var nodes []graphNode
	var edges []graphEdge

	for _, col := range cols {
		fields := h.cache.Fields(col.ID)

		// Count non-layout fields.
		fieldCount := 0
		for _, f := range fields {
			if !f.FieldType.IsLayout() {
				fieldCount++
			}
		}
		nodes = append(nodes, graphNode{
			ID:         col.ID,
			Label:      col.Label,
			Icon:       col.Icon,
			FieldCount: fieldCount,
		})

		// Build edges from relation fields.
		for _, f := range fields {
			if f.FieldType != schema.FieldRelation || f.Relation == nil {
				continue
			}
			edges = append(edges, graphEdge{
				ID:           f.ID,
				SourceID:     col.ID,
				TargetID:     f.Relation.TargetCollectionID,
				Label:        f.Label,
				RelationType: string(f.Relation.RelationType),
			})
		}
	}

	if nodes == nil {
		nodes = []graphNode{}
	}
	if edges == nil {
		edges = []graphEdge{}
	}

	writeJSON(w, http.StatusOK, graphResponse{Nodes: nodes, Edges: edges})
}

// ---------------------------------------------------------------------------
// Available Transitions  GET /api/schema/collections/{id}/process/transitions
// ---------------------------------------------------------------------------

type availableTransition struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	ToStatus         string   `json:"to_status"`
	ToColor          string   `json:"to_color"`
	AllowedUserNames []string `json:"allowed_user_names,omitempty"`
	IsBlocked        bool     `json:"is_blocked,omitempty"`
	BlockedReason    string   `json:"blocked_reason,omitempty"`
}

type transitionsResponse struct {
	Transitions  []availableTransition `json:"transitions"`
	AllowedMoves map[string][]string   `json:"allowed_moves"`
}

func (h *SchemaHandler) AvailableTransitions(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")
	proc, ok := h.cache.ProcessByCollectionID(collectionID)
	if !ok || !proc.IsEnabled {
		writeJSON(w, http.StatusOK, transitionsResponse{
			Transitions:  []availableTransition{},
			AllowedMoves: map[string][]string{},
		})
		return
	}

	user, _ := middleware.GetUser(r.Context())
	statusParam := r.URL.Query().Get("status")

	// Build status lookups.
	idToStatus := make(map[string]schema.ProcessStatus, len(proc.Statuses))
	nameToStatus := make(map[string]schema.ProcessStatus, len(proc.Statuses))
	for _, s := range proc.Statuses {
		idToStatus[s.ID] = s
		nameToStatus[s.Name] = s
	}

	// Collect all user IDs from transitions for name resolution.
	allUserIDs := make(map[string]struct{})
	for _, t := range proc.Transitions {
		for _, uid := range t.AllowedUserIDs {
			allUserIDs[uid] = struct{}{}
		}
	}
	userNames := h.resolveUserNames(r.Context(), allUserIDs)

	// If a specific status is requested, filter transitions from that status.
	var transitions []availableTransition
	if statusParam != "" {
		// Find current status by name or ID.
		var currentStatus *schema.ProcessStatus
		if s, ok := nameToStatus[statusParam]; ok {
			currentStatus = &s
		} else if s, ok := idToStatus[statusParam]; ok {
			currentStatus = &s
		}

		if currentStatus != nil {
			for _, t := range proc.Transitions {
				if t.FromStatusID != currentStatus.ID {
					continue
				}
				toStatus := idToStatus[t.ToStatusID]
				at := availableTransition{
					ID:       t.ID,
					Label:    t.Label,
					ToStatus: toStatus.Name,
					ToColor:  toStatus.Color,
				}
				// Resolve allowed user names for display.
				for _, uid := range t.AllowedUserIDs {
					if name, ok := userNames[uid]; ok {
						at.AllowedUserNames = append(at.AllowedUserNames, name)
					}
				}
				if !isTransitionAllowed(t, user.Role, user.UserID) {
					at.IsBlocked = true
					at.BlockedReason = buildBlockedReason(t, userNames)
				}
				transitions = append(transitions, at)
			}
		}
	}
	if transitions == nil {
		transitions = []availableTransition{}
	}

	// Build full allowed_moves map (for Kanban).
	allowedMoves := buildAllowedMoves(proc, user.Role, user.UserID)

	writeJSON(w, http.StatusOK, transitionsResponse{
		Transitions:  transitions,
		AllowedMoves: allowedMoves,
	})
}

// buildBlockedReason constructs a human-readable Korean reason for a blocked transition.
func buildBlockedReason(t schema.ProcessTransition, userNames map[string]string) string {
	roleLabels := map[string]string{
		"director": "관리자",
		"pm":       "운영자",
		"engineer": "담당자",
		"viewer":   "열람자",
	}
	var parts []string
	for _, r := range t.AllowedRoles {
		if label, ok := roleLabels[r]; ok {
			parts = append(parts, label)
		} else {
			parts = append(parts, r)
		}
	}
	for _, uid := range t.AllowedUserIDs {
		if name, ok := userNames[uid]; ok {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "권한이 없습니다"
	}
	return strings.Join(parts, ", ") + "만 전환 가능"
}

// resolveUserNames batch-queries auth.users for a set of user IDs and returns a map of id→name.
func (h *SchemaHandler) resolveUserNames(ctx context.Context, ids map[string]struct{}) map[string]string {
	if len(ids) == 0 {
		return nil
	}
	// Build ($1,$2,...) placeholders and args.
	args := make([]any, 0, len(ids))
	placeholders := make([]string, 0, len(ids))
	i := 1
	for uid := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		args = append(args, uid)
		i++
	}
	query := fmt.Sprintf(
		`SELECT id::text, name FROM auth.users WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		slog.Warn("resolveUserNames query failed", "error", err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]string, len(ids))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		result[id] = name
	}
	return result
}
