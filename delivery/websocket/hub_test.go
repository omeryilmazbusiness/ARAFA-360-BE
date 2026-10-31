package websocket

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

func startHub(t *testing.T, maxConns int) *Hub {
	t.Helper()
	h := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)), maxConns)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(cancel)
	return h
}

func recv(t *testing.T, c *Client) ([]byte, bool) {
	t.Helper()
	select {
	case m, ok := <-c.send:
		return m, ok
	case <-time.After(time.Second):
		return nil, false
	}
}

func TestBroadcastIsTenantIsolated(t *testing.T) {
	h := startHub(t, 10)
	tenantA, tenantB := uuid.New(), uuid.New()
	a1, a2, b1 := newClient(tenantA, nil), newClient(tenantA, nil), newClient(tenantB, nil)
	for _, c := range []*Client{a1, a2, b1} {
		if !h.Register(c) {
			t.Fatal("register failed")
		}
	}

	h.Dispatch(tenantA, []byte(`{"type":"incident.reported"}`))

	for _, c := range []*Client{a1, a2} {
		if m, ok := recv(t, c); !ok || string(m) != `{"type":"incident.reported"}` {
			t.Fatalf("tenant A client missed event: %q ok=%v", m, ok)
		}
	}
	select {
	case m := <-b1.send:
		t.Fatalf("tenant B received tenant A event: %q", m)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSlowClientIsDroppedWithoutBlockingOthers(t *testing.T) {
	h := startHub(t, 10)
	tenant := uuid.New()
	slow, fast := newClient(tenant, nil), newClient(tenant, nil)
	h.Register(slow)
	h.Register(fast)

	// Never drain `slow`; drain `fast` concurrently.
	got := make(chan int)
	go func() {
		n := 0
		for range fast.send {
			n++
			if n == clientSendSize+50 {
				got <- n
				return
			}
		}
	}()
	for i := 0; i < clientSendSize+50; i++ {
		h.Dispatch(tenant, []byte("x"))
		time.Sleep(50 * time.Microsecond) // let fast keep up; slow cannot
	}
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("fast client starved by slow client")
	}
	if h.Stats().SlowKicked != 1 {
		t.Fatalf("slow kicked = %d, want 1", h.Stats().SlowKicked)
	}
}

func TestConnectionLimit(t *testing.T) {
	h := startHub(t, 1)
	if !h.Register(newClient(uuid.New(), nil)) {
		t.Fatal("first register should succeed")
	}
	if h.Register(newClient(uuid.New(), nil)) {
		t.Fatal("second register should be refused at limit")
	}
}

func TestUnregisterClosesSendOnce(t *testing.T) {
	h := startHub(t, 10)
	c := newClient(uuid.New(), nil)
	h.Register(c)
	h.Unregister(c)
	h.Unregister(c) // idempotent: must not double-close/panic
	if _, ok := recv(t, c); ok {
		t.Fatal("send should be closed")
	}
	if h.Stats().Connections != 0 {
		t.Fatalf("connections = %d", h.Stats().Connections)
	}
}

func TestShutdownDisconnectsClientsAndRefusesNew(t *testing.T) {
	h := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)), 10)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	c := newClient(uuid.New(), nil)
	h.Register(c)
	cancel()
	if _, ok := recv(t, c); ok {
		t.Fatal("client should be disconnected on shutdown")
	}
	<-h.done
	if h.Register(newClient(uuid.New(), nil)) {
		t.Fatal("register after shutdown must fail")
	}
}
