package database

import (
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

func TestResolveQuery_KeyPresent(t *testing.T) {
	n := NewMongoDBNode()
	runData := &expressions.RunExecutionData{
		ResultData: &expressions.RunData{
			NodeData: map[string][]expressions.NodeExecutionResult{
				"Async Subworkflow": {{
					Data: []model.DataItem{{JSON: map[string]interface{}{"key": "exec_live"}}},
				}},
			},
		},
		ExecutionData: &expressions.ExecutionData{
			ContextData: map[string]interface{}{"subworkflowKey": "exec_live"},
		},
	}
	params := map[string]interface{}{
		"query": `={ "key": "{{$('Async Subworkflow').item.json.key}}" }`,
	}
	out, err := n.resolveQueryExpressions(params, []model.DataItem{{JSON: map[string]interface{}{}}}, &mongoRunContext{
		workflow: &model.Workflow{Name: "t"},
		runData:  runData,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("resolved query=%q type=%T", out["query"], out["query"])
}
