package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BidRole values for AccessConfig.BidRole. An empty string means the collection
// is not part of the bidding domain and no row-level sealed filter applies.
//
// Runtime enforcement (SealedReadFilter, scheduler, award/PO actions) only
// activates for collections with a non-empty BidRole. See docs/10-BID-EXTENSION.md.
const (
	BidRoleRfq      = "rfq"      // RFQ announcement collection (공고)
	BidRoleBid      = "bid"      // Bid submission — triggers sealed row filter (입찰서)
	BidRoleSupplier = "supplier" // Supplier registry (공급사)
	BidRoleAward    = "award"    // Award decision (낙찰)
	BidRolePO       = "po"       // Purchase order (발주)
)

// validBidRoles enumerates the allowed values for AccessConfig.BidRole.
// Empty string is also valid (non-bid collection).
var validBidRoles = map[string]bool{
	BidRoleRfq: true, BidRoleBid: true, BidRoleSupplier: true,
	BidRoleAward: true, BidRolePO: true,
}

// ValidateBidRole checks that a BidRole value is empty or one of the known values.
func ValidateBidRole(role string) error {
	if role == "" {
		return nil
	}
	if !validBidRoles[role] {
		return fmt.Errorf("%w: invalid bid_role %q; must be one of rfq, bid, supplier, award, po", ErrInvalidInput, role)
	}
	return nil
}

// SealedOptions configures row-level sealed bid visibility on a field.
// Stored inside a field's Options JSON alongside other type-specific keys
// (e.g. "choices" for select). Nil-valued — a field without sealed config
// simply has neither key set.
//
// UntilAt formats:
//   - "field:<slug>" — resolves at read time to the value of the named
//     datetime field on the same row (e.g. "field:open_at")
//   - RFC3339 absolute time (e.g. "2026-05-01T00:00:00Z")
//
// UnlockByStatus is an OR list: if the row's status field value is in this
// list, the field is readable regardless of UntilAt.
//
// Only fields whose parent collection has AccessConfig.BidRole=="bid" are
// subject to the sealed filter at query time; validation here is purely
// structural.
type SealedOptions struct {
	UntilAt        string   `json:"sealed_until_at,omitempty"`
	UnlockByStatus []string `json:"unlock_by_status,omitempty"`
}

// HasSealing reports whether any sealing configuration is present.
func (s *SealedOptions) HasSealing() bool {
	return s != nil && (s.UntilAt != "" || len(s.UnlockByStatus) > 0)
}

// ExtractSealedOptions parses sealed_until_at / unlock_by_status from a field's
// raw Options JSON. Returns nil (no error) if neither key is present or the
// input is empty/null. Coexists with other option keys on the same object.
func ExtractSealedOptions(raw json.RawMessage) (*SealedOptions, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var opts SealedOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("sealed options: %w", err)
	}
	if !opts.HasSealing() {
		return nil, nil
	}
	return &opts, nil
}

// ValidateSealedOptions checks the format of sealed_until_at and
// unlock_by_status if present. No-op if neither key is set.
//
//   - sealed_until_at must be either "field:<slug>" with a valid field slug
//     or an RFC3339 timestamp.
//   - unlock_by_status entries must be non-empty strings.
//
// Runtime enforcement (row-level filter) happens in the bid access layer
// and only activates for collections with bid_role="bid".
func ValidateSealedOptions(raw json.RawMessage) error {
	opts, err := ExtractSealedOptions(raw)
	if err != nil {
		return err
	}
	if opts == nil {
		return nil
	}
	if opts.UntilAt != "" {
		if err := validateSealedUntilAt(opts.UntilAt); err != nil {
			return err
		}
	}
	for i, s := range opts.UnlockByStatus {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%w: unlock_by_status[%d] is empty", ErrInvalidInput, i)
		}
	}
	return nil
}

// validateSealedUntilAt checks that the value is either "field:<slug>" with a
// valid slug, or a parseable RFC3339 timestamp.
func validateSealedUntilAt(v string) error {
	if strings.HasPrefix(v, "field:") {
		slug := strings.TrimPrefix(v, "field:")
		if err := ValidateSlug(slug); err != nil {
			return fmt.Errorf("%w: sealed_until_at field ref %q: %v", ErrInvalidInput, v, err)
		}
		return nil
	}
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		return fmt.Errorf("%w: sealed_until_at must be 'field:<slug>' or RFC3339 time, got %q", ErrInvalidInput, v)
	}
	return nil
}
