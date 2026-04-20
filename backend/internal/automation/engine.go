// Package automation implements the trigger->condition->action pipeline.
//
// When a record is created, updated, deleted, or transitions status, the event bus
// notifies the automation engine. The engine:
//  1. Loads enabled automations for the collection and trigger type
//  2. Evaluates conditions against the record (all conditions are AND-joined)
//  3. Executes actions (send_notification, update_field, call_webhook)
//  4. Logs the run result (success/error/skipped)
//
// Execution is asynchronous via a worker pool to avoid blocking API responses.
// An automation_depth context value prevents infinite loops: if an action updates
// a record, the resulting event is ignored (depth > 0).
package automation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/infra/workerpool"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

type contextKey string

const depthKey contextKey = "automation_depth"

// Engine orchestrates the trigger→condition→action pipeline.
type Engine struct {
	baseCtx context.Context
	pool    *pgxpool.Pool
	cache   *schema.Cache
	wp      *workerpool.Pool
	webhook *WebhookSender
}

// New creates an automation engine.
func New(pool *pgxpool.Pool, cache *schema.Cache, wp *workerpool.Pool) *Engine {
	return &Engine{
		baseCtx: context.Background(),
		pool:    pool,
		cache:   cache,
		wp:      wp,
		webhook: NewWebhookSender(),
	}
}

// SetBaseContext sets the parent context for all automation executions,
// enabling graceful cancellation on shutdown. Must be called before
// the engine starts processing events.
func (e *Engine) SetBaseContext(ctx context.Context) {
	e.baseCtx = ctx
}

// Subscribe registers the engine on the event bus.
func (e *Engine) Subscribe(bus *events.Bus) {
	bus.Subscribe(func(ctx context.Context, ev events.Event) {
		triggerType := mapEventType(ev.Type)
		if triggerType == "" {
			return
		}

		// Prevent infinite loops: skip if already inside an automation.
		if depth, _ := ctx.Value(depthKey).(int); depth > 0 {
			return
		}

		automations, err := e.loadAutomations(ctx, ev.CollectionID, triggerType)
		if err != nil {
			slog.Error("automation: load failed", "collection_id", ev.CollectionID, "error", err)
			return
		}

		for _, a := range automations {
			a := a
			e.wp.Submit(func() {
				autoCtx, cancel := context.WithTimeout(e.baseCtx, 2*time.Minute)
				defer cancel()
				autoCtx = context.WithValue(autoCtx, depthKey, 1)
				e.execute(autoCtx, a, ev)
			})
		}
	})
}

// mapEventType converts an event bus EventType to the corresponding automation
// trigger type string. Returns "" for event types that have no automation trigger,
// causing the caller to skip processing.
func mapEventType(t events.EventType) string {
	switch t {
	case events.EventRecordCreate:
		return TriggerRecordCreated
	case events.EventRecordUpdate:
		return TriggerRecordUpdated
	case events.EventRecordDelete:
		return TriggerRecordDeleted
	case events.EventStateChange:
		return TriggerStatusChange
	case events.EventFormSubmit:
		return TriggerFormSubmit
	default:
		return ""
	}
}

// execute runs the full evaluation pipeline for a single automation against an event:
//  1. Checks trigger-specific config (e.g. from/to status must match for status_change)
//  2. Evaluates all AND-joined conditions against the record data
//  3. Executes actions sequentially in sort_order
//  4. Logs the run result to _meta.automation_runs (success, error, or skipped)
//
// If the trigger config doesn't match or conditions aren't met, the run is logged
// as "skipped" with the reason. Errors during action execution are logged as "error".
func (e *Engine) execute(ctx context.Context, a Automation, ev events.Event) {
	start := time.Now()

	// Check trigger-specific config (e.g. from/to status, form slug).
	if !matchTriggerConfig(a, ev.StatusFrom, ev.StatusTo, ev.FormSlug) {
		logRun(ctx, e.pool, a.ID, a.CollectionID, ev.RecordID, a.TriggerType, "skipped", "trigger config mismatch", time.Since(start))
		return
	}

	// Evaluate conditions against the record.
	record := ev.NewRecord
	if record == nil {
		record = ev.OldRecord
	}
	if len(a.Conditions) > 0 && !evaluateConditions(a.Conditions, record) {
		logRun(ctx, e.pool, a.ID, a.CollectionID, ev.RecordID, a.TriggerType, "skipped", "conditions not met", time.Since(start))
		return
	}

	// Execute all actions.
	if err := e.executeActions(ctx, a, ev); err != nil {
		slog.Error("automation: execution failed", "automation_id", a.ID, "error", err)
		logRun(ctx, e.pool, a.ID, a.CollectionID, ev.RecordID, a.TriggerType, "error", err.Error(), time.Since(start))
		return
	}

	logRun(ctx, e.pool, a.ID, a.CollectionID, ev.RecordID, a.TriggerType, "success", "", time.Since(start))
}

// loadAutomations fetches enabled automations for a collection and trigger type
// from _meta.automations, then eagerly loads their conditions (from
// _meta.automation_conditions) and actions (from _meta.automation_actions).
// Results are ordered by created_at so older automations fire first.
func (e *Engine) loadAutomations(ctx context.Context, collectionID, triggerType string) ([]Automation, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, collection_id, name, is_enabled, trigger_type, trigger_config, created_by, created_at, updated_at
		FROM _meta.automations
		WHERE collection_id = $1 AND trigger_type = $2 AND is_enabled = TRUE
		ORDER BY created_at`, collectionID, triggerType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var automations []Automation
	for rows.Next() {
		var a Automation
		if err := rows.Scan(&a.ID, &a.CollectionID, &a.Name, &a.IsEnabled, &a.TriggerType, &a.TriggerConfig, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		automations = append(automations, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load conditions and actions for each automation.
	for i := range automations {
		automations[i].Conditions, err = e.loadConditions(ctx, automations[i].ID)
		if err != nil {
			return nil, err
		}
		automations[i].Actions, err = e.loadActions(ctx, automations[i].ID)
		if err != nil {
			return nil, err
		}
	}

	return automations, nil
}

func (e *Engine) loadConditions(ctx context.Context, automationID string) ([]Condition, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, field_slug, operator, COALESCE(value, ''), sort_order
		FROM _meta.automation_conditions
		WHERE automation_id = $1
		ORDER BY sort_order`, automationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conditions []Condition
	for rows.Next() {
		var c Condition
		if err := rows.Scan(&c.ID, &c.FieldSlug, &c.Operator, &c.Value, &c.SortOrder); err != nil {
			return nil, err
		}
		conditions = append(conditions, c)
	}
	return conditions, rows.Err()
}

func (e *Engine) loadActions(ctx context.Context, automationID string) ([]Action, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, action_type, action_config, sort_order
		FROM _meta.automation_actions
		WHERE automation_id = $1
		ORDER BY sort_order`, automationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []Action
	for rows.Next() {
		var a Action
		var cfgBytes []byte
		if err := rows.Scan(&a.ID, &a.ActionType, &cfgBytes, &a.SortOrder); err != nil {
			return nil, err
		}
		a.ActionConfig = json.RawMessage(cfgBytes)
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
