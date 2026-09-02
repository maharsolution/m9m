package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfNode_Execute(t *testing.T) {
	node := NewIfNode()

	items := []model.DataItem{
		{JSON: map[string]interface{}{"name": "Alice", "age": float64(30)}},
		{JSON: map[string]interface{}{"name": "Bob", "age": float64(17)}},
		{JSON: map[string]interface{}{"name": "Charlie", "age": float64(25)}},
	}

	t.Run("filters items matching condition", func(t *testing.T) {
		params := map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "$json.age",
					"operator":   "greaterThanOrEqual",
					"rightValue": float64(18),
				},
			},
			"combiner": "and",
		}

		result, err := node.Execute(items, params)
		require.NoError(t, err)
		// n8n IF always emits both branches (PARITY_REPORT §6 Gap #1
		// fix): the engine router splits true/false across the
		// outgoing `main` connections. So the IF node itself must
		// return every input item, tagged with `_ifResult`.
		assert.Len(t, result, 3)
		// Filter to the true branch for the assertion below.
		var trueItems []model.DataItem
		for _, item := range result {
			if item.JSON["_ifResult"] == true {
				trueItems = append(trueItems, item)
			}
		}
		assert.Len(t, trueItems, 2)
		assert.Equal(t, "Alice", trueItems[0].JSON["name"])
		assert.Equal(t, "Charlie", trueItems[1].JSON["name"])
	})

	t.Run("returns both branches when requested", func(t *testing.T) {
		params := map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "$json.age",
					"operator":   "greaterThanOrEqual",
					"rightValue": float64(18),
				},
			},
			"combiner":           "and",
			"returnBothBranches": true,
		}

		result, err := node.Execute(items, params)
		require.NoError(t, err)
		assert.Len(t, result, 3)

		trueCount := 0
		falseCount := 0
		for _, item := range result {
			if item.JSON["_ifResult"] == true {
				trueCount++
			} else {
				falseCount++
			}
		}
		assert.Equal(t, 2, trueCount)
		assert.Equal(t, 1, falseCount)
	})

	t.Run("empty input", func(t *testing.T) {
		params := map[string]interface{}{
			"conditions": []interface{}{},
			"combiner":   "and",
		}
		result, err := node.Execute(nil, params)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("or combiner", func(t *testing.T) {
		params := map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "$json.name",
					"operator":   "equals",
					"rightValue": "Alice",
				},
				map[string]interface{}{
					"leftValue":  "$json.name",
					"operator":   "equals",
					"rightValue": "Bob",
				},
			},
			"combiner": "or",
		}

		result, err := node.Execute(items, params)
		require.NoError(t, err)
		// IF emits both branches: 2 true (Alice, Bob) + 1 false (Charlie).
		assert.Len(t, result, 3)
		var trueCount int
		for _, item := range result {
			if item.JSON["_ifResult"] == true {
				trueCount++
			}
		}
		assert.Equal(t, 2, trueCount, "Alice and Bob match the OR, Charlie doesn't")
	})

	t.Run("missing conditions", func(t *testing.T) {
		_, err := node.Execute(items, map[string]interface{}{})
		assert.Error(t, err)
	})

	t.Run("string equals condition", func(t *testing.T) {
		params := map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "$json.name",
					"operator":   "equals",
					"rightValue": "Bob",
				},
			},
			"combiner": "and",
		}

		result, err := node.Execute(items, params)
		require.NoError(t, err)
		// IF emits both branches: 1 true (Bob) + 2 false (Alice, Charlie).
		assert.Len(t, result, 3)
		var trueItems []model.DataItem
		for _, item := range result {
			if item.JSON["_ifResult"] == true {
				trueItems = append(trueItems, item)
			}
		}
		assert.Len(t, trueItems, 1)
		assert.Equal(t, "Bob", trueItems[0].JSON["name"])
	})
}

func TestIfNode_ValidateParameters(t *testing.T) {
	node := NewIfNode()

	assert.Error(t, node.ValidateParameters(nil))
	assert.Error(t, node.ValidateParameters(map[string]interface{}{}))
	assert.NoError(t, node.ValidateParameters(map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"leftValue":  "$json.name",
				"operator":   "equals",
				"rightValue": "test",
			},
		},
		"combiner": "and",
	}))
}

func TestIfNode_Description(t *testing.T) {
	node := NewIfNode()
	desc := node.Description()
	assert.Equal(t, "IF", desc.Name)
}

// TestIfNode_ExecuteReturnsBothBranchesTagged verifies the fix for
// PARITY_REPORT §6 Gap #1: the IF node must emit every input item
// (true AND false) with an `_ifResult` boolean tag so the engine
// router can split items across connections.Main[0] (true) and
// connections.Main[1] (false). Previously the false branch was
// silently dropped because the implementation returned only trueItems.
//
// The test asserts:
//   - the count of returned items equals the count of input items;
//   - every returned item carries an `_ifResult` boolean field;
//   - items from both branches appear in the output;
//   - items from neither branch are absent (i.e. there are exactly
//     the inputs we started with, no extras).
func TestIfNode_ExecuteReturnsBothBranchesTagged(t *testing.T) {
	node := NewIfNode()

	items := []model.DataItem{
		{JSON: map[string]interface{}{"name": "Alice", "age": float64(30)}},
		{JSON: map[string]interface{}{"name": "Bob", "age": float64(17)}},
		{JSON: map[string]interface{}{"name": "Charlie", "age": float64(25)}},
	}

	params := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"leftValue":  "$json.age",
				"operator":   "greaterThanOrEqual",
				"rightValue": float64(18),
			},
		},
		"combiner": "and",
	}

	result, err := node.Execute(items, params)
	require.NoError(t, err)

	// Must produce every input item, not just the true ones.
	require.Len(t, result, 3, "IF must emit all input items tagged with _ifResult (PARITY Gap #1)")

	var trueCount, falseCount int
	for _, item := range result {
		tag, ok := item.JSON["_ifResult"].(bool)
		require.True(t, ok, "every IF output item must carry a bool _ifResult tag")
		if tag {
			trueCount++
		} else {
			falseCount++
		}
	}
	assert.Equal(t, 2, trueCount, "Alice and Charlie should pass the age>=18 check")
	assert.Equal(t, 1, falseCount, "Bob (age 17) should be in the false branch")
}
