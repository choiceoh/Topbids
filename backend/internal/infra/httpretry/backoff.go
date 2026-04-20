package httpretry

import (
	"math"
	"math/rand/v2"
	"time"
)

type Backoff struct {
	Base   time.Duration
	Max    time.Duration
	Jitter float64
}

func (b Backoff) Delay(attempt int) time.Duration {
	delay := time.Duration(float64(b.Base) * math.Pow(2, float64(attempt-1)))
	if delay > b.Max {
		delay = b.Max
	}
	if b.Jitter > 0 && delay > 0 {
		jitter := time.Duration(rand.Int64N(int64(float64(delay) * b.Jitter)))
		delay += jitter
	}
	return delay
}
