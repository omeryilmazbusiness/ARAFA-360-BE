package observability

import (
	"context"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
)

// PGXTracer records every query executed inside a traced request as a
// `db.query` span, and reports query errors on that span. Queries outside a
// sampled transaction cost one context lookup.
type PGXTracer struct{}

type spanKey struct{}

func (PGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	parent := sentry.TransactionFromContext(ctx)
	if parent == nil || !parent.IsTransaction() || parent.Sampled != sentry.SampledTrue {
		return ctx
	}
	span := sentry.StartSpan(ctx, "db.query", sentry.WithDescription(data.SQL))
	span.SetData("db.system", "postgresql")
	return context.WithValue(span.Context(), spanKey{}, span)
}

func (PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanKey{}).(*sentry.Span)
	if !ok {
		return
	}
	if data.Err != nil {
		span.Status = sentry.SpanStatusInternalError
	} else {
		span.Status = sentry.SpanStatusOK
	}
	span.Finish()
}
