package engine

import (
	"context"
	"testing"
	"time"

	"github.com/neul-labs/m9m/internal/connections"
	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/transform"
	"github.com/neul-labs/m9m/internal/nodes/trigger"
)

// pollMongo is a fake `n8n-nodes-base.mongoDb` node. It records the
// attempt value the loop published for the current iteration and
// either misses (empty result) or hits, depending on hitsAfter.
type pollMongo struct {
	*base.BaseNode
	calls     int
	hitsAfter int
	attempts  []int
}

func (p *pollMongo) Execute(input []model.DataItem, params map[string]interface{}) ([]model.DataItem, error) {
	return p.ExecuteWithRun(nil, nil, 0, 0, input, params)
}

func (p *pollMongo) ExecuteWithRun(wf *model.Workflow, runData *expressions.RunExecutionData, runIndex, itemIndex int, input []model.DataItem, params map[string]interface{}) ([]model.DataItem, error) {
	attempt := 0
	if runData != nil && runData.ResultData != nil {
		if slots := runData.ResultData.NodeData["Set Counter"]; runIndex < len(slots) && len(slots[runIndex].Data) > 0 {
			if v, ok := slots[runIndex].Data[0].JSON["attempt"].(int64); ok {
				attempt = int(v)
			} else if v, ok := slots[runIndex].Data[0].JSON["attempt"].(float64); ok {
				attempt = int(v)
			} else if v, ok := slots[runIndex].Data[0].JSON["attempt"].(int); ok {
				attempt = v
			}
		}
	}
	p.attempts = append(p.attempts, attempt)
	p.calls++
	if p.hitsAfter > 0 && p.calls >= p.hitsAfter {
		return []model.DataItem{{JSON: map[string]interface{}{
			"key":          "k",
			"respond":      "<balance>1</balance>",
			"respond_type": "xml",
		}}}, nil
	}
	return []model.DataItem{}, nil
}

func (p *pollMongo) ValidateParameters(map[string]interface{}) error { return nil }

type noOpNode struct{ *base.BaseNode }

func (n *noOpNode) Execute(input []model.DataItem, _ map[string]interface{}) ([]model.DataItem, error) {
	return input, nil
}
func (n *noOpNode) ValidateParameters(map[string]interface{}) error { return nil }

// retryLoopWorkflow builds a Finnet-shaped retry loop:
//
//	Async Subworkflow → Set Counter → Find Respond → Value →
//	Check Key Exists1 → (false) If (guard) → (true) Find Respond
//
// `counterCode` is the Set Counter snippet, `guardParams` the guard's
// condition table.
func retryLoopWorkflow(counterCode string, guardParams map[string]interface{}) *model.Workflow {
	return &model.Workflow{
		ID: "retry-loop",
		Nodes: []model.Node{
			{Name: "Async Subworkflow", Type: "n8n-nodes-base.executeWorkflowTrigger"},
			{Name: "Set Counter", Type: "n8n-nodes-base.code", Parameters: map[string]interface{}{
				"jsCode": counterCode,
			}},
			{Name: "Find Respond", Type: "n8n-nodes-base.mongoDb", Parameters: map[string]interface{}{
				"collection": "finnet_store",
				"query":      `={ "key": "{{$('Async Subworkflow').item.json.key}}" }`,
			}},
			{Name: "Value", Type: "n8n-nodes-base.set", Parameters: map[string]interface{}{
				"assignments": map[string]interface{}{"assignments": []interface{}{
					map[string]interface{}{"name": "value", "type": "string", "value": "={{ $json.respond }}"},
					map[string]interface{}{"name": "respond_type", "type": "string", "value": "={{ $json.respond_type }}"},
				}},
			}},
			{Name: "Check Key Exists1", Type: "n8n-nodes-base.if", Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "and",
					"conditions": []interface{}{map[string]interface{}{
						"leftValue":  "={{ $json.respond_type }}",
						"rightValue": "",
						"operator":   map[string]interface{}{"operation": "notEmpty", "type": "string", "singleValue": true},
					}},
				},
			}},
			{Name: "If", Type: "n8n-nodes-base.if", Parameters: guardParams},
			{Name: "Result", Type: "n8n-nodes-base.noOp"},
		},
		Connections: map[string]model.Connections{
			"Async Subworkflow": {Main: [][]model.Connection{{{Node: "Set Counter", Type: "main", Index: 0}}}},
			"Set Counter":       {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}}},
			"Find Respond":      {Main: [][]model.Connection{{{Node: "Value", Type: "main", Index: 0}}}},
			"Value":             {Main: [][]model.Connection{{{Node: "Check Key Exists1", Type: "main", Index: 0}}}},
			"Check Key Exists1": {Main: [][]model.Connection{{{Node: "Result", Type: "main", Index: 0}}, {{Node: "If", Type: "main", Index: 0}}}},
			"If":                {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}, {{Node: "Result", Type: "main", Index: 0}}}},
		},
	}
}

// defaultGuardParams is the Finnet guard: `attempt < maxAttempts`.
func defaultGuardParams() map[string]interface{} {
	return map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "and",
			"conditions": []interface{}{map[string]interface{}{
				"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
				"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
				"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
			}},
		},
	}
}

// newRetryLoopEngine wires the fake Mongo node into a fresh engine.
func newRetryLoopEngine(mongo *pollMongo) WorkflowEngine {
	eng := NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.executeWorkflowTrigger", trigger.NewExecuteWorkflowTriggerNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.code", transform.NewCodeNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.set", transform.NewSetNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.if", transform.NewIfNode())
	eng.RegisterNodeExecutor("n8n-nodes-base.mongoDb", mongo)
	eng.RegisterNodeExecutor("n8n-nodes-base.noOp", &noOpNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "NoOp"})})
	return eng
}

func runRetryWorkflow(t *testing.T, eng WorkflowEngine, wf *model.Workflow) {
	t.Helper()
	input := []model.DataItem{{JSON: map[string]interface{}{"key": "exec_key"}}}
	ctx := WithSubWorkflowStart(context.Background(), 1)
	res, err := eng.(ContextWorkflowEngine).ExecuteWorkflowWithContext(ctx, wf, input)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res != nil && res.Error != nil {
		t.Fatalf("result.Error: %v", res.Error)
	}
}

// TestRetryLoopPollInterval_Resolution covers the precedence rules:
// test override > workflow setting > environment > default.
func TestRetryLoopPollInterval_Resolution(t *testing.T) {
	guard := &model.Node{Name: "If", Type: "n8n-nodes-base.if"}
	wf := &model.Workflow{ID: "wf"}

	t.Setenv(RetryLoopPollIntervalEnv, "")
	if got := resolveRetryLoopPollInterval(wf, guard, connections.IfRetryLoop{}); got != DefaultRetryLoopPollInterval {
		t.Fatalf("default interval: want %s, got %s", DefaultRetryLoopPollInterval, got)
	}

	t.Setenv(RetryLoopPollIntervalEnv, "50ms")
	if got := resolveRetryLoopPollInterval(wf, guard, connections.IfRetryLoop{}); got != 50*time.Millisecond {
		t.Fatalf("env interval: want 50ms, got %s", got)
	}

	// Clamped to [10ms, 5s].
	t.Setenv(RetryLoopPollIntervalEnv, "1ms")
	if got := resolveRetryLoopPollInterval(wf, guard, connections.IfRetryLoop{}); got != MinRetryLoopPollInterval {
		t.Fatalf("env floor: want %s, got %s", MinRetryLoopPollInterval, got)
	}
	t.Setenv(RetryLoopPollIntervalEnv, "30s")
	if got := resolveRetryLoopPollInterval(wf, guard, connections.IfRetryLoop{}); got != MaxRetryLoopPollInterval {
		t.Fatalf("env ceiling: want %s, got %s", MaxRetryLoopPollInterval, got)
	}

	// An explicit workflow setting wins over the environment.
	guardWithSetting := &model.Node{
		Name: "If",
		Type: "n8n-nodes-base.if",
		Parameters: map[string]interface{}{
			"options": map[string]interface{}{"waitBetweenTries": "750ms"},
		},
	}
	if got := resolveRetryLoopPollInterval(wf, guardWithSetting, connections.IfRetryLoop{}); got != 750*time.Millisecond {
		t.Fatalf("workflow interval: want 750ms, got %s", got)
	}

	// The test override wins over everything.
	SetRetryLoopPollIntervalForTesting(3 * time.Millisecond)
	defer ResetRetryLoopPollIntervalForTesting()
	if got := resolveRetryLoopPollInterval(wf, guardWithSetting, connections.IfRetryLoop{}); got != 3*time.Millisecond {
		t.Fatalf("test override: want 3ms, got %s", got)
	}
}

// TestRetryLoopPollInterval_EnvInvalidFallsBackToDefault proves a
// broken environment value does not break the loop.
func TestRetryLoopPollInterval_EnvInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv(RetryLoopPollIntervalEnv, "not-a-duration")
	guard := &model.Node{Name: "If", Type: "n8n-nodes-base.if"}
	if got := resolveRetryLoopPollInterval(&model.Workflow{}, guard, connections.IfRetryLoop{}); got != DefaultRetryLoopPollInterval {
		t.Fatalf("invalid env: want %s, got %s", DefaultRetryLoopPollInterval, got)
	}
}

// TestRetryLoop_CounterAdvancesAndGuardTerminates proves the loop
// re-executes the counter node, so the `attempt < maxAttempts` guard
// advances and stops the loop before the hard iteration cap.
//
// The counter's `maxAttempts` is not a literal (it is computed), so
// the workflow-level iteration cap stays at the default 10 while the
// guard's limit is 5. A loop that never advances the counter would
// run all 10 iterations.
func TestRetryLoop_CounterAdvancesAndGuardTerminates(t *testing.T) {
	SetRetryLoopPollIntervalForTesting(time.Millisecond)
	defer ResetRetryLoopPollIntervalForTesting()

	counterCode := `let currentAttempt = $input.item.json.attempt || 0;
currentAttempt += 1;
let limit = Number($input.item.json.limit || 5);
return { json: { attempt: currentAttempt, maxAttempts: limit } };`

	mongo := &pollMongo{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Mongo"})}
	eng := newRetryLoopEngine(mongo)

	start := time.Now()
	runRetryWorkflow(t, eng, retryLoopWorkflow(counterCode, defaultGuardParams()))
	elapsed := time.Since(start)

	if mongo.calls != 5 {
		t.Fatalf("Find Respond calls: want 5 (guard limit), got %d (attempts=%v)", mongo.calls, mongo.attempts)
	}
	for i, attempt := range mongo.attempts {
		if attempt != i+1 {
			t.Fatalf("attempt[%d]: want %d, got %d (counter did not advance: %v)", i, i+1, attempt, mongo.attempts)
		}
	}
	// The old hard-coded 2 s sleep would need 8 s for 5 iterations.
	if elapsed > 2*time.Second {
		t.Fatalf("loop took %s — the poll interval is not being applied", elapsed)
	}
}

// TestRetryLoop_ExitsEarlyWhenReplyArrives proves the loop stops as
// soon as the body finds the reply instead of polling to the limit.
func TestRetryLoop_ExitsEarlyWhenReplyArrives(t *testing.T) {
	SetRetryLoopPollIntervalForTesting(time.Millisecond)
	defer ResetRetryLoopPollIntervalForTesting()

	counterCode := `let currentAttempt = $input.item.json.attempt || 0;
currentAttempt += 1;
return { json: { attempt: currentAttempt, maxAttempts: 10 } };`

	mongo := &pollMongo{
		BaseNode:  base.NewBaseNode(base.NodeDescription{Name: "Mongo"}),
		hitsAfter: 2, // first poll misses, second poll finds the reply
	}
	eng := newRetryLoopEngine(mongo)
	runRetryWorkflow(t, eng, retryLoopWorkflow(counterCode, defaultGuardParams()))

	if mongo.calls != 2 {
		t.Fatalf("Find Respond calls: want 2 (early exit), got %d", mongo.calls)
	}
}

// TestRetryLoop_MultiConditionGuardIsDetected proves a guard whose
// retry predicate is not the first condition is still recognised as a
// retry guard (the detector used to look at the first row only, so
// such workflows were rejected as "contains cycles").
func TestRetryLoop_MultiConditionGuardIsDetected(t *testing.T) {
	SetRetryLoopPollIntervalForTesting(time.Millisecond)
	defer ResetRetryLoopPollIntervalForTesting()

	counterCode := `let currentAttempt = $input.item.json.attempt || 0;
currentAttempt += 1;
return { json: { attempt: currentAttempt, maxAttempts: 3 } };`

	// The retry predicate is the SECOND row; the first row is an
	// unrelated "the reply has not arrived yet" check.
	guardParams := map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "and",
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "={{ $json.respond_type }}",
					"rightValue": "",
					"operator":   map[string]interface{}{"operation": "empty", "type": "string", "singleValue": true},
				},
				map[string]interface{}{
					"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
					"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
					"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
				},
			},
		},
	}

	mongo := &pollMongo{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Mongo"})}
	eng := newRetryLoopEngine(mongo)
	runRetryWorkflow(t, eng, retryLoopWorkflow(counterCode, guardParams))

	if mongo.calls != 3 {
		t.Fatalf("Find Respond calls: want 3 (multi-condition guard driven), got %d", mongo.calls)
	}
}

// TestRetryLoop_MultiConditionGuardOrShortCircuits proves a guard
// with an OR combinator leaves the loop as soon as one of its
// conditions holds.
func TestRetryLoop_MultiConditionGuardOrShortCircuits(t *testing.T) {
	SetRetryLoopPollIntervalForTesting(time.Millisecond)
	defer ResetRetryLoopPollIntervalForTesting()

	counterCode := `let currentAttempt = $input.item.json.attempt || 0;
currentAttempt += 1;
return { json: { attempt: currentAttempt, maxAttempts: 10 } };`

	// OR: the loop stops as soon as the reply is present (the
	// second condition holds) — the first condition (attempt <
	// maxAttempts) would otherwise keep it going.
	guardParams := map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "or",
			"conditions": []interface{}{
				map[string]interface{}{
					"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
					"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
					"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
				},
				map[string]interface{}{
					"leftValue":  "={{ $json.respond_type }}",
					"rightValue": "",
					"operator":   map[string]interface{}{"operation": "notEmpty", "type": "string", "singleValue": true},
				},
			},
		},
	}

	mongo := &pollMongo{
		BaseNode:  base.NewBaseNode(base.NodeDescription{Name: "Mongo"}),
		hitsAfter: 2,
	}
	eng := newRetryLoopEngine(mongo)
	runRetryWorkflow(t, eng, retryLoopWorkflow(counterCode, guardParams))

	if mongo.calls != 2 {
		t.Fatalf("Find Respond calls: want 2, got %d", mongo.calls)
	}
}
