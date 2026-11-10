package engine

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

func TestChooseBranchInput_DefaultsToInput1(t *testing.T) {
	node := &model.Node{
		Name:       "Merge",
		Type:       "n8n-nodes-base.merge",
		Parameters: map[string]interface{}{"mode": "chooseBranch"},
	}
	byInput := map[int][]model.DataItem{
		0: {{JSON: map[string]interface{}{"key": "exec_abc", "data": map[string]interface{}{"x": 1}}}},
		1: {{JSON: map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"route": "async"}}}}},
	}
	got, ok := chooseBranchInput(node, byInput)
	if !ok {
		t.Fatal("expected chooseBranch selection")
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].JSON["key"] != "exec_abc" {
		t.Fatalf("want Payload key, got %#v", got[0].JSON)
	}
}

func TestIncomingInputIndex(t *testing.T) {
	wf := &model.Workflow{
		Connections: map[string]model.Connections{
			"Payload": {Main: [][]model.Connection{{{Node: "Merge", Type: "main", Index: 0}}}},
			"isAsync": {Main: [][]model.Connection{{{Node: "Merge", Type: "main", Index: 1}}}},
		},
	}
	if got := incomingInputIndex(wf, "Payload", "Merge"); got != 0 {
		t.Fatalf("Payload→Merge: want 0 got %d", got)
	}
	if got := incomingInputIndex(wf, "isAsync", "Merge"); got != 1 {
		t.Fatalf("isAsync→Merge: want 1 got %d", got)
	}
}
