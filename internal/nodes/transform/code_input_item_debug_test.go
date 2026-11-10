package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

// TestSetCounter_InputItemProperty reproduces the Finnet Set Counter
// snippet that requires `$input.item` to be a property (not a function).
func TestSetCounter_InputItemProperty(t *testing.T) {
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
  return { typeofItem: typeofItem, hasJson: hasJson };
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
	if probeErr != nil {
		t.Fatalf("probe: %v", probeErr)
	}
	probeMap, ok := probeResult.(map[string]interface{})
	if !ok {
		t.Fatalf("probe result type %T", probeResult)
	}
	if probeMap["typeofItem"] != "object" {
		t.Fatalf("$input.item typeof: want object, got %v", probeMap["typeofItem"])
	}
	if probeMap["hasJson"] != true {
		t.Fatalf("$input.item.json missing: %+v", probeMap)
	}

	setCounter := `let currentAttempt = $input.item.json.attempt || 0;
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
	if err != nil {
		t.Fatalf("Set Counter should succeed after $input.item property fix: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out))
	}
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
