package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/events"
	"github.com/choiceoh/topbid/backend/internal/handler"
	"github.com/choiceoh/topbid/backend/internal/middleware"
	"github.com/choiceoh/topbid/backend/internal/migration"
	"github.com/choiceoh/topbid/backend/internal/schema"
	"github.com/choiceoh/topbid/backend/internal/testutil"
)

// setupDynRouter creates a router with schema + dynamic data endpoints.
// Returns the router, migration engine, and schema cache for setting up test data.
func setupDynRouter(t *testing.T) (*chi.Mux, *migration.Engine, *schema.Cache) {
	t.Helper()
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	engine := migration.NewEngine(pool, store, cache)
	bus := events.NewBus()
	dyn := handler.NewDynHandler(pool, cache, bus)
	sh := handler.NewSchemaHandler(pool, store, cache, engine)

	r := chi.NewRouter()
	r.Use(handler.WithRequestID)
	// Schema endpoints (for setup).
	r.Post("/api/schema/collections", sh.CreateCollection)
	r.Post("/api/schema/collections/{id}/fields", sh.AddField)
	// Data endpoints.
	r.Get("/api/data/{slug}", dyn.List)
	r.Post("/api/data/{slug}", dyn.Create)
	r.Get("/api/data/{slug}/{id}", dyn.Get)
	r.Patch("/api/data/{slug}/{id}", dyn.Update)
	r.Delete("/api/data/{slug}/{id}", dyn.Delete)

	return r, engine, cache
}

// createTestCollection creates a collection via the schema API and returns its ID and slug.
func createTestCollection(t *testing.T, router *chi.Mux, slug, label string) (id string) {
	t.Helper()
	body := fmt.Sprintf(`{"slug":%q,"label":%q}`, slug, label)
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: "00000000-0000-0000-0000-000000000001", Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create collection %q: status = %d, body = %s", slug, w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Data.ID
}

// addTestField adds a field to a collection and returns the field ID.
func addTestField(t *testing.T, router *chi.Mux, collectionID, slug, label, fieldType string) string {
	t.Helper()
	body := fmt.Sprintf(`{"slug":%q,"label":%q,"field_type":%q}`, slug, label, fieldType)
	url := fmt.Sprintf("/api/schema/collections/%s/fields", collectionID)
	req := httptest.NewRequest("POST", url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: "00000000-0000-0000-0000-000000000001", Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("add field %q: status = %d, body = %s", slug, w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Data.ID
}

var testClaims = middleware.UserClaims{
	UserID: "00000000-0000-0000-0000-000000000001",
	Email:  "test@test.com",
	Name:   "Tester",
	Role:   "director",
}

func TestDynIntegration_CRUD(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	// 1. Create a collection with fields.
	colID := createTestCollection(t, router, "tasks", "앱")
	addTestField(t, router, colID, "title", "제목", "text")
	addTestField(t, router, colID, "count", "수량", "number")

	// 2. List — should be empty.
	{
		req := httptest.NewRequest("GET", "/api/data/tasks", nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("list empty: status = %d, body = %s", w.Code, w.Body.String())
		}
		var env struct {
			Total int64 `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		if env.Total != 0 {
			t.Errorf("expected total=0, got %d", env.Total)
		}
	}

	// 3. Create a record.
	var recordID string
	{
		body := `{"title":"보고서 작성","count":42}`
		req := httptest.NewRequest("POST", "/api/data/tasks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create: status = %d, body = %s", w.Code, w.Body.String())
		}

		var env struct {
			Data struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		recordID = env.Data.ID
		if recordID == "" {
			t.Fatal("expected record ID")
		}
		if env.Data.Title != "보고서 작성" {
			t.Errorf("title = %q, want 보고서 작성", env.Data.Title)
		}
	}

	// 4. Get the record.
	{
		req := httptest.NewRequest("GET", "/api/data/tasks/"+recordID, nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("get: status = %d, body = %s", w.Code, w.Body.String())
		}
		var env struct {
			Data struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		if env.Data.ID != recordID {
			t.Errorf("id = %q, want %q", env.Data.ID, recordID)
		}
	}

	// 5. Update the record.
	{
		body := `{"title":"보고서 수정"}`
		req := httptest.NewRequest("PATCH", "/api/data/tasks/"+recordID, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("update: status = %d, body = %s", w.Code, w.Body.String())
		}
	}

	// 6. Verify update via Get.
	{
		req := httptest.NewRequest("GET", "/api/data/tasks/"+recordID, nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var env struct {
			Data struct {
				Title string `json:"title"`
			} `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		if env.Data.Title != "보고서 수정" {
			t.Errorf("updated title = %q, want 보고서 수정", env.Data.Title)
		}
	}

	// 7. List — should have 1 record.
	{
		req := httptest.NewRequest("GET", "/api/data/tasks", nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var env struct {
			Total int64 `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		if env.Total != 1 {
			t.Errorf("expected total=1, got %d", env.Total)
		}
	}

	// 8. Delete the record.
	{
		req := httptest.NewRequest("DELETE", "/api/data/tasks/"+recordID, nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("delete: status = %d, body = %s", w.Code, w.Body.String())
		}
	}

	// 9. List — total should be 0 (soft deleted).
	{
		req := httptest.NewRequest("GET", "/api/data/tasks", nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var env struct {
			Total int64 `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &env)
		if env.Total != 0 {
			t.Errorf("expected total=0 after delete, got %d", env.Total)
		}
	}

	// 10. Get deleted record — should be 404.
	{
		req := httptest.NewRequest("GET", "/api/data/tasks/"+recordID, nil)
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("get deleted: status = %d, want 404", w.Code)
		}
	}
}

func TestDynIntegration_List_Pagination(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	colID := createTestCollection(t, router, "pagtest", "페이지네이션")
	addTestField(t, router, colID, "name", "이름", "text")

	// Create 5 records.
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"name":"Record %d"}`, i)
		req := httptest.NewRequest("POST", "/api/data/pagtest", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d: status = %d", i, w.Code)
		}
	}

	// List with limit=2.
	req := httptest.NewRequest("GET", "/api/data/pagtest?limit=2&page=1", nil)
	req = injectUser(req, testClaims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d", w.Code)
	}

	var env struct {
		Data       []map[string]any `json:"data"`
		Total      int64            `json:"total"`
		Page       int              `json:"page"`
		Limit      int              `json:"limit"`
		TotalPages int              `json:"total_pages"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)

	if env.Total != 5 {
		t.Errorf("total = %d, want 5", env.Total)
	}
	if len(env.Data) != 2 {
		t.Errorf("data length = %d, want 2", len(env.Data))
	}
	if env.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", env.TotalPages)
	}
}

func TestDynIntegration_Create_InvalidJSON(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	colID := createTestCollection(t, router, "badjson", "BadJSON")
	addTestField(t, router, colID, "x", "X", "text")

	req := httptest.NewRequest("POST", "/api/data/badjson", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, testClaims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestDynIntegration_Get_NotFound(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	createTestCollection(t, router, "nftest", "NotFound")

	req := httptest.NewRequest("GET", "/api/data/nftest/00000000-0000-0000-0000-ffffffffffff", nil)
	req = injectUser(req, testClaims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestDynIntegration_CollectionNotFound(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	req := httptest.NewRequest("GET", "/api/data/nonexistent", nil)
	req = injectUser(req, testClaims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestDynIntegration_List_Search(t *testing.T) {
	router, _, _ := setupDynRouter(t)

	colID := createTestCollection(t, router, "srchtest", "검색")
	addTestField(t, router, colID, "title", "제목", "text")

	// Create records.
	for _, title := range []string{"사과", "바나나", "사과주스"} {
		body := fmt.Sprintf(`{"title":%q}`, title)
		req := httptest.NewRequest("POST", "/api/data/srchtest", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = injectUser(req, testClaims)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %q: status = %d", title, w.Code)
		}
	}

	// Search for "사과".
	req := httptest.NewRequest("GET", "/api/data/srchtest?q=사과", nil)
	req = injectUser(req, testClaims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var env struct {
		Total int64 `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Total != 2 {
		t.Errorf("search total = %d, want 2 (사과, 사과주스)", env.Total)
	}
}
