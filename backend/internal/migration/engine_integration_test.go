package migration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
	"github.com/choiceoh/phaeton/backend/internal/testutil"
)

func TestEngine_CreateAndDropCollection(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	// Create collection with fields.
	col, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug:  "tasks",
		Label: "Tasks",
		Fields: []schema.CreateFieldIn{
			{Slug: "title", Label: "Title", FieldType: schema.FieldText, IsRequired: true},
			{Slug: "count", Label: "Count", FieldType: schema.FieldInteger},
			{Slug: "done", Label: "Done", FieldType: schema.FieldBoolean},
		},
	})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	if col.Slug != "tasks" {
		t.Errorf("slug = %q, want %q", col.Slug, "tasks")
	}
	if len(col.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(col.Fields))
	}

	// Verify the data table exists by inserting a row.
	_, err = pool.Exec(ctx, `INSERT INTO data.tasks (title, count, done) VALUES ('Test', 1, true)`)
	if err != nil {
		t.Fatalf("insert into data.tasks: %v", err)
	}

	// Verify cache is updated.
	cached, ok := cache.CollectionBySlug("tasks")
	if !ok {
		t.Fatal("expected tasks in cache")
	}
	if len(cached.Fields) != 3 {
		t.Errorf("cache fields = %d, want 3", len(cached.Fields))
	}

	// Drop collection.
	if err := engine.DropCollection(ctx, col.ID); err != nil {
		t.Fatalf("DropCollection: %v", err)
	}

	// Verify table is gone.
	_, err = pool.Exec(ctx, `SELECT 1 FROM data.tasks LIMIT 1`)
	if err == nil {
		t.Error("expected error querying dropped table")
	}

	// Verify cache is updated.
	_, ok = cache.CollectionByID(col.ID)
	if ok {
		t.Error("expected collection removed from cache")
	}
}

func TestEngine_AddAndDropField(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	// Create base collection.
	col, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug:  "items",
		Label: "Items",
		Fields: []schema.CreateFieldIn{
			{Slug: "name", Label: "Name", FieldType: schema.FieldText},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add a new field.
	field, _, err := engine.AddField(ctx, col.ID, &schema.CreateFieldIn{
		Slug:      "price",
		Label:     "Price",
		FieldType: schema.FieldNumber,
	}, true)
	if err != nil {
		t.Fatalf("AddField: %v", err)
	}
	if field.Slug != "price" {
		t.Errorf("slug = %q, want %q", field.Slug, "price")
	}

	// Verify column exists.
	_, err = pool.Exec(ctx, `INSERT INTO data.items (name, price) VALUES ('Widget', 9.99)`)
	if err != nil {
		t.Fatalf("insert with new column: %v", err)
	}

	// Verify cache is updated.
	fields := cache.Fields(col.ID)
	if len(fields) != 2 {
		t.Errorf("cache fields = %d, want 2", len(fields))
	}

	// Drop the field.
	if err := engine.DropField(ctx, field.ID); err != nil {
		t.Fatalf("DropField: %v", err)
	}

	// Verify column is gone.
	_, err = pool.Exec(ctx, `INSERT INTO data.items (name, price) VALUES ('Fail', 1.0)`)
	if err == nil {
		t.Error("expected error inserting into dropped column")
	}

	// Verify cache is updated.
	fields = cache.Fields(col.ID)
	if len(fields) != 1 {
		t.Errorf("cache fields after drop = %d, want 1", len(fields))
	}
}

func TestEngine_CreateDuplicateSlug(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	_, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "dups", Label: "Dups",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "dups", Label: "Dups Again",
	})
	if err == nil {
		t.Fatal("expected error for duplicate slug")
	}
}

func TestEngine_UpdateCollection(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	col, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "docs", Label: "Documents",
	})
	if err != nil {
		t.Fatal(err)
	}

	newLabel := "Updated Docs"
	updated, err := engine.UpdateCollection(ctx, col.ID, &schema.UpdateCollectionReq{
		Label: &newLabel,
	})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if updated.Label != "Updated Docs" {
		t.Errorf("label = %q, want %q", updated.Label, "Updated Docs")
	}
}

func TestEngine_LayoutAndComputedFields(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	col, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "forms", Label: "Forms",
		Fields: []schema.CreateFieldIn{
			{Slug: "amount", Label: "Amount", FieldType: schema.FieldNumber},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Layout field (no DB column).
	_, _, err = engine.AddField(ctx, col.ID, &schema.CreateFieldIn{
		Slug: "sep", Label: "Separator", FieldType: schema.FieldLine,
	}, true)
	if err != nil {
		t.Fatalf("AddField layout: %v", err)
	}

	// Formula field (no DB column).
	_, _, err = engine.AddField(ctx, col.ID, &schema.CreateFieldIn{
		Slug:      "doubled",
		Label:     "Doubled",
		FieldType: schema.FieldFormula,
		Options:   mustJSON(map[string]any{"expression": "amount * 2", "result_type": "number"}),
	}, true)
	if err != nil {
		t.Fatalf("AddField formula: %v", err)
	}

	// Verify no extra columns were added to the table.
	var colCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'data' AND table_name = 'forms'
	`).Scan(&colCount)
	if err != nil {
		t.Fatal(err)
	}
	// Expected: id, amount, created_at, updated_at, created_by, updated_by, deleted_at, _version = 8
	if colCount != 8 {
		t.Errorf("column count = %d, want 8 (no layout/formula columns)", colCount)
	}

	// But cache should have all 3 fields.
	fields := cache.Fields(col.ID)
	if len(fields) != 3 {
		t.Errorf("cache fields = %d, want 3", len(fields))
	}
}

func TestEngine_RelationField(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	if err := cache.Load(ctx); err != nil {
		t.Fatal(err)
	}

	engine := migration.NewEngine(pool, store, cache)

	// Create target collection.
	target, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "categories", Label: "Categories",
		Fields: []schema.CreateFieldIn{
			{Slug: "name", Label: "Name", FieldType: schema.FieldText},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create collection with relation.
	col, err := engine.CreateCollection(ctx, &schema.CreateCollectionReq{
		Slug: "products", Label: "Products",
		Fields: []schema.CreateFieldIn{
			{Slug: "title", Label: "Title", FieldType: schema.FieldText},
			{
				Slug: "category", Label: "Category", FieldType: schema.FieldRelation,
				Relation: &schema.CreateRelIn{
					TargetCollectionID: target.ID,
					RelationType:       schema.RelOneToMany,
					OnDelete:           "SET NULL",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateCollection with relation: %v", err)
	}

	// Verify relation field.
	fields := cache.Fields(col.ID)
	var relField *schema.Field
	for i := range fields {
		if fields[i].Slug == "category" {
			relField = &fields[i]
			break
		}
	}
	if relField == nil {
		t.Fatal("relation field not found")
	}
	if relField.Relation == nil {
		t.Fatal("relation config is nil")
	}
	if relField.Relation.TargetCollectionID != target.ID {
		t.Errorf("target = %s, want %s", relField.Relation.TargetCollectionID, target.ID)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
