package middleware

import "testing"

func TestRateLimiterAllowsInitial(t *testing.T) {
	rl := NewRateLimiter(5, 60000, 30000)
	defer rl.Close()

	allowed, _ := rl.Check("1.2.3.4")
	if !allowed {
		t.Error("first check should be allowed")
	}
}

func TestRateLimiterLocksOutAfterMaxFailures(t *testing.T) {
	rl := NewRateLimiter(3, 60000, 30000)
	defer rl.Close()

	ip := "10.0.0.1"
	for range 3 {
		rl.RecordFailure(ip)
	}

	allowed, retryAfter := rl.Check(ip)
	if allowed {
		t.Error("should be locked out after 3 failures")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %d, want > 0", retryAfter)
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := NewRateLimiter(3, 60000, 30000)
	defer rl.Close()

	ip := "10.0.0.2"
	for range 3 {
		rl.RecordFailure(ip)
	}
	rl.Reset(ip)

	allowed, _ := rl.Check(ip)
	if !allowed {
		t.Error("should be allowed after reset")
	}
}

func TestRateLimiterBelowThreshold(t *testing.T) {
	rl := NewRateLimiter(5, 60000, 30000)
	defer rl.Close()

	ip := "10.0.0.3"
	for range 4 {
		rl.RecordFailure(ip)
	}

	allowed, _ := rl.Check(ip)
	if !allowed {
		t.Error("should still be allowed below max failures")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, 60000, 30000)
	defer rl.Close()

	for range 2 {
		rl.RecordFailure("ip_a")
	}

	allowed, _ := rl.Check("ip_b")
	if !allowed {
		t.Error("different IP should not be affected")
	}
}

func TestRateLimiterMaxEntries(t *testing.T) {
	rl := NewRateLimiter(3, 60000, 30000)
	defer rl.Close()

	// Fill up to max entries
	for i := range maxRateLimitEntries {
		rl.RecordFailure(string(rune('A'+i%26)) + string(rune('0'+i/26)))
	}

	// New IP should be silently dropped (no panic)
	rl.RecordFailure("new_ip")
	allowed, _ := rl.Check("new_ip")
	if !allowed {
		t.Error("new IP beyond limit should still be allowed (failure was dropped)")
	}
}

func TestRateLimiterCloseIdempotent(t *testing.T) {
	rl := NewRateLimiter(3, 60000, 30000)
	rl.Close()
	rl.Close() // should not panic
}
