package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Admin@Example.COM", "admin@example.com"},
		{"  user@test.com  ", "user@test.com"},
		{"UPPER@CASE.KR", "upper@case.kr"},
		{"", ""},
		{"  ", ""},
	}
	for _, tt := range tests {
		got := normalizeEmail(tt.input)
		if got != tt.want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single IP",
			xff:        "1.2.3.4",
			remoteAddr: "127.0.0.1:9999",
			want:       "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			xff:        "10.0.0.1, 10.0.0.2, 10.0.0.3",
			remoteAddr: "127.0.0.1:9999",
			want:       "10.0.0.1",
		},
		{
			name:       "no XFF uses RemoteAddr",
			xff:        "",
			remoteAddr: "192.168.1.100:12345",
			want:       "192.168.1.100",
		},
		{
			name:       "no XFF no port",
			xff:        "",
			remoteAddr: "192.168.1.100",
			want:       "192.168.1.100",
		},
		{
			name:       "XFF with leading spaces",
			xff:        "  10.0.0.1 ",
			remoteAddr: "127.0.0.1:9999",
			want:       "10.0.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			got := clientIP(r)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {

	deptID := "dept-1"
	subID := "sub-1"
	user := User{
		ID:           "user-123",
		Email:        "test@example.com",
		Name:         "Test User",
		Role:         RoleDirector,
		DepartmentID: &deptID,
		SubsidiaryID: &subID,
	}

	tokenStr, err := generateToken(user, "test-secret-key")
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	if tokenStr == "" {
		t.Fatal("generateToken() returned empty string")
	}

	// Parse and verify claims.
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte("test-secret-key"), nil
	})
	if err != nil {
		t.Fatalf("jwt.Parse() error = %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("could not cast claims")
	}

	if claims["userId"] != "user-123" {
		t.Errorf("userId = %v, want user-123", claims["userId"])
	}
	if claims["email"] != "test@example.com" {
		t.Errorf("email = %v, want test@example.com", claims["email"])
	}
	if claims["name"] != "Test User" {
		t.Errorf("name = %v, want Test User", claims["name"])
	}
	if claims["role"] != RoleDirector {
		t.Errorf("role = %v, want %s", claims["role"], RoleDirector)
	}
	if claims["departmentId"] != "dept-1" {
		t.Errorf("departmentId = %v, want dept-1", claims["departmentId"])
	}
	if claims["subsidiaryId"] != "sub-1" {
		t.Errorf("subsidiaryId = %v, want sub-1", claims["subsidiaryId"])
	}
	if _, ok := claims["exp"]; !ok {
		t.Error("missing exp claim")
	}
}

func TestGenerateToken_NoDepartment(t *testing.T) {

	user := User{
		ID:    "user-456",
		Email: "nodept@example.com",
		Name:  "No Dept",
		Role:  RoleViewer,
	}

	tokenStr, err := generateToken(user, "test-secret-key")
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}

	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte("test-secret-key"), nil
	})
	claims := token.Claims.(jwt.MapClaims)

	if _, ok := claims["departmentId"]; ok {
		t.Error("departmentId should not be present when nil")
	}
	if _, ok := claims["subsidiaryId"]; ok {
		t.Error("subsidiaryId should not be present when nil")
	}
}

func TestGenerateToken_ExplicitSecret(t *testing.T) {
	secret := "my-custom-secret"
	user := User{ID: "u1", Email: "a@b.com", Name: "A", Role: RoleViewer}
	tokenStr, err := generateToken(user, secret)
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}

	// Should be parseable with the same secret.
	_, err = jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Errorf("token should be valid with provided secret: %v", err)
	}
}

func TestRoleConstants(t *testing.T) {
	roles := []string{RoleDirector, RolePM, RoleEngineer, RoleViewer}
	for _, r := range roles {
		if r == "" {
			t.Error("role constant should not be empty")
		}
		if r != strings.ToLower(r) {
			t.Errorf("role %q should be lowercase", r)
		}
	}
}
