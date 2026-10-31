package websocket

import (
	"slices"
	"time"

	fastws "github.com/fasthttp/websocket"
	"github.com/getsentry/sentry-go"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type HandlerConfig struct {
	// Origins allowed to connect (browser CSRF protection). Empty = any.
	Origins []string
	// TenantLocalsKey is the fiber Locals key under which an upstream guard
	// middleware stored the authenticated tenant (uuid.UUID).
	TenantLocalsKey string
}

type Handler struct {
	hub *Hub
	cfg HandlerConfig
}

func NewHandler(hub *Hub, cfg HandlerConfig) *Handler { return &Handler{hub: hub, cfg: cfg} }

// Register mounts GET path as a WebSocket endpoint. guards run before the
// upgrade and must authenticate the caller and set the tenant Locals key;
// rejecting there returns a normal HTTP error instead of a half-open socket.
func (h *Handler) Register(r fiber.Router, path string, guards ...fiber.Handler) {
	handlers := append([]fiber.Handler{}, guards...)
	handlers = append(handlers,
		func(c *fiber.Ctx) error {
			if !websocket.IsWebSocketUpgrade(c) {
				return fiber.ErrUpgradeRequired
			}
			if _, ok := c.Locals(h.cfg.TenantLocalsKey).(uuid.UUID); !ok {
				return fiber.ErrUnauthorized
			}
			// Browser CSRF defence (cross-site WebSocket hijacking). Browsers
			// always send Origin, so it must be allow-listed; non-browser
			// clients (native apps, load tools) send none and are authenticated
			// by the guard above instead. The contrib library's own check
			// would reject those clients, so it is left open on purpose.
			if origin := c.Get(fiber.HeaderOrigin); origin != "" && !h.originAllowed(origin) {
				return fiber.NewError(fiber.StatusForbidden, "origin not allowed")
			}
			return c.Next()
		},
		websocket.New(h.serve, websocket.Config{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// A panic in one connection must not take down the process; report it.
			RecoverHandler: func(*websocket.Conn) {
				if r := recover(); r != nil {
					sentry.CurrentHub().Recover(r)
				}
			},
		}),
	)
	r.Get(path, handlers...)
}

func (h *Handler) originAllowed(origin string) bool {
	return len(h.cfg.Origins) == 0 || slices.Contains(h.cfg.Origins, origin)
}

// serve runs for the lifetime of one connection (own goroutine, started by
// the upgrader). Everything it needs from the request was copied into the
// connection's locals before the upgrade.
func (h *Handler) serve(conn *websocket.Conn) {
	tenantID, _ := conn.Locals(h.cfg.TenantLocalsKey).(uuid.UUID)
	raw := conn.Conn // the underlying connection outlives the contrib wrapper

	c := newClient(tenantID, raw)
	if !h.hub.Register(c) {
		_ = raw.WriteControl(fastws.CloseMessage,
			fastws.FormatCloseMessage(fastws.CloseTryAgainLater, "server busy"), time.Now().Add(writeWait))
		_ = raw.Close()
		return
	}

	// Queued before writePump starts, so it is always the first frame.
	c.send <- []byte(`{"type":"connection.ready"}`)

	writerDone := make(chan struct{})
	go c.writePump(writerDone)

	c.readPump() // blocks until the client goes away

	h.hub.Unregister(c) // hub closes c.send -> writePump sends Close and exits
	<-writerDone
}
