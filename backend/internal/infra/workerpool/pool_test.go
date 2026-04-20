package workerpool

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestPoolSubmitAndComplete(t *testing.T) {
	p := New(4)
	var count atomic.Int64
	var wg sync.WaitGroup

	const n = 100
	wg.Add(n)
	for range n {
		p.Submit(func() {
			defer wg.Done()
			count.Add(1)
		})
	}
	wg.Wait()

	if got := count.Load(); got != n {
		t.Errorf("count = %d, want %d", got, n)
	}
	stats := p.Stats()
	if stats.Done != n {
		t.Errorf("Stats.Done = %d, want %d", stats.Done, n)
	}
	if stats.MaxSize != 4 {
		t.Errorf("Stats.MaxSize = %d, want 4", stats.MaxSize)
	}
}

func TestPoolDefaultSize(t *testing.T) {
	p := New(0)
	stats := p.Stats()
	if stats.MaxSize < 4 {
		t.Errorf("default MaxSize = %d, want >= 4", stats.MaxSize)
	}
	if stats.MaxSize > 128 {
		t.Errorf("default MaxSize = %d, want <= 128", stats.MaxSize)
	}
}

func TestPoolBackpressure(t *testing.T) {
	p := New(1)
	started := make(chan struct{})
	release := make(chan struct{})

	p.Submit(func() {
		close(started)
		<-release
	})
	<-started

	// At this point the single worker slot is occupied
	stats := p.Stats()
	if stats.Active != 1 {
		t.Errorf("Active = %d, want 1", stats.Active)
	}

	done := make(chan struct{})
	go func() {
		p.Submit(func() {}) // should block until release
		close(done)
	}()

	// Release the blocking task
	close(release)
	<-done
}
