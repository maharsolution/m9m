package engine

import (
	"context"
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/transform"
	"github.com/neul-labs/m9m/internal/nodes/trigger"
)

type passThroughNoOp struct{ *base.BaseNode }

func (p *passThroughNoOp) Execute(input []model.DataItem, _ map[string]interface{}) ([]model.DataItem, error) {
	return input, nil
}
func (p *passThroughNoOp) ValidateParameters(map[string]interface{}) error { return nil }

// recordingMongo captures the query filter seen by Find Respond.
type recordingMongo struct {
	*base.BaseNode
	queries []interface{}
	runKeys []string
}

func (r *recordingMongo) Execute(input []model.DataItem, params map[string]interface{}) ([]model.DataItem, error) {
	return r.ExecuteWithRun(nil, nil, 0, 0, input, params)
}

func (r *recordingMongo) ExecuteWithRun(wf *model.Workflow, runData *expressions.RunExecutionData, runIndex, itemIndex int, input []model.DataItem, params map[string]interface{}) ([]model.DataItem, error) {
	key := ""
	if runData != nil && runData.ResultData != nil {
		if slots := runData.ResultData.NodeData["Async Subworkflow"]; len(slots) > 0 && len(slots[0].Data) > 0 {
			if k, ok := slots[0].Data[0].JSON["key"].(string); ok {
				key = k
			}
		}
	}
	r.runKeys = append(r.runKeys, key)
	r.queries = append(r.queries, params["query"])
	// Always miss so the If-retry loop keeps polling briefly.
	return []model.DataItem{}, nil
}

func (r *recordingMongo) ValidateParameters(params map[string]interface{}) error { return nil }

func TestFinnetChild_FindSeesAsyncSubworkflowKey(t *testing.T) {
	eng := NewWorkflowEngine()
	mongo := &recordingMongo{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Mongo"})}
	eng.RegisterNodeExecutor("n8n-nodes-base.executeWorkflowTrigger", trigger.NewExecuteWorkflowTriggerNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.code", transform.NewCodeNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.set", transform.NewSetNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.if", transform.NewIfNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.mongoDb", mongo)
	eng.RegisterNodeExecutor("n8n-nodes-base.noOp", &passThroughNoOp{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "NoOp"})})

	wf := &model.Workflow{
		ID: "child",
		Nodes: []model.Node{
			{Name: "Async Subworkflow", Type: "n8n-nodes-base.executeWorkflowTrigger"},
			{Name: "Set Counter", Type: "n8n-nodes-base.code", Parameters: map[string]interface{}{
				"jsCode": "let currentAttempt = $input.item.json.attempt || 0; currentAttempt += 1; return { json: { attempt: currentAttempt, maxAttempts: 2 } };",
			}},
			{Name: "Find Respond", Type: "n8n-nodes-base.mongoDb", Parameters: map[string]interface{}{
				"collection": "finnet_store",
				"query":      `={ "key": "{{$('Async Subworkflow').item.json.key}}" }`,
			}},
			{Name: "Value", Type: "n8n-nodes-base.set", Parameters: map[string]interface{}{
				"assignments": map[string]interface{}{"assignments": []interface{}{
					map[string]interface{}{"name": "value", "type": "string", "value": "={{ $json.respond }}"},
					map[string]interface{}{"name": "respond_type", "type": "string", "value": "={{ $json.respond_type }}"},
				}},
			}},
			{Name: "Check Key Exists1", Type: "n8n-nodes-base.if", Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "and",
					"conditions": []interface{}{map[string]interface{}{
						"leftValue":  "={{ $json.respond_type }}",
						"rightValue": "",
						"operator":   map[string]interface{}{"operation": "notEmpty", "type": "string", "singleValue": true},
					}},
				},
			}},
			{Name: "If", Type: "n8n-nodes-base.if", Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "and",
					"conditions": []interface{}{map[string]interface{}{
						"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
						"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
						"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
					}},
				},
			}},
			{Name: "Result", Type: "n8n-nodes-base.noOp"},
		},
		Connections: map[string]model.Connections{
			"Async Subworkflow": {Main: [][]model.Connection{{{Node: "Set Counter", Type: "main", Index: 0}}}},
			"Set Counter":       {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}}},
			"Find Respond":      {Main: [][]model.Connection{{{Node: "Value", Type: "main", Index: 0}}}},
			"Value":             {Main: [][]model.Connection{{{Node: "Check Key Exists1", Type: "main", Index: 0}}}},
			"Check Key Exists1": {Main: [][]model.Connection{{{Node: "Result", Type: "main", Index: 0}}, {{Node: "If", Type: "main", Index: 0}}}},
			"If":                {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}, {{Node: "Result", Type: "main", Index: 0}}}},
		},
	}

	input := []model.DataItem{{JSON: map[string]interface{}{"key": "exec_test_key", "data": map[string]interface{}{"x": 1}}}}
	ctx := WithSubWorkflowStart(context.Background(), 1)
	// Short-circuit poll sleep by capping attempts at 2 via Set Counter maxAttempts.
	res, err := eng.(ContextWorkflowEngine).ExecuteWorkflowWithContext(ctx, wf, input)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res != nil && res.Error != nil {
		t.Fatalf("result.Error: %v", res.Error)
	}
	if len(mongo.runKeys) == 0 {
		t.Fatal("Find Respond never ran")
	}
	for i, k := range mongo.runKeys {
		if k != "exec_test_key" {
			t.Fatalf("Find[%d] Async Subworkflow key: want exec_test_key, got %q (queries=%v)", i, k, mongo.queries)
		}
	}
}
