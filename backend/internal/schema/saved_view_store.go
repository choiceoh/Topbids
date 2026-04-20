package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------- Saved View ----------

// ListSavedViews returns public saved views plus the caller's private views.
func (s *Store) ListSavedViews(ctx context.Context, collectionID, userID string) ([]SavedView, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	userUID, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, collection_id, name, filter_config, sort_config,
		       visible_fields, is_default, is_public, created_by, created_at, updated_at
		FROM _meta.saved_views
		WHERE collection_id = $1
		  AND (is_public = TRUE OR created_by = $2)
		ORDER BY is_default DESC, name`, colUID, userUID)
	if err != nil {
		return nil, fmt.Errorf("list saved views: %w", err)
	}
	defer rows.Close()

	var out []SavedView
	for rows.Next() {
		v, err := scanSavedView(rows)
		if err != nil {
			return nil, fmt.Errorf("scan saved view: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetSavedView fetches a single saved view by ID.
func (s *Store) GetSavedView(ctx context.Context, id string) (SavedView, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return SavedView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	row := s.pool.QueryRow(ctx, `
		SELECT id, collection_id, name, filter_config, sort_config,
		       visible_fields, is_default, is_public, created_by, created_at, updated_at
		FROM _meta.saved_views WHERE id = $1`, uid)
	v, err := scanSavedView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SavedView{}, fmt.Errorf("saved view %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return SavedView{}, fmt.Errorf("get saved view: %w", err)
	}
	return v, nil
}

// CreateSavedView inserts a new saved view.
func (s *Store) CreateSavedView(ctx context.Context, collectionID, userID string, req *CreateSavedViewReq) (SavedView, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return SavedView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	userUID, err := parseUUID(userID)
	if err != nil {
		return SavedView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var v SavedView
	var id pgtype.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO _meta.saved_views
			(collection_id, name, filter_config, sort_config, visible_fields, is_default, is_public, created_by)
		VALUES ($1, $2, COALESCE($3::jsonb, '{}'), $4, $5::jsonb, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		colUID, req.Name, jsonOrNil(req.FilterConfig), req.SortConfig,
		jsonOrNil(req.VisibleFields), req.IsDefault, req.IsPublic, userUID,
	).Scan(&id, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return SavedView{}, fmt.Errorf("insert saved view: %w", err)
	}

	v.ID = uuidStr(id)
	v.CollectionID = collectionID
	v.Name = req.Name
	v.FilterConfig = req.FilterConfig
	v.SortConfig = req.SortConfig
	v.VisibleFields = req.VisibleFields
	v.IsDefault = req.IsDefault
	v.IsPublic = req.IsPublic
	v.CreatedBy = &userID
	return v, nil
}

// UpdateSavedView patches an existing saved view.
func (s *Store) UpdateSavedView(ctx context.Context, id string, req *UpdateSavedViewReq) (SavedView, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return SavedView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	sets := []string{}
	args := []any{}
	argIdx := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.FilterConfig != nil {
		sets = append(sets, fmt.Sprintf("filter_config = $%d", argIdx))
		args = append(args, jsonOrNil(req.FilterConfig))
		argIdx++
	}
	if req.SortConfig != nil {
		sets = append(sets, fmt.Sprintf("sort_config = $%d", argIdx))
		args = append(args, *req.SortConfig)
		argIdx++
	}
	if req.VisibleFields != nil {
		sets = append(sets, fmt.Sprintf("visible_fields = $%d", argIdx))
		args = append(args, jsonOrNil(req.VisibleFields))
		argIdx++
	}
	if req.IsDefault != nil {
		sets = append(sets, fmt.Sprintf("is_default = $%d", argIdx))
		args = append(args, *req.IsDefault)
		argIdx++
	}
	if req.IsPublic != nil {
		sets = append(sets, fmt.Sprintf("is_public = $%d", argIdx))
		args = append(args, *req.IsPublic)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetSavedView(ctx, id)
	}

	query := fmt.Sprintf(
		"UPDATE _meta.saved_views SET %s, updated_at = now() WHERE id = $%d",
		joinStrings(sets, ", "), argIdx,
	)
	args = append(args, uid)

	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return SavedView{}, fmt.Errorf("update saved view: %w", err)
	}
	return s.GetSavedView(ctx, id)
}

// DeleteSavedView removes a saved view.
func (s *Store) DeleteSavedView(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM _meta.saved_views WHERE id = $1`, uid)
	if err != nil {
		return fmt.Errorf("delete saved view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("saved view %s: %w", id, ErrNotFound)
	}
	return nil
}

func scanSavedView(row pgx.Row) (SavedView, error) {
	var (
		v         SavedView
		id        pgtype.UUID
		colID     pgtype.UUID
		filterCfg []byte
		visFlds   []byte
		createdBy pgtype.UUID
	)
	err := row.Scan(&id, &colID, &v.Name, &filterCfg, &v.SortConfig,
		&visFlds, &v.IsDefault, &v.IsPublic, &createdBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return SavedView{}, err
	}
	v.ID = uuidStr(id)
	v.CollectionID = uuidStr(colID)
	if len(filterCfg) > 0 {
		v.FilterConfig = json.RawMessage(filterCfg)
	}
	if len(visFlds) > 0 {
		v.VisibleFields = json.RawMessage(visFlds)
	}
	if createdBy.Valid {
		s := uuidStr(createdBy)
		v.CreatedBy = &s
	}
	return v, nil
}
