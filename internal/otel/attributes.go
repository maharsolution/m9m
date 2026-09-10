package otel

import (
	"go.opentelemetry.io/otel/attribute"
)

// Attribute key prefixes used across the m9m telemetry surface. Kept in
// one place so renaming (and auditing) stays straightforward. The names
// mirror n8n's N8N_OTEL_* attribute contract 1:1 except for the M9M_
// prefix swap:
//
//	n8n.workflow.id            -> m9m.workflow.id
//	n8n.execution.mode         -> m9m.execution.mode
//	n8n.project.custom.<k>     -> m9m.project.custom.<k>
//	n8n.node.custom.<k>       -> m9m.node.custom.<k>
//
// See https://docs.n8n.io/deploy/host-n8n/keep-n8n-running/trace-executions-with-opentelemetry
// for the upstream reference.
const (
	AttrWorkflowID         = attribute.Key("m9m.workflow.id")
	AttrWorkflowName       = attribute.Key("m9m.workflow.name")
	AttrWorkflowVersionID  = attribute.Key("m9m.workflow.version_id")
	AttrWorkflowNodeCount  = attribute.Key("m9m.workflow.node_count")

	AttrProjectID          = attribute.Key("m9m.project.id")
	AttrProjectCustomPrefix = "m9m.project.custom."

	AttrExecutionID        = attribute.Key("m9m.execution.id")
	AttrExecutionMode      = attribute.Key("m9m.execution.mode")
	AttrExecutionStatus    = attribute.Key("m9m.execution.status")
	AttrExecutionIsRetry   = attribute.Key("m9m.execution.is_retry")
	AttrExecutionRetryOf   = attribute.Key("m9m.execution.retry_of")
	AttrExecutionErrorType = attribute.Key("m9m.execution.error_type")

	AttrNodeID                = attribute.Key("m9m.node.id")
	AttrNodeName              = attribute.Key("m9m.node.name")
	AttrNodeType              = attribute.Key("m9m.node.type")
	AttrNodeTypeVersion       = attribute.Key("m9m.node.type_version")
	AttrNodeItemsInput        = attribute.Key("m9m.node.items.input")
	AttrNodeItemsOutput       = attribute.Key("m9m.node.items.output")
	AttrNodeTerminationReason = attribute.Key("m9m.node.termination_reason")
	AttrNodeCustomPrefix      = "m9m.node.custom."

	AttrWorkflowCustomPrefix = "m9m.workflow.custom."
	AttrContinuationReason   = attribute.Key("m9m.continuation.reason")

	AttrInstanceID   = attribute.Key("m9m.instance.id")
	AttrInstanceRole = attribute.Key("m9m.instance.role")
)

// CustomAttributeLimit caps how many custom attributes a workflow or
// node can attach. n8n does not publish a hard limit; 64 keeps the
// worst-case span small without restricting common use.
const CustomAttributeLimit = 64

// WorkflowAttrs bundles the attributes a workflow.execute span expects.
// The engine fills this in from the model.Workflow + execution metadata
// and passes it to StartWorkflowSpan.
type WorkflowAttrs struct {
	WorkflowID        string
	WorkflowName      string
	WorkflowVersionID string
	NodeCount         int

	ProjectID string

	ExecutionID      string
	ExecutionMode    string // "manual", "trigger", "webhook", "retry", "cli", "schedule"
	ExecutionStatus  string // "running", "completed", "failed", "cancelled"
	IsRetry          bool
	RetryOf          string // original execution id when IsRetry
	ErrorType        string

	// Custom attributes. Keys are appended verbatim under the
	// m9m.workflow.custom.* and m9m.project.custom.* prefixes.
	WorkflowCustom map[string]string
	ProjectCustom  map[string]string
}

// BuildAttributes renders w into a slice of attribute.KeyValue ready to
// pass to span.SetAttributes.
func (w WorkflowAttrs) BuildAttributes() []attribute.KeyValue {
	out := []attribute.KeyValue{
		AttrWorkflowID.String(w.WorkflowID),
		AttrWorkflowName.String(w.WorkflowName),
		AttrWorkflowVersionID.String(w.WorkflowVersionID),
		AttrWorkflowNodeCount.Int(w.NodeCount),
		AttrExecutionID.String(w.ExecutionID),
		AttrExecutionMode.String(w.ExecutionMode),
		AttrExecutionStatus.String(w.ExecutionStatus),
		AttrExecutionIsRetry.Bool(w.IsRetry),
	}
	if w.ProjectID != "" {
		out = append(out, AttrProjectID.String(w.ProjectID))
	}
	if w.RetryOf != "" {
		out = append(out, AttrExecutionRetryOf.String(w.RetryOf))
	}
	if w.ErrorType != "" {
		out = append(out, AttrExecutionErrorType.String(w.ErrorType))
	}
	out = appendCustomAttrs(out, w.WorkflowCustom, AttrWorkflowCustomPrefix)
	out = appendCustomAttrs(out, w.ProjectCustom, AttrProjectCustomPrefix)
	return out
}

// NodeAttrs bundles the attributes a node.execute span expects. The
// engine fills this from model.Node + the runtime input/output item
// counts.
type NodeAttrs struct {
	NodeID          string
	NodeName        string
	NodeType        string
	NodeTypeVersion int

	ItemsInput  int
	ItemsOutput int

	// Custom node attributes. Keys are appended verbatim under the
	// m9m.node.custom.* prefix.
	Custom map[string]interface{}
}

// BuildAttributes renders n into a slice of attribute.KeyValue. Custom
// values that cannot be converted to a string / bool / int / float are
// dropped silently — same approach as the OTel attribute API when given
// an unsupported type.
func (n NodeAttrs) BuildAttributes() []attribute.KeyValue {
	out := []attribute.KeyValue{
		AttrNodeID.String(n.NodeID),
		AttrNodeName.String(n.NodeName),
		AttrNodeType.String(n.NodeType),
		AttrNodeTypeVersion.Int(n.NodeTypeVersion),
		AttrNodeItemsInput.Int(n.ItemsInput),
		AttrNodeItemsOutput.Int(n.ItemsOutput),
	}
	out = appendCustomAttrsAny(out, n.Custom, AttrNodeCustomPrefix)
	return out
}

// appendCustomAttrs adds each k=v pair under prefix+key. Empty keys are
// dropped; values are coerced to strings.
func appendCustomAttrs(out []attribute.KeyValue, m map[string]string, prefix string) []attribute.KeyValue {
	if len(m) == 0 {
		return out
	}
	count := 0
	for k, v := range m {
		if k == "" {
			continue
		}
		if count >= CustomAttributeLimit {
			break
		}
		out = append(out, attribute.String(prefix+k, v))
		count++
	}
	return out
}

// appendCustomAttrsAny accepts the broader interface{} shape used by
// node-level custom attributes. Booleans and numbers preserve their
// type; everything else falls back to AsString().
func appendCustomAttrsAny(out []attribute.KeyValue, m map[string]interface{}, prefix string) []attribute.KeyValue {
	if len(m) == 0 {
		return out
	}
	count := 0
	for k, v := range m {
		if k == "" {
			continue
		}
		if count >= CustomAttributeLimit {
			break
		}
		switch val := v.(type) {
		case bool:
			out = append(out, attribute.Bool(prefix+k, val))
		case int:
			out = append(out, attribute.Int(prefix+k, val))
		case int64:
			out = append(out, attribute.Int64(prefix+k, val))
		case float64:
			out = append(out, attribute.Float64(prefix+k, val))
		case string:
			out = append(out, attribute.String(prefix+k, val))
		default:
			out = append(out, attribute.String(prefix+k, attributeValueAsString(val)))
		}
		count++
	}
	return out
}

// attributeValueAsString renders an arbitrary value as a string. It is
// only used as a last-resort fallback when the value is not one of the
// recognised primitive types.
func attributeValueAsString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return ""
}
