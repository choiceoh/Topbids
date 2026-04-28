package schema_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
	"github.com/choiceoh/topbid/backend/internal/testutil"
)

func TestStore_CollectionCRUD(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	ctx := context.Background()

	// Create
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	col, err := store.CreateCollectionTx(ctx, tx, &schema.CreateCollectionReq{
		Slug:  "tasks",
		Label: "Tasks",
	})
	if err != nil {
		t.Fatalf("CreateCollectionTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if col.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if col.Slug != "tasks" {
		t.Errorf("slug = %q, want %q", col.Slug, "tasks")
	}

	// List
	cols, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(cols))
	}

	// Get by ID
	got, err := store.GetCollection(ctx, col.ID)
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.Label != "Tasks" {
		t.Errorf("label = %q, want %q", got.Label, "Tasks")
	}

	// Get by slug
	got2, err := store.GetCollectionBySlug(ctx, "tasks")
	if err != nil {
		t.Fatalf("GetCollectionBySlug: %v", err)
	}
	if got2.ID != col.ID {
		t.Errorf("ID mismatch: %s != %s", got2.ID, col.ID)
	}

	// Update
	newLabel := "Updated Tasks"
	updated, err := store.UpdateCollection(ctx, col.ID, &schema.UpdateCollectionReq{
		Label: &newLabel,
	})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if updated.Label != "Updated Tasks" {
		t.Errorf("label = %q, want %q", updated.Label, "Updated Tasks")
	}

	// Delete
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCollectionTx(ctx, tx2, col.ID); err != nil {
		t.Fatalf("DeleteCollectionTx: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	cols, err = store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections after delete: %v", err)
	}
	if len(cols) != 0 {
		t.Errorf("expected 0 collections after delete, got %d", len(cols))
	}
}

func TestStore_FieldCRUD(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	ctx := context.Background()

	// Create collection first.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	col, err := store.CreateCollectionTx(ctx, tx, &schema.CreateCollectionReq{
		Slug:  "tickets",
		Label: "Tickets",
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Create field.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	field, err := store.CreateFieldTx(ctx, tx2, col.ID, &schema.CreateFieldIn{
		Slug:      "title",
		Label:     "Title",
		FieldType: schema.FieldText,
	})
	if err != nil {
		t.Fatalf("CreateFieldTx: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if field.ID == "" {
		t.Fatal("expected non-empty field ID")
	}
	if field.Slug != "title" {
		t.Errorf("slug = %q, want %q", field.Slug, "title")
	}
	if field.Width != 6 {
		t.Errorf("default width = %d, want 6", field.Width)
	}

	// List fields.
	fields, err := store.ListFields(ctx, col.ID)
	if err != nil {
		t.Fatalf("ListFields: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}

	// Get field.
	got, err := store.GetField(ctx, field.ID)
	if err != nil {
		t.Fatalf("GetField: %v", err)
	}
	if got.Label != "Title" {
		t.Errorf("label = %q, want %q", got.Label, "Title")
	}

	// Update field.
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newLabel := "Updated Title"
	if err := store.UpdateFieldTx(ctx, tx3, field.ID, &schema.UpdateFieldReq{
		Label: &newLabel,
	}); err != nil {
		t.Fatalf("UpdateFieldTx: %v", err)
	}
	if err := tx3.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got2, err := store.GetField(ctx, field.ID)
	if err != nil {
		t.Fatalf("GetField after update: %v", err)
	}
	if got2.Label != "Updated Title" {
		t.Errorf("label = %q, want %q", got2.Label, "Updated Title")
	}

	// Delete field.
	tx4, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteFieldTx(ctx, tx4, field.ID); err != nil {
		t.Fatalf("DeleteFieldTx: %v", err)
	}
	if err := tx4.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	fields, err = store.ListFields(ctx, col.ID)
	if err != nil {
		t.Fatalf("ListFields after delete: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("expected 0 fields after delete, got %d", len(fields))
	}
}

func TestStore_FieldWithOptions(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	col, err := store.CreateCollectionTx(ctx, tx, &schema.CreateCollectionReq{
		Slug: "items", Label: "Items",
	})
	if err != nil {
		t.Fatal(err)
	}

	opts := json.RawMessage(`{"choices":["open","closed"]}`)
	field, err := store.CreateFieldTx(ctx, tx, col.ID, &schema.CreateFieldIn{
		Slug:      "status",
		Label:     "Status",
		FieldType: schema.FieldSelect,
		Options:   opts,
	})
	if err != nil {
		t.Fatalf("create select field: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetField(ctx, field.ID)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(got.Options, &parsed); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	choices, ok := parsed["choices"].([]any)
	if !ok || len(choices) != 2 {
		t.Errorf("expected 2 choices, got %v", parsed)
	}
}

func TestStore_CacheIntegration(t *testing.T) {
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	ctx := context.Background()

	// Load empty.
	if err := cache.Load(ctx); err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(cache.Collections()) != 0 {
		t.Fatalf("expected 0 collections, got %d", len(cache.Collections()))
	}

	// Create a collection and reload cache.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	col, err := store.CreateCollectionTx(ctx, tx, &schema.CreateCollectionReq{
		Slug: "projects", Label: "Projects",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.CreateFieldTx(ctx, tx, col.ID, &schema.CreateFieldIn{
		Slug: "name", Label: "Name", FieldType: schema.FieldText,
	})
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := cache.ReloadCollection(ctx, col.ID); err != nil {
		t.Fatalf("ReloadCollection: %v", err)
	}

	cached, ok := cache.CollectionBySlug("projects")
	if !ok {
		t.Fatal("expected to find projects in cache")
	}
	if len(cached.Fields) != 1 {
		t.Errorf("expected 1 field in cache, got %d", len(cached.Fields))
	}

	// Remove and verify.
	cache.RemoveCollection(col.ID)
	_, ok = cache.CollectionByID(col.ID)
	if ok {
		t.Error("expected collection removed from cache")
	}
}
