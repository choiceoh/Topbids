// Package apierr provides structured API error types for REST responses.
// Adapted from Deneb gateway rpcerr.
package apierr

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Error codes
const (
	CodeBadRequest      = "BAD_REQUEST"
	CodeUnauthorized    = "UNAUTHORIZED"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeValidation      = "VALIDATION_FAILED"
	CodeTooManyRequests = "TOO_MANY_REQUESTS"
	CodeInternal        = "INTERNAL_ERROR"
)

// Error is a structured API error with code, message, HTTP status, and context.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Status  int            `json:"-"`
	Context map[string]any `json:"context,omitempty"`
	Cause   error          `json:"-"`
}

func New(status int, code, message string) *Error {
	return &Error{Code: code, Message: message, Status: status}
}

func Newf(status int, code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Status: status}
}

func Wrap(status int, code string, err error) *Error {
	return &Error{Code: code, Message: err.Error(), Status: status, Cause: err}
}

func (e *Error) Error() string {
	if len(e.Context) == 0 {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s] %s %v", e.Code, e.Message, e.Context)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) With(key string, value any) *Error {
	if e.Context == nil {
		e.Context = make(map[string]any, 4)
	}
	e.Context[key] = value
	return e
}

// Write sends the error as JSON response.
func (e *Error) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	json.NewEncoder(w).Encode(e)
}

// LogAttrs returns context as slog key-value pairs.
func (e *Error) LogAttrs() []any {
	attrs := make([]any, 0, 2+len(e.Context)*2)
	attrs = append(attrs, "code", e.Code, "message", e.Message)
	for k, v := range e.Context {
		attrs = append(attrs, k, v)
	}
	return attrs
}

// --- Convenience constructors ---

func BadRequest(msg string) *Error {
	return New(http.StatusBadRequest, CodeBadRequest, msg)
}

func Unauthorized(msg string) *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, msg)
}

func Forbidden(msg string) *Error {
	return New(http.StatusForbidden, CodeForbidden, msg)
}

func NotFound(resource string) *Error {
	return New(http.StatusNotFound, CodeNotFound, resource+" not found")
}

func Conflict(msg string) *Error {
	return New(http.StatusConflict, CodeConflict, msg)
}

func Validation(msg string) *Error {
	return New(http.StatusUnprocessableEntity, CodeValidation, msg)
}

func TooManyRequests(msg string) *Error {
	return New(http.StatusTooManyRequests, CodeTooManyRequests, msg)
}

func Internal(msg string) *Error {
	return New(http.StatusInternalServerError, CodeInternal, msg)
}

func WrapInternal(msg string, err error) *Error {
	return &Error{
		Code: CodeInternal, Message: msg + ": " + err.Error(),
		Status: http.StatusInternalServerError, Cause: err,
	}
}

func WrapBadRequest(msg string, err error) *Error {
	return &Error{
		Code: CodeBadRequest, Message: msg + ": " + err.Error(),
		Status: http.StatusBadRequest, Cause: err,
	}
}
