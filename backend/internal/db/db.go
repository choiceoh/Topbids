package db

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/choiceoh/phaeton/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pool from the provided config.
// In production, sslmode is forced to "require" unless already set to a
// stricter mode.
func NewPool(ctx context.Context, dbCfg config.DBConfig, isProd bool) (*pgxpool.Pool, error) {
	dsn := dbCfg.URL
	if dsn == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Enforce SSL in production.
	if isProd {
		dsn = enforceSSL(dsn)
	}

	return NewPoolFromDSN(ctx, dsn, dbCfg.MaxConns, dbCfg.MinConns, dbCfg.StatementTimeoutMS)
}

// NewPoolFromDSN creates a pool from an explicit DSN with the given pool
// settings. Useful for tests that need to point at an isolated database.
func NewPoolFromDSN(ctx context.Context, dsn string, maxConns, minConns, stmtTimeoutMS int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	cfg.MaxConns = int32(maxConns)
	cfg.MinConns = int32(minConns)

	// statement_timeout: auto-kill queries exceeding this duration (ms).
	// Prevents runaway queries from holding connections.
	if stmtTimeoutMS > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.Itoa(stmtTimeoutMS)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return pool, nil
}

// enforceSSL upgrades sslmode=disable to sslmode=require.
// If already set to require/verify-ca/verify-full, leaves it alone.
func enforceSSL(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	mode := q.Get("sslmode")
	if mode == "" || mode == "disable" || mode == "prefer" {
		q.Set("sslmode", "require")
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn
}

