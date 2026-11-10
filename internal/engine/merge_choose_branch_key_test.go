package engine

import (
	"context"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/transform"
)

type captureNode struct {
	*base.BaseNode
	got []model.DataItem
}

func (c *captureNode) Execute(input []model.DataItem, _ map[string]interface{}) ([]model.DataItem, error) {
	c.got = append([]model.DataItem{}, input...)
	return input, nil
}
func (c *captureNode) ValidateParameters(map[string]interface{}) error { return nil }

func TestMergeChooseBranch_PassesOnlyPayloadKey(t *testing.T) {
	eng := NewWorkflowEngine()
	sink := &captureNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Sink"})}
	eng.RegisterNodeExecutor("n8n-nodes-base.webhook", &passThroughNoOp{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Webhook"})})
	eng.RegisterNodeExecutor("n8n-nodes-base.set", transform.NewSetNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.httpRequest", &captureNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "HTTP"})})
	// ServiceRoutePlans mock: return async route shape without key
	route := &captureNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Route"})}
	route.got = nil
	eng.RegisterNodeExecutor("n8n-nodes-base.httpRequest", &recordingHTTP{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "HTTP"}),
		out: []model.DataItem{{JSON: map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"route": "async"}}}}}})
	eng.RegisterNodeExecutor("n8n-nodes-base.if", transform.NewIfNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.merge", transform.NewMergeNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.noOp", sink)

	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook"},
			{Name: "Payload", Type: "n8n-nodes-base.set", Parameters: map[string]interface{}{
				"assignments": map[string]interface{}{"assignments": []interface{}{
					map[string]interface{}{"name": "key", "type": "string", "value": "={{ $execution.id }}"},
					map[string]interface{}{"name": "data", "type": "object", "value": "={{ $json.body }}"},
				}},
			}},
			{Name: "ServiceRoutePlans", Type: "n8n-nodes-base.httpRequest", Parameters: map[string]interface{}{"url": "http://example.invalid"}},
			{Name: "isAsync", Type: "n8n-nodes-base.if", Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "or",
					"conditions": []interface{}{map[string]interface{}{
						"leftValue": "={{ $json.data.attributes.route.toLowerCase() }}", "rightValue": "async",
						"operator": map[string]interface{}{"operation": "equals", "type": "string"},
					}},
				},
			}},
			{Name: "Merge", Type: "n8n-nodes-base.merge", Parameters: map[string]interface{}{"mode": "chooseBranch"}},
			{Name: "Sink", Type: "n8n-nodes-base.noOp"},
		},
		Connections: map[string]model.Connections{
			"Webhook":           {Main: [][]model.Connection{{{Node: "Payload", Type: "main", Index: 0}}}},
			"Payload":           {Main: [][]model.Connection{{{Node: "Merge", Type: "main", Index: 0}, {Node: "ServiceRoutePlans", Type: "main", Index: 0}}}},
			"ServiceRoutePlans": {Main: [][]model.Connection{{{Node: "isAsync", Type: "main", Index: 0}}}},
			"isAsync":           {Main: [][]model.Connection{{{Node: "Merge", Type: "main", Index: 1}}, {{Node: "Sink", Type: "main", Index: 0}}}},
			"Merge":             {Main: [][]model.Connection{{{Node: "Sink", Type: "main", Index: 0}}}},
		},
	}

	ctx := WithExecutionMode(context.Background(), "webhook")
	ctx = WithExecutionID(ctx, "exec_merge_key_1")
	input := []model.DataItem{{JSON: map[string]interface{}{"body": map[string]interface{}{"amount": 140000}}}}
	res, err := eng.(ContextWorkflowEngine).ExecuteWorkflowWithContext(ctx, wf, input)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res != nil && res.Error != nil {
		t.Fatalf("result.Error: %v", res.Error)
	}
	if len(sink.got) == 0 {
		t.Fatal("Sink got no items")
	}
	// Sink may receive from Merge and/or isAsync false — assert at least one item has key.
	var key string
	for _, it := range sink.got {
		if k, ok := it.JSON["key"].(string); ok && k != "" {
			key = k
			break
		}
	}
	if key != "exec_merge_key_1" {
		t.Fatalf("expected Merge to forward Payload key, sink=%v", sink.got)
	}
}

type recordingHTTP struct {
	*base.BaseNode
	out []model.DataItem
}

func (r *recordingHTTP) Execute(input []model.DataItem, _ map[string]interface{}) ([]model.DataItem, error) {
	return r.out, nil
}
func (r *recordingHTTP) ValidateParameters(map[string]interface{}) error { return nil }
