// Package observability wires glossa-server's logs, traces and metrics
// (RFC 0002 §11): bolt as the slog handler, OpenTelemetry for traces,
// Prometheus for metrics, and the HTTP middleware that ties a request's
// log lines, span and metrics together with one correlation ID.
package observability

import (
	"context"
	"io"
	"log/slog"

	"go.klarlabs.de/bolt"
	"go.opentelemetry.io/otel/trace"
)

// ContextAttrs extracts log attributes from a context, such as the
// tenant ID. The composition root supplies these so this package stays
// independent of the bounded contexts.
type ContextAttrs func(context.Context) []slog.Attr

// NewLogger returns a JSON logger on bolt's slog handler. Every record
// logged with a context gains trace_id/span_id, request_id and whatever
// the extractors return.
func NewLogger(w io.Writer, level slog.Leveler, extractors ...ContextAttrs) *slog.Logger {
	inner := bolt.NewSlogHandler(w, &bolt.SlogHandlerOptions{Level: level})
	return slog.New(&contextHandler{inner: inner, extractors: extractors})
}

// contextHandler adds correlation fields from the context. bolt's slog
// handler ignores the context, so trace correlation happens here.
type contextHandler struct {
	inner      slog.Handler
	extractors []ContextAttrs
}

func (h *contextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := correlationAttrs(ctx)
	for _, extract := range h.extractors {
		attrs = append(attrs, extract(ctx)...)
	}
	if len(attrs) > 0 {
		r = r.Clone()
		r.AddAttrs(attrs...)
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs), extractors: h.extractors}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{inner: h.inner.WithGroup(name), extractors: h.extractors}
}

func correlationAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	var attrs []slog.Attr
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()))
	}
	if id, ok := RequestIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("request_id", id))
	}
	return attrs
}
