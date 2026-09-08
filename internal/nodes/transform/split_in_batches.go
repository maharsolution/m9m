/*
Package transform provides data transformation node implementations for m9m.
*/
package transform

import (
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// SplitInBatchesNode implements the Split In Batches node
// functionality.
//
// Split In Batches is n8n's loop construct. The workflow pattern it
// enables is:
//
//	Upstream -> Split In Batches -- main[1] (process) -> Process Item
//	                                ^                          |
//	                                +------- back-edge --------+
//
// and once the upstream items are exhausted:
//
//	                       -- main[0] (done) -> Done / Output Final
//
// The actual iteration is driven by the workflow engine (see
// `engine.executeSplitInBatchesLoop`). This node's `Execute`
// therefore acts as a no-op pass-through: the engine uses the
// node only for parameter validation and as the entry point into
// the per-batch body chain. Returning the input unchanged keeps
// the engine's per-node routing happy - the engine will route the
// per-batch slice to `main[1]` and the aggregated result to
// `main[0]` itself, and the downstream nodes (Process Item, Done /
// Output Final) run with real n8n-compatible data.
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

// ValidateParameters validates Split In Batches node parameters.
//
// The valid parameter set matches what n8n's UI exposes for the
// Loop node:
//
//	{
//	  "batchSize": <positive number>,           // optional, default 10
//	  "options":   { "reset": <bool>, ... }     // optional
//	}
//
// Older n8n exports ship this node with only `options: {}` and no
// explicit `batchSize`; we accept that as "use the engine default"
// and let the loop driver pick batchSize=10 at iteration time.
func (s *SplitInBatchesNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return nil
	}

	// If batchSize is absent, default to 10 in the loop driver.
	batchSize, ok := params["batchSize"]
	if !ok {
		return nil
	}

	// batchSize must be numeric
	batchSizeFloat, ok := batchSize.(float64)
	if !ok {
		if batchSizeInt, ok := batchSize.(int); ok {
			batchSizeFloat = float64(batchSizeInt)
		} else {
			return s.CreateError("batchSize must be a number", nil)
		}
	}

	// batchSize must be positive (0 is not allowed)
	if batchSizeFloat <= 0 {
		return s.CreateError("batchSize must be positive", nil)
	}

	// options is optional
	options, ok := params["options"]
	if ok {
		optionsMap, ok := options.(map[string]interface{})
		if !ok {
			return s.CreateError("options must be an object", nil)
		}

		// Validate reset option if present
		if reset, ok := optionsMap["reset"]; ok {
			if _, ok := reset.(bool); !ok {
				return s.CreateError("reset option must be a boolean", nil)
			}
		}
	}

	return nil
}

// Execute returns the input items unchanged.
//
// The real iteration logic lives in the workflow engine
// (`engine.executeSplitInBatchesLoop`). The engine detects
// splitInBatches nodes by type and runs the per-batch body chain
// followed by the once-per-loop done branch on its own. This
// method exists so that the node still satisfies the
// `NodeExecutor` interface and so that any other engine path
// (older legacy execution) sees a sane pass-through instead of a
// panicking stub.
func (s *SplitInBatchesNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}
	if err := s.ValidateParameters(nodeParams); err != nil {
		return nil, err
	}
	return inputData, nil
}
