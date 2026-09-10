package otel

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Extract reads a remote SpanContext from carrier and returns a context
// that carries it. Safe on a nil Manager (returns ctx unchanged).
//
// The carrier is expected to be a TextMapCarrier (typically a HTTP
// header). Callers usually pass an http.Header wrapped via
// propagation.HeaderCarrier (the package exposes a small helper for
// that, see httpCarrier).
func (m *Manager) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if m == nil || carrier == nil {
		return ctx
	}
	return m.propagator.Extract(ctx, carrier)
}

// Inject writes the active SpanContext on ctx into carrier so downstream
// services can continue the trace. Safe on a nil Manager (no-op).
func (m *Manager) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if m == nil || carrier == nil {
		return
	}
	m.propagator.Inject(ctx, carrier)
}

// ExtractFromRequest reads the traceparent / baggage headers off an
// incoming HTTP request and returns a context that carries the upstream
// SpanContext. The engine uses this on webhook ingress to chain the
// m9m workflow span onto the caller's trace.
func (m *Manager) ExtractFromRequest(ctx context.Context, r *http.Request) context.Context {
	if m == nil || r == nil {
		return ctx
	}
	return m.Extract(ctx, propagation.HeaderCarrier(r.Header))
}

// InjectIntoRequest writes the active trace context into an outbound
// HTTP request. The HTTP Request node uses this just before sending
// when cfg.InjectOutbound is true.
func (m *Manager) InjectIntoRequest(ctx context.Context, r *http.Request) {
	if m == nil || r == nil {
		return
	}
	m.Inject(ctx, propagation.HeaderCarrier(r.Header))
}

// GlobalExtract / GlobalInject route through the otel package's global
// propagator. They are useful when the caller doesn't hold a *Manager
// (e.g. inside the webhook handler that has only the request in hand).
// Safe even when no Manager is configured — they fall through to the
// package-global propagator which the SDK initialises to a no-op.
func GlobalExtract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if carrier == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func GlobalInject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if carrier == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// HeaderCarrier is a tiny shim over http.Header implementing
// propagation.TextMapCarrier. It exists so the AI / HTTP request nodes
// can write `otel.HeaderCarrier(req.Header)` without having to import
// the OTel propagation package directly (and without accidentally
// wrapping the wrong type — http.Header is a map, not the carrier).
type HeaderCarrier http.Header

func (h HeaderCarrier) Get(key string) string {
	return http.Header(h).Get(key)
}

func (h HeaderCarrier) Set(key, value string) {
	http.Header(h).Set(key, value)
}

func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}
