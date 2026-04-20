// Seed script — runs bootstrap, creates admin user, and installs preset collections.
// Run: go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/choiceoh/phaeton/backend/internal/config"
	"github.com/choiceoh/phaeton/backend/internal/db"
	"github.com/choiceoh/phaeton/backend/internal/handler"
	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
	"github.com/choiceoh/phaeton/backend/internal/seed"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config:", err)
	}

	pool, err := db.NewPool(ctx, cfg.DB, cfg.IsProd)
	if err != nil {
		log.Fatal("db:", err)
	}
	defer pool.Close()

	if err := db.Bootstrap(ctx, pool); err != nil {
		log.Fatal("bootstrap:", err)
	}

	if err := handler.SeedDirector(ctx, pool); err != nil {
		log.Fatal("seed director:", err)
	}
	fmt.Fprintln(os.Stderr, "✓ admin user (choiceoh@topsolar.kr)")

	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(ctx); err != nil {
		log.Fatal("cache load:", err)
	}
	engine := migration.NewEngine(pool, store, cache)

	if err := seed.Run(ctx, engine, cache); err != nil {
		log.Fatal("seed collections:", err)
	}
	fmt.Fprintln(os.Stderr, "✓ preset collections seeded")

	if err := seed.SeedData(ctx, pool, store, cache); err != nil {
		log.Fatal("seed data:", err)
	}
	fmt.Fprintln(os.Stderr, "✓ sample data seeded")
	fmt.Fprintln(os.Stderr, "seed complete")
}
