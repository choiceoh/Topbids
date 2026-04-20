package automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func logRun(ctx context.Context, pool *pgxpool.Pool, automationID, collectionID, recordID, triggerType, status, errMsg string, duration time.Duration) {
	durationMs := int(duration.Milliseconds())
	_, err := pool.Exec(ctx, `
		INSERT INTO _history.automation_runs (automation_id, collection_id, record_id, trigger_type, status, error_message, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		automationID, collectionID, recordID, triggerType, status, errMsg, durationMs,
	)
	if err != nil {
		slog.Error("automation: failed to log run", "automation_id", automationID, "error", err)
	}
}
