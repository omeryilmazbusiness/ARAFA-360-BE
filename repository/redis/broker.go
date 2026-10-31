// Package redis implements the Command Center event bus on Redis Pub/Sub.
//
// Topology: one channel per tenant (`arafa:events:<tenant_id>`). Publishers
// write to their tenant's channel; every API replica holds exactly ONE
// pattern subscription (`arafa:events:*`) and fans messages out to its local
// WebSocket clients. Redis connections therefore scale with replicas, not
// with dashboard users.
//
// Pub/Sub is at-most-once and has no history: a dashboard that is offline
// misses events. If incidents must be replayable, add a Redis Stream or a
// persisted incidents table alongside this (see README "Next steps").
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/arafat-360/arafa-360-be/domain"
)

const channelPrefix = "arafa:events:"

type Broker struct{ rdb *redis.Client }

func NewBroker(rdb *redis.Client) *Broker { return &Broker{rdb} }

var (
	_ domain.EventPublisher  = (*Broker)(nil)
	_ domain.EventSubscriber = (*Broker)(nil)
)

func (b *Broker) Publish(ctx context.Context, e domain.Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := b.rdb.Publish(ctx, channelPrefix+e.TenantID.String(), payload).Err(); err != nil {
		return fmt.Errorf("redis publish: %w", err)
	}
	return nil
}

// Subscribe blocks, delivering every tenant's events to handler. go-redis
// transparently reconnects and re-subscribes if the connection drops; the
// returned error is only for the initial subscription or ctx cancellation.
func (b *Broker) Subscribe(ctx context.Context, handler func(tenantID uuid.UUID, payload []byte)) error {
	ps := b.rdb.PSubscribe(ctx, channelPrefix+"*")
	defer ps.Close()

	if _, err := ps.Receive(ctx); err != nil { // wait for subscription confirmation
		return fmt.Errorf("redis psubscribe: %w", err)
	}

	ch := ps.Channel(redis.WithChannelSize(4096))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return fmt.Errorf("redis subscription closed")
			}
			tenantID, err := uuid.Parse(strings.TrimPrefix(msg.Channel, channelPrefix))
			if err != nil {
				continue // foreign channel under our prefix; ignore
			}
			handler(tenantID, []byte(msg.Payload))
		}
	}
}
