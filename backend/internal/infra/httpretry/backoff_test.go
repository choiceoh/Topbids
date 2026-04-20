package httpretry

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	b := Backoff{Base: 100 * time.Millisecond, Max: 5 * time.Second, Jitter: 0}

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
		{6, 3200 * time.Millisecond},
		{7, 5 * time.Second}, // capped at Max
		{10, 5 * time.Second},
	}
	for _, tc := range cases {
		got := b.Delay(tc.attempt)
		if got != tc.want {
			t.Errorf("Delay(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoffDelayWithJitter(t *testing.T) {
	b := Backoff{Base: 100 * time.Millisecond, Max: 10 * time.Second, Jitter: 0.5}

	for i := 0; i < 100; i++ {
		d := b.Delay(1)
		// With jitter=0.5, delay should be in [100ms, 150ms)
		if d < 100*time.Millisecond || d >= 150*time.Millisecond {
			t.Errorf("Delay(1) with jitter=0.5 = %v, want [100ms, 150ms)", d)
		}
	}
}

func TestBackoffDelayZeroBase(t *testing.T) {
	b := Backoff{Base: 0, Max: time.Second, Jitter: 0.5}
	got := b.Delay(1)
	if got != 0 {
		t.Errorf("Delay(1) with zero base = %v, want 0", got)
	}
}
