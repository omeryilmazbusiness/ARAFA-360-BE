// Package observability wires Sentry (errors, panics, performance traces).
package observability

import (
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

type SentryConfig struct {
	DSN              string
	Environment      string
	Release          string
	ServerName       string
	TracesSampleRate float64
	Debug            bool
}

// ignoredTransactions never produce traces: probes would burn the quota.
var ignoredTransactions = []string{"GET /healthz", "GET /readyz", "GET /ws"}

// InitSentry configures the global hub. An empty DSN yields a disabled
// (no-op) client, so local development needs no Sentry project.
// The returned func flushes buffered events; defer it in main.
func InitSentry(c SentryConfig) (flush func(), err error) {
	err = sentry.Init(sentry.ClientOptions{
		Dsn:              c.DSN,
		Environment:      c.Environment,
		Release:          c.Release,
		ServerName:       c.ServerName,
		Debug:            c.Debug,
		AttachStacktrace: true,
		EnableTracing:    true,
		// Parent-based: honour an upstream sampling decision, otherwise sample
		// at TracesSampleRate. Probe endpoints are never sampled.
		TracesSampler: func(ctx sentry.SamplingContext) float64 {
			if ctx.Span != nil {
				for _, name := range ignoredTransactions {
					if strings.HasPrefix(ctx.Span.Name, name) {
						return 0
					}
				}
			}
			if ctx.Parent != nil && ctx.Parent.Sampled != sentry.SampledUndefined {
				if ctx.Parent.Sampled == sentry.SampledTrue {
					return 1
				}
				return 0
			}
			return c.TracesSampleRate
		},
		BeforeSend: func(ev *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// Never ship credentials that might sit in headers.
			if ev.Request != nil {
				for _, h := range []string{"Authorization", "Cookie", "X-Api-Key"} {
					delete(ev.Request.Headers, h)
				}
			}
			return ev
		},
	})
	if err != nil {
		return func() {}, err
	}
	return func() { sentry.Flush(5 * time.Second) }, nil
}
