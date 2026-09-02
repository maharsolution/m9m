/*
Package connections provides connection routing and data flow management for m9m workflows.
*/
package connections

import (
	"fmt"
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
// node, which tags every item with `_ifResult`), the router partitions items
// across branches based on that metadata:
//
//	_main[0]_ → items whose `_ifResult` is true
//	_main[1]_ → items whose `_ifResult` is false
//	_main[k]_ (k>1) → no items (mirrors n8n: only main[0] and main[1] exist for IF)
//
// For nodes that do not emit routing metadata, the router falls back to the
// historic behaviour: every item produced by the source node is forwarded to
// every target connected at that branch index. This preserves correctness for
// the common case (single-branch nodes) while enabling true IF routing.
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
	// node (and any future node that opts into routing) tags every item
	// with `_ifResult` — we use that as the signal to partition data
	// across main[0] (true) and main[1] (false).
	branchable, trueItems, falseItems := partitionByRoutingMetadata(data)

	// Create result map
	routedData := make(map[string][]model.DataItem)

	// For each branch (top-level slice of connections.Main)
	for branchIndex, typeConnections := range connections.Main {
		// For each connection in this branch
		for _, connection := range typeConnections {
			var branchData []model.DataItem
			switch {
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

	// Strip the internal `_ifResult` metadata before items reach downstream
	// nodes — n8n does not expose this field on user data.
	if branchable {
		for nodeName := range routedData {
			for i := range routedData[nodeName] {
				delete(routedData[nodeName][i].JSON, "_ifResult")
			}
		}
	}

	return routedData, nil
}

// partitionByRoutingMetadata inspects every item for the `_ifResult`
// routing tag. If the tag is present on all items, it returns true and
// splits items into the true/false slices. Otherwise it returns false
// and nil slices, signalling to RouteData that the legacy broadcast
// behaviour should be used.
func partitionByRoutingMetadata(data []model.DataItem) (branchable bool, trueItems, falseItems []model.DataItem) {
	if len(data) == 0 {
		return false, nil, nil
	}
	for _, item := range data {
		if _, ok := item.JSON["_ifResult"]; !ok {
			return false, nil, nil
		}
	}
	trueItems = make([]model.DataItem, 0)
	falseItems = make([]model.DataItem, 0)
	for _, item := range data {
		switch item.JSON["_ifResult"] {
		case true:
			trueItems = append(trueItems, item)
		case false:
			falseItems = append(falseItems, item)
		default:
			// _ifResult exists but is neither true nor false; treat
			// the whole batch as unbranched to be safe.
			return false, nil, nil
		}
	}
	return true, trueItems, falseItems
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
	
	// Build dependency graph
	dependencies := make(map[string][]string) // node -> list of dependencies
	allNodes := make(map[string]bool)
	
	// Initialize dependencies for all nodes
	for _, node := range workflow.Nodes {
		allNodes[node.Name] = true
		dependencies[node.Name] = []string{}
	}
	
	// Build dependency relationships from connections
	for sourceNode, connections := range workflow.Connections {
		for _, typeConnections := range connections.Main {
			for _, connection := range typeConnections {
				targetNode := connection.Node
				// Add sourceNode as a dependency of targetNode
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