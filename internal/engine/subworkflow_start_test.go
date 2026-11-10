package engine

import (
	"context"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/trigger"
)

// Finnet post_balance calls Execute Workflow on the SAME workflow.
// Without sub-workflow start selection the engine re-seeds Webhook
// and recurses (inputLen 1→2→4→…→64) until the client times out.
// With WithSubWorkflowStart only executeWorkflowTrigger runs.
func TestSubWorkflowStart_OnlyExecuteWorkflowTrigger(t *testing.T) {
	eng := NewWorkflowEngine()

	webhook := newRecordingNode("Webhook", []model.DataItem{{JSON: map[string]interface{}{"from": "webhook"}}})
	payload := newRecordingNode("Payload", []model.DataItem{{JSON: map[string]interface{}{"from": "payload"}}})
	setCounter := newRecordingNode("Set Counter", []model.DataItem{{JSON: map[string]interface{}{"attempt": 1}}})

	eng.RegisterNodeExecutor("n8n-nodes-base.executeWorkflowTrigger", trigger.NewExecuteWorkflowTriggerNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.webhook", webhook)
	eng.RegisterNodeExecutor("n8n-nodes-base.set", payload)
	eng.RegisterNodeExecutor("n8n-nodes-base.code", setCounter)

	wf := &model.Workflow{
		ID:   "finnet-like",
		Name: "Finnet-like",
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{0, 0}},
			{Name: "Async Subworkflow", Type: "n8n-nodes-base.executeWorkflowTrigger", Position: []int{0, 200}},
			{Name: "Set Counter", Type: "n8n-nodes-base.code", Position: []int{200, 200}},
			{Name: "Payload", Type: "n8n-nodes-base.set", Position: []int{200, 0}},
		},
		Connections: map[string]model.Connections{
			"Webhook":           {Main: [][]model.Connection{{{Node: "Payload", Type: "main", Index: 0}}}},
			"Async Subworkflow": {Main: [][]model.Connection{{{Node: "Set Counter", Type: "main", Index: 0}}}},
		},
	}

	input := []model.DataItem{{JSON: map[string]interface{}{"key": "k1"}}}
	ctx := WithSubWorkflowStart(context.Background(), 1)
	ctxEng, ok := eng.(ContextWorkflowEngine)
	if !ok {
		t.Fatal("engine must implement ContextWorkflowEngine")
	}
	res, err := ctxEng.ExecuteWorkflowWithContext(ctx, wf, input)
	if err != nil {
		t.Fatalf("ExecuteWorkflowWithContext: %v", err)
	}
	if res != nil && res.Error != nil {
		t.Fatalf("result.Error: %v", res.Error)
	}

	if webhook.calls != 0 {
		t.Fatalf("Webhook must not run on subworkflow start; calls=%d", webhook.calls)
	}
	if payload.calls != 0 {
		t.Fatalf("Payload (webhook branch) must not run; calls=%d", payload.calls)
	}
	if setCounter.calls != 1 {
		t.Fatalf("Set Counter should run once via EWT; calls=%d", setCounter.calls)
	}
}
