/*
Package connections provides connection routing and data flow management for m9m workflows.
*/
package connections

import (
	"fmt"
	"log"

	"github.com/neul-labs/m9m/internal/model"
)

// ConnectionRouter handles routing data between nodes in a workflow
type ConnectionRouter interface {
	// RouteData routes data from a source node to connected target nodes
	RouteData(sourceNode string, workflow *model.Workflow, data []model.DataItem) (map[string][]model.DataItem, error)
	
	// GetConnections returns the connections for a specific node
	GetConnections(nodeName string, workflow *model.Workflow) *model.Connections
	
	// ValidateConnections validates all connections in a workflow
	ValidateConnections(workflow *model.Workflow) error
	
	// GetExecutionOrder determines the order in which nodes should be executed
	GetExecutionOrder(workflow *model.Workflow) ([]string, error)
	
	// HasCycles detects if there are cycles in the workflow
	HasCycles(workflow *model.Workflow) (bool, error)
}

// connectionRouterImpl is the concrete implementation of ConnectionRouter
type connectionRouterImpl struct{}

// NewConnectionRouter creates a new connection router
func NewConnectionRouter() ConnectionRouter {
	return &connectionRouterImpl{}
}

// RouteData routes data from a source node to connected target nodes.
//
// n8n's connection shape is `connections.Main = [[{node:T,...}], [{node:F,...}]]` —
// each top-level slice is a branch (main[0] = first output / "true", main[1] =
// "false", etc.). For nodes that emit per-item routing metadata (currently the IF
// node, which tags every item with `_ifResult`, and the Switch node, which tags
// every item with `_switchRuleIndex`), the router partitions items across
// branches based on that metadata:
//
//	IF:        main[0] ← _ifResult==true;  main[1] ← _ifResult==false
//	Switch:    main[k] ← _switchRuleIndex==k
//	Switch fallback (no match): main[last] ← unmatched items
//
// For nodes that do not emit routing metadata, the router falls back to the
// historic behaviour: every item produced by the source node is forwarded to
// every target connected at that branch index. This preserves correctness for
// the common case (single-branch nodes) while enabling true IF/Switch routing.
func (r *connectionRouterImpl) RouteData(sourceNode string, workflow *model.Workflow, data []model.DataItem) (map[string][]model.DataItem, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}

	// Get connections for the source node
	connections := r.GetConnections(sourceNode, workflow)
	if connections == nil {
		// No connections, return empty map
		return make(map[string][]model.DataItem), nil
	}

	// Detect whether the source emitted per-item routing metadata. The IF
	// node tags every item with `_ifResult`; the Switch node tags every
	// item with `_switchRuleIndex`. The partition helper returns one of
	// three shapes depending on the tag, plus a `branchable` flag.
	branchable, trueItems, falseItems, switchItems := partitionByRoutingMetadata(data)
	branchCount := len(connections.Main)

	// Create result map
	routedData := make(map[string][]model.DataItem)

	// TEMPORARY DEBUG: trace RouteData entry so we can confirm
	// whether the 5→1 collapse happens inside the router (when
	// partitionByRoutingMetadata misfires) or somewhere upstream of
	// the connection router. Companion to the engine-level
	// debugTrace* helpers. Remove once root cause is confirmed.
	debugTraceRouteDataEntry(sourceNode, data, routedData)

	// For each branch (top-level slice of connections.Main)
	for branchIndex, typeConnections := range connections.Main {
		// For each connection in this branch
		for _, connection := range typeConnections {
			var branchData []model.DataItem
			switch {
			case branchable && switchItems != nil:
				// Switch routing. Items tagged with `_switchRuleIndex`
				// for a rule whose index matches `branchIndex` go to
				// main[branchIndex]. Items whose rule index was outside
				// the workflow's branch count (which is rare — n8n
				// always exposes every rule as its own branch) fall
				// through to the last branch as a fallback, matching
				// the Set node's `fallbackToLast` behaviour.
				if items, ok := switchItems[branchIndex]; ok {
					branchData = items
				} else if branchIndex == branchCount-1 {
					// Last branch — collect items with no matching rule
					// by unioning all switchItems that were not assigned
					// to a branch above. Only meaningful when the
					// workflow has at least one rule.
					for idx, items := range switchItems {
						if idx >= branchCount {
							branchData = append(branchData, items...)
						}
					}
				}
			case branchable && branchIndex == 0:
				branchData = trueItems
			case branchable && branchIndex == 1:
				branchData = falseItems
			case branchable:
				// main[k] for k>1 is unused by IF; mirror n8n by
				// forwarding nothing rather than every item.
				branchData = nil
			default:
				// Non-routing node: forward all items to every target
				// on every branch (legacy behaviour).
				branchData = data
			}

			if routedData[connection.Node] == nil {
				routedData[connection.Node] = make([]model.DataItem, len(branchData))
				copy(routedData[connection.Node], branchData)
			} else {
				routedData[connection.Node] = append(routedData[connection.Node], branchData...)
			}
		}
	}

	// Strip the internal routing metadata before items reach downstream
	// nodes — n8n does not expose these fields on user data.
	if branchable {
		for nodeName := range routedData {
			for i := range routedData[nodeName] {
				delete(routedData[nodeName][i].JSON, "_ifResult")
				delete(routedData[nodeName][i].JSON, "_switchRuleIndex")
				delete(routedData[nodeName][i].JSON, "_loopDone")
			}
		}
	}

	return routedData, nil
}

// partitionByRoutingMetadata inspects every item for a routing tag and
// partitions items across branches accordingly. Three tags are supported:
//
//   - `_ifResult` (bool): routes to main[0] for true, main[1] for false.
//     Set by the IF node.
//   - `_switchRuleIndex` (int): routes to main[index] for the rule that
//     matched. Set by the Switch node. Items that did not match any
//     rule fall back to the last branch (main[len-1]) which matches
//     n8n's `fallbackToLast` semantics.
//   - `_loopDone` (bool): routes to main[0] for items with `_loopDone=true`
//     (the "done" branch of a splitInBatches loop) and main[1] for items
//     with `_loopDone=false` (the "process" branch carrying the next
//     batch). Set by SplitInBatchesNode.Execute.
//
// If any tag is present on every item, items are partitioned and the
// branchable flag is true. If multiple tags are present (unlikely but
// possible), `_ifResult` takes precedence — IF and Switch aren't chained
// in the same node.
func partitionByRoutingMetadata(data []model.DataItem) (branchable bool, trueItems, falseItems []model.DataItem, switchItems map[int][]model.DataItem) {
	if len(data) == 0 {
		return false, nil, nil, nil
	}
	ifAll := true
	switchAll := true
	loopAll := true
	for _, item := range data {
		if _, ok := item.JSON["_ifResult"]; !ok {
			ifAll = false
		}
		if _, ok := item.JSON["_switchRuleIndex"]; !ok {
			switchAll = false
		}
		if _, ok := item.JSON["_loopDone"]; !ok {
			loopAll = false
		}
	}

	if ifAll {
		trueItems = make([]model.DataItem, 0)
		falseItems = make([]model.DataItem, 0)
		for _, item := range data {
			switch item.JSON["_ifResult"] {
			case true:
				trueItems = append(trueItems, item)
			case false:
				falseItems = append(falseItems, item)
			default:
				return false, nil, nil, nil
			}
		}
		return true, trueItems, falseItems, nil
	}

	if switchAll {
		// _switchRuleIndex was added by SwitchNode.Execute. n8n's Switch
		// routes each matched rule to its own `main[k]` branch — items
		// whose rule didn't match either go to the last branch when
		// `fallbackToLast` is set, or are dropped entirely otherwise.
		// We do not know the rule count here without inspecting the
		// workflow, so the caller (RouteData) determines the effective
		// branch count from `connections.Main` and falls back to
		// broadcasting when an item's index is out of range.
		switchItems = make(map[int][]model.DataItem)
		for _, item := range data {
			idx, ok := item.JSON["_switchRuleIndex"].(int)
			if !ok {
				return false, nil, nil, nil
			}
			switchItems[idx] = append(switchItems[idx], item)
		}
		return true, nil, nil, switchItems
	}

	if loopAll {
		// _loopDone was added by SplitInBatchesNode.Execute. Items with
		// _loopDone=true carry the accumulated "done" payload and are
		// routed to main[0] (the user's "Done" branch); items with
		// _loopDone=false carry the next batch and are routed to
		// main[1] (the user's "Process Item" branch). This mirrors the
		// n8n splitInBatches convention where main[0] is the
		// completion branch and main[1] is the per-iteration branch.
		trueItems = make([]model.DataItem, 0)
		falseItems = make([]model.DataItem, 0)
		for _, item := range data {
			switch item.JSON["_loopDone"] {
			case true:
				trueItems = append(trueItems, item)
			case false:
				falseItems = append(falseItems, item)
			default:
				return false, nil, nil, nil
			}
		}
		return true, trueItems, falseItems, nil
	}

	return false, nil, nil, nil
}

// debugTraceRouteDataEntry logs the input → per-target data
// mapping produced by RouteData so we can see exactly which slice
// the router delivered to each downstream node. Companion to the
// engine-level debugTrace* helpers; remove once the webhook_loop
// flake is fixed.
func debugTraceRouteDataEntry(sourceNode string, data []model.DataItem, routed map[string][]model.DataItem) {
	// TEMPORARY DEBUG: log ALL sources, not just Code, so we can
	// see exactly which node produced the 5→1 collapse.
	if sourceNode != "Generate Mock Data" {
		return
	}
	log.Printf("DEBUG-ROUTE-ENTRY [%s] in=%d", sourceNode, len(data))
	keys := make([]string, 0, len(data))
	for _, item := range data {
		for k := range item.JSON {
			if len(keys) >= 5 {
				break
			}
			keys = append(keys, k)
		}
	}
	log.Printf("DEBUG-ROUTE-KEYS [%s] keys=%v", sourceNode, keys)
}

// GetConnections returns the connections for a specific node
func (r *connectionRouterImpl) GetConnections(nodeName string, workflow *model.Workflow) *model.Connections {
	if workflow == nil || workflow.Connections == nil {
		return nil
	}
	
	connections, exists := workflow.Connections[nodeName]
	if !exists {
		return nil
	}
	
	return &connections
}

// ValidateConnections validates all connections in a workflow
func (r *connectionRouterImpl) ValidateConnections(workflow *model.Workflow) error {
	if workflow == nil {
		return fmt.Errorf("workflow cannot be nil")
	}
	
	if workflow.Connections == nil {
		// No connections is valid
		return nil
	}
	
	// Create a set of all node names for quick lookup
	nodeNames := make(map[string]bool)
	for _, node := range workflow.Nodes {
		nodeNames[node.Name] = true
	}
	
	// Validate each connection
	for sourceNode, connections := range workflow.Connections {
		// Check if source node exists
		if !nodeNames[sourceNode] {
			return fmt.Errorf("connection references non-existent source node: %s", sourceNode)
		}
		
		// Validate main connections
		for _, typeConnections := range connections.Main {
			for _, connection := range typeConnections {
				// Check if target node exists
				if !nodeNames[connection.Node] {
					return fmt.Errorf("connection from %s references non-existent target node: %s", sourceNode, connection.Node)
				}
				
				// Validate connection type
				if connection.Type == "" {
					return fmt.Errorf("connection from %s to %s has empty type", sourceNode, connection.Node)
				}
				
				// Validate index
				if connection.Index < 0 {
					return fmt.Errorf("connection from %s to %s has negative index: %d", sourceNode, connection.Node, connection.Index)
				}
			}
		}
	}
	
	return nil
}

// GetExecutionOrder determines the order in which nodes should be executed
func (r *connectionRouterImpl) GetExecutionOrder(workflow *model.Workflow) ([]string, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}

	// Detect the splitInBatches loop pattern. When present, edges that
	// target a `splitInBatches` node are back-edges of an n8n loop
	// (e.g. `Process Item → Loop Over Items`) and should be ignored
	// for ordering purposes — the engine runs the loop node's body
	// to completion before producing output.
	tolerateCycles := WorkflowUsesSplitInBatches(workflow)

	// Build dependency graph
	dependencies := make(map[string][]string) // node -> list of dependencies
	allNodes := make(map[string]bool)

	// Initialize dependencies for all nodes
	for _, node := range workflow.Nodes {
		allNodes[node.Name] = true
		dependencies[node.Name] = []string{}
	}

	// Build dependency relationships from connections. Edges that
	// target a `splitInBatches` node are back-edges of the n8n loop
	// pattern and are skipped when computing execution order; the
	// engine instead executes the loop to completion before yielding
	// to the next node.
	for sourceNode, connections := range workflow.Connections {
		for _, typeConnections := range connections.Main {
			for _, connection := range typeConnections {
				targetNode := connection.Node
				if tolerateCycles && isSplitInBatchesNode(workflow, targetNode) {
					continue
				}
				dependencies[targetNode] = append(dependencies[targetNode], sourceNode)
			}
		}
	}

	// Perform topological sort
	executionOrder := []string{}
	visited := make(map[string]bool)
	temporaryMark := make(map[string]bool)

	// Visit each node
	for nodeName := range allNodes {
		hasCycle, err := r.visitNode(nodeName, dependencies, visited, temporaryMark, &executionOrder)
		if err != nil {
			return nil, fmt.Errorf("error during topological sort: %v", err)
		}
		if hasCycle {
			return nil, fmt.Errorf("workflow contains cycles - cannot determine execution order")
		}
	}

	return executionOrder, nil
}

// WorkflowUsesSplitInBatches reports whether the workflow contains
// n8n's splitInBatches (Loop) node. The caller uses this to decide
// whether the connection-level cycle (`Process Item → Loop Over
// Items`) should be tolerated when computing execution order and
// detecting cycles.
//
// Exported so the engine package can reuse the same predicate.
func WorkflowUsesSplitInBatches(workflow *model.Workflow) bool {
	if workflow == nil {
		return false
	}
	for _, node := range workflow.Nodes {
		if node.Type == "n8n-nodes-base.splitInBatches" {
			return true
		}
	}
	return false
}

// isSplitInBatchesNode returns true if the named node is the n8n
// splitInBatches (Loop) node in the given workflow.
func isSplitInBatchesNode(workflow *model.Workflow, nodeName string) bool {
	if workflow == nil {
		return false
	}
	for _, node := range workflow.Nodes {
		if node.Name == nodeName {
			return node.Type == "n8n-nodes-base.splitInBatches"
		}
	}
	return false
}

// visitNode is a helper function for topological sorting
func (r *connectionRouterImpl) visitNode(node string, dependencies map[string][]string, visited, temporaryMark map[string]bool, order *[]string) (bool, error) {
	// If temporarily marked, we have a cycle
	if temporaryMark[node] {
		return true, nil // Cycle detected
	}
	
	// If not visited yet
	if !visited[node] {
		// Mark temporarily
		temporaryMark[node] = true
		
		// Visit all dependencies
		for _, dependency := range dependencies[node] {
			hasCycle, err := r.visitNode(dependency, dependencies, visited, temporaryMark, order)
			if err != nil {
				return false, err
			}
			if hasCycle {
				return true, nil // Cycle detected in dependency
			}
		}
		
		// Mark as permanently visited
		visited[node] = true
		temporaryMark[node] = false
		
		// Add to order
		*order = append(*order, node)
	}
	
	return false, nil // No cycle
}

// HasCycles detects if there are cycles in the workflow
func (r *connectionRouterImpl) HasCycles(workflow *model.Workflow) (bool, error) {
	if workflow == nil {
		return false, fmt.Errorf("workflow cannot be nil")
	}
	
	// Try to get execution order - if it fails due to cycles, there are cycles
	_, err := r.GetExecutionOrder(workflow)
	if err != nil && fmt.Sprintf("%v", err) == "workflow contains cycles - cannot determine execution order" {
		return true, nil
	}
	
	return false, nil
}