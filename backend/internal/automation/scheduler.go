package automation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/events"
)

// Scheduler periodically checks for schedule-type automations and fires them.
type Scheduler struct {
	engine   *Engine
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewScheduler creates a scheduler that checks every interval.
func NewScheduler(engine *Engine, interval time.Duration) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		engine:   engine,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the scheduler loop in a goroutine.
// The ctx controls the scheduler lifecycle — cancelling it stops the loop
// and propagates to all in-flight automation executions.
func (s *Scheduler) Start(ctx context.Context) {
	s.cancel() // cancel the placeholder context
	s.ctx, s.cancel = context.WithCancel(ctx)
	go s.run()
}

// Stop signals the scheduler to stop.
func (s *Scheduler) Stop() {
	s.cancel()
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *Scheduler) tick(now time.Time) {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()

	// Fetch all enabled schedule automations.
	rows, err := s.engine.pool.Query(ctx, `
		SELECT id, collection_id, name, is_enabled, trigger_type, trigger_config, created_by, created_at, updated_at
		FROM _meta.automations
		WHERE trigger_type = $1 AND is_enabled = TRUE
		ORDER BY created_at`, TriggerSchedule)
	if err != nil {
		slog.Error("scheduler: query failed", "error", err)
		return
	}
	defer rows.Close()

	var automations []Automation
	for rows.Next() {
		var a Automation
		if err := rows.Scan(&a.ID, &a.CollectionID, &a.Name, &a.IsEnabled, &a.TriggerType, &a.TriggerConfig, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			slog.Error("scheduler: scan failed", "error", err)
			continue
		}
		automations = append(automations, a)
	}
	if err := rows.Err(); err != nil {
		slog.Error("scheduler: rows error", "error", err)
		return
	}

	for _, a := range automations {
		var cfg ScheduleConfig
		if err := json.Unmarshal(a.TriggerConfig, &cfg); err != nil {
			slog.Error("scheduler: invalid config", "automation_id", a.ID, "error", err)
			continue
		}

		if !cronMatches(cfg.Cron, cfg.Timezone, now) {
			continue
		}

		// Load conditions and actions.
		a.Conditions, err = s.engine.loadConditions(ctx, a.ID)
		if err != nil {
			slog.Error("scheduler: load conditions failed", "automation_id", a.ID, "error", err)
			continue
		}
		a.Actions, err = s.engine.loadActions(ctx, a.ID)
		if err != nil {
			slog.Error("scheduler: load actions failed", "automation_id", a.ID, "error", err)
			continue
		}

		slog.Info("scheduler: firing", "automation_id", a.ID, "name", a.Name)
		s.engine.executeScheduled(ctx, a)
	}
}

// executeScheduled runs a scheduled automation (no record context).
func (e *Engine) executeScheduled(ctx context.Context, a Automation) {
	start := time.Now()

	if err := e.executeActions(ctx, a, emptyScheduleEvent(a.CollectionID)); err != nil {
		slog.Error("scheduler: execution failed", "automation_id", a.ID, "error", err)
		logRun(ctx, e.pool, a.ID, a.CollectionID, "", a.TriggerType, "error", err.Error(), time.Since(start))
		return
	}

	logRun(ctx, e.pool, a.ID, a.CollectionID, "", a.TriggerType, "success", "", time.Since(start))
}

// cronMatches checks if the given cron expression matches the current minute.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week.
func cronMatches(expr, tz string, now time.Time) bool {
	if expr == "" {
		return false
	}

	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	t := now.In(loc)

	fields := splitFields(expr)
	if len(fields) != 5 {
		return false
	}

	return fieldMatches(fields[0], t.Minute(), 0, 59) &&
		fieldMatches(fields[1], t.Hour(), 0, 23) &&
		fieldMatches(fields[2], t.Day(), 1, 31) &&
		fieldMatches(fields[3], int(t.Month()), 1, 12) &&
		fieldMatches(fields[4], int(t.Weekday()), 0, 6)
}

func splitFields(expr string) []string {
	var fields []string
	field := ""
	for _, c := range expr {
		if c == ' ' || c == '\t' {
			if field != "" {
				fields = append(fields, field)
				field = ""
			}
		} else {
			field += string(c)
		}
	}
	if field != "" {
		fields = append(fields, field)
	}
	return fields
}

func fieldMatches(field string, value, min, max int) bool {
	if field == "*" {
		return true
	}

	// Support simple values like "9", "0", "15"
	var v int
	if _, err := parseIntSafe(field, &v); err == nil {
		return v == value
	}

	// Support step syntax like "*/5"
	if len(field) > 2 && field[0] == '*' && field[1] == '/' {
		var step int
		if _, err := parseIntSafe(field[2:], &step); err == nil && step > 0 {
			return (value-min)%step == 0
		}
	}

	// Support comma-separated: "1,3,5"
	for _, part := range splitComma(field) {
		var pv int
		if _, err := parseIntSafe(part, &pv); err == nil && pv == value {
			return true
		}
	}

	return false
}

func splitComma(s string) []string {
	var result []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		result = append(result, cur)
	}
	return result
}

func parseIntSafe(s string, v *int) (bool, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, errInvalidInt
		}
		n = n*10 + int(c-'0')
	}
	*v = n
	return true, nil
}

func emptyScheduleEvent(collectionID string) events.Event {
	return events.Event{
		Type:         events.EventType(TriggerSchedule),
		CollectionID: collectionID,
	}
}

var errInvalidInt = &parseError{"invalid integer"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
