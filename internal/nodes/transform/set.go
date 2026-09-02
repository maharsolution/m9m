/*
Package transform provides data transformation node implementations for m9m.
*/
package transform

import (
	"fmt"
	"strings"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// SetNode implements the Set node functionality for assigning values to fields
type SetNode struct {
	*base.BaseNode
	evaluator *expressions.GojaExpressionEvaluator
}

// NewSetNode creates a new Set node
func NewSetNode() *SetNode {
	description := base.NodeDescription{
		Name:        "Set",
		Description: "Sets values on items",
		Category:    "Data Transformation",
	}

	return &SetNode{
		BaseNode:  base.NewBaseNode(description),
		evaluator: expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig()),
	}
}

// Description returns the node description
func (s *SetNode) Description() base.NodeDescription {
	return s.BaseNode.Description()
}

// extractAssignments accepts both the legacy flat shape and the newer
// n8n typeVersion>=3 nested shape:
//
//	flat:    parameters.assignments = [{name, value, ...}, ...]
//	nested:  parameters.assignments = { assignments: [{name, value, ...}, ...] }
//
// It returns the raw []interface{} of assignment maps. A nil result means
// "no assignments found" — callers should decide whether that is an error
// (ValidateParameters) or a no-op (Execute with empty list).
func extractAssignments(params map[string]interface{}) []interface{} {
	raw, ok := params["assignments"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		return v
	case map[string]interface{}:
		// Newer n8n (typeVersion 3+) wraps assignments under another
		// `assignments` key inside the parameters object.
		if nested, ok := v["assignments"]; ok {
			if arr, ok := nested.([]interface{}); ok {
				return arr
			}
		}
		return nil
	default:
		return nil
	}
}

// ValidateParameters validates Set node parameters
func (s *SetNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return s.CreateError("parameters cannot be nil", nil)
	}

	assignmentsArr := extractAssignments(params)
	if assignmentsArr == nil {
		return s.CreateError("assignments must be an array (or an object with an 'assignments' array)", nil)
	}

	// Validate each assignment
	for i, assignment := range assignmentsArr {
		assignmentMap, ok := assignment.(map[string]interface{})
		if !ok {
			return s.CreateError(fmt.Sprintf("assignment %d must be an object", i), nil)
		}

		// Check required fields
		if _, ok := assignmentMap["name"]; !ok {
			return s.CreateError(fmt.Sprintf("assignment %d missing 'name' field", i), nil)
		}

		if _, ok := assignmentMap["value"]; !ok {
			return s.CreateError(fmt.Sprintf("assignment %d missing 'value' field", i), nil)
		}
	}

	return nil
}

// Execute processes the Set node operation
func (s *SetNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Get assignments — accept both flat and nested shapes.
	assignments := extractAssignments(nodeParams)
	if assignments == nil {
		return nil, s.CreateError("assignments must be an array (or an object with an 'assignments' array)", nil)
	}

	// Process each input data item
	result := make([]model.DataItem, len(inputData))

	for i, item := range inputData {
		// Create expression context for current item
		context := &expressions.ExpressionContext{
			ActiveNodeName:      "Set",
			RunIndex:            0,
			ItemIndex:           0,
			Mode:                expressions.ModeManual,
			ConnectionInputData: []model.DataItem{item},
			Workflow: &model.Workflow{
				Name: "Set Processing",
			},
			AdditionalKeys: &expressions.AdditionalKeys{
				ExecutionId: "set-processing",
			},
		}

		// Copy the original item
		newItem := model.DataItem{
			JSON: make(map[string]interface{}),
		}

		// Copy existing JSON data
		for k, v := range item.JSON {
			newItem.JSON[k] = v
		}

		// Apply each assignment
		for _, assignment := range assignments {
			assignmentMap, ok := assignment.(map[string]interface{})
			if !ok {
				continue
			}

			name, nameOk := assignmentMap["name"].(string)
			value := assignmentMap["value"]

			if !nameOk {
				continue
			}

			// Check if the value is a string that might contain expressions
			if valueStr, ok := value.(string); ok {
				// Determine whether the value is an n8n expression and the
				// form it is in. The expression evaluator (parser.go)
				// natively understands three shapes:
				//
				//   1. leading `=`:   "={{ $json.x }}"   (n8n expression mode)
				//   2. already wrapped: "{{ $json.x }}"  (template fragment)
				//   3. bare:            "$json.x"         (treated as a value reference)
				//
				// Passing the value through as-is for the first two avoids
				// double-wrapping `{{ {{ $json.x }} }}` which would otherwise
				// produce a JS syntax error at evaluation time.
				var toEvaluate string
				switch {
				case strings.HasPrefix(valueStr, "="):
					// Strip the leading `=`; the parser's IsExpression flag
					// then makes it evaluate as a pure expression.
					toEvaluate = strings.TrimPrefix(valueStr, "=")
				case strings.HasPrefix(valueStr, "{{") && strings.HasSuffix(valueStr, "}}"):
					// Already wrapped — pass through unchanged.
					toEvaluate = valueStr
				default:
					// Plain literal value, no expression evaluation needed.
					newItem.JSON[name] = value
					continue
				}

				evaluatedValue, err := s.evaluator.EvaluateExpression(toEvaluate, context)
				if err != nil {
					return nil, s.CreateError(fmt.Sprintf("failed to evaluate expression '%s': %v", valueStr, err), nil)
				}
				newItem.JSON[name] = evaluatedValue
			} else {
				// Use the literal value
				newItem.JSON[name] = value
			}
		}

		// Copy binary data if present
		if item.Binary != nil {
			newItem.Binary = make(map[string]model.BinaryData)
			for k, v := range item.Binary {
				newItem.Binary[k] = v
			}
		}

		// Copy paired item data if present
		if item.PairedItem != nil {
			newItem.PairedItem = item.PairedItem
		}

		result[i] = newItem
	}

	return result, nil
}
