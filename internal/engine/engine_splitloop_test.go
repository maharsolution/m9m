package engine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/transform"
)

// countingSetNode is a minimal Set-like node for splitInBatches tests.
// It records each invocation so tests can assert the number of times
// the body chain ran (== number of loop iterations) and the inputs
// the body chain saw (== per-batch slices).
type countingSetNode struct {
	*base.BaseNode

	// assignments is a list of static field names + value resolvers.
	// The node appends one assignment per call to `output` so tests
	// can verify the loop driver's per-iteration state handling.
	assignments []struct {
		name  string
		value func(in model.DataItem) interface{}
	}

	mu        sync.Mutex
	callCount int
	lastInput []model.DataItem
}

func newCountingSetNode() *countingSetNode {
	return &countingSetNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "CountingSet",
			Description: "Test-only Set node that records invocations",
			Category:    "Test",
		}),
	}
}

func (c *countingSetNode) Description() base.NodeDescription {
	return c.BaseNode.Description()
}

func (c *countingSetNode) ValidateParameters(params map[string]interface{}) error {
	return nil
}

func (c *countingSetNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	c.mu.Lock()
	c.callCount++
	c.lastInput = inputData
	c.mu.Unlock()

	out := make([]model.DataItem, 0, len(inputData))
	for _, item := range inputData {
		newJSON := make(map[string]interface{}, len(item.JSON)+len(c.assignments))
		for k, v := range item.JSON {
			newJSON[k] = v
		}
		for _, a := range c.assignments {
			newJSON[a.name] = a.value(item)
		}
		out = append(out, model.DataItem{JSON: newJSON})
	}
	return out, nil
}

// TestSplitInBatchesLoopDrivesBodyChain verifies that the engine
// actually iterates the body chain once per batch instead of
// running Process Item a single time with the full input. n8n's
// Loop pattern requires per-batch iteration: a 6-item input with
// batchSize=10 must yield 1 body invocation (not 6, not 0).
func TestSplitInBatchesLoopDrivesBodyChain(t *testing.T) {
	setupTestEnv(t)
	e := NewWorkflowEngine().(*workflowEngineImpl)

	processNode := newCountingSetNode()
	processNode.assignments = append(processNode.assignments, struct {
		name  string
		value func(in model.DataItem) interface{}
	}{"processedAt", func(in model.DataItem) interface{} { return "2026-09-08" }})

	doneNode := newCountingSetNode()
	doneNode.assignments = append(doneNode.assignments, struct {
		name  string
		value func(in model.DataItem) interface{}
	}{"response", func(in model.DataItem) interface{} { return "success" }})

	e.RegisterNodeExecutor("n8n-nodes-base.splitInBatches", transform.NewSplitInBatchesNode())
	e.RegisterNodeExecutor("n8n-nodes-base.set.process", processNode)
	e.RegisterNodeExecutor("n8n-nodes-base.set.done", doneNode)
	// Wire a simple webhook trigger so findStartingNodes has an
	// explicit entry point. The webhook executor is just a
	// pass-through for testing.
	e.RegisterNodeExecutor("n8n-nodes-base.webhook", &passThroughNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:     "Webhook",
			Category: "Trigger",
		}),
	})

	workflow := &model.Workflow{
		ID:     "test-loop",
		Name:   "Loop Test",
		Active: true,
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{0, 0}},
			{
				Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches",
				Position: []int{200, 0},
				Parameters: map[string]interface{}{
					"batchSize": 10,
				},
			},
			{Name: "Process Item", Type: "n8n-nodes-base.set.process", Position: []int{400, 100}},
			{Name: "Done / Output Final", Type: "n8n-nodes-base.set.done", Position: []int{400, -100}},
		},
		Connections: map[string]model.Connections{
			"Webhook": {
				Main: [][]model.Connection{
					{{Node: "Loop Over Items", Type: "main", Index: 0}},
				},
			},
			"Loop Over Items": {
				Main: [][]model.Connection{
					// main[0] = done branch (n8n convention)
					{{Node: "Done / Output Final", Type: "main", Index: 0}},
					// main[1] = process branch
					{{Node: "Process Item", Type: "main", Index: 0}},
				},
			},
			"Process Item": {
				Main: [][]model.Connection{
					// back-edge to splitInBatches
					{{Node: "Loop Over Items", Type: "main", Index: 0}},
				},
			},
		},
	}

	input := []model.DataItem{
		{JSON: map[string]interface{}{"id": float64(1), "name": "Andi", "status": "pending"}},
		{JSON: map[string]interface{}{"id": float64(2), "name": "Budi", "status": "pending"}},
		{JSON: map[string]interface{}{"id": float64(3), "name": "Citra", "status": "pending"}},
		{JSON: map[string]interface{}{"id": float64(4), "name": "Dewi", "status": "pending"}},
		{JSON: map[string]interface{}{"id": float64(5), "name": "Eko", "status": "pending"}},
		{JSON: map[string]interface{}{"id": float64(6), "name": "Erwan", "status": "ok"}},
	}

	result, err := e.ExecuteWorkflow(workflow, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Process Item must have been invoked exactly once (one batch
	// fits all 6 items at batchSize=10). The body chain's input
	// should have been the full 6-item slice.
	if processNode.callCount != 1 {
		t.Errorf("expected Process Item to run 1 time (single batch), got %d", processNode.callCount)
	}
	if len(processNode.lastInput) != 6 {
		t.Errorf("expected Process Item to see 6 input items, got %d", len(processNode.lastInput))
	}

	// Done / Output Final must have been invoked exactly once with
	// the full 6-item aggregated output.
	if doneNode.callCount != 1 {
		t.Errorf("expected Done / Output Final to run 1 time, got %d", doneNode.callCount)
	}
	if len(doneNode.lastInput) != 6 {
		t.Errorf("expected Done / Output Final to see 6 items, got %d", len(doneNode.lastInput))
	}

	// The engine's published output should reflect the done branch
	// (the last node in the execution graph) and therefore include
	// `processedAt` and `response` for every item, with `status`
	// preserved from the original input (no overwrites).
	if result == nil || len(result.Data) != 6 {
		t.Fatalf("expected 6 items in final result, got %d", len(result.Data))
	}
	for i, item := range result.Data {
		if _, ok := item.JSON["processedAt"]; !ok {
			t.Errorf("item %d missing processedAt: %+v", i, item.JSON)
		}
		if v, _ := item.JSON["response"].(string); v != "success" {
			t.Errorf("item %d expected response=success, got %v", i, item.JSON["response"])
		}
		if v, _ := item.JSON["status"].(string); v != input[i].JSON["status"] {
			t.Errorf("item %d expected status=%v (preserved from input), got %v", i, input[i].JSON["status"], v)
		}
	}
}

// TestSplitInBatchesLoopIteratesMultipleBatches verifies that with
// batchSize smaller than the input length, the engine iterates the
// body chain once per batch and the done branch sees the full
// aggregated output.
func TestSplitInBatchesLoopIteratesMultipleBatches(t *testing.T) {
	setupTestEnv(t)
	e := NewWorkflowEngine().(*workflowEngineImpl)

	processNode := newCountingSetNode()
	processNode.assignments = append(processNode.assignments, struct {
		name  string
		value func(in model.DataItem) interface{}
	}{"processedAt", func(in model.DataItem) interface{} { return "2026-09-08" }})

	doneNode := newCountingSetNode()
	doneNode.assignments = append(doneNode.assignments, struct {
		name  string
		value func(in model.DataItem) interface{}
	}{"response", func(in model.DataItem) interface{} { return "ok" }})

	e.RegisterNodeExecutor("n8n-nodes-base.splitInBatches", transform.NewSplitInBatchesNode())
	e.RegisterNodeExecutor("n8n-nodes-base.set.process", processNode)
	e.RegisterNodeExecutor("n8n-nodes-base.set.done", doneNode)
	e.RegisterNodeExecutor("n8n-nodes-base.webhook", &passThroughNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:     "Webhook",
			Category: "Trigger",
		}),
	})

	workflow := &model.Workflow{
		ID:   "test-loop-multi",
		Name: "Loop Multi-Batch",
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{0, 0}},
			{Name: "Loop Over Items", Type: "n8n-nodes-base.splitInBatches",
				Position:    []int{200, 0},
				Parameters:  map[string]interface{}{"batchSize": 2}},
			{Name: "Process Item", Type: "n8n-nodes-base.set.process", Position: []int{400, 100}},
			{Name: "Done", Type: "n8n-nodes-base.set.done", Position: []int{400, -100}},
		},
		Connections: map[string]model.Connections{
			"Webhook":          {Main: [][]model.Connection{{{Node: "Loop Over Items", Type: "main", Index: 0}}}},
			"Loop Over Items":  {Main: [][]model.Connection{{{Node: "Done", Type: "main", Index: 0}}, {{Node: "Process Item", Type: "main", Index: 0}}}},
			"Process Item":     {Main: [][]model.Connection{{{Node: "Loop Over Items", Type: "main", Index: 0}}}},
		},
	}

	input := make([]model.DataItem, 6)
	for i := range input {
		input[i] = model.DataItem{JSON: map[string]interface{}{
			"id": float64(i + 1), "name": fmt.Sprintf("Item%d", i+1), "status": "pending",
		}}
	}

	result, err := e.ExecuteWorkflow(workflow, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if processNode.callCount != 3 {
		t.Errorf("expected Process Item to run 3 times (6 items / batchSize=2), got %d", processNode.callCount)
	}
	if doneNode.callCount != 1 {
		t.Errorf("expected Done to run 1 time, got %d", doneNode.callCount)
	}
	if len(result.Data) != 6 {
		t.Errorf("expected 6 items in final result, got %d", len(result.Data))
	}
}

// passThroughNode is a no-op executor that returns its input
// unchanged. Used in tests where a real trigger behaviour is not
// relevant.
type passThroughNode struct {
	*base.BaseNode
}

func (p *passThroughNode) Execute(input []model.DataItem, _ map[string]interface{}) ([]model.DataItem, error) {
	return input, nil
}

func (p *passThroughNode) ValidateParameters(_ map[string]interface{}) error {
	return nil
}
