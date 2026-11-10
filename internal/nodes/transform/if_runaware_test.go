package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIfNode_ExecuteWithRun_ResolvesSiblingNode ensures the Finnet
// If-retry guard `$('Set Counter').item.json.attempt < maxAttempts`
// evaluates true when Set Counter is present in runExecutionData.
func TestIfNode_ExecuteWithRun_ResolvesSiblingNode(t *testing.T) {
	node := NewIfNode()
	params := map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "and",
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
					"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
					"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
				},
			},
		},
	}

	runData := &expressions.RunExecutionData{
		ResultData: &expressions.RunData{
			NodeData: map[string][]expressions.NodeExecutionResult{
				"Set Counter": {{
					Data: []model.DataItem{{
						JSON: map[string]interface{}{"attempt": float64(3), "maxAttempts": float64(10)},
					}},
				}},
			},
		},
	}
	wf := &model.Workflow{Name: "Finnet"}
	in := []model.DataItem{{JSON: map[string]interface{}{"respond_type": ""}}}

	out, err := node.ExecuteWithRun(wf, runData, 0, 0, in, params)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, true, out[0].JSON["_ifResult"],
		"attempt 3 < maxAttempts 10 must be true when RunExecutionData is wired")

	// Without run data the same expression collapses to nil/nil → false.
	out2, err := node.Execute(in, params)
	require.NoError(t, err)
	require.Len(t, out2, 1)
	assert.Equal(t, false, out2[0].JSON["_ifResult"],
		"plain Execute cannot resolve $('Set Counter') and must not spuriously continue")
}
