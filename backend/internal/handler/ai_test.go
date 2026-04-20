package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- extractJSON tests ---

func TestExtractJSON_Plain(t *testing.T) {
	input := `{"slug":"tasks","label":"앱"}`
	got := extractJSON(input)
	if got != input {
		t.Errorf("expected plain JSON returned as-is, got %q", got)
	}
}

func TestExtractJSON_MarkdownFences(t *testing.T) {
	input := "```json\n{\"slug\":\"test\"}\n```"
	got := extractJSON(input)
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("failed to parse extracted JSON: %v (got %q)", err, got)
	}
	if obj["slug"] != "test" {
		t.Errorf("slug = %q, want %q", obj["slug"], "test")
	}
}

func TestExtractJSON_ExtraText(t *testing.T) {
	input := "Here is the result:\n{\"slug\":\"demo\"}\nHope this helps!"
	got := extractJSON(input)
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("failed to parse: %v (got %q)", err, got)
	}
	if obj["slug"] != "demo" {
		t.Errorf("slug = %q, want %q", obj["slug"], "demo")
	}
}

func TestExtractJSON_NestedObjects(t *testing.T) {
	input := `{"slug":"test","fields":[{"slug":"f1","options":{"choices":["a","b"]}}]}`
	got := extractJSON(input)
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("failed to parse nested: %v", err)
	}
	fields, ok := obj["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Errorf("expected 1 field, got %v", obj["fields"])
	}
}

// --- sanitizeSlug tests ---

func TestSanitizeSlug_Korean(t *testing.T) {
	got := sanitizeSlug("앱_관리")
	// Korean chars are replaced; should start with letter
	if len(got) == 0 || got[0] < 'a' || got[0] > 'z' {
		t.Errorf("slug should start with letter, got %q", got)
	}
}

func TestSanitizeSlug_UpperCase(t *testing.T) {
	got := sanitizeSlug("ProjectManagement")
	if got != "projectmanagement" {
		t.Errorf("got %q, want lowercase", got)
	}
}

func TestSanitizeSlug_SpecialChars(t *testing.T) {
	got := sanitizeSlug("my-task!@#")
	if strings.ContainsAny(got, "-!@#") {
		t.Errorf("slug should not contain special chars, got %q", got)
	}
}

func TestSanitizeSlug_NumberPrefix(t *testing.T) {
	got := sanitizeSlug("123tasks")
	if got[0] < 'a' || got[0] > 'z' {
		t.Errorf("slug starting with number should be prefixed, got %q", got)
	}
}

func TestSanitizeSlug_MaxLength(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := sanitizeSlug(long)
	if len(got) > 63 {
		t.Errorf("slug too long: %d chars", len(got))
	}
}

// --- parseAndSanitize tests ---

func TestParseAndSanitize_ValidJSON(t *testing.T) {
	raw := `{"slug":"Test-App","label":"테스트","description":"desc","fields":[{"slug":"Field1","label":"필드","field_type":"text","is_required":true}]}`
	result, err := parseAndSanitize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Slug != "test_app" {
		t.Errorf("slug not sanitized: %q", result.Slug)
	}
	if result.Fields[0].Slug != "field1" {
		t.Errorf("field slug not sanitized: %q", result.Fields[0].Slug)
	}
	if result.Fields[0].Width != 6 {
		t.Errorf("default width not set: %d", result.Fields[0].Width)
	}
	if result.Fields[0].Height != 1 {
		t.Errorf("default height not set: %d", result.Fields[0].Height)
	}
}

func TestParseAndSanitize_InvalidJSON(t *testing.T) {
	_, err := parseAndSanitize("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseAndSanitize_MarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"slug\":\"demo\",\"label\":\"데모\",\"fields\":[]}\n```"
	result, err := parseAndSanitize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Slug != "demo" {
		t.Errorf("slug = %q, want demo", result.Slug)
	}
}

// --- parseTriageResponse tests ---

func TestParseTriageResponse_Proceed(t *testing.T) {
	raw := `{"mode":"proceed"}`
	questions, ok := parseTriageResponse(raw)
	if ok || len(questions) > 0 {
		t.Error("expected no questions for proceed mode")
	}
}

func TestParseTriageResponse_Questions(t *testing.T) {
	raw := `{"mode":"questions","questions":[{"id":"q1","question":"어떤 종류?","choices":["A","B"]}]}`
	questions, ok := parseTriageResponse(raw)
	if !ok {
		t.Fatal("expected ok=true for questions mode")
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}
	if questions[0].ID != "q1" {
		t.Errorf("question id = %q, want q1", questions[0].ID)
	}
}

func TestParseTriageResponse_InvalidJSON(t *testing.T) {
	questions, ok := parseTriageResponse("broken")
	if ok || len(questions) > 0 {
		t.Error("expected no result for invalid JSON")
	}
}

// --- AI handler HTTP tests (error scenarios) ---
// These test the handlers at the HTTP level using httptest,
// with a mock vLLM backend that returns errors.

func TestBuildCollection_MissingDescription(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-collection",
		strings.NewReader(`{"description":""}`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.BuildCollection(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body struct {
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.Message != "description is required" {
		t.Errorf("message = %q", body.Message)
	}
}

func TestBuildCollection_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-collection",
		strings.NewReader(`not json`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.BuildCollection(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGenerateSlug_MissingLabel(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/generate-slug",
		strings.NewReader(`{"label":""}`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.GenerateSlug(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGenerateSlug_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/generate-slug",
		strings.NewReader(`{bad}`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.GenerateSlug(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestChat_MissingMessage(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/chat",
		strings.NewReader(`{"message":""}`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.Chat(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestChat_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/chat",
		strings.NewReader(`not-json`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.Chat(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestBuildCollection_VLLMDown tests the scenario where the vLLM server is unreachable.
// We spin up a mock HTTP server that always returns 503.
func TestBuildCollection_VLLMDown(t *testing.T) {
	// Start a mock vLLM server that returns 503.
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"service unavailable"}`)
	}))
	defer vllm.Close()

	// Create an AI client pointing to the mock server.
	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-collection",
		strings.NewReader(`{"description":"앱 관리","answers":{"q1":"answer"}}`))
	r.Header.Set("Content-Type", "application/json")

	h.BuildCollection(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d (502 Bad Gateway)", w.Code, http.StatusBadGateway)
	}
	var body struct {
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body.Message, "AI 서버") {
		t.Errorf("message should mention AI server, got %q", body.Message)
	}
}

func TestGenerateSlug_VLLMDown(t *testing.T) {
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer vllm.Close()

	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/generate-slug",
		strings.NewReader(`{"label":"프로젝트 관리"}`))
	r.Header.Set("Content-Type", "application/json")

	h.GenerateSlug(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

// Note: TestChat_VLLMDown requires a real store (for tool resolver).
// Chat handler validation (missing message, invalid JSON) is tested above.

// TestBuildCollection_VLLMReturnsGarbage tests when vLLM returns invalid JSON as schema.
func TestBuildCollection_VLLMReturnsGarbage(t *testing.T) {
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "test-model"}},
			})
			return
		}
		// Return a valid chat completion response but with garbage content.
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "this is not json at all, just random text"}},
			},
		})
	}))
	defer vllm.Close()

	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-collection",
		strings.NewReader(`{"description":"테스트","answers":{"q1":"a"}}`))
	r.Header.Set("Content-Type", "application/json")

	h.BuildCollection(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 for unparseable AI response", w.Code)
	}
	var body struct {
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body.Message, "파싱") {
		t.Errorf("message should mention parsing, got %q", body.Message)
	}
}

// TestBuildCollection_VLLMValidResponse tests the successful path.
func TestBuildCollection_VLLMValidResponse(t *testing.T) {
	schema := `{"slug":"test_app","label":"테스트","description":"테스트 앱","fields":[{"slug":"title","label":"제목","field_type":"text","is_required":true,"width":6,"height":1}]}`

	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "test-model"}},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": schema}},
			},
		})
	}))
	defer vllm.Close()

	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-collection",
		strings.NewReader(`{"description":"테스트 앱 만들기","answers":{"q1":"a"}}`))
	r.Header.Set("Content-Type", "application/json")

	h.BuildCollection(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body envelope
	json.NewDecoder(w.Body).Decode(&body)
	if body.Data == nil {
		t.Error("expected data in response")
	}
}

// TestHealthCheck tests the AI health check endpoint.
func TestHealthCheck_VLLMDown(t *testing.T) {
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer vllm.Close()

	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ai/health", nil)

	h.HealthCheck(w, r)

	var result map[string]bool
	json.NewDecoder(w.Body).Decode(&result)
	if result["available"] != false {
		t.Error("expected available=false when vLLM is down")
	}
}

func TestHealthCheck_VLLMHealthy(t *testing.T) {
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "test-model"}},
		})
	}))
	defer vllm.Close()

	client := newTestAIClient(vllm.URL)

	h := &AIHandler{client: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ai/health", nil)

	h.HealthCheck(w, r)

	var result map[string]bool
	json.NewDecoder(w.Body).Decode(&result)
	if result["available"] != true {
		t.Error("expected available=true when vLLM is healthy")
	}
}

// TestBuildAutomation_MissingDescription tests missing description error.
func TestBuildAutomation_MissingDescription(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ai/build-automation",
		strings.NewReader(`{"description":""}`))
	r.Header.Set("Content-Type", "application/json")

	h := &AIHandler{}
	h.BuildAutomation(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// Note: TestBuildAutomation_VLLMDown requires a real store (for tool resolver).
// BuildAutomation validation (missing description) is tested above.
