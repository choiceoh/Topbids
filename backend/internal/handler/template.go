package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// TemplateHandler serves app template export/import endpoints.
type TemplateHandler struct {
	store  *schema.Store
	cache  *schema.Cache
	engine *migration.Engine
	pool   *pgxpool.Pool
}

func NewTemplateHandler(store *schema.Store, cache *schema.Cache, engine *migration.Engine, pool *pgxpool.Pool) *TemplateHandler {
	return &TemplateHandler{store: store, cache: cache, engine: engine, pool: pool}
}

// --- Template types (no IDs, slug-based references) ---

type AppTemplate struct {
	Version     int                  `json:"version"`
	ExportedAt  time.Time            `json:"exported_at"`
	Collection  TemplateCollection   `json:"collection"`
	Fields      []TemplateField      `json:"fields"`
	Views       []TemplateView       `json:"views,omitempty"`
	Process     *TemplateProcess     `json:"process,omitempty"`
	Automations []TemplateAutomation `json:"automations,omitempty"`
	Charts      []TemplateChart      `json:"charts,omitempty"`
}

type TemplateCollection struct {
	Slug         string               `json:"slug"`
	Label        string               `json:"label"`
	Description  string               `json:"description,omitempty"`
	Icon         string               `json:"icon,omitempty"`
	AccessConfig *schema.AccessConfig `json:"access_config,omitempty"`
}

type TemplateField struct {
	Slug         string            `json:"slug"`
	Label        string            `json:"label"`
	FieldType    schema.FieldType  `json:"field_type"`
	IsRequired   bool              `json:"is_required"`
	IsUnique     bool              `json:"is_unique"`
	IsIndexed    bool              `json:"is_indexed"`
	DefaultValue json.RawMessage   `json:"default_value,omitempty"`
	Options      json.RawMessage   `json:"options,omitempty"`
	Width        int16             `json:"width"`
	Height       int16             `json:"height"`
	Relation     *TemplateRelation `json:"relation,omitempty"`
}

type TemplateRelation struct {
	TargetCollectionSlug string              `json:"target_collection_slug"`
	RelationType         schema.RelationType `json:"relation_type"`
	OnDelete             string              `json:"on_delete,omitempty"`
}

type TemplateView struct {
	Name      string          `json:"name"`
	ViewType  string          `json:"view_type"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder int             `json:"sort_order"`
	IsDefault bool            `json:"is_default"`
}

type TemplateProcess struct {
	IsEnabled   bool                             `json:"is_enabled"`
	Statuses    []schema.SaveProcessStatusIn     `json:"statuses"`
	Transitions []schema.SaveProcessTransitionIn `json:"transitions"`
}

type TemplateAutomation struct {
	Name          string              `json:"name"`
	IsEnabled     bool                `json:"is_enabled"`
	TriggerType   string              `json:"trigger_type"`
	TriggerConfig json.RawMessage     `json:"trigger_config,omitempty"`
	Conditions    []templateCondition `json:"conditions,omitempty"`
	Actions       []templateAction    `json:"actions,omitempty"`
}

type templateCondition struct {
	FieldSlug string `json:"field_slug"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

type templateAction struct {
	ActionType   string          `json:"action_type"`
	ActionConfig json.RawMessage `json:"action_config"`
	SortOrder    int             `json:"sort_order"`
}

type TemplateChart struct {
	Name      string          `json:"name"`
	ChartType string          `json:"chart_type"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder int             `json:"sort_order"`
}

// --- Export ---

func (h *TemplateHandler) ExportCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	col, err := h.store.GetCollection(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Build template fields.
	tplFields := make([]TemplateField, 0, len(col.Fields))
	for _, f := range col.Fields {
		tf := TemplateField{
			Slug:         f.Slug,
			Label:        f.Label,
			FieldType:    f.FieldType,
			IsRequired:   f.IsRequired,
			IsUnique:     f.IsUnique,
			IsIndexed:    f.IsIndexed,
			DefaultValue: f.DefaultValue,
			Options:      f.Options,
			Width:        f.Width,
			Height:       f.Height,
		}
		if f.Relation != nil {
			targetCol, ok := h.cache.CollectionByID(f.Relation.TargetCollectionID)
			if ok {
				tf.Relation = &TemplateRelation{
					TargetCollectionSlug: targetCol.Slug,
					RelationType:         f.Relation.RelationType,
					OnDelete:             f.Relation.OnDelete,
				}
			}
		}
		tplFields = append(tplFields, tf)
	}

	// Views.
	views, err := h.store.ListViews(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	tplViews := make([]TemplateView, 0, len(views))
	for _, v := range views {
		tplViews = append(tplViews, TemplateView{
			Name:      v.Name,
			ViewType:  v.ViewType,
			Config:    v.Config,
			SortOrder: v.SortOrder,
			IsDefault: v.IsDefault,
		})
	}

	// Process.
	var tplProcess *TemplateProcess
	proc, err := h.store.GetProcess(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	if proc.ID != "" {
		// Build index map for status ID → index.
		statusIdx := make(map[string]int, len(proc.Statuses))
		statuses := make([]schema.SaveProcessStatusIn, 0, len(proc.Statuses))
		for i, s := range proc.Statuses {
			statusIdx[s.ID] = i
			statuses = append(statuses, schema.SaveProcessStatusIn{
				Name:      s.Name,
				Color:     s.Color,
				SortOrder: s.SortOrder,
				IsInitial: s.IsInitial,
			})
		}
		transitions := make([]schema.SaveProcessTransitionIn, 0, len(proc.Transitions))
		for _, t := range proc.Transitions {
			transitions = append(transitions, schema.SaveProcessTransitionIn{
				FromIndex:    statusIdx[t.FromStatusID],
				ToIndex:      statusIdx[t.ToStatusID],
				Label:        t.Label,
				AllowedRoles: t.AllowedRoles,
			})
		}
		tplProcess = &TemplateProcess{
			IsEnabled:   proc.IsEnabled,
			Statuses:    statuses,
			Transitions: transitions,
		}
	}

	// Automations.
	tplAutomations, err := h.exportAutomations(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// Charts.
	charts, err := h.store.ListCharts(r.Context(), id)
	if err != nil {
		handleErr(w, r, err)
		return
	}
	tplCharts := make([]TemplateChart, 0, len(charts))
	for _, c := range charts {
		tplCharts = append(tplCharts, TemplateChart{
			Name:      c.Name,
			ChartType: c.ChartType,
			Config:    c.Config,
			SortOrder: c.SortOrder,
		})
	}

	tpl := AppTemplate{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Collection: TemplateCollection{
			Slug:         col.Slug,
			Label:        col.Label,
			Description:  col.Description,
			Icon:         col.Icon,
			AccessConfig: &col.AccessConfig,
		},
		Fields:      tplFields,
		Views:       tplViews,
		Process:     tplProcess,
		Automations: tplAutomations,
		Charts:      tplCharts,
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_template.json"`, col.Slug))
	writeJSON(w, http.StatusOK, tpl)
}

func (h *TemplateHandler) exportAutomations(ctx context.Context, collectionID string) ([]TemplateAutomation, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, name, is_enabled, trigger_type, trigger_config
		FROM _meta.automations
		WHERE collection_id = $1
		ORDER BY created_at`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TemplateAutomation
	for rows.Next() {
		var (
			autoID        string
			name          string
			isEnabled     bool
			triggerType   string
			triggerConfig json.RawMessage
		)
		if err := rows.Scan(&autoID, &name, &isEnabled, &triggerType, &triggerConfig); err != nil {
			return nil, err
		}

		// Conditions.
		conditions, err := h.loadTemplateConditions(ctx, autoID)
		if err != nil {
			return nil, err
		}

		// Actions.
		actions, err := h.loadTemplateActions(ctx, autoID)
		if err != nil {
			return nil, err
		}

		result = append(result, TemplateAutomation{
			Name:          name,
			IsEnabled:     isEnabled,
			TriggerType:   triggerType,
			TriggerConfig: triggerConfig,
			Conditions:    conditions,
			Actions:       actions,
		})
	}
	return result, rows.Err()
}

func (h *TemplateHandler) loadTemplateConditions(ctx context.Context, automationID string) ([]templateCondition, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT field_slug, operator, COALESCE(value, ''), sort_order
		FROM _meta.automation_conditions
		WHERE automation_id = $1 ORDER BY sort_order`, automationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []templateCondition
	for rows.Next() {
		var c templateCondition
		if err := rows.Scan(&c.FieldSlug, &c.Operator, &c.Value, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *TemplateHandler) loadTemplateActions(ctx context.Context, automationID string) ([]templateAction, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT action_type, action_config, sort_order
		FROM _meta.automation_actions
		WHERE automation_id = $1 ORDER BY sort_order`, automationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []templateAction
	for rows.Next() {
		var a templateAction
		if err := rows.Scan(&a.ActionType, &a.ActionConfig, &a.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Import ---

func (h *TemplateHandler) ImportTemplate(w http.ResponseWriter, r *http.Request) {
	var tpl AppTemplate
	if err := readJSON(r, &tpl); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if tpl.Version != 1 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported template version %d", tpl.Version))
		return
	}
	if tpl.Collection.Slug == "" || tpl.Collection.Label == "" {
		writeError(w, http.StatusBadRequest, "collection slug and label are required")
		return
	}

	// Allow slug override via query param.
	slugOverride := r.URL.Query().Get("slug")
	if slugOverride != "" {
		tpl.Collection.Slug = slugOverride
	}

	// Validate slug format.
	if err := schema.ValidateSlug(tpl.Collection.Slug); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid slug: %v", err))
		return
	}

	// Check slug uniqueness.
	if _, exists := h.cache.CollectionBySlug(tpl.Collection.Slug); exists {
		writeError(w, http.StatusConflict, fmt.Sprintf("collection slug %q already exists; use ?slug=new_slug to override", tpl.Collection.Slug))
		return
	}

	ctx := r.Context()
	var warnings []string

	// Separate relation and non-relation fields.
	var normalFields []schema.CreateFieldIn
	var selfRefFields []TemplateField
	for _, tf := range tpl.Fields {
		if tf.Relation == nil {
			normalFields = append(normalFields, toCreateFieldIn(tf, nil))
			continue
		}
		if tf.Relation.TargetCollectionSlug == tpl.Collection.Slug {
			// Self-reference: defer to second pass.
			selfRefFields = append(selfRefFields, tf)
			continue
		}
		// External relation: resolve target.
		targetCol, ok := h.cache.CollectionBySlug(tf.Relation.TargetCollectionSlug)
		if !ok {
			// Downgrade to text field.
			warnings = append(warnings, fmt.Sprintf("relation target %q not found — field %q imported as text", tf.Relation.TargetCollectionSlug, tf.Slug))
			downgraded := tf
			downgraded.FieldType = schema.FieldText
			downgraded.Relation = nil
			normalFields = append(normalFields, toCreateFieldIn(downgraded, nil))
			continue
		}
		normalFields = append(normalFields, toCreateFieldIn(tf, &schema.CreateRelIn{
			TargetCollectionID: targetCol.ID,
			RelationType:       tf.Relation.RelationType,
			OnDelete:           tf.Relation.OnDelete,
		}))
	}

	// 1. Create collection with non-self-referencing fields.
	createReq := &schema.CreateCollectionReq{
		Slug:         tpl.Collection.Slug,
		Label:        tpl.Collection.Label,
		Description:  tpl.Collection.Description,
		Icon:         tpl.Collection.Icon,
		AccessConfig: tpl.Collection.AccessConfig,
		Fields:       normalFields,
	}

	col, err := h.engine.CreateCollection(ctx, createReq)
	if err != nil {
		handleErr(w, r, err)
		return
	}

	// 2. Add self-referencing fields (second pass).
	for _, tf := range selfRefFields {
		fieldReq := &schema.CreateFieldIn{
			Slug:         tf.Slug,
			Label:        tf.Label,
			FieldType:    tf.FieldType,
			IsRequired:   tf.IsRequired,
			IsUnique:     tf.IsUnique,
			IsIndexed:    tf.IsIndexed,
			DefaultValue: tf.DefaultValue,
			Options:      tf.Options,
			Width:        tf.Width,
			Height:       tf.Height,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: col.ID,
				RelationType:       tf.Relation.RelationType,
				OnDelete:           tf.Relation.OnDelete,
			},
		}
		if _, _, err := h.engine.AddField(ctx, col.ID, fieldReq, true); err != nil {
			warnings = append(warnings, fmt.Sprintf("self-reference field %q failed: %v", tf.Slug, err))
		}
	}

	// 3. Create views.
	for _, tv := range tpl.Views {
		_, err := h.store.CreateView(ctx, col.ID, &schema.CreateViewReq{
			Name:      tv.Name,
			ViewType:  tv.ViewType,
			Config:    tv.Config,
			SortOrder: tv.SortOrder,
			IsDefault: tv.IsDefault,
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("view %q failed: %v", tv.Name, err))
		}
	}

	// 4. Save process.
	if tpl.Process != nil && len(tpl.Process.Statuses) > 0 {
		_, err := h.engine.SaveProcess(ctx, col.ID, &schema.SaveProcessReq{
			IsEnabled:   tpl.Process.IsEnabled,
			Statuses:    tpl.Process.Statuses,
			Transitions: tpl.Process.Transitions,
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("process failed: %v", err))
		}
	}

	// 5. Create automations.
	for _, ta := range tpl.Automations {
		if err := h.createAutomation(ctx, col.ID, ta); err != nil {
			warnings = append(warnings, fmt.Sprintf("automation %q failed: %v", ta.Name, err))
		}
	}

	// 6. Create charts.
	for _, tc := range tpl.Charts {
		_, err := h.store.CreateChart(ctx, col.ID, schema.CreateChartReq{
			Name:      tc.Name,
			ChartType: tc.ChartType,
			Config:    tc.Config,
			SortOrder: tc.SortOrder,
		}, "")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("chart %q failed: %v", tc.Name, err))
		}
	}

	if warnings == nil {
		warnings = []string{}
	}

	// Determine whether any warning represents a data-loss scenario
	// (e.g. relation fields downgraded to text) so the caller can decide
	// whether the import result is acceptable.
	hasDataLoss := false
	for _, w := range warnings {
		if len(w) > 0 && (strings.Contains(w, "imported as text") || strings.Contains(w, "self-reference field")) {
			hasDataLoss = true
			break
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"collection_id": col.ID,
		"slug":          col.Slug,
		"warnings":      warnings,
		"has_data_loss": hasDataLoss,
	})
}

func (h *TemplateHandler) createAutomation(ctx context.Context, collectionID string, ta TemplateAutomation) error {
	triggerConfig := ta.TriggerConfig
	if triggerConfig == nil {
		triggerConfig = json.RawMessage(`{}`)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var automationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO _meta.automations (collection_id, name, is_enabled, trigger_type, trigger_config)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		collectionID, ta.Name, ta.IsEnabled, ta.TriggerType, triggerConfig,
	).Scan(&automationID)
	if err != nil {
		return err
	}

	for i, c := range ta.Conditions {
		_, err := tx.Exec(ctx, `
			INSERT INTO _meta.automation_conditions (automation_id, field_slug, operator, value, sort_order)
			VALUES ($1, $2, $3, $4, $5)`,
			automationID, c.FieldSlug, c.Operator, c.Value, i,
		)
		if err != nil {
			return err
		}
	}

	for i, a := range ta.Actions {
		actionConfig := a.ActionConfig
		if actionConfig == nil {
			actionConfig = json.RawMessage(`{}`)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO _meta.automation_actions (automation_id, action_type, action_config, sort_order)
			VALUES ($1, $2, $3, $4)`,
			automationID, a.ActionType, actionConfig, i,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func toCreateFieldIn(tf TemplateField, rel *schema.CreateRelIn) schema.CreateFieldIn {
	return schema.CreateFieldIn{
		Slug:         tf.Slug,
		Label:        tf.Label,
		FieldType:    tf.FieldType,
		IsRequired:   tf.IsRequired,
		IsUnique:     tf.IsUnique,
		IsIndexed:    tf.IsIndexed,
		DefaultValue: tf.DefaultValue,
		Options:      tf.Options,
		Width:        tf.Width,
		Height:       tf.Height,
		Relation:     rel,
	}
}
