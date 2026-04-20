package shortid

import (
	"strings"
	"sync"
	"testing"
)

func TestNewFormat(t *testing.T) {
	// Reset counter for predictable test
	counter.Store(0)

	got := New("app")
	if got != "app_0000" {
		t.Errorf("New(app) = %q, want %q", got, "app_0000")
	}
	got = New("app")
	if got != "app_0001" {
		t.Errorf("second New(app) = %q, want %q", got, "app_0001")
	}
}

func TestNewPrefix(t *testing.T) {
	counter.Store(0)
	got := New("fld")
	if !strings.HasPrefix(got, "fld_") {
		t.Errorf("New(fld) = %q, want prefix fld_", got)
	}
}

func TestNewWrapsAt10000(t *testing.T) {
	counter.Store(9999)
	got := New("x")
	if got != "x_9999" {
		t.Errorf("at 9999: got %q, want x_9999", got)
	}
	got = New("x")
	if got != "x_0000" {
		t.Errorf("at 10000: got %q, want x_0000 (wrap)", got)
	}
}

func TestNewConcurrent(t *testing.T) {
	counter.Store(0)
	const n = 1000
	ids := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = New("c")
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %q", id)
		}
		seen[id] = true
	}
}
