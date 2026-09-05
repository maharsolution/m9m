package connections

import (
	"fmt"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSplitInBatchesLoop_RoutingOrder prints the execution order for
// the canonical n8n loop pattern so we can confirm both Done and
// Process Item appear in the topological sort.
func TestSplitInBatchesLoop_RoutingOrder(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Mock Data", Type: "n8n-nodes-base.code"},
			{Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches"},
			{Name: "Process Item", Type: "n8n-nodes-base.set"},
			{Name: "Done", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Mock Data":     {Main: [][]model.Connection{{{Node: "Loop Over Items"}}}},
			"Loop Over Items": {
				Main: [][]model.Connection{
					{{Node: "Done"}},
					{{Node: "Process Item"}},
				},
			},
			"Process Item": {Main: [][]model.Connection{{{Node: "Loop Over Items"}}}},
		},
	}
	router := NewConnectionRouter()
	order, err := router.GetExecutionOrder(wf)
	if err != nil {
		t.Fatalf("GetExecutionOrder: %v", err)
	}
	fmt.Printf("execution order: %v\n", order)
	for i, n := range order {
		fmt.Printf("  [%d] %s\n", i, n)
	}
}
