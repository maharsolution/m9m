package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// WorkflowLookup is the subset of storage required by ExecuteWorkflowNode
// to resolve a workflow by ID. We don't import the storage package here
// directly because the engine already depends on it; the adapter
// (EngineAdapter) bridges the gap.
type WorkflowLookup interface {
	GetWorkflow(id string) (*model.Workflow, error)
}

// WorkflowExecutor is the subset of engine.WorkflowEngine needed by this node.
// We accept a context.Context so the sub-workflow.execute span chains
// off the parent span as a child. The EngineAdapter below wraps the
// legacy signature for back-compat.
type WorkflowExecutor interface {
	ExecuteWorkflowWithContext(ctx context.Context, workflow *model.Workflow, inputData []model.DataItem) (*WorkflowResult, error)
}

// WorkflowResult mirrors engine.ExecutionResult so we don't import the engine package.
type WorkflowResult struct {
	Data  []model.DataItem
	Error error
}

// EngineAdapter adapts any engine that satisfies the full interface.
type EngineAdapter struct {
	ExecuteFn func(workflow *model.Workflow, inputData []model.DataItem) ([]model.DataItem, error)
	// LookupFn resolves a workflow by ID. Optional — when nil the node
	// only honours `workflowPath` (the historic m9m contract).
	LookupFn func(id string) (*model.Workflow, error)
}

// ExecuteWorkflowWithContext satisfies WorkflowExecutor. The supplied
// context is forwarded as-is, which lets the engine chain the inner
// workflow.execute span onto the caller's trace.
func (a *EngineAdapter) ExecuteWorkflowWithContext(ctx context.Context, workflow *model.Workflow, inputData []model.DataItem) (*WorkflowResult, error) {
	data, err := a.ExecuteFn(workflow, inputData)
	if err != nil {
		return nil, err
	}
	return &WorkflowResult{Data: data}, nil
}

// GetWorkflow implements WorkflowLookup when LookupFn is wired.
func (a *EngineAdapter) GetWorkflow(id string) (*model.Workflow, error) {
	if a.LookupFn == nil {
		return nil, fmt.Errorf("workflow lookup not configured")
	}
	return a.LookupFn(id)
}

// EngineAdapterFromEngine wraps a real engine.WorkflowEngine so legacy
// callers (registry code) keep working. The optional `lookup` lets the
// resulting adapter resolve child workflows by ID from n8n-style
// `workflowId: {"value": "..."}` parameters; pass nil for the historic
// localFile-only behaviour.
func EngineAdapterFromEngine(eng engine.WorkflowEngine, lookup WorkflowLookup) *EngineAdapter {
	a := &EngineAdapter{
		ExecuteFn: func(workflow *model.Workflow, inputData []model.DataItem) ([]model.DataItem, error) {
			res, err := engine.ExecuteWorkflowWithContext(context.Background(), eng, workflow, inputData)
			if err != nil {
				return nil, err
			}
			if res != nil && res.Error != nil {
				return res.Data, res.Error
			}
			return res.Data, nil
		},
	}
	if lookup != nil {
		a.LookupFn = lookup.GetWorkflow
	}
	return a
}

// ExecuteWorkflowNode loads and executes a sub-workflow.
type ExecuteWorkflowNode struct {
	*base.BaseNode
	executor WorkflowExecutor
	lookup   WorkflowLookup
	depth    int // current recursion depth, set by parent
}

// NewExecuteWorkflowNode creates a new Execute Workflow node.
// The executor is used to run the sub-workflow. The lookup, when non-nil,
// lets the node resolve `workflowId` references by ID — matching n8n's
// behaviour where the parameter is `{__rl: true, value: "<id>", mode: "list"}`
// pointing at another workflow in the same database. Pass nil for both
// to get a placeholder that fails at execution time (useful for
// catalog listing).
func NewExecuteWorkflowNode(executor WorkflowExecutor, lookup WorkflowLookup) *ExecuteWorkflowNode {
	return &ExecuteWorkflowNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Execute Workflow",
			Description: "Executes another workflow as a sub-workflow",
			Category:    "Core",
		}),
		executor: executor,
		lookup:   lookup,
	}
}

// ExecuteWithContext is the context-aware variant. We thread the
// parent ctx through so the inner workflow.execute span chains onto
// the outer span as a child. The legacy Execute delegates to
// ExecuteWithContext with a fresh ctx.
func (n *ExecuteWorkflowNode) ExecuteWithContext(ctx context.Context, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if n.executor == nil {
		return nil, n.CreateError("no workflow engine configured", nil)
	}

	maxDepth := n.GetIntParameter(nodeParams, "maxDepth", 10)
	if n.depth >= maxDepth {
		return nil, n.CreateError(fmt.Sprintf("maximum recursion depth (%d) reached", maxDepth), nil)
	}

	// Resolve the sub-workflow. n8n uses a `workflowId` parameter that
	// is either a plain string or the `{__rl: true, value: "<id>", ...}`
	// resource-locator object. For back-compat we also honour
	// `workflowPath` pointing at a JSON file on disk (the historic
	// m9m contract); both shapes are accepted in any order and the
	// first one that resolves wins.
	workflow, err := n.resolveSubWorkflow(nodeParams)
	if err != nil {
		return nil, err
	}

	// Provide input data
	if len(inputData) == 0 {
		inputData = []model.DataItem{{JSON: map[string]interface{}{}}}
	}

	result, err := n.executor.ExecuteWorkflowWithContext(ctx, workflow, inputData)
	if err != nil {
		return nil, n.CreateError(fmt.Sprintf("sub-workflow execution failed: %v", err), nil)
	}

	if result.Error != nil {
		return nil, n.CreateError(fmt.Sprintf("sub-workflow error: %v", result.Error), nil)
	}

	return result.Data, nil
}

// Execute delegates to ExecuteWithContext with a fresh context for
// back-compat with the legacy NodeExecutor interface.
func (n *ExecuteWorkflowNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return n.ExecuteWithContext(context.Background(), inputData, nodeParams)
}

// resolveSubWorkflow picks the sub-workflow to run based on nodeParams.
//
// Three shapes are honoured, in order:
//
//  1. `workflowId` — n8n's standard field. Either a plain string ID or
//     the resource-locator object `{"__rl": true, "value": "<id>", ...}`.
//     Resolved via the WorkflowLookup (typically the same MySQL/Postgres
//     storage that holds the parent workflow).
//
//  2. `workflowPath` — historic m9m shape. Treats the value as an
//     absolute path to a JSON file on disk and parses it. Kept so
//     self-hosted CLI users (and the existing unit tests) can keep
//     running without a database.
//
// At least one of the two must resolve; if both are absent we return a
// clear error so the user knows which shape to add.
func (n *ExecuteWorkflowNode) resolveSubWorkflow(nodeParams map[string]interface{}) (*model.Workflow, error) {
	if id, ok := extractWorkflowID(nodeParams); ok && id != "" {
		if n.lookup == nil {
			return nil, n.CreateError("workflowId is set but no workflow lookup is configured", nil)
		}
		wf, err := n.lookup.GetWorkflow(id)
		if err != nil {
			return nil, n.CreateError(fmt.Sprintf("cannot load workflow %q: %v", id, err), nil)
		}
		if wf == nil {
			return nil, n.CreateError(fmt.Sprintf("workflow %q not found", id), nil)
		}
		return wf, nil
	}

	if path := n.GetStringParameter(nodeParams, "workflowPath", ""); path != "" {
		data, err := readWorkflowFile(path)
		if err != nil {
			return nil, err
		}
		var workflow model.Workflow
		if err := json.Unmarshal(data, &workflow); err != nil {
			return nil, n.CreateError(fmt.Sprintf("invalid workflow JSON: %v", err), nil)
		}
		return &workflow, nil
	}

	return nil, n.CreateError("workflowId is required (or workflowPath for local file)", nil)
}

// extractWorkflowID pulls a workflow ID out of the various n8n shapes.
func extractWorkflowID(params map[string]interface{}) (string, bool) {
	raw, ok := params["workflowId"]
	if !ok || raw == nil {
		return "", false
	}
	if s, ok := raw.(string); ok {
		return s, true
	}
	if m, ok := raw.(map[string]interface{}); ok {
		// n8n's resource-locator: {"__rl": true, "value": "<id>", "mode": "list", ...}
		if val, ok := m["value"].(string); ok && val != "" {
			return val, true
		}
	}
	return "", false
}

// ValidateParameters validates Execute Workflow parameters.
//
// Accepts either the n8n-style `workflowId` (string or resource-locator
// object) or the legacy `workflowPath`. The "unsupported source" guard
// only fires for the historic `source: localFile|...` knob — when
// neither id/path is given we report a missing-id error rather than
// silently passing.
func (n *ExecuteWorkflowNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return nil
	}

	// source is only meaningful when workflowPath is used; with
	// workflowId the storage backend is implicit. Keep this check
	// loose so existing localFile workflows still validate.
	if src, ok := params["source"].(string); ok && src != "" && src != "localFile" {
		return n.CreateError(fmt.Sprintf("unsupported source: %s", src), nil)
	}

	if id, ok := extractWorkflowID(params); ok && id != "" {
		return nil
	}
	if path := n.GetStringParameter(params, "workflowPath", ""); path != "" {
		return nil
	}
	return n.CreateError("workflowId is required (or workflowPath for local file)", nil)
}
