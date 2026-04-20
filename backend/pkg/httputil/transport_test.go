package httputil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient(5 * time.Second)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.Timeout)
	}
}

func TestSetVersion_UserAgent(t *testing.T) {
	SetVersion("1.2.3")
	defer SetVersion("")

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := NewClient(3 * time.Second)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != "Topworks/1.2.3" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "Topworks/1.2.3")
	}
}

func TestUserAgent_Default(t *testing.T) {
	SetVersion("")

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := NewClient(3 * time.Second)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != "Topworks" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "Topworks")
	}
}

func TestUserAgent_PreserveExisting(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := NewClient(3 * time.Second)
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("User-Agent", "CustomAgent/1.0")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != "CustomAgent/1.0" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "CustomAgent/1.0")
	}
}

func TestWaitForHealth_AlreadyHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForHealth(ctx, srv.URL, 100*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForHealth failed: %v", err)
	}
}

func TestWaitForHealth_BecomesHealthy(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForHealth(ctx, srv.URL, 50*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForHealth failed: %v", err)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 calls, got %d", calls)
	}
}

func TestWaitForHealth_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := WaitForHealth(ctx, srv.URL, 50*time.Millisecond)
	if err == nil {
		t.Error("expected error on context timeout")
	}
}
