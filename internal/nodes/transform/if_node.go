package transform

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// IfNode routes items based on conditions, tagging each with the evaluation result.
type IfNode struct {
	*base.BaseNode
	evaluator *expressions.GojaExpressionEvaluator
}

// ifRunContext carries the live workflow/runExecutionData so
// condition expressions like `$('Set Counter').item.json.attempt`
// resolve the same way Set/Code/Mongo do under RunAwareNodeExecutor.
type ifRunContext struct {
	workflow  *model.Workflow
	runData   *expressions.RunExecutionData
	runIndex  int
	itemIndex int
}

// NewIfNode creates a new IF node.
func NewIfNode() *IfNode {
	return &IfNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "IF",
			Description: "Routes items based on conditions",
			Category:    "Data Transformation",
			Properties:  ifProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main", "main"},
		}),
		evaluator: expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig()),
	}
}

// ifProperties returns the property descriptors for the IF node.
// The combinator + conditions shape matches n8n's
// INodeTypeDescription.properties so the n8n-style editor renders
// the same Parameters form.
func ifProperties() []base.NodeProperty {
	combinators := []base.Option{
		{Name: "All conditions must be true (AND)", Value: "and"},
		{Name: "Any condition may be true (OR)", Value: "or"},
	}
	// Subset of n8n's IF operators. The engine evaluates them via
	// the existing condition logic — adding more here is purely a
	// UI affordance and does not change runtime behaviour.
	operators := []base.Option{
		{Name: "equals", Value: "equals"},
		{Name: "not equals", Value: "notEquals"},
		{Name: "contains", Value: "contains"},
		{Name: "does not contain", Value: "notContains"},
		{Name: "starts with", Value: "startsWith"},
		{Name: "ends with", Value: "endsWith"},
		{Name: "is empty", Value: "isEmpty"},
		{Name: "is not empty", Value: "isNotEmpty"},
		{Name: "greater than", Value: "greaterThan"},
		{Name: "less than", Value: "lessThan"},
		{Name: "greater than or equal", Value: "gte"},
		{Name: "less than or equal", Value: "lte"},
		{Name: "is true", Value: "isTrue"},
		{Name: "is false", Value: "isFalse"},
	}
	conditionRow := base.CollectionProp("Conditions", "conditions.conditions", "Conditions to evaluate per item.", operators)
	props := []base.NodeProperty{
		base.StringOpt("Combinator", "conditions.combinator", "and", "How to combine multiple conditions.", combinators, true),
		conditionRow,
		base.BoolProp("Case sensitive", "options.caseSensitive", true, "Whether string comparisons are case-sensitive."),
		base.BoolProp("Type validation", "options.typeValidation", false, "When true, mismatched types compare as unequal instead of being coerced."),
	}
	return append(props, base.CommonSettings()...)
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
// ExecuteWithRun is the run-aware path. Engines that implement
// RunAwareNodeExecutor call this so Finnet's If-retry guard
// (`$('Set Counter').item.json.attempt < maxAttempts`) and
// Check Key Exists1 (`$json.respond_type`) share the live run.
func (n *IfNode) ExecuteWithRun(workflow *model.Workflow, runData *expressions.RunExecutionData, runIndex, itemIndex int, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return n.execute(inputData, nodeParams, &ifRunContext{
		workflow:  workflow,
		runData:   runData,
		runIndex:  runIndex,
		itemIndex: itemIndex,
	})
}

func (n *IfNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return n.execute(inputData, nodeParams, nil)
}

func (n *IfNode) execute(inputData []model.DataItem, nodeParams map[string]interface{}, runCtx *ifRunContext) ([]model.DataItem, error) {
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
	// n8n IF v2 stores the combinator under conditions.combinator.
	if combiner == "and" {
		if wrap, ok := conditions.(map[string]interface{}); ok {
			if c, ok := wrap["combinator"].(string); ok && c != "" {
				combiner = c
			}
		}
	}

	var trueItems, falseItems []model.DataItem

	for i, item := range inputData {
		itemRunCtx := runCtx
		if runCtx != nil {
			// Per-item index so $('…').item resolves paired items.
			copied := *runCtx
			copied.itemIndex = i
			itemRunCtx = &copied
		}
		passes := EvaluateConditionsWithRun(item, conditionsArr, combiner, n.evaluator, itemRunCtx)
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
