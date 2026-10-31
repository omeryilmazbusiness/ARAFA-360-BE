// Package postgres builds the pgx connection pool and runs migrations.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
	Tracer          pgx.QueryTracer // optional (Sentry spans)
}

// NewPool creates a tuned pgxpool and waits (with backoff) until Postgres
// answers: on Railway the app and database boot concurrently.
func NewPool(ctx context.Context, c PoolConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(c.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = c.MaxConns
	cfg.MinConns = c.MinConns
	cfg.MaxConnLifetime = c.MaxConnLifetime
	// Jitter stops every connection from being recycled in the same instant.
	cfg.MaxConnLifetimeJitter = c.MaxConnLifetime / 10
	cfg.MaxConnIdleTime = c.MaxConnIdleTime
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.ConnectTimeout = c.ConnectTimeout
	cfg.ConnConfig.Tracer = c.Tracer
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "arafa-360"
	// Bound runaway queries server-side as a safety net.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	backoff := 200 * time.Millisecond
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, c.ConnectTimeout)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if attempt >= 8 || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("postgres unreachable after %d attempts: %w", attempt, err)
		}
		log.Warn("postgres not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}
