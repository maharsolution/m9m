/*
Package connections — tests for If-retry loop detection.

The Finnet "Webhook use Redist - Kafka/Rabbit" workflow is the
canonical example: a Webhook → Payload → kafka produce → Set
Counter (Code) → Find Respond (mongo) → Value (Set) → Check Key
Exists1 (If) → IsRespondJson (If) → Respond to Json/XML →
findAndReplace mongo. The other branch from Check Key Exists1
falls into the `If` guard, which checks
`$('Set Counter').item.json.attempt < $('Set Counter').item.json.maxAttempts`
and on true routes back to `Find Respond` (the back-edge), on
false routes to `Respond Failed`.

The pre-fix m9m behavior: cycle detection rejected the
workflow outright. Post-fix: the engine detects the
`attempt < maxAttempts` pattern, drops the back-edge for
topological ordering, and drives the iteration itself (capped at
`maxAttempts`, default 10).
*/
package connections

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// finnetWorkflow returns a minimal model of the Finnet
// post_balance workflow, with the If-retry loop in place.
// Only the nodes and connections that affect loop detection
// are populated; the rest of the workflow's nodes are stubbed
// out so the model is compact.
func finnetWorkflow() *model.Workflow {
	return &model.Workflow{
		ID:   "yN3rxd0toTRNWVBO",
		Name: "Finnet - Webhook use Redist - Kafka/Rabbit",
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{-1088, -640}, Parameters: map[string]interface{}{"path": "post_balance"}},
			{Name: "Payload", Type: "n8n-nodes-base.set", Position: []int{-816, -640}, Parameters: map[string]interface{}{}},
			{Name: "finnet-queue", Type: "n8n-nodes-base.kafka", Position: []int{-608, -640}, Parameters: map[string]interface{}{"topic": "finnet-queue"}},
			{Name: "Set Counter", Type: "n8n-nodes-base.code", Position: []int{-336, -736}, Parameters: map[string]interface{}{"jsCode": "let currentAttempt = $input.item.json.attempt || 0;\ncurrentAttempt += 1;\nreturn {\n  json: {\n    attempt: currentAttempt,\n    maxAttempts: 10\n  }\n};"}},
			{Name: "Find Respond", Type: "n8n-nodes-base.mongoDb", Position: []int{-112, -736}, Parameters: map[string]interface{}{"collection": "finnet_store"}},
			{Name: "Value", Type: "n8n-nodes-base.set", Position: []int{128, -736}, Parameters: map[string]interface{}{}},
			{Name: "Check Key Exists1", Type: "n8n-nodes-base.if", Position: []int{304, -736}, Parameters: map[string]interface{}{}},
			{Name: "IsRespondJson", Type: "n8n-nodes-base.if", Position: []int{500, -800}, Parameters: map[string]interface{}{}},
			{Name: "Respond to Json", Type: "n8n-nodes-base.respondToWebhook", Position: []int{700, -800}, Parameters: map[string]interface{}{}},
			{Name: "Respond to XML", Type: "n8n-nodes-base.respondToWebhook", Position: []int{700, -640}, Parameters: map[string]interface{}{}},
			{Name: "If", Type: "n8n-nodes-base.if", Position: []int{608, -576}, Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "and",
					"conditions": []interface{}{
						map[string]interface{}{
							"id":         "90996403-bbd0-45bb-a853-bb2839a4a8e8",
							"leftValue":  "={{ $('Set Counter').item.json.attempt }}",
							"operator":   map[string]interface{}{"operation": "lt", "type": "number"},
							"rightValue": "={{ $('Set Counter').item.json.maxAttempts }}",
						},
					},
					"options": map[string]interface{}{"caseSensitive": true, "leftValue": "", "typeValidation": "strict", "version": 3},
				},
			}},
			{Name: "Respond Failed", Type: "n8n-nodes-base.respondToWebhook", Position: []int{944, -560}, Parameters: map[string]interface{}{}},
		},
		Connections: map[string]model.Connections{
			"Webhook":           {Main: [][]model.Connection{{{Node: "Payload", Type: "main", Index: 0}}}},
			"Payload":           {Main: [][]model.Connection{{{Node: "finnet-queue", Type: "main", Index: 0}}}},
			"finnet-queue":      {Main: [][]model.Connection{{{Node: "Set Counter", Type: "main", Index: 0}}}},
			"Set Counter":       {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}}},
			"Find Respond":      {Main: [][]model.Connection{{{Node: "Value", Type: "main", Index: 0}}}},
			"Value":             {Main: [][]model.Connection{{{Node: "Check Key Exists1", Type: "main", Index: 0}}}},
			"Check Key Exists1": {Main: [][]model.Connection{{{Node: "IsRespondJson", Type: "main", Index: 0}}, {{Node: "If", Type: "main", Index: 0}}}},
			"IsRespondJson":     {Main: [][]model.Connection{{{Node: "Respond to Json", Type: "main", Index: 0}}, {{Node: "Respond to XML", Type: "main", Index: 0}}}},
			"If":                {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}, {{Node: "Respond Failed", Type: "main", Index: 0}}}},
		},
	}
}

func TestDetectIfRetryLoops_Finnet(t *testing.T) {
	wf := finnetWorkflow()
	loops := DetectIfRetryLoops(wf)
	if len(loops) != 1 {
		t.Fatalf("expected 1 If-retry loop in Finnet workflow, got %d", len(loops))
	}
	got := loops[0]
	if got.GuardNode != "If" {
		t.Errorf("GuardNode: want %q, got %q", "If", got.GuardNode)
	}
	if got.BackEdgeTarget != "Find Respond" {
		t.Errorf("BackEdgeTarget: want %q, got %q", "Find Respond", got.BackEdgeTarget)
	}
	if got.MaxIterations != 10 {
		t.Errorf("MaxIterations: want 10, got %d", got.MaxIterations)
	}
	// Body should include Find Respond, Value, Check Key
	// Exists1 — the closed cycle nodes (F ∩ B in the
	// detector) that can also reach the guard. The guard
	// itself is excluded, as are exit-branch nodes
	// (IsRespondJson, Respond to Json/XML, Respond Failed)
	// which the main engine handles when the body's
	// terminal If routes items to them.
	wantBody := map[string]bool{
		"Find Respond":      true,
		"Value":             true,
		"Check Key Exists1": true,
	}
	if len(got.BodyNodes) != len(wantBody) {
		t.Errorf("BodyNodes size: want %d, got %d (%v)", len(wantBody), len(got.BodyNodes), got.BodyNodes)
	}
	for _, n := range got.BodyNodes {
		if !wantBody[n] {
			t.Errorf("unexpected body node %q", n)
		}
	}
}

func TestWorkflowUsesIfRetryLoop_Finnet(t *testing.T) {
	wf := finnetWorkflow()
	if !WorkflowUsesIfRetryLoop(wf) {
		t.Fatal("expected WorkflowUsesIfRetryLoop to return true for the Finnet workflow")
	}
}

func TestHasCycles_AllowsIfRetryLoop(t *testing.T) {
	router := NewConnectionRouter()
	wf := finnetWorkflow()
	hasCycles, err := router.HasCycles(wf)
	if err != nil {
		t.Fatalf("HasCycles: unexpected error: %v", err)
	}
	if hasCycles {
		t.Fatal("expected HasCycles to return false for a workflow whose only cycle is an If-retry loop")
	}
}

func TestGetExecutionOrder_IfRetryLoopIsTopological(t *testing.T) {
	router := NewConnectionRouter()
	wf := finnetWorkflow()
	order, err := router.GetExecutionOrder(wf)
	if err != nil {
		t.Fatalf("GetExecutionOrder: %v", err)
	}
	// The guard `If` should appear after the body nodes in
	// the execution order (since it depends on them).
	ifIdx := -1
	for i, n := range order {
		if n == "If" {
			ifIdx = i
			break
		}
	}
	if ifIdx == -1 {
		t.Fatal("guard `If` not present in execution order")
	}
	for _, bodyNode := range []string{"Set Counter", "Find Respond", "Value", "Check Key Exists1"} {
		bodyIdx := -1
		for i, n := range order {
			if n == bodyNode {
				bodyIdx = i
				break
			}
		}
		if bodyIdx == -1 {
			t.Errorf("body node %q not in execution order", bodyNode)
			continue
		}
		if bodyIdx >= ifIdx {
			t.Errorf("body node %q (idx %d) should execute before guard `If` (idx %d)", bodyNode, bodyIdx, ifIdx)
		}
	}
}

func TestDetectIfRetryLoops_NoRetryLoop(t *testing.T) {
	// Plain DAG with no If-retry pattern.
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook", Position: []int{0, 0}, Parameters: map[string]interface{}{}},
			{Name: "Set", Type: "n8n-nodes-base.set", Position: []int{100, 0}, Parameters: map[string]interface{}{}},
		},
		Connections: map[string]model.Connections{
			"Webhook": {Main: [][]model.Connection{{{Node: "Set", Type: "main", Index: 0}}}},
		},
	}
	if WorkflowUsesIfRetryLoop(wf) {
		t.Fatal("DAG without If-retry pattern should not match")
	}
}

func TestDetectIfRetryLoops_IfWithoutMaxAttempts(t *testing.T) {
	// If node with a different condition (not `attempt <
	// maxAttempts`) should NOT be classified as a retry guard.
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Set1", Type: "n8n-nodes-base.set", Position: []int{0, 0}, Parameters: map[string]interface{}{}},
			{Name: "If", Type: "n8n-nodes-base.if", Position: []int{100, 0}, Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"leftValue":  "={{ $json.status }}",
							"operator":   "equals",
							"rightValue": "ok",
						},
					},
				},
			}},
			{Name: "Set2", Type: "n8n-nodes-base.set", Position: []int{200, 0}, Parameters: map[string]interface{}{}},
		},
		Connections: map[string]model.Connections{
			"Set1": {Main: [][]model.Connection{{{Node: "If", Type: "main", Index: 0}}}},
			"If":   {Main: [][]model.Connection{{{Node: "Set2", Type: "main", Index: 0}}}},
		},
	}
	if WorkflowUsesIfRetryLoop(wf) {
		t.Fatal("If with non-retry condition should not match")
	}
}

func TestIsIfRetryLoopGuard(t *testing.T) {
	wf := finnetWorkflow()
	if !IsIfRetryLoopGuard(wf, "If") {
		t.Error("expected `If` to be detected as a retry-loop guard")
	}
	if IsIfRetryLoopGuard(wf, "Webhook") {
		t.Error("Webhook is not a retry-loop guard")
	}
	if IsIfRetryLoopGuard(wf, "DoesNotExist") {
		t.Error("nonexistent node is not a guard")
	}
}

func TestExtractMaxAttempts_FromCode(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Counter", Type: "n8n-nodes-base.code", Parameters: map[string]interface{}{
				"jsCode": "return { json: { attempt: 1, maxAttempts: 25 } };",
			}},
		},
	}
	if got := extractMaxAttempts(wf); got != 25 {
		t.Errorf("extractMaxAttempts from Code: want 25, got %d", got)
	}
}

func TestExtractMaxAttempts_Default(t *testing.T) {
	wf := &model.Workflow{
		Nodes: []model.Node{
			{Name: "Plain", Type: "n8n-nodes-base.set", Parameters: map[string]interface{}{}},
		},
	}
	if got := extractMaxAttempts(wf); got != defaultMaxIterations {
		t.Errorf("extractMaxAttempts default: want %d, got %d", defaultMaxIterations, got)
	}
}
