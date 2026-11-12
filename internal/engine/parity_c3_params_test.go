package engine

import (
	"testing"
)

// parityC3NestedValue reads a dotted path out of a parameter map.
func parityC3NestedValue(params map[string]interface{}, path ...string) (interface{}, bool) {
	var current interface{} = params
	for _, segment := range path {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// TestParityC3FlatAndNestedParamsAreEquivalent pins the C3 parameter
// contract: a parameter saved with a literal dotted key ("a.b") and the
// same parameter saved in the nested n8n wire shape ({"a": {"b": ...}})
// must be readable through BOTH accessors after normalisation, and both
// shapes must carry the same value.
//
// This is a read-only contract test; no runtime file is modified.
func TestParityC3FlatAndNestedParamsAreEquivalent(t *testing.T) {
	flat := map[string]interface{}{
		"response.includeResponseHeadersAndStatus": true,
		"options.timeout":                          30,
	}
	nested := map[string]interface{}{
		"response": map[string]interface{}{
			"includeResponseHeadersAndStatus": true,
		},
		"options": map[string]interface{}{
			"timeout": 30,
		},
	}

	flatNormalized := NormalizeNodeParameters(flat)
	nestedNormalized := NormalizeNodeParameters(nested)

	t.Run("flat_input_exposes_nested_path", func(t *testing.T) {
		value, ok := parityC3NestedValue(flatNormalized, "response", "includeResponseHeadersAndStatus")
		if !ok {
			t.Fatalf("normalised flat params do not expose response.includeResponseHeadersAndStatus: %#v", flatNormalized)
		}
		if value != true {
			t.Fatalf("nested value = %v, want true", value)
		}
		if flatNormalized["response.includeResponseHeadersAndStatus"] != true {
			t.Fatalf("flat alias lost after normalisation: %#v", flatNormalized)
		}
	})

	t.Run("nested_input_exposes_flat_alias", func(t *testing.T) {
		value, ok := nestedNormalized["options.timeout"]
		if !ok {
			t.Fatalf("normalised nested params do not expose options.timeout: %#v", nestedNormalized)
		}
		if value != 30 {
			t.Fatalf("flat alias = %v, want 30", value)
		}
		nestedValue, ok := parityC3NestedValue(nestedNormalized, "options", "timeout")
		if !ok {
			t.Fatalf("normalised nested params lost the nested path: %#v", nestedNormalized)
		}
		if nestedValue != 30 {
			t.Fatalf("nested value = %v, want 30", nestedValue)
		}
	})

	t.Run("both_shapes_agree", func(t *testing.T) {
		for _, path := range [][]string{
			{"response", "includeResponseHeadersAndStatus"},
			{"options", "timeout"},
		} {
			flatKey := path[0]
			for _, segment := range path[1:] {
				flatKey += "." + segment
			}
			left, leftOK := flatNormalized[flatKey]
			right, rightOK := nestedNormalized[flatKey]
			if !leftOK || !rightOK {
				t.Fatalf("flat alias %q missing: flat=%v nested=%v", flatKey, leftOK, rightOK)
			}
			if left != right {
				t.Fatalf("flat alias %q differs between shapes: %v vs %v", flatKey, left, right)
			}
			leftNested, _ := parityC3NestedValue(flatNormalized, path...)
			rightNested, _ := parityC3NestedValue(nestedNormalized, path...)
			if leftNested != rightNested {
				t.Fatalf("nested value for %q differs between shapes: %v vs %v", flatKey, leftNested, rightNested)
			}
		}
	})

	t.Run("normalisation_does_not_mutate_caller", func(t *testing.T) {
		original := map[string]interface{}{"a.b": 1}
		NormalizeNodeParameters(original)
		if len(original) != 1 {
			t.Fatalf("caller map was mutated: %#v", original)
		}
		if _, ok := original["a"]; ok {
			t.Fatalf("caller map gained a nested key: %#v", original)
		}
	})
}
