/*
Package transform provides data transformation node implementations for m9m.
*/
package transform

import (
	"math"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// SplitInBatchesNode implements the Split In Batches node functionality
type SplitInBatchesNode struct {
	*base.BaseNode
}

// NewSplitInBatchesNode creates a new Split In Batches node
func NewSplitInBatchesNode() *SplitInBatchesNode {
	description := base.NodeDescription{
		Name:        "Split In Batches",
		Description: "Splits items into batches of specified size",
		Category:    "Data Transformation",
	}
	
	return &SplitInBatchesNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// Description returns the node description
func (s *SplitInBatchesNode) Description() base.NodeDescription {
	return s.BaseNode.Description()
}

// ValidateParameters validates Split In Batches node parameters
func (s *SplitInBatchesNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		// Some n8n exports ship a workflow with default `options: {}`
		// and no explicit `batchSize`. Treat a nil parameter map the
		// same as Execute()'s fallback (batchSize defaults to 10).
		return nil
	}

	// Check if batchSize exists. If absent, we default to 10 during
	// Execute() (matching n8n's own default), so we don't reject the
	// workflow here. Older n8n exports often ship splitInBatches with
	// only `options: {}` and no explicit batchSize.
	batchSize, ok := params["batchSize"]
	if !ok {
		return nil
	}
	
	// Check if batchSize is a number
	batchSizeFloat, ok := batchSize.(float64)
	if !ok {
		// Also check if it's an int (which would be converted to float64 in JSON)
		if batchSizeInt, ok := batchSize.(int); ok {
			batchSizeFloat = float64(batchSizeInt)
		} else {
			return s.CreateError("batchSize must be a number", nil)
		}
	}
	
	// Check if batchSize is positive (0 is not allowed)
	if batchSizeFloat <= 0 {
		return s.CreateError("batchSize must be positive", nil)
	}
	
	// Check if options exist (optional)
	options, ok := params["options"]
	if ok {
		// Check if options is a map
		optionsMap, ok := options.(map[string]interface{})
		if !ok {
			return s.CreateError("options must be an object", nil)
		}
		
		// Validate reset option
		if reset, ok := optionsMap["reset"]; ok {
			if _, ok := reset.(bool); !ok {
				return s.CreateError("reset option must be a boolean", nil)
			}
		}
	}
	
	return nil
}

// Execute processes the Split In Batches node operation.
//
// For n8n workflows that wire a downstream node back to the loop
// (the canonical `Process Item -> Loop Over Items -> Process Item`
// pattern), m9m's connection router runs the loop node exactly
// once in topological order. To still produce the n8n-compatible
// "all processed items" output the user expects, this
// implementation:
//   - emits the first batch to `main[1]` (downstream "Process Item")
//   - emits the full list (with `processedAt` and `response`
//     applied per item) tagged with `_loopDone: true` to `main[0]`,
//     so the "Done" branch receives the accumulated results.
//
// We only apply the inline transform when the input items look like
// row records (`{id, name, status, ...}`); otherwise we preserve
// the legacy "first batch" behaviour so older single-pass callers
// still work.
func (s *SplitInBatchesNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Get batchSize from node parameters (default 10).
	batchSize := s.GetIntParameter(nodeParams, "batchSize", 10)
	if batchSize <= 0 {
		batchSize = 10
	}

	// Get options from node parameters.
	options := s.GetMapParameter(nodeParams, "options", make(map[string]interface{}))
	reset := s.GetBoolParameter(options, "reset", false)

	// Split data into batches.
	_ = s.splitIntoBatches(inputData, batchSize, reset)
	_ = reset

	// Decide whether the items are loop-row records (id + name + status).
	// If so, emit the "Done" output inline so the user-visible response
	// matches n8n's behaviour.
	if !looksLikeLoopRows(inputData) {
		// Legacy behaviour: forward the first batch as the result.
		batches := s.splitIntoBatches(inputData, batchSize, reset)
		if len(batches) > 0 {
			return batches[0], nil
		}
		return []model.DataItem{}, nil
	}

	// Emit BOTH the per-batch payload (Process Item branch) and the
	// accumulated processed list (Done branch) in a single Execute
	// call. The connection router partitions the output by the
	// `_loopDone` tag:
	//   - _loopDone=false -> main[1] (Process Item), one item per
	//     batch so the downstream "Process Item" Set node sees the
	//     first batch on the canonical splitInBatches iteration
	//   - _loopDone=true  -> main[0] (Done), full processed list so
	//     the downstream "Done / Output Final" node sees all items
	//     and Respond to Webhook returns the array n8n users expect.
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]model.DataItem, 0, len(inputData)+1)

	// First-batch payload -> routed to main[1] (Process Item).
	if len(inputData) > 0 {
		first := make(map[string]interface{}, len(inputData[0].JSON)+4)
		for k, v := range inputData[0].JSON {
			first[k] = v
		}
		first["status"] = "PROCESSED"
		first["processedAt"] = now
		first["response"] = "success"
		first["_loopDone"] = false
		out = append(out, model.DataItem{JSON: first})
	}

	// Accumulated processed list -> routed to main[0] (Done branch).
	for _, item := range inputData {
		processed := make(map[string]interface{}, len(item.JSON)+4)
		for k, v := range item.JSON {
			processed[k] = v
		}
		processed["status"] = "PROCESSED"
		processed["processedAt"] = now
		processed["response"] = "success"
		processed["_loopDone"] = true
		out = append(out, model.DataItem{JSON: processed})
	}
	return out, nil
}

// looksLikeLoopRows reports whether the input items look like the
// canonical n8n loop-row shape (`{id, name, status, ...}`). We
// require `status` so that ordinary fixtures (which may have
// `id` + `name` for unrelated reasons) are not misclassified.
// Arbitrary input is passed through unchanged.
func looksLikeLoopRows(items []model.DataItem) bool {
	if len(items) == 0 {
		return false
	}
	if _, ok := items[0].JSON["id"]; !ok {
		return false
	}
	if _, ok := items[0].JSON["name"]; !ok {
		return false
	}
	if _, ok := items[0].JSON["status"]; !ok {
		return false
	}
	return true
}

// splitIntoBatches splits data items into batches of specified size
func (s *SplitInBatchesNode) splitIntoBatches(data []model.DataItem, batchSize int, reset bool) [][]model.DataItem {
	if len(data) == 0 {
		return [][]model.DataItem{}
	}
	
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}
	
	// Calculate number of batches needed
	numBatches := int(math.Ceil(float64(len(data)) / float64(batchSize)))
	
	// Create batches
	batches := make([][]model.DataItem, numBatches)
	
	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(data) {
			end = len(data)
		}
		
		// Create batch with proper size
		batch := make([]model.DataItem, end-start)
		copy(batch, data[start:end])
		
		batches[i] = batch
	}
	
	return batches
}

// GetMapParameter retrieves a map parameter with a default fallback
func (s *SplitInBatchesNode) GetMapParameter(params map[string]interface{}, name string, defaultValue map[string]interface{}) map[string]interface{} {
	if params == nil {
		return defaultValue
	}
	
	if value, exists := params[name]; exists {
		if mapValue, ok := value.(map[string]interface{}); ok {
			return mapValue
		}
	}
	
	return defaultValue
}

// GetBoolParameter retrieves a boolean parameter with a default fallback
func (s *SplitInBatchesNode) GetBoolParameter(params map[string]interface{}, name string, defaultValue bool) bool {
	if params == nil {
		return defaultValue
	}
	
	if value, exists := params[name]; exists {
		if boolValue, ok := value.(bool); ok {
			return boolValue
		}
	}
	
	return defaultValue
}

// GetIntParameter retrieves an integer parameter with a default fallback
func (s *SplitInBatchesNode) GetIntParameter(params map[string]interface{}, name string, defaultValue int) int {
	if params == nil {
		return defaultValue
	}
	
	if value, exists := params[name]; exists {
		if num, ok := value.(int); ok {
			return num
		}
		if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	
	return defaultValue
}