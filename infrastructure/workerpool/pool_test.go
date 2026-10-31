package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunsAllTasksAndDrainsOnShutdown(t *testing.T) {
	p := New(Options{Workers: 4, QueueSize: 100})
	var n atomic.Int64
	for i := 0; i < 100; i++ {
		if !p.TrySubmit(func(context.Context) { n.Add(1) }) {
			t.Fatal("unexpected rejection")
		}
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 100 {
		t.Fatalf("ran %d tasks, want 100", n.Load())
	}
}

func TestRejectsWhenQueueFull(t *testing.T) {
	p := New(Options{Workers: 1, QueueSize: 1})
	block := make(chan struct{})
	started := make(chan struct{})
	p.TrySubmit(func(context.Context) { close(started); <-block })
	<-started // worker busy
	if !p.TrySubmit(func(context.Context) {}) {
		t.Fatal("queue slot should be free")
	}
	if p.TrySubmit(func(context.Context) {}) {
		t.Fatal("expected rejection on full queue")
	}
	if p.Stats().Rejected != 1 {
		t.Fatalf("rejected = %d", p.Stats().Rejected)
	}
	close(block)
	_ = p.Shutdown(context.Background())
}

func TestPanicIsContainedAndWorkerSurvives(t *testing.T) {
	var recovered atomic.Int64
	p := New(Options{Workers: 1, QueueSize: 4, OnPanic: func(any) { recovered.Add(1) }})
	p.TrySubmit(func(context.Context) { panic("boom") })
	done := make(chan struct{})
	p.TrySubmit(func(context.Context) { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker died after panic")
	}
	_ = p.Shutdown(context.Background())
	if recovered.Load() != 1 {
		t.Fatalf("recovered = %d", recovered.Load())
	}
}

func TestSubmitAfterShutdownIsRejected(t *testing.T) {
	p := New(Options{Workers: 1, QueueSize: 1})
	_ = p.Shutdown(context.Background())
	if p.TrySubmit(func(context.Context) {}) {
		t.Fatal("submit after shutdown must fail")
	}
}

func TestShutdownTimeoutCancelsTasks(t *testing.T) {
	p := New(Options{Workers: 1, QueueSize: 1})
	cancelled := make(chan struct{})
	p.TrySubmit(func(ctx context.Context) { <-ctx.Done(); close(cancelled) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := p.Shutdown(ctx); err == nil {
		t.Fatal("expected timeout error")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("task not cancelled")
	}
}
