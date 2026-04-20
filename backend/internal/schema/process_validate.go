package schema

import (
	"fmt"
	"strings"
)

// ValidateProcessSave checks the SaveProcessReq for consistency.
func ValidateProcessSave(req *SaveProcessReq) error {
	if !req.IsEnabled {
		return nil // disabling requires no validation
	}

	if len(req.Statuses) == 0 {
		return fmt.Errorf("%w: 최소 하나의 상태가 필요합니다", ErrInvalidInput)
	}

	initialCount := 0
	names := make(map[string]bool, len(req.Statuses))
	for i, s := range req.Statuses {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("%w: statuses[%d] 이름이 비어있습니다", ErrInvalidInput, i)
		}
		lower := strings.ToLower(s.Name)
		if names[lower] {
			return fmt.Errorf("%w: 상태 이름 %q이(가) 중복됩니다", ErrInvalidInput, s.Name)
		}
		names[lower] = true
		if s.IsInitial {
			initialCount++
		}
	}

	if initialCount != 1 {
		return fmt.Errorf("%w: 정확히 하나의 초기 상태가 필요합니다 (현재 %d개)", ErrInvalidInput, initialCount)
	}

	validRoles := map[string]bool{"director": true, "pm": true, "engineer": true, "viewer": true}

	for i, t := range req.Transitions {
		if t.FromIndex < 0 || t.FromIndex >= len(req.Statuses) {
			return fmt.Errorf("%w: transitions[%d].from_index %d 범위 초과", ErrInvalidInput, i, t.FromIndex)
		}
		if t.ToIndex < 0 || t.ToIndex >= len(req.Statuses) {
			return fmt.Errorf("%w: transitions[%d].to_index %d 범위 초과", ErrInvalidInput, i, t.ToIndex)
		}
		if strings.TrimSpace(t.Label) == "" {
			return fmt.Errorf("%w: transitions[%d] 이름이 비어있습니다", ErrInvalidInput, i)
		}
		for _, role := range t.AllowedRoles {
			if !validRoles[role] {
				return fmt.Errorf("%w: transitions[%d] 유효하지 않은 역할 %q", ErrInvalidInput, i, role)
			}
		}
		for j, uid := range t.AllowedUserIDs {
			if _, err := parseUUID(uid); err != nil {
				return fmt.Errorf("%w: transitions[%d].allowed_user_ids[%d] 유효하지 않은 UUID %q", ErrInvalidInput, i, j, uid)
			}
		}
	}

	// Warn about isolated (unreachable) statuses.
	// A non-initial status that has no incoming transition is unreachable.
	if len(req.Transitions) > 0 {
		hasIncoming := make(map[int]bool, len(req.Statuses))
		for _, t := range req.Transitions {
			hasIncoming[t.ToIndex] = true
		}
		for i, s := range req.Statuses {
			if !s.IsInitial && !hasIncoming[i] {
				return fmt.Errorf("%w: 상태 %q(은)는 어떤 전이에서도 도달할 수 없습니다", ErrInvalidInput, s.Name)
			}
		}
	}

	return nil
}
