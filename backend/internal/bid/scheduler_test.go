package bid

import (
	"testing"
	"time"
)

func TestNewScheduler_DefaultInterval(t *testing.T) {
	s := NewScheduler(nil, nil, nil, 0)
	if s.interval != 30*time.Second {
		t.Errorf("default interval = %v, want 30s", s.interval)
	}
	s2 := NewScheduler(nil, nil, nil, 5*time.Second)
	if s2.interval != 5*time.Second {
		t.Errorf("custom interval = %v, want 5s", s2.interval)
	}
}

func TestScheduler_StopIdempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, time.Second)
	// Stop before start is a no-op.
	s.Stop()
	// Stop twice is safe.
	s.Stop()
}

func TestRFQStatusConstants(t *testing.T) {
	// Keep these in sync with the rfqs preset choices. The scheduler only
	// transitions into these three values; a drift here would silently
	// break RFQ lifecycle.
	cases := map[string]string{
		RFQStatusPublished: "published",
		RFQStatusClosed:    "closed",
		RFQStatusOpened:    "opened",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("status %q drifted from %q", got, want)
		}
	}
}
