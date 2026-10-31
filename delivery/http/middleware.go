package http

import (
	"errors"
	"fmt"

	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
	"github.com/arafat-360/arafa-360-be/usecase"
)

// TenantKey is the Locals key holding the request's tenant (uuid.UUID).
const TenantKey = "tenant_id"

const tenantHeader = "X-Tenant-ID"

// Tenant identifies and validates the calling agency and stores it in Locals.
//
// SECURITY: the tenant currently comes from a client-supplied header. That is
// a placeholder for development: before production, derive it from a verified
// credential (JWT claim / API key) here, and ignore the header. This is the
// only place that has to change; handlers and use cases never read headers.
//
// allowQuery additionally accepts ?tenant_id=, for browsers, whose WebSocket
// API cannot set headers.
func Tenant(tenants *usecase.TenantUsecase, allowQuery bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw := c.Get(tenantHeader)
		if raw == "" && allowQuery {
			raw = c.Query("tenant_id")
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "missing or invalid tenant")
		}
		t, err := tenants.Resolve(c.UserContext(), id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) { // don't reveal which tenant ids exist
				return fiber.NewError(fiber.StatusUnauthorized, "missing or invalid tenant")
			}
			return err
		}
		c.Locals(TenantKey, t.ID)
		if hub := sentryfiber.GetHubFromContext(c); hub != nil {
			hub.Scope().SetTag("tenant_id", t.ID.String())
			hub.Scope().SetTag("tenant_slug", t.Slug)
		}
		return c.Next()
	}
}

func tenantID(c *fiber.Ctx) uuid.UUID {
	id, _ := c.Locals(TenantKey).(uuid.UUID)
	return id
}

// errorBoundary renders errors inside the Sentry transaction, so the trace's
// http.response.status_code reflects the real status (Fiber v2 otherwise
// calls the ErrorHandler only after all middleware has unwound).
func errorBoundary(c *fiber.Ctx) error {
	if err := c.Next(); err != nil {
		return ErrorHandler(c, err)
	}
	return nil
}

func parseUUID(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.Invalid("%s must be a UUID", field)
	}
	return id, nil
}

func bindJSON(c *fiber.Ctx, dst any) error {
	if err := c.BodyParser(dst); err != nil {
		return domain.Invalid("%s", fmt.Sprintf("invalid JSON body: %v", err))
	}
	return nil
}
