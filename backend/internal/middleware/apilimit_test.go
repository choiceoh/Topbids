package middleware

import (
	"testing"
	"time"
)

func TestAPILimiterAllow(t *testing.T) {
	// 10 requests/sec, burst of 5.
	al := NewAPILimiter(10, 5)
	defer al.Close()

	// First 5 requests should be allowed (burst).
	for i := 0; i < 5; i++ {
		allowed, _ := al.Allow("user:test")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied (burst exhausted, no time to refill).
	allowed, retryAfter := al.Allow("user:test")
	if allowed {
		t.Fatal("6th request should be denied")
	}
	if retryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}
}

func TestAPILimiterRefill(t *testing.T) {
	// 100 requests/sec, burst of 2 — fast refill for testing.
	al := NewAPILimiter(100, 2)
	defer al.Close()

	// Exhaust burst.
	al.Allow("user:refill")
	al.Allow("user:refill")

	// Should be denied.
	allowed, _ := al.Allow("user:refill")
	if allowed {
		t.Fatal("should be denied after burst exhausted")
	}

	// Wait for refill.
	time.Sleep(30 * time.Millisecond)

	// Should be allowed again.
	allowed, _ = al.Allow("user:refill")
	if !allowed {
		t.Fatal("should be allowed after refill")
	}
}

func TestAPILimiterDifferentKeys(t *testing.T) {
	al := NewAPILimiter(10, 2)
	defer al.Close()

	// Exhaust user A.
	al.Allow("a")
	al.Allow("a")
	allowed, _ := al.Allow("a")
	if allowed {
		t.Fatal("user A should be denied")
	}

	// User B should still be allowed.
	allowed, _ = al.Allow("b")
	if !allowed {
		t.Fatal("user B should be allowed")
	}
}
