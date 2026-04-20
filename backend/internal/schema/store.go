package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides CRUD operations on the _meta schema tables.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool exposes the underlying pool for use by the migration engine.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---------- Collection ----------

const colCols = `id, slug, label, description, icon, is_system, process_enabled, sort_order, title_field_id, default_sort_field, default_sort_order, access_config, created_at, updated_at, created_by`

// scanCollection reads a single row matching the colCols column list into a Collection.
// It handles NULL-able optional columns (description, icon, created_by) via pointer intermediaries
// and unmarshals the access_config JSONB column into the AccessConfig struct.
func scanCollection(row pgx.Row) (Collection, error) {
	var (
		c                Collection
		id               pgtype.UUID
		createdBy        pgtype.UUID
		titleFieldID     pgtype.UUID
		desc             *string
		icon             *string
		defaultSortField *string
		defaultSortOrder *string
		acRaw            []byte
	)
	err := row.Scan(
		&id, &c.Slug, &c.Label, &desc, &icon,
		&c.IsSystem, &c.ProcessEnabled, &c.SortOrder,
		&titleFieldID, &defaultSortField, &defaultSortOrder,
		&acRaw, &c.CreatedAt, &c.UpdatedAt, &createdBy,
	)
	if err != nil {
		return Collection{}, err
	}
	c.ID = uuidStr(id)
	c.CreatedBy = uuidStr(createdBy)
	c.TitleFieldID = uuidStr(titleFieldID)
	if desc != nil {
		c.Description = *desc
	}
	if icon != nil {
		c.Icon = *icon
	}
	if defaultSortField != nil {
		c.DefaultSortField = *defaultSortField
	}
	if defaultSortOrder != nil {
		c.DefaultSortOrder = *defaultSortOrder
	}
	if len(acRaw) > 0 {
		if err := json.Unmarshal(acRaw, &c.AccessConfig); err != nil {
			return Collection{}, fmt.Errorf("unmarshal access_config: %w", err)
		}
	}
	return c, nil
}

// ListCollections returns all collections ordered by sort_order then label.
// Fields are NOT populated — call GetCollection for a single collection with fields.
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+colCols+` FROM _meta.collections ORDER BY sort_order, label`)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var out []Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCollection fetches a single collection by UUID, including its fields (with relations).
// Returns an error wrapping ErrNotFound if the UUID does not match any collection.
func (s *Store) GetCollection(ctx context.Context, id string) (Collection, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Collection{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	row := s.pool.QueryRow(ctx,
		`SELECT `+colCols+` FROM _meta.collections WHERE id = $1`, uid)
	c, err := scanCollection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("collection %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Collection{}, fmt.Errorf("get collection: %w", err)
	}

	fields, err := s.ListFields(ctx, c.ID)
	if err != nil {
		return Collection{}, err
	}
	c.Fields = fields
	return c, nil
}

func (s *Store) GetCollectionBySlug(ctx context.Context, slug string) (Collection, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+colCols+` FROM _meta.collections WHERE slug = $1`, slug)
	c, err := scanCollection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("collection %q: %w", slug, ErrNotFound)
	}
	if err != nil {
		return Collection{}, fmt.Errorf("get collection by slug: %w", err)
	}

	fields, err := s.ListFields(ctx, c.ID)
	if err != nil {
		return Collection{}, err
	}
	c.Fields = fields
	return c, nil
}

// CreateCollectionTx inserts a collection row inside an existing transaction.
// It takes a pgx.Tx (not the pool) because collection creation must be atomic
// with the corresponding CREATE TABLE DDL executed by the migration engine.
func (s *Store) CreateCollectionTx(ctx context.Context, tx pgx.Tx, req *CreateCollectionReq) (Collection, error) {
	var (
		id pgtype.UUID
		c  Collection
	)
	var acJSON []byte
	if req.AccessConfig != nil {
		var err error
		acJSON, err = json.Marshal(req.AccessConfig)
		if err != nil {
			return Collection{}, fmt.Errorf("marshal access_config: %w", err)
		}
	}
	var createdByUUID pgtype.UUID
	if req.CreatedBy != "" {
		createdByUUID, _ = parseUUID(req.CreatedBy)
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO _meta.collections (slug, label, description, icon, is_system, access_config, created_by)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'), $7)
		RETURNING id, created_at, updated_at`,
		req.Slug, req.Label, nilIfEmpty(req.Description), nilIfEmpty(req.Icon), req.IsSystem, jsonOrNil(acJSON), createdByUUID,
	).Scan(&id, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Collection{}, fmt.Errorf("insert collection: %w", err)
	}
	c.ID = uuidStr(id)
	c.Slug = req.Slug
	c.Label = req.Label
	c.Description = req.Description
	c.Icon = req.Icon
	c.IsSystem = req.IsSystem
	c.CreatedBy = req.CreatedBy
	return c, nil
}

func (s *Store) UpdateCollection(ctx context.Context, id string, req *UpdateCollectionReq) (Collection, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Collection{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Build dynamic SET clause.
	sets := []string{}
	args := []any{}
	argIdx := 1

	if req.Label != nil {
		sets = append(sets, fmt.Sprintf("label = $%d", argIdx))
		args = append(args, *req.Label)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, nilIfEmpty(*req.Description))
		argIdx++
	}
	if req.Icon != nil {
		sets = append(sets, fmt.Sprintf("icon = $%d", argIdx))
		args = append(args, nilIfEmpty(*req.Icon))
		argIdx++
	}
	if req.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}
	if req.ProcessEnabled != nil {
		sets = append(sets, fmt.Sprintf("process_enabled = $%d", argIdx))
		args = append(args, *req.ProcessEnabled)
		argIdx++
	}
	if req.AccessConfig != nil {
		sets = append(sets, fmt.Sprintf("access_config = $%d", argIdx))
		acJSON, err := json.Marshal(req.AccessConfig)
		if err != nil {
			return Collection{}, fmt.Errorf("marshal access_config: %w", err)
		}
		args = append(args, acJSON)
		argIdx++
	}
	if req.TitleFieldID != nil {
		sets = append(sets, fmt.Sprintf("title_field_id = $%d", argIdx))
		u, _ := parseUUID(*req.TitleFieldID)
		args = append(args, u)
		argIdx++
	}
	if req.DefaultSortField != nil {
		sets = append(sets, fmt.Sprintf("default_sort_field = $%d", argIdx))
		args = append(args, nilIfEmpty(*req.DefaultSortField))
		argIdx++
	}
	if req.DefaultSortOrder != nil {
		sets = append(sets, fmt.Sprintf("default_sort_order = $%d", argIdx))
		args = append(args, nilIfEmpty(*req.DefaultSortOrder))
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetCollection(ctx, id)
	}

	query := fmt.Sprintf(
		"UPDATE _meta.collections SET %s, updated_at = now() WHERE id = $%d RETURNING updated_at",
		joinStrings(sets, ", "), argIdx,
	)
	args = append(args, uid)

	var updatedAt pgtype.Timestamptz
	err = s.pool.QueryRow(ctx, query, args...).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Collection{}, fmt.Errorf("collection %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Collection{}, fmt.Errorf("update collection: %w", err)
	}

	return s.GetCollection(ctx, id)
}

func (s *Store) DeleteCollectionTx(ctx context.Context, tx pgx.Tx, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM _meta.collections WHERE id = $1`, uid)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("collection %s: %w", id, ErrNotFound)
	}
	return nil
}

// ---------- Field ----------

// ListFields returns all fields for a collection, ordered by sort_order then slug.
// Each field includes its relation metadata (if any) via a LEFT JOIN on _meta.relations,
// so relation fields are fully populated in a single query.
func (s *Store) ListFields(ctx context.Context, collectionID string) ([]Field, error) {
	uid, err := parseUUID(collectionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.collection_id, f.slug, f.label, f.field_type,
		       f.is_required, f.is_unique, f.is_indexed,
		       f.default_value, f.options, f.width, f.height, f.sort_order, f.is_layout,
		       f.created_at, f.updated_at,
		       r.id, r.target_collection_id, r.relation_type, r.junction_table, r.on_delete
		FROM _meta.fields f
		LEFT JOIN _meta.relations r ON r.field_id = f.id
		WHERE f.collection_id = $1
		ORDER BY f.sort_order, f.slug`, uid)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	defer rows.Close()

	var out []Field
	for rows.Next() {
		f, err := scanFieldRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan field: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetField(ctx context.Context, id string) (Field, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Field{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	row := s.pool.QueryRow(ctx, `
		SELECT f.id, f.collection_id, f.slug, f.label, f.field_type,
		       f.is_required, f.is_unique, f.is_indexed,
		       f.default_value, f.options, f.width, f.height, f.sort_order, f.is_layout,
		       f.created_at, f.updated_at,
		       r.id, r.target_collection_id, r.relation_type, r.junction_table, r.on_delete
		FROM _meta.fields f
		LEFT JOIN _meta.relations r ON r.field_id = f.id
		WHERE f.id = $1`, uid)
	f, err := scanFieldRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Field{}, fmt.Errorf("field %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Field{}, fmt.Errorf("get field: %w", err)
	}
	return f, nil
}

// CreateFieldTx inserts a field (and optional relation) inside an existing transaction.
// It takes a pgx.Tx because field creation must be atomic with the corresponding
// ALTER TABLE ADD COLUMN DDL executed by the migration engine.
func (s *Store) CreateFieldTx(ctx context.Context, tx pgx.Tx, collectionID string, req *CreateFieldIn) (Field, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return Field{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var fieldID pgtype.UUID
	var f Field

	isLayout := req.FieldType.IsLayout()
	width := req.Width
	if width == 0 {
		width = 6
	}
	height := req.Height
	if height == 0 {
		height = 1
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO _meta.fields
			(collection_id, slug, label, field_type, is_required, is_unique, is_indexed, default_value, options, width, height, sort_order, is_layout)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at`,
		colUID, req.Slug, req.Label, string(req.FieldType),
		req.IsRequired, req.IsUnique, req.IsIndexed,
		jsonOrNil(req.DefaultValue), jsonOrNil(req.Options),
		width, height,
		0, isLayout,
	).Scan(&fieldID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return Field{}, fmt.Errorf("insert field: %w", err)
	}

	f.ID = uuidStr(fieldID)
	f.CollectionID = collectionID
	f.Slug = req.Slug
	f.Label = req.Label
	f.FieldType = req.FieldType
	f.IsRequired = req.IsRequired
	f.IsUnique = req.IsUnique
	f.IsIndexed = req.IsIndexed
	f.DefaultValue = req.DefaultValue
	f.Options = req.Options
	f.Width = width
	f.Height = height
	f.IsLayout = isLayout

	// Insert relation if present.
	if req.Relation != nil {
		rel, err := s.createRelationTx(ctx, tx, f.ID, req.Relation)
		if err != nil {
			return Field{}, err
		}
		f.Relation = &rel
	}

	return f, nil
}

// UpdateFieldTx applies a partial update to a field's metadata inside an existing transaction.
// Only non-nil fields in the request are updated (dynamic SET clause construction).
// The actual DB column type change (if field_type changed) is handled by the migration engine.
func (s *Store) UpdateFieldTx(ctx context.Context, tx pgx.Tx, id string, req *UpdateFieldReq) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	sets := []string{}
	args := []any{}
	argIdx := 1

	if req.Label != nil {
		sets = append(sets, fmt.Sprintf("label = $%d", argIdx))
		args = append(args, *req.Label)
		argIdx++
	}
	if req.FieldType != nil {
		sets = append(sets, fmt.Sprintf("field_type = $%d", argIdx))
		args = append(args, string(*req.FieldType))
		argIdx++
	}
	if req.IsRequired != nil {
		sets = append(sets, fmt.Sprintf("is_required = $%d", argIdx))
		args = append(args, *req.IsRequired)
		argIdx++
	}
	if req.IsUnique != nil {
		sets = append(sets, fmt.Sprintf("is_unique = $%d", argIdx))
		args = append(args, *req.IsUnique)
		argIdx++
	}
	if req.IsIndexed != nil {
		sets = append(sets, fmt.Sprintf("is_indexed = $%d", argIdx))
		args = append(args, *req.IsIndexed)
		argIdx++
	}
	if req.DefaultValue != nil {
		sets = append(sets, fmt.Sprintf("default_value = $%d", argIdx))
		args = append(args, jsonOrNil(req.DefaultValue))
		argIdx++
	}
	if req.Options != nil {
		sets = append(sets, fmt.Sprintf("options = $%d", argIdx))
		args = append(args, jsonOrNil(req.Options))
		argIdx++
	}
	if req.Width != nil {
		sets = append(sets, fmt.Sprintf("width = $%d", argIdx))
		args = append(args, *req.Width)
		argIdx++
	}
	if req.Height != nil {
		sets = append(sets, fmt.Sprintf("height = $%d", argIdx))
		args = append(args, *req.Height)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		"UPDATE _meta.fields SET %s, updated_at = now() WHERE id = $%d",
		joinStrings(sets, ", "), argIdx,
	)
	args = append(args, uid)

	_, err = tx.Exec(ctx, query, args...)
	return err
}

// DeleteFieldTx removes a field's metadata row from _meta.fields inside an existing transaction.
// It does NOT drop the physical DB column — that is the migration engine's responsibility,
// ensuring the DDL and meta deletion happen atomically within the same transaction.
func (s *Store) DeleteFieldTx(ctx context.Context, tx pgx.Tx, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM _meta.fields WHERE id = $1`, uid)
	if err != nil {
		return fmt.Errorf("delete field: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("field %s: %w", id, ErrNotFound)
	}
	return nil
}

// ---------- Relation ----------

func (s *Store) createRelationTx(ctx context.Context, tx pgx.Tx, fieldID string, req *CreateRelIn) (Relation, error) {
	fUID, err := parseUUID(fieldID)
	if err != nil {
		return Relation{}, err
	}
	tUID, err := parseUUID(req.TargetCollectionID)
	if err != nil {
		return Relation{}, err
	}

	onDel := req.OnDelete
	if onDel == "" {
		onDel = "SET NULL"
	}

	var relID pgtype.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO _meta.relations (field_id, target_collection_id, relation_type, junction_table, on_delete)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		fUID, tUID, string(req.RelationType), nilIfEmpty(req.JunctionTable), onDel,
	).Scan(&relID)
	if err != nil {
		return Relation{}, fmt.Errorf("insert relation: %w", err)
	}

	return Relation{
		ID:                 uuidStr(relID),
		FieldID:            fieldID,
		TargetCollectionID: req.TargetCollectionID,
		RelationType:       req.RelationType,
		JunctionTable:      req.JunctionTable,
		OnDelete:           onDel,
	}, nil
}

// ---------- scan helpers ----------

// scanFieldRow reads a single row from the fields+relations LEFT JOIN query into a Field.
// The last five columns (rID, rTarget, rType, rJunc, rOnDel) are nullable because
// non-relation fields have no matching row in _meta.relations.
func scanFieldRow(row pgx.Row) (Field, error) {
	var (
		f     Field
		fID   pgtype.UUID
		colID pgtype.UUID
		ft    string
		defV  []byte
		opts  []byte
		// relation columns (nullable via LEFT JOIN)
		rID     pgtype.UUID
		rTarget pgtype.UUID
		rType   *string
		rJunc   *string
		rOnDel  *string
	)
	err := row.Scan(
		&fID, &colID, &f.Slug, &f.Label, &ft,
		&f.IsRequired, &f.IsUnique, &f.IsIndexed,
		&defV, &opts, &f.Width, &f.Height, &f.SortOrder, &f.IsLayout,
		&f.CreatedAt, &f.UpdatedAt,
		&rID, &rTarget, &rType, &rJunc, &rOnDel,
	)
	if err != nil {
		return Field{}, err
	}

	f.ID = uuidStr(fID)
	f.CollectionID = uuidStr(colID)
	f.FieldType = FieldType(ft)
	if defV != nil {
		f.DefaultValue = json.RawMessage(defV)
	}
	if opts != nil {
		f.Options = json.RawMessage(opts)
	}

	if rID.Valid {
		f.Relation = &Relation{
			ID:                 uuidStr(rID),
			FieldID:            f.ID,
			TargetCollectionID: uuidStr(rTarget),
		}
		if rType != nil {
			f.Relation.RelationType = RelationType(*rType)
		}
		if rJunc != nil {
			f.Relation.JunctionTable = *rJunc
		}
		if rOnDel != nil {
			f.Relation.OnDelete = *rOnDel
		}
	}

	return f, nil
}

// ---------- tiny helpers ----------

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func jsonOrNil(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += sep + s
	}
	return out
}
