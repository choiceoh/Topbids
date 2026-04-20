package seed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// SeedData inserts sample records into the bid domain collections.
// Idempotent: skips if records already exist.
//
// The Topbids refactor dropped the Phaeton no-code sample views, automations,
// and charts. Only minimal domain-demonstration rows remain.
func SeedData(ctx context.Context, pool *pgxpool.Pool, store *schema.Store, cache *schema.Cache) error {
	// Get admin user ID (best-effort — seed data is optional).
	var userID string
	err := pool.QueryRow(ctx, `SELECT id FROM auth.users WHERE email = 'choiceoh@topsolar.kr'`).Scan(&userID)
	if err != nil {
		slog.Warn("seed: admin user not found, skipping sample data", "error", err)
		return nil
	}

	// Sample suppliers for demo / dev.
	if _, ok := cache.CollectionBySlug("suppliers"); ok {
		_, err := seedRecords(ctx, pool, "suppliers", userID, []map[string]any{
			{
				"name":       "(주)대한자재",
				"biz_no":     "123-45-67890",
				"ceo":        "홍길동",
				"email":      "sales@daehan.example",
				"phone":      "02-1234-5678",
				"categories": []string{"자재"},
				"status":     "활성",
			},
			{
				"name":       "동산엔지니어링",
				"biz_no":     "234-56-78901",
				"ceo":        "김철수",
				"email":      "info@dongsan.example",
				"phone":      "031-2345-6789",
				"categories": []string{"시공", "용역"},
				"status":     "활성",
			},
			{
				"name":       "남양글로벌",
				"biz_no":     "345-67-89012",
				"ceo":        "이영희",
				"email":      "contact@namyang.example",
				"phone":      "051-3456-7890",
				"categories": []string{"자재", "장비"},
				"status":     "활성",
			},
		})
		if err != nil {
			return fmt.Errorf("seed suppliers: %w", err)
		}
	}

	_ = store // retained for future seed helpers (views, etc.)
	return nil
}

// seedRecords inserts rows into the data table for `slug`. Idempotent:
// if any row already exists, returns existing IDs without inserting.
func seedRecords(ctx context.Context, pool *pgxpool.Pool, slug, userID string, rows []map[string]any) ([]string, error) {
	var count int
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "data".%q`, slug)).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count %s: %w", slug, err)
	}
	if count > 0 {
		// Return existing IDs for relation wiring.
		existingRows, err := pool.Query(ctx, fmt.Sprintf(`SELECT id FROM "data".%q ORDER BY created_at`, slug))
		if err != nil {
			return nil, err
		}
		defer existingRows.Close()
		var ids []string
		for existingRows.Next() {
			var id string
			if err := existingRows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		slog.Info("seed: data exists, skipping", "slug", slug, "count", count)
		return ids, existingRows.Err()
	}

	var ids []string
	for _, row := range rows {
		cols := `"created_by"`
		vals := "$1"
		args := []any{userID}
		idx := 2

		for k, v := range row {
			cols += fmt.Sprintf(", %q", k)
			vals += fmt.Sprintf(", $%d", idx)
			args = append(args, v)
			idx++
		}

		sql := fmt.Sprintf(`INSERT INTO "data".%q (%s) VALUES (%s) RETURNING id`, slug, cols, vals)
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			return nil, fmt.Errorf("insert %s: %w", slug, err)
		}
		ids = append(ids, id)
	}

	slog.Info("seed: inserted records", "slug", slug, "count", len(rows))
	return ids, nil
}
