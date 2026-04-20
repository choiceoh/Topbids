package schema

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetProcess returns the process configuration for a collection.
// If no process row exists, it returns a zero-value Process with IsEnabled=false (not an error).
func (s *Store) GetProcess(ctx context.Context, collectionID string) (Process, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return Process{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var (
		proc Process
		id   pgtype.UUID
	)
	err = s.pool.QueryRow(ctx, `
		SELECT id, collection_id, is_enabled, created_at, updated_at
		FROM _meta.processes
		WHERE collection_id = $1`, colUID,
	).Scan(&id, &colUID, &proc.IsEnabled, &proc.CreatedAt, &proc.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return Process{CollectionID: collectionID}, nil
		}
		return Process{}, fmt.Errorf("get process: %w", err)
	}
	proc.ID = uuidStr(id)
	proc.CollectionID = uuidStr(colUID)

	// Load statuses.
	statuses, err := s.listProcessStatuses(ctx, proc.ID)
	if err != nil {
		return Process{}, err
	}
	proc.Statuses = statuses

	// Load transitions.
	transitions, err := s.listProcessTransitions(ctx, proc.ID)
	if err != nil {
		return Process{}, err
	}
	proc.Transitions = transitions

	return proc, nil
}

func (s *Store) listProcessStatuses(ctx context.Context, processID string) ([]ProcessStatus, error) {
	pUID, err := parseUUID(processID)
	if err != nil {
		return nil, fmt.Errorf("invalid process ID %q: %w", processID, err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, process_id, name, color, sort_order, is_initial, created_at, updated_at
		FROM _meta.process_statuses
		WHERE process_id = $1
		ORDER BY sort_order, name`, pUID)
	if err != nil {
		return nil, fmt.Errorf("list process statuses: %w", err)
	}
	defer rows.Close()

	var out []ProcessStatus
	for rows.Next() {
		var (
			ps  ProcessStatus
			id  pgtype.UUID
			pid pgtype.UUID
		)
		if err := rows.Scan(&id, &pid, &ps.Name, &ps.Color, &ps.SortOrder, &ps.IsInitial, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan process status: %w", err)
		}
		ps.ID = uuidStr(id)
		ps.ProcessID = uuidStr(pid)
		out = append(out, ps)
	}
	return out, rows.Err()
}

func (s *Store) listProcessTransitions(ctx context.Context, processID string) ([]ProcessTransition, error) {
	pUID, err := parseUUID(processID)
	if err != nil {
		return nil, fmt.Errorf("invalid process ID %q: %w", processID, err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, process_id, from_status_id, to_status_id, label, allowed_roles, allowed_user_ids, created_at
		FROM _meta.process_transitions
		WHERE process_id = $1
		ORDER BY created_at`, pUID)
	if err != nil {
		return nil, fmt.Errorf("list process transitions: %w", err)
	}
	defer rows.Close()

	var out []ProcessTransition
	for rows.Next() {
		var (
			pt   ProcessTransition
			id   pgtype.UUID
			pid  pgtype.UUID
			from pgtype.UUID
			to   pgtype.UUID
		)
		if err := rows.Scan(&id, &pid, &from, &to, &pt.Label, &pt.AllowedRoles, &pt.AllowedUserIDs, &pt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan process transition: %w", err)
		}
		pt.ID = uuidStr(id)
		pt.ProcessID = uuidStr(pid)
		pt.FromStatusID = uuidStr(from)
		pt.ToStatusID = uuidStr(to)
		if pt.AllowedRoles == nil {
			pt.AllowedRoles = []string{}
		}
		if pt.AllowedUserIDs == nil {
			pt.AllowedUserIDs = []string{}
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}

// SaveProcessTx saves the entire process configuration inside an existing transaction.
// It performs an UPSERT on the process row, then replaces all statuses and transitions.
func (s *Store) SaveProcessTx(ctx context.Context, tx pgx.Tx, collectionID string, req *SaveProcessReq) (Process, error) {
	colUID, err := parseUUID(collectionID)
	if err != nil {
		return Process{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Upsert process row.
	var (
		procID pgtype.UUID
		proc   Process
	)
	err = tx.QueryRow(ctx, `
		INSERT INTO _meta.processes (collection_id, is_enabled)
		VALUES ($1, $2)
		ON CONFLICT (collection_id) DO UPDATE SET
			is_enabled = EXCLUDED.is_enabled,
			updated_at = now()
		RETURNING id, created_at, updated_at`,
		colUID, req.IsEnabled,
	).Scan(&procID, &proc.CreatedAt, &proc.UpdatedAt)
	if err != nil {
		return Process{}, fmt.Errorf("upsert process: %w", err)
	}
	proc.ID = uuidStr(procID)
	proc.CollectionID = collectionID
	proc.IsEnabled = req.IsEnabled

	// Delete existing transitions first (FK to statuses).
	if _, err := tx.Exec(ctx, `DELETE FROM _meta.process_transitions WHERE process_id = $1`, procID); err != nil {
		return Process{}, fmt.Errorf("delete transitions: %w", err)
	}
	// Delete existing statuses.
	if _, err := tx.Exec(ctx, `DELETE FROM _meta.process_statuses WHERE process_id = $1`, procID); err != nil {
		return Process{}, fmt.Errorf("delete statuses: %w", err)
	}

	// Insert new statuses, collecting generated UUIDs.
	statusIDs := make([]string, len(req.Statuses))
	proc.Statuses = make([]ProcessStatus, len(req.Statuses))
	for i, s := range req.Statuses {
		var sid pgtype.UUID
		var ps ProcessStatus
		err := tx.QueryRow(ctx, `
			INSERT INTO _meta.process_statuses (process_id, name, color, sort_order, is_initial)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at, updated_at`,
			procID, s.Name, s.Color, s.SortOrder, s.IsInitial,
		).Scan(&sid, &ps.CreatedAt, &ps.UpdatedAt)
		if err != nil {
			return Process{}, fmt.Errorf("insert status[%d]: %w", i, err)
		}
		ps.ID = uuidStr(sid)
		ps.ProcessID = proc.ID
		ps.Name = s.Name
		ps.Color = s.Color
		ps.SortOrder = s.SortOrder
		ps.IsInitial = s.IsInitial
		statusIDs[i] = ps.ID
		proc.Statuses[i] = ps
	}

	// Insert new transitions, resolving indices to UUIDs.
	proc.Transitions = make([]ProcessTransition, len(req.Transitions))
	for i, t := range req.Transitions {
		fromUID, _ := parseUUID(statusIDs[t.FromIndex])
		toUID, _ := parseUUID(statusIDs[t.ToIndex])
		roles := t.AllowedRoles
		if roles == nil {
			roles = []string{}
		}
		userIDs := t.AllowedUserIDs
		if userIDs == nil {
			userIDs = []string{}
		}
		// Convert string user IDs to pgtype.UUID slice for the UUID[] column.
		pgUserIDs := make([]pgtype.UUID, len(userIDs))
		for j, uid := range userIDs {
			pgUserIDs[j], _ = parseUUID(uid)
		}
		var tid pgtype.UUID
		var pt ProcessTransition
		err := tx.QueryRow(ctx, `
			INSERT INTO _meta.process_transitions (process_id, from_status_id, to_status_id, label, allowed_roles, allowed_user_ids)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at`,
			procID, fromUID, toUID, t.Label, roles, pgUserIDs,
		).Scan(&tid, &pt.CreatedAt)
		if err != nil {
			return Process{}, fmt.Errorf("insert transition[%d]: %w", i, err)
		}
		pt.ID = uuidStr(tid)
		pt.ProcessID = proc.ID
		pt.FromStatusID = statusIDs[t.FromIndex]
		pt.ToStatusID = statusIDs[t.ToIndex]
		pt.Label = t.Label
		pt.AllowedRoles = roles
		pt.AllowedUserIDs = userIDs
		proc.Transitions[i] = pt
	}

	return proc, nil
}
