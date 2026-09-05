package connections

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSplitInBatchesLoop_NoCycleError verifies that the canonical
// n8n splitInBatches loop pattern (`Process Item → Loop Over Items`
// back-edge) does not cause HasCycles or GetExecutionOrder to fail.
//
// The pattern is:
//
//	Loop Over Items (splitInBatches) → Done / Output Final (main[0])
//	                              → Process Item (main[1])
//	Process Item → Loop Over Items   (back-edge, tolerated)
func TestSplitInBatchesLoop_NoCycleError(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Mock Data", Type: "n8n-nodes-base.code"},
			{Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches"},
			{Name: "Process Item", Type: "n8n-nodes-base.set"},
			{Name: "Done", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Mock Data": {
				Main: [][]model.Connection{
					{{Node: "Loop Over Items"}},
				},
			},
			"Loop Over Items": {
				Main: [][]model.Connection{
					{{Node: "Done"}},        // main[0] = done / no more batches
					{{Node: "Process Item"}}, // main[1] = process next batch
				},
			},
			"Process Item": {
				Main: [][]model.Connection{
					{{Node: "Loop Over Items"}}, // back-edge
				},
			},
		},
	}

	router := NewConnectionRouter()
	hasCycles, err := router.HasCycles(wf)
	if err != nil {
		t.Fatalf("HasCycles returned error: %v", err)
	}
	if hasCycles {
		t.Fatalf("HasCycles returned true for canonical splitInBatches loop, want false")
	}

	order, err := router.GetExecutionOrder(wf)
	if err != nil {
		t.Fatalf("GetExecutionOrder returned error: %v", err)
	}
	// All four nodes must appear in the execution order; the back-edge
	// is ignored so ordering is well-defined.
	if len(order) != 4 {
		t.Fatalf("expected 4 nodes in execution order, got %d: %v", len(order), order)
	}
}

// TestSplitInBatchesLoop_RealCycleStillFails verifies that genuine
// cycles (no splitInBatches present) still fail, so we don't
// accidentally mask broken workflows.
func TestSplitInBatchesLoop_RealCycleStillFails(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "A", Type: "n8n-nodes-base.code"},
			{Name: "B", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"A": {Main: [][]model.Connection{{{Node: "B"}}}},
			"B": {Main: [][]model.Connection{{{Node: "A"}}}},
		},
	}
	router := NewConnectionRouter()
	hasCycles, err := router.HasCycles(wf)
	if err != nil {
		t.Fatalf("HasCycles returned error: %v", err)
	}
	if !hasCycles {
		t.Fatal("expected HasCycles=true for genuine cycle")
	}
}
