package schema

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Cache holds an in-memory snapshot of _meta tables.
// It supports both full reload (on startup) and partial per-collection updates
// (during normal schema changes) to avoid re-reading the entire meta table
// on every single DDL.
//
// Internally, collections are stored by value in two maps (id → Collection, slug → Collection).
// This avoids the pointer-stability issue that would arise if we stored them as
// slice elements and resized the slice.
type Cache struct {
	mu        sync.RWMutex
	store     *Store
	byID      map[string]Collection
	bySlug    map[string]Collection
	processes map[string]Process // keyed by collection_id
}

func NewCache(store *Store) *Cache {
	return &Cache{
		store:     store,
		byID:      make(map[string]Collection),
		bySlug:    make(map[string]Collection),
		processes: make(map[string]Process),
	}
}

// Load reads the full meta state from PostgreSQL.
// Call this once on startup, or after an unknown change where partial updates
// are not tractable (e.g. rollback of an old migration).
func (c *Cache) Load(ctx context.Context) error {
	collections, err := c.store.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("cache load collections: %w", err)
	}

	byID := make(map[string]Collection, len(collections))
	bySlug := make(map[string]Collection, len(collections))

	for i := range collections {
		fields, err := c.store.ListFields(ctx, collections[i].ID)
		if err != nil {
			return fmt.Errorf("cache load fields for %s: %w", collections[i].Slug, err)
		}
		collections[i].Fields = fields
		byID[collections[i].ID] = collections[i]
		bySlug[collections[i].Slug] = collections[i]
	}

	// Load process configs for all collections.
	procs := make(map[string]Process, len(collections))
	for _, col := range collections {
		proc, err := c.store.GetProcess(ctx, col.ID)
		if err != nil {
			return fmt.Errorf("cache load process for %s: %w", col.Slug, err)
		}
		if proc.ID != "" {
			procs[col.ID] = proc
		}
	}

	c.mu.Lock()
	c.byID = byID
	c.bySlug = bySlug
	c.processes = procs
	c.mu.Unlock()
	return nil
}

// Invalidate is an alias for Load — kept for backward compatibility with call sites
// that want "refresh everything". Prefer ReloadCollection / RemoveCollection when
// you know the exact change.
func (c *Cache) Invalidate(ctx context.Context) error {
	return c.Load(ctx)
}

// ReloadCollection fetches a single collection (plus its fields and relations) from
// the database and updates both the byID and bySlug maps in the cache.
// If the collection's slug changed, the stale slug mapping is removed first.
// Used after CreateCollection, AddField, AlterField, DropField — any change that
// modifies exactly one collection.
func (c *Cache) ReloadCollection(ctx context.Context, id string) error {
	col, err := c.store.GetCollection(ctx, id)
	if err != nil {
		return fmt.Errorf("reload collection %s: %w", id, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the slug changed (future-proofing), drop the stale slug mapping first.
	if old, ok := c.byID[id]; ok && old.Slug != col.Slug {
		delete(c.bySlug, old.Slug)
	}
	c.byID[id] = col
	c.bySlug[col.Slug] = col
	return nil
}

// RemoveCollection deletes a single collection from both the byID and bySlug maps.
// Used after DropCollection. Safe to call with an ID that is not in the cache.
func (c *Cache) RemoveCollection(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	col, ok := c.byID[id]
	if !ok {
		return
	}
	delete(c.byID, id)
	delete(c.bySlug, col.Slug)
}

// Collections returns all collections sorted by sort_order then label.
// The returned slice is a fresh copy and safe for the caller to mutate.
func (c *Cache) Collections() []Collection {
	c.mu.RLock()
	out := make([]Collection, 0, len(c.byID))
	for _, col := range c.byID {
		out = append(out, col)
	}
	c.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// CollectionByID returns a collection by its UUID, along with a boolean indicating
// whether the collection was found. The returned Collection is a value copy —
// mutating it does not affect the cache.
func (c *Cache) CollectionByID(id string) (Collection, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	col, ok := c.byID[id]
	return col, ok
}

// CollectionBySlug returns a collection by its slug (e.g. "permit_checklist").
// The returned Collection is a value copy.
func (c *Cache) CollectionBySlug(slug string) (Collection, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	col, ok := c.bySlug[slug]
	return col, ok
}

// CollectionByFieldID returns the collection that owns the given field.
func (c *Cache) CollectionByFieldID(fieldID string) (Collection, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, col := range c.byID {
		for _, f := range col.Fields {
			if f.ID == fieldID {
				return col, true
			}
		}
	}
	return Collection{}, false
}

// Fields returns the field list for a collection (empty slice if not found).
func (c *Cache) Fields(collectionID string) []Field {
	c.mu.RLock()
	defer c.mu.RUnlock()
	col, ok := c.byID[collectionID]
	if !ok {
		return nil
	}
	out := make([]Field, len(col.Fields))
	copy(out, col.Fields)
	return out
}

// ProcessByCollectionID returns the process config (워크플로우 설정) for a collection.
// Returns false if the collection has no process configured or process is not enabled.
func (c *Cache) ProcessByCollectionID(collectionID string) (Process, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.processes[collectionID]
	return p, ok
}

// ReloadProcess fetches the process config for a single collection from the DB
// and updates the cache. If the collection has no process (empty ID), the entry
// is removed from the cache instead.
func (c *Cache) ReloadProcess(ctx context.Context, collectionID string) error {
	proc, err := c.store.GetProcess(ctx, collectionID)
	if err != nil {
		return fmt.Errorf("reload process %s: %w", collectionID, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if proc.ID != "" {
		c.processes[collectionID] = proc
	} else {
		delete(c.processes, collectionID)
	}
	return nil
}

// RemoveProcess deletes a process entry from the cache by collection ID.
func (c *Cache) RemoveProcess(collectionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.processes, collectionID)
}
