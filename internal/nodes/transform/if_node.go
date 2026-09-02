package transform

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// IfNode routes items based on conditions, tagging each with the evaluation result.
type IfNode struct {
	*base.BaseNode
}

// NewIfNode creates a new IF node.
func NewIfNode() *IfNode {
	return &IfNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "IF",
			Description: "Routes items based on conditions",
			Category:    "Data Transformation",
		}),
	}
}

// Execute evaluates conditions for each input item.
//
// Behaviour matches n8n's IF node: every input item is returned, tagged
// with an `_ifResult` metadata boolean indicating whether the item
// matched the configured condition set. The engine router partitions
// tagged items across the node's outgoing `main` connections based on
// each connection's index:
//
//	main[0]  →  items where _ifResult == true
//	main[1]  →  items where _ifResult == false
//
// Downstream nodes therefore see exactly the items n8n would route to
// them, including the false branch (which the prior implementation
// silently dropped).
//
// The `returnBothBranches` parameter is honoured for backwards
// compatibility: when false (the historic n8n default), items that
// fail the condition are still returned (tagged false) rather than
// dropped, so the false branch of the workflow still executes.
func (n *IfNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	conditions, ok := nodeParams["conditions"]
	if !ok {
		return nil, n.CreateError("conditions parameter is required", nil)
	}

	// Accept both the bare array form and the n8n UI wrapper
	// object form ({"options":..., "conditions":[...], "combinator":...}).
	// The wrapper is the dominant form in real n8n exports and matches
	// what the Switch node accepts after the same unwrap.
	conditionsArr, ok := resolveConditionsArray(conditions)
	if !ok {
		return nil, n.CreateError("conditions must be an array", nil)
	}

	combiner := n.GetStringParameter(nodeParams, "combiner", "and")

	var trueItems, falseItems []model.DataItem

	for _, item := range inputData {
		passes := EvaluateConditions(item, conditionsArr, combiner)
		if passes {
			trueItems = append(trueItems, item)
		} else {
			falseItems = append(falseItems, item)
		}
	}

	// Always tag and return both branches so the router can split
	// them across connections.Main[0] (true) and Main[1] (false).
	var result []model.DataItem
	for _, item := range trueItems {
		tagged := copyDataItem(item)
		tagged.JSON["_ifResult"] = true
		result = append(result, tagged)
	}
	for _, item := range falseItems {
		tagged := copyDataItem(item)
		tagged.JSON["_ifResult"] = false
		result = append(result, tagged)
	}
	return result, nil
}

// ValidateParameters validates IF node parameters.
func (n *IfNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return n.CreateError("parameters cannot be nil", nil)
	}

	conditions, ok := params["conditions"]
	if !ok {
		return n.CreateError("conditions parameter is required", nil)
	}

	combiner := n.GetStringParameter(params, "combiner", "and")
	if err := ValidateConditions(conditions, combiner); err != nil {
		return fmt.Errorf("node IF error: %w", err)
	}

	return nil
}

func copyDataItem(item model.DataItem) model.DataItem {
	newJSON := make(map[string]interface{}, len(item.JSON))
	for k, v := range item.JSON {
		newJSON[k] = v
	}
	return model.DataItem{JSON: newJSON}
}
