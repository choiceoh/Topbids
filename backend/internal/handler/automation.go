package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/middleware"
)

// AutomationHandler provides CRUD for automation rules.
type AutomationHandler struct {
	pool *pgxpool.Pool
}

func NewAutomationHandler(pool *pgxpool.Pool) *AutomationHandler {
	return &AutomationHandler{pool: pool}
}

// --- request/response types ---

type createAutomationReq struct {
	Name          string          `json:"name"`
	IsEnabled     bool            `json:"is_enabled"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Conditions    []conditionReq  `json:"conditions"`
	Actions       []actionReq     `json:"actions"`
}

type conditionReq struct {
	FieldSlug string `json:"field_slug"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

type actionReq struct {
	ActionType   string          `json:"action_type"`
	ActionConfig json.RawMessage `json:"action_config"`
	SortOrder    int             `json:"sort_order"`
}

// --- List ---

func (h *AutomationHandler) List(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")

	rows, err := h.pool.Query(r.Context(), `
		SELECT a.id, a.collection_id, a.name, a.is_enabled, a.trigger_type, a.trigger_config, a.created_by, a.created_at, a.updated_at,
		       (SELECT count(*) FROM _meta.automation_actions aa WHERE aa.automation_id = a.id) AS action_count
		FROM _meta.automations a
		WHERE a.collection_id = $1
		ORDER BY a.created_at`, collectionID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	type automationRow struct {
		ID            string          `json:"id"`
		CollectionID  string          `json:"collection_id"`
		Name          string          `json:"name"`
		IsEnabled     bool            `json:"is_enabled"`
		TriggerType   string          `json:"trigger_type"`
		TriggerConfig json.RawMessage `json:"trigger_config"`
		CreatedBy     *string         `json:"created_by"`
		CreatedAt     string          `json:"created_at"`
		UpdatedAt     string          `json:"updated_at"`
		ActionCount   int             `json:"action_count"`
	}

	var result []automationRow
	for rows.Next() {
		var a automationRow
		if err := rows.Scan(&a.ID, &a.CollectionID, &a.Name, &a.IsEnabled, &a.TriggerType, &a.TriggerConfig, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt, &a.ActionCount); err != nil {
			handleErr(w, r, err)
			return
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		handleErr(w, r, err)
		return
	}
	if result == nil {
		result = []automationRow{}
	}

	writeJSON(w, http.StatusOK, result)
}

// --- Get ---

func (h *AutomationHandler) Get(w http.ResponseWriter, r *http.Request) {
	automationID := chi.URLParam(r, "automationId")

	// Main row.
	var a struct {
		ID            string          `json:"id"`
		CollectionID  string          `json:"collection_id"`
		Name          string          `json:"name"`
		IsEnabled     bool            `json:"is_enabled"`
		TriggerType   string          `json:"trigger_type"`
		TriggerConfig json.RawMessage `json:"trigger_config"`
		CreatedBy     *string         `json:"created_by"`
		CreatedAt     string          `json:"created_at"`
		UpdatedAt     string          `json:"updated_at"`
		Conditions    []any           `json:"conditions"`
		Actions       []any           `json:"actions"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, collection_id, name, is_enabled, trigger_type, trigger_config, created_by, created_at, updated_at
		FROM _meta.automations WHERE id = $1`, automationID,
	).Scan(&a.ID, &a.CollectionID, &a.Name, &a.IsEnabled, &a.TriggerType, &a.TriggerConfig, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}

	// Conditions.
	cRows, err := h.pool.Query(r.Context(), `
		SELECT id, field_slug, operator, COALESCE(value, ''), sort_order
		FROM _meta.automation_conditions WHERE automation_id = $1 ORDER BY sort_order`, automationID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c struct {
			ID        string `json:"id"`
			FieldSlug string `json:"field_slug"`
			Operator  string `json:"operator"`
			Value     string `json:"value"`
			SortOrder int    `json:"sort_order"`
		}
		if err := cRows.Scan(&c.ID, &c.FieldSlug, &c.Operator, &c.Value, &c.SortOrder); err != nil {
			handleErr(w, r, err)
			return
		}
		a.Conditions = append(a.Conditions, c)
	}
	if err := cRows.Err(); err != nil {
		handleErr(w, r, err)
		return
	}
	if a.Conditions == nil {
		a.Conditions = []any{}
	}

	// Actions.
	aRows, err := h.pool.Query(r.Context(), `
		SELECT id, action_type, action_config, sort_order
		FROM _meta.automation_actions WHERE automation_id = $1 ORDER BY sort_order`, automationID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer aRows.Close()
	for aRows.Next() {
		var act struct {
			ID           string          `json:"id"`
			ActionType   string          `json:"action_type"`
			ActionConfig json.RawMessage `json:"action_config"`
			SortOrder    int             `json:"sort_order"`
		}
		if err := aRows.Scan(&act.ID, &act.ActionType, &act.ActionConfig, &act.SortOrder); err != nil {
			handleErr(w, r, err)
			return
		}
		a.Actions = append(a.Actions, act)
	}
	if err := aRows.Err(); err != nil {
		handleErr(w, r, err)
		return
	}
	if a.Actions == nil {
		a.Actions = []any{}
	}

	writeJSON(w, http.StatusOK, a)
}

// --- Create ---

func (h *AutomationHandler) Create(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "id")

	var req createAutomationReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validateAutomationReq(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TriggerConfig == nil {
		req.TriggerConfig = json.RawMessage(`{}`)
	}

	user, _ := middleware.GetUser(r.Context())

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var automationID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO _meta.automations (collection_id, name, is_enabled, trigger_type, trigger_config, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		collectionID, req.Name, req.IsEnabled, req.TriggerType, req.TriggerConfig, user.UserID,
	).Scan(&automationID)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	for i, c := range req.Conditions {
		_, err := tx.Exec(r.Context(), `
			INSERT INTO _meta.automation_conditions (automation_id, field_slug, operator, value, sort_order)
			VALUES ($1, $2, $3, $4, $5)`,
			automationID, c.FieldSlug, c.Operator, c.Value, i,
		)
		if err != nil {
			handleErr(w, r, err)
			return
		}
	}

	for i, a := range req.Actions {
		if a.ActionConfig == nil {
			a.ActionConfig = json.RawMessage(`{}`)
		}
		_, err := tx.Exec(r.Context(), `
			INSERT INTO _meta.automation_actions (automation_id, action_type, action_config, sort_order)
			VALUES ($1, $2, $3, $4)`,
			automationID, a.ActionType, a.ActionConfig, i,
		)
		if err != nil {
			handleErr(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		handleErr(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": automationID})
}

// --- Update ---

func (h *AutomationHandler) Update(w http.ResponseWriter, r *http.Request) {
	automationID := chi.URLParam(r, "automationId")

	var req createAutomationReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateAutomationReq(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TriggerConfig == nil {
		req.TriggerConfig = json.RawMessage(`{}`)
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	tag, err := tx.Exec(r.Context(), `
		UPDATE _meta.automations
		SET name = $1, is_enabled = $2, trigger_type = $3, trigger_config = $4, updated_at = now()
		WHERE id = $5`,
		req.Name, req.IsEnabled, req.TriggerType, req.TriggerConfig, automationID,
	)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}

	// Delete and re-insert conditions and actions.
	if _, err := tx.Exec(r.Context(), `DELETE FROM _meta.automation_conditions WHERE automation_id = $1`, automationID); err != nil {
		handleErr(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM _meta.automation_actions WHERE automation_id = $1`, automationID); err != nil {
		handleErr(w, r, err)
		return
	}

	for i, c := range req.Conditions {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO _meta.automation_conditions (automation_id, field_slug, operator, value, sort_order)
			VALUES ($1, $2, $3, $4, $5)`,
			automationID, c.FieldSlug, c.Operator, c.Value, i,
		); err != nil {
			handleErr(w, r, err)
			return
		}
	}

	for i, a := range req.Actions {
		if a.ActionConfig == nil {
			a.ActionConfig = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO _meta.automation_actions (automation_id, action_type, action_config, sort_order)
			VALUES ($1, $2, $3, $4)`,
			automationID, a.ActionType, a.ActionConfig, i,
		); err != nil {
			handleErr(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		handleErr(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// --- Delete ---

func (h *AutomationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	automationID := chi.URLParam(r, "automationId")

	tag, err := h.pool.Exec(r.Context(), `DELETE FROM _meta.automations WHERE id = $1`, automationID)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "automation not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- ListRuns ---

func (h *AutomationHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	automationID := chi.URLParam(r, "automationId")
	params := r.URL.Query()
	_, limit, offset := ParsePagination(params)

	var total int64
	h.pool.QueryRow(r.Context(), `SELECT count(*) FROM _history.automation_runs WHERE automation_id = $1`, automationID).Scan(&total)

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, automation_id, collection_id, record_id, trigger_type, status, COALESCE(error_message, ''), COALESCE(duration_ms, 0), created_at
		FROM _history.automation_runs
		WHERE automation_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, automationID, limit, offset)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	defer rows.Close()

	type runRow struct {
		ID           string `json:"id"`
		AutomationID string `json:"automation_id"`
		CollectionID string `json:"collection_id"`
		RecordID     string `json:"record_id"`
		TriggerType  string `json:"trigger_type"`
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message,omitempty"`
		DurationMs   int    `json:"duration_ms"`
		CreatedAt    string `json:"created_at"`
	}

	var result []runRow
	for rows.Next() {
		var run runRow
		if err := rows.Scan(&run.ID, &run.AutomationID, &run.CollectionID, &run.RecordID, &run.TriggerType, &run.Status, &run.ErrorMessage, &run.DurationMs, &run.CreatedAt); err != nil {
			handleErr(w, r, err)
			return
		}
		result = append(result, run)
	}
	if result == nil {
		result = []runRow{}
	}

	page, _, _ := ParsePagination(params)
	writeList(w, result, total, page, limit)
}

// --- Validation ---

var validTriggerTypes = map[string]bool{
	"record_created": true, "record_updated": true, "record_deleted": true,
	"status_change": true, "schedule": true, "form_submit": true,
}

var validActionTypes = map[string]bool{
	"send_notification": true, "update_field": true, "call_webhook": true,
}

var validConditionOperators = map[string]bool{
	"equals": true, "not_equals": true, "contains": true,
	"gt": true, "lt": true, "is_empty": true, "is_not_empty": true,
}

func validateAutomationReq(req createAutomationReq) error {
	if req.TriggerType == "" {
		return fmt.Errorf("trigger_type is required")
	}
	if !validTriggerTypes[req.TriggerType] {
		return fmt.Errorf("invalid trigger_type %q; allowed: record_created, record_updated, record_deleted, status_change, schedule, form_submit", req.TriggerType)
	}
	for i, c := range req.Conditions {
		if c.FieldSlug == "" {
			return fmt.Errorf("conditions[%d].field_slug is required", i)
		}
		if c.Operator == "" {
			return fmt.Errorf("conditions[%d].operator is required", i)
		}
		if !validConditionOperators[c.Operator] {
			return fmt.Errorf("conditions[%d]: invalid operator %q; allowed: equals, not_equals, contains, gt, lt, is_empty, is_not_empty", i, c.Operator)
		}
	}
	for i, a := range req.Actions {
		if a.ActionType == "" {
			return fmt.Errorf("actions[%d].action_type is required", i)
		}
		if !validActionTypes[a.ActionType] {
			return fmt.Errorf("actions[%d]: invalid action_type %q; allowed: send_notification, update_field, call_webhook", i, a.ActionType)
		}
	}
	return nil
}
