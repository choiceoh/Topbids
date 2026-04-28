package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/topbid/backend/internal/handler"
	"github.com/choiceoh/topbid/backend/internal/migration"
	"github.com/choiceoh/topbid/backend/internal/schema"
	"github.com/choiceoh/topbid/backend/internal/testutil"
)

// setupSchemaRouter creates a chi router wired to real DB for integration tests.
func setupSchemaRouter(t *testing.T) (*chi.Mux, *migration.Engine, *schema.Cache) {
	t.Helper()
	pool := testutil.SetupDB(t)
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	engine := migration.NewEngine(pool, store, cache)
	h := handler.NewSchemaHandler(pool, store, cache, engine)

	r := chi.NewRouter()
	r.Use(handler.WithRequestID)
	r.Get("/api/schema/collections", h.ListCollections)
	r.Post("/api/schema/collections", h.CreateCollection)
	r.Get("/api/schema/collections/{id}", h.GetCollection)
	r.Patch("/api/schema/collections/{id}", h.UpdateCollection)
	r.Delete("/api/schema/collections/{id}", h.DeleteCollection)
	r.Post("/api/schema/collections/{id}/fields", h.AddField)
	r.Delete("/api/schema/collections/{id}/fields/{fieldId}", h.DeleteField)

	return r, engine, cache
}

func TestSchemaAPI_ListCollections_Empty(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	req := httptest.NewRequest("GET", "/api/schema/collections", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct{ Data []json.RawMessage }
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Data == nil {
		// Collections returns empty slice, not null.
		var env2 struct{ Data json.RawMessage }
		json.Unmarshal(w.Body.Bytes(), &env2)
		if string(env2.Data) != "[]" && string(env2.Data) != "null" {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	}
}

func TestSchemaAPI_CreateAndGetCollection(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	// Create.
	body := `{"slug":"tasks","label":"Tasks","fields":[{"slug":"title","label":"Title","field_type":"text"}]}`
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	var createEnv struct {
		Data struct {
			ID     string        `json:"id"`
			Slug   string        `json:"slug"`
			Fields []interface{} `json:"fields"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &createEnv)
	if createEnv.Data.Slug != "tasks" {
		t.Errorf("slug = %q, want %q", createEnv.Data.Slug, "tasks")
	}
	if len(createEnv.Data.Fields) != 1 {
		t.Errorf("fields = %d, want 1", len(createEnv.Data.Fields))
	}

	// Get.
	req2 := httptest.NewRequest("GET", "/api/schema/collections/"+createEnv.Data.ID, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", w2.Code)
	}
}

func TestSchemaAPI_UpdateCollection(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	// Create.
	body := `{"slug":"docs","label":"Documents"}`
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct{ Data struct{ ID string } }
	json.Unmarshal(w.Body.Bytes(), &env)

	// Update label.
	patchBody := `{"label":"Updated Docs"}`
	req2 := httptest.NewRequest("PATCH", "/api/schema/collections/"+env.Data.ID, bytes.NewBufferString(patchBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body: %s", w2.Code, w2.Body.String())
	}

	var updated struct{ Data struct{ Label string } }
	json.Unmarshal(w2.Body.Bytes(), &updated)
	if updated.Data.Label != "Updated Docs" {
		t.Errorf("label = %q, want %q", updated.Data.Label, "Updated Docs")
	}
}

func TestSchemaAPI_DeleteCollection_WithConfirmation(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	// Create.
	body := `{"slug":"temp","label":"Temp"}`
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct{ Data struct{ ID string } }
	json.Unmarshal(w.Body.Bytes(), &env)

	// Delete without confirm — should return preview.
	req2 := httptest.NewRequest("DELETE", "/api/schema/collections/"+env.Data.ID, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", w2.Code)
	}
	var previewEnv struct {
		Data struct {
			ConfirmationRequired bool `json:"confirmation_required"`
		} `json:"data"`
	}
	json.Unmarshal(w2.Body.Bytes(), &previewEnv)
	if !previewEnv.Data.ConfirmationRequired {
		t.Errorf("expected confirmation_required = true, body: %s", w2.Body.String())
	}

	// Delete with confirm.
	req3 := httptest.NewRequest("DELETE", "/api/schema/collections/"+env.Data.ID+"?confirm=true", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body: %s", w3.Code, w3.Body.String())
	}
}

func TestSchemaAPI_AddField(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	// Create collection.
	body := `{"slug":"orders","label":"Orders"}`
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env struct{ Data struct{ ID string } }
	json.Unmarshal(w.Body.Bytes(), &env)

	// Add field.
	fieldBody := `{"slug":"amount","label":"Amount","field_type":"number"}`
	req2 := httptest.NewRequest("POST", "/api/schema/collections/"+env.Data.ID+"/fields?confirm=true", bytes.NewBufferString(fieldBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusCreated {
		t.Fatalf("add field status = %d, want 201, body: %s", w2.Code, w2.Body.String())
	}

	var fieldEnv struct{ Data struct{ Slug string } }
	json.Unmarshal(w2.Body.Bytes(), &fieldEnv)
	if fieldEnv.Data.Slug != "amount" {
		t.Errorf("slug = %q, want %q", fieldEnv.Data.Slug, "amount")
	}
}

func TestSchemaAPI_InvalidInput(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	// Missing required fields.
	body := `{"slug":"","label":""}`
	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body: %s", w.Code, w.Body.String())
	}
}

func TestSchemaAPI_DuplicateSlug(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	body := `{"slug":"uniq","label":"Unique"}`

	req := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: %d", w.Code)
	}

	// Duplicate.
	req2 := httptest.NewRequest("POST", "/api/schema/collections", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate status = %d, want 409, body: %s", w2.Code, w2.Body.String())
	}
}

func TestSchemaAPI_NotFound(t *testing.T) {
	r, _, _ := setupSchemaRouter(t)

	req := httptest.NewRequest("GET", "/api/schema/collections/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
