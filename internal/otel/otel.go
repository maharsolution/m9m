/*
Package otel wires OpenTelemetry tracing into m9m.

It exposes:

  - Config + LoadConfigFromEnv(): env-driven configuration aligned with the
    n8n N8N_OTEL_* / N8N_AGENTS_TRACING_* variable names, but with the
    M9M_ prefix so the integration is vendor-neutral and can co-deploy with
    an n8n instance.
  - Manager: owns the TracerProvider lifecycle, the global propagator, and
    shutdown. A nil Manager is a valid no-op (every helper short-circuits),
    so callers can leave tracing disabled without sprinkling nil checks.
  - Helpers for workflow.execute / node.execute / agent / tool spans
    (see spans.go) and attribute builders (see attributes.go).

The integration is strictly standard OpenTelemetry: OTLP over
http/protobuf (default) or grpc, W3C Trace Context propagation, and
gen_ai.* GenAI semantic conventions. Any OTLP-compatible backend (SigNoz,
Jaeger, Tempo, Honeycomb, Datadog, etc.) can ingest the spans without
modification.
*/
package otel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ServiceName is the value exported as service.name on every span.
const ServiceName = "m9m"

// RoleMain / RoleWebhook identify the m9m instance role, mirroring n8n's
// main / worker / webhook taxonomy. The m9m v1 binary serves both the API
// and the webhook ingress in a single process, so the default is RoleMain;
// the webhook handler promotes the role on its incoming requests (see
// attributes.go SetInstanceRole).
const (
	RoleMain    = "main"
	RoleWebhook = "webhook"
)

// Config is the runtime configuration of the OTEL pipeline. Values are
// typically loaded from environment variables via LoadConfigFromEnv but can
// also be set programmatically for tests.
type Config struct {
	// Enabled is the master switch. When false, NewManager returns a nil
	// Manager and every helper is a no-op.
	Enabled bool

	// Endpoint is the OTLP collector base URL (no /v1/traces suffix).
	// The exporter appends the correct path for the chosen Protocol.
	Endpoint string

	// Protocol is one of "http/protobuf" or "grpc". Empty defaults to
	// "http/protobuf" to match the upstream OTel Go default.
	Protocol string

	// Headers are the comma-separated k=v pairs sent as headers (http)
	// or gRPC metadata (grpc). Optional.
	Headers string

	// HeadersFile is the path to a file containing the same k=v pairs.
	// When non-empty, takes precedence over Headers.
	HeadersFile string

	// SampleRate is a 0.0 - 1.0 ratio passed to the TraceIDRatioBased
	// sampler. Values outside the range are clamped.
	SampleRate float64

	// ProductionOnly mirrors n8n's behaviour: when true, executions
	// whose mode is not "webhook", "trigger", "manual" or "retry" are
	// sampled with NeverSample. Production modes always pass through.
	ProductionOnly bool

	// IncludeNodeSpans is the span-volume toggle. When false, the
	// engine skips node.execute spans and only emits workflow.execute.
	IncludeNodeSpans bool

	// InjectOutbound controls whether the HTTP Request node injects a
	// traceparent header on outbound requests.
	InjectOutbound bool

	// ServiceName overrides the default service.name on the resource.
	// Empty keeps the package default ("m9m").
	ServiceName string

	// ServiceVersion populates service.version on the resource. The
	// server bootstrap wires the build-time version into this field.
	ServiceVersion string

	// InstanceID identifies the m9m instance. Defaults to a generated
	// UUID at startup so each binary gets a stable id for the process.
	InstanceID string

	// AgentsEnabled toggles GenAI agent + tool-call spans.
	AgentsEnabled bool

	// AgentsRecordInputs controls whether prompts / tool args are
	// recorded as gen_ai.prompt / gen_ai.tool.call.arguments.
	AgentsRecordInputs bool

	// AgentsRecordOutputs controls whether completions / tool results
	// are recorded as gen_ai.completion / gen_ai.tool.call.result.
	AgentsRecordOutputs bool
}

// ApplyDefaults fills in zero-valued fields with the documented defaults.
// It is called by LoadConfigFromEnv and by NewManager so callers never
// have to worry about zero-valued configs slipping through.
func (c *Config) ApplyDefaults() {
	if c.Protocol == "" {
		c.Protocol = "http/protobuf"
	}
	if c.Endpoint == "" {
		if c.Protocol == "grpc" {
			c.Endpoint = "http://localhost:4317"
		} else {
			c.Endpoint = "http://localhost:4318"
		}
	}
	if c.SampleRate < 0 {
		c.SampleRate = 0
	}
	if c.SampleRate > 1 {
		c.SampleRate = 1
	}
	if c.ServiceName == "" {
		c.ServiceName = ServiceName
	}
	if c.InstanceID == "" {
		c.InstanceID = generateInstanceID()
	}
}

// Manager owns the OpenTelemetry TracerProvider and the configured
// propagator. Construct one via NewManager and call Shutdown on shutdown.
//
// A nil *Manager is a valid no-op: every helper on Manager (StartWorkflowSpan,
// StartNodeSpan, Inject, Extract, ...) accepts a nil receiver and returns
// the input context unchanged. This lets callers in the engine and node
// code stay branch-free whether tracing is enabled or not.
type Manager struct {
	mu           sync.RWMutex
	cfg          Config
	tp           *sdktrace.TracerProvider
	propagator   propagation.TextMapPropagator
	tracer       oteltrace.Tracer
	resource     *sdkresource.Resource
	shutdownOnce sync.Once
	shutdownErr  error
}

// NewManager builds a Manager from cfg. When cfg.Enabled is false the
// returned Manager is nil; callers should treat that as "no tracing".
func NewManager(ctx context.Context, cfg Config) (*Manager, error) {
	cfg.ApplyDefaults()

	if !cfg.Enabled {
		return nil, nil
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	exporter, err := buildExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("otel: build exporter: %w", err)
	}

	sampler := buildSampler(cfg)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(prop)

	m := &Manager{
		cfg:        cfg,
		tp:         tp,
		propagator: prop,
		tracer:     tp.Tracer(cfg.ServiceName, oteltrace.WithInstrumentationAttributes(attribute.String("m9m.instance.id", cfg.InstanceID))),
		resource:   res,
	}
	return m, nil
}

// Shutdown flushes any pending spans and releases the TracerProvider. It
// is safe to call multiple times. Safe to call on a nil receiver.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.shutdownOnce.Do(func() {
		if m.tp == nil {
			return
		}
		// Give the exporter a small grace period for the final flush.
		flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		m.shutdownErr = m.tp.Shutdown(flushCtx)
	})
	return m.shutdownErr
}

// Reload atomically swaps in a new TracerProvider built from cfg. The
// previous provider is flushed and shut down; in-flight spans complete
// before shutdown returns. After Reload returns the new tracer is
// registered as the OTel global tracer provider.
//
// Reload is the runtime side of the UI "save settings" button: the
// API handler calls Reload after persisting the new override so the
// change takes effect immediately, with no process restart.
//
// Safe on a nil receiver (returns nil).
func (m *Manager) Reload(ctx context.Context, cfg Config) error {
	if m == nil {
		return nil
	}
	cfg.ApplyDefaults()

	// If the operator just turned tracing off, swap to a no-op tracer
	// rather than tearing the provider down — that way callers in the
	// engine can keep the same *Manager handle.
	if !cfg.Enabled {
		newTracer := oteltrace.NewNoopTracerProvider().Tracer(cfg.ServiceName)
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		m.mu.Lock()
		oldTP := m.tp
		m.tp = nil
		m.tracer = newTracer
		m.cfg = cfg
		m.mu.Unlock()
		if oldTP != nil {
			flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return oldTP.Shutdown(flushCtx)
		}
		return nil
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return fmt.Errorf("otel: reload build resource: %w", err)
	}
	exporter, err := buildExporter(ctx, cfg)
	if err != nil {
		return fmt.Errorf("otel: reload build exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildSampler(cfg)),
	)

	m.mu.Lock()
	oldTP := m.tp
	m.tp = tp
	m.tracer = tp.Tracer(cfg.ServiceName, oteltrace.WithInstrumentationAttributes(attribute.String("m9m.instance.id", cfg.InstanceID)))
	m.cfg = cfg
	m.resource = res
	m.mu.Unlock()

	otel.SetTracerProvider(tp)

	if oldTP != nil {
		flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := oldTP.Shutdown(flushCtx); err != nil {
			// Don't fail the reload — the new provider is up.
			return fmt.Errorf("otel: shutdown previous provider: %w", err)
		}
	}
	return nil
}

// Config returns the configuration the Manager was built with. Safe on a
// nil receiver (returns the zero Config).
func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Tracer returns the underlying oteltrace.Tracer. Safe on a nil receiver.
func (m *Manager) Tracer() oteltrace.Tracer {
	if m == nil {
		return oteltrace.NewNoopTracerProvider().Tracer("m9m/noop")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tracer
}

// SetInstanceRole returns an attribute.Option that records the role of
// the current instance (main vs webhook). Use it on per-span SetAttributes
// where the role matters (the workflow span on a webhook ingress).
func (m *Manager) SetInstanceRole(role string) attribute.KeyValue {
	return attribute.String("m9m.instance.role", role)
}

// IsEnabled reports whether tracing is active.
func (m *Manager) IsEnabled() bool { return m != nil }

// ErrShutdown is returned by Shutdown when the underlying TracerProvider
// reports a non-nil error during shutdown. Wrapped so callers can use
// errors.Is to match.
var ErrShutdown = errors.New("otel: tracer provider shutdown failed")
