package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

// TestSetNode_ExecutionIdFromRunData verifies Finnet Payload's
// `key={{ $execution.id }}` resolves to the live execution id rather
// than the hardcoded "set-processing" stub (which made every
// post_balance request share one mongo document and return stale
// balances).
func TestSetNode_ExecutionIdFromRunData(t *testing.T) {
	node := NewSetNode()
	runData := &expressions.RunExecutionData{
		ExecutionID: "exec_finnet_unique_42",
	}
	params := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"name":  "key",
					"type":  "string",
					"value": "={{ $execution.id }}",
				},
			},
		},
	}
	out, err := node.ExecuteWithRun(
		&model.Workflow{Name: "Finnet"},
		runData,
		0,
		0,
		[]model.DataItem{{JSON: map[string]interface{}{"body": map[string]interface{}{"amount": 140000}}}},
		params,
	)
	if err != nil {
		t.Fatalf("ExecuteWithRun: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	got, _ := out[0].JSON["key"].(string)
	if got != "exec_finnet_unique_42" {
		t.Fatalf("key: want exec_finnet_unique_42, got %q", got)
	}
}
