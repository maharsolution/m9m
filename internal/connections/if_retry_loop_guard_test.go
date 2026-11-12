/*
Package connections — tests for the canonical IF-condition reader and
the retry-guard detection that depends on it.

The retry guard of an If-retry loop is an ordinary n8n IF node: its
condition table decides whether the loop keeps polling. n8n stores
that table in two shapes — the bare array of older exports and the
`{conditions: [...], combinator: "and"|"or"}` wrapper the current UI
writes — and the operator of a row is either a bare string or a typed
object (`{type, operation, singleValue}`).

These tests pin the canonical reading of both shapes and the n8n
combination semantics (AND: all rows must hold; OR: any row may
hold), because the engine's retry-guard evaluation and the detector
must agree on them: a guard whose retry predicate is the *second*
row of the table still has to be recognised as a retry guard.
*/
package connections

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// ifRow builds one condition row in the n8n v2 shape.
func ifRow(left, right string, operation, typ string, singleValue bool) map[string]interface{} {
	return map[string]interface{}{
		"leftValue":  left,
		"rightValue": right,
		"operator": map[string]interface{}{
			"operation":   operation,
			"type":        typ,
			"singleValue": singleValue,
		},
	}
}

// TestResolveIfCombinator_V1AndV2Shapes proves the combinator is
// read from both n8n shapes and defaults to "and".
func TestResolveIfCombinator_V1AndV2Shapes(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "v2 wrapper combinator wins",
			params: map[string]interface{}{"conditions": map[string]interface{}{"combinator": "or", "conditions": []interface{}{}}},
			want:   IfCombinatorOr,
		},
		{
			name:   "v1 flat combinator",
			params: map[string]interface{}{"combinator": "or"},
			want:   IfCombinatorOr,
		},
		{
			name:   "v1 flat combiner spelling",
			params: map[string]interface{}{"combiner": "or"},
			want:   IfCombinatorOr,
		},
		{
			name:   "v2 wrapper combiner spelling",
			params: map[string]interface{}{"conditions": map[string]interface{}{"combiner": "or", "conditions": []interface{}{}}},
			want:   IfCombinatorOr,
		},
		{
			name:   "default is and",
			params: map[string]interface{}{},
			want:   IfCombinatorAnd,
		},
		{
			name:   "unknown spelling falls back to and",
			params: map[string]interface{}{"combinator": "xor"},
			want:   IfCombinatorAnd,
		},
		{
			name:   "wrapper wins over flat key",
			params: map[string]interface{}{"combinator": "and", "conditions": map[string]interface{}{"combinator": "or", "conditions": []interface{}{}}},
			want:   IfCombinatorOr,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveIfCombinator(tc.params); got != tc.want {
				t.Fatalf("ResolveIfCombinator: want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestExtractIfConditions_BothShapes proves both n8n shapes yield
// the same typed rows and that the short operator names are
// canonicalised.
func TestExtractIfConditions_BothShapes(t *testing.T) {
	rows := []interface{}{
		ifRow("={{ $json.attempt }}", "={{ $json.maxAttempts }}", "lt", "number", false),
		ifRow("={{ $json.respond_type }}", "", "notEmpty", "string", true),
	}
	bare := map[string]interface{}{"conditions": rows}
	wrapper := map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "and",
			"conditions": rows,
			"options":    map[string]interface{}{"version": 3},
		},
	}

	for _, params := range []map[string]interface{}{bare, wrapper} {
		conditions, err := ExtractIfConditions(params)
		if err != nil {
			t.Fatalf("ExtractIfConditions: unexpected error: %v", err)
		}
		if len(conditions) != 2 {
			t.Fatalf("condition count: want 2, got %d", len(conditions))
		}
		if conditions[0].Op != "lessThan" {
			t.Errorf("conditions[0].Op: want %q, got %q", "lessThan", conditions[0].Op)
		}
		if conditions[0].Type != "number" {
			t.Errorf("conditions[0].Type: want %q, got %q", "number", conditions[0].Type)
		}
		if conditions[0].SingleValue {
			t.Error("conditions[0].SingleValue: want false")
		}
		if conditions[1].Op != "notEmpty" {
			t.Errorf("conditions[1].Op: want %q, got %q", "notEmpty", conditions[1].Op)
		}
		if !conditions[1].SingleValue {
			t.Error("conditions[1].SingleValue: want true")
		}
	}
}

// TestExtractIfConditions_BareStringOperator proves the older bare
// string operator shape is accepted.
func TestExtractIfConditions_BareStringOperator(t *testing.T) {
	conditions, err := ExtractIfConditions(map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"leftValue": "a", "rightValue": "b", "operator": "equals"},
		},
	})
	if err != nil {
		t.Fatalf("ExtractIfConditions: unexpected error: %v", err)
	}
	if len(conditions) != 1 || conditions[0].Op != "equals" {
		t.Fatalf("conditions: want one 'equals' row, got %+v", conditions)
	}
}

// TestExtractIfConditions_Invalid proves a malformed table is
// reported instead of silently treated as "no conditions".
func TestExtractIfConditions_Invalid(t *testing.T) {
	if _, err := ExtractIfConditions(map[string]interface{}{}); err == nil {
		t.Fatal("expected an error for a missing condition table")
	}
	if _, err := ExtractIfConditions(map[string]interface{}{
		"conditions": []interface{}{map[string]interface{}{"rightValue": "b", "operator": "equals"}},
	}); err == nil {
		t.Fatal("expected an error for a row without 'leftValue'")
	}
	if _, err := ExtractIfConditions(map[string]interface{}{
		"conditions": []interface{}{map[string]interface{}{"leftValue": "a", "rightValue": "b"}},
	}); err == nil {
		t.Fatal("expected an error for a row without 'operator'")
	}
}

// TestNormalizeIfParameters proves the wrapper is flattened without
// mutating the input map.
func TestNormalizeIfParameters(t *testing.T) {
	params := map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "or",
			"conditions": []interface{}{ifRow("a", "b", "equals", "string", false)},
		},
	}
	normalized := NormalizeIfParameters(params)
	if got := normalized["combinator"]; got != IfCombinatorOr {
		t.Fatalf("combinator: want %q, got %v", IfCombinatorOr, got)
	}
	rows, ok := normalized["conditions"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("conditions: want one row, got %v", normalized["conditions"])
	}
	// The input must be untouched.
	if _, ok := params["combinator"]; ok {
		t.Fatal("NormalizeIfParameters mutated the input map")
	}
	if wrap, ok := params["conditions"].(map[string]interface{}); !ok || wrap["combinator"] != "or" {
		t.Fatal("NormalizeIfParameters mutated the input condition wrapper")
	}
}

// TestEvaluateIfConditions_OR_SecondConditionHolds proves an OR table
// is satisfied by its second row even when the first one fails.
func TestEvaluateIfConditions_OR_SecondConditionHolds(t *testing.T) {
	conditions := []IfCondition{
		{Left: 1, Right: 10, Op: "lessThan", Type: "number"},
		{Left: "xml", Right: "", Op: "notEmpty", Type: "string", SingleValue: true},
	}
	if !EvaluateIfConditions(conditions, IfCombinatorOr, nil) {
		t.Fatal("OR: want true when the second condition holds")
	}
}

// TestEvaluateIfConditions_OR_OnlyFirstConditionHolds proves an OR
// table is satisfied by its first row.
func TestEvaluateIfConditions_OR_OnlyFirstConditionHolds(t *testing.T) {
	conditions := []IfCondition{
		{Left: 1, Right: 10, Op: "lessThan", Type: "number"},
		{Left: "", Right: "", Op: "notEmpty", Type: "string", SingleValue: true},
	}
	if !EvaluateIfConditions(conditions, IfCombinatorOr, nil) {
		t.Fatal("OR: want true when only the first condition holds")
	}
}

// TestEvaluateIfConditions_OR_NoneHolds proves an OR table is false
// when no row holds.
func TestEvaluateIfConditions_OR_NoneHolds(t *testing.T) {
	conditions := []IfCondition{
		{Left: 10, Right: 5, Op: "lessThan", Type: "number"},
		{Left: "", Right: "", Op: "notEmpty", Type: "string", SingleValue: true},
	}
	if EvaluateIfConditions(conditions, IfCombinatorOr, nil) {
		t.Fatal("OR: want false when no condition holds")
	}
}

// TestEvaluateIfConditions_AND_RequiresEveryRow proves an AND table
// needs every row to hold.
func TestEvaluateIfConditions_AND_RequiresEveryRow(t *testing.T) {
	conditions := []IfCondition{
		{Left: 1, Right: 10, Op: "lessThan", Type: "number"},
		{Left: "xml", Right: "", Op: "notEmpty", Type: "string", SingleValue: true},
	}
	if !EvaluateIfConditions(conditions, IfCombinatorAnd, nil) {
		t.Fatal("AND: want true when both conditions hold")
	}
	conditions[1].Op = "empty"
	if EvaluateIfConditions(conditions, IfCombinatorAnd, nil) {
		t.Fatal("AND: want false when the second condition fails")
	}
}

// TestEvaluateIfConditions_EmptyTable proves an empty condition set
// is true (n8n's behaviour).
func TestEvaluateIfConditions_EmptyTable(t *testing.T) {
	if !EvaluateIfConditions(nil, IfCombinatorAnd, nil) {
		t.Fatal("empty condition table: want true")
	}
}

// TestEvaluateIfConditions_UsesResolver proves the caller's resolver
// (which evaluates n8n expressions) is used for the operands.
func TestEvaluateIfConditions_UsesResolver(t *testing.T) {
	conditions := []IfCondition{
		{Left: "={{ $json.attempt }}", Right: "={{ $json.maxAttempts }}", Op: "lessThan", Type: "number"},
	}
	resolved := false
	resolve := func(v interface{}) interface{} {
		resolved = true
		switch v {
		case "={{ $json.attempt }}":
			return 3
		case "={{ $json.maxAttempts }}":
			return 10
		}
		return nil
	}
	if !EvaluateIfConditions(conditions, IfCombinatorAnd, resolve) {
		t.Fatal("want true for 3 < 10")
	}
	if !resolved {
		t.Fatal("resolver was not used")
	}
}

// TestCompareIfValues_MixedOperators covers the operator families the
// retry guard and the body's terminal If node use.
func TestCompareIfValues_MixedOperators(t *testing.T) {
	cases := []struct {
		name     string
		left     interface{}
		right    interface{}
		op       string
		typ      string
		expected bool
	}{
		{"equals strings", "xml", "xml", "equals", "string", true},
		{"equals strings differ", "xml", "json", "equals", "string", false},
		{"notEquals strings", "xml", "json", "notEquals", "string", true},
		{"equals numbers", 3, 3.0, "equals", "number", true},
		{"equals numbers differ", 3, 4, "equals", "number", false},
		{"equals numeric string", "3", 3, "equals", "number", true},
		{"equals booleans", true, "true", "equals", "boolean", true},
		{"less than numbers", 3, 10, "lessThan", "number", true},
		{"less than numbers false", 10, 3, "lessThan", "number", false},
		{"less than strings", "abc", "abd", "lessThan", "string", true},
		{"greater than numbers", 10, 3, "greaterThan", "number", true},
		{"greater than strings", "abd", "abc", "greaterThan", "string", true},
		{"less than or equal", 10, 10, "lessThanOrEqual", "number", true},
		{"greater than or equal", 10, 10, "greaterThanOrEqual", "number", true},
		{"exists on a value", "value", nil, "exists", "", true},
		{"exists on nil", nil, nil, "exists", "", false},
		{"notEmpty on a value", "xml", nil, "notEmpty", "string", true},
		{"notEmpty on empty string", "", nil, "notEmpty", "string", false},
		{"empty on empty string", "", nil, "empty", "string", true},
		{"empty on a value", "xml", nil, "empty", "string", false},
		{"contains", "post_balance", "balance", "contains", "string", true},
		{"notContains", "post_balance", "xml", "notContains", "string", true},
		{"startsWith", "post_balance", "post", "startsWith", "string", true},
		{"endsWith", "post_balance", "balance", "endsWith", "string", true},
		{"regex", "post_balance", "^post_", "regex", "string", true},
		{"between", 5, "1,10", "between", "number", true},
		{"between outside", 50, "1,10", "between", "number", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompareIfValues(tc.left, tc.right, tc.op, tc.typ); got != tc.expected {
				t.Fatalf("CompareIfValues(%v, %v, %q, %q): want %v, got %v",
					tc.left, tc.right, tc.op, tc.typ, tc.expected, got)
			}
		})
	}
}

// TestCanonicalIfOperatorName proves the short operator names n8n
// uses in its exports map onto the long form.
func TestCanonicalIfOperatorName(t *testing.T) {
	cases := map[string]string{
		"lt":  "lessThan",
		"lte": "lessThanOrEqual",
		"gt":  "greaterThan",
		"gte": "greaterThanOrEqual",
		"eq":  "equals",
		"neq": "notEquals",
	}
	for short, long := range cases {
		if got := CanonicalIfOperatorName(short); got != long {
			t.Errorf("CanonicalIfOperatorName(%q): want %q, got %q", short, long, got)
		}
	}
}

// multiConditionGuardWorkflow builds a retry loop whose guard holds
// the retry predicate in the SECOND row of its condition table (the
// first row is an unrelated check).
func multiConditionGuardWorkflow() *model.Workflow {
	return &model.Workflow{
		ID: "multi-condition-guard",
		Nodes: []model.Node{
			{Name: "Set Counter", Type: "n8n-nodes-base.code", Position: []int{0, 0}, Parameters: map[string]interface{}{
				"jsCode": "let currentAttempt = $input.item.json.attempt || 0;\ncurrentAttempt += 1;\nreturn { json: { attempt: currentAttempt, maxAttempts: 10 } };",
			}},
			{Name: "Find Respond", Type: "n8n-nodes-base.mongoDb", Position: []int{100, 0}, Parameters: map[string]interface{}{}},
			{Name: "If", Type: "n8n-nodes-base.if", Position: []int{200, 0}, Parameters: map[string]interface{}{
				"conditions": map[string]interface{}{
					"combinator": "and",
					"conditions": []interface{}{
						ifRow("={{ $json.respond_type }}", "", "empty", "string", true),
						ifRow("={{ $('Set Counter').item.json.attempt }}", "={{ $('Set Counter').item.json.maxAttempts }}", "lt", "number", false),
					},
				},
			}},
		},
		Connections: map[string]model.Connections{
			"Set Counter":  {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}}},
			"Find Respond": {Main: [][]model.Connection{{{Node: "If", Type: "main", Index: 0}}}},
			"If":           {Main: [][]model.Connection{{{Node: "Find Respond", Type: "main", Index: 0}}}},
		},
	}
}

// TestDetectIfRetryLoops_MultiConditionGuard proves a guard whose
// retry predicate is not the first row is still detected — only the
// first row used to be inspected, so such workflows were rejected as
// "contains cycles".
func TestDetectIfRetryLoops_MultiConditionGuard(t *testing.T) {
	loops := DetectIfRetryLoops(multiConditionGuardWorkflow())
	if len(loops) != 1 {
		t.Fatalf("expected 1 retry loop, got %d", len(loops))
	}
	if loops[0].GuardNode != "If" {
		t.Errorf("GuardNode: want %q, got %q", "If", loops[0].GuardNode)
	}
	if loops[0].BackEdgeTarget != "Find Respond" {
		t.Errorf("BackEdgeTarget: want %q, got %q", "Find Respond", loops[0].BackEdgeTarget)
	}
	if loops[0].CounterNode != "Set Counter" {
		t.Errorf("CounterNode: want %q, got %q", "Set Counter", loops[0].CounterNode)
	}
}

// TestDetectIfRetryLoops_OrGuard proves an OR guard whose retry
// predicate is one of several rows is detected as well.
func TestDetectIfRetryLoops_OrGuard(t *testing.T) {
	wf := multiConditionGuardWorkflow()
	guard := &wf.Nodes[2]
	guard.Parameters = map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "or",
			"conditions": []interface{}{
				ifRow("={{ $('Set Counter').item.json.attempt }}", "={{ $('Set Counter').item.json.maxAttempts }}", "lt", "number", false),
				ifRow("={{ $json.respond_type }}", "", "notEmpty", "string", true),
			},
		},
	}
	loops := DetectIfRetryLoops(wf)
	if len(loops) != 1 {
		t.Fatalf("expected 1 retry loop, got %d", len(loops))
	}
	if loops[0].GuardNode != "If" {
		t.Errorf("GuardNode: want %q, got %q", "If", loops[0].GuardNode)
	}
}
