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

// TestRecordEdgesTaken_SwitchMatchedRule0DoesNotMarkLastBranch is
// the regression test for the bug where a Switch node that routed
// a matched item to rule 0 was ALSO marking the LAST branch as
// taken. Cause: recordEdgesTaken used to unconditionally set
// `branchWithItems[len(conns.Main)-1] = true` for every Switch
// output, on the theory that "unmatched items fall through to
// main[last]". That assumption is wrong — the Switch node itself
// handles `fallbackToLast` by emitting an item with
// `_switchRuleIndex: len(rules)`, which the loop above already
// routes to branch `len-1`. Adding the unconditional `true` here
// caused the UI to green-paint the path to "Set Others" even when
// the input only matched "Set Tisha" — exactly the user's
// webhook `m9m` workflow bug ("others line should be gray").
func TestRecordEdgesTaken_SwitchMatchedRule0DoesNotMarkLastBranch(t *testing.T) {
	workflow := &model.Workflow{
		Name: "webhook m9m",
		Nodes: []model.Node{
			{ID: "switch-id", Name: "Switch", Type: "n8n-nodes-base.switch"},
			{ID: "set-tisha", Name: "Set Tisha", Type: "n8n-nodes-base.set"},
			{ID: "set-erwan", Name: "Set Erwan", Type: "n8n-nodes-base.set"},
			{ID: "set-others", Name: "Set Others", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Switch": {Main: [][]model.Connection{
				{{Node: "Set Tisha", Type: "main", Index: 0}},
				{{Node: "Set Erwan", Type: "main", Index: 0}},
				{{Node: "Set Others", Type: "main", Index: 0}},
			}},
		},
	}

	// The Switch emitted exactly 1 item: the matched Tisha rule.
	switchOutput := []model.DataItem{
		{JSON: map[string]interface{}{
			"_switchRuleIndex": 0,
			"nama":             "Tisha",
		}},
	}

	taken := map[string]bool{}
	recordEdgesTaken(taken, &workflow.Nodes[0], switchOutput, workflow)

	if !taken["switch-id:0:set-tisha:0"] {
		t.Errorf("matched branch 0 (Tisha) should be taken; got %v", taken)
	}
	if taken["switch-id:1:set-erwan:0"] {
		t.Errorf("unmatched branch 1 (Erwan) should NOT be taken; got %v", taken)
	}
	if taken["switch-id:2:set-others:0"] {
		t.Errorf("unmatched last branch (Others) should NOT be taken; got %v", taken)
	}
}

// TestRecordEdgesTaken_SwitchFallbackToLastStillMarksLastBranch
// covers the case where fallbackToLast IS supposed to route the
// item down the last branch. When the Switch sees no matching rule
// AND fallbackToLast is set, it emits an item with
// `_switchRuleIndex: len(rules)` — which IS routed to branch
// `len-1`. The regression must not have over-corrected by
// discarding the last branch entirely: when the input item
// genuinely arrives with `_switchRuleIndex == len(rules)`, the
// last branch MUST still be marked taken.
func TestRecordEdgesTaken_SwitchFallbackToLastStillMarksLastBranch(t *testing.T) {
	workflow := &model.Workflow{
		Name: "SwitchFallback",
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

	// The Switch emitted one item with the fallback tag — `len(rules)=3`.
	// The loop in recordEdgesTaken should pick this up and mark branch 2.
	switchOutput := []model.DataItem{
		{JSON: map[string]interface{}{"_switchRuleIndex": 3, "v": "fallback"}},
	}

	taken := map[string]bool{}
	recordEdgesTaken(taken, &workflow.Nodes[0], switchOutput, workflow)

	if !taken["src:2:r2:0"] {
		t.Errorf("fallback branch 2 should be taken; got %v", taken)
	}
	if taken["src:0:r0:0"] {
		t.Errorf("non-fallback branch 0 should NOT be taken; got %v", taken)
	}
	if taken["src:1:r1:0"] {
		t.Errorf("non-fallback branch 1 should NOT be taken; got %v", taken)
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

// TestExecuteWorkflow_SubWorkflowEdgesMergedIntoParent covers the
// regression fixed in the EdgesTaken parent-merge path: when a
// parent workflow invokes an ExecuteWorkflow node that runs a
// sub-workflow, the parent's edgesTaken accumulator used to be
// reset on the inner call and clobbered by the sub-workflow's
// keys. The frontend then couldn't find any of the parent
// workflow's edges in the persisted EdgesTaken map and fell
// through to the default grey stroke for every parent edge.
//
// This test simulates the recursion directly: the engine field
// `edgesTaken` is the shared accumulator, so manually seeding it
// with parent edges and then calling ExecuteWorkflow on a
// different workflow exercises the same merge path the
// ExecuteWorkflowNode → EngineAdapter recursion uses. After the
// call, both the parent-seeded edge AND the sub-workflow's
// newly-recorded edge must be present.
func TestExecuteWorkflow_SubWorkflowEdgesMergedIntoParent(t *testing.T) {
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

	// Seed the parent's accumulator with an edge that
	// represents the parent's Webhook → Call SubWorkflow
	// connection. In production this key is set when the
	// parent's Webhook executes; the regression was that the
	// sub-workflow's inner ExecuteWorkflowWithContext call
	// wiped this map on reset.
	parentEdge := "parent-webhook:0:parent-call:0"
	engine.edgesTaken = map[string]bool{parentEdge: true}

	// Now run a separate workflow on the same engine instance
	// (mirrors what the ExecuteWorkflowNode does internally
	// when it recurses into ExecuteWorkflowWithContext with
	// the child workflow). The accumulator must be merged,
	// not replaced.
	subWorkflow := &model.Workflow{
		Name:   "Sub",
		Active: false,
		Nodes: []model.Node{
			{ID: "s1", Name: "SubStart", Type: "n8n-nodes-base.start"},
			{ID: "s2", Name: "SubSet", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"SubStart": {Main: [][]model.Connection{{{Node: "SubSet", Type: "main", Index: 0}}}},
		},
	}

	result, err := engine.ExecuteWorkflow(subWorkflow, []model.DataItem{{JSON: map[string]interface{}{"x": 1}}})
	if err != nil {
		t.Fatalf("ExecuteWorkflow failed: %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil ExecutionResult")
	}

	// Parent's seeded edge must still be present — that's
	// the fix. Pre-fix this assertion would fail because the
	// reset on the inner call dropped the parent's keys.
	if !result.EdgesTaken[parentEdge] {
		t.Errorf("parent edge %q was lost; EdgesTaken=%v", parentEdge, result.EdgesTaken)
	}
	// Sub-workflow's own edge must also be present so the UI
	// can colour it green.
	if !result.EdgesTaken["s1:0:s2:0"] {
		t.Errorf("sub-workflow edge s1:0:s2:0 missing; EdgesTaken=%v", result.EdgesTaken)
	}

	// And the engine field itself must still be populated so
	// any further edges the parent records after this call
	// (i.e. the parent's edges LEAVING the Call SubWorkflow
	// node) keep accumulating into the same combined map
	// rather than starting a fresh one.
	if engine.edgesTaken == nil {
		t.Fatalf("engine.edgesTaken is nil after sub-workflow call; parent loop would lose edges")
	}
	if !engine.edgesTaken[parentEdge] || !engine.edgesTaken["s1:0:s2:0"] {
		t.Errorf("engine.edgesTaken missing merged keys after sub-workflow call; got %v", engine.edgesTaken)
	}
}
