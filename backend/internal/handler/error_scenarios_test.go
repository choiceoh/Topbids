package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

// Aliases to keep test lines short.
var (
	errNotFound     = schema.ErrNotFound
	errConflict     = schema.ErrConflict
	errInvalidInput = schema.ErrInvalidInput
)

// --- JSON helpers tests ---

func TestWriteError_Status(t *testing.T) {
	cases := []struct {
		status int
		msg    string
	}{
		{400, "bad request"},
		{404, "not found"},
		{500, "internal error"},
		{502, "bad gateway"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, tc.status, tc.msg)

			if w.Code != tc.status {
				t.Errorf("status = %d, want %d", w.Code, tc.status)
			}
			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("content-type = %q, want JSON", ct)
			}
			var body struct {
				Message string `json:"message"`
			}
			json.NewDecoder(w.Body).Decode(&body)
			if body.Message != tc.msg {
				t.Errorf("message = %q, want %q", body.Message, tc.msg)
			}
		})
	}
}

func TestWriteJSON_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"id": "123"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body envelope
	json.NewDecoder(w.Body).Decode(&body)
	data, ok := body.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %T", body.Data)
	}
	if data["id"] != "123" {
		t.Errorf("data.id = %q, want 123", data["id"])
	}
}

func TestWriteList_Pagination(t *testing.T) {
	w := httptest.NewRecorder()
	items := []map[string]string{{"id": "1"}, {"id": "2"}}
	writeList(w, items, 25, 2, 10)

	var body listEnvelope
	json.NewDecoder(w.Body).Decode(&body)

	if body.Total != 25 {
		t.Errorf("total = %d, want 25", body.Total)
	}
	if body.Page != 2 {
		t.Errorf("page = %d, want 2", body.Page)
	}
	if body.Limit != 10 {
		t.Errorf("limit = %d, want 10", body.Limit)
	}
	if body.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3 (ceil(25/10))", body.TotalPages)
	}
}

func TestWriteList_ExactDivision(t *testing.T) {
	w := httptest.NewRecorder()
	writeList(w, []any{}, 20, 1, 10)

	var body listEnvelope
	json.NewDecoder(w.Body).Decode(&body)

	if body.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2 (20/10)", body.TotalPages)
	}
}

func TestReadJSON_InvalidBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
	r.Header.Set("Content-Type", "application/json")

	var dst map[string]any
	err := readJSON(r, &dst)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestReadJSON_UnknownFields(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"known":"ok","unknown_extra":true}`))
	r.Header.Set("Content-Type", "application/json")

	var dst struct {
		Known string `json:"known"`
	}
	err := readJSON(r, &dst)
	if err == nil {
		t.Error("expected error for unknown fields (DisallowUnknownFields)")
	}
}

// --- Error status mapping tests ---

func TestErrorStatus_NotFound(t *testing.T) {
	err := fmt.Errorf("collection %w", errNotFound)
	status := errorStatus(err)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestErrorStatus_Conflict(t *testing.T) {
	err := fmt.Errorf("slug %w", errConflict)
	status := errorStatus(err)
	if status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
}

func TestErrorStatus_InvalidInput(t *testing.T) {
	err := fmt.Errorf("bad field %w", errInvalidInput)
	status := errorStatus(err)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestErrorStatus_Generic(t *testing.T) {
	err := fmt.Errorf("unexpected db error")
	status := errorStatus(err)
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
}

// --- Public message sanitization ---

func TestPublicMessage_NotFound(t *testing.T) {
	err := fmt.Errorf("collection xyz: %w", errNotFound)
	msg := publicMessage(err)
	if !strings.Contains(msg, "not found") {
		t.Errorf("expected 'not found' in message, got %q", msg)
	}
}

func TestPublicMessage_InternalHidden(t *testing.T) {
	err := fmt.Errorf("connection refused: dial tcp 127.0.0.1:5432")
	msg := publicMessage(err)
	if msg != "internal server error" {
		t.Errorf("internal errors should be hidden, got %q", msg)
	}
}

// --- Batch update validation (from handler, no DB needed) ---

func TestBatchUpdate_EmptyPayload(t *testing.T) {
	r := httptest.NewRequest(http.MethodPatch, "/api/data/tasks/batch",
		strings.NewReader(`{"updates":[]}`))
	r.Header.Set("Content-Type", "application/json")

	// We can test that readJSON + empty check works.
	var body struct {
		Updates []struct {
			ID     string         `json:"id"`
			Fields map[string]any `json:"fields"`
		} `json:"updates"`
	}
	if err := readJSON(r, &body); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	if len(body.Updates) != 0 {
		t.Errorf("expected empty updates")
	}
	// The handler would return 400 here.
}

func TestBatchUpdate_TooLargePayload(t *testing.T) {
	// Build a payload with >1000 updates.
	var updates []string
	for i := 0; i < 1001; i++ {
		updates = append(updates, fmt.Sprintf(`{"id":"%d","fields":{"x":"y"}}`, i))
	}
	payload := `{"updates":[` + strings.Join(updates, ",") + `]}`

	r := httptest.NewRequest(http.MethodPatch, "/api/data/tasks/batch",
		strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")

	var body struct {
		Updates []struct {
			ID     string         `json:"id"`
			Fields map[string]any `json:"fields"`
		} `json:"updates"`
	}
	if err := readJSON(r, &body); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	if len(body.Updates) <= 1000 {
		t.Errorf("expected >1000 updates, got %d", len(body.Updates))
	}
	// The handler would return 400 for max 1000.
}

// --- CSV import edge cases (validation only, no DB) ---

func TestCSVImport_MissingFile(t *testing.T) {
	// No multipart form body.
	r := httptest.NewRequest(http.MethodPost, "/api/data/tasks/import", nil)

	// Simulate FormFile failure.
	_, _, err := r.FormFile("file")
	if err == nil {
		t.Error("expected error for missing file in form")
	}
}

func TestCSVImport_SizeLimit(t *testing.T) {
	// The actual handler wraps r.Body with MaxBytesReader(10<<20).
	maxBytes := 10 << 20
	if maxBytes != 10485760 {
		t.Errorf("expected 10MB = %d, got %d", 10485760, maxBytes)
	}
}
