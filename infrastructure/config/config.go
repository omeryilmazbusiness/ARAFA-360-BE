// Package config loads runtime configuration from the environment
// (12-factor; Railway injects PORT, DATABASE_URL, REDIS_URL, ...).
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env     string
	Version string

	HTTPAddr        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	BodyLimitBytes  int
	AccessLog       bool

	DatabaseURL      string
	DBMaxConns       int32
	DBMinConns       int32
	DBMaxConnLife    time.Duration
	DBMaxConnIdle    time.Duration
	DBConnectTimeout time.Duration
	AutoMigrate      bool

	RedisURL      string
	RedisPoolSize int

	WorkerPoolSize  int
	WorkerQueueSize int

	WSMaxConns       int
	AllowedOrigins   []string
	TenantCacheTTL   time.Duration
	SentryDSN        string
	SentryTraceRate  float64
	SentryDebug      bool
	SentryServerName string
}

func (c Config) Production() bool { return c.Env == "production" }

// Load reads .env (if present, for local development) and the process env.
func Load() (Config, error) {
	_ = godotenv.Load() // optional

	cpu := runtime.NumCPU()
	var errs []error
	c := Config{
		Env:     str("APP_ENV", "development"),
		Version: firstNonEmpty(os.Getenv("APP_VERSION"), os.Getenv("RAILWAY_GIT_COMMIT_SHA"), "dev"),

		// Railway provides PORT. HTTP_ADDR wins when set explicitly.
		HTTPAddr:        firstNonEmpty(os.Getenv("HTTP_ADDR"), ":"+str("PORT", "8080")),
		ReadTimeout:     dur("HTTP_READ_TIMEOUT", 10*time.Second, &errs),
		WriteTimeout:    dur("HTTP_WRITE_TIMEOUT", 15*time.Second, &errs),
		IdleTimeout:     dur("HTTP_IDLE_TIMEOUT", 60*time.Second, &errs),
		ShutdownTimeout: dur("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second, &errs),
		BodyLimitBytes:  num("HTTP_BODY_LIMIT_BYTES", 1<<20, &errs),
		AccessLog:       boolean("HTTP_ACCESS_LOG", true),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		// Railway Postgres defaults to ~100 connections shared by every replica:
		// keep MaxConns * replicas comfortably below that.
		DBMaxConns:       int32(num("DB_MAX_CONNS", 25, &errs)),
		DBMinConns:       int32(num("DB_MIN_CONNS", 5, &errs)),
		DBMaxConnLife:    dur("DB_MAX_CONN_LIFETIME", 30*time.Minute, &errs),
		DBMaxConnIdle:    dur("DB_MAX_CONN_IDLE", 5*time.Minute, &errs),
		DBConnectTimeout: dur("DB_CONNECT_TIMEOUT", 10*time.Second, &errs),
		AutoMigrate:      boolean("AUTO_MIGRATE", true),

		RedisURL:      os.Getenv("REDIS_URL"),
		RedisPoolSize: num("REDIS_POOL_SIZE", 10*cpu, &errs),

		// Pool tasks are I/O bound (Redis publish), so oversubscribe the CPUs.
		WorkerPoolSize:  num("WORKER_POOL_SIZE", 16*cpu, &errs),
		WorkerQueueSize: num("WORKER_QUEUE_SIZE", 20_000, &errs),

		WSMaxConns:     num("WS_MAX_CONNS", 10_000, &errs),
		AllowedOrigins: csv("ALLOWED_ORIGINS"),
		TenantCacheTTL: dur("TENANT_CACHE_TTL", 30*time.Second, &errs),

		SentryDSN:        os.Getenv("SENTRY_DSN"),
		SentryTraceRate:  float("SENTRY_TRACES_SAMPLE_RATE", 0.1, &errs),
		SentryDebug:      boolean("SENTRY_DEBUG", false),
		SentryServerName: firstNonEmpty(os.Getenv("RAILWAY_REPLICA_ID"), os.Getenv("HOSTNAME")),
	}

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL is required"))
	}
	if c.DBMinConns > c.DBMaxConns {
		errs = append(errs, errors.New("DB_MIN_CONNS must be <= DB_MAX_CONNS"))
	}
	if c.WorkerPoolSize < 1 || c.WorkerQueueSize < 1 {
		errs = append(errs, errors.New("WORKER_POOL_SIZE and WORKER_QUEUE_SIZE must be >= 1"))
	}
	if c.Production() && len(c.AllowedOrigins) == 0 {
		errs = append(errs, errors.New("ALLOWED_ORIGINS is required in production (WebSocket origin check)"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func num(key string, def int, errs *[]error) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return n
}

func float(key string, def float64, errs *[]error) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 1 {
		*errs = append(*errs, fmt.Errorf("%s must be a number in [0,1]", key))
		return def
	}
	return f
}

func dur(key string, def time.Duration, errs *[]error) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return d
}

func boolean(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return b
}

func csv(key string) []string {
	var out []string
	for _, p := range strings.Split(os.Getenv(key), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
