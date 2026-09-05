/*
Package transform provides data transformation node implementations for m9m.
*/
package transform

import (
	"fmt"
	"strconv"
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
	if params == nil {
		return []interface{}{}
	}
	raw, ok := params["assignments"]
	if !ok {
		// Some n8n exports store only an `options` map (no `assignments`
		// key at all). Treat that as pass-through so workflows that
		// legitimately have an empty Set still execute.
		if _, hasOptions := params["options"]; hasOptions {
			return []interface{}{}
		}
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
		// Empty options map from n8n export (e.g. {"options": {}}) — treat
		// as pass-through rather than an error so workflows without set
		// fields execute instead of failing validation.
		return []interface{}{}
	default:
		return nil
	}
}

// writeDotPath assigns `value` to `name` inside `root`, treating dot
// separators as nested object paths. A plain name (`"foo"`) writes
// the value directly; a dotted name (`"body.variable"`) descends into
// `root["body"]` (creating the map if missing) and writes at
// `["variable"]`. This matches n8n's Set node semantics for
// typeVersion >= 3.
//
// Returns an error if a dotted path segment collides with a non-map
// value (e.g. `root["body"]` already holds a string), so misconfigured
// assignments fail loudly instead of silently overwriting prior work.
func writeDotPath(root map[string]interface{}, name string, value interface{}) error {
	if name == "" {
		return fmt.Errorf("assignment name cannot be empty")
	}
	segments := strings.Split(name, ".")
	if len(segments) == 1 {
		root[name] = value
		return nil
	}
	current := root
	for i, seg := range segments {
		if i == len(segments)-1 {
			current[seg] = value
			return nil
		}
		existing, ok := current[seg]
		if !ok {
			nested := map[string]interface{}{}
			current[seg] = nested
			current = nested
			continue
		}
		nested, ok := existing.(map[string]interface{})
		if !ok {
			return fmt.Errorf("cannot assign %q: path segment %q is %T, not an object", name, seg, existing)
		}
		current = nested
	}
	return nil
}

// coerceAssignmentValue converts a value produced by an assignment (after any
// expression evaluation) to the type declared in the assignment's `type`
// field. This matches n8n's behaviour where `type: "number"` coerces string
// inputs to numeric values, so expressions like `{{ $json.a + $json.b }}`
// perform arithmetic on string-encoded numbers rather than concatenation.
//
// Supported `type` values (case-insensitive):
//
//	"string"   — fmt.Stringer-style conversion; nil becomes "".
//	"number"   — float64; strings are parsed (errors propagate), bool→0/1,
//	             nil→0, all numeric kinds pass through widened to float64.
//	"boolean"  — bool; "true"/"false" (any case) parse, truthy numerics→true,
//	             nil→false.
//	"object"   — returned unchanged.
//	"array"    — returned unchanged.
//
// An empty or unknown `type` returns the value unchanged so workflows that
// pre-date the typed-assignment model (e.g. webhook-processing.json) keep
// their existing behaviour byte-for-byte.
func coerceAssignmentValue(value interface{}, typeName string) (interface{}, error) {
	if typeName == "" {
		return value, nil
	}
	switch strings.ToLower(typeName) {
	case "string":
		if value == nil {
			return "", nil
		}
		switch v := value.(type) {
		case string:
			return v, nil
		case bool:
			return strconv.FormatBool(v), nil
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64), nil
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
		case int:
			return strconv.Itoa(v), nil
		case int32:
			return strconv.FormatInt(int64(v), 10), nil
		case int64:
			return strconv.FormatInt(v, 10), nil
		default:
			return fmt.Sprintf("%v", value), nil
		}
	case "number":
		switch v := value.(type) {
		case nil:
			return float64(0), nil
		case bool:
			if v {
				return float64(1), nil
			}
			return float64(0), nil
		case float64:
			return v, nil
		case float32:
			return float64(v), nil
		case int:
			return float64(v), nil
		case int32:
			return float64(v), nil
		case int64:
			return float64(v), nil
		case string:
			// Use ParseFloat so we accept both "5" and "5.0" and reject
			// empty strings. Whitespace is intentionally not trimmed —
			// that matches n8n's strict behaviour and surfaces
			// misconfigured assignments loudly.
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %q to number: %w", v, err)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to number", value)
		}
	case "boolean":
		switch v := value.(type) {
		case nil:
			return false, nil
		case bool:
			return v, nil
		case string:
			// Match n8n's leniency: only the literal string "true" (any
			// case) parses as true; everything else is false rather than
			// an error, so noisy data doesn't break the workflow.
			b, err := strconv.ParseBool(v)
			if err != nil {
				return false, nil
			}
			return b, nil
		case float64:
			return v != 0, nil
		case float32:
			return v != 0, nil
		case int:
			return v != 0, nil
		case int32:
			return v != 0, nil
		case int64:
			return v != 0, nil
		default:
			return false, nil
		}
	case "object", "array":
		return value, nil
	default:
		// Unknown type — keep value unchanged so we don't surprise
		// workflows that use vendor-specific type names.
		return value, nil
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

// shouldReplaceJSON reports whether the Set node should *replace* the
// upstream JSON with the assignments only (n8n's `keepOnlySet` default for
// typeVersion 3+), rather than merge the assignments into the upstream
// data.
//
// n8n Set node semantics (typeVersion >= 3.1):
//
//	options: {}                                → default → REPLACE
//	options: { keepOnlySet: true }             → REPLACE
//	options: { keepOnlySet: false }            → MERGE (legacy behaviour)
//
// Earlier typeVersions (and the legacy flat-assignments shape) merge by
// default; that matches what m9m shipped before this fix and we keep
// that as the fallback for any n8n export that explicitly asks for it.
func shouldReplaceJSON(nodeParams map[string]interface{}) bool {
	if nodeParams == nil {
		return true
	}
	optsRaw, ok := nodeParams["options"]
	if !ok {
		// No options at all → n8n's behaviour is to replace.
		return true
	}
	opts, ok := optsRaw.(map[string]interface{})
	if !ok {
		// Malformed options — fall back to replace (the safer default
		// for n8n typeVersion 3+ exports).
		return true
	}
	if v, ok := opts["keepOnlySet"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	// n8n's Set node typeVersion 3.0 used the legacy `includeOtherFields`
	// flag (true = merge upstream + assignments, false = replace with
	// assignments only). typeVersion 3.1+ renamed it to `keepOnlySet`
	// with the inverted meaning. Workflows that have not yet been
	// re-saved through the v3.1+ editor still ship `includeOtherFields`;
	// honour it so MERGE vs REPLACE behaves the way the workflow author
	// intended. `includeOtherFields=true` means "keep upstream fields",
	// which is the opposite of `keepOnlySet=true` ("discard upstream
	// fields"), so the boolean is inverted here.
	if v, ok := opts["includeOtherFields"]; ok {
		if b, ok := v.(bool); ok {
			return !b
		}
	}
	// options present but no keepOnlySet / includeOtherFields flag —
	// n8n defaults to keepOnlySet=true for typeVersion 3+ when `options`
	// is empty.
	return true
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

	// n8n Set node default for typeVersion >= 3.1 is "Keep Only Set" — i.e.
	// replace the upstream JSON with the assignments rather than merge.
	// This is what makes workflows like `Webhook → Set → Set (sum)`
	// return only the final Set's keys (e.g. `[{"total":3}]`) instead of
	// the merged upstream context. Honour that default here so m9m's
	// wire shape matches n8n's exactly.
	replaceJSON := shouldReplaceJSON(nodeParams)

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

		// Start with either a fresh map (replace mode) or a copy of the
		// upstream JSON (merge mode).
		newItem := model.DataItem{
			JSON: make(map[string]interface{}),
		}

		if !replaceJSON {
			for k, v := range item.JSON {
				newItem.JSON[k] = v
			}
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

			// `type` is optional. When present, coerce the (possibly
			// evaluated) value to that type before writing. n8n Set nodes
			// ignore `type` at runtime for the legacy flat shape but rely
			// on it for the typeVersion>=3 nested shape — so honouring
			// it here is what makes `{{ $json.a + $json.b }}` sum rather
			// than concatenate when both operands are stringified.
			assignmentType, _ := assignmentMap["type"].(string)

			// Check if the value is a string that might contain expressions
			var resolvedValue interface{}
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
					resolvedValue = value
				}
				if toEvaluate != "" {
					evaluatedValue, err := s.evaluator.EvaluateExpression(toEvaluate, context)
					if err != nil {
						return nil, s.CreateError(fmt.Sprintf("failed to evaluate expression '%s': %v", valueStr, err), nil)
					}
					resolvedValue = evaluatedValue
				}
			} else {
				// Non-string literal value (number, bool, object, …).
				resolvedValue = value
			}

			coerced, err := coerceAssignmentValue(resolvedValue, assignmentType)
			if err != nil {
				return nil, s.CreateError(fmt.Sprintf("failed to coerce assignment %q to type %q: %v", name, assignmentType, err), nil)
			}
			// n8n treats dots in assignment names as nested object
			// paths: `name: "body.variable", value: 1` produces
			// `{"body": {"variable": 1}}` rather than a literal
			// `"body.variable"` key. Mirror that behaviour so
			// existing workflow exports continue to produce the
			// same wire shape without forcing operators to flatten
			// the assignments.
			if err := writeDotPath(newItem.JSON, name, coerced); err != nil {
				return nil, s.CreateError(err.Error(), nil)
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
