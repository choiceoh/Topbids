package schema

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestExtractSealedOptions(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantNil bool
		check   func(t *testing.T, got *SealedOptions)
	}{
		{"empty", ``, true, nil},
		{"null", `null`, true, nil},
		{"no sealed keys", `{"choices":["A","B"]}`, true, nil},
		{
			name: "until_at only",
			raw:  `{"sealed_until_at":"field:open_at"}`,
			check: func(t *testing.T, got *SealedOptions) {
				if got.UntilAt != "field:open_at" {
					t.Errorf("UntilAt = %q, want %q", got.UntilAt, "field:open_at")
				}
			},
		},
		{
			name: "unlock_by_status only",
			raw:  `{"unlock_by_status":["opened","awarded"]}`,
			check: func(t *testing.T, got *SealedOptions) {
				if len(got.UnlockByStatus) != 2 {
					t.Errorf("UnlockByStatus len = %d, want 2", len(got.UnlockByStatus))
				}
			},
		},
		{
			name: "coexists with select choices",
			raw:  `{"choices":["A"],"sealed_until_at":"2026-05-01T00:00:00Z"}`,
			check: func(t *testing.T, got *SealedOptions) {
				if got.UntilAt != "2026-05-01T00:00:00Z" {
					t.Errorf("UntilAt = %q", got.UntilAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractSealedOptions(json.RawMessage(tt.raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil")
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestExtractSealedOptions_InvalidJSON(t *testing.T) {
	_, err := ExtractSealedOptions(json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateSealedOptions(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		errSub  string
	}{
		{"empty no-op", ``, false, ""},
		{"null no-op", `null`, false, ""},
		{"no sealed keys no-op", `{"choices":["A"]}`, false, ""},
		{"valid field ref", `{"sealed_until_at":"field:open_at"}`, false, ""},
		{"valid RFC3339", `{"sealed_until_at":"2026-05-01T00:00:00Z"}`, false, ""},
		{"valid unlock statuses", `{"unlock_by_status":["opened","awarded"]}`, false, ""},
		{"valid both", `{"sealed_until_at":"field:open_at","unlock_by_status":["opened"]}`, false, ""},

		{
			name: "invalid field slug (uppercase)",
			raw:  `{"sealed_until_at":"field:OpenAt"}`, wantErr: true,
			errSub: "sealed_until_at",
		},
		{
			name: "invalid field slug (reserved word)",
			raw:  `{"sealed_until_at":"field:order"}`, wantErr: true,
			errSub: "reserved",
		},
		{
			name: "invalid time format",
			raw:  `{"sealed_until_at":"2026-05-01"}`, wantErr: true,
			errSub: "RFC3339",
		},
		{
			name: "garbage value",
			raw:  `{"sealed_until_at":"notavalidvalue"}`, wantErr: true,
			errSub: "RFC3339",
		},
		{
			name: "empty status entry",
			raw:  `{"unlock_by_status":["opened","","awarded"]}`, wantErr: true,
			errSub: "unlock_by_status[1]",
		},
		{
			name: "whitespace status entry",
			raw:  `{"unlock_by_status":["   "]}`, wantErr: true,
			errSub: "unlock_by_status[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSealedOptions(json.RawMessage(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSub)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateBidRole(t *testing.T) {
	valid := []string{"", BidRoleRfq, BidRoleBid, BidRoleSupplier, BidRoleAward, BidRolePO}
	for _, r := range valid {
		if err := ValidateBidRole(r); err != nil {
			t.Errorf("ValidateBidRole(%q) unexpected error: %v", r, err)
		}
	}

	invalid := []string{"bidder", "rfp", "other", "BID"}
	for _, r := range invalid {
		err := ValidateBidRole(r)
		if err == nil {
			t.Errorf("ValidateBidRole(%q) expected error, got nil", r)
			continue
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ValidateBidRole(%q) expected ErrInvalidInput, got %v", r, err)
		}
	}
}

func TestSealedOptions_HasSealing(t *testing.T) {
	var nilPtr *SealedOptions
	if nilPtr.HasSealing() {
		t.Error("nil should not have sealing")
	}
	if (&SealedOptions{}).HasSealing() {
		t.Error("empty struct should not have sealing")
	}
	if !(&SealedOptions{UntilAt: "field:x"}).HasSealing() {
		t.Error("UntilAt set should have sealing")
	}
	if !(&SealedOptions{UnlockByStatus: []string{"opened"}}).HasSealing() {
		t.Error("UnlockByStatus set should have sealing")
	}
}
