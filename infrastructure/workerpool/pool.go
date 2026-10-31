// Package workerpool is a bounded goroutine pool with a bounded queue.
//
// Under a surge (millions of QR scans) unbounded `go func()` fan-out turns a
// slow dependency into an OOM. This pool gives explicit back-pressure:
// TrySubmit never blocks, and returns false when the queue is full so the
// caller can shed load (drop a non-critical side effect, or answer 503).
package workerpool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task is an alias (not a defined type) so any `func(context.Context)`
// satisfies consumer-side interfaces without conversion.
type Task = func(ctx context.Context)

type Options struct {
	Workers     int
	QueueSize   int
	TaskTimeout time.Duration // per-task deadline; 0 = none
	OnPanic     func(recovered any)
}

type Stats struct {
	Queued    int
	Workers   int
	Submitted uint64
	Rejected  uint64
	Panics    uint64
}

type Pool struct {
	opts   Options
	tasks  chan Task
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.RWMutex // guards closed + sends vs close(tasks)
	closed bool

	submitted atomic.Uint64
	rejected  atomic.Uint64
	panics    atomic.Uint64
}

func New(o Options) *Pool {
	if o.Workers < 1 {
		o.Workers = 1
	}
	if o.QueueSize < 1 {
		o.QueueSize = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{opts: o, tasks: make(chan Task, o.QueueSize), ctx: ctx, cancel: cancel}
	p.wg.Add(o.Workers)
	for i := 0; i < o.Workers; i++ {
		go p.worker()
	}
	return p
}

// TrySubmit enqueues t without blocking. It returns false if the queue is
// full or the pool is shutting down.
func (p *Pool) TrySubmit(t Task) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		p.rejected.Add(1)
		return false
	}
	select {
	case p.tasks <- t:
		p.submitted.Add(1)
		return true
	default:
		p.rejected.Add(1)
		return false
	}
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for t := range p.tasks {
		p.run(t)
	}
}

func (p *Pool) run(t Task) {
	ctx := p.ctx
	if p.opts.TaskTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.opts.TaskTimeout)
		defer cancel()
	}
	defer func() {
		if r := recover(); r != nil {
			p.panics.Add(1)
			if p.opts.OnPanic != nil {
				p.opts.OnPanic(r)
			}
		}
	}()
	t(ctx)
}

// Shutdown stops accepting work and waits for queued tasks to finish. If ctx
// expires first, running tasks are cancelled and ctx.Err() is returned.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.tasks)
	}
	p.mu.Unlock()

	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()

	select {
	case <-done:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}

func (p *Pool) Stats() Stats {
	return Stats{
		Queued:    len(p.tasks),
		Workers:   p.opts.Workers,
		Submitted: p.submitted.Load(),
		Rejected:  p.rejected.Load(),
		Panics:    p.panics.Load(),
	}
}
