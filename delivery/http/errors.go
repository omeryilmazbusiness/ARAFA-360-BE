package http

import (
	"errors"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"

	"github.com/arafat-360/arafa-360-be/domain"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorHandler is the single place where errors become HTTP responses.
// Domain errors map to 4xx/503 with their own (client-safe) message. Anything
// else is an unexpected failure: it is reported to Sentry with full context
// and the client only sees a generic 500.
func ErrorHandler(c *fiber.Ctx, err error) error {
	status, code, msg := fiber.StatusInternalServerError, "internal_error", "internal server error"

	var fe *fiber.Error
	var de *domain.Error
	switch {
	case errors.As(err, &de):
		switch {
		case errors.Is(de.Kind, domain.ErrInvalid):
			status, code = fiber.StatusBadRequest, "invalid_request"
		case errors.Is(de.Kind, domain.ErrNotFound):
			status, code = fiber.StatusNotFound, "not_found"
		case errors.Is(de.Kind, domain.ErrConflict):
			status, code = fiber.StatusConflict, "conflict"
		case errors.Is(de.Kind, domain.ErrForbidden):
			status, code = fiber.StatusForbidden, "forbidden"
		case errors.Is(de.Kind, domain.ErrOverloaded):
			status, code = fiber.StatusServiceUnavailable, "unavailable"
			c.Set(fiber.HeaderRetryAfter, "1")
		}
		msg = de.Msg
	case errors.As(err, &fe):
		status, msg = fe.Code, fe.Message
		code = "http_error"
	}

	// 5xx are our fault: capture the real error (incl. wrapped causes).
	if status >= 500 {
		report(c, err)
	}

	return c.Status(status).JSON(errorBody{Error: errorDetail{
		Code: code, Message: msg, RequestID: c.GetRespHeader(fiber.HeaderXRequestID),
	}})
}

func report(c *fiber.Ctx, err error) {
	hub := sentryfiber.GetHubFromContext(c)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.CaptureException(err)
}
