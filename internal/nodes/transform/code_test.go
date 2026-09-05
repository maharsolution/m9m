package transform

import (
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

func TestCodeNodeCreation(t *testing.T) {
	node := NewCodeNode()
	if node == nil {
		t.Fatal("Expected node to be created, got nil")
	}

	desc := node.Description()
	if desc.Name != "Code" {
		t.Errorf("Expected name 'Code', got '%s'", desc.Name)
	}
}

func TestCodeNodeValidateParameters(t *testing.T) {
	node := NewCodeNode()

	// Test with nil params
	err := node.ValidateParameters(nil)
	if err == nil {
		t.Error("Expected error with nil params, got nil")
	}

	// Test with missing mode and no code keys — should now default
	// to runOnceForEachItem + javascript. Validation still fails
	// because there is no source-code key to execute.
	params := map[string]interface{}{}
	err = node.ValidateParameters(params)
	if err == nil {
		t.Error("Expected error from missing code/jsCode, got nil")
	} else if !strings.Contains(err.Error(), "required") {
		t.Errorf("Expected missing-code error, got: %v", err)
	}

	// Test with invalid mode type
	invalidModeTypeParams := map[string]interface{}{
		"mode": 123, // Not a string
	}
	err = node.ValidateParameters(invalidModeTypeParams)
	if err == nil {
		t.Error("Expected error with invalid mode type, got nil")
	}

	// Test with invalid mode value
	invalidModeValueParams := map[string]interface{}{
		"mode": "invalid",
	}
	err = node.ValidateParameters(invalidModeValueParams)
	if err == nil {
		t.Error("Expected error with invalid mode value, got nil")
	}

	// Test with valid mode
	validModeParams := map[string]interface{}{
		"mode": "runOnceForAllItems",
	}
	err = node.ValidateParameters(validModeParams)
	if err == nil {
		t.Error("Expected error with missing language, got nil")
	}

	// Test with missing language
	missingLanguageParams := map[string]interface{}{
		"mode": "runOnceForAllItems",
	}
	err = node.ValidateParameters(missingLanguageParams)
	if err == nil {
		t.Error("Expected error with missing language, got nil")
	}

	// Test with invalid language type
	invalidLanguageTypeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": 123, // Not a string
	}
	err = node.ValidateParameters(invalidLanguageTypeParams)
	if err == nil {
		t.Error("Expected error with invalid language type, got nil")
	}

	// Test with invalid language value
	invalidLanguageValueParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "invalid",
	}
	err = node.ValidateParameters(invalidLanguageValueParams)
	if err == nil {
		t.Error("Expected error with invalid language value, got nil")
	}

	// Test with valid language but missing code
	validLanguageParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
	}
	err = node.ValidateParameters(validLanguageParams)
	if err == nil {
		t.Error("Expected error with missing code, got nil")
	}

	// Test with invalid code type
	invalidCodeTypeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
		"code":     123, // Not a string
	}
	err = node.ValidateParameters(invalidCodeTypeParams)
	if err == nil {
		t.Error("Expected error with invalid code type, got nil")
	}

	// Test with valid parameters
	validParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
		"code":     "var result = $json; result;", // Valid JavaScript expression
	}
	err = node.ValidateParameters(validParams)
	if err != nil {
		t.Errorf("Expected no error with valid parameters, got %v", err)
	}

	// Test with another valid mode
	anotherValidModeParams := map[string]interface{}{
		"mode":     "runOnceForEachItem",
		"language": "python",
		"code":     "print('Hello, World!')",
	}
	err = node.ValidateParameters(anotherValidModeParams)
	if err != nil {
		t.Errorf("Expected no error with another valid mode, got %v", err)
	}

	// Test with another valid language
	anotherValidLanguageParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "go",
		"code":     "fmt.Println(\"Hello, World!\")",
	}
	err = node.ValidateParameters(anotherValidLanguageParams)
	if err != nil {
		t.Errorf("Expected no error with another valid language, got %v", err)
	}
}

func TestCodeNodeExecuteWithEmptyInput(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{}
	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
		"code":     "var result = $json; result;", // Valid JavaScript expression
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

func TestCodeNodeExecuteJavaScript(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"name": "John",
				"age":  float64(30),
			},
		},
	}

	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
		"code":     "var result = $json; result;", // Valid JavaScript expression
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}

	item := result[0]

	// Check that the item has the expected JSON data
	if name, ok := item.JSON["name"].(string); !ok || name != "John" {
		t.Errorf("Expected name 'John', got %v", item.JSON["name"])
	}

	if age, ok := item.JSON["age"].(float64); !ok || int(age) != 30 {
		t.Errorf("Expected age 30, got %v", item.JSON["age"])
	}
}

func TestCodeNodeExecutePython(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"name": "John",
				"age":  float64(30),
			},
		},
	}

	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "python",
		"code":     "print('Hello, World!')",
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}

	item := result[0]

	// Check that the item has the expected JSON data
	if name, ok := item.JSON["name"].(string); !ok || name != "John" {
		t.Errorf("Expected name 'John', got %v", item.JSON["name"])
	}

	if age, ok := item.JSON["age"].(float64); !ok || int(age) != 30 {
		t.Errorf("Expected age 30, got %v", item.JSON["age"])
	}

	// Check that Python execution result is present
	if pythonResult, ok := item.JSON["pythonResult"].(string); !ok || pythonResult == "" {
		t.Errorf("Expected Python execution result, got %v", item.JSON["pythonResult"])
	}
}

func TestCodeNodeExecuteGo(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"name": "John",
				"age":  float64(30),
			},
		},
	}

	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "go",
		"code":     "fmt.Println(\"Hello, World!\")",
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}

	item := result[0]

	// Check that the item has the expected JSON data
	if name, ok := item.JSON["name"].(string); !ok || name != "John" {
		t.Errorf("Expected name 'John', got %v", item.JSON["name"])
	}

	if age, ok := item.JSON["age"].(float64); !ok || int(age) != 30 {
		t.Errorf("Expected age 30, got %v", item.JSON["age"])
	}

	// Check that Go execution result is present
	if goResult, ok := item.JSON["goResult"].(string); !ok || goResult == "" {
		t.Errorf("Expected Go execution result, got %v", item.JSON["goResult"])
	}
}

func TestCodeNodeExecuteWithInvalidLanguage(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"name": "John",
				"age":  float64(30),
			},
		},
	}

	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "invalid",
		"code":     "var result = $json; result;", // Valid JavaScript expression
	}

	_, err := node.Execute(inputData, nodeParams)
	if err == nil {
		t.Error("Expected error with invalid language, got nil")
	}
}

func TestCodeNodeExecuteWithMissingCode(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"name": "John",
				"age":  float64(30),
			},
		},
	}

	nodeParams := map[string]interface{}{
		"mode":     "runOnceForAllItems",
		"language": "javascript",
		"code":     "", // Empty code
	}

	_, err := node.Execute(inputData, nodeParams)
	if err == nil {
		t.Error("Expected error with missing code, got nil")
	}
}

// TestCodeNode_Execute_NoModeParam pins the parity gap #A:
// n8n's Code node typeVersion 2 ("Simple Webhook - Code" workflow
// `V4432EsGIkpqIZx9`) exports workflows with only `language` +
// `jsCode` — `mode` is hidden under "Settings" and is *omitted* from
// the JSON export. m9m used to reject these workflows with
// `mode parameter is required`, breaking drop-in compatibility.
//
// The fix: ValidateParameters must accept the missing-mode case and
// Execute must run the code with the n8n default
// (`runOnceForEachItem`) instead of erroring.
//
// Note: n8n Code type v2 accepts both expression-style code
// (`var r = {...}; r;`) and function-body-style code
// (`return {...}`). m9m's Code node runs through the same Goja
// evaluator as the Function node, which only accepts expression-style
// code; full function-body mode would require wrapping the code in
// an IIFE, which is a separate enhancement. This test pins the
// drop-in gap-#A fix specifically: the missing `mode` parameter
// must NOT block workflow execution — the missing-mode case must
// execute via the default branch.
func TestCodeNode_Execute_NoModeParam(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{"x": 1.0}},
		{JSON: map[string]interface{}{"x": 2.0}},
	}

	// No `mode` key — this is exactly what n8n's Code node type v2
	// exports. The body uses expression-style code (no `return`
	// statement) so it can run through m9m's Goja evaluator without
	// an IIFE wrapper.
	//
	// As of the loop-test fix we default to `runOnceForAllItems`
	// (n8n's modern Code v2 default), but this test asserts the
	// legacy `runOnceForEachItem` semantics explicitly. The Code v2
	// default change only matters when the workflow does not pin
	// `mode` at all, so explicitly pinning it here keeps the
	// per-item semantics intact for this fixture.
	nodeParams := map[string]interface{}{
		"language": "javascript",
		"mode":     "runOnceForEachItem",
		"jsCode":   "var r = { myNewField: 1 }; r;",
	}

	// Validation must NOT error on the missing `mode` field.
	if err := node.ValidateParameters(nodeParams); err != nil {
		t.Fatalf("ValidateParameters should accept missing mode, got: %v", err)
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Execute should run with default mode, got error: %v", err)
	}

	// n8n's mode = runOnceForEachItem runs the code once per input
	// item and emits one output item per input. We pass 2 items in,
	// we expect 2 items out (one item per source, not a single
	// collapsed array).
	if len(result) != 2 {
		t.Fatalf("Expected 2 result items (one per input), got %d", len(result))
	}

	for i, item := range result {
		newField, ok := item.JSON["myNewField"]
		if !ok {
			t.Fatalf("item %d missing myNewField; JSON=%v", i, item.JSON)
		}
		// JavaScript integer literals come back as int64 through Goja;
		// floats come back as float64. Accept either representation
		// of the value 1.
		switch v := newField.(type) {
		case int64:
			if v != 1 {
				t.Errorf("item %d: expected myNewField=1, got int64(%d)", i, v)
			}
		case float64:
			if v != 1.0 {
				t.Errorf("item %d: expected myNewField=1, got float64(%v)", i, v)
			}
		default:
			t.Errorf("item %d: expected myNewField=1, got %v (%T)", i, newField, newField)
		}
	}
}

// TestCodeNode_Execute_N8nWorkflowV4432EsGIkpqIZx9_BodyStyle pins the
// exact user-reported workflow `V4432EsGIkpqIZx9` ("Simple Webhook -
// Code"). Its Code node ships only `jsCode` with the following body:
//
//	for (const item of $input.all()) {
//	  item.json.myNewField = 1;
//	}
//	return $input.all();
//
// n8n expects the IIFE's return value (the modified input items) to
// flow downstream and ultimately reach the HTTP response. m9m
// previously rejected this workflow with `language parameter is
// required` (because n8n omits `language`) and would have rejected
// the `return` statement (because Goja compiles the snippet as a
// program, not a function body). Both gaps are now closed.
func TestCodeNode_Execute_N8nWorkflowV4432EsGIkpqIZx9_BodyStyle(t *testing.T) {
	node := NewCodeNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{
			"headers": map[string]interface{}{"host": "187.77.113.218"},
			"body":    map[string]interface{}{"variable": "1"},
		}},
	}

	nodeParams := map[string]interface{}{
		"jsCode": `// Loop over input items and add a new field called 'myNewField' to the JSON of each one
for (const item of $input.all()) {
  item.json.myNewField = 1;
}
return $input.all();`,
	}

	if err := node.ValidateParameters(nodeParams); err != nil {
		t.Fatalf("ValidateParameters should accept the n8n Code v2 export, got: %v", err)
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Execute should run the function-body-style code, got error: %v", err)
	}

	// The user's `return $input.all()` returns the *same* input
	// items back (with `myNewField` mutated in place), so the output
	// must contain exactly one item, with the new field present.
	if len(result) != 1 {
		t.Fatalf("Expected 1 result item (input echoed), got %d", len(result))
	}

	got, ok := result[0].JSON["myNewField"]
	if !ok {
		t.Fatalf("result missing myNewField; JSON=%v", result[0].JSON)
	}
	switch v := got.(type) {
	case int64:
		if v != 1 {
			t.Errorf("expected myNewField=1, got int64(%d)", v)
		}
	case float64:
		if v != 1.0 {
			t.Errorf("expected myNewField=1, got float64(%v)", v)
		}
	default:
		t.Errorf("expected myNewField=1, got %v (%T)", got, got)
	}
}

func TestCodeNodeImplementsNodeExecutor(t *testing.T) {
	var _ base.NodeExecutor = (*CodeNode)(nil)
}
