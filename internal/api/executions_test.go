package api

import (
	"testing"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
)

// TestBuildExecutionNodeData_ProductionMode_FallbackToResultData verifies the
// behaviour requested in PR #3237038 follow-up: when Debug is OFF and the
// topology-derived "last node" has no per-node output (e.g. an IF routed every
// item down a sibling branch), the helper falls back to recording the engine's
// canonical workflow.Data under the last-node key. Without this fallback the
// user would see an empty Output tab on the rightmost node in the canvas even
// though the workflow clearly produced output.
func TestBuildExecutionNodeData_ProductionMode_FallbackToResultData(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Webhook-Jwt-If-Respond",
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook"},
			{Name: "Edit Fields", Type: "n8n-nodes-base.set"},
			{Name: "If", Type: "n8n-nodes-base.if"},
			{Name: "Respond to Webhook", Type: "n8n-nodes-base.respondToWebhook"},
		},
		Connections: map[string]model.Connections{
			"Webhook":     {Main: [][]model.Connection{{{Node: "Edit Fields", Type: "main", Index: 0}}}},
			"Edit Fields": {Main: [][]model.Connection{{{Node: "If", Type: "main", Index: 0}}}},
			"If":          {Main: [][]model.Connection{{{Node: "Respond to Webhook", Type: "main", Index: 0}}}},
		},
		Debug: false, // production mode (default)
	}

	// Engine routed the IF item down main[1] (false branch) and never
	// touched Respond to Webhook, so NodeOutputs["Respond to Webhook"]
	// is empty — but result.Data still carries the workflow-level
	// final output (whatever the engine picked as the last node with
	// data).
	result := &engine.ExecutionResult{
		NodeOutputs: map[string][]model.DataItem{
			"Webhook":            {{JSON: map[string]interface{}{"body": "req"}}},
			"Edit Fields":        {{JSON: map[string]interface{}{"=response": "test"}}},
			"If":                 {{JSON: map[string]interface{}{"=response": "test", "_ifResult": false}}},
			"Respond to Webhook": {}, // skipped — no items routed here
		},
		Data: []model.DataItem{{JSON: map[string]interface{}{"=response": "test"}}}, // canonical workflow result
	}

	out := buildExecutionNodeData(workflow, result)

	startKey, _ := findStartNodeName(workflow), ""
	lastKey := findLastNodeName(workflow)

	if startKey == "" {
		t.Fatal("findStartNodeName returned empty for a simple linear chain")
	}
	if lastKey != "Respond to Webhook" {
		t.Fatalf("findLastNodeName = %q, want Respond to Webhook", lastKey)
	}

	// Start node (Webhook) snapshot must be retained verbatim so the
	// NDV Input tab shows the trigger payload.
	if got, ok := out[startKey]; !ok {
		t.Errorf("start node %q missing from NodeData", startKey)
	} else if len(got) != 1 || got[0].JSON["body"] != "req" {
		t.Errorf("start node data wrong: %+v", got)
	}

	// Last node (Respond to Webhook) must show the workflow-level
	// result, NOT an empty slice — this is the bug the user reported.
	if got, ok := out[lastKey]; !ok {
		t.Errorf("last node %q missing from NodeData", lastKey)
	} else if len(got) == 0 {
		t.Errorf("last node data is empty; should fall back to result.Data when the topology leaf was skipped")
	} else if got[0].JSON["=response"] != "test" {
		t.Errorf("last node data wrong: %+v", got)
	}

	// Middle nodes must NOT be retained in production mode.
	if _, ok := out["Edit Fields"]; ok {
		t.Errorf("middle node Edit Fields should not appear in production-mode NodeData")
	}
	if _, ok := out["If"]; ok {
		t.Errorf("middle node If should not appear in production-mode NodeData")
	}
}

// TestBuildExecutionNodeData_ProductionMode_EmptyLastNodeAndEmptyResult
// documents the intentional "empty placeholder" behaviour: when Debug=OFF,
// the topology last node ran but produced no items, AND the engine's
// canonical result is also empty, the helper records an explicit empty
// slice so the NDV can render "no output" instead of silently dropping
// the node from the data map.
func TestBuildExecutionNodeData_ProductionMode_EmptyLastNodeAndEmptyResult(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Single-Node-Empty-Result",
		Nodes: []model.Node{
			{Name: "Webhook"},
			{Name: "End"},
		},
		Connections: map[string]model.Connections{
			"Webhook": {Main: [][]model.Connection{{{Node: "End", Type: "main", Index: 0}}}},
		},
		Debug: false,
	}

	result := &engine.ExecutionResult{
		NodeOutputs: map[string][]model.DataItem{
			"Webhook": {{JSON: map[string]interface{}{"body": "req"}}},
			"End":     {}, // ran but produced no output
		},
		Data: nil, // engine found no node with output either
	}

	out := buildExecutionNodeData(workflow, result)

	endKey := findLastNodeName(workflow)
	if _, ok := out[endKey]; !ok {
		t.Errorf("last node %q must be present (as empty slice), got map=%+v", endKey, out)
	}
}

// TestBuildExecutionNodeData_DebugMode_RetainsAllNodes verifies the
// complementary case: when Debug=ON, every node's per-node snapshot is
// preserved (no filtering).
func TestBuildExecutionNodeData_DebugMode_RetainsAllNodes(t *testing.T) {
	workflow := &model.Workflow{
		Name: "Debug-On",
		Nodes: []model.Node{
			{Name: "A"}, {Name: "B"}, {Name: "C"},
		},
		Connections: map[string]model.Connections{
			"A": {Main: [][]model.Connection{{{Node: "B"}}}},
			"B": {Main: [][]model.Connection{{{Node: "C"}}}},
		},
		Debug: true,
	}

	result := &engine.ExecutionResult{
		NodeOutputs: map[string][]model.DataItem{
			"A": {{JSON: map[string]interface{}{"a": 1}}},
			"B": {{JSON: map[string]interface{}{"b": 2}}},
			"C": {{JSON: map[string]interface{}{"c": 3}}},
		},
		Data: []model.DataItem{{JSON: map[string]interface{}{"c": 3}}},
	}

	out := buildExecutionNodeData(workflow, result)

	for _, name := range []string{"A", "B", "C"} {
		if _, ok := out[name]; !ok {
			t.Errorf("debug mode must retain node %q, got map=%+v", name, out)
		}
	}
}
