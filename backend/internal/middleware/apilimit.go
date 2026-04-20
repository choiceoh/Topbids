package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/choiceoh/phaeton/backend/internal/infra/apierr"
)

// APILimiter is a per-user token-bucket rate limiter for API requests.
type APILimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   int     // max tokens
	stopCh  chan struct{}
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

const maxAPILimitEntries = 50000

// NewAPILimiter creates a rate limiter that allows `rate` requests per second
// with a burst capacity of `burst`.
func NewAPILimiter(rate float64, burst int) *APILimiter {
	al := &APILimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		stopCh:  make(chan struct{}),
	}
	go al.gcLoop()
	return al
}

// Close stops the background GC goroutine.
func (al *APILimiter) Close() {
	select {
	case <-al.stopCh:
	default:
		close(al.stopCh)
	}
}

func (al *APILimiter) gcLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-al.stopCh:
			return
		case <-ticker.C:
			al.mu.Lock()
			now := time.Now()
			for key, b := range al.buckets {
				if now.Sub(b.lastSeen) > 5*time.Minute {
					delete(al.buckets, key)
				}
			}
			al.mu.Unlock()
		}
	}
}

// Allow checks whether the given key (user ID or IP) is allowed to make a request.
// Returns true if allowed, false with retry-after duration if rate limited.
func (al *APILimiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	al.mu.Lock()
	defer al.mu.Unlock()

	now := time.Now()
	b, exists := al.buckets[key]
	if !exists {
		if len(al.buckets) >= maxAPILimitEntries {
			// Under memory pressure, allow the request but don't track.
			return true, 0
		}
		al.buckets[key] = &bucket{
			tokens:   float64(al.burst) - 1,
			lastSeen: now,
		}
		return true, 0
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * al.rate
	if b.tokens > float64(al.burst) {
		b.tokens = float64(al.burst)
	}
	b.lastSeen = now

	if b.tokens < 1 {
		// Calculate when the next token will be available.
		wait := time.Duration((1 - b.tokens) / al.rate * float64(time.Second))
		return false, wait
	}

	b.tokens--
	return true, 0
}

// Middleware returns an HTTP middleware that rate-limits by authenticated user ID,
// falling back to IP if no user is present.
func (al *APILimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			if user, ok := GetUser(r.Context()); ok {
				key = "user:" + user.UserID
			}

			allowed, retryAfter := al.Allow(key)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				apierr.TooManyRequests("rate limit exceeded").Write(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
