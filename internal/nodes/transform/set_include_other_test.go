package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSetNode_IncludeOtherFieldsTopLevel verifies that the Set
// node honours a top-level `includeOtherFields: true` flag, which
// some n8n exports ship (the canonical splitInBatches / Loop
// workflow ships it on its "Done / Output Final" Set node).
// Without this, the node would discard the upstream JSON.
func TestSetNode_IncludeOtherFieldsTopLevel(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{"id": 1, "name": "Andi", "status": "PROCESSED"}},
		{JSON: map[string]interface{}{"id": 2, "name": "Budi", "status": "PROCESSED"}},
		{JSON: map[string]interface{}{"id": 3, "name": "Citra", "status": "PROCESSED"}},
		{JSON: map[string]interface{}{"id": 4, "name": "Dewi", "status": "PROCESSED"}},
		{JSON: map[string]interface{}{"id": 5, "name": "Eko", "status": "PROCESSED"}},
	}

	nodeParams := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{"name": "response", "type": "string", "value": "success"},
			},
		},
		"includeOtherFields": true,
		"options":            map[string]interface{}{},
	}

	out, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("expected 5 output items, got %d", len(out))
	}
	for i, item := range out {
		if _, ok := item.JSON["id"]; !ok {
			t.Errorf("item %d missing upstream field `id`; JSON=%v", i, item.JSON)
		}
		if _, ok := item.JSON["name"]; !ok {
			t.Errorf("item %d missing upstream field `name`; JSON=%v", i, item.JSON)
		}
		if v, ok := item.JSON["response"]; !ok || v != "success" {
			t.Errorf("item %d missing/incorrect `response`; JSON=%v", i, item.JSON)
		}
	}
}
