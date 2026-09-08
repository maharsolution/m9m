/*
Package connections provides connection routing and data flow management for m9m workflows.
*/
package connections

import (
	"fmt"
	"sort"

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

// debugTraceRouteDataEntry is retained as a no-op stub so we don't
// have to remove the call site. The flake it was investigating
// (randomised map iteration in GetExecutionOrder) is now fixed by
// sortedNodeNamesByPosition, so this hook is no longer needed.
func debugTraceRouteDataEntry(sourceNode string, data []model.DataItem, routed map[string][]model.DataItem) {
	// intentionally empty — see comment above.
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

	// For workflows that contain a splitInBatches (Loop) node, the
	// done-branch (`main[0]`) must execute AFTER the process-branch
	// (`main[1]`) downstream completes — that's the n8n invariant
	// the "done" port encodes: "fire me once the body has finished
	// every iteration". m9m's topological sort treats `main[0]` and
	// `main[1]` as siblings because both are direct children of the
	// Loop node, which causes the done-branch (e.g. "Format Summary
	// Data") to run BEFORE the body has executed. The reader then
	// sees an empty `$("Filter Paid & In-Stock").all()` and emits a
	// partial / empty response.
	//
	// The fix: for each splitInBatches node, compute the set of
	// nodes reachable from `main[1]` (process-branch downstream),
	// then add a synthetic dependency from the *deepest* node in
	// that set to each `main[0]` (done-branch) target. This forces
	// the done-branch to be scheduled after the body has run, while
	// leaving the process-branch ordering itself untouched.
	if tolerateCycles {
		addSplitInBatchesDoneBranchDependencies(workflow, dependencies)
	}

	// Perform topological sort
	executionOrder := []string{}
	visited := make(map[string]bool)
	temporaryMark := make(map[string]bool)

	// Visit each node. We sort the candidates by their n8n canvas
	// position (X then Y) so the resulting topological order is
	// stable across runs — Go's map iteration order is randomised,
	// which used to make execution order (and therefore routing /
	// timing / telemetry output) flip-flop between equivalent but
	// visually different valid orderings. n8n orders nodes by their
	// `position` field (top-left to bottom-right), which is how
	// users visually lay out their workflow, so following the same
	// convention gives m9m's execution order a stable, predictable
	// shape that matches what an n8n user would expect when reading
	// the execution trace.
	nodeOrder := sortedNodeNamesByPosition(workflow, allNodes)
	for _, nodeName := range nodeOrder {
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

// sortedNodeNamesByPosition returns `allNodes` ordered by the
// workflow's canvas position (X ascending, then Y ascending). This
// gives GetExecutionOrder a stable, n8n-style traversal regardless
// of Go's randomised map iteration order — without it, two
// equivalent workflow JSONs could produce different execution
// orders run-to-run, which in turn would cause intermittent
// routing / shape divergence (e.g. the webhook_loop flake where
// the same workflow sometimes produced 5 items and sometimes 1).
func sortedNodeNamesByPosition(workflow *model.Workflow, allNodes map[string]bool) []string {
	// Build a position lookup keyed by node name. Nodes without a
	// position (synthetic / programmatically constructed workflows)
	// fall back to a zero position so they sort before everything
	// else.
	positions := make(map[string][2]int, len(workflow.Nodes))
	for _, n := range workflow.Nodes {
		var x, y int
		if len(n.Position) >= 2 {
			x = n.Position[0]
			y = n.Position[1]
		}
		positions[n.Name] = [2]int{x, y}
	}
	out := make([]string, 0, len(allNodes))
	for name := range allNodes {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool {
		pi := positions[out[i]]
		pj := positions[out[j]]
		if pi[0] != pj[0] {
			return pi[0] < pj[0]
		}
		return pi[1] < pj[1]
	})
	return out
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

// splitInBatchesDoneBranchDependencies adds synthetic ordering
// edges to `dependencies` so that the done-branch (`main[0]`) of
// every splitInBatches node is scheduled AFTER the process-branch
// (`main[1]`) body has run to completion. See the comment in
// GetExecutionOrder for the rationale.
func addSplitInBatchesDoneBranchDependencies(workflow *model.Workflow, dependencies map[string][]string) {
	if workflow == nil || dependencies == nil {
		return
	}
	for _, node := range workflow.Nodes {
		if node.Type != "n8n-nodes-base.splitInBatches" {
			continue
		}
		doneTargets := CollectDirectBranchTargets(workflow, node.Name, 0)
		processDescendants := CollectBranchDescendants(workflow, node.Name, 1)
		if len(doneTargets) == 0 || len(processDescendants) == 0 {
			continue
		}
		anchor := deepestProcessBranchNode(workflow, processDescendants)
		if anchor == "" {
			continue
		}
		for _, target := range doneTargets {
			if target == anchor || target == node.Name {
				continue
			}
			if !containsString(dependencies[target], anchor) {
				dependencies[target] = append(dependencies[target], anchor)
			}
		}
	}
}

// CollectDirectBranchTargets returns the immediate downstream
// nodes connected to `source` at the given `branchIndex` in
// connections.Main (0 = done, 1 = process for splitInBatches).
// Exported so the engine package can reuse it when driving
// splitInBatches iteration (the engine needs to know which
// downstream chain corresponds to the per-iteration "Process Item"
// branch vs. the once-per-loop "Done" branch).
func CollectDirectBranchTargets(workflow *model.Workflow, source string, branchIndex int) []string {
	conns, ok := workflow.Connections[source]
	if !ok {
		return nil
	}
	if branchIndex >= len(conns.Main) {
		return nil
	}
	var out []string
	for _, c := range conns.Main[branchIndex] {
		if c.Node == "" {
			continue
		}
		out = append(out, c.Node)
	}
	return out
}

// CollectBranchDescendants returns every node reachable from
// `source` via main[branchIndex] and below (transitive closure,
// excluding back-edges that target splitInBatches nodes).
// Exported so the engine can iterate the per-iteration "Process
// Item" body chain for each batch.
func CollectBranchDescendants(workflow *model.Workflow, source string, branchIndex int) map[string]bool {
	visited := make(map[string]bool)
	var walk func(string)
	walk = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		conns, ok := workflow.Connections[node]
		if !ok {
			return
		}
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				if isSplitInBatchesNode(workflow, c.Node) {
					continue
				}
				walk(c.Node)
			}
		}
	}
	for _, target := range CollectDirectBranchTargets(workflow, source, branchIndex) {
		walk(target)
	}
	delete(visited, source)
	return visited
}

// deepestProcessBranchNode picks the node in the process-branch
// descendant set that is the *last* to execute in topological
// order — the body-end aggregation node. The cleanest signal is
// the count of *outgoing* edges to other descendant nodes: a node
// with no further outgoing edges within the branch is the end of
// the body. Ties are broken by string ordering for determinism.
func deepestProcessBranchNode(workflow *model.Workflow, descendants map[string]bool) string {
	if len(descendants) == 0 {
		return ""
	}
	// hasOutgoing[n] = true if n has at least one Main edge to
	// another descendant (i.e. there's more body after n).
	hasOutgoing := make(map[string]bool, len(descendants))
	for n := range descendants {
		conns, ok := workflow.Connections[n]
		if !ok {
			continue
		}
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				if descendants[c.Node] {
					hasOutgoing[n] = true
				}
			}
		}
	}
	// The anchor is the node with the *smallest* outgoing-edge
	// count to other descendants (preferring nodes with none). This
	// matches n8n's "deepest" node in the chain — the leaf that
	// finishes the body. When multiple candidates tie (e.g. several
	// sibling leaves), pick the lexicographically smallest name for
	// determinism.
	var pick string
	for n := range descendants {
		if hasOutgoing[n] {
			continue
		}
		if pick == "" || n < pick {
			pick = n
		}
	}
	if pick != "" {
		return pick
	}
	// Fallback: every candidate has further descendants (shouldn't
	// happen for acyclic branches). Pick the lex-smallest name.
	for n := range descendants {
		if pick == "" || n < pick {
			pick = n
		}
	}
	return pick
}

func containsString(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
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

// GetLoopIterationOrder returns the per-iteration "Process Item"
// body chain (the nodes that run once per batch, between the
// splitInBatches node's `main[1]` branch and the back-edge that
// returns control to the splitInBatches node) in execution order.
// The back-edge targets are excluded because the engine itself
// drives the iteration — it does not need to traverse the
// back-edge as part of a single iteration's body walk.
//
// This helper is consumed by the engine's splitInBatches loop
// driver so that, for each batch, it knows the ordered list of
// body nodes to execute before resuming the outer loop.
//
// Returns an empty slice if `loopNodeName` is not a splitInBatches
// node, the workflow is nil, or the body chain is empty.
func GetLoopIterationOrder(workflow *model.Workflow, loopNodeName string) []string {
	if workflow == nil || loopNodeName == "" {
		return nil
	}
	if !isSplitInBatchesNode(workflow, loopNodeName) {
		return nil
	}
	descendants := CollectBranchDescendants(workflow, loopNodeName, 1)
	if len(descendants) == 0 {
		return nil
	}
	// Use the workflow's existing topological order so we honour
	// every dependency (including any IF/Switch branches inside
	// the body). We restrict to the descendant set, which already
	// excludes the back-edge targets.
	fullOrder, err := NewConnectionRouter().GetExecutionOrder(workflow)
	if err != nil {
		// On cycle error, fall back to a stable alphabetical sort
		// so the engine still has a deterministic iteration order
		// for the body chain.
		var fallback []string
		for n := range descendants {
			fallback = append(fallback, n)
		}
		sort.Strings(fallback)
		return fallback
	}
	var order []string
	for _, n := range fullOrder {
		if descendants[n] {
			order = append(order, n)
		}
	}
	return order
}

// GetLoopDoneOrder returns the nodes on the once-per-loop "Done"
// branch (the nodes downstream of splitInBatches's `main[0]`,
// excluding the back-edge) in execution order. Like
// GetLoopIterationOrder, this drives the engine's splitInBatches
// loop driver so that after the per-iteration body has run for
// every batch, the engine can execute the Done branch exactly once
// with the aggregated output.
func GetLoopDoneOrder(workflow *model.Workflow, loopNodeName string) []string {
	if workflow == nil || loopNodeName == "" {
		return nil
	}
	if !isSplitInBatchesNode(workflow, loopNodeName) {
		return nil
	}
	descendants := CollectBranchDescendants(workflow, loopNodeName, 0)
	if len(descendants) == 0 {
		return nil
	}
	fullOrder, err := NewConnectionRouter().GetExecutionOrder(workflow)
	if err != nil {
		var fallback []string
		for n := range descendants {
			fallback = append(fallback, n)
		}
		sort.Strings(fallback)
		return fallback
	}
	var order []string
	for _, n := range fullOrder {
		if descendants[n] {
			order = append(order, n)
		}
	}
	return order
}