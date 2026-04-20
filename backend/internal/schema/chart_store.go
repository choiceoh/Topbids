package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Chart represents a saved chart configuration for a collection.
type Chart struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Name         string          `json:"name"`
	ChartType    string          `json:"chart_type"` // bar, line, pie, doughnut, area
	Config       json.RawMessage `json:"config"`
	SortOrder    int             `json:"sort_order"`
	CreatedBy    *string         `json:"created_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type CreateChartReq struct {
	Name      string          `json:"name"`
	ChartType string          `json:"chart_type"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder int             `json:"sort_order"`
}

type UpdateChartReq struct {
	Name      *string         `json:"name,omitempty"`
	ChartType *string         `json:"chart_type,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
	SortOrder *int            `json:"sort_order,omitempty"`
}

const chartCols = `id, collection_id, name, chart_type, config, sort_order, created_by, created_at, updated_at`

func scanChart(row pgx.Row) (Chart, error) {
	var (
		c         Chart
		id        pgtype.UUID
		colID     pgtype.UUID
		createdBy pgtype.UUID
		cfgRaw    []byte
	)
	err := row.Scan(&id, &colID, &c.Name, &c.ChartType, &cfgRaw, &c.SortOrder, &createdBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Chart{}, err
	}
	c.ID = uuidStr(id)
	c.CollectionID = uuidStr(colID)
	if createdBy.Valid {
		s := uuidStr(createdBy)
		c.CreatedBy = &s
	}
	c.Config = cfgRaw
	return c, nil
}

// ListCharts returns all charts for a collection ordered by sort_order.
func (s *Store) ListCharts(ctx context.Context, collectionID string) ([]Chart, error) {
	colUUID, err := parseUUID(collectionID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid collection_id", ErrInvalidInput)
	}
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf("SELECT %s FROM _meta.charts WHERE collection_id = $1 ORDER BY sort_order, created_at", chartCols),
		colUUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var charts []Chart
	for rows.Next() {
		c, err := scanChart(rows)
		if err != nil {
			return nil, err
		}
		charts = append(charts, c)
	}
	return charts, rows.Err()
}

// GetChart returns a single chart by ID.
func (s *Store) GetChart(ctx context.Context, chartID string) (Chart, error) {
	id, err := parseUUID(chartID)
	if err != nil {
		return Chart{}, fmt.Errorf("%w: invalid chart id", ErrInvalidInput)
	}
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT %s FROM _meta.charts WHERE id = $1", chartCols), id)
	c, err := scanChart(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Chart{}, fmt.Errorf("%w: chart not found", ErrNotFound)
		}
		return Chart{}, err
	}
	return c, nil
}

// CreateChart inserts a new chart.
func (s *Store) CreateChart(ctx context.Context, collectionID string, req CreateChartReq, userID string) (Chart, error) {
	colUUID, err := parseUUID(collectionID)
	if err != nil {
		return Chart{}, fmt.Errorf("%w: invalid collection_id", ErrInvalidInput)
	}
	userUUID, _ := parseUUID(userID)

	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO _meta.charts (collection_id, name, chart_type, config, sort_order, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING %s`, chartCols),
		colUUID, req.Name, req.ChartType, cfg, req.SortOrder, userUUID,
	)
	return scanChart(row)
}

// UpdateChart patches a chart.
func (s *Store) UpdateChart(ctx context.Context, chartID string, req UpdateChartReq) (Chart, error) {
	id, err := parseUUID(chartID)
	if err != nil {
		return Chart{}, fmt.Errorf("%w: invalid chart id", ErrInvalidInput)
	}

	// Build SET clause dynamically.
	sets := []string{"updated_at = now()"}
	args := []any{id}
	idx := 2

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.ChartType != nil {
		sets = append(sets, fmt.Sprintf("chart_type = $%d", idx))
		args = append(args, *req.ChartType)
		idx++
	}
	if req.Config != nil {
		sets = append(sets, fmt.Sprintf("config = $%d", idx))
		args = append(args, []byte(req.Config))
		idx++
	}
	if req.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", idx))
		args = append(args, *req.SortOrder)
	}

	sql := fmt.Sprintf("UPDATE _meta.charts SET %s WHERE id = $1 RETURNING %s",
		joinSets(sets), chartCols)

	row := s.pool.QueryRow(ctx, sql, args...)
	c, err := scanChart(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Chart{}, fmt.Errorf("%w: chart not found", ErrNotFound)
		}
		return Chart{}, err
	}
	return c, nil
}

// DeleteChart removes a chart by ID.
func (s *Store) DeleteChart(ctx context.Context, chartID string) error {
	id, err := parseUUID(chartID)
	if err != nil {
		return fmt.Errorf("%w: invalid chart id", ErrInvalidInput)
	}
	tag, err := s.pool.Exec(ctx, "DELETE FROM _meta.charts WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: chart not found", ErrNotFound)
	}
	return nil
}

// joinSets is a tiny helper to join SET fragments.
func joinSets(parts []string) string {
	result := parts[0]
	for _, p := range parts[1:] {
		result += ", " + p
	}
	return result
}
