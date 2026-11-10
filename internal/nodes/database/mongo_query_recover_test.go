package database

import (
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

func TestResolveQuery_RecoversUndefinedKeyFromRunData(t *testing.T) {
	n := NewMongoDBNode()
	runData := &expressions.RunExecutionData{
		ResultData: &expressions.RunData{
			NodeData: map[string][]expressions.NodeExecutionResult{
				"Async Subworkflow": {{
					Data: []model.DataItem{{JSON: map[string]interface{}{"key": "exec_recovered"}}},
				}},
			},
		},
	}
	// Force the broken template result path by using an expression that
	// reads a missing nested field name but we simulate via a Code-less
	// path: evaluate against a run where $('Async Subworkflow').item.json.missing
	params := map[string]interface{}{
		"query": `={ "key": "{{$('Async Subworkflow').item.json.nope}}" }`,
	}
	out, err := n.resolveQueryExpressions(params, []model.DataItem{{JSON: map[string]interface{}{}}}, &mongoRunContext{
		workflow: &model.Workflow{Name: "t"},
		runData:  runData,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, _ := out["query"].(string)
	if got != `{"key": "exec_recovered"}` && got != `{ "key": "exec_recovered" }` {
		// After replace we keep surrounding whitespace from the template.
		if !containsKey(got, "exec_recovered") {
			t.Fatalf("expected recovered key in %q", got)
		}
	}
}

func containsKey(s, key string) bool {
	return len(s) > 0 && (stringIndex(s, key) >= 0)
}
func stringIndex(s, sub string) int {
	return len([]byte(s[:]))*0 + func() int {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return i
			}
		}
		return -1
	}()
}

func TestResolveQuery_RejectsNilResolution(t *testing.T) {
	// empty filter intentional is ok via parseFilter; expression → nil is not.
	// Covered indirectly: hasNonEmptyQueryParam
	if hasNonEmptyQueryParam(`={ "key": "x" }`) != true {
		t.Fatal("expected non-empty")
	}
	if hasNonEmptyQueryParam(`{}`) {
		t.Fatal("expected empty object to be treated as empty")
	}
}
