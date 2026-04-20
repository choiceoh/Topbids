// Package workerpool provides a bounded goroutine pool.
// Ported from Deneb gateway, metrics dependency removed.
package workerpool

import (
	"runtime"
	"sync/atomic"
)

type Pool struct {
	sem     chan struct{}
	maxSize int
	active  atomic.Int64
	queued  atomic.Int64
	done    atomic.Int64
}

func New(maxWorkers int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = defaultSize()
	}
	return &Pool{
		sem:     make(chan struct{}, maxWorkers),
		maxSize: maxWorkers,
	}
}

func defaultSize() int {
	n := runtime.NumCPU() * 2
	if n < 4 {
		n = 4
	}
	if n > 128 {
		n = 128
	}
	return n
}

// Submit queues a task. Blocks if all workers are busy (back-pressure).
func (p *Pool) Submit(task func()) {
	p.queued.Add(1)
	p.sem <- struct{}{}
	p.queued.Add(-1)
	p.active.Add(1)

	go func() {
		defer func() {
			p.active.Add(-1)
			p.done.Add(1)
			<-p.sem
		}()
		task()
	}()
}

// Wait blocks until all currently active tasks finish.
func (p *Pool) Wait() {
	for range p.maxSize {
		p.sem <- struct{}{}
	}
	for range p.maxSize {
		<-p.sem
	}
}

type Stats struct {
	MaxSize int `json:"maxSize"`
	Active  int `json:"active"`
	Queued  int `json:"queued"`
	Done    int `json:"done"`
}

func (p *Pool) Stats() Stats {
	return Stats{
		MaxSize: p.maxSize,
		Active:  int(p.active.Load()),
		Queued:  int(p.queued.Load()),
		Done:    int(p.done.Load()),
	}
}
