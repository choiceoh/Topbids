package middleware

import "testing"

func TestCanonicalizePathSimple(t *testing.T) {
	r := CanonicalizePath("/api/users")
	if r.CanonicalPath != "/api/users" {
		t.Errorf("CanonicalPath = %q, want /api/users", r.CanonicalPath)
	}
	if r.DecodePasses != 0 {
		t.Errorf("DecodePasses = %d, want 0", r.DecodePasses)
	}
}

func TestCanonicalizePathEncoded(t *testing.T) {
	r := CanonicalizePath("/api/%75sers")
	if r.CanonicalPath != "/api/users" {
		t.Errorf("CanonicalPath = %q, want /api/users", r.CanonicalPath)
	}
	if r.DecodePasses < 1 {
		t.Errorf("DecodePasses = %d, want >= 1", r.DecodePasses)
	}
}

func TestCanonicalizePathDoubleEncoded(t *testing.T) {
	// %25 → %, then %75 → u
	r := CanonicalizePath("/api/%2575sers")
	if r.CanonicalPath != "/api/users" {
		t.Errorf("CanonicalPath = %q, want /api/users", r.CanonicalPath)
	}
}

func TestCanonicalizePathNormalizesCase(t *testing.T) {
	r := CanonicalizePath("/API/Users")
	if r.CanonicalPath != "/api/users" {
		t.Errorf("CanonicalPath = %q, want /api/users", r.CanonicalPath)
	}
}

func TestCanonicalizePathDoubleSlash(t *testing.T) {
	r := CanonicalizePath("//api///users//")
	if r.CanonicalPath != "/api/users" {
		t.Errorf("CanonicalPath = %q, want /api/users", r.CanonicalPath)
	}
}

func TestCanonicalizePathDotSegments(t *testing.T) {
	r := CanonicalizePath("/api/../admin/users")
	if r.CanonicalPath != "/admin/users" {
		t.Errorf("CanonicalPath = %q, want /admin/users", r.CanonicalPath)
	}
}

func TestIsProtectedPathBasic(t *testing.T) {
	prefixes := []string{"/admin", "/api/internal"}

	cases := []struct {
		path string
		want bool
	}{
		{"/admin", true},
		{"/admin/settings", true},
		{"/ADMIN", true},
		{"/api/internal/health", true},
		{"/api/users", false},
		{"/public", false},
	}
	for _, tc := range cases {
		got := IsProtectedPath(tc.path, prefixes)
		if got != tc.want {
			t.Errorf("IsProtectedPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsProtectedPathEncoded(t *testing.T) {
	prefixes := []string{"/admin"}
	// %61dmin → admin
	if !IsProtectedPath("/%61dmin/secrets", prefixes) {
		t.Error("encoded /admin should be protected")
	}
}

func TestIsProtectedPathEmptyPrefixes(t *testing.T) {
	if IsProtectedPath("/anything", nil) {
		t.Error("empty prefixes should never protect")
	}
}

func TestCanonicalizePathMalformedEncoding(t *testing.T) {
	r := CanonicalizePath("/api/%ZZinvalid")
	if !r.MalformedEncoding {
		t.Error("expected MalformedEncoding = true for %%ZZ")
	}
}
