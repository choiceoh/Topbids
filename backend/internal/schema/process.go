package schema

import "time"

// Process represents the workflow configuration for a collection.
type Process struct {
	ID           string              `json:"id"`
	CollectionID string              `json:"collection_id"`
	IsEnabled    bool                `json:"is_enabled"`
	Statuses     []ProcessStatus     `json:"statuses"`
	Transitions  []ProcessTransition `json:"transitions"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// ProcessStatus is a single state node in a process workflow.
type ProcessStatus struct {
	ID        string    `json:"id"`
	ProcessID string    `json:"process_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	SortOrder int       `json:"sort_order"`
	IsInitial bool      `json:"is_initial"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProcessTransition is a directed edge between two statuses.
type ProcessTransition struct {
	ID             string    `json:"id"`
	ProcessID      string    `json:"process_id"`
	FromStatusID   string    `json:"from_status_id"`
	ToStatusID     string    `json:"to_status_id"`
	Label          string    `json:"label"`
	AllowedRoles   []string  `json:"allowed_roles"`
	AllowedUserIDs []string  `json:"allowed_user_ids"`
	CreatedAt      time.Time `json:"created_at"`
}

// --- Request DTOs ---

// SaveProcessReq is the payload for PUT /api/schema/collections/{id}/process.
// The entire process configuration (statuses + transitions) is saved atomically.
type SaveProcessReq struct {
	IsEnabled   bool                      `json:"is_enabled"`
	Statuses    []SaveProcessStatusIn     `json:"statuses"`
	Transitions []SaveProcessTransitionIn `json:"transitions"`
}

// SaveProcessStatusIn defines a status node in the save request.
type SaveProcessStatusIn struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	IsInitial bool   `json:"is_initial"`
}

// SaveProcessTransitionIn defines a transition edge in the save request.
// FromIndex/ToIndex reference positions in the Statuses array (resolved to UUIDs
// by the backend after inserting statuses).
type SaveProcessTransitionIn struct {
	FromIndex      int      `json:"from_index"`
	ToIndex        int      `json:"to_index"`
	Label          string   `json:"label"`
	AllowedRoles   []string `json:"allowed_roles"`
	AllowedUserIDs []string `json:"allowed_user_ids"`
}
