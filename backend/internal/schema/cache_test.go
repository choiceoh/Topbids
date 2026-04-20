package schema

import (
	"testing"
)

// newTestCache creates a cache with pre-populated data (no DB required).
func newTestCache(cols ...Collection) *Cache {
	c := &Cache{
		byID:      make(map[string]Collection),
		bySlug:    make(map[string]Collection),
		processes: make(map[string]Process),
	}
	for _, col := range cols {
		c.byID[col.ID] = col
		c.bySlug[col.Slug] = col
	}
	return c
}

func TestCache_CollectionByID(t *testing.T) {
	c := newTestCache(
		Collection{ID: "id-1", Slug: "tasks", Label: "Tasks"},
		Collection{ID: "id-2", Slug: "issues", Label: "Issues"},
	)

	col, ok := c.CollectionByID("id-1")
	if !ok || col.Slug != "tasks" {
		t.Errorf("expected tasks, got %v", col)
	}

	_, ok = c.CollectionByID("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent id")
	}
}

func TestCache_CollectionBySlug(t *testing.T) {
	c := newTestCache(
		Collection{ID: "id-1", Slug: "tasks", Label: "Tasks"},
	)

	col, ok := c.CollectionBySlug("tasks")
	if !ok || col.ID != "id-1" {
		t.Errorf("expected id-1, got %v", col)
	}

	_, ok = c.CollectionBySlug("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent slug")
	}
}

func TestCache_Collections_Sorted(t *testing.T) {
	c := newTestCache(
		Collection{ID: "1", Slug: "c", Label: "C", SortOrder: 2},
		Collection{ID: "2", Slug: "a", Label: "A", SortOrder: 1},
		Collection{ID: "3", Slug: "b", Label: "B", SortOrder: 1},
	)

	cols := c.Collections()
	if len(cols) != 3 {
		t.Fatalf("expected 3 collections, got %d", len(cols))
	}
	// SortOrder=1 first (A, B), then SortOrder=2 (C).
	if cols[0].Label != "A" {
		t.Errorf("first should be A, got %s", cols[0].Label)
	}
	if cols[1].Label != "B" {
		t.Errorf("second should be B, got %s", cols[1].Label)
	}
	if cols[2].Label != "C" {
		t.Errorf("third should be C, got %s", cols[2].Label)
	}
}

func TestCache_Collections_ReturnsCopy(t *testing.T) {
	c := newTestCache(Collection{ID: "1", Slug: "a", Label: "A"})

	cols := c.Collections()
	cols[0].Label = "MUTATED"

	col, _ := c.CollectionByID("1")
	if col.Label != "A" {
		t.Error("mutating returned slice should not affect cache")
	}
}

func TestCache_RemoveCollection(t *testing.T) {
	c := newTestCache(
		Collection{ID: "id-1", Slug: "tasks", Label: "Tasks"},
	)

	c.RemoveCollection("id-1")

	_, ok := c.CollectionByID("id-1")
	if ok {
		t.Error("collection should be removed by ID")
	}
	_, ok = c.CollectionBySlug("tasks")
	if ok {
		t.Error("collection should be removed by slug")
	}
}

func TestCache_RemoveCollection_Nonexistent(t *testing.T) {
	c := newTestCache()
	// Should not panic.
	c.RemoveCollection("does-not-exist")
}

func TestCache_Fields(t *testing.T) {
	c := newTestCache(Collection{
		ID:   "id-1",
		Slug: "tasks",
		Fields: []Field{
			{ID: "f1", Slug: "title", FieldType: FieldText},
			{ID: "f2", Slug: "count", FieldType: FieldInteger},
		},
	})

	fields := c.Fields("id-1")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Slug != "title" {
		t.Errorf("expected title, got %s", fields[0].Slug)
	}

	// Mutating should not affect cache.
	fields[0].Slug = "MUTATED"
	cached := c.Fields("id-1")
	if cached[0].Slug != "title" {
		t.Error("mutating returned fields should not affect cache")
	}
}

func TestCache_Fields_NotFound(t *testing.T) {
	c := newTestCache()
	fields := c.Fields("nonexistent")
	if fields != nil {
		t.Errorf("expected nil for nonexistent collection, got %v", fields)
	}
}

func TestCache_Process(t *testing.T) {
	c := newTestCache()
	c.processes["col-1"] = Process{ID: "proc-1", CollectionID: "col-1"}

	p, ok := c.ProcessByCollectionID("col-1")
	if !ok || p.ID != "proc-1" {
		t.Errorf("expected proc-1, got %v", p)
	}

	_, ok = c.ProcessByCollectionID("col-2")
	if ok {
		t.Error("expected not found for nonexistent process")
	}
}

func TestCache_RemoveProcess(t *testing.T) {
	c := newTestCache()
	c.processes["col-1"] = Process{ID: "proc-1"}

	c.RemoveProcess("col-1")

	_, ok := c.ProcessByCollectionID("col-1")
	if ok {
		t.Error("process should be removed")
	}
}
