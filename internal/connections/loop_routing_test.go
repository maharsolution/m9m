package connections

import (
	"fmt"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSplitInBatchesLoop_RoutingData verifies that the connection
// router partitions splitInBatches output by `_loopDone` so that
// main[0] (Done) gets the full list and main[1] (Process Item)
// gets the batch payload.
func TestSplitInBatchesLoop_RoutingData(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Mock Data", Type: "n8n-nodes-base.code"},
			{Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches"},
			{Name: "Process Item", Type: "n8n-nodes-base.set"},
			{Name: "Done", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Mock Data": {Main: [][]model.Connection{{{Node: "Loop Over Items"}}}},
			"Loop Over Items": {
				Main: [][]model.Connection{
					{{Node: "Done"}},
					{{Node: "Process Item"}},
				},
			},
			"Process Item": {Main: [][]model.Connection{{{Node: "Loop Over Items"}}}},
		},
	}

	// Construct splitInBatches-style output: 1 batch payload
	// (_loopDone=false) + 5 done items (_loopDone=true).
	items := []model.DataItem{
		{JSON: map[string]interface{}{"id": 1, "name": "Andi", "status": "PROCESSED", "_loopDone": false}},
		{JSON: map[string]interface{}{"id": 1, "name": "Andi", "status": "PROCESSED", "_loopDone": true}},
		{JSON: map[string]interface{}{"id": 2, "name": "Budi", "status": "PROCESSED", "_loopDone": true}},
		{JSON: map[string]interface{}{"id": 3, "name": "Citra", "status": "PROCESSED", "_loopDone": true}},
		{JSON: map[string]interface{}{"id": 4, "name": "Dewi", "status": "PROCESSED", "_loopDone": true}},
		{JSON: map[string]interface{}{"id": 5, "name": "Eko", "status": "PROCESSED", "_loopDone": true}},
	}

	router := NewConnectionRouter()
	routed, err := router.RouteData("Loop Over Items", wf, items)
	if err != nil {
		t.Fatalf("RouteData: %v", err)
	}
	fmt.Printf("routed keys: %v\n", keys(routed))
	for k, v := range routed {
		fmt.Printf("  %s -> %d items\n", k, len(v))
		for _, item := range v {
			fmt.Printf("    %v\n", item.JSON)
		}
	}
	if got := len(routed["Done"]); got != 5 {
		t.Fatalf("Done branch expected 5 items, got %d", got)
	}
	if got := len(routed["Process Item"]); got != 1 {
		t.Fatalf("Process Item branch expected 1 item, got %d", got)
	}
}

func keys(m map[string][]model.DataItem) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
