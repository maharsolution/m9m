package transform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// SwitchNode implements the Switch node functionality for conditional routing
type SwitchNode struct {
	*base.BaseNode
	evaluator *expressions.GojaExpressionEvaluator
}

// NewSwitchNode creates a new Switch node
func NewSwitchNode() *SwitchNode {
	description := base.NodeDescription{
		Name:        "Switch",
		Description: "Route data based on conditions",
		Category:    "Flow Control",
		Properties:  switchProperties(),
		Inputs:      []string{"main"},
		Outputs:     []string{"main", "main", "main", "main"},
	}

	return &SwitchNode{
		BaseNode:  base.NewBaseNode(description),
		evaluator: expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig()),
	}
}

// switchProperties returns the Switch node's property
// descriptors. The rule structure mirrors n8n:
//
//   rules.values[]    — list of routing conditions
//   options.fallbackOutput — index of the connection to use when no
//                            rule matches (matches `outputIndex` of
//                            a `main` slot)
func switchProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.FixedCollectionProp("Routing Rules", "rules.values", "Conditions that route items to each output."),
		base.NumberProp("Fallback output", "options.fallbackOutput", 0, "Output index to use when no rule matches.", false),
		base.BoolProp("Case sensitive", "options.caseSensitive", true, "Whether string comparisons are case-sensitive."),
		base.BoolProp("Type validation", "options.typeValidation", false, "If true, mismatched types compare as unequal instead of being coerced."),
	}
	return append(props, base.CommonSettings()...)
}

// ExtractSwitchRules pulls the Switch routing rules out of the parameter
// map, accepting the two aliases n8n emits today:
//
//   - `rules`        — the legacy / docs-only key
//   - `conditions`   — what the n8n UI writes today (the data flow editor
//     saves a list of routing conditions under `conditions`, not `rules`)
//
// The slice elements can be either `[]interface{}` (from generic
// JSON unmarshal) or `[]map[string]interface{}` (from typed unmarshal /
// the workflow loader); both are normalised to `[]map[string]interface{}`
// so the rest of the node can rely on a single shape.
func ExtractSwitchRules(params map[string]interface{}) ([]map[string]interface{}, error) {
	if len(params) == 0 {
		return nil, fmt.Errorf("switch rules are required")
	}

	raw, ok := params["rules"]
	if !ok || raw == nil {
		raw = params["conditions"] // n8n UI compatibility
	}
	if raw == nil {
		return nil, fmt.Errorf("switch rules are required")
	}

	switch v := raw.(type) {
	case map[string]interface{}:
		// n8n Switch v3+ wraps its rules in {"values": [...]}. Each
		// value contains a conditions wrapper, so normalise those
		// conditions to the flat rule shape used by this executor.
		values, ok := v["values"]
		if !ok || values == nil {
			return nil, fmt.Errorf("switch rules object must contain a values array")
		}
		return extractNestedSwitchRules(values)
	case []interface{}:
		return normalizeSwitchRuleSlice(v)
	case []map[string]interface{}:
		return normalizeSwitchRuleMaps(v)
	default:
		return nil, fmt.Errorf("switch rules must be an array or object with values")
	}
}

func extractNestedSwitchRules(raw interface{}) ([]map[string]interface{}, error) {
	switch v := raw.(type) {
	case []interface{}:
		return normalizeSwitchRuleSlice(v)
	case []map[string]interface{}:
		return normalizeSwitchRuleMaps(v)
	default:
		return nil, fmt.Errorf("switch rules values must be an array")
	}
}

func normalizeSwitchRuleSlice(values []interface{}) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(values))
	for i, value := range values {
		rule, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("switch rule %d is not an object", i)
		}
		rules, err := normalizeSwitchRule(rule)
		if err != nil {
			return nil, fmt.Errorf("switch rule %d: %w", i, err)
		}
		out = append(out, rules...)
	}
	return out, nil
}

func normalizeSwitchRuleMaps(values []map[string]interface{}) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(values))
	for i, value := range values {
		rules, err := normalizeSwitchRule(value)
		if err != nil {
			return nil, fmt.Errorf("switch rule %d: %w", i, err)
		}
		out = append(out, rules...)
	}
	return out, nil
}

func normalizeSwitchRule(rule map[string]interface{}) ([]map[string]interface{}, error) {
	conditions, ok := rule["conditions"]
	if !ok {
		return []map[string]interface{}{rule}, nil
	}

	conditionList, ok := conditions.([]interface{})
	if !ok {
		if wrapper, wrapperOK := conditions.(map[string]interface{}); wrapperOK {
			conditionList, ok = wrapper["conditions"].([]interface{})
		}
	}
	if !ok {
		return nil, fmt.Errorf("conditions must contain an array")
	}

	out := make([]map[string]interface{}, 0, len(conditionList))
	for _, rawCondition := range conditionList {
		condition, ok := rawCondition.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("condition is not an object")
		}
		field, _ := condition["leftValue"].(string)
		value := condition["rightValue"]
		operation, err := switchOperation(condition["operator"])
		if err != nil {
			return nil, err
		}
		if field == "" || operation == "" {
			return nil, fmt.Errorf("condition must contain leftValue, operator, and rightValue")
		}
		out = append(out, map[string]interface{}{
			"field": field, "operation": operation, "value": value,
		})
	}
	return out, nil
}

func switchOperation(raw interface{}) (string, error) {
	operation, ok := raw.(string)
	if !ok {
		if typed, ok := raw.(map[string]interface{}); ok {
			operation, _ = typed["operation"].(string)
		}
	}
	mapped := map[string]string{
		"equals": "equal", "notEquals": "notEqual", "contains": "contains",
		"notContains": "notContains", "startsWith": "startsWith", "endsWith": "endsWith",
		"regex": "regex", "greaterThan": "greater", "greaterThanOrEqual": "greaterEqual",
		"lessThan": "smaller", "lessThanOrEqual": "smallerEqual",
		"isEmpty": "isEmpty", "isNotEmpty": "isNotEmpty",
	}
	if normalized, ok := mapped[operation]; ok {
		return normalized, nil
	}
	return "", fmt.Errorf("unsupported switch operation: %s", operation)
}

// Execute processes the Switch node operation
func (s *SwitchNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Get routing rules (`rules` or `conditions` — both supported).
	rules, err := ExtractSwitchRules(nodeParams)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("switch rules are required")
	}

	var outputData []model.DataItem

	for _, item := range inputData {
		context := &expressions.ExpressionContext{
			ActiveNodeName:      "Switch",
			RunIndex:            0,
			ItemIndex:           0,
			Mode:                expressions.ModeManual,
			ConnectionInputData: []model.DataItem{item},
			Workflow: &model.Workflow{
				Name: "Switch Processing",
			},
			AdditionalKeys: &expressions.AdditionalKeys{
				ExecutionId: "switch-processing",
			},
		}

		// Check each rule in order
		matched := false
		for ruleIndex, ruleMap := range rules {
			matches, err := s.evaluateRule(ruleMap, context)
			if err != nil {
				return nil, fmt.Errorf("error evaluating rule %d: %w", ruleIndex, err)
			}

			if matches {
				// Add output index to indicate which rule matched
				outputItem := model.DataItem{
					JSON: item.JSON,
				}

				// Add metadata about which rule matched
				if outputItem.JSON == nil {
					outputItem.JSON = make(map[string]interface{})
				}
				outputItem.JSON["_switchRuleIndex"] = ruleIndex

				outputData = append(outputData, outputItem)
				matched = true

				// Check if we should stop after first match
				stopAfterFirstMatch, _ := nodeParams["stopAfterFirstMatch"].(bool)
				if stopAfterFirstMatch {
					break
				}
			}
		}

		// Handle fallback for unmatched items
		if !matched {
			fallbackToLast, _ := nodeParams["fallbackToLast"].(bool)
			if fallbackToLast {
				outputItem := model.DataItem{
					JSON: item.JSON,
				}
				if outputItem.JSON == nil {
					outputItem.JSON = make(map[string]interface{})
				}
				outputItem.JSON["_switchRuleIndex"] = len(rules) // Indicates fallback
				outputData = append(outputData, outputItem)
			}
		}
	}

	return outputData, nil
}

// evaluateRule evaluates a single switch rule
func (s *SwitchNode) evaluateRule(rule map[string]interface{}, context *expressions.ExpressionContext) (bool, error) {
	// Get rule properties
	field, _ := rule["field"].(string)
	operation, _ := rule["operation"].(string)
	value := rule["value"]

	if field == "" || operation == "" {
		return false, fmt.Errorf("field and operation are required for switch rule")
	}

	// Resolve the field value. n8n exports may already wrap the field
	// in `={{ ... }}`; avoid double-wrapping those expressions.
	fieldExpr := field
	if !strings.HasPrefix(strings.TrimSpace(fieldExpr), "{{") && !strings.HasPrefix(strings.TrimSpace(fieldExpr), "={{") {
		fieldExpr = fmt.Sprintf("{{ %s }}", fieldExpr)
	}
	fieldValue, err := s.evaluator.EvaluateExpression(fieldExpr, context)
	if err != nil {
		return false, fmt.Errorf("failed to resolve field %s: %w", field, err)
	}

	// Perform the comparison based on operation
	return s.compareValues(fieldValue, operation, value)
}

// compareValues compares two values based on the specified operation
func (s *SwitchNode) compareValues(fieldValue interface{}, operation string, expectedValue interface{}) (bool, error) {
	switch operation {
	case "equal":
		return s.isEqual(fieldValue, expectedValue), nil
	case "notEqual":
		return !s.isEqual(fieldValue, expectedValue), nil
	case "contains":
		return s.contains(fieldValue, expectedValue), nil
	case "notContains":
		return !s.contains(fieldValue, expectedValue), nil
	case "startsWith":
		return s.startsWith(fieldValue, expectedValue), nil
	case "endsWith":
		return s.endsWith(fieldValue, expectedValue), nil
	case "regex":
		return s.matchesRegex(fieldValue, expectedValue)
	case "greater":
		return s.isGreater(fieldValue, expectedValue), nil
	case "greaterEqual":
		return s.isGreaterOrEqual(fieldValue, expectedValue), nil
	case "smaller":
		return s.isSmaller(fieldValue, expectedValue), nil
	case "smallerEqual":
		return s.isSmallerOrEqual(fieldValue, expectedValue), nil
	case "isEmpty":
		return s.isEmpty(fieldValue), nil
	case "isNotEmpty":
		return !s.isEmpty(fieldValue), nil
	default:
		return false, fmt.Errorf("unsupported operation: %s", operation)
	}
}

// Helper functions for comparisons
func (s *SwitchNode) isEqual(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func (s *SwitchNode) contains(haystack, needle interface{}) bool {
	haystackStr := fmt.Sprintf("%v", haystack)
	needleStr := fmt.Sprintf("%v", needle)
	return strings.Contains(strings.ToLower(haystackStr), strings.ToLower(needleStr))
}

func (s *SwitchNode) startsWith(value, prefix interface{}) bool {
	valueStr := fmt.Sprintf("%v", value)
	prefixStr := fmt.Sprintf("%v", prefix)
	return strings.HasPrefix(strings.ToLower(valueStr), strings.ToLower(prefixStr))
}

func (s *SwitchNode) endsWith(value, suffix interface{}) bool {
	valueStr := fmt.Sprintf("%v", value)
	suffixStr := fmt.Sprintf("%v", suffix)
	return strings.HasSuffix(strings.ToLower(valueStr), strings.ToLower(suffixStr))
}

func (s *SwitchNode) matchesRegex(value, pattern interface{}) (bool, error) {
	valueStr := fmt.Sprintf("%v", value)
	patternStr := fmt.Sprintf("%v", pattern)

	regex, err := regexp.Compile(patternStr)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return regex.MatchString(valueStr), nil
}

func (s *SwitchNode) isGreater(a, b interface{}) bool {
	aNum, aErr := s.toNumber(a)
	bNum, bErr := s.toNumber(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return aNum > bNum
}

func (s *SwitchNode) isGreaterOrEqual(a, b interface{}) bool {
	aNum, aErr := s.toNumber(a)
	bNum, bErr := s.toNumber(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return aNum >= bNum
}

func (s *SwitchNode) isSmaller(a, b interface{}) bool {
	aNum, aErr := s.toNumber(a)
	bNum, bErr := s.toNumber(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return aNum < bNum
}

func (s *SwitchNode) isSmallerOrEqual(a, b interface{}) bool {
	aNum, aErr := s.toNumber(a)
	bNum, bErr := s.toNumber(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return aNum <= bNum
}

func (s *SwitchNode) isEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	default:
		return false
	}
}

func (s *SwitchNode) toNumber(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to number", value)
	}
}

// ValidateParameters validates Switch node parameters
func (s *SwitchNode) ValidateParameters(params map[string]interface{}) error {
	rules, err := ExtractSwitchRules(params)
	if err != nil {
		// Preserve the legacy error message ("rules parameter is required")
		// so existing log scrapers and the failing workflow ID on the
		// server can still grep for it; just append the support for the
		// n8n-UI alias `conditions` to the message.
		return fmt.Errorf("rules parameter is required (or `conditions` alias): %w", err)
	}

	if len(rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	// Validate each rule
	for i, ruleMap := range rules {

		field, ok := ruleMap["field"].(string)
		if !ok || field == "" {
			return fmt.Errorf("rule %d: field is required", i)
		}

		operation, ok := ruleMap["operation"].(string)
		if !ok || operation == "" {
			return fmt.Errorf("rule %d: operation is required", i)
		}

		validOperations := map[string]bool{
			"equal": true, "notEqual": true, "contains": true, "notContains": true,
			"startsWith": true, "endsWith": true, "regex": true,
			"greater": true, "greaterEqual": true, "smaller": true, "smallerEqual": true,
			"isEmpty": true, "isNotEmpty": true,
		}

		if !validOperations[operation] {
			return fmt.Errorf("rule %d: invalid operation %s", i, operation)
		}

		// Value is required for most operations
		if operation != "isEmpty" && operation != "isNotEmpty" {
			if _, ok := ruleMap["value"]; !ok {
				return fmt.Errorf("rule %d: value is required for operation %s", i, operation)
			}
		}
	}

	return nil
}
