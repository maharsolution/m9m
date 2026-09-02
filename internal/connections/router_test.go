package connections

import (
	"testing"
	"github.com/neul-labs/m9m/internal/model"
)

func TestConnectionRouterCreation(t *testing.T) {
	router := NewConnectionRouter()
	if router == nil {
		t.Fatal("Expected router to be created, got nil")
	}
}

func TestRouteDataWithNilWorkflow(t *testing.T) {
	router := NewConnectionRouter()
	
	_, err := router.RouteData("source", nil, nil)
	if err == nil {
		t.Error("Expected error with nil workflow, got nil")
	}
}

func TestRouteDataWithNoConnections(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Connections: make(map[string]model.Connections),
	}
	
	data := []model.DataItem{
		{JSON: map[string]interface{}{"test": "data"}},
	}
	
	routedData, err := router.RouteData("source-node", workflow, data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(routedData) != 0 {
		t.Errorf("Expected empty routed data, got %d entries", len(routedData))
	}
}

func TestRouteDataWithValidConnections(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "source", Type: "type1"},
			{Name: "target", Type: "type2"},
		},
		Connections: map[string]model.Connections{
			"source": {
				Main: [][]model.Connection{
					{
						{
							Node:  "target",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
		},
	}
	
	data := []model.DataItem{
		{JSON: map[string]interface{}{"test": "data"}},
	}
	
	routedData, err := router.RouteData("source", workflow, data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(routedData) != 1 {
		t.Errorf("Expected 1 routed data entry, got %d", len(routedData))
	}
	
	targetData, exists := routedData["target"]
	if !exists {
		t.Error("Expected data routed to 'target' node")
	}
	
	if len(targetData) != len(data) {
		t.Errorf("Expected %d data items, got %d", len(data), len(targetData))
	}
}

func TestGetConnectionsForNonExistentNode(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Connections: make(map[string]model.Connections),
	}
	
	connections := router.GetConnections("non-existent", workflow)
	if connections != nil {
		t.Error("Expected nil connections for non-existent node, got connections")
	}
}

func TestGetConnectionsForNodeWithoutConnections(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Connections: map[string]model.Connections{
			"node1": {}, // Empty connections
		},
	}
	
	connections := router.GetConnections("node1", workflow)
	if connections == nil {
		t.Fatal("Expected empty connections, got nil")
	}
	
	// Should be empty but not nil
	if len(connections.Main) != 0 {
		t.Errorf("Expected empty main connections, got %d", len(connections.Main))
	}
}

func TestValidateConnectionsWithNilWorkflow(t *testing.T) {
	router := NewConnectionRouter()
	
	err := router.ValidateConnections(nil)
	if err == nil {
		t.Error("Expected error for nil workflow, got nil")
	}
}

func TestValidateConnectionsWithValidWorkflow(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node1", Type: "type1"},
			{Name: "node2", Type: "type2"},
		},
		Connections: map[string]model.Connections{
			"node1": {
				Main: [][]model.Connection{
					{
						{
							Node:  "node2",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
		},
	}
	
	err := router.ValidateConnections(workflow)
	if err != nil {
		t.Errorf("Expected valid connections, got error: %v", err)
	}
}

func TestValidateConnectionsWithNonExistentSourceNode(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node2", Type: "type2"},
		},
		Connections: map[string]model.Connections{
			"node1": { // node1 doesn't exist
				Main: [][]model.Connection{
					{
						{
							Node:  "node2",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
		},
	}
	
	err := router.ValidateConnections(workflow)
	if err == nil {
		t.Error("Expected error for non-existent source node, got nil")
	}
}

func TestValidateConnectionsWithNonExistentTargetNode(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node1", Type: "type1"},
		},
		Connections: map[string]model.Connections{
			"node1": {
				Main: [][]model.Connection{
					{
						{
							Node:  "node2",
							Type:  "main",
							Index: 0, // node2 doesn't exist
						},
					},
				},
			},
		},
	}
	
	err := router.ValidateConnections(workflow)
	if err == nil {
		t.Error("Expected error for non-existent target node, got nil")
	}
}

func TestGetExecutionOrderWithNilWorkflow(t *testing.T) {
	router := NewConnectionRouter()
	
	_, err := router.GetExecutionOrder(nil)
	if err == nil {
		t.Error("Expected error for nil workflow, got nil")
	}
}

func TestGetExecutionOrderWithNoNodes(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{}, // No nodes
	}
	
	order, err := router.GetExecutionOrder(workflow)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(order) != 0 {
		t.Errorf("Expected empty execution order, got %d nodes", len(order))
	}
}

func TestGetExecutionOrderWithSingleNode(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node1", Type: "type1"},
		},
	}
	
	order, err := router.GetExecutionOrder(workflow)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(order) != 1 {
		t.Fatalf("Expected 1 node in execution order, got %d", len(order))
	}
	
	if order[0] != "node1" {
		t.Errorf("Expected 'node1', got '%s'", order[0])
	}
}

func TestGetExecutionOrderWithLinearDependencies(t *testing.T) {
	router := NewConnectionRouter()
	
	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node1", Type: "type1"},
			{Name: "node2", Type: "type2"},
			{Name: "node3", Type: "type3"},
		},
		Connections: map[string]model.Connections{
			"node1": {
				Main: [][]model.Connection{
					{
						{
							Node:  "node2",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
			"node2": {
				Main: [][]model.Connection{
					{
						{
							Node:  "node3",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
		},
	}
	
	order, err := router.GetExecutionOrder(workflow)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(order) != 3 {
		t.Fatalf("Expected 3 nodes in execution order, got %d", len(order))
	}
	
	// Check that dependencies are respected
	// node1 -> node2 -> node3
	node1Index := -1
	node2Index := -1
	node3Index := -1
	
	for i, node := range order {
		switch node {
		case "node1":
			node1Index = i
		case "node2":
			node2Index = i
		case "node3":
			node3Index = i
		}
	}
	
	if node1Index == -1 || node2Index == -1 || node3Index == -1 {
		t.Fatal("Not all nodes found in execution order")
	}
	
	// node1 should come before node2
	if node1Index >= node2Index {
		t.Error("Expected node1 to come before node2")
	}
	
	// node2 should come before node3
	if node2Index >= node3Index {
		t.Error("Expected node2 to come before node3")
	}
}

func TestHasCyclesWithNilWorkflow(t *testing.T) {
	router := NewConnectionRouter()
	
	_, err := router.HasCycles(nil)
	if err == nil {
		t.Error("Expected error for nil workflow, got nil")
	}
}

func TestHasCyclesWithAcyclicWorkflow(t *testing.T) {
	router := NewConnectionRouter()

	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "node1", Type: "type1"},
			{Name: "node2", Type: "type2"},
		},
		Connections: map[string]model.Connections{
			"node1": {
				Main: [][]model.Connection{
					{
						{
							Node:  "node2",
							Type:  "main",
							Index: 0,
						},
					},
				},
			},
		},
	}

	hasCycles, err := router.HasCycles(workflow)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if hasCycles {
		t.Error("Expected acyclic workflow, but cycles detected")
	}
}

// TestRouteDataPartitionByIfResult verifies the fix for PARITY_REPORT
// §6 Gap #1: when the source node tags items with `_ifResult` (the IF
// node), the router must split items across connections.Main[0]
// (true branch) and connections.Main[1] (false branch) and strip the
// internal tag from the data that reaches downstream nodes.
func TestRouteDataPartitionByIfResult(t *testing.T) {
	router := NewConnectionRouter()

	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "IF", Type: "n8n-nodes-base.if"},
			{Name: "TrueBranch", Type: "n8n-nodes-base.set"},
			{Name: "FalseBranch", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"IF": {
				Main: [][]model.Connection{
					{{Node: "TrueBranch", Type: "main", Index: 0}},
					{{Node: "FalseBranch", Type: "main", Index: 0}},
				},
			},
		},
	}

	data := []model.DataItem{
		{JSON: map[string]interface{}{"name": "Alice", "_ifResult": true}},
		{JSON: map[string]interface{}{"name": "Bob", "_ifResult": false}},
		{JSON: map[string]interface{}{"name": "Charlie", "_ifResult": true}},
	}

	routed, err := router.RouteData("IF", workflow, data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	trueBranch, ok := routed["TrueBranch"]
	if !ok {
		t.Fatal("Expected TrueBranch in routed data")
	}
	if len(trueBranch) != 2 {
		t.Errorf("Expected 2 items on TrueBranch (Alice, Charlie), got %d", len(trueBranch))
	}
	for _, item := range trueBranch {
		if _, leaked := item.JSON["_ifResult"]; leaked {
			t.Errorf("TrueBranch items must NOT carry _ifResult metadata, got %v", item.JSON)
		}
	}

	falseBranch, ok := routed["FalseBranch"]
	if !ok {
		t.Fatal("Expected FalseBranch in routed data")
	}
	if len(falseBranch) != 1 {
		t.Errorf("Expected 1 item on FalseBranch (Bob), got %d", len(falseBranch))
	}
	if falseBranch[0].JSON["name"] != "Bob" {
		t.Errorf("Expected Bob on FalseBranch, got %v", falseBranch[0].JSON)
	}
	for _, item := range falseBranch {
		if _, leaked := item.JSON["_ifResult"]; leaked {
			t.Errorf("FalseBranch items must NOT carry _ifResult metadata, got %v", item.JSON)
		}
	}
}

// TestRouteDataNoRoutingMetadataFallsBackToBroadcast covers the
// backwards-compat case: a source node that does NOT tag items with
// `_ifResult` must still have all of its data routed to every
// connected target, preserving the legacy single-branch behaviour
// for the (Set, Webhook, Function, …) majority of nodes.
func TestRouteDataNoRoutingMetadataFallsBackToBroadcast(t *testing.T) {
	router := NewConnectionRouter()

	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Set", Type: "n8n-nodes-base.set"},
			{Name: "Down", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Set": {
				Main: [][]model.Connection{
					{{Node: "Down", Type: "main", Index: 0}},
				},
			},
		},
	}

	data := []model.DataItem{
		{JSON: map[string]interface{}{"x": 1}},
		{JSON: map[string]interface{}{"x": 2}},
	}

	routed, err := router.RouteData("Set", workflow, data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	down, ok := routed["Down"]
	if !ok || len(down) != 2 {
		t.Fatalf("Expected 2 items routed to Down, got %v", routed)
	}
}

// TestRouteDataMixedIfResultFallsBackToBroadcast guards against
// accidental partial tagging: if even a single item is missing the
// `_ifResult` tag the router must NOT silently drop branches — it
// falls back to broadcast-to-all-targets to keep the workflow alive.
func TestRouteDataMixedIfResultFallsBackToBroadcast(t *testing.T) {
	router := NewConnectionRouter()

	workflow := &model.Workflow{
		Nodes: []model.Node{
			{Name: "IF", Type: "n8n-nodes-base.if"},
			{Name: "TrueBranch", Type: "n8n-nodes-base.set"},
			{Name: "FalseBranch", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"IF": {
				Main: [][]model.Connection{
					{{Node: "TrueBranch", Type: "main", Index: 0}},
					{{Node: "FalseBranch", Type: "main", Index: 0}},
				},
			},
		},
	}

	data := []model.DataItem{
		{JSON: map[string]interface{}{"a": "_ifResult"}},
		{JSON: map[string]interface{}{"b": true}}, // missing _ifResult
	}

	routed, err := router.RouteData("IF", workflow, data)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(routed["TrueBranch"]) != 2 || len(routed["FalseBranch"]) != 2 {
		t.Errorf("Mixed-tag input must fall back to broadcast, got True=%v False=%v",
			len(routed["TrueBranch"]), len(routed["FalseBranch"]))
	}
}