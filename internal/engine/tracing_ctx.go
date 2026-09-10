package engine

import (
	"context"

	"github.com/neul-labs/m9m/internal/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ctxKey is a private type to avoid collisions with other context keys in
// the engine / api / webhooks packages.
type ctxKey int

const (
	ctxWorkflowSpan ctxKey = iota
	ctxExecutionMode
	ctxExecutionID
	ctxExecutionIsRetry
	ctxExecutionRetryOf
	ctxContinuationSpan
)

// WithExecutionMode stores mode ("manual", "trigger", "webhook", "retry",
// "cli", "schedule") on ctx. Callers (webhook handler, scheduler, MCP)
// set this before invoking ExecuteWorkflowWithContext so the engine
// records it as m9m.execution.mode on the workflow.execute span.
func WithExecutionMode(ctx context.Context, mode string) context.Context {
	if mode == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxExecutionMode, mode)
}

// WithExecutionID stores the workflow execution id on ctx so it can be
// attached to the span as m9m.execution.id.
func WithExecutionID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxExecutionID, id)
}

// WithExecutionRetry records retry metadata so the span can carry
// m9m.execution.is_retry=true and m9m.execution.retry_of=<id>.
func WithExecutionRetry(ctx context.Context, retryOf string) context.Context {
	ctx = context.WithValue(ctx, ctxExecutionIsRetry, retryOf != "")
	if retryOf != "" {
		ctx = context.WithValue(ctx, ctxExecutionRetryOf, retryOf)
	}
	return ctx
}

// WithContinuationSpan stores the previous workflow span so the new
// workflow.execute can link to it (used after Wait / pause).
func WithContinuationSpan(ctx context.Context, sc trace.SpanContext) context.Context {
	if !sc.IsValid() {
		return ctx
	}
	return context.WithValue(ctx, ctxContinuationSpan, sc)
}

// workflowExecutionMode reads the mode attached via WithExecutionMode.
// Returns "" when unset.
func workflowExecutionMode(ctx context.Context) string {
	if v, ok := ctx.Value(ctxExecutionMode).(string); ok {
		return v
	}
	return ""
}

func workflowExecutionID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxExecutionID).(string); ok {
		return v
	}
	return ""
}

func workflowExecutionIsRetry(ctx context.Context) bool {
	if v, ok := ctx.Value(ctxExecutionIsRetry).(bool); ok {
		return v
	}
	return false
}

func workflowExecutionRetryOf(ctx context.Context) string {
	if v, ok := ctx.Value(ctxExecutionRetryOf).(string); ok {
		return v
	}
	return ""
}

func workflowContinuationSpan(ctx context.Context) trace.SpanContext {
	if v, ok := ctx.Value(ctxContinuationSpan).(trace.SpanContext); ok {
		return v
	}
	return trace.SpanContext{}
}

// withWorkflowSpan stashes the active workflow.execute span on ctx so
// per-node spans can attach as children, and so the deferred cleanup
// can find the span to End.
func withWorkflowSpan(ctx context.Context, span trace.Span) context.Context {
	if span == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxWorkflowSpan, span)
}

// workflowSpanFromContext returns the workflow span attached by
// withWorkflowSpan, or nil if there isn't one.
func workflowSpanFromContext(ctx context.Context) trace.Span {
	if v, ok := ctx.Value(ctxWorkflowSpan).(trace.Span); ok {
		return v
	}
	return nil
}

// spanFromContext returns the active OTel span on ctx (set by
// StartWorkflowSpan). The Engine uses this to record its own span so
// per-node spans can chain off it via trace.ContextWithSpan.
func spanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// mergeNodeCustomAttrs returns a single map containing the static
// customSpanAttributes from model.Node. Programmatic metadata
// contributed via the MetadataAwareNodeExecutor interface is merged
// in by the engine loop, AFTER the executor runs, so programmatic
// values overwrite the static config on key collisions.
func mergeNodeCustomAttrs(node *model.Node) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range node.CustomSpanAttributes {
		out[k] = v
	}
	return out
}

// mergeProgrammaticNodeAttrs overwrites keys in static with the values
// from programmatic. Both may be nil; returns a fresh map.
func mergeProgrammaticNodeAttrs(static, programmatic map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(static)+len(programmatic))
	for k, v := range static {
		out[k] = v
	}
	for k, v := range programmatic {
		out[k] = v
	}
	return out
}

// attributeFromValue is a tiny adapter that mirrors attribute.String /
// Bool / Int / Float64 picking based on the value's dynamic type.
// Anything that doesn't match a primitive shape falls back to a string
// rendition. We keep this helper here (instead of importing the OTel
// attribute package widely) so the engine package stays narrow.
func attributeFromValue(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case string:
		return attribute.String(key, v)
	default:
		return attribute.String(key, "")
	}
}
