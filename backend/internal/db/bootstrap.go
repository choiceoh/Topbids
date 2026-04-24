package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Bootstrap creates the PostgreSQL schemas (_meta, _history, data, auth)
// and their tables if they don't already exist.
// All operations run inside a single transaction for atomicity.
func Bootstrap(ctx context.Context, pool *pgxpool.Pool) error {
	// Acquire an advisory lock so parallel test processes serialise bootstrap.
	// The lock is session-level and released when the connection is returned.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn for advisory lock: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(42)` /* 42 = bootstrap */); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock(42)`) //nolint:errcheck

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bootstrap tx: %w", err)
	}
	defer tx.Rollback(ctx)

	stmts := []string{
		// --- schemas ---
		`CREATE SCHEMA IF NOT EXISTS _meta`,
		`CREATE SCHEMA IF NOT EXISTS _history`,
		`CREATE SCHEMA IF NOT EXISTS data`,
		`CREATE SCHEMA IF NOT EXISTS auth`,

		// Required for gen_random_uuid().
		`CREATE EXTENSION IF NOT EXISTS pgcrypto`,

		// --- auth.users ---
		// Not part of the schema engine metadata — this is an auth primitive
		// managed directly by the handler/auth.go code, not through collections.
		`CREATE TABLE IF NOT EXISTS auth.users (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email       VARCHAR(255) NOT NULL,
			name        VARCHAR(255) NOT NULL,
			password    TEXT NOT NULL,
			role        VARCHAR(31) NOT NULL DEFAULT 'viewer',
			is_active   BOOLEAN NOT NULL DEFAULT true,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email_lower ON auth.users (LOWER(email))`,
		`CREATE INDEX IF NOT EXISTS idx_auth_users_role ON auth.users(role)`,

		// --- auth.subsidiaries ---
		`CREATE TABLE IF NOT EXISTS auth.subsidiaries (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			external_code VARCHAR(63),
			name          VARCHAR(255) NOT NULL,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			is_active     BOOLEAN NOT NULL DEFAULT true,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_subsidiaries_external_code
		     ON auth.subsidiaries(external_code) WHERE external_code IS NOT NULL`,

		// --- auth.departments ---
		`CREATE TABLE IF NOT EXISTS auth.departments (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			external_code VARCHAR(63),
			name          VARCHAR(255) NOT NULL,
			parent_id     UUID REFERENCES auth.departments(id) ON DELETE SET NULL,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_departments_external_code
		     ON auth.departments(external_code) WHERE external_code IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_auth_departments_parent ON auth.departments(parent_id)`,

		// Upgrade: extend auth.departments with subsidiary reference.
		`ALTER TABLE auth.departments ADD COLUMN IF NOT EXISTS subsidiary_id UUID REFERENCES auth.subsidiaries(id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_departments_subsidiary ON auth.departments(subsidiary_id)`,

		// Upgrade: extend auth.users with profile columns.
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS external_id    VARCHAR(255)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS department_id  UUID REFERENCES auth.departments(id)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS subsidiary_id  UUID REFERENCES auth.subsidiaries(id)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS position       VARCHAR(127)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS title          VARCHAR(127)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS phone          VARCHAR(31)`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS avatar         TEXT`,
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS joined_at      DATE`,
		// Topbids: link supplier-role users to a row in data.suppliers.
		// Plain UUID (no FK) because data.suppliers is a dynamic collection
		// whose table is created by the schema engine after bootstrap runs.
		`ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS supplier_id    UUID`,
		`CREATE INDEX IF NOT EXISTS idx_auth_users_supplier ON auth.users(supplier_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_external_id
		     ON auth.users(external_id) WHERE external_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_auth_users_department ON auth.users(department_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_users_subsidiary ON auth.users(subsidiary_id)`,

		// --- _meta.collections ---
		`CREATE TABLE IF NOT EXISTS _meta.collections (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			slug        VARCHAR(63) UNIQUE NOT NULL,
			label       VARCHAR(255) NOT NULL,
			description TEXT,
			icon        VARCHAR(63),
			is_system   BOOLEAN NOT NULL DEFAULT FALSE,
			sort_order  INTEGER NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			created_by  UUID
		)`,

		// Upgrade: add access_config column for per-collection permissions.
		`ALTER TABLE _meta.collections ADD COLUMN IF NOT EXISTS access_config JSONB DEFAULT '{}'`,

		// --- _meta.fields ---
		`CREATE TABLE IF NOT EXISTS _meta.fields (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			slug            VARCHAR(63) NOT NULL,
			label           VARCHAR(255) NOT NULL,
			field_type      VARCHAR(31) NOT NULL,
			is_required     BOOLEAN NOT NULL DEFAULT FALSE,
			is_unique       BOOLEAN NOT NULL DEFAULT FALSE,
			is_indexed      BOOLEAN NOT NULL DEFAULT FALSE,
			default_value   JSONB,
			options         JSONB,
			width           SMALLINT NOT NULL DEFAULT 6,
			height          SMALLINT NOT NULL DEFAULT 1,
			sort_order      INTEGER NOT NULL DEFAULT 0,
			is_layout       BOOLEAN NOT NULL DEFAULT FALSE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(collection_id, slug)
		)`,

		// Upgrade: add is_layout column for existing installations.
		`ALTER TABLE _meta.fields ADD COLUMN IF NOT EXISTS is_layout BOOLEAN NOT NULL DEFAULT FALSE`,

		// Performance: index on fields.collection_id (used in every schema lookup).
		`CREATE INDEX IF NOT EXISTS idx_meta_fields_collection
		     ON _meta.fields(collection_id)`,

		// --- _meta.relations ---
		`CREATE TABLE IF NOT EXISTS _meta.relations (
			id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			field_id             UUID UNIQUE NOT NULL REFERENCES _meta.fields(id) ON DELETE CASCADE,
			target_collection_id UUID NOT NULL REFERENCES _meta.collections(id),
			relation_type        VARCHAR(15) NOT NULL,
			junction_table       VARCHAR(63),
			on_delete            VARCHAR(15) NOT NULL DEFAULT 'SET NULL'
		)`,

		// --- _meta.processes ---
		`CREATE TABLE IF NOT EXISTS _meta.processes (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID UNIQUE NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			is_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		// --- _meta.process_statuses ---
		`CREATE TABLE IF NOT EXISTS _meta.process_statuses (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			process_id  UUID NOT NULL REFERENCES _meta.processes(id) ON DELETE CASCADE,
			name        VARCHAR(255) NOT NULL,
			color       VARCHAR(31) NOT NULL DEFAULT '#6b7280',
			sort_order  INTEGER NOT NULL DEFAULT 0,
			is_initial  BOOLEAN NOT NULL DEFAULT FALSE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		// --- _meta.process_transitions ---
		`CREATE TABLE IF NOT EXISTS _meta.process_transitions (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			process_id      UUID NOT NULL REFERENCES _meta.processes(id) ON DELETE CASCADE,
			from_status_id  UUID NOT NULL REFERENCES _meta.process_statuses(id) ON DELETE CASCADE,
			to_status_id    UUID NOT NULL REFERENCES _meta.process_statuses(id) ON DELETE CASCADE,
			label           VARCHAR(255) NOT NULL,
			allowed_roles   TEXT[] NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		// --- _meta.views ---
		`CREATE TABLE IF NOT EXISTS _meta.views (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			name            VARCHAR(255) NOT NULL,
			view_type       VARCHAR(31) NOT NULL DEFAULT 'list',
			config          JSONB DEFAULT '{}',
			sort_order      INTEGER NOT NULL DEFAULT 0,
			is_default      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		// --- _history.schema_migrations ---
		`CREATE TABLE IF NOT EXISTS _history.schema_migrations (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL,
			operation       VARCHAR(31) NOT NULL,
			payload         JSONB NOT NULL,
			ddl_up          TEXT NOT NULL,
			ddl_down        TEXT NOT NULL,
			safety_level    VARCHAR(15) NOT NULL DEFAULT 'SAFE',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			applied_at      TIMESTAMPTZ,
			applied_by      UUID,
			rolled_back_at  TIMESTAMPTZ
		)`,
	}

	// --- comments ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.comments (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL,
			record_id       UUID NOT NULL,
			user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
			user_name       VARCHAR(255) NOT NULL,
			body            TEXT NOT NULL,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_comments_lookup ON _meta.comments(collection_id, record_id)`,
	)

	// --- notifications ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.notifications (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
			type                VARCHAR(31) NOT NULL,
			title               VARCHAR(255) NOT NULL,
			body                TEXT,
			ref_collection_id   UUID,
			ref_record_id       UUID,
			is_read             BOOLEAN NOT NULL DEFAULT FALSE,
			created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_user ON _meta.notifications(user_id, is_read) WHERE is_read = FALSE`,
	)

	// --- collection members (app-level access control) ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.collection_members (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
			role            VARCHAR(15) NOT NULL DEFAULT 'viewer',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(collection_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_collection_members_user ON _meta.collection_members(user_id)`,
	)

	// --- record change history ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _history.record_changes (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL,
			record_id       UUID NOT NULL,
			user_id         UUID,
			user_name       VARCHAR(255),
			operation       VARCHAR(15) NOT NULL,
			diff            JSONB NOT NULL DEFAULT '{}',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_record_changes_lookup ON _history.record_changes(collection_id, record_id)`,
		`CREATE INDEX IF NOT EXISTS idx_record_changes_time ON _history.record_changes(created_at DESC)`,
	)

	// --- saved views ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.saved_views (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			name            VARCHAR(255) NOT NULL,
			filter_config   JSONB NOT NULL DEFAULT '{}',
			sort_config     VARCHAR(255) NOT NULL DEFAULT '',
			visible_fields  JSONB,
			is_default      BOOLEAN NOT NULL DEFAULT FALSE,
			is_public       BOOLEAN NOT NULL DEFAULT TRUE,
			created_by      UUID REFERENCES auth.users(id) ON DELETE SET NULL,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_saved_views_collection ON _meta.saved_views(collection_id)`,
	)

	// --- automations ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.automations (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			name            VARCHAR(255) NOT NULL,
			is_enabled      BOOLEAN NOT NULL DEFAULT TRUE,
			trigger_type    VARCHAR(31) NOT NULL,
			trigger_config  JSONB NOT NULL DEFAULT '{}',
			created_by      UUID,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automations_collection ON _meta.automations(collection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_automations_trigger ON _meta.automations(collection_id, trigger_type) WHERE is_enabled = TRUE`,

		`CREATE TABLE IF NOT EXISTS _meta.automation_conditions (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			automation_id   UUID NOT NULL REFERENCES _meta.automations(id) ON DELETE CASCADE,
			field_slug      VARCHAR(63) NOT NULL,
			operator        VARCHAR(31) NOT NULL,
			value           TEXT,
			sort_order      INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_conditions_auto ON _meta.automation_conditions(automation_id)`,

		`CREATE TABLE IF NOT EXISTS _meta.automation_actions (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			automation_id   UUID NOT NULL REFERENCES _meta.automations(id) ON DELETE CASCADE,
			action_type     VARCHAR(31) NOT NULL,
			action_config   JSONB NOT NULL DEFAULT '{}',
			sort_order      INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_actions_auto ON _meta.automation_actions(automation_id)`,

		`CREATE TABLE IF NOT EXISTS _history.automation_runs (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			automation_id   UUID NOT NULL,
			collection_id   UUID NOT NULL,
			record_id       UUID NOT NULL,
			trigger_type    VARCHAR(31) NOT NULL,
			status          VARCHAR(15) NOT NULL DEFAULT 'success',
			error_message   TEXT,
			duration_ms     INTEGER,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_runs_auto ON _history.automation_runs(automation_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_runs_collection ON _history.automation_runs(collection_id, created_at DESC)`,
	)

	// --- charts ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.charts (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			collection_id   UUID NOT NULL REFERENCES _meta.collections(id) ON DELETE CASCADE,
			name            VARCHAR(255) NOT NULL,
			chart_type      VARCHAR(31) NOT NULL DEFAULT 'bar',
			config          JSONB NOT NULL DEFAULT '{}',
			sort_order      INTEGER NOT NULL DEFAULT 0,
			created_by      UUID REFERENCES auth.users(id) ON DELETE SET NULL,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_charts_collection ON _meta.charts(collection_id)`,
	)

	// --- webhook events ---
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.webhook_events (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			topic       VARCHAR(255) NOT NULL,
			source      VARCHAR(255) NOT NULL DEFAULT '',
			payload     JSONB NOT NULL DEFAULT '{}',
			processed   BOOLEAN NOT NULL DEFAULT FALSE,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_events_topic_time ON _meta.webhook_events(topic, received_at DESC)`,
	)

	// --- Topbids bid audit log (append-only compliance trail) ---
	// Record sealed-bid read attempts, submissions, scheduler opens, awards.
	// No UPDATE or DELETE grants at the application layer — callers only INSERT.
	// actor_id is nullable so background jobs (scheduler) can record without a user.
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS _meta.bid_audit_log (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			actor_id    UUID,
			actor_name  VARCHAR(255) NOT NULL DEFAULT '',
			action      VARCHAR(31) NOT NULL,
			app_slug    VARCHAR(63) NOT NULL,
			row_id      UUID,
			ip          INET,
			detail      JSONB NOT NULL DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bid_audit_log_time
		     ON _meta.bid_audit_log(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_bid_audit_log_app_row
		     ON _meta.bid_audit_log(app_slug, row_id) WHERE row_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_bid_audit_log_actor
		     ON _meta.bid_audit_log(actor_id, created_at DESC) WHERE actor_id IS NOT NULL`,
	)

	// --- incremental schema evolution (safe for existing deployments) ---
	alters := []string{
		`ALTER TABLE _meta.collections ADD COLUMN IF NOT EXISTS process_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE _meta.process_transitions ADD COLUMN IF NOT EXISTS allowed_roles TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE _meta.fields ADD COLUMN IF NOT EXISTS width SMALLINT NOT NULL DEFAULT 6`,
		`ALTER TABLE _meta.fields ADD COLUMN IF NOT EXISTS height SMALLINT NOT NULL DEFAULT 1`,
		`ALTER TABLE _meta.process_transitions ADD COLUMN IF NOT EXISTS allowed_user_ids UUID[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE _meta.webhook_events ADD COLUMN IF NOT EXISTS error_message TEXT`,
		`ALTER TABLE _meta.collections ADD COLUMN IF NOT EXISTS title_field_id UUID REFERENCES _meta.fields(id) ON DELETE SET NULL`,
		`ALTER TABLE _meta.collections ADD COLUMN IF NOT EXISTS default_sort_field VARCHAR(63)`,
		`ALTER TABLE _meta.collections ADD COLUMN IF NOT EXISTS default_sort_order VARCHAR(4)`,
	}

	for _, stmt := range append(stmts, alters...) {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("bootstrap exec: %w", err)
		}
	}

	// Add _version column to all existing dynamic data tables.
	rows, err := tx.Query(ctx, `SELECT slug FROM _meta.collections`)
	if err != nil {
		return fmt.Errorf("bootstrap: list collections: %w", err)
	}
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return fmt.Errorf("bootstrap: scan slug: %w", err)
		}
		slugs = append(slugs, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("bootstrap: rows iteration: %w", err)
	}
	for _, slug := range slugs {
		// Check if the data table actually exists before altering it.
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'data' AND table_name = $1)`,
			slug,
		).Scan(&exists); err != nil {
			return fmt.Errorf("bootstrap: check table %s: %w", slug, err)
		}
		if !exists {
			continue
		}
		stmt := fmt.Sprintf(`ALTER TABLE "data".%q ADD COLUMN IF NOT EXISTS _version INTEGER NOT NULL DEFAULT 1`, slug)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("bootstrap: add _version to %s: %w", slug, err)
		}
	}

	return tx.Commit(ctx)
}
