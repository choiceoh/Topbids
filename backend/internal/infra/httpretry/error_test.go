package httpretry

import (
	"errors"
	"testing"
)

func TestAPIError(t *testing.T) {
	cause := errors.New("connection reset")
	e := &APIError{StatusCode: 503, Message: "service unavailable", Cause: cause}

	if got := e.Error(); got != "API error 503: service unavailable" {
		t.Errorf("Error() = %q, want %q", got, "API error 503: service unavailable")
	}
	if !e.IsRetryable() {
		t.Error("503 should be retryable")
	}
	if e.IsRateLimited() {
		t.Error("503 should not be rate limited")
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return cause")
	}
}

func TestAPIErrorRateLimited(t *testing.T) {
	e := &APIError{StatusCode: 429, Message: "too many requests"}
	if !e.IsRateLimited() {
		t.Error("429 should be rate limited")
	}
	if !e.IsRetryable() {
		t.Error("429 should be retryable")
	}
}

func TestAPIErrorNotRetryable(t *testing.T) {
	e := &APIError{StatusCode: 400, Message: "bad request"}
	if e.IsRetryable() {
		t.Error("400 should not be retryable")
	}
	if e.IsRateLimited() {
		t.Error("400 should not be rate limited")
	}
}
