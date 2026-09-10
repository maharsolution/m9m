package trigger

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// ExecuteWorkflowTriggerNode is the entry-point node for sub-workflows
// in n8n's "Execute Workflow" flow.
//
// n8n wires this into any workflow that's called from another workflow
// via the `Execute Workflow` core node. When the parent node calls
// `sub-workflow.execute`, n8n invokes this trigger and passes the
// `workflowInputs` mapping as the input item. The sub-workflow then
// runs as if it had been triggered externally, and any later Respond to
// Webhook node in the chain sees the result.
//
// In m9m the equivalent is "the engine already gave me the input data
// on Execute()", so this node is a pass-through: it returns whatever
// inputData the parent called it with. The "real" input — including
// the `workflowInputs` mapping from the parent's `Execute Workflow`
// node — is plumbed in by the engine when the Execute Workflow node
// calls ExecuteWorkflowWithContext.
//
// We still register the node so the connection-router / executor-lookup
// can find it; otherwise the engine returns "failed to get executor
// for node ..." and the sub-workflow dies at trigger-eval time.
type ExecuteWorkflowTriggerNode struct {
	*base.BaseNode
}

// NewExecuteWorkflowTriggerNode creates the trigger node.
func NewExecuteWorkflowTriggerNode() *ExecuteWorkflowTriggerNode {
	return &ExecuteWorkflowTriggerNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Execute Workflow Trigger",
			Description: "Trigger node for sub-workflows called via the Execute Workflow node",
			Category:    "Trigger",
			Inputs:      []string{},
			Outputs:     []string{"main"},
		}),
	}
}

// Execute is a pass-through — the parent has already set up the input
// data correctly, so we just hand it downstream.
func (n *ExecuteWorkflowTriggerNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		// Sub-workflows that don't take inputs (or whose parent
		// forgot to wire any) still need at least one item so the
		// downstream Set / Respond-to-Webhook nodes have something
		// to read from `$json`. n8n does the same: even with no
		// input mapping, the trigger emits an empty item.
		return []model.DataItem{{JSON: map[string]interface{}{}}}, nil
	}
	return inputData, nil
}

// ValidateParameters accepts anything n8n may have placed in the
// nodeParameters (workflowInputs mapping is the typical case).
func (n *ExecuteWorkflowTriggerNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	// `workflowInputs.mappingMode` ("defineBelow" | "auto") is the
	// only field n8n places here. Anything else is unexpected but
	// not an error — n8n may add new keys in future versions.
	if _, ok := params["workflowInputs"]; ok {
		if v, ok := params["workflowInputs"].(map[string]interface{}); ok {
			if mode, ok := v["mappingMode"].(string); ok && mode != "" && mode != "defineBelow" && mode != "auto" {
				return n.CreateError(fmt.Sprintf("unsupported workflowInputs.mappingMode: %s", mode), nil)
			}
		}
	}
	return nil
}
