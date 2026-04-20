package pgutil

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRoundTrip(t *testing.T) {
	cases := []string{
		"00000000-0000-0000-0000-000000000000",
		"550e8400-e29b-41d4-a716-446655440000",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
		"a1b2c3d4-e5f6-0718-293a-4b5c6d7e8f90",
	}
	for _, in := range cases {
		u := ParseUUID(in)
		if !u.Valid {
			t.Errorf("ParseUUID(%q) returned invalid", in)
			continue
		}
		out := UUIDToString(u)
		if out != in {
			t.Errorf("round trip mismatch: got %q, want %q", out, in)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{
		"not-a-uuid",
		"550e8400-e29b-41d4-a716", // too short
		"550e8400-e29b-41d4-a716-446655440000-extra", // too long
		"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",       // invalid hex
	}
	for _, in := range cases {
		u := ParseUUID(in)
		if u.Valid {
			t.Errorf("ParseUUID(%q) should be invalid", in)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	u := ParseUUID("")
	if u.Valid {
		t.Error("empty string should yield invalid UUID")
	}
}

func TestUUIDToStringNull(t *testing.T) {
	u := pgtype.UUID{Valid: false}
	if s := UUIDToString(u); s != "" {
		t.Errorf("invalid UUID should return empty string, got %q", s)
	}
}

func TestFormatUUIDKnownBytes(t *testing.T) {
	// "550e8400-e29b-41d4-a716-446655440000"
	b := [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4,
		0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
	got := FormatUUID(b)
	want := "550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Errorf("FormatUUID: got %q, want %q", got, want)
	}
}

func TestCaseInsensitiveParse(t *testing.T) {
	upper := "550E8400-E29B-41D4-A716-446655440000"
	u := ParseUUID(upper)
	if !u.Valid {
		t.Fatal("uppercase hex should parse")
	}
	lower := UUIDToString(u)
	if lower != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected lowercase output, got %q", lower)
	}
}
