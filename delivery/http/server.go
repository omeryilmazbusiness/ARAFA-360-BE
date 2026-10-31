// Package http is the REST delivery layer built on Go Fiber.
package http

import (
	"context"
	"log/slog"
	"strings"
	"time"

	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/arafat-360/arafa-360-be/delivery/websocket"
	"github.com/arafat-360/arafa-360-be/usecase"
)

// Check is a named dependency probe used by /readyz.
type Check struct {
	Name string
	Run  func(ctx context.Context) error
}

type Config struct {
	AppName        string
	Version        string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	BodyLimitBytes int
	AccessLog      bool
	AllowedOrigins []string
}

type Deps struct {
	Log       *slog.Logger
	Tenants   *usecase.TenantUsecase
	Handlers  *Handlers
	WebSocket *websocket.Handler
	Readiness []Check
}

// NewApp builds the Fiber application: middleware stack, routes, WebSocket.
func NewApp(cfg Config, d Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               cfg.AppName,
		ServerHeader:          "arafa-360",
		DisableStartupMessage: true,
		ErrorHandler:          ErrorHandler,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		IdleTimeout:           cfg.IdleTimeout,
		BodyLimit:             cfg.BodyLimitBytes,
		CaseSensitive:         true,
		// Railway terminates TLS and proxies: take the client IP from here.
		ProxyHeader: fiber.HeaderXForwardedFor,
		// Prefork is intentionally off: the WebSocket hub, worker pool and
		// tenant cache are per-process state. Scale out with Railway replicas.
		Prefork: false,
	})

	// Middleware order matters (outermost first):
	//  1. recover      - last resort for panics in the layers below
	//  2. sentryfiber  - hub + performance transaction per request
	//  3. requestid    - correlation id on every response and error
	//  4. errorBoundary- render errors inside the transaction
	//  5. recover      - handler panics -> Sentry + 500 (via errorBoundary)
	app.Use(recover.New())
	app.Use(sentryfiber.New(sentryfiber.Options{Repanic: true, Timeout: 3 * time.Second}))
	app.Use(requestid.New())
	if cfg.AccessLog {
		app.Use(logger.New(logger.Config{
			Next:   func(c *fiber.Ctx) bool { return isProbe(c.Path()) },
			Format: "${time} ${status} ${latency} ${method} ${path} rid=${locals:requestid}\n",
		}))
	}
	app.Use(errorBoundary)
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			hub := sentryfiber.GetHubFromContext(c)
			if hub != nil {
				hub.Recover(e)
			}
			d.Log.Error("panic recovered", "panic", e, "path", c.Path())
		},
	}))
	if len(cfg.AllowedOrigins) > 0 {
		app.Use(cors.New(cors.Config{
			AllowOrigins: strings.Join(cfg.AllowedOrigins, ","),
			AllowHeaders: "Content-Type, X-Tenant-ID, Authorization",
			MaxAge:       600,
		}))
	}
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
		Next:  func(c *fiber.Ctx) bool { return c.Path() == "/ws" },
	}))

	// Probes: no tenant, no tracing (see observability.ignoredTransactions).
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": cfg.Version})
	})
	app.Get("/readyz", readyz(d.Readiness))

	// Tenant API.
	api := app.Group("/api/v1", Tenant(d.Tenants, false))
	h := d.Handlers
	api.Post("/zones", h.CreateZone)
	api.Post("/tents", h.CreateTent)
	api.Post("/tents/:id/beds", h.ProvisionBeds)
	api.Post("/beds/scan", h.ScanBed)
	api.Post("/beds/check-in", h.CheckIn)
	api.Post("/beds/check-out", h.CheckOut)
	api.Post("/incidents", h.ReportIncident)

	// Command Center live feed.
	d.WebSocket.Register(app, "/ws", Tenant(d.Tenants, true))

	return app
}

func isProbe(path string) bool { return path == "/healthz" || path == "/readyz" }

func readyz(checks []Check) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()
		status := fiber.StatusOK
		out := fiber.Map{}
		for _, ch := range checks {
			if err := ch.Run(ctx); err != nil {
				status, out[ch.Name] = fiber.StatusServiceUnavailable, "down"
			} else {
				out[ch.Name] = "up"
			}
		}
		return c.Status(status).JSON(out)
	}
}
