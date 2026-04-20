package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/choiceoh/phaeton/backend/internal/handler"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/testutil"
)

// setupAuthRouter creates a chi router with auth endpoints wired to the test DB.
func setupAuthRouter(t *testing.T) *chi.Mux {
	t.Helper()
	pool := testutil.SetupDB(t)

	r := chi.NewRouter()
	r.Use(handler.WithRequestID)
	r.Post("/api/auth/login", handler.Login(pool, nil, "test-secret"))
	r.Post("/api/auth/logout", handler.Logout())
	r.Get("/api/auth/me", handler.Me(pool))
	r.Post("/api/users", handler.CreateUser(pool))
	r.Get("/api/users", handler.ListUsers(pool))
	r.Get("/api/users/{id}", handler.GetUser(pool))
	r.Patch("/api/users/{id}", handler.UpdateUser(pool))
	r.Patch("/api/auth/me", handler.UpdateMe(pool))
	r.Post("/api/auth/password", handler.ChangePassword(pool))
	return r
}

// injectUser adds a UserClaims to the request context for handlers that require auth.
func injectUser(r *http.Request, claims middleware.UserClaims) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ExportedUserContextKey, claims)
	return r.WithContext(ctx)
}

// seedDirector creates a director user directly via the handler and returns the user ID.
func seedDirector(t *testing.T, router *chi.Mux) string {
	t.Helper()
	pool := testutil.SetupDB(t)
	if err := handler.SeedDirector(context.Background(), pool); err != nil {
		t.Fatalf("SeedDirector: %v", err)
	}

	// Login as the seeded director to get user info.
	body := `{"email":"choiceoh@topsolar.kr","password":"135792ch"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("seed login status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Data.User.ID
}

func TestAuthIntegration_LoginSuccess(t *testing.T) {
	router := setupAuthRouter(t)
	seedDirector(t, router)

	body := `{"email":"choiceoh@topsolar.kr","password":"135792ch"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				Email string `json:"email"`
				Role  string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Data.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Data.User.Role != "director" {
		t.Errorf("role = %q, want director", resp.Data.User.Role)
	}

	// Check cookie is set.
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" {
			found = true
			if c.Value == "" {
				t.Error("token cookie should not be empty")
			}
		}
	}
	if !found {
		t.Error("expected token cookie to be set")
	}
}

func TestAuthIntegration_LoginInvalidPassword(t *testing.T) {
	router := setupAuthRouter(t)
	seedDirector(t, router)

	body := `{"email":"choiceoh@topsolar.kr","password":"wrongpassword"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthIntegration_LoginNonExistentUser(t *testing.T) {
	router := setupAuthRouter(t)

	body := `{"email":"nobody@test.com","password":"anything"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthIntegration_LoginInvalidJSON(t *testing.T) {
	router := setupAuthRouter(t)

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAuthIntegration_Logout(t *testing.T) {
	router := setupAuthRouter(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Token cookie should be cleared (MaxAge = -1).
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "token" {
			if c.MaxAge != -1 {
				t.Errorf("token cookie MaxAge = %d, want -1", c.MaxAge)
			}
		}
	}
}

func TestAuthIntegration_CreateUser(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	body := `{"email":"newuser@test.com","name":"New User","password":"password123","role":"engineer"}`
	req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{
		UserID: directorID,
		Email:  "choiceoh@topsolar.kr",
		Name:   "Director",
		Role:   "director",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.ID == "" {
		t.Error("expected user ID in response")
	}

	// Verify login with new user.
	loginBody := `{"email":"newuser@test.com","password":"password123"}`
	loginReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	router.ServeHTTP(lw, loginReq)

	if lw.Code != http.StatusOK {
		t.Fatalf("new user login status = %d, want 200", lw.Code)
	}
}

func TestAuthIntegration_CreateUser_NonDirectorForbidden(t *testing.T) {
	router := setupAuthRouter(t)

	body := `{"email":"x@test.com","name":"X","password":"123456","role":"viewer"}`
	req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{
		UserID: "eng-1",
		Role:   "engineer",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAuthIntegration_CreateUser_DuplicateEmail(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	claims := middleware.UserClaims{UserID: directorID, Role: "director"}

	body := `{"email":"dup@test.com","name":"First","password":"123456","role":"viewer"}`
	req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, claims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", w.Code, w.Body.String())
	}

	// Duplicate.
	req2 := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2 = injectUser(req2, claims)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", w2.Code)
	}
}

func TestAuthIntegration_CreateUser_InvalidRole(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	body := `{"email":"bad@test.com","name":"Bad","password":"123456","role":"admin"}`
	req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: directorID, Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAuthIntegration_CreateUser_MissingFields(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)
	claims := middleware.UserClaims{UserID: directorID, Role: "director"}

	// Missing email.
	body := `{"name":"NoEmail","password":"123456","role":"viewer"}`
	req := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, claims)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing email: status = %d, want 400", w.Code)
	}
}

func TestAuthIntegration_ChangePassword(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	// Change password.
	body := `{"current_password":"135792ch","new_password":"newpassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{
		UserID: directorID,
		Email:  "choiceoh@topsolar.kr",
		Role:   "director",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("change password status = %d, body = %s", w.Code, w.Body.String())
	}

	// Old password should no longer work.
	loginBody := `{"email":"choiceoh@topsolar.kr","password":"135792ch"}`
	loginReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	router.ServeHTTP(lw, loginReq)
	if lw.Code != http.StatusUnauthorized {
		t.Errorf("old password should fail: status = %d", lw.Code)
	}

	// New password should work.
	loginBody2 := `{"email":"choiceoh@topsolar.kr","password":"newpassword123"}`
	loginReq2 := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody2))
	loginReq2.Header.Set("Content-Type", "application/json")
	lw2 := httptest.NewRecorder()
	router.ServeHTTP(lw2, loginReq2)
	if lw2.Code != http.StatusOK {
		t.Errorf("new password should work: status = %d", lw2.Code)
	}
}

func TestAuthIntegration_ChangePassword_WrongCurrent(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	body := `{"current_password":"wrongwrong","new_password":"newpassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: directorID, Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAuthIntegration_ChangePassword_TooShort(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	body := `{"current_password":"135792ch","new_password":"12345"}`
	req := httptest.NewRequest("POST", "/api/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: directorID, Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAuthIntegration_ListUsers(t *testing.T) {
	router := setupAuthRouter(t)
	seedDirector(t, router)

	req := httptest.NewRequest("GET", "/api/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) == 0 {
		t.Error("expected at least one user after seeding")
	}
}

func TestAuthIntegration_GetUser_NotFound(t *testing.T) {
	router := setupAuthRouter(t)

	req := httptest.NewRequest("GET", "/api/users/00000000-0000-0000-0000-ffffffffffff", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestAuthIntegration_UpdateUser(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)
	claims := middleware.UserClaims{UserID: directorID, Role: "director"}

	// Create a user to update.
	createBody := `{"email":"update@test.com","name":"Before","password":"123456","role":"viewer"}`
	createReq := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = injectUser(createReq, claims)
	cw := httptest.NewRecorder()
	router.ServeHTTP(cw, createReq)

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(cw.Body.Bytes(), &created)
	userID := created.Data.ID

	// Update name and role.
	updateBody := `{"name":"After","role":"pm"}`
	updateReq := httptest.NewRequest("PATCH", "/api/users/"+userID, bytes.NewBufferString(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq = injectUser(updateReq, claims)
	uw := httptest.NewRecorder()
	router.ServeHTTP(uw, updateReq)

	if uw.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", uw.Code, uw.Body.String())
	}

	// Verify via GetUser.
	getReq := httptest.NewRequest("GET", "/api/users/"+userID, nil)
	gw := httptest.NewRecorder()
	router.ServeHTTP(gw, getReq)

	var user struct {
		Data struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"data"`
	}
	json.Unmarshal(gw.Body.Bytes(), &user)
	if user.Data.Name != "After" {
		t.Errorf("name = %q, want After", user.Data.Name)
	}
	if user.Data.Role != "pm" {
		t.Errorf("role = %q, want pm", user.Data.Role)
	}
}

func TestAuthIntegration_UpdateUser_NoFields(t *testing.T) {
	router := setupAuthRouter(t)
	directorID := seedDirector(t, router)

	req := httptest.NewRequest("PATCH", "/api/users/"+directorID, bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = injectUser(req, middleware.UserClaims{UserID: directorID, Role: "director"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no fields to update)", w.Code)
	}
}

func TestAuthIntegration_Me_Unauthenticated(t *testing.T) {
	router := setupAuthRouter(t)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
