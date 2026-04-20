package httpretry

import (
	"fmt"
	"time"
)

type APIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
	Cause      error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error       { return e.Cause }
func (e *APIError) IsRetryable() bool   { return IsRetryable(e.StatusCode) }
func (e *APIError) IsRateLimited() bool { return e.StatusCode == 429 }
