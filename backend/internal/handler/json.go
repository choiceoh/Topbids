package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/choiceoh/topbid/backend/internal/infra/apierr"
	"github.com/choiceoh/topbid/backend/internal/schema"
)

// withDeadline returns a new request with the given timeout applied to its context.
// The caller must call the returned cancel function when done.
func withDeadline(r *http.Request, d time.Duration) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), d)
	return r.WithContext(ctx), cancel
}

// isTimeout returns true if the error is a context deadline exceeded.
func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// handleTimeout writes a 504 Gateway Timeout if the error is a context deadline exceeded.
// Returns true if it handled the error, false otherwise.
func handleTimeout(w http.ResponseWriter, r *http.Request, err error) bool {
	if isTimeout(err) {
		apierr.New(http.StatusGatewayTimeout, "TIMEOUT", "request timed out").Write(w)
		return true
	}
	return false
}

type envelope struct {
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type listEnvelope struct {
	Data       any   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"total_pages"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(envelope{Data: data})
}

func writeList(w http.ResponseWriter, data any, total int64, page, limit int) {
	pages := int(total) / limit
	if int(total)%limit != 0 {
		pages++
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listEnvelope{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: pages,
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	code := statusToCode(status)
	apierr.New(status, code, msg).Write(w)
}

func statusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return apierr.CodeBadRequest
	case http.StatusUnauthorized:
		return apierr.CodeUnauthorized
	case http.StatusForbidden:
		return apierr.CodeForbidden
	case http.StatusNotFound:
		return apierr.CodeNotFound
	case http.StatusConflict:
		return apierr.CodeConflict
	case http.StatusUnprocessableEntity:
		return apierr.CodeValidation
	case http.StatusTooManyRequests:
		return apierr.CodeTooManyRequests
	default:
		return apierr.CodeInternal
	}
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// errorStatus maps domain errors to HTTP status codes.
func errorStatus(err error) int {
	switch {
	case errors.Is(err, schema.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, schema.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, schema.ErrInvalidInput):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// publicMessage decides what to expose to the client. Domain errors (validation,
// not-found, conflict) are user-facing and shown in full. Anything else is
// considered internal and replaced with a generic message — the original error
// is logged at error level for operators.
func publicMessage(err error) string {
	switch {
	case errors.Is(err, schema.ErrNotFound),
		errors.Is(err, schema.ErrConflict),
		errors.Is(err, schema.ErrInvalidInput):
		return err.Error()
	default:
		return "internal server error"
	}
}

// handleErr is the central error response helper. It logs internal errors at
// error level (with request context) and returns a sanitized message to the client.
// If err is already an *apierr.Error, its status/code/context are preserved.
func handleErr(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		if ae.Status >= 500 {
			Log(r).Error("internal error", ae.LogAttrs()...)
		}
		ae.Write(w)
		return
	}

	status := errorStatus(err)
	msg := publicMessage(err)

	if status >= 500 {
		// Log full details for operators; user sees only "internal server error".
		Log(r).Error("internal error",
			slog.String("error", err.Error()),
			slog.Int("status", status),
		)
	}

	apierr.New(status, statusToCode(status), msg).Write(w)
}
