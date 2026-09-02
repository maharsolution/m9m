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

	// n8n's default for typeVersion 3+ is "Keep Only Set" (replace), so
	// the legacy flat-assignments test that relied on merge behaviour
	// has to opt into merge explicitly here.
	nodeParams := map[string]interface{}{
		"options": map[string]interface{}{"keepOnlySet": false},
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

	// Check that existing field is preserved (with explicit merge opt-in).
	if existing, ok := item.JSON["existing"].(string); !ok || existing != "value" {
		t.Errorf("Expected existing field to be preserved (merge mode), got %v", item.JSON["existing"])
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

	// Opt into the legacy merge behaviour: the new n8n-default replace
	// mode would drop the upstream `id` field, which this test asserts
	// is preserved.
	nodeParams := map[string]interface{}{
		"options": map[string]interface{}{"keepOnlySet": false},
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
	// is now treated as a pass-through so workflows exported by n8n with
	// empty options maps still execute.
	badNested := map[string]interface{}{
		"assignments": map[string]interface{}{
			"not_assignments": []interface{}{},
		},
	}
	if err := node.ValidateParameters(badNested); err != nil {
		t.Errorf("Expected no error (pass-through) for nested object without 'assignments' key, got %v", err)
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

	// Opt into merge mode explicitly: by default, n8n typeVersion 3+
	// replaces the upstream JSON, which would drop `existing` here.
	nestedParams := map[string]interface{}{
		"options": map[string]interface{}{"keepOnlySet": false},
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
		t.Errorf("Expected existing field preserved (merge opt-in), got %v", result[0].JSON["existing"])
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
			} else {
				switch v := got.(type) {
				case float64:
					if v != 42 {
						t.Errorf("Expected result=42, got %v", v)
					}
				case int:
					if v != 42 {
						t.Errorf("Expected result=42, got %v", v)
					}
				case int64:
					if v != 42 {
						t.Errorf("Expected result=42, got %v", v)
					}
				case int32:
					if v != 42 {
						t.Errorf("Expected result=42, got %v", v)
					}
				default:
					t.Errorf("Expected result=42 (numeric), got %#v (type %T)", got, got)
				}
			}
		})
	}
}

// TestSetNodeValidateEmptyOptions covers the n8n export case where the
// Set node's parameters contain only an `options` map (no `assignments`
// key at all). This happens when n8n exports a workflow whose Set nodes
// have no fields configured. The Set node should treat this as a valid
// pass-through rather than failing validation.
func TestSetNodeValidateEmptyOptions(t *testing.T) {
	node := NewSetNode()

	params := map[string]interface{}{
		"options": map[string]interface{}{},
	}
	if err := node.ValidateParameters(params); err != nil {
		t.Errorf("Expected nil error for empty options, got %v", err)
	}
}

// TestSetNodeExecuteEmptyOptions verifies the Execute path also tolerates
// the empty-options case: it runs as a no-op but, because n8n's Set
// default is now "Keep Only Set" (replace), the empty `options:{}`
// does NOT propagate the upstream JSON — the resulting item has no
// keys. The Set node still must not error out.
func TestSetNodeExecuteEmptyOptions(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{"hello": "world"}},
	}
	params := map[string]interface{}{
		"options": map[string]interface{}{},
	}
	out, err := node.Execute(inputData, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(out))
	}
	// Replace mode + zero assignments = empty JSON. This is the
	// production bocahtuanakal case: an `options:{}` Set with no fields
	// is a pass-through in the *error* sense (no crash) but it does
	// drop the upstream data. The legacy merge behaviour is opt-in
	// via `keepOnlySet:false`.
	if len(out[0].JSON) != 0 {
		t.Errorf("Expected empty JSON in replace mode, got %#v", out[0].JSON)
	}
}

// TestSetNodeExecuteTypeCoercion_NumberFromString verifies that a Set
// assignment with `type: "number"` coerces a string value (the literal
// "5") to a float64(5) before writing it onto the item. This is the
// single most common fix-up needed when migrating workflows from
// n8n's older flat Set format, where the assignment's `type` field
// was ignored at runtime.
func TestSetNodeExecuteTypeCoercion_NumberFromString(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{}},
	}
	params := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "count",
				"type":  "number",
				"value": "5",
			},
		},
	}

	out, err := node.Execute(inputData, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(out))
	}
	got, ok := out[0].JSON["count"].(float64)
	if !ok {
		t.Fatalf("Expected count to be float64, got %T (%#v)", out[0].JSON["count"], out[0].JSON["count"])
	}
	if got != 5 {
		t.Errorf("Expected count=5, got %v", got)
	}
}

// TestSetNodeExecuteTypeCoercion_SumTwoStringsAsNumbers is the regression
// test for the production bug:
//
//	Webhook body: {"varA":"5","varB":"10"}
//	Node "Edit Fields"  sets variableA = $json.body.varA  (type: number)
//	Node "Edit Fields"  sets variableB = $json.body.varB  (type: number)
//	Node "Edit Fields1" sets total     = $json.variableA + $json.variableb
//	                                                       (type: number)
//
// Before the fix, the first node left `varA`/`varB` as the strings
// "5"/"10", so the second node's expression `+` concatenated them and
// `total` became the string "510". After the fix, the first node
// promotes each value to float64, the second node's `+` performs real
// arithmetic on the numeric operands, and the final `type: "number"`
// coercion writes `total` as float64(15).
//
// The test runs the workflow exactly as the server does: the output
// of the first Set node feeds straight into the second.
func TestSetNodeExecuteTypeCoercion_SumTwoStringsAsNumbers(t *testing.T) {
	node := NewSetNode()

	// Simulated webhook payload: vars arrive as strings (HTTP body is
	// always text on the wire), exactly as the production case.
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"body": map[string]interface{}{
					"varA": "5",
					"varB": "10",
				},
			},
		},
	}

	// First Set node: promote varA/varB to numbers on the item.
	firstParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "variableA",
				"type":  "number",
				"value": "={{ $json.body.varA }}",
			},
			map[string]interface{}{
				"name":  "variableb",
				"type":  "number",
				"value": "={{ $json.body.varB }}",
			},
		},
	}
	afterFirst, err := node.Execute(inputData, firstParams)
	if err != nil {
		t.Fatalf("First Set node failed: %v", err)
	}
	if len(afterFirst) != 1 {
		t.Fatalf("Expected 1 result after first Set, got %d", len(afterFirst))
	}
	if v, ok := afterFirst[0].JSON["variableA"].(float64); !ok || v != 5 {
		t.Fatalf("Expected variableA=5 (float64) after first Set, got %T (%#v)", afterFirst[0].JSON["variableA"], afterFirst[0].JSON["variableA"])
	}
	if v, ok := afterFirst[0].JSON["variableb"].(float64); !ok || v != 10 {
		t.Fatalf("Expected variableb=10 (float64) after first Set, got %T (%#v)", afterFirst[0].JSON["variableb"], afterFirst[0].JSON["variableb"])
	}

	// Second Set node: sum the now-numeric fields.
	secondParams := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "total",
				"type":  "number",
				"value": "={{ $json.variableA + $json.variableb }}",
			},
		},
	}
	afterSecond, err := node.Execute(afterFirst, secondParams)
	if err != nil {
		t.Fatalf("Second Set node failed: %v", err)
	}
	if len(afterSecond) != 1 {
		t.Fatalf("Expected 1 result after second Set, got %d", len(afterSecond))
	}
	got, ok := afterSecond[0].JSON["total"].(float64)
	if !ok {
		t.Fatalf("Expected total to be float64, got %T (%#v)", afterSecond[0].JSON["total"], afterSecond[0].JSON["total"])
	}
	if got != 15 {
		t.Errorf("Expected total=15, got %v (this is the production bug regression)", got)
	}
}

// TestSetNodeExecuteTypeCoercion_Boolean checks the boolean coercion
// branch. The expression `"true"` evaluates to the Go string "true";
// with `type: "boolean"` declared, that must be parsed into a real
// bool. Also exercises the case-insensitive parse path.
func TestSetNodeExecuteTypeCoercion_Boolean(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{}},
	}

	cases := []struct {
		name     string
		value    string
		expected bool
	}{
		{"lowercase true", "true", true},
		{"uppercase TRUE", "TRUE", true},
		{"numeric 1", "1", true},
		{"false string", "false", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"assignments": []interface{}{
					map[string]interface{}{
						"name":  "flag",
						"type":  "boolean",
						"value": tc.value,
					},
				},
			}
			out, err := node.Execute(inputData, params)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("Expected 1 result, got %d", len(out))
			}
			got, ok := out[0].JSON["flag"].(bool)
			if !ok {
				t.Fatalf("Expected flag to be bool, got %T (%#v)", out[0].JSON["flag"], out[0].JSON["flag"])
			}
			if got != tc.expected {
				t.Errorf("Expected flag=%v, got %v", tc.expected, got)
			}
		})
	}
}

// TestSetNodeExecuteNoTypePreservesLegacyBehaviour guards against the
// fix accidentally coercing values in workflows that don't declare a
// `type` field on their assignments (the legacy n8n flat shape used
// by examples/api-integration/webhook-processing.json).
//
// In particular, the value produced by `{{ $json.body.user_id || 'unknown' }}`
// must remain a string when the assignment has no `type` declared —
// never silently coerced to a number, bool, or anything else.
func TestSetNodeExecuteNoTypePreservesLegacyBehaviour(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"body": map[string]interface{}{
					"user_id": "u-42",
				},
			},
		},
	}
	params := map[string]interface{}{
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "userId",
				"value": "{{ $json.body.user_id || 'unknown' }}",
				// intentionally NO `type` field
			},
		},
	}

	out, err := node.Execute(inputData, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(out))
	}
	got, ok := out[0].JSON["userId"].(string)
	if !ok {
		t.Fatalf("Expected userId to remain a string (no `type` field set), got %T (%#v)", out[0].JSON["userId"], out[0].JSON["userId"])
	}
	if got != "u-42" {
		t.Errorf("Expected userId='u-42', got %q", got)
	}
}

// TestSetNodeExecuteReplaceModeByDefault reproduces the production
// workflow `Webhook → Set (variableA, variableb) → Set (total = a+b)`
// end-to-end and asserts the second Set's output is just `{"total": …}`
// — i.e. upstream webhook context is REPLACED, not merged.
//
// This is the byte-equivalent regression test for the live n8n
// bocahtuanakal webhook at `http://…/webhook/bocahtuanakal`, whose
// response body is `[{"total":11115}]` (17 bytes) on n8n and was
// `[{"body":…,"headers":…,"total":11115,…}]` (265 bytes) on the
// pre-fix m9m. The fix: n8n's Set node defaults to `keepOnlySet=true`
// for typeVersion 3+, so m9m's Set must replace the upstream JSON by
// default when the workflow declares an empty `options: {}` map.
func TestSetNodeExecuteReplaceModeByDefault(t *testing.T) {
	node := NewSetNode()

	// Simulated webhook payload — vars arrive as strings, exactly as
	// the production case (HTTP body is always text on the wire).
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"headers": map[string][]string{
					"Content-Type": {"application/json"},
					"User-Agent":   {"curl/8.21.0"},
				},
				"params": map[string][]string{},
				"query":  map[string][]string{},
				"body": map[string]interface{}{
					"varA": "5",
					"varB": "11110",
				},
				"method": "POST",
				"path":   "bocahtuanakal",
			},
		},
	}

	// First Set node: typeVersion 3.5 nested shape, `options: {}` —
	// n8n defaults to replace.
	firstParams := map[string]interface{}{
		"options": map[string]interface{}{},
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"name":  "variableA",
					"type":  "number",
					"value": "={{ $json.body.varA }}",
				},
				map[string]interface{}{
					"name":  "variableb",
					"type":  "number",
					"value": "={{ $json.body.varB }}",
				},
			},
		},
	}
	afterFirst, err := node.Execute(inputData, firstParams)
	if err != nil {
		t.Fatalf("First Set node failed: %v", err)
	}
	if len(afterFirst) != 1 {
		t.Fatalf("Expected 1 result after first Set, got %d", len(afterFirst))
	}

	// The first Set's output should be ONLY the two assigned keys —
	// the upstream `body`, `headers`, `params`, `query`, `method`,
	// `path` must all be gone. That's the whole point of the fix.
	first := afterFirst[0].JSON
	if v, ok := first["variableA"].(float64); !ok || v != 5 {
		t.Errorf("Expected variableA=5 (float64), got %T (%#v)", first["variableA"], first["variableA"])
	}
	if v, ok := first["variableb"].(float64); !ok || v != 11110 {
		t.Errorf("Expected variableb=11110 (float64), got %T (%#v)", first["variableb"], first["variableb"])
	}
	for _, leaked := range []string{"body", "headers", "params", "query", "method", "path"} {
		if _, ok := first[leaked]; ok {
			t.Errorf("Replace mode must drop upstream field %q, but found it: %#v", leaked, first[leaked])
		}
	}

	// Second Set node: sum the now-numeric fields, also `options: {}`.
	secondParams := map[string]interface{}{
		"options": map[string]interface{}{},
		"assignments": map[string]interface{}{
			"assignments": []interface{}{
				map[string]interface{}{
					"name":  "total",
					"type":  "number",
					"value": "={{ $json.variableA + $json.variableb }}",
				},
			},
		},
	}
	afterSecond, err := node.Execute(afterFirst, secondParams)
	if err != nil {
		t.Fatalf("Second Set node failed: %v", err)
	}
	if len(afterSecond) != 1 {
		t.Fatalf("Expected 1 result after second Set, got %d", len(afterSecond))
	}

	// Final output must be exactly `{"total":11115}` — same shape
	// n8n returns on the wire. Anything else is a regression.
	final := afterSecond[0].JSON
	if len(final) != 1 {
		t.Fatalf("Expected exactly 1 key after second Set (replace mode), got %d: %#v", len(final), final)
	}
	got, ok := final["total"].(float64)
	if !ok {
		t.Fatalf("Expected total to be float64, got %T (%#v)", final["total"], final["total"])
	}
	if got != 11115 {
		t.Errorf("Expected total=11115, got %v (this is the production bocahtuanakal regression)", got)
	}
}

// TestSetNodeExecuteKeepOnlySetFalseHonoursMerge verifies that an n8n
// workflow explicitly opting into `keepOnlySet: false` keeps the
// pre-fix merge behaviour. Without this opt-in, legacy workflows that
// relied on the merge would silently lose fields.
func TestSetNodeExecuteKeepOnlySetFalseHonoursMerge(t *testing.T) {
	node := NewSetNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"existing": "value",
				"body":     map[string]interface{}{"x": 1},
			},
		},
	}
	params := map[string]interface{}{
		"options": map[string]interface{}{"keepOnlySet": false},
		"assignments": []interface{}{
			map[string]interface{}{
				"name":  "added",
				"value": "hello",
			},
		},
	}
	out, err := node.Execute(inputData, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(out))
	}
	item := out[0].JSON
	if item["existing"] != "value" {
		t.Errorf("Expected existing preserved with keepOnlySet=false, got %#v", item["existing"])
	}
	if item["added"] != "hello" {
		t.Errorf("Expected added='hello', got %#v", item["added"])
	}
	if _, ok := item["body"]; !ok {
		t.Errorf("Expected body preserved with keepOnlySet=false, got %#v", item)
	}
}

// TestShouldReplaceJSON covers the decision matrix for the helper that
// picks replace-vs-merge mode for the Set node.
func TestShouldReplaceJSON(t *testing.T) {
	cases := []struct {
		name     string
		params   map[string]interface{}
		expected bool
	}{
		{"nil params → replace", nil, true},
		{"no options key → replace", map[string]interface{}{}, true},
		{"empty options → replace", map[string]interface{}{"options": map[string]interface{}{}}, true},
		{"keepOnlySet=true → replace", map[string]interface{}{"options": map[string]interface{}{"keepOnlySet": true}}, true},
		{"keepOnlySet=false → merge", map[string]interface{}{"options": map[string]interface{}{"keepOnlySet": false}}, false},
		{"malformed options → replace (safer default)", map[string]interface{}{"options": "nope"}, true},
		{"unrelated options keys → replace", map[string]interface{}{"options": map[string]interface{}{"dotValues": false}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldReplaceJSON(tc.params); got != tc.expected {
				t.Errorf("shouldReplaceJSON(%#v) = %v, want %v", tc.params, got, tc.expected)
			}
		})
	}
}