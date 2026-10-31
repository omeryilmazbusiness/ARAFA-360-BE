// ARAFA-360: multi-tenant Command Center backend for Mount Arafat logistics.
//
// main.go is the composition root: it is the only place that knows every
// concrete type. It builds infrastructure (Sentry, PostgreSQL, Redis, worker
// pool), injects it into repositories -> use cases -> delivery, then runs the
// Fiber server and the WebSocket hub until SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"

	httpdelivery "github.com/arafat-360/arafa-360-be/delivery/http"
	"github.com/arafat-360/arafa-360-be/delivery/websocket"
	"github.com/arafat-360/arafa-360-be/infrastructure/config"
	"github.com/arafat-360/arafa-360-be/infrastructure/observability"
	"github.com/arafat-360/arafa-360-be/infrastructure/postgres"
	"github.com/arafat-360/arafa-360-be/infrastructure/redisx"
	"github.com/arafat-360/arafa-360-be/infrastructure/workerpool"
	"github.com/arafat-360/arafa-360-be/migrations"
	pgrepo "github.com/arafat-360/arafa-360-be/repository/postgres"
	redisrepo "github.com/arafat-360/arafa-360-be/repository/redis"
	"github.com/arafat-360/arafa-360-be/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		sentry.Flush(5 * time.Second)
		os.Exit(1)
	}
}

func run() (err error) {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// --- Observability first, so even startup failures are reported. -------
	flushSentry, err := observability.InitSentry(observability.SentryConfig{
		DSN:              cfg.SentryDSN,
		Environment:      cfg.Env,
		Release:          "arafa-360@" + cfg.Version,
		ServerName:       cfg.SentryServerName,
		TracesSampleRate: cfg.SentryTraceRate,
		Debug:            cfg.SentryDebug,
	})
	if err != nil {
		return fmt.Errorf("sentry init: %w", err)
	}
	defer flushSentry()
	defer func() { // capture panics in the main goroutine, then crash loudly
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			flushSentry()
			panic(r)
		}
	}()

	log := newLogger(cfg)
	slog.SetDefault(log)
	log.Info("starting", "env", cfg.Env, "version", cfg.Version, "addr", cfg.HTTPAddr,
		"sentry", cfg.SentryDSN != "")

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Infrastructure ----------------------------------------------------
	startCtx, cancelStart := context.WithTimeout(rootCtx, 60*time.Second)
	defer cancelStart()

	pool, err := postgres.NewPool(startCtx, postgres.PoolConfig{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLife,
		MaxConnIdleTime: cfg.DBMaxConnIdle,
		ConnectTimeout:  cfg.DBConnectTimeout,
		Tracer:          observability.PGXTracer{},
	}, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := postgres.Migrate(startCtx, pool, migrations.FS, log); err != nil {
			return err
		}
	}

	rdb, err := redisx.New(startCtx, cfg.RedisURL, cfg.RedisPoolSize)
	if err != nil {
		return err
	}
	defer rdb.Close()
	cancelStart()

	// Bounded goroutine pool for non-critical async work (dashboard fan-out of
	// scan events). Panics inside tasks are reported, never fatal.
	workers := workerpool.New(workerpool.Options{
		Workers:     cfg.WorkerPoolSize,
		QueueSize:   cfg.WorkerQueueSize,
		TaskTimeout: 5 * time.Second,
		OnPanic: func(r any) {
			sentry.CurrentHub().Recover(r)
			log.Error("worker task panicked", "panic", r)
		},
	})

	// --- Repositories (adapters for the domain ports) ----------------------
	tenantRepo := pgrepo.NewTenantRepository(pool)
	zoneRepo := pgrepo.NewZoneRepository(pool)
	tentRepo := pgrepo.NewTentRepository(pool)
	bedRepo := pgrepo.NewBedRepository(pool)
	broker := redisrepo.NewBroker(rdb) // implements EventPublisher + EventSubscriber

	// --- Use cases ---------------------------------------------------------
	tenants := usecase.NewTenantUsecase(tenantRepo, cfg.TenantCacheTTL)
	inventory := usecase.NewInventoryUsecase(zoneRepo, tentRepo, bedRepo)
	beds := usecase.NewBedUsecase(bedRepo, broker, workers, log)
	incidents := usecase.NewIncidentUsecase(tentRepo, broker)

	// --- Real-time layer: Redis Pub/Sub -> Hub -> WebSocket clients --------
	hub := websocket.NewHub(log, cfg.WSMaxConns)
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	hubStopped := make(chan struct{})
	go func() { hub.Run(hubCtx); close(hubStopped) }()
	go hub.RunSubscriber(hubCtx, broker)

	// --- Delivery ----------------------------------------------------------
	app := httpdelivery.NewApp(httpdelivery.Config{
		AppName:        "arafa-360",
		Version:        cfg.Version,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		BodyLimitBytes: cfg.BodyLimitBytes,
		AccessLog:      cfg.AccessLog,
		AllowedOrigins: cfg.AllowedOrigins,
	}, httpdelivery.Deps{
		Log:     log,
		Tenants: tenants,
		Handlers: &httpdelivery.Handlers{
			Inventory: inventory, Beds: beds, Incidents: incidents,
		},
		WebSocket: websocket.NewHandler(hub, websocket.HandlerConfig{
			Origins:         cfg.AllowedOrigins,
			TenantLocalsKey: httpdelivery.TenantKey,
		}),
		Readiness: []httpdelivery.Check{
			{Name: "postgres", Run: pool.Ping},
			{Name: "redis", Run: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
		},
	})

	serverErr := make(chan error, 1)
	go func() { serverErr <- app.Listen(cfg.HTTPAddr) }()
	log.Info("listening", "addr", cfg.HTTPAddr)

	select {
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	// --- Graceful shutdown (reverse order of startup) ----------------------
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	var errs []error
	// 1. Stop accepting HTTP, let in-flight requests finish.
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	// 2. Disconnect dashboards (clients get a Close frame and reconnect to a healthy replica).
	stopHub()
	select {
	case <-hubStopped:
	case <-shutdownCtx.Done():
	}
	// 3. Drain queued async work.
	if err := workers.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("worker pool shutdown: %w", err))
	}
	log.Info("worker pool", "stats", fmt.Sprintf("%+v", workers.Stats()), "hub", fmt.Sprintf("%+v", hub.Stats()))
	// 4. pool.Close / rdb.Close / sentry flush run via defers.
	return errors.Join(errs...)
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler = slog.NewTextHandler(os.Stdout, opts)
	if cfg.Production() {
		h = slog.NewJSONHandler(os.Stdout, opts) // structured logs for Railway
	}
	// Every log.Error(..., "error", err) also lands in Sentry.
	return slog.New(observability.NewSentryHandler(h)).With("service", "arafa-360")
}
