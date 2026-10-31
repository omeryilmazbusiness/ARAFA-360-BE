package websocket

import (
	"time"

	fastws "github.com/fasthttp/websocket"
	"github.com/google/uuid"
)

const (
	writeWait    = 10 * time.Second
	pongWait     = 60 * time.Second
	pingInterval = 30 * time.Second // must be < pongWait
	maxReadBytes = 1024             // dashboards only listen; inbound frames are tiny
)

// Client is one dashboard connection. The hub owns `send` (and closes it);
// writePump is the only goroutine that writes to the socket.
type Client struct {
	tenantID uuid.UUID
	conn     *fastws.Conn // nil in unit tests
	send     chan []byte
}

func newClient(tenantID uuid.UUID, conn *fastws.Conn) *Client {
	return &Client{tenantID: tenantID, conn: conn, send: make(chan []byte, clientSendSize)}
}

// readPump discards inbound data but must run: it processes pong/close
// control frames and enforces the liveness deadline. It returns when the
// peer disconnects, misses a pong, or misbehaves.
func (c *Client) readPump() {
	c.conn.SetReadLimit(maxReadBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump drains `send` to the socket and sends periodic pings. It exits
// (closing the socket) on write error or when the hub closes `send`.
func (c *Client) writePump(done chan<- struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
		close(done)
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(fastws.CloseMessage,
					fastws.FormatCloseMessage(fastws.CloseGoingAway, "server closing"))
				return
			}
			if err := c.conn.WriteMessage(fastws.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(fastws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
