package monitoring

// This file replaces the original jaeger-based tracing manager with a
// thin compatibility shim over internal/otel. The legacy symbols
// (TracingConfig, TracingManager, NewTracingManager, StartSpan,
// StartWorkflowSpan, StartNodeSpan, StartHTTPSpan,
// StartDatabaseSpan, RecordError, AddEvent, SetAttributes,
// ExtractSpanContext, InjectSpanContext, GetTracer) are preserved so
// plugins and downstream callers that imported them keep building.
//
// The pre-existing implementation depended on opentelemetry's deprecated
// jaeger exporter, which is incompatible with the OTLP-only pipeline
// n8n documents. m9m's tracing now lives in internal/otel — this file
// is purely a back-compat wrapper.

import (
	"context"
	"fmt"

	"github.com/neul-labs/m9m/internal/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TracingConfig is the legacy configuration struct. The new
// configuration model is otel.Config; this shim translates the
// legacy fields. SamplingRate == 0 falls back to the otel package
// default; ExporterType == "jaeger" is still accepted (we map it to
// OTLP over gRPC, since the jaeger collector is wire-compatible).
type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	// "jaeger", "otlp" or "stdout" (stdout is mapped to no-op).
	// "otlp" picks gRPC when Endpoint looks like host:port and http/protobuf
	// when it starts with http://.
	ExporterType string
	Endpoint     string
	SamplingRate float64
}

// toOTelConfig translates TracingConfig to otel.Config. Returns the
// translated config and any error the otel package would raise when
// validating it.
func (c TracingConfig) toOTelConfig() otel.Config {
	protocol := "grpc"
	endpoint := c.Endpoint
	// If the legacy caller gave us an http endpoint, pick up the
	// http/protobuf convention so the migration stories match.
	if len(endpoint) >= 7 && (endpoint[:7] == "http://" || endpoint[:8] == "https://") {
		protocol = "http/protobuf"
	} else if c.ExporterType == "jaeger" {
		// jaeger exporters historically use port 14250 for gRPC and 14268
		// for HTTP; default to gRPC with the legacy OTLP port so the
		// UI keeps showing data.
		if endpoint == "" {
			endpoint = "localhost:4317"
		}
	}
	return otel.Config{
		Enabled:        true,
		Endpoint:       endpoint,
		Protocol:       protocol,
		SampleRate:     c.SamplingRate,
		ServiceName:    c.ServiceName,
		ServiceVersion: c.ServiceVersion,
		InstanceID:     c.Environment,
		IncludeNodeSpans: true,
		InjectOutbound: true,
	}
}

// TracingManager is the legacy wrapper. Every method delegates to the
// embedded *otel.Manager.
type TracingManager struct {
	otel     *otel.Manager
	tracer   oteltrace.Tracer
	config   TracingConfig
}

// NewTracingManager constructs a TracingManager from a TracingConfig.
// On errors the returned Manager still satisfies the wrapper surface;
// callers should follow up with the otel package via Tracer() to
// check IsEnabled.
func NewTracingManager(config TracingConfig) (*TracingManager, error) {
	cfg := config.toOTelConfig()
	mgr, err := otel.NewManager(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("monitoring: new tracing manager: %w", err)
	}
	return &TracingManager{
		otel:   mgr,
		tracer: mgr.Tracer(),
		config: config,
	}, nil
}

// Shutdown flushes and tears down the tracer provider.
func (tm *TracingManager) Shutdown(ctx context.Context) error {
	return tm.otel.Shutdown(ctx)
}

// StartSpan opens a generic span. Kept for back-compat — new callers
// should reach for otel.Manager.StartWorkflowSpan / StartNodeSpan /
// StartAgentSpan / StartToolSpan.
func (tm *TracingManager) StartSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return tm.tracer.Start(ctx, spanName, opts...)
}

// StartWorkflowSpan opens a workflow.execute-style span via the
// embedded otel.Manager.
func (tm *TracingManager) StartWorkflowSpan(ctx context.Context, workflowID, workflowName string) (context.Context, oteltrace.Span) {
	return tm.otel.StartWorkflowSpan(ctx, otel.WorkflowAttrs{
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
	})
}

// StartNodeSpan opens a node.execute-style span via the embedded
// otel.Manager.
func (tm *TracingManager) StartNodeSpan(ctx context.Context, nodeType, nodeName string) (context.Context, oteltrace.Span) {
	return tm.otel.StartNodeSpan(ctx, otel.NodeAttrs{
		NodeID:   nodeName,
		NodeName: nodeName,
		NodeType: nodeType,
	})
}

// StartHTTPSpan opens a span for an outbound HTTP call. Delegates to
// the OTel global tracer so the result chains onto the active span.
func (tm *TracingManager) StartHTTPSpan(ctx context.Context, method, url string) (context.Context, oteltrace.Span) {
	ctx, span := tm.tracer.Start(ctx, fmt.Sprintf("http.%s", method))
	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.url", url),
	)
	return ctx, span
}

// StartDatabaseSpan opens a span for a database call.
func (tm *TracingManager) StartDatabaseSpan(ctx context.Context, dbType, operation, query string) (context.Context, oteltrace.Span) {
	ctx, span := tm.tracer.Start(ctx, fmt.Sprintf("db.%s.%s", dbType, operation))
	span.SetAttributes(
		attribute.String("db.system", dbType),
		attribute.String("db.operation", operation),
		attribute.String("db.statement", query),
	)
	return ctx, span
}

// RecordError stamps an error on the span.
func (tm *TracingManager) RecordError(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// AddEvent attaches a named event with attributes to the span.
func (tm *TracingManager) AddEvent(span oteltrace.Span, name string, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	span.AddEvent(name, oteltrace.WithAttributes(attrs...))
}

// SetAttributes replaces (adds) attributes on the span.
func (tm *TracingManager) SetAttributes(span oteltrace.Span, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	span.SetAttributes(attrs...)
}

// ExtractSpanContext extracts the upstream SpanContext from a carrier.
func (tm *TracingManager) ExtractSpanContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return tm.otel.Extract(ctx, carrier)
}

// InjectSpanContext writes the active SpanContext into a carrier.
func (tm *TracingManager) InjectSpanContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	tm.otel.Inject(ctx, carrier)
}

// GetTracer returns the underlying oteltrace.Tracer.
func (tm *TracingManager) GetTracer() oteltrace.Tracer {
	return tm.tracer
}
