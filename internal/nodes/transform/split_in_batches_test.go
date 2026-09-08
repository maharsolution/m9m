package transform

import (
	"reflect"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

func TestSplitInBatchesNodeCreation(t *testing.T) {
	node := NewSplitInBatchesNode()
	if node == nil {
		t.Fatal("Expected node to be created, got nil")
	}

	desc := node.Description()
	if desc.Name != "Split In Batches" {
		t.Errorf("Expected name 'Split In Batches', got '%s'", desc.Name)
	}
}

func TestSplitInBatchesNodeValidateParameters(t *testing.T) {
	node := NewSplitInBatchesNode()

	// n8n exports frequently ship a workflow with default `options: {}`
	// and no explicit `batchSize`. We default to 10 in the loop
	// driver, so nil/missing batchSize is fine - only an *invalid*
	// batchSize (non-numeric, zero, or negative) is rejected.

	// Test with nil params (older n8n export style)
	err := node.ValidateParameters(nil)
	if err != nil {
		t.Errorf("Expected no error with nil params (defaults to 10), got %v", err)
	}

	// Test with missing batchSize (also defaults to 10)
	params := map[string]interface{}{}
	err = node.ValidateParameters(params)
	if err != nil {
		t.Errorf("Expected no error with missing batchSize (defaults to 10), got %v", err)
	}

	// Test with invalid batchSize type
	invalidTypeParams := map[string]interface{}{
		"batchSize": "not a number",
	}
	err = node.ValidateParameters(invalidTypeParams)
	if err == nil {
		t.Error("Expected error with invalid batchSize type, got nil")
	}

	// Test with zero batchSize (should be rejected during validation)
	zeroBatchSizeParams := map[string]interface{}{
		"batchSize": 0,
	}
	err = node.ValidateParameters(zeroBatchSizeParams)
	if err == nil {
		t.Error("Expected error with zero batchSize, got nil")
	}

	// Test with negative batchSize (should be rejected during validation)
	negativeBatchSizeParams := map[string]interface{}{
		"batchSize": -5,
	}
	err = node.ValidateParameters(negativeBatchSizeParams)
	if err == nil {
		t.Error("Expected error with negative batchSize, got nil")
	}

	// Test with valid batchSize
	validBatchSizeParams := map[string]interface{}{
		"batchSize": 10,
	}
	err = node.ValidateParameters(validBatchSizeParams)
	if err != nil {
		t.Errorf("Expected no error with valid batchSize, got %v", err)
	}

	// Test with valid options
	validOptionsParams := map[string]interface{}{
		"batchSize": 10,
		"options": map[string]interface{}{
			"reset": true,
		},
	}
	err = node.ValidateParameters(validOptionsParams)
	if err != nil {
		t.Errorf("Expected no error with valid options, got %v", err)
	}

	// Test with valid options and reset false
	validOptionsResetFalseParams := map[string]interface{}{
		"batchSize": 10,
		"options": map[string]interface{}{
			"reset": false,
		},
	}
	err = node.ValidateParameters(validOptionsResetFalseParams)
	if err != nil {
		t.Errorf("Expected no error with valid options and reset false, got %v", err)
	}
}

func TestSplitInBatchesNodeExecutePassThrough(t *testing.T) {
	node := NewSplitInBatchesNode()

	inputData := []model.DataItem{
		{JSON: map[string]interface{}{"id": float64(1), "name": "Andi"}},
		{JSON: map[string]interface{}{"id": float64(2), "name": "Budi"}},
		{JSON: map[string]interface{}{"id": float64(3), "name": "Citra"}},
	}

	// The node is now an engine-driven pass-through: real iteration
	// happens inside engine.executeSplitInBatchesLoop. Execute just
	// hands the input back unchanged so the engine's per-node routing
	// sees a well-formed slice it can re-route per batch.
	result, err := node.Execute(inputData, map[string]interface{}{"batchSize": 2})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !reflect.DeepEqual(result, inputData) {
		t.Fatalf("Expected Execute to return input unchanged.\n got: %+v\nwant: %+v", result, inputData)
	}
}

func TestSplitInBatchesNodeExecuteWithEmptyInput(t *testing.T) {
	node := NewSplitInBatchesNode()

	result, err := node.Execute([]model.DataItem{}, map[string]interface{}{"batchSize": 10})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

func TestSplitInBatchesNodeImplementsNodeExecutor(t *testing.T) {
	var _ base.NodeExecutor = (*SplitInBatchesNode)(nil)
}
