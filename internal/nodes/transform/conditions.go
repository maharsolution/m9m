package transform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neul-labs/m9m/internal/model"
)

// ConditionEvaluator provides shared condition evaluation logic for Filter and IF nodes.
type ConditionEvaluator struct{}

// ValidOperators is the set of supported condition operators.
var ValidOperators = map[string]bool{
	"equals":             true,
	"notEquals":          true,
	"contains":           true,
	"notContains":        true,
	"startsWith":         true,
	"endsWith":           true,
	"regex":              true,
	"exists":             true,
	"notExists":          true,
	"greaterThan":        true,
	"lessThan":           true,
	"greaterThanOrEqual": true,
	"lessThanOrEqual":    true,
	"between":            true,
	"empty":              true,
	"notEmpty":           true,
}

// ValidateConditions validates the conditions array and combiner.
//
// The IF node accepts two shapes for `conditions`:
//
//  1. A bare array — older n8n exports and the explicit form
//     used in tests:
//        [{"leftValue":..,"rightValue":..,"operator":..}, ...]
//
//  2. The wrapper object the current n8n UI writes, which nests
//     the array under `conditions` and pairs it with `options`
//     and `combinator`:
//        {"options":{...}, "conditions":[{...}], "combinator":"and"}
//
// The second shape is the dominant form in real exports, so we
// unwrap it here before validating.
func ValidateConditions(conditions interface{}, combiner string) error {
	conditionsArr, ok := resolveConditionsArray(conditions)
	if !ok {
		return fmt.Errorf("conditions must be an array")
	}

	for i, condition := range conditionsArr {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			return fmt.Errorf("condition %d must be an object", i)
		}

		if _, ok := conditionMap["leftValue"]; !ok {
			return fmt.Errorf("condition %d missing 'leftValue' field", i)
		}
		if _, ok := conditionMap["rightValue"]; !ok {
			return fmt.Errorf("condition %d missing 'rightValue' field", i)
		}
		if _, ok := conditionMap["operator"]; !ok {
			return fmt.Errorf("condition %d missing 'operator' field", i)
		}

		// n8n's IF v2 emits `operator` either as a bare string
		// (legacy) or as a typed object `{type, operation, singleValue}`
		// (current). We accept both shapes here and only validate
		// the resolved operation name so downstream evaluation can
		// reuse the same helper.
		operation, err := extractOperatorOperation(conditionMap["operator"])
		if err != nil {
			return err
		}
		if !ValidOperators[operation] {
			return fmt.Errorf("condition %d has invalid operator: %s", i, operation)
		}
	}

	if combiner != "and" && combiner != "or" {
		return fmt.Errorf("combiner must be 'and' or 'or'")
	}

	return nil
}

// extractOperatorOperation accepts either a string operator (legacy /
// explicit form) or an operator object with an `operation` field and
// returns the canonical operation name. Errors are returned so callers
// can surface a precise validation message.
func extractOperatorOperation(op interface{}) (string, error) {
	switch v := op.(type) {
	case string:
		return v, nil
	case map[string]interface{}:
		if name, ok := v["operation"].(string); ok && name != "" {
			return name, nil
		}
		return "", fmt.Errorf("operator object missing 'operation' field")
	default:
		return "", fmt.Errorf("operator must be a string or object")
	}
}

// resolveConditionsArray accepts either of the two IF-node `conditions`
// shapes (bare array or n8n's wrapper object) and returns the inner
// conditions array. The boolean reports whether unwrapping succeeded.
func resolveConditionsArray(conditions interface{}) ([]interface{}, bool) {
	switch v := conditions.(type) {
	case []interface{}:
		return v, true
	case map[string]interface{}:
		// n8n UI wrapper: {"options":{...},"conditions":[...],"combinator":...}
		inner, ok := v["conditions"]
		if !ok {
			return nil, false
		}
		arr, ok := inner.([]interface{})
		return arr, ok
	default:
		return nil, false
	}
}

// EvaluateConditions evaluates all conditions against a data item.
func EvaluateConditions(item model.DataItem, conditions []interface{}, combiner string) bool {
	if len(conditions) == 0 {
		return true
	}

	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			if combiner == "and" {
				return false
			}
			continue
		}

		result := evaluateCondition(item, conditionMap)

		if combiner == "and" && !result {
			return false
		}
		if combiner == "or" && result {
			return true
		}
	}

	return combiner == "and"
}

func evaluateCondition(item model.DataItem, condition map[string]interface{}) bool {
	leftValue := condition["leftValue"]
	rightValue := condition["rightValue"]
	// n8n's IF v2 emits the operator as either a bare string or a
	// typed object `{type, operation, singleValue}`. Normalize both
	// shapes to the operation name so the switch below can stay flat.
	operatorRaw := condition["operator"]
	operator, _ := extractOperatorOperation(operatorRaw)
	if operator == "" {
		operator = "equals"
	}

	leftResolved := resolveValue(item.JSON, leftValue)
	rightResolved := rightValue

	switch operator {
	case "exists":
		return leftResolved != nil
	case "notExists":
		return leftResolved == nil
	case "empty":
		return condIsEmpty(leftResolved)
	case "notEmpty":
		return !condIsEmpty(leftResolved)
	}

	if leftResolved == nil || rightResolved == nil {
		return false
	}

	switch operator {
	case "equals":
		return condEquals(leftResolved, rightResolved)
	case "notEquals":
		return !condEquals(leftResolved, rightResolved)
	case "contains":
		return condContains(leftResolved, rightResolved)
	case "notContains":
		return !condContains(leftResolved, rightResolved)
	case "startsWith":
		return strings.HasPrefix(fmt.Sprintf("%v", leftResolved), fmt.Sprintf("%v", rightResolved))
	case "endsWith":
		return strings.HasSuffix(fmt.Sprintf("%v", leftResolved), fmt.Sprintf("%v", rightResolved))
	case "regex":
		match, err := regexp.MatchString(fmt.Sprintf("%v", rightResolved), fmt.Sprintf("%v", leftResolved))
		return err == nil && match
	case "greaterThan":
		return condCompare(leftResolved, rightResolved, func(l, r float64) bool { return l > r })
	case "lessThan":
		return condCompare(leftResolved, rightResolved, func(l, r float64) bool { return l < r })
	case "greaterThanOrEqual":
		return condCompare(leftResolved, rightResolved, func(l, r float64) bool { return l >= r })
	case "lessThanOrEqual":
		return condCompare(leftResolved, rightResolved, func(l, r float64) bool { return l <= r })
	case "between":
		return condBetween(leftResolved, rightResolved)
	default:
		return false
	}
}

func resolveValue(data map[string]interface{}, value interface{}) interface{} {
	strValue, ok := value.(string)
	if !ok || !strings.HasPrefix(strValue, "$json.") {
		return value
	}
	path := strings.TrimPrefix(strValue, "$json.")
	return getValueAtPath(data, path)
}

func getValueAtPath(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return data
	}
	parts := strings.Split(path, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			return current[part]
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

func condIsEmpty(value interface{}) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case float64:
		return v == 0
	case bool:
		return !v
	default:
		return false
	}
}

func condEquals(left, right interface{}) bool {
	return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
}

func condContains(left, right interface{}) bool {
	return strings.Contains(fmt.Sprintf("%v", left), fmt.Sprintf("%v", right))
}

func condCompare(left, right interface{}, cmp func(float64, float64) bool) bool {
	l, lok := condToFloat64(left)
	r, rok := condToFloat64(right)
	if !lok || !rok {
		return false
	}
	return cmp(l, r)
}

func condToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	case string:
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return num, true
		}
	}
	return 0, false
}

func condBetween(left, right interface{}) bool {
	rightStr := fmt.Sprintf("%v", right)
	parts := strings.Split(rightStr, ",")
	if len(parts) != 2 {
		return false
	}
	min, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	max, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return false
	}
	leftNum, ok := condToFloat64(left)
	if !ok {
		return false
	}
	return leftNum >= min && leftNum <= max
}
