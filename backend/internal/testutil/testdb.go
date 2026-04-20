// Package testutil provides helpers for integration tests that require PostgreSQL.
//
// Two modes are available:
//
//	SetupDB(t)     — connects to phaeton_test, bootstraps schemas, truncates between tests.
//	                  Requires a local phaeton_test database.
//	NewTestPool(t) — connects to TEST_DATABASE_URL, creates an isolated schema per test.
//	                  Skipped if TEST_DATABASE_URL is not set.
package testutil

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/db"
)

const defaultTestDSN = "postgres://phaeton:phaeton@localhost:5432/phaeton_test?sslmode=disable"

// bootstrapOnce ensures bootstrap runs only once per test process.
var bootstrapOnce sync.Once
var bootstrapErr error

// mu serialises test setup/teardown to prevent deadlocks when multiple
// test functions run in parallel against the same database.
var mu sync.Mutex

// SetupDB returns a pool connected to the test database.
// It bootstraps the schema (once) and truncates all tables before each test.
// The test is skipped if the database is unreachable.
func SetupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPoolFromDSN(ctx, dsn, 10, 2, 30000)
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to test DB: %v", err)
	}

	// Bootstrap schemas and tables (once per process).
	bootstrapOnce.Do(func() {
		bootstrapErr = db.Bootstrap(ctx, pool)
	})
	if bootstrapErr != nil {
		pool.Close()
		t.Fatalf("bootstrap test DB: %v", bootstrapErr)
	}

	// Serialise truncation to prevent deadlocks.
	mu.Lock()
	truncate(t, pool)
	mu.Unlock()

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// NewTestPool returns a pgxpool connected to the test database.
// It creates a unique schema for isolation and drops it on cleanup.
// The test is skipped if TEST_DATABASE_URL is not set.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Create an isolated schema for this test.
	schemaName := fmt.Sprintf("test_%s_%d", sanitize(t.Name()), os.Getpid())
	if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %q", schemaName)); err != nil {
		pool.Close()
		t.Fatalf("failed to create test schema: %v", err)
	}

	// Set search_path so unqualified queries use the test schema.
	if _, err := pool.Exec(ctx, fmt.Sprintf("SET search_path TO %q, public", schemaName)); err != nil {
		pool.Close()
		t.Fatalf("failed to set search_path: %v", err)
	}

	t.Cleanup(func() {
		// Drop the test schema.
		_, _ = pool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %q CASCADE", schemaName))
		pool.Close()
	})

	return pool
}

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	// Drop all dynamic tables and sequences in the data schema first
	// (single statement avoids deadlocks).
	var dataTables []string
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'data'`)
	if err != nil {
		t.Fatalf("list data tables: %v", err)
	}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		dataTables = append(dataTables, "data."+name)
	}
	rows.Close()
	if len(dataTables) > 0 {
		drop := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", joinComma(dataTables))
		if _, err := pool.Exec(ctx, drop); err != nil {
			t.Fatalf("drop data tables: %v", err)
		}
	}

	var seqs []string
	srows, err := pool.Query(ctx, `SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema = 'data'`)
	if err != nil {
		t.Fatalf("list data sequences: %v", err)
	}
	for srows.Next() {
		var name string
		srows.Scan(&name)
		seqs = append(seqs, "data."+name)
	}
	srows.Close()
	if len(seqs) > 0 {
		drop := fmt.Sprintf("DROP SEQUENCE IF EXISTS %s", joinComma(seqs))
		pool.Exec(ctx, drop)
	}

	// Truncate meta/history/auth tables in one TRUNCATE CASCADE statement.
	metaTables := []string{
		"_history.automation_runs",
		"_history.record_changes",
		"_history.schema_migrations",
		"_meta.automation_actions",
		"_meta.automation_conditions",
		"_meta.automations",
		"_meta.saved_views",
		"_meta.collection_members",
		"_meta.notifications",
		"_meta.comments",
		"_meta.process_transitions",
		"_meta.process_statuses",
		"_meta.processes",
		"_meta.views",
		"_meta.relations",
		"_meta.fields",
		"_meta.collections",
		"auth.users",
		"auth.departments",
	}
	trunc := fmt.Sprintf("TRUNCATE %s CASCADE", joinComma(metaTables))
	if _, err := pool.Exec(ctx, trunc); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}

// sanitize removes characters that are not safe for a schema name.
func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		}
	}
	if len(out) > 50 {
		out = out[:50]
	}
	return string(out)
}
