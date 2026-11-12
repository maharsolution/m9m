/*
Package connections — canonical n8n IF-node condition semantics.

n8n's IF node stores its condition table in one of two shapes:

 1. A bare array (older exports, hand-written workflows):

    conditions: [{leftValue, rightValue, operator}, ...]

 2. The wrapper object the current n8n UI writes:

    conditions: {options: {...}, conditions: [...], combinator: "and"}

The operator itself is either a bare string ("equals") or a typed
object ({type, operation, singleValue}) where `type` selects the
comparison domain ("string", "number", "boolean") and `operation`
is the short or long operator name.

This file provides the *canonical* reading of those shapes plus the
comparison semantics n8n applies, so every consumer (the If-retry
loop detector, the engine's retry-guard evaluation, tests) agrees on
one implementation:

  - ResolveIfCombinator — AND / OR resolution from either shape.
  - NormalizeIfParameters — flatten the wrapper into
    {conditions: [...], combinator: "and"|"or"}.
  - ExtractIfConditions — typed condition rows.
  - EvaluateIfConditions — combine the rows with n8n semantics
    (AND: all must hold, OR: any may hold; both short-circuit).
  - CompareIfValues — one comparison, honouring the operator's
    type domain (numeric / string / boolean).

The comparison helpers are deliberately dependency-free (no expression
runtime) so they can be unit-tested in isolation; callers resolve
expressions first and hand the resolved values in.
*/
package connections

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Combinator values accepted by n8n's IF node.
const (
	// IfCombinatorAnd requires every condition to hold.
	IfCombinatorAnd = "and"
	// IfCombinatorOr requires at least one condition to hold.
	IfCombinatorOr = "or"
)

// IfCondition is one row of an n8n IF node's condition table.
//
// Left / Right hold the *raw* operands (literals or expression
// strings as they appear in the workflow JSON). Op is the
// canonical operation name ("equals", "lessThan", "notEmpty", …),
// Type is the operator's comparison domain ("string", "number",
// "boolean" or "" when the operator did not declare one) and
// SingleValue marks the n8n operators that ignore the right operand
// ("exists", "notEmpty", "empty", "true", "false").
type IfCondition struct {
	Left        interface{}
	Right       interface{}
	Op          string
	Type        string
	SingleValue bool
}

// ResolveIfCombinator returns the combinator ("and" / "or") an IF
// node's parameters request.
//
// n8n writes the combinator inside the `conditions` wrapper
// ("conditions.combinator"); older exports and m9m's own metadata
// use the flat "combiner" key. Both are honoured, wrapper first
// (it is the authoritative location in current exports), then the
// flat keys. The default is "and", matching n8n.
func ResolveIfCombinator(params map[string]interface{}) string {
	if params == nil {
		return IfCombinatorAnd
	}
	if wrap, ok := params["conditions"].(map[string]interface{}); ok {
		if c, ok := wrap["combinator"].(string); ok && c != "" {
			return NormalizeIfCombinator(c)
		}
		if c, ok := wrap["combiner"].(string); ok && c != "" {
			return NormalizeIfCombinator(c)
		}
	}
	for _, key := range []string{"combinator", "combiner"} {
		if c, ok := params[key].(string); ok && c != "" {
			return NormalizeIfCombinator(c)
		}
	}
	return IfCombinatorAnd
}

// NormalizeIfCombinator maps the spellings n8n uses onto the two
// canonical combinators. Unknown values fall back to "and".
func NormalizeIfCombinator(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case IfCombinatorOr, "any", "one":
		return IfCombinatorOr
	case IfCombinatorAnd, "all":
		return IfCombinatorAnd
	}
	return IfCombinatorAnd
}

// NormalizeIfParameters returns a copy of `params` with the IF
// node's condition table flattened into the canonical shape:
//
//	conditions: [ ...rows... ]
//	combinator: "and" | "or"
//
// The input map is never mutated — callers pass `node.Parameters`
// straight from the workflow store.
func NormalizeIfParameters(params map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(params)+2)
	for k, v := range params {
		out[k] = v
	}
	if rows, ok := ifConditionsArray(params); ok {
		out["conditions"] = rows
	}
	combinator := ResolveIfCombinator(params)
	out["combinator"] = combinator
	out["combiner"] = combinator
	return out
}

// ExtractIfConditions reads the condition rows out of an IF node's
// parameters. Both n8n shapes are accepted:
//
//	conditions: [{leftValue, rightValue, operator}, ...]
//	conditions: {options: {...}, conditions: [...], combinator: "or"}
//
// Each row is returned with its operator normalised to the canonical
// operation name and the operator's type domain preserved so
// comparisons can be type-aware.
func ExtractIfConditions(params map[string]interface{}) ([]IfCondition, error) {
	rows, ok := ifConditionsArray(params)
	if !ok {
		return nil, fmt.Errorf("conditions must be an array")
	}
	out := make([]IfCondition, 0, len(rows))
	for i, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("condition %d must be an object", i)
		}
		if _, ok := row["leftValue"]; !ok {
			return nil, fmt.Errorf("condition %d missing 'leftValue' field", i)
		}
		if _, ok := row["rightValue"]; !ok {
			return nil, fmt.Errorf("condition %d missing 'rightValue' field", i)
		}
		rawOp, ok := row["operator"]
		if !ok {
			return nil, fmt.Errorf("condition %d missing 'operator' field", i)
		}
		op, typ, single, err := parseIfOperator(rawOp)
		if err != nil {
			return nil, fmt.Errorf("condition %d: %w", i, err)
		}
		out = append(out, IfCondition{
			Left:        row["leftValue"],
			Right:       row["rightValue"],
			Op:          op,
			Type:        typ,
			SingleValue: single,
		})
	}
	return out, nil
}

// EvaluateIfConditions combines the conditions the way n8n's IF node
// does:
//
//   - "and": every condition must hold (short-circuits on the first
//     false).
//   - "or": at least one condition must hold (short-circuits on the
//     first true).
//
// `resolve` maps a raw operand to its evaluated value (expressions
// are resolved by the caller); a nil resolver means "use the operands
// as they are". An empty condition set is true, matching n8n.
func EvaluateIfConditions(conditions []IfCondition, combinator string, resolve func(interface{}) interface{}) bool {
	if len(conditions) == 0 {
		return true
	}
	if resolve == nil {
		resolve = func(v interface{}) interface{} { return v }
	}
	isOr := NormalizeIfCombinator(combinator) == IfCombinatorOr
	for _, condition := range conditions {
		left := resolve(condition.Left)
		right := resolve(condition.Right)
		holds := CompareIfValues(left, right, condition.Op, condition.Type)
		if isOr {
			if holds {
				return true
			}
			continue
		}
		if !holds {
			return false
		}
	}
	return !isOr
}

// CompareIfValues applies one IF condition to two already-resolved
// values.
//
// `op` is the canonical operation name (see CanonicalIfOperatorName)
// and `typ` is the operator's comparison domain:
//
//   - "number": both operands are parsed as numbers; a non-numeric
//     operand makes the comparison false.
//   - "string": both operands are compared as strings.
//   - "boolean": both operands are parsed as booleans.
//   - "" / "any": numeric comparison when both operands are numeric,
//     string comparison otherwise.
//
// The single-value operators ("exists", "notExists", "empty",
// "notEmpty", "true", "false") ignore the right operand, exactly
// like n8n's `singleValue: true` operators.
func CompareIfValues(left, right interface{}, op, typ string) bool {
	switch CanonicalIfOperatorName(op) {
	case "exists":
		return left != nil
	case "notExists":
		return left == nil
	case "empty":
		return ifValueIsEmpty(left)
	case "notEmpty":
		return !ifValueIsEmpty(left)
	case "true":
		return ifValueIsTruthy(left)
	case "false":
		return !ifValueIsTruthy(left)
	}

	if left == nil || right == nil {
		return false
	}

	switch CanonicalIfOperatorName(op) {
	case "equals":
		return ifValuesEqual(left, right, typ)
	case "notEquals":
		return !ifValuesEqual(left, right, typ)
	case "contains":
		return strings.Contains(ifValueToString(left), ifValueToString(right))
	case "notContains":
		return !strings.Contains(ifValueToString(left), ifValueToString(right))
	case "startsWith":
		return strings.HasPrefix(ifValueToString(left), ifValueToString(right))
	case "endsWith":
		return strings.HasSuffix(ifValueToString(left), ifValueToString(right))
	case "regex":
		match, err := regexp.MatchString(ifValueToString(right), ifValueToString(left))
		return err == nil && match
	case "greaterThan":
		return ifCompareOrdered(left, right, typ, func(c int) bool { return c > 0 })
	case "lessThan":
		return ifCompareOrdered(left, right, typ, func(c int) bool { return c < 0 })
	case "greaterThanOrEqual":
		return ifCompareOrdered(left, right, typ, func(c int) bool { return c >= 0 })
	case "lessThanOrEqual":
		return ifCompareOrdered(left, right, typ, func(c int) bool { return c <= 0 })
	case "between":
		return ifValueBetween(left, right)
	}
	return false
}

// CanonicalIfOperatorName maps n8n's short operator names onto the
// long form used internally ("lt" → "lessThan", "eq" → "equals", …).
// Unknown names are returned unchanged.
func CanonicalIfOperatorName(op string) string {
	switch strings.TrimSpace(op) {
	case "lt":
		return "lessThan"
	case "lte", "lteq":
		return "lessThanOrEqual"
	case "gt":
		return "greaterThan"
	case "gte", "gteq":
		return "greaterThanOrEqual"
	case "eq":
		return "equals"
	case "neq", "ne":
		return "notEquals"
	}
	return op
}

// ifConditionsArray unwraps the two accepted `conditions` shapes and
// returns the inner rows.
func ifConditionsArray(params map[string]interface{}) ([]interface{}, bool) {
	if params == nil {
		return nil, false
	}
	switch v := params["conditions"].(type) {
	case []interface{}:
		return v, true
	case map[string]interface{}:
		rows, ok := v["conditions"].([]interface{})
		return rows, ok
	}
	return nil, false
}

// parseIfOperator reads n8n's operator shape — either a bare string
// or a typed object `{type, operation, singleValue}` — and returns
// the canonical operation name, the comparison domain and the
// single-value flag.
func parseIfOperator(raw interface{}) (op string, typ string, single bool, err error) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", "", false, fmt.Errorf("operator must not be empty")
		}
		return CanonicalIfOperatorName(v), "", false, nil
	case map[string]interface{}:
		name, ok := v["operation"].(string)
		if !ok || name == "" {
			return "", "", false, fmt.Errorf("operator object missing 'operation' field")
		}
		if t, ok := v["type"].(string); ok {
			typ = strings.ToLower(strings.TrimSpace(t))
		}
		if s, ok := v["singleValue"].(bool); ok {
			single = s
		}
		return CanonicalIfOperatorName(name), typ, single, nil
	default:
		return "", "", false, fmt.Errorf("operator must be a string or object")
	}
}

// ifValueToString renders a value the way n8n renders it inside a
// comparison: strings stay as they are, everything else uses its
// default Go formatting.
func ifValueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return fmt.Sprintf("%v", value)
}

// ifValueToFloat64 parses a value as a number. Strings are parsed
// (n8n coerces "10" to 10), booleans are 1/0.
func ifValueToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// ifValueToBool parses a value as a boolean the way n8n's boolean
// operators do: real booleans, the strings "true"/"false" and
// numbers (0 = false).
func ifValueToBool(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no", "":
			return false, true
		}
		return false, false
	}
	if f, ok := ifValueToFloat64(value); ok {
		return f != 0, true
	}
	return false, false
}

// ifValuesEqual compares two values in the operator's domain.
func ifValuesEqual(left, right interface{}, typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "number", "numeric":
		l, lok := ifValueToFloat64(left)
		r, rok := ifValueToFloat64(right)
		return lok && rok && l == r
	case "boolean", "bool":
		l, lok := ifValueToBool(left)
		r, rok := ifValueToBool(right)
		return lok && rok && l == r
	case "string", "text":
		return ifValueToString(left) == ifValueToString(right)
	}
	// Untyped ("any"): numeric when both sides are numbers,
	// string comparison otherwise — this is n8n's loose mode.
	l, lok := ifValueToFloat64(left)
	r, rok := ifValueToFloat64(right)
	if lok && rok {
		return l == r
	}
	return ifValueToString(left) == ifValueToString(right)
}

// ifCompareOrdered orders two values and reports the comparison
// through `cmp` (-1, 0, 1). Numeric domains compare numerically;
// string domains compare lexicographically.
func ifCompareOrdered(left, right interface{}, typ string, cmp func(int) bool) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "number", "numeric":
		l, lok := ifValueToFloat64(left)
		r, rok := ifValueToFloat64(right)
		if !lok || !rok {
			return false
		}
		return cmp(ifCompareFloat(l, r))
	case "string", "text":
		return cmp(strings.Compare(ifValueToString(left), ifValueToString(right)))
	}
	l, lok := ifValueToFloat64(left)
	r, rok := ifValueToFloat64(right)
	if lok && rok {
		return cmp(ifCompareFloat(l, r))
	}
	return cmp(strings.Compare(ifValueToString(left), ifValueToString(right)))
}

// ifCompareFloat returns -1, 0 or 1 for the two floats.
func ifCompareFloat(l, r float64) int {
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
	}
}

// ifValueBetween implements n8n's `between` operator: the right
// operand carries "min,max".
func ifValueBetween(left, right interface{}) bool {
	parts := strings.Split(ifValueToString(right), ",")
	if len(parts) != 2 {
		return false
	}
	min, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return false
	}
	max, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return false
	}
	value, ok := ifValueToFloat64(left)
	if !ok {
		return false
	}
	return value >= min && value <= max
}

// ifValueIsEmpty reports whether a value is "empty" in n8n's sense:
// nil, the empty string, an empty collection or zero.
func ifValueIsEmpty(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []interface{}:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case bool:
		return !v
	case float64:
		return v == 0
	case float32:
		return v == 0
	case int:
		return v == 0
	case int64:
		return v == 0
	}
	return false
}

// ifValueIsTruthy reports whether a value is truthy in n8n's sense:
// nil, "", empty collections, zero and false are falsy.
func ifValueIsTruthy(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != "" && !strings.EqualFold(v, "false")
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	}
	return true
}
