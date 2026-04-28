package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

// GetProcess returns the process config for a collection, preferring the cache.
func (e *Engine) GetProcess(ctx context.Context, collectionID string) (schema.Process, error) {
	if p, ok := e.cache.ProcessByCollectionID(collectionID); ok {
		return p, nil
	}
	return e.store.GetProcess(ctx, collectionID)
}

// SaveProcess validates and saves the process configuration, managing the _status
// column DDL when the process is enabled or disabled.
func (e *Engine) SaveProcess(ctx context.Context, collectionID string, req *schema.SaveProcessReq) (schema.Process, error) {
	if err := schema.ValidateProcessSave(req); err != nil {
		return schema.Process{}, err
	}

	col, ok := e.cache.CollectionByID(collectionID)
	if !ok {
		return schema.Process{}, fmt.Errorf("collection %s: %w", collectionID, schema.ErrNotFound)
	}

	// Check previous state to determine DDL changes.
	wasEnabled := false
	if prev, ok := e.cache.ProcessByCollectionID(collectionID); ok {
		wasEnabled = prev.IsEnabled
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return schema.Process{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	proc, err := e.store.SaveProcessTx(ctx, tx, collectionID, req)
	if err != nil {
		return schema.Process{}, err
	}

	// DDL: add or drop _status column as needed.
	var ddlUp, ddlDown []string
	var op Operation

	switch {
	case !wasEnabled && req.IsEnabled:
		// Newly enabled: add _status column.
		ddlUp, ddlDown = GenerateAddStatusColumn(col.Slug)
		op = OpEnableProcess
	case wasEnabled && !req.IsEnabled:
		// Disabled: drop _status column.
		ddlUp, ddlDown = GenerateDropStatusColumn(col.Slug)
		op = OpDisableProcess
	default:
		// No DDL change needed (both enabled or both disabled).
	}

	if len(ddlUp) > 0 {
		if err := execStmts(ctx, tx, ddlUp); err != nil {
			return schema.Process{}, fmt.Errorf("exec process DDL: %w", err)
		}
		payload, _ := json.Marshal(map[string]any{
			"collection_id":   collectionID,
			"collection_slug": col.Slug,
			"is_enabled":      req.IsEnabled,
		})
		if err := recordMigration(ctx, tx, collectionID, op, payload, ddlUp, ddlDown, Safe); err != nil {
			return schema.Process{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return schema.Process{}, fmt.Errorf("commit: %w", err)
	}

	if err := e.cache.ReloadProcess(ctx, collectionID); err != nil {
		slog.Warn("cache reload process after SaveProcess", "error", err)
	}
	return proc, nil
}
