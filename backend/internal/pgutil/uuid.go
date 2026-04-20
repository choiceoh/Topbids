// Package pgutil provides small PostgreSQL helpers shared across the Phaeton engine.
package pgutil

import "github.com/jackc/pgx/v5/pgtype"

// UUIDToString converts a pgtype.UUID into the canonical 36-char dashed string.
// Returns "" if the UUID is NULL.
func UUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return FormatUUID(u.Bytes)
}

// FormatUUID converts a 16-byte UUID value into the canonical dashed string.
func FormatUUID(b [16]byte) string {
	const hex = "0123456789abcdef"
	var buf [36]byte
	// positions of each byte's first hex digit in the output buffer
	positions := [16]int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34}
	for i, p := range positions {
		buf[p] = hex[b[i]>>4]
		buf[p+1] = hex[b[i]&0x0f]
	}
	buf[8] = '-'
	buf[13] = '-'
	buf[18] = '-'
	buf[23] = '-'
	return string(buf[:])
}

// ParseUUID converts a 36-char dashed UUID string into a pgtype.UUID.
// An empty string returns an invalid (NULL) UUID without error.
func ParseUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	clean := make([]byte, 0, 32)
	for i := range len(s) {
		if s[i] != '-' {
			clean = append(clean, s[i])
		}
	}
	if len(clean) != 32 {
		return pgtype.UUID{}
	}
	var u pgtype.UUID
	for i := range 16 {
		hi := unhex(clean[i*2])
		lo := unhex(clean[i*2+1])
		if hi == 0xff || lo == 0xff {
			return pgtype.UUID{}
		}
		u.Bytes[i] = hi<<4 | lo
	}
	u.Valid = true
	return u
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xff
	}
}
