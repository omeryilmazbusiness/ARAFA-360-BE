package http

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/delivery/websocket"
	"github.com/arafat-360/arafa-360-be/domain"
	"github.com/arafat-360/arafa-360-be/usecase"
)

type fakeTenants struct{ t domain.Tenant }

func (f fakeTenants) GetByID(_ context.Context, id uuid.UUID) (domain.Tenant, error) {
	if id != f.t.ID {
		return domain.Tenant{}, domain.NotFound("tenant not found")
	}
	return f.t, nil
}

type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *captureTransport) Configure(sentry.ClientOptions) {}
func (c *captureTransport) SendEvent(e *sentry.Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}
func (c *captureTransport) Flush(time.Duration) bool              { return true }
func (c *captureTransport) FlushWithContext(context.Context) bool { return true }
func (c *captureTransport) Close()                                {}

func newTestApp(t *testing.T, status domain.TenantStatus) (*testApp, uuid.UUID) {
	t.Helper()
	tr := &captureTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "http://k@localhost/1", Transport: tr}); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tenant := domain.Tenant{ID: uuid.New(), Slug: "t", Status: status}
	tenants := usecase.NewTenantUsecase(fakeTenants{tenant}, time.Minute)
	app := NewApp(Config{BodyLimitBytes: 1 << 20}, Deps{
		Log:     log,
		Tenants: tenants,
		// Inventory is deliberately nil: the handler dereferences it and panics.
		Handlers:  &Handlers{},
		WebSocket: websocket.NewHandler(websocket.NewHub(log, 1), websocket.HandlerConfig{TenantLocalsKey: TenantKey}),
	})
	return &testApp{app: app, tr: tr}, tenant.ID
}

type testApp struct {
	app *fiber.App
	tr  *captureTransport
}

func TestHandlerPanicBecomes500AndIsCapturedBySentry(t *testing.T) {
	ta, tenantID := newTestApp(t, domain.TenantActive)

	req := httptest.NewRequest("POST", "/api/v1/zones", strings.NewReader(
		`{"name":"x","coordinates_polygon":[{"lat":1,"lng":1},{"lat":1,"lng":2},{"lat":2,"lng":2}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(tenantHeader, tenantID.String())
	resp, err := ta.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 500 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 500: %s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "internal server error") || strings.Contains(string(body), "nil pointer") {
		t.Fatalf("body leaks internals or is wrong: %s", body)
	}
	sentry.Flush(time.Second)
	ta.tr.mu.Lock()
	defer ta.tr.mu.Unlock()
	if len(ta.tr.events) == 0 {
		t.Fatal("panic was not reported to Sentry")
	}
}

func TestTenantGuard(t *testing.T) {
	for name, tc := range map[string]struct {
		status   domain.TenantStatus
		header   func(uuid.UUID) string
		wantCode int
	}{
		"missing header":   {domain.TenantActive, func(uuid.UUID) string { return "" }, 401},
		"garbage header":   {domain.TenantActive, func(uuid.UUID) string { return "nope" }, 401},
		"unknown tenant":   {domain.TenantActive, func(uuid.UUID) string { return uuid.NewString() }, 401},
		"suspended tenant": {domain.TenantSuspended, func(id uuid.UUID) string { return id.String() }, 403},
	} {
		t.Run(name, func(t *testing.T) {
			ta, id := newTestApp(t, tc.status)
			req := httptest.NewRequest("POST", "/api/v1/zones", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if h := tc.header(id); h != "" {
				req.Header.Set(tenantHeader, h)
			}
			resp, err := ta.app.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantCode)
			}
		})
	}
}

func TestProbesNeedNoTenant(t *testing.T) {
	ta, _ := newTestApp(t, domain.TenantActive)
	resp, err := ta.app.Test(httptest.NewRequest("GET", "/healthz", nil), -1)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", err, resp)
	}
}
