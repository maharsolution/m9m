package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSwitchValidateParameters_ConditionsAlias_Accepted covers the bug
// reported against m9m's hosted instance: the user re-restructured a
// Switch node in the n8n UI, which exports the routing rules under the
// key `conditions` (not `rules`). The legacy validator rejected this
// with "rules parameter is required" and the workflow failed before any
// node ran.
//
// The whole point of this test is to pin the contract that BOTH the
// `rules` key and the `conditions` key are accepted and produce the
// same validation outcome.
func TestSwitchValidateParameters_ConditionsAlias_Accepted(t *testing.T) {
	node := NewSwitchNode()

	// Conditions value shape is `[]interface{}` of `map`s (what the
	// generic JSON unmarshal produces when the n8n UI saves the node).
	params := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"field":     "={{$json[\"status\"]}}",
				"operation": "equal",
				"value":     "active",
			},
			map[string]interface{}{
				"field":     "={{$json[\"role\"]}}",
				"operation": "notEqual",
				"value":     "guest",
			},
		},
	}

	err := node.ValidateParameters(params)
	assert.NoError(t, err, "ValidateParameters must accept `conditions` as an alias for `rules`")
}

// TestSwitchValidateParameters_RulesAlias_StillAccepted makes sure the
// fix didn't regress the legacy `rules` key — both keys must continue to
// work for at least one release cycle so we can deprecate `rules` later
// if we want to.
func TestSwitchValidateParameters_RulesAlias_StillAccepted(t *testing.T) {
	node := NewSwitchNode()

	params := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":     "={{$json[\"amount\"]}}",
				"operation": "greater",
				"value":     "100",
			},
		},
	}

	err := node.ValidateParameters(params)
	assert.NoError(t, err, "ValidateParameters must still accept the legacy `rules` key")
}

// TestSwitchValidateParameters_TypedMapSlice_Accepted covers the second
// half of the n8n compatibility gap: typed unmarshallers (or direct map
// literals in code) produce `[]map[string]interface{}` rather than
// `[]interface{}`. The new extractor must handle both shapes.
func TestSwitchValidateParameters_TypedMapSlice_Accepted(t *testing.T) {
	node := NewSwitchNode()

	params := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"field":     "name",
				"operation": "isNotEmpty",
			},
		},
	}

	err := node.ValidateParameters(params)
	assert.NoError(t, err, "typed `[]map[string]interface{}` slice must be accepted under the `conditions` key")
}

// TestSwitchValidateParameters_BothKeysProvided_PreferRules makes the
// disambiguation rule explicit so a future refactor doesn't silently
// pick the wrong one. When both keys are present, legacy `rules` wins
// (it was the original API; `conditions` is the newer alias).
func TestSwitchValidateParameters_BothKeysProvided_PreferRules(t *testing.T) {
	node := NewSwitchNode()

	params := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":     "kind",
				"operation": "equal",
				"value":     "user",
			},
		},
		// This entry is intentionally invalid (no value); if the
		// extractor ever flips to preferring `conditions`, this test
		// catches it.
		"conditions": []interface{}{
			map[string]interface{}{"field": "kind", "operation": "equal"},
		},
	}

	err := node.ValidateParameters(params)
	assert.NoError(t, err, "when both keys are present, `rules` should win and the rule must remain valid")
}

// TestSwitchValidateParameters_NeitherKeyPresent confirms the original
// error is preserved when neither alias is supplied — so the failing
// workflow ID on the hosted instance still surfaces a grep-able error
// for operators.
func TestSwitchValidateParameters_NeitherKeyPresent(t *testing.T) {
	node := NewSwitchNode()

	err := node.ValidateParameters(map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules parameter is required",
		"the legacy error message must remain so existing log scrapers keep working")
}

// TestSwitchValidateParameters_EmptyConditionsArray confirms the same
// "at least one rule" guarantee applies to the alias.
func TestSwitchValidateParameters_EmptyConditionsArray(t *testing.T) {
	node := NewSwitchNode()

	err := node.ValidateParameters(map[string]interface{}{
		"conditions": []interface{}{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one rule is required")
}

// TestExtractSwitchRules_InvalidShape covers the edge cases of the new
// extractor directly, so a regression in either alias or shape can't
// silently fall through and cause an obscure panic later.
func TestExtractSwitchRules_InvalidShape(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
	}{
		{"nil params", nil},
		{"empty params", map[string]interface{}{}},
		{"rules wrong type", map[string]interface{}{"rules": "not-an-array"}},
		{"conditions wrong type", map[string]interface{}{"conditions": 42}},
		{"element not an object", map[string]interface{}{
			"rules": []interface{}{"plain-string"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExtractSwitchRules(tc.params)
			assert.Error(t, err)
		})
	}
}

// TestExtractSwitchRules_HappyPath is the round-trip validation: every
// accepted alias and every accepted shape produces the same canonical
// `[]map[string]interface{}` value.
func TestExtractSwitchRules_HappyPath(t *testing.T) {
	wantRule := map[string]interface{}{"field": "x", "operation": "equal", "value": "1"}

	t.Run("interface slice under rules", func(t *testing.T) {
		out, err := ExtractSwitchRules(map[string]interface{}{
			"rules": []interface{}{wantRule},
		})
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, wantRule, out[0])
	})

	t.Run("interface slice under conditions", func(t *testing.T) {
		out, err := ExtractSwitchRules(map[string]interface{}{
			"conditions": []interface{}{wantRule},
		})
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, wantRule, out[0])
	})

	t.Run("typed map slice under conditions", func(t *testing.T) {
		out, err := ExtractSwitchRules(map[string]interface{}{
			"conditions": []map[string]interface{}{wantRule},
		})
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, wantRule, out[0])
	})
}
