// Package jsonutil provides JSON parsing helpers with error context.
// Ported from Deneb gateway.
package jsonutil

import (
	"encoding/json"
	"fmt"
)

// Unmarshal decodes JSON data into T with consistent error wrapping.
func Unmarshal[T any](context string, data []byte) (T, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("parse %s: %w", context, err)
	}
	return v, nil
}

// UnmarshalInto decodes JSON data into v (pointer) with consistent error wrapping.
func UnmarshalInto(context string, data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", context, err)
	}
	return nil
}
