package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// Finnet post_balance ships Merge v3 with only `mode: chooseBranch` —
// no branchCondition. n8n accepts that; m9m must too.
func TestMerge_ChooseBranch_NoBranchCondition(t *testing.T) {
	node := NewMergeNode()
	params := map[string]interface{}{"mode": "chooseBranch"}
	if err := node.ValidateParameters(params); err != nil {
		t.Fatalf("ValidateParameters should accept chooseBranch without branchCondition: %v", err)
	}

	in := []model.DataItem{{JSON: map[string]interface{}{"key": "k1", "data": map[string]interface{}{"id": "U001"}}}}
	out, err := node.Execute(in, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected passthrough of 1 item, got %d", len(out))
	}
	if out[0].JSON["key"] != "k1" {
		t.Fatalf("unexpected output: %+v", out[0].JSON)
	}
}
