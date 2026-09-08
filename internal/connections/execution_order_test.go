package connections

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestGetExecutionOrder_StableAcrossRuns is the regression test for
// the webhook_loop flake that surfaced on 2026-09-05: the same
// workflow JSON produced different execution orders from run to
// run because Go's map iteration order is randomised, so the
// topological sort in GetExecutionOrder picked a different
// starting node each time. With one ordering the
// "Code -> splitInBatches -> Done" path was taken (5 items); with
// another, "splitInBatches -> Done -> Code" was taken (1 item).
//
// The fix is sortedNodeNamesByPosition, which gives the engine a
// deterministic traversal that follows the n8n canvas layout
// (top-left to bottom-right). This test runs GetExecutionOrder
// many times and asserts the order is identical every time.
func TestGetExecutionOrder_StableAcrossRuns(t *testing.T) {
	wf := buildWebhookLoopWorkflow()
	router := NewConnectionRouter()

	first, err := router.GetExecutionOrder(wf)
	if err != nil {
		t.Fatalf("first GetExecutionOrder: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := router.GetExecutionOrder(wf)
		if err != nil {
			t.Fatalf("iteration %d: GetExecutionOrder: %v", i, err)
		}
		if !equalStringSlices(first, got) {
			t.Fatalf("iteration %d: order drifted.\nfirst: %v\ngot:   %v", i, first, got)
		}
	}
}

// TestGetExecutionOrder_RespectsPosition asserts that the stable
// ordering follows the n8n canvas position convention (X ascending,
// then Y ascending) so the resulting execution order reads like the
// workflow looks on screen.
func TestGetExecutionOrder_RespectsPosition(t *testing.T) {
	wf := buildWebhookLoopWorkflow()
	router := NewConnectionRouter()

	got, err := router.GetExecutionOrder(wf)
	if err != nil {
		t.Fatalf("GetExecutionOrder: %v", err)
	}

	// Expected order: "Process Item" (the body) must execute
	// before "Done / Output Final" (the done-branch) because the
	// done-branch aggregates the body's output. The router adds a
	// synthetic dependency from the deepest process-branch node
	// to each main[0] target to enforce n8n's "fire done after
	// body" invariant (see addSplitInBatchesDoneBranchDependencies).
	// Position-based tie-breaking then orders within the same
	// dependency level: at X=240, "Done / Output Final" (Y=192)
	// would normally come before "Process Item" (Y=384), but the
	// synthetic dependency forces Process Item first.
	want := []string{
		"When clicking 'Test workflow'", // position [-432, 192] (manualTrigger)
		"Webhook",                       // position [-432, 384]
		"Generate Mock Data",            // position [-208, 336]
		"Loop Over Items",               // position [16, 336] (splitInBatches)
		"Process Item",                  // position [240, 384] — body must run before done-branch
		"Done / Output Final",           // position [240, 192] — done-branch fires after body
		"Respond to Webhook",            // position [464, 192]
	}
	if !equalStringSlices(want, got) {
		t.Fatalf("execution order did not match expected position-based order.\nwant: %v\ngot:  %v", want, got)
	}
}

// buildWebhookLoopWorkflow returns a minimal copy of the webhook_loop
// workflow that surfaced the flake. The exact node/connection shapes
// don't matter for this test — only that GetExecutionOrder is called
// on a workflow with splitInBatches and that the node positions
// produce the expected stable order.
func buildWebhookLoopWorkflow() *model.Workflow {
	return &model.Workflow{
		ID:   "t8xPqfr92w5HGeOv",
		Name: "Simple Webhook - Loop",
		Nodes: []model.Node{
			{Name: "When clicking 'Test workflow'", Type: "n8n-nodes-base.manualTrigger", Position: []int{-432, 192}, TypeVersion: 1},
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{-432, 384}, TypeVersion: 2},
			{Name: "Generate Mock Data", Type: "n8n-nodes-base.code", Position: []int{-208, 336}, TypeVersion: 2},
			{Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches", Position: []int{16, 336}, TypeVersion: 3},
			{Name: "Process Item", Type: "n8n-nodes-base.set", Position: []int{240, 384}, TypeVersion: 3},
			{Name: "Done / Output Final", Type: "n8n-nodes-base.set", Position: []int{240, 192}, TypeVersion: 3},
			{Name: "Respond to Webhook", Type: "n8n-nodes-base.respondToWebhook", Position: []int{464, 192}, TypeVersion: 1},
		},
		Connections: map[string]model.Connections{
			"When clicking 'Test workflow'": {
				Main: [][]model.Connection{
					{{Node: "Generate Mock Data", Type: "main", Index: 0}},
				},
			},
			"Webhook": {
				Main: [][]model.Connection{
					{{Node: "Generate Mock Data", Type: "main", Index: 0}},
				},
			},
			"Generate Mock Data": {
				Main: [][]model.Connection{
					{{Node: "Loop Over Items", Type: "main", Index: 0}},
				},
			},
			"Loop Over Items": {
				Main: [][]model.Connection{
					{{Node: "Done / Output Final", Type: "main", Index: 0}},
					{{Node: "Process Item", Type: "main", Index: 0}},
				},
			},
			"Process Item": {
				Main: [][]model.Connection{
					{{Node: "Loop Over Items", Type: "main", Index: 0}},
				},
			},
			"Done / Output Final": {
				Main: [][]model.Connection{
					{{Node: "Respond to Webhook", Type: "main", Index: 0}},
				},
			},
		},
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
