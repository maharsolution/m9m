package engine

import (
	"github.com/neul-labs/m9m/internal/model"
)

// BuildExecutionNodeData copies the engine's per-node output snapshot
// into a `map[nodeName]DataItem[]` ready to be assigned to
// `model.WorkflowExecution.NodeData` and persisted by storage.
//
// This is shared across every code path that materialises an execution:
//
//   - internal/api.ExecuteWorkflow     (HTTP `POST /workflows/{id}/execute`)
//   - internal/api.RetryExecution      (HTTP `POST /executions/{id}/retry`)
//   - internal/api.RetryNode           (HTTP `POST /executions/{id}/retry-node`)
//   - internal/webhooks.Manager        (inbound HTTP webhook triggers)
//
// Centralising the filter means Debug=OFF workflows (production /
// default) consistently keep only the start node's input and the last
// data-flow node's output, regardless of which path triggered the run.
// Before this was shared, webhook-triggered executions silently skipped
// the snapshot — leaving the NDV's Input / Output tabs empty even for
// the first / last node.
//
// Returns an empty (non-nil) map when result is nil so callers can
// always assign the result unconditionally without nil-checking.
func BuildExecutionNodeData(workflow *model.Workflow, result *ExecutionResult) map[string][]model.DataItem {
	out := make(map[string][]model.DataItem)
	if result == nil {
		return out
	}

	if workflow != nil && workflow.Debug {
		// Debug mode: copy everything.
		for nodeName, items := range result.NodeOutputs {
			if len(items) == 0 {
				out[nodeName] = []model.DataItem{}
				continue
			}
			out[nodeName] = items
		}
		return out
	}

	// Production mode: keep only start-node + last-node.
	startName := findStartNodeName(workflow)
	endName := findLastNodeName(workflow)

	if startName != "" {
		if items, ok := result.NodeOutputs[startName]; ok {
			if len(items) == 0 {
				out[startName] = []model.DataItem{}
			} else {
				out[startName] = items
			}
		}
	}
	if endName != "" && endName != startName {
		if items, ok := result.NodeOutputs[endName]; ok && len(items) > 0 {
			out[endName] = items
		} else if len(result.Data) > 0 {
			// The "last node" topology heuristic doesn't always
			// match the actual data-flow leaf of the workflow.
			// Common cases:
			//
			//   - Branching workflows: an IF routes the item down
			//     one path, so the topological last node (which
			//     sits on the OTHER branch) ends up empty even
			//     though the workflow clearly produced output.
			//   - Decorated terminal nodes (Respond to Webhook
			//     with responseMode: responseNode, debug branches,
			//     etc): the engine's `finalResult` picks the last
			//     node in execution order with non-empty output,
			//     which may not be the workflow's last-declared
			//     leaf.
			//
			// n8n's NDV shows the workflow-level result on the
			// "last node" the user sees in the canvas regardless
			// of which branch actually carried the item. To match
			// that, when the topology-derived last node has no
			// per-node snapshot but the engine still produced a
			// workflow-level `result.Data`, surface that data
			// under the last-node key. The user clicks the
			// rightmost node in the canvas and gets the data
			// the workflow emitted.
			out[endName] = result.Data
		} else {
			// Both last-node snapshot AND result.Data are empty
			// (e.g. the workflow didn't reach its declared leaf
			// at all). Surface this so the UI can render
			// "no output" rather than silently dropping the
			// key.
			out[endName] = []model.DataItem{}
		}
	}
	if len(out) == 0 && len(result.Data) > 0 {
		// Topology fallback: if we couldn't identify a start or
		// end node at all (cycle-only workflow, etc.), record
		// the workflow-level final output under the conventional
		// "__end__" key so the NDV still has something to
		// render.
		out["__end__"] = result.Data
	}
	return out
}

// findStartNodeName returns the name of the first node that has no
// incoming connections. The convention matches the engine's
// `findStartingNodes` (a node is a "start" if nothing routes INTO it),
// so we agree with the engine on which node gets the trigger payload.
//
// Decorative nodes (Sticky Notes, Canvas Notes) are skipped: they
// have no executor, never carry data, and would otherwise hijack the
// "first node" slot — the NDV would then claim the Sticky Note's
// empty payload is the trigger's input.
//
// Returns "" when the graph has no obvious start (e.g. a loop-only
// workflow where every node has at least one incoming edge).
func findStartNodeName(workflow *model.Workflow) string {
	if workflow == nil || len(workflow.Nodes) == 0 {
		return ""
	}
	hasIncoming := make(map[string]bool, len(workflow.Nodes))
	for _, conns := range workflow.Connections {
		if conns.Main == nil {
			continue
		}
		for _, outputs := range conns.Main {
			for _, c := range outputs {
				hasIncoming[c.Node] = true
			}
		}
	}
	for _, n := range workflow.Nodes {
		if isDecorativeNodeType(n.Type) {
			continue
		}
		if !hasIncoming[n.Name] {
			return n.Name
		}
	}
	return ""
}

// findLastNodeName returns the name of the last node in execution
// order. The engine returns `result.Data` as the last node's
// output, so we approximate by walking connections in reverse (a
// node with no outgoing edges is a leaf). When multiple leaves
// exist (branching workflows), we return the first one found —
// the same heuristic the engine uses for `responseMode: lastNode`.
//
// Decorative nodes (Sticky Notes, Canvas Notes) are skipped so
// they don't hijack the slot with an empty payload.
//
// Returns "" when the workflow has no nodes.
func findLastNodeName(workflow *model.Workflow) string {
	if workflow == nil || len(workflow.Nodes) == 0 {
		return ""
	}
	hasOutgoing := make(map[string]bool, len(workflow.Nodes))
	for source, conns := range workflow.Connections {
		if conns.Main == nil {
			continue
		}
		for _, outputs := range conns.Main {
			if len(outputs) > 0 {
				hasOutgoing[source] = true
			}
		}
	}
	for i := len(workflow.Nodes) - 1; i >= 0; i-- {
		n := workflow.Nodes[i]
		if isDecorativeNodeType(n.Type) {
			continue
		}
		if !hasOutgoing[n.Name] {
			return n.Name
		}
	}
	return ""
}
