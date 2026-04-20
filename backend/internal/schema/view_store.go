package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------- View ----------

func (s *Store) ListViews(ctx context.Context, collectionID string) ([]View, error) {
	uid, err := parseUUID(collectionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, collection_id, name, view_type, config, sort_order, is_default, created_at, updated_at
		FROM _meta.views
		WHERE collection_id = $1
		ORDER BY sort_order, name`, uid)
	if err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	defer rows.Close()

	var out []View
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, fmt.Errorf("scan view: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetView(ctx context.Context, id string) (View, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	row := s.pool.QueryRow(ctx, `
		SELECT id, collection_id, name, view_type, config, sort_order, is_default, created_at, updated_at
		FROM _meta.views WHERE id = $1`, uid)
	v, err := scanView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, fmt.Errorf("view %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return View{}, fmt.Errorf("get view: %w", err)
	}
	return v, nil
}

func (s *Store) CreateView(ctx context.Context, collectionID string, req *CreateViewReq) (View, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var v View
	var id pgtype.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO _meta.views (collection_id, name, view_type, config, sort_order, is_default)
		VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'), $5, $6)
		RETURNING id, created_at, updated_at`,
		colUID, req.Name, req.ViewType, jsonOrNil(req.Config), req.SortOrder, req.IsDefault,
	).Scan(&id, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return View{}, fmt.Errorf("insert view: %w", err)
	}
	v.ID = uuidStr(id)
	v.CollectionID = collectionID
	v.Name = req.Name
	v.ViewType = req.ViewType
	v.Config = req.Config
	v.SortOrder = req.SortOrder
	v.IsDefault = req.IsDefault
	return v, nil
}

func (s *Store) UpdateView(ctx context.Context, id string, req *UpdateViewReq) (View, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	sets := []string{}
	args := []any{}
	argIdx := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Config != nil {
		sets = append(sets, fmt.Sprintf("config = $%d", argIdx))
		args = append(args, jsonOrNil(req.Config))
		argIdx++
	}
	if req.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}
	if req.IsDefault != nil {
		sets = append(sets, fmt.Sprintf("is_default = $%d", argIdx))
		args = append(args, *req.IsDefault)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetView(ctx, id)
	}

	query := fmt.Sprintf(
		"UPDATE _meta.views SET %s, updated_at = now() WHERE id = $%d",
		joinStrings(sets, ", "), argIdx,
	)
	args = append(args, uid)

	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return View{}, fmt.Errorf("update view: %w", err)
	}

	return s.GetView(ctx, id)
}

func (s *Store) DeleteView(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM _meta.views WHERE id = $1`, uid)
	if err != nil {
		return fmt.Errorf("delete view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("view %s: %w", id, ErrNotFound)
	}
	return nil
}

func scanView(row pgx.Row) (View, error) {
	var (
		v     View
		id    pgtype.UUID
		colID pgtype.UUID
		cfg   []byte
	)
	err := row.Scan(&id, &colID, &v.Name, &v.ViewType, &cfg, &v.SortOrder, &v.IsDefault, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return View{}, err
	}
	v.ID = uuidStr(id)
	v.CollectionID = uuidStr(colID)
	if len(cfg) > 0 {
		v.Config = json.RawMessage(cfg)
	}
	return v, nil
}
