// Package domain holds the enterprise rules of ARAFA-360: entities, value
// objects and the ports (interfaces) the outer layers implement.
// It must not import Fiber, pgx, Redis or Sentry.
package domain

import (
	"errors"
	"fmt"
)

// Sentinel kinds. Delivery maps these to transport status codes (HTTP/WS),
// so use cases never need to know about HTTP.
var (
	ErrInvalid    = errors.New("invalid")
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrForbidden  = errors.New("forbidden")
	ErrOverloaded = errors.New("overloaded")
)

// Error is a domain error: a Kind (one of the sentinels above) plus a
// message that is safe to show to API clients.
//
// Cause optionally carries the underlying infrastructure error. It is never
// shown to clients but stays reachable via errors.Is/As for logging and Sentry.
type Error struct {
	Kind  error
	Msg   string
	Cause error
}

func (e *Error) Error() string { return e.Msg }

func (e *Error) Unwrap() []error {
	if e.Cause != nil {
		return []error{e.Kind, e.Cause}
	}
	return []error{e.Kind}
}

func Invalid(format string, a ...any) error {
	return &Error{Kind: ErrInvalid, Msg: fmt.Sprintf(format, a...)}
}

func NotFound(format string, a ...any) error {
	return &Error{Kind: ErrNotFound, Msg: fmt.Sprintf(format, a...)}
}

func Conflict(format string, a ...any) error {
	return &Error{Kind: ErrConflict, Msg: fmt.Sprintf(format, a...)}
}

func Overloaded(format string, a ...any) error {
	return &Error{Kind: ErrOverloaded, Msg: fmt.Sprintf(format, a...)}
}

func Forbidden(format string, a ...any) error {
	return &Error{Kind: ErrForbidden, Msg: fmt.Sprintf(format, a...)}
}

// OverloadedBy is Overloaded with the underlying cause attached.
func OverloadedBy(cause error, format string, a ...any) error {
	return &Error{Kind: ErrOverloaded, Msg: fmt.Sprintf(format, a...), Cause: cause}
}
