package transform

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

func agentDebugLog(hypothesisID, location, message string, data map[string]interface{}) {
	f, err := os.OpenFile(`c:\Users\mahar\OneDrive\workspace\m9m\debug-31457b.log`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]interface{}{
		"sessionId":    "31457b",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
		"runId":        "pre-fix",
	}
	b, _ := json.Marshal(payload)
	_, _ = f.Write(append(b, '\n'))
}

// TestDebug_SetCounter_InputItemShape reproduces the Finnet Set Counter
// snippet that fails live on /webhook/post_balance and logs the shape of
// $input.item so we can confirm whether it is a property or a function.
func TestDebug_SetCounter_InputItemShape(t *testing.T) {
	input := []model.DataItem{{
		JSON: map[string]interface{}{
			"key":  "exec-test",
			"data": map[string]interface{}{"id": "U001"},
		},
	}}

	probe := `(function(){
  var itemRef = $input.item;
  var typeofItem = typeof itemRef;
  var hasJson = !!(itemRef && itemRef.json);
  var attemptVal = (itemRef && itemRef.json) ? itemRef.json.attempt : undefined;
  return { typeofItem: typeofItem, hasJson: hasJson, attemptVal: attemptVal, itemIndexKeys: Object.keys(itemRef || {}) };
})();`

	evaluator := expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig())
	ctx := &expressions.ExpressionContext{
		RunIndex:            0,
		ItemIndex:           0,
		ActiveNodeName:      "Set Counter",
		ConnectionInputData: input,
		Mode:                expressions.ModeManual,
		AdditionalKeys: &expressions.AdditionalKeys{
			CurrentNodeParameters: map[string]interface{}{"codeNodeInputItems": true},
		},
	}
	probeResult, probeErr := evaluator.EvaluateCode(probe, ctx)
	agentDebugLog("H1", "code_input_item_debug_test.go:probe", "$input.item shape probe", map[string]interface{}{
		"probeResult": fmt.Sprintf("%v", probeResult),
		"probeErr":    fmt.Sprintf("%v", probeErr),
	})

	setCounter := `// Mengambil nilai attempt dari item yang masuk saat ini, default ke 0 jika belum ada
let currentAttempt = $input.item.json.attempt || 0;

// Tambahkan 1 setiap kali melewati node ini
currentAttempt += 1;

return {
  json: {
    attempt: currentAttempt,
    maxAttempts: 10
  }
};`

	node := NewCodeNode()
	out, err := node.Execute(input, map[string]interface{}{
		"jsCode": setCounter,
	})
	agentDebugLog("H1", "code_input_item_debug_test.go:setCounter", "Set Counter Execute result", map[string]interface{}{
		"err":     fmt.Sprintf("%v", err),
		"outLen":  len(out),
		"outJSON": fmt.Sprintf("%v", func() interface{} {
			if len(out) == 0 {
				return nil
			}
			return out[0].JSON
		}()),
	})

	if err != nil {
		t.Fatalf("Set Counter should succeed after $input.item property fix: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out))
	}
	if got, _ := out[0].JSON["attempt"].(int64); got != 1 {
		// goja may decode numbers as int/float64
		switch v := out[0].JSON["attempt"].(type) {
		case int:
			if v != 1 {
				t.Fatalf("attempt: want 1, got %d", v)
			}
		case int64:
			if v != 1 {
				t.Fatalf("attempt: want 1, got %d", v)
			}
		case float64:
			if v != 1 {
				t.Fatalf("attempt: want 1, got %v", v)
			}
		default:
			t.Fatalf("attempt: unexpected type %T value %v", out[0].JSON["attempt"], out[0].JSON["attempt"])
		}
	}
	t.Logf("Set Counter succeeded: %+v", out[0].JSON)
}
