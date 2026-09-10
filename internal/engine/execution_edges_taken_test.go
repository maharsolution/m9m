package engine

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// TestRecordEdgesTaken_NonRoutingAllEdgesTaken verifies the trivial
// case: a node that emits no routing metadata forwards every item to
// every downstream branch (the historic engine behaviour). Every
// connection leaving the source must be marked taken.
func TestRecordEdgesTaken_NonRoutingAllEdgesTaken(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Linear",
		Nodes: []model.Node{
			{ID: "n1", Name: "Start", Type: "n8n-nodes-base.manualTrigger"},
			{ID: "n2", Name: "A", Type: "n8n-nodes-base.set"},
			{ID: "n3", Name: "B", Type: "n8n-nodes-base.set"},
			{ID: "n4", Name: "C", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{
				{{Node: "A", Type: "main", Index: 0}, {Node: "B", Type: "main", Index: 0}},
			}},
			// Fan-in: A and B both connect to C.
			"A": {Main: [][]model.Connection{{{Node: "C", Type: "main", Index: 0}}}},
			"B": {Main: [][]model.Connection{{{Node: "C", Type: "main", Index: 0}}}},
		},
	}

	taken := map[string]bool{}
	outputData := []model.DataItem{{JSON: map[string]interface{}{"x": 1}}}
	recordEdgesTaken(taken, &workflow.Nodes[0], outputData, workflow)

	// Every connection leaving "Start" should be marked taken.
	if !taken["n1:0:n2:0"] {
		t.Errorf("expected edge n1:0:n2:0 to be taken; got %v", taken)
	}
	if !taken["n1:0:n3:0"] {
		t.Errorf("expected edge n1:0:n3:0 to be taken; got %v", taken)
	}
}

// TestRecordEdgesTaken_IFRoute marks only the branch that received
// items. With `_ifResult: true` tagged on every item, only main[0]
// edges should be marked taken; main[1] edges must NOT be marked.
// This is the behaviour the UI relies on to grey out the unselected
// branch.
func TestRecordEdgesTaken_IFRoute(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Branch",
		Nodes: []model.Node{
			{ID: "src", Name: "If", Type: "n8n-nodes-base.if"},
			{ID: "trueT", Name: "TrueBranch", Type: "n8n-nodes-base.set"},
			{ID: "falseT", Name: "FalseBranch", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"If": {Main: [][]model.Connection{
				{{Node: "TrueBranch", Type: "main", Index: 0}},
				{{Node: "FalseBranch", Type: "main", Index: 0}},
			}},
		},
	}

	taken := map[string]bool{}
	outputData := []model.DataItem{{JSON: map[string]interface{}{"_ifResult": true, "value": 1}}}
	recordEdgesTaken(taken, &workflow.Nodes[0], outputData, workflow)

	if !taken["src:0:trueT:0"] {
		t.Errorf("expected true-branch edge to be taken; got %v", taken)
	}
	if taken["src:1:falseT:0"] {
		t.Errorf("expected false-branch edge NOT to be taken; got %v", taken)
	}
}

// TestRecordEdgesTaken_SwitchRouteRule2 covers the Switch variant: a
// single branch (rule index 2) gets the items; branches 0 and 1 do
// not.
func TestRecordEdgesTaken_SwitchRouteRule2(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Switch",
		Nodes: []model.Node{
			{ID: "src", Name: "Switch", Type: "n8n-nodes-base.switch"},
			{ID: "r0", Name: "Rule0", Type: "n8n-nodes-base.set"},
			{ID: "r1", Name: "Rule1", Type: "n8n-nodes-base.set"},
			{ID: "r2", Name: "Rule2", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Switch": {Main: [][]model.Connection{
				{{Node: "Rule0", Type: "main", Index: 0}},
				{{Node: "Rule1", Type: "main", Index: 0}},
				{{Node: "Rule2", Type: "main", Index: 0}},
			}},
		},
	}

	taken := map[string]bool{}
	outputData := []model.DataItem{
		{JSON: map[string]interface{}{"_switchRuleIndex": 2, "v": 1}},
	}
	recordEdgesTaken(taken, &workflow.Nodes[0], outputData, workflow)

	if taken["src:0:r0:0"] {
		t.Errorf("rule 0 edge should not be taken")
	}
	if taken["src:1:r1:0"] {
		t.Errorf("rule 1 edge should not be taken")
	}
	if !taken["src:2:r2:0"] {
		t.Errorf("rule 2 edge should be taken; got %v", taken)
	}
}

// TestRecordEdgesTaken_NoConnections verifies the no-op path: a node
// with no downstream connections must not panic and must not record
// any edges. This guards against workflows that have a single end
// node (e.g. Webhook -> Respond to Webhook -> end with Respond being
// a terminal node itself).
func TestRecordEdgesTaken_NoConnections(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Terminal",
		Nodes: []model.Node{
			{ID: "end", Name: "End", Type: "n8n-nodes-base.noOp"},
		},
		Connections: map[string]model.Connections{},
	}

	taken := map[string]bool{}
	outputData := []model.DataItem{{JSON: map[string]interface{}{"x": 1}}}
	recordEdgesTaken(taken, &workflow.Nodes[0], outputData, workflow)

	if len(taken) != 0 {
		t.Errorf("expected empty taken set; got %v", taken)
	}
}

// TestExecuteWorkflowPopulatesEdgesTaken exercises the full
// engine.ExecuteWorkflow path end-to-end and asserts the result has
// EdgesTaken populated for a simple linear workflow. Uses the
// mockNodeExecutor pattern from engine_test.go so the test doesn't
// require any production node executors to be registered.
func TestExecuteWorkflowPopulatesEdgesTaken(t *testing.T) {
	engine := NewWorkflowEngine().(*workflowEngineImpl)

	engine.RegisterNodeExecutor("n8n-nodes-base.start", &mockNodeExecutor{
		name: "start-mock",
		description: base.NodeDescription{
			Name:        "Start",
			Description: "Trigger-only manual start",
			Category:    "Trigger",
		},
	})
	engine.RegisterNodeExecutor("n8n-nodes-base.set", &mockNodeExecutor{
		name: "set-mock",
		description: base.NodeDescription{
			Name:        "Set",
			Description: "Mock Set node",
			Category:    "Transform",
		},
	})

	workflow := &model.Workflow{
		Name:   "Linear-EdgesTaken",
		Active: false,
		Nodes: []model.Node{
			{ID: "n1", Name: "Start", Type: "n8n-nodes-base.start"},
			{ID: "n2", Name: "Set", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{{{Node: "Set", Type: "main", Index: 0}}}},
		},
	}

	result, err := engine.ExecuteWorkflow(workflow, []model.DataItem{{JSON: map[string]interface{}{"hello": "world"}}})
	if err != nil {
		t.Fatalf("ExecuteWorkflow failed: %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil ExecutionResult")
	}
	if result.EdgesTaken == nil {
		t.Fatalf("expected EdgesTaken to be populated; got nil")
	}
	if !result.EdgesTaken["n1:0:n2:0"] {
		t.Errorf("expected edge n1:0:n2:0 to be marked taken; got %v", result.EdgesTaken)
	}
}
