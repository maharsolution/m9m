package transform

import (
	"testing"
	
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

func TestSetNodeCreation(t *testing.T) {
	node := NewSetNode()
	if node == nil {
		t.Fatal("Expected node to be created, got nil")
	}
	
	desc := node.Description()
	if desc.Name != "Set" {
		t.Errorf("Expected name 'Set', got '%s'", desc.Name)
	}
}

func TestSetNodeValidateParameters(t *testing.T) {
	node := NewSetNode()
	
	// Test with nil params
	err := node.ValidateParameters(nil)
	if err == nil {
		t.Error("Expected error with nil params, got nil")
	}
	
	// Test with missing assignments
	params := map[string]interface{}{}
	err = node.ValidateParameters(params)
	if err == nil {
		t.Error("Expected error with missing assignments, got nil")
	}
	
	// Test with invalid assignments type
	invalidParams := map[string]interface{}{
		"assignments": "not an array",
	}
	err = node.ValidateParameters(invalidParams)
	if err == nil {
		t.Error("Expected error with invalid assignments type, got nil")
	}
	
	// Test with empty assignments array
	emptyParams := map[string]interface{}{
		"assignments": []interface{}{},
	}
	err = node.ValidateParameters(emptyParams)
	if err != nil {
		t.Errorf("Expected no error with empty assignments, got %v", err)
	}
	
	// Test with invalid assignment object
	invalidAssignmentParams := map[string]interface{}{
		"assignments": []interface{}{
			"not an object",
		},
	}
	err = node.ValidateParameters(invalidAssignmentParams)
	if err == nil {
		t.Error("Expected error with invalid assignment object, got nil")
	}
	
	// Test with assignment missing name
	missingNameParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"value": "test",
			},
		},
	}
	err = node.ValidateParameters(missingNameParams)
	if err == nil {
		t.Error("Expected error with missing name, got nil")
	}
	
	// Test with assignment missing value
	missingValueParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name": "field1",
			},
		},
	}
	err = node.ValidateParameters(missingValueParams)
	if err == nil {
		t.Error("Expected error with missing value, got nil")
	}
	
	// Test with valid assignments
	validParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "field1",
				"value": "value1",
			},
			map[string]interface{}{
				"name":  "field2",
				"value": 42,
			},
		},
	}
	err = node.ValidateParameters(validParams)
	if err != nil {
		t.Errorf("Expected no error with valid params, got %v", err)
	}
}

func TestSetNodeExecuteWithEmptyInput(t *testing.T) {
	node := NewSetNode()
	
	inputData := []model.DataItem{}
	nodeParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "field1",
				"value": "value1",
			},
		},
	}
	
	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

func TestSetNodeExecuteWithValidAssignments(t *testing.T) {
	node := NewSetNode()
	
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"existing": "value",
			},
		},
	}
	
	nodeParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "newField",
				"value": "newValue",
			},
			map[string]interface{}{
				"name":  "numberField",
				"value": 123,
			},
		},
	}
	
	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}
	
	item := result[0]
	
	// Check that existing field is preserved
	if existing, ok := item.JSON["existing"].(string); !ok || existing != "value" {
		t.Errorf("Expected existing field to be preserved, got %v", item.JSON["existing"])
	}
	
	// Check that new fields are added
	if newValue, ok := item.JSON["newField"].(string); !ok || newValue != "newValue" {
		t.Errorf("Expected newField to be 'newValue', got %v", item.JSON["newField"])
	}
	
	if numberValue, ok := item.JSON["numberField"].(int); !ok || numberValue != 123 {
		t.Errorf("Expected numberField to be 123, got %v", item.JSON["numberField"])
	}
}

func TestSetNodeExecuteWithMultipleItems(t *testing.T) {
	node := NewSetNode()
	
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"id": 1,
			},
		},
		{
			JSON: map[string]interface{}{
				"id": 2,
			},
		},
	}
	
	nodeParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "processed",
				"value": true,
			},
		},
	}
	
	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(result) != 2 {
		t.Fatalf("Expected 2 result items, got %d", len(result))
	}
	
	// Check first item
	if id, ok := result[0].JSON["id"].(int); !ok || id != 1 {
		t.Errorf("Expected first item id to be 1, got %v", result[0].JSON["id"])
	}
	
	if processed, ok := result[0].JSON["processed"].(bool); !ok || !processed {
		t.Errorf("Expected first item processed to be true, got %v", result[0].JSON["processed"])
	}
	
	// Check second item
	if id, ok := result[1].JSON["id"].(int); !ok || id != 2 {
		t.Errorf("Expected second item id to be 2, got %v", result[1].JSON["id"])
	}
	
	if processed, ok := result[1].JSON["processed"].(bool); !ok || !processed {
		t.Errorf("Expected second item processed to be true, got %v", result[1].JSON["processed"])
	}
}

func TestSetNodeImplementsNodeExecutor(t *testing.T) {
	var _ base.NodeExecutor = (*SetNode)(nil)
}

// TestSetNodeValidateNestedAssignments covers the newer n8n typeVersion>=3
// parameter shape where `assignments` is wrapped under another `assignments`
// key (i.e. parameters.assignments.assignments[]). The Set node must accept
// this in addition to the legacy flat shape.
func TestSetNodeValidateNestedAssignments(t *testing.T) {
	node := NewSetNode()

	// Nested shape: parameters.assignments = { assignments: [...] }
	nestedParams := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"id":    "abc-123",
					"name":  "variableA",
					"type":  "number",
					"value": "={{ $json.body.varA }}",
				},
				map[string]interface{}{
					"id":    "def-456",
					"name":  "variableB",
					"type":  "number",
					"value": "={{ $json.body.varB }}",
				},
			},
		},
	}
	if err := node.ValidateParameters(nestedParams); err != nil {
		t.Errorf("Expected no error with nested assignments, got %v", err)
	}

	// Nested shape with empty inner array is valid (no-op at runtime).
	emptyNested := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{},
		},
	}
	if err := node.ValidateParameters(emptyNested); err != nil {
		t.Errorf("Expected no error with empty nested assignments, got %v", err)
	}

	// Nested shape where the inner `assignments` key is missing/wrong type
	// should be reported as an error.
	badNested := map[string]interface{}{
		"assignments": map[string]interface{}{
			"not_assignments": []interface{}{},
		},
	}
	if err := node.ValidateParameters(badNested); err == nil {
		t.Error("Expected error for nested object without 'assignments' key")
	}
}

// TestSetNodeExecuteNestedAssignments verifies the Execute path also
// unwraps the nested shape correctly so workflows imported from newer
// n8n (typeVersion 3+) run end-to-end.
func TestSetNodeExecuteNestedAssignments(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{"existing": "value"}},
	}

	nestedParams := map[string]interface{}{
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"name":  "added",
					"value": "hello",
				},
			},
		},
	}

	result, err := node.Execute(inputData, nestedParams)
	if err != nil {
		t.Fatalf("Unexpected error with nested assignments: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}
	if result[0].JSON["existing"] != "value" {
		t.Errorf("Expected existing field preserved, got %v", result[0].JSON["existing"])
	}
	if result[0].JSON["added"] != "hello" {
		t.Errorf("Expected added='hello', got %v", result[0].JSON["added"])
	}
}

// TestSetNodeExecuteN8nExpressionNoDoubleWrap guards against a regression
// where the Set node would wrap an already-`{{ }}`-bracketed expression a
// second time, producing "SyntaxError: Unexpected token {" at runtime.
//
// Both input shapes produced by the n8n UI must be accepted:
//
//	={{ $json.body.x }}     (n8n expression mode, leading =)
//	{{ $json.body.x }}      (already wrapped, template form)
func TestSetNodeExecuteN8nExpressionNoDoubleWrap(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"body": map[string]interface{}{"x": 42},
			},
		},
	}

	cases := []struct {
		name  string
		value string
	}{
		{"leading equals", "={{ $json.body.x }}"},
		{"already wrapped", "{{ $json.body.x }}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"assignments": []interface{}{
					map[string]interface{}{
						"name":  "result",
						"value": tc.value,
					},
				},
			}
			out, err := node.Execute(inputData, params)
			if err != nil {
				t.Fatalf("Execute failed for %q: %v", tc.value, err)
			}
			if len(out) != 1 {
				t.Fatalf("Expected 1 result, got %d", len(out))
			}
			if got, ok := out[0].JSON["result"]; !ok {
				t.Errorf("Expected 'result' field, got %#v", out[0].JSON)
			} else if v, isNum := got.(float64); !isNum || v != 42 {
				t.Errorf("Expected result=42, got %#v", got)
			}
		})
	}
}