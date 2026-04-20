package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/events"
)

func (e *Engine) executeActions(ctx context.Context, a Automation, ev events.Event) error {
	var errs []string
	for _, action := range a.Actions {
		if err := e.executeAction(ctx, action, ev); err != nil {
			slog.Error("automation: action failed",
				"automation_id", a.ID,
				"action_id", action.ID,
				"action_type", action.ActionType,
				"error", err,
			)
			errs = append(errs, fmt.Sprintf("action %s (%s): %s", action.ID, action.ActionType, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d action(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (e *Engine) executeAction(ctx context.Context, action Action, ev events.Event) error {
	switch action.ActionType {
	case ActionSendNotification:
		return e.actionNotify(ctx, action, ev)
	case ActionUpdateField:
		return e.actionUpdateField(ctx, action, ev)
	case ActionCallWebhook:
		return e.actionWebhook(ctx, action, ev)
	default:
		return fmt.Errorf("unknown action type: %s", action.ActionType)
	}
}

func (e *Engine) actionNotify(ctx context.Context, action Action, ev events.Event) error {
	var cfg NotificationConfig
	if err := json.Unmarshal(action.ActionConfig, &cfg); err != nil {
		return fmt.Errorf("parse notification config: %w", err)
	}

	userID, err := e.resolveRecipient(ctx, cfg, ev)
	if err != nil {
		return err
	}
	if userID == "" {
		slog.Warn("automation: no recipient resolved", "action_id", action.ID)
		return nil
	}

	_, err = e.pool.Exec(ctx, `
		INSERT INTO _meta.notifications (user_id, type, title, body, ref_collection_id, ref_record_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, "automation", cfg.Title, cfg.Body, ev.CollectionID, ev.RecordID,
	)
	return err
}

func (e *Engine) resolveRecipient(ctx context.Context, cfg NotificationConfig, ev events.Event) (string, error) {
	switch cfg.Recipient {
	case "record_creator":
		record := ev.NewRecord
		if record == nil {
			record = ev.OldRecord
		}
		if record == nil {
			return "", nil
		}
		if uid, ok := record["created_by"].(string); ok {
			return uid, nil
		}
		return "", nil

	case "specific_user":
		return cfg.UserID, nil

	case "field_ref":
		record := ev.NewRecord
		if record == nil {
			record = ev.OldRecord
		}
		if record == nil {
			return "", nil
		}
		if uid, ok := record[cfg.FieldSlug].(string); ok {
			return uid, nil
		}
		return "", nil

	default:
		return "", fmt.Errorf("unknown recipient type: %s", cfg.Recipient)
	}
}

func (e *Engine) actionUpdateField(ctx context.Context, action Action, ev events.Event) error {
	var cfg UpdateFieldConfig
	if err := json.Unmarshal(action.ActionConfig, &cfg); err != nil {
		return fmt.Errorf("parse update_field config: %w", err)
	}

	col, ok := e.cache.CollectionByID(ev.CollectionID)
	if !ok {
		return fmt.Errorf("collection %s not found", ev.CollectionID)
	}

	// Use pgx.Identifier for safe quoting.
	table := pgx.Identifier{"data", col.Slug}.Sanitize()
	column := pgx.Identifier{cfg.FieldSlug}.Sanitize()

	sql := fmt.Sprintf(
		`UPDATE %s SET %s = $1, "updated_at" = now() WHERE id = $2 AND deleted_at IS NULL`,
		table, column,
	)

	tag, err := e.pool.Exec(ctx, sql, cfg.Value, ev.RecordID)
	if err != nil {
		return fmt.Errorf("update field: %w", err)
	}
	if tag.RowsAffected() == 0 {
		slog.Warn("automation: update_field affected 0 rows", "record_id", ev.RecordID)
	}

	// Record change history.
	recordAutomationChange(ctx, e.pool, ev.CollectionID, ev.RecordID, cfg.FieldSlug, cfg.Value)

	return nil
}

func recordAutomationChange(ctx context.Context, pool *pgxpool.Pool, collectionID, recordID, fieldSlug, value string) {
	diff := map[string]any{
		fieldSlug: map[string]any{"new": value, "by": "automation"},
	}
	diffJSON, _ := json.Marshal(diff)
	_, _ = pool.Exec(ctx, `
		INSERT INTO _history.record_changes (collection_id, record_id, user_name, operation, diff)
		VALUES ($1, $2, $3, $4, $5)`,
		collectionID, recordID, "automation", "update", diffJSON,
	)
}

func (e *Engine) actionWebhook(ctx context.Context, action Action, ev events.Event) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(action.ActionConfig, &cfg); err != nil {
		return fmt.Errorf("parse webhook config: %w", err)
	}

	record := ev.NewRecord
	if record == nil {
		record = ev.OldRecord
	}

	payload := WebhookPayload{
		CollectionID: ev.CollectionID,
		RecordID:     ev.RecordID,
		TriggerType:  string(ev.Type),
		Record:       record,
	}

	return e.webhook.Send(ctx, cfg, payload)
}
