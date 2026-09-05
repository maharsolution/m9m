package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSetNode_StringCoercesUndefinedToEmptyString pins the Set
// node's coercion behaviour for missing upstream fields. When the
// assignment has `type: "string"` and the expression evaluates to
// undefined, m9m coerces the value to `""` (empty string). This
// matches n8n's behaviour: the assignment still exists, downstream
// comparisons like `notEquals "1"` route the item to the "Failed"
// branch.
func TestSetNode_StringCoercesUndefinedToEmptyString(t *testing.T) {
	node := NewSetNode()

	input := []model.DataItem{
		{JSON: map[string]interface{}{
			"body": map[string]interface{}{"var": "a"},
		}},
	}

	params := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"id":    "test-assignment-1",
					"name":  "inputan",
					"type":  "string",
					"value": "={{ $json.body.variable }}",
				},
			},
		},
		"options": map[string]interface{}{},
	}

	out, err := node.Execute(input, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out))
	}
	got, exists := out[0].JSON["inputan"]
	if !exists {
		t.Fatalf("expected inputan key to be present, got map=%v", out[0].JSON)
	}
	// Empty string is fine — it's still not equal to "1", so the
	// downstream Switch's `notEquals "1"` rule correctly fires.
	if s, ok := got.(string); !ok || s != "" {
		t.Fatalf("expected inputan to be empty string, got %T (%v)", got, got)
	}
}

