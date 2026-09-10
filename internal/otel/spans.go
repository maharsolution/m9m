package otel

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SpanNameWorkflowExecute is the canonical span name for a workflow
// execution. Matches n8n's span name exactly.
const SpanNameWorkflowExecute = "workflow.execute"

// SpanNameNodeExecute is the canonical span name for a node execution.
const SpanNameNodeExecute = "node.execute"

// SpanNameAgentGenerate / SpanNameAgentStream / SpanNameExecuteTool
// follow the GenAI semantic conventions for agent runs. The agent name
// is interpolated into the span name so dashboards can group by agent.
const (
	SpanNameAgentGenerate = "%s.generate"
	SpanNameAgentStream   = "%s.stream"
	SpanNameExecuteTool   = "execute_tool %s"
)

// SpanKind defaults we use across the surface. workflow.execute is
// SpanKindInternal because m9m is the orchestrator; HTTP Request nodes
// that emit outbound spans use SpanKindClient (see nodes/http/request.go).
var (
	workflowSpanKind = oteltrace.SpanKindInternal
	nodeSpanKind     = oteltrace.SpanKindInternal
	agentSpanKind    = oteltrace.SpanKindInternal
	toolSpanKind     = oteltrace.SpanKindInternal
)

// StartWorkflowSpan opens a workflow.execute span attached to ctx. The
// returned context is the parent for any node.execute spans the engine
// opens afterwards.
//
// When the receiver is nil (tracing disabled) the function returns the
// input context and a no-op span so callers can stay branch-free.
//
// When cfg.IncludeNodeSpans is false the engine still opens the
// workflow span — only the per-node spans are skipped. The flag is
// consulted at the engine level (see internal/engine/engine.go) so the
// no-op span here still represents the workflow root.
func (m *Manager) StartWorkflowSpan(ctx context.Context, attrs WorkflowAttrs, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if m == nil {
		return ctx, oteltrace.SpanFromContext(ctx) // no-op
	}
	m.mu.RLock()
	cfg := m.cfg
	tracer := m.tracer
	m.mu.RUnlock()

	// Prepend the m9m.execution.mode attribute on the workflow root so
	// the productionOnlySampler can decide on the very first span of
	// the trace. Without this the sampler doesn't know what mode the
	// execution is in until later in the trace, which can let spans
	// through that should be dropped.
	startOpts := append([]oteltrace.SpanStartOption{
		oteltrace.WithSpanKind(workflowSpanKind),
		oteltrace.WithAttributes(
			AttrInstanceID.String(cfg.InstanceID),
			AttrInstanceRole.String(RoleMain),
			AttrExecutionMode.String(attrs.ExecutionMode),
		),
	}, opts...)

	ctx, span := tracer.Start(ctx, SpanNameWorkflowExecute, startOpts...)
	span.SetAttributes(attrs.BuildAttributes()...)
	return ctx, span
}

// StartNodeSpan opens a node.execute span as a child of the workflow
// span on ctx. Callers must pass the post-StartWorkflowSpan context in
// so the parent relationship is preserved.
//
// The cfg.IncludeNodeSpans flag gates whether this function actually
// opens a span. When false, it returns the input context and the active
// no-op span — so call-sites in the engine stay branch-free.
func (m *Manager) StartNodeSpan(ctx context.Context, attrs NodeAttrs, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if m == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	m.mu.RLock()
	includeNodes := m.cfg.IncludeNodeSpans
	tracer := m.tracer
	m.mu.RUnlock()
	if !includeNodes {
		return ctx, oteltrace.SpanFromContext(ctx)
	}

	startOpts := append([]oteltrace.SpanStartOption{
		oteltrace.WithSpanKind(nodeSpanKind),
	}, opts...)

	ctx, span := tracer.Start(ctx, SpanNameNodeExecute, startOpts...)
	span.SetAttributes(attrs.BuildAttributes()...)
	return ctx, span
}

// EndNodeSpan is a convenience wrapper that records err on span (if
// non-nil) and ends it. err is mapped to an exception event with the
// standard OTel exception attributes (exception.type, exception.message,
// exception.stacktrace). Safe on a nil / no-op span.
func EndNodeSpan(span oteltrace.Span, err error) {
	if span == nil || !span.SpanContext().IsValid() {
		return
	}
	if err != nil {
		recordException(span, err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// EndWorkflowSpan mirrors EndNodeSpan for the workflow span. It also
// accepts a status string so the engine can record the final execution
// state without rewriting the Status code by hand.
func EndWorkflowSpan(span oteltrace.Span, status string, err error) {
	if span == nil || !span.SpanContext().IsValid() {
		return
	}
	span.SetAttributes(AttrExecutionStatus.String(status))
	if err != nil {
		span.SetAttributes(AttrExecutionErrorType.String(errorTypeName(err)))
		recordException(span, err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// recordException writes an exception event onto span with the standard
// OTel exception attributes. The OTel SDK does this automatically when
// span.RecordError is called; this helper exists so the package can add
// a stable exception.type attribute without depending on the SDK's
// unexported helpers.
func recordException(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
}

// errorTypeName returns the Go type name for err, mirroring what n8n
// puts into n8n.execution.error_type (the JS error class name). We use
// the reflected concrete type name where possible and fall back to
// err.Error()'s first word.
func errorTypeName(err error) string {
	if err == nil {
		return ""
	}
	// errors.As handles wrapped errors.
	var as interface{ TypeName() string }
	if errors.As(err, &as) {
		return as.TypeName()
	}
	// fmt.Sprintf("%T", ...) returns the package-qualified type name
	// for non-pointer concrete types. For pointer types it returns
	// "*pkg.Type" — strip the asterisk to match n8n's convention.
	return trimPtr(fmt.Sprintf("%T", err))
}

func trimPtr(s string) string {
	if len(s) > 0 && s[0] == '*' {
		return s[1:]
	}
	return s
}

// SetNodeTerminationReason records the reason a node span ended
// without a normal completion (for example "workflow_cancelled",
// "continue_on_fail", "skipped"). Called by the engine right before
// span.End() in those branches.
func SetNodeTerminationReason(span oteltrace.Span, reason string) {
	if span == nil || !span.SpanContext().IsValid() || reason == "" {
		return
	}
	span.SetAttributes(AttrNodeTerminationReason.String(reason))
}

// AddSpanLink attaches a link to the previous SpanContext on the
// current span. Used by the engine when a workflow resumes after a
// Wait node: the new workflow.execute span links to the previous
// span, with attribute m9m.continuation.reason set to "after_wait".
//
// Safe on a nil / no-op span — does nothing.
func AddSpanLink(span oteltrace.Span, target oteltrace.SpanContext, attrs ...attribute.KeyValue) {
	if span == nil || !span.SpanContext().IsValid() || !target.IsValid() {
		return
	}
	span.AddLink(oteltrace.Link{
		SpanContext: target,
		Attributes:  attrs,
	})
}

// ---------------------------------------------------------------------------
// GenAI / agent tracing
// ---------------------------------------------------------------------------

// AgentSpanOptions carries everything the AI nodes need to emit a
// <agent name>.generate / <agent name>.stream span. The agent name is
// taken from the workflow / node parameter set; the GenAI semconv
// attributes follow the conventions documented at
// https://opentelemetry.io/docs/specs/semconv/gen-ai/.
type AgentSpanOptions struct {
	// AgentName populates both the span name and the gen_ai.agent.name
	// attribute.
	AgentName string

	// ModelID is the provider-prefixed model name (e.g. "openai/gpt-4o",
	// "anthropic/claude-3.5-sonnet").
	ModelID string

	// ConversationID populates gen_ai.conversation.id (= thread id).
	ConversationID string

	// Stream selects between <name>.generate and <name>.stream.
	Stream bool

	// Prompt is the rendered prompt sent to the model. Recorded as the
	// gen_ai.prompt attribute when AgentsRecordInputs is true.
	Prompt string

	// ToolCatalog is the list of tool names the agent was offered. Each
	// name is rendered into the gen_ai.prompt.tool_count attribute and
	// the comma-joined list into gen_ai.prompt.tool_names.
	ToolCatalog []string

	// InputsAllowed mirrors cfg.AgentsRecordInputs. When false the
	// prompt / tool arguments are dropped from the span.
	InputsAllowed bool
}

// StartAgentSpan opens an agent span with the standard gen_ai.*
// attributes. Returns the parent context for nested execute_tool spans.
func (m *Manager) StartAgentSpan(ctx context.Context, opts AgentSpanOptions, extra ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if m == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	m.mu.RLock()
	agentsEnabled := m.cfg.AgentsEnabled
	tracer := m.tracer
	m.mu.RUnlock()
	if !agentsEnabled {
		return ctx, oteltrace.SpanFromContext(ctx)
	}

	name := SpanNameAgentGenerate
	if opts.Stream {
		name = SpanNameAgentStream
	}
	name = fmt.Sprintf(name, opts.AgentName)

	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", "invoke_agent"),
		attribute.String("gen_ai.agent.name", opts.AgentName),
	}
	if opts.ModelID != "" {
		attrs = append(attrs, attribute.String("gen_ai.request.model", opts.ModelID))
	}
	if opts.ConversationID != "" {
		attrs = append(attrs, attribute.String("gen_ai.conversation.id", opts.ConversationID))
	}
	if opts.InputsAllowed {
		if opts.Prompt != "" {
			attrs = append(attrs, attribute.String("gen_ai.prompt", opts.Prompt))
		}
		if len(opts.ToolCatalog) > 0 {
			attrs = append(attrs,
				attribute.Int("gen_ai.prompt.tool_count", len(opts.ToolCatalog)),
				attribute.StringSlice("gen_ai.prompt.tool_names", opts.ToolCatalog),
			)
		}
	}
	startOpts := append([]oteltrace.SpanStartOption{
		oteltrace.WithSpanKind(agentSpanKind),
		oteltrace.WithAttributes(attrs...),
	}, extra...)

	ctx, span := tracer.Start(ctx, name, startOpts...)
	return ctx, span
}

// EndAgentSpan records an error on the agent span (when err != nil) and
// ends it. Safe on a nil receiver / no-op span.
func (m *Manager) EndAgentSpan(span oteltrace.Span, err error) {
	if m == nil || span == nil {
		return
	}
	if !span.SpanContext().IsValid() {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// EndToolSpan records an error on a tool-call span (when err != nil) and
// ends it.
func (m *Manager) EndToolSpan(span oteltrace.Span, err error) {
	if m == nil || span == nil {
		return
	}
	if !span.SpanContext().IsValid() {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// ToolSpanOptions describes a single tool invocation: tool name, call id,
// arguments, result. Args and result are dropped from the span when the
// corresponding AgentsRecordInputs / AgentsRecordOutputs flags are false.
type ToolSpanOptions struct {
	ToolName    string
	ToolCallID  string
	Arguments   string // JSON-encoded
	Result      string // JSON-encoded or plain text
	InputsOK    bool   // mirrors cfg.AgentsRecordInputs
	OutputsOK   bool   // mirrors cfg.AgentsRecordOutputs
}

// StartToolSpan opens a `execute_tool <tool name>` span under the parent
// agent span on ctx. Returns the parent context for nested calls.
func (m *Manager) StartToolSpan(ctx context.Context, opts ToolSpanOptions, extra ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if m == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	m.mu.RLock()
	agentsEnabled := m.cfg.AgentsEnabled
	tracer := m.tracer
	m.mu.RUnlock()
	if !agentsEnabled {
		return ctx, oteltrace.SpanFromContext(ctx)
	}

	name := fmt.Sprintf(SpanNameExecuteTool, opts.ToolName)
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", "execute_tool"),
		attribute.String("gen_ai.tool.name", opts.ToolName),
	}
	if opts.ToolCallID != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call.id", opts.ToolCallID))
	}
	if opts.InputsOK && opts.Arguments != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call.arguments", opts.Arguments))
	}
	if opts.OutputsOK && opts.Result != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call.result", opts.Result))
	}

	startOpts := append([]oteltrace.SpanStartOption{
		oteltrace.WithSpanKind(toolSpanKind),
		oteltrace.WithAttributes(attrs...),
	}, extra...)

	ctx, span := tracer.Start(ctx, name, startOpts...)
	return ctx, span
}

// RecordUsage attaches token-usage attributes to an agent span.
// Mirrors the GenAI semconv for usage:
//   - gen_ai.usage.input_tokens
//   - gen_ai.usage.output_tokens
func RecordUsage(span oteltrace.Span, inputTokens, outputTokens int) {
	if span == nil || !span.SpanContext().IsValid() {
		return
	}
	span.SetAttributes(
		attribute.Int("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int("gen_ai.usage.output_tokens", outputTokens),
	)
}

// IsNoopSpan reports whether span is the SDK no-op span (i.e. no
// tracing is active). Useful in hot paths where a per-span attribute
// assignment would otherwise waste cycles.
func IsNoopSpan(span oteltrace.Span) bool {
	return span == nil || !span.SpanContext().IsValid()
}
