package data

import (
	"fmt"
	"reflect"
	"strings"
)

// reflectEqual compares two values for "equals" semantics. n8n's
// dataTable treats `123` (number) and `"123"` (string) as equal
// for this operator; we do the same via a string-coercion
// fallback when the strict types don't match.
func reflectEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if reflect.DeepEqual(a, b) {
		return true
	}
	// Coerce numerics across int/float/json-number boundaries.
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return af == bf
		}
	}
	// Coerce bool ↔ string ("true"/"false").
	if ab, ok := toBool(a); ok {
		if bb, ok := toBool(b); ok {
			return ab == bb
		}
	}
	// Last resort: string-coerce both sides.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// reflectContains is the `contains` operator. For strings it's
// a substring match; for arrays it's a membership check; for
// maps it's a key-membership check.
func reflectContains(haystack, needle interface{}) bool {
	if haystack == nil {
		return false
	}
	switch t := haystack.(type) {
	case string:
		if ns, ok := needle.(string); ok {
			return strings.Contains(t, ns)
		}
		return strings.Contains(t, fmt.Sprintf("%v", needle))
	case []interface{}:
		for _, v := range t {
			if reflectEqual(v, needle) {
				return true
			}
		}
	case map[string]interface{}:
		if ns, ok := needle.(string); ok {
			_, has := t[ns]
			return has
		}
	}
	return false
}

// reflectCompare returns -1, 0, +1 (or 0 if incomparable). Used
// for `greaterThan` / `lessThan`.
func reflectCompare(a, b interface{}) int {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

func reflectBoolTrue(v interface{}) bool {
	b, _ := toBool(v)
	return b
}

func reflectBoolFalse(v interface{}) bool {
	b, ok := toBool(v)
	if !ok {
		// A non-boolean (e.g. an empty string, a missing
		// key) is not `true`; for `isFalse` we want it to
		// match. This matches n8n's behaviour where the
		// boolean value is what matters, not its type.
		return true
	}
	return !b
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		var f float64
		_, err := fmt.Sscanf(t, "%g", &f)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func toBool(v interface{}) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off", "":
			return false, true
		}
	}
	return false, false
}
