// Package websocket is the real-time delivery layer: a tenant-aware broadcast
// hub plus the Fiber handler that upgrades dashboard connections.
package websocket

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
)

const (
	inboundBuffer  = 8192 // events waiting for the hub loop
	clientSendSize = 256  // per-client outbound buffer
)

type message struct {
	tenantID uuid.UUID
	payload  []byte // shared, read-only
}

// Hub fans events out to the WebSocket clients of the matching tenant.
//
// Concurrency model: a single goroutine (Run) owns the `rooms` map, so there
// are no locks on the hot path. Everything else talks to it through
// channels. The hub never blocks on a client: each client has a bounded
// buffer, and a client that cannot keep up is disconnected (it should
// reconnect and re-sync) instead of stalling the broadcast for everyone.
//
// Scaling: one hub per API replica. Cross-replica delivery is Redis Pub/Sub's
// job (see repository/redis). If one replica ever serves tens of thousands of
// dashboards, shard `rooms` by tenant across N hub loops.
type Hub struct {
	log      *slog.Logger
	maxConns int64

	register   chan *Client
	unregister chan *Client
	inbound    chan message
	done       chan struct{} // closed when Run returns

	rooms map[uuid.UUID]map[*Client]struct{} // owned by Run

	conns       atomic.Int64
	delivered   atomic.Uint64
	shedInbound atomic.Uint64
	slowKicked  atomic.Uint64
}

func NewHub(log *slog.Logger, maxConns int) *Hub {
	return &Hub{
		log:        log,
		maxConns:   int64(maxConns),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		inbound:    make(chan message, inboundBuffer),
		done:       make(chan struct{}),
		rooms:      make(map[uuid.UUID]map[*Client]struct{}),
	}
}

type Stats struct {
	Connections int64
	Delivered   uint64
	ShedInbound uint64
	SlowKicked  uint64
}

func (h *Hub) Stats() Stats {
	return Stats{h.conns.Load(), h.delivered.Load(), h.shedInbound.Load(), h.slowKicked.Load()}
}

// Run owns the broadcast loop until ctx is cancelled, then disconnects everyone.
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			for _, room := range h.rooms {
				for c := range room {
					close(c.send)
				}
			}
			h.rooms = nil
			return

		case c := <-h.register:
			room := h.rooms[c.tenantID]
			if room == nil {
				room = make(map[*Client]struct{})
				h.rooms[c.tenantID] = room
			}
			room[c] = struct{}{}

		case c := <-h.unregister:
			h.remove(c)

		case m := <-h.inbound:
			room := h.rooms[m.tenantID]
			for c := range room {
				select {
				case c.send <- m.payload:
					h.delivered.Add(1)
				default:
					// Slow consumer: cut it loose rather than block the loop.
					h.slowKicked.Add(1)
					h.remove(c)
				}
			}
		}
	}
}

// remove deletes c from its room and closes its send channel exactly once.
// Only called from the Run goroutine.
func (h *Hub) remove(c *Client) {
	room, ok := h.rooms[c.tenantID]
	if !ok {
		return
	}
	if _, ok := room[c]; !ok {
		return
	}
	delete(room, c)
	if len(room) == 0 {
		delete(h.rooms, c.tenantID)
	}
	close(c.send)
	h.conns.Add(-1)
}

// Register adds c to its tenant's room. It returns false when the replica is
// at its connection limit or shutting down.
func (h *Hub) Register(c *Client) bool {
	if h.conns.Add(1) > h.maxConns {
		h.conns.Add(-1)
		return false
	}
	select {
	case h.register <- c:
		return true
	case <-h.done:
		h.conns.Add(-1)
		return false
	}
}

func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Dispatch queues a payload for every client of tenantID. It never blocks;
// if the hub loop is saturated the event is shed and counted.
func (h *Hub) Dispatch(tenantID uuid.UUID, payload []byte) {
	select {
	case h.inbound <- message{tenantID: tenantID, payload: payload}:
	default:
		h.shedInbound.Add(1)
	}
}

// RunSubscriber feeds the hub from the event bus (Redis Pub/Sub) and
// re-establishes the subscription with backoff if it ever fails.
func (h *Hub) RunSubscriber(ctx context.Context, sub domain.EventSubscriber) {
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := sub.Subscribe(ctx, h.Dispatch)
		if ctx.Err() != nil {
			return
		}
		h.log.Error("event subscription lost, reconnecting", "error", err, "retry_in", backoff)
		sentry.CaptureException(err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second // it was healthy for a while; start over
		} else {
			backoff = min(backoff*2, 30*time.Second)
		}
	}
}
