package observability

import (
	"context"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

// SentryHandler wraps a slog.Handler and forwards every Error-level record to
// Sentry: if the record has an `error` attribute holding an error it is
// captured as an exception (with stack), otherwise as a message. This makes
// `log.Error("...", "error", err)` anywhere in the codebase observable
// without importing Sentry into the use-case layer.
type SentryHandler struct{ next slog.Handler }

func NewSentryHandler(next slog.Handler) *SentryHandler { return &SentryHandler{next: next} }

func (h *SentryHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *SentryHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			hub = sentry.CurrentHub()
		}
		var cause error
		fields := sentry.Context{"message": r.Message}
		r.Attrs(func(a slog.Attr) bool {
			if err, ok := a.Value.Any().(error); ok && a.Key == "error" {
				cause = err
			} else {
				fields[a.Key] = a.Value.String()
			}
			return true
		})
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetContext("log", fields)
			if cause != nil {
				hub.CaptureException(cause)
			} else {
				hub.CaptureMessage(r.Message)
			}
		})
	}
	return h.next.Handle(ctx, r)
}

func (h *SentryHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &SentryHandler{next: h.next.WithAttrs(a)}
}

func (h *SentryHandler) WithGroup(n string) slog.Handler {
	return &SentryHandler{next: h.next.WithGroup(n)}
}
