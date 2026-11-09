package engine

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// recordingNode captures the items it was invoked with so the test can
// assert that the Manual Trigger actually forwarded its item.
type recordingNode struct {
	*base.BaseNode
	calls   int
	lastIn  []model.DataItem
	lastOut []model.DataItem
}

func newRecordingNode(name string, out []model.DataItem) *recordingNode {
	return &recordingNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:       name,
			Category:   "Transform",
			Properties: []base.NodeProperty{},
		}),
		lastOut: out,
	}
}

func (r *recordingNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	r.calls++
	r.lastIn = inputData
	return r.lastOut, nil
}

func (r *recordingNode) ValidateParameters(params map[string]interface{}) error { return nil }

// TestManualTriggerForwardsItemsDownstream covers the regression where
// a workflow whose first node is a Manual Trigger produced NO output
// at all.
//
// The engine treated trigger-only nodes as pure no-ops: it cleared
// `nodeResults` for them and never routed their items downstream, so
// every node after the Manual Trigger was skipped with "empty input".
// A `Manual Trigger → Set → Kafka Produce` workflow therefore
// published nothing to the broker.
func TestManualTriggerForwardsItemsDownstream(t *testing.T) {
	eng := NewWorkflowEngine()

	producer := newRecordingNode("Producer", []model.DataItem{
		{JSON: map[string]interface{}{"produced": true}},
	})
	eng.RegisterNodeExecutor("n8n-nodes-base.manualTrigger", &noopTrigger{})
	eng.RegisterNodeExecutor("n8n-nodes-base.set", producer)

	workflow := &model.Workflow{
		ID:   "wf-manual-trigger",
		Name: "Manual Trigger Forwarding",
		Nodes: []model.Node{
			{Name: "Start", Type: "n8n-nodes-base.manualTrigger", Position: []int{-300, 0}},
			{Name: "Producer", Type: "n8n-nodes-base.set", Position: []int{0, 0}},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{{{Node: "Producer", Type: "main", Index: 0}}}},
		},
	}

	result, err := eng.ExecuteWorkflow(workflow, []model.DataItem{{JSON: map[string]interface{}{"seed": 1}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error != nil {
		t.Fatalf("workflow error: %v", result.Error)
	}

	if producer.calls != 1 {
		t.Fatalf("expected the downstream node to run exactly once, ran %d times", producer.calls)
	}
	if len(producer.lastIn) != 1 {
		t.Fatalf("expected 1 item forwarded to the downstream node, got %d", len(producer.lastIn))
	}
	if producer.lastIn[0].JSON["seed"] != 1 {
		t.Errorf("expected the trigger's item to be forwarded verbatim, got %#v", producer.lastIn[0].JSON)
	}

	if len(result.Data) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(result.Data))
	}
	if result.Data[0].JSON["produced"] != true {
		t.Errorf("expected the downstream node's output, got %#v", result.Data[0].JSON)
	}
}

// noopTrigger stands in for the Manual Trigger node. m9m has no
// executor registered for `n8n-nodes-base.manualTrigger` — the engine
// recognises it as a trigger-only node and skips it — but the test
// registers one so the code path under test (the executor lookup
// succeeding) matches production more closely than a lookup failure.
type noopTrigger struct {
	*base.BaseNode
}

func (n *noopTrigger) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return inputData, nil
}

func (n *noopTrigger) ValidateParameters(params map[string]interface{}) error { return nil }
