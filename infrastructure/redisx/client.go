// Package redisx builds the go-redis client.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// New parses REDIS_URL (redis:// or rediss://) and returns a pooled client.
// The same client serves Publish; Pub/Sub subscriptions take their own
// dedicated connection, outside this pool.
func New(ctx context.Context, url string, poolSize int) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	opts.PoolSize = poolSize
	opts.MinIdleConns = min(poolSize, 4)
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second
	opts.MaxRetries = 3
	opts.ConnMaxIdleTime = 5 * time.Minute

	rdb := redis.NewClient(opts)

	backoff := 200 * time.Millisecond
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()
		if err == nil {
			return rdb, nil
		}
		if attempt >= 8 || ctx.Err() != nil {
			_ = rdb.Close()
			return nil, fmt.Errorf("redis unreachable after %d attempts: %w", attempt, err)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			_ = rdb.Close()
			return nil, ctx.Err()
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}
