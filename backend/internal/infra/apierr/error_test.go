package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	e := New(400, CodeBadRequest, "invalid input")
	if e.Status != 400 {
		t.Errorf("Status = %d, want 400", e.Status)
	}
	if e.Code != CodeBadRequest {
		t.Errorf("Code = %q, want %q", e.Code, CodeBadRequest)
	}
	if e.Message != "invalid input" {
		t.Errorf("Message = %q, want %q", e.Message, "invalid input")
	}
}

func TestNewf(t *testing.T) {
	e := Newf(404, CodeNotFound, "user %d not found", 42)
	if e.Message != "user 42 not found" {
		t.Errorf("Message = %q, want %q", e.Message, "user 42 not found")
	}
}

func TestErrorString(t *testing.T) {
	e := BadRequest("oops")
	want := "[BAD_REQUEST] oops"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrorStringWithContext(t *testing.T) {
	e := BadRequest("oops").With("field", "name")
	got := e.Error()
	if !strings.Contains(got, "[BAD_REQUEST]") || !strings.Contains(got, "oops") {
		t.Errorf("Error() = %q, want to contain code and message", got)
	}
}

func TestWith(t *testing.T) {
	e := NotFound("user").With("id", "abc").With("extra", 42)
	if len(e.Context) != 2 {
		t.Fatalf("Context len = %d, want 2", len(e.Context))
	}
	if e.Context["id"] != "abc" {
		t.Errorf("Context[id] = %v, want abc", e.Context["id"])
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("pg: connection refused")
	e := Wrap(500, CodeInternal, cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return cause")
	}
	if e.Message != "pg: connection refused" {
		t.Errorf("Message = %q, want cause message", e.Message)
	}
}

func TestConvenienceConstructors(t *testing.T) {
	cases := []struct {
		name   string
		err    *Error
		status int
		code   string
	}{
		{"BadRequest", BadRequest("x"), 400, CodeBadRequest},
		{"Unauthorized", Unauthorized("x"), 401, CodeUnauthorized},
		{"Forbidden", Forbidden("x"), 403, CodeForbidden},
		{"NotFound", NotFound("item"), 404, CodeNotFound},
		{"Conflict", Conflict("x"), 409, CodeConflict},
		{"Validation", Validation("x"), 422, CodeValidation},
		{"Internal", Internal("x"), 500, CodeInternal},
	}
	for _, tc := range cases {
		if tc.err.Status != tc.status {
			t.Errorf("%s: Status = %d, want %d", tc.name, tc.err.Status, tc.status)
		}
		if tc.err.Code != tc.code {
			t.Errorf("%s: Code = %q, want %q", tc.name, tc.err.Code, tc.code)
		}
	}
}

func TestWrapInternal(t *testing.T) {
	cause := errors.New("disk full")
	e := WrapInternal("write failed", cause)
	if e.Status != 500 {
		t.Errorf("Status = %d, want 500", e.Status)
	}
	if !strings.Contains(e.Message, "write failed") || !strings.Contains(e.Message, "disk full") {
		t.Errorf("Message = %q, want both prefix and cause", e.Message)
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return cause")
	}
}

func TestWrapBadRequest(t *testing.T) {
	cause := errors.New("json syntax error")
	e := WrapBadRequest("parse body", cause)
	if e.Status != 400 {
		t.Errorf("Status = %d, want 400", e.Status)
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap should return cause")
	}
}

func TestWrite(t *testing.T) {
	e := NotFound("widget").With("id", "xyz")
	w := httptest.NewRecorder()
	e.Write(w)

	if w.Code != http.StatusNotFound {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusNotFound)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["code"] != CodeNotFound {
		t.Errorf("body.code = %v, want %v", body["code"], CodeNotFound)
	}
}

func TestLogAttrs(t *testing.T) {
	e := BadRequest("bad").With("field", "name")
	attrs := e.LogAttrs()
	// Should contain: "code", val, "message", val, "field", val
	if len(attrs) < 6 {
		t.Errorf("LogAttrs len = %d, want >= 6", len(attrs))
	}
	if attrs[0] != "code" || attrs[1] != CodeBadRequest {
		t.Errorf("first attr pair = (%v, %v), want (code, BAD_REQUEST)", attrs[0], attrs[1])
	}
}
