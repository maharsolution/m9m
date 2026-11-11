package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	httpnode "github.com/neul-labs/m9m/internal/nodes/http"
)

// ---------------------------------------------------------------------------
// Unit tests for NormalizeNodeParameters
// ---------------------------------------------------------------------------

// TestNormalizeNodeParametersFlatKeyBecomesNested covers the flat -> nested
// direction: a literal dotted key must become reachable under the nested
// path the metadata-driven editor writes.
func TestNormalizeNodeParametersFlatKeyBecomesNested(t *testing.T) {
	params := map[string]interface{}{
		"response.includeResponseHeadersAndStatus": true,
	}

	out := NormalizeNodeParameters(params)

	response, ok := out["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested \"response\" object, got %#v", out["response"])
	}
	if got, ok := response["includeResponseHeadersAndStatus"].(bool); !ok || !got {
		t.Errorf("expected nested includeResponseHeadersAndStatus=true, got %#v", response["includeResponseHeadersAndStatus"])
	}
	// The original flat key must survive untouched.
	if got, ok := out["response.includeResponseHeadersAndStatus"].(bool); !ok || !got {
		t.Errorf("expected the flat key to be preserved, got %#v", out["response.includeResponseHeadersAndStatus"])
	}
}

// TestNormalizeNodeParametersNestedBecomesFlat covers the nested -> flat
// direction: every entry of a nested object also gets a flat dotted alias.
func TestNormalizeNodeParametersNestedBecomesFlat(t *testing.T) {
	params := map[string]interface{}{
		"options": map[string]interface{}{
			"responseHeaders": map[string]interface{}{
				"entries": []interface{}{map[string]interface{}{"name": "X-Test"}},
			},
		},
	}

	out := NormalizeNodeParameters(params)

	headers, ok := out["options.responseHeaders"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected flat alias \"options.responseHeaders\", got %#v", out["options.responseHeaders"])
	}
	if _, ok := headers["entries"]; !ok {
		t.Errorf("expected the aliased object to keep its entries, got %#v", headers)
	}
	if _, ok := out["options.responseHeaders.entries"]; !ok {
		t.Errorf("expected deep alias \"options.responseHeaders.entries\", got %#v", out)
	}
	// The nested shape must stay intact.
	nested, ok := out["options"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested \"options\" object, got %#v", out["options"])
	}
	if _, ok := nested["responseHeaders"]; !ok {
		t.Errorf("expected the nested shape to be preserved, got %#v", nested)
	}
}

// TestNormalizeNodeParametersArrayAliases documents the array rule: an array
// is aliased as a whole value and is never descended into, because array
// elements have no stable flat key.
func TestNormalizeNodeParametersArrayAliases(t *testing.T) {
	params := map[string]interface{}{
		"conditions": map[string]interface{}{
			"conditions": []interface{}{map[string]interface{}{"leftValue": "a"}},
		},
	}

	out := NormalizeNodeParameters(params)

	if _, ok := out["conditions.conditions"]; !ok {
		t.Fatalf("expected flat alias \"conditions.conditions\", got %#v", out)
	}
	if _, ok := out["conditions.conditions.leftValue"]; ok {
		t.Errorf("arrays must not be descended into, got %#v", out)
	}
}

// TestNormalizeNodeParametersPrecedenceKeepsBothShapes pins the precedence
// rule: normalisation never overwrites an existing value. When a parameter
// exists in both shapes with different values, each shape keeps the value it
// had.
func TestNormalizeNodeParametersPrecedenceKeepsBothShapes(t *testing.T) {
	params := map[string]interface{}{
		"response.includeResponseHeadersAndStatus": false,
		"response": map[string]interface{}{
			"includeResponseHeadersAndStatus": true,
		},
	}

	out := NormalizeNodeParameters(params)

	flat, ok := out["response.includeResponseHeadersAndStatus"].(bool)
	if !ok || flat {
		t.Errorf("expected the explicit flat value (false) to be preserved, got %#v", out["response.includeResponseHeadersAndStatus"])
	}
	nested, ok := out["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested \"response\" object, got %#v", out["response"])
	}
	if got, ok := nested["includeResponseHeadersAndStatus"].(bool); !ok || !got {
		t.Errorf("expected the explicit nested value (true) to be preserved, got %#v", nested["includeResponseHeadersAndStatus"])
	}
}

// TestNormalizeNodeParametersIdempotent verifies that normalising an
// already normalised map is a no-op.
func TestNormalizeNodeParametersIdempotent(t *testing.T) {
	params := map[string]interface{}{
		"response.includeResponseHeadersAndStatus": true,
		"options": map[string]interface{}{
			"responseHeaders": map[string]interface{}{"entries": []interface{}{}},
		},
		"plain": "value",
	}

	once := NormalizeNodeParameters(params)
	twice := NormalizeNodeParameters(once)

	if !reflect.DeepEqual(once, twice) {
		t.Errorf("normalisation is not idempotent:\nonce  = %#v\ntwice = %#v", once, twice)
	}
	if reflect.ValueOf(once).Pointer() != reflect.ValueOf(twice).Pointer() {
		t.Errorf("expected the second normalisation to return the same map without copying it")
	}
}

// TestNormalizeNodeParametersPlainKeysAreNoOp verifies that a parameter map
// without dotted keys and without nested objects is returned untouched.
func TestNormalizeNodeParametersPlainKeysAreNoOp(t *testing.T) {
	params := map[string]interface{}{
		"url":     "https://example.test",
		"method":  "GET",
		"timeout": 30000,
	}

	out := NormalizeNodeParameters(params)

	if reflect.ValueOf(out).Pointer() != reflect.ValueOf(params).Pointer() {
		t.Errorf("expected the original map to be returned for plain keys")
	}
	if len(out) != len(params) {
		t.Errorf("expected %d keys, got %d", len(params), len(out))
	}
}

// TestNormalizeNodeParametersDoesNotMutateCaller verifies the copy-on-write
// contract: neither the caller's top-level map nor the nested maps it points
// at are modified.
func TestNormalizeNodeParametersDoesNotMutateCaller(t *testing.T) {
	nested := map[string]interface{}{"existing": "kept"}
	params := map[string]interface{}{
		"a.b":      "flat value",
		"a":        nested,
		"n":        map[string]interface{}{"x": 1},
		"plainKey": "plain",
	}
	before := map[string]interface{}{
		"a.b":      "flat value",
		"a":        map[string]interface{}{"existing": "kept"},
		"n":        map[string]interface{}{"x": 1},
		"plainKey": "plain",
	}

	out := NormalizeNodeParameters(params)

	if !reflect.DeepEqual(params, before) {
		t.Errorf("the caller's map was mutated:\ngot  %#v\nwant %#v", params, before)
	}
	if _, ok := nested["b"]; ok {
		t.Errorf("the caller's nested map was mutated: %#v", nested)
	}
	// The nested map must be COPIED before it is extended — the caller's map
	// must not be aliased into the normalised output.
	if fmt.Sprintf("%p", out["a"]) == fmt.Sprintf("%p", nested) {
		t.Errorf("expected the nested map to be copied before it was extended")
	}
	extended, ok := out["a"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested \"a\" object, got %#v", out["a"])
	}
	if got, ok := extended["b"].(string); !ok || got != "flat value" {
		t.Errorf("expected the flat value to be materialised nested, got %#v", extended["b"])
	}
	if got, ok := extended["existing"].(string); !ok || got != "kept" {
		t.Errorf("expected the existing nested value to survive, got %#v", extended["existing"])
	}
	// The nested-only branch must not be mutated either.
	if _, ok := params["n"].(map[string]interface{})["x"]; !ok {
		t.Errorf("expected the nested-only map to stay intact, got %#v", params["n"])
	}
	if _, ok := out["n.x"]; !ok {
		t.Errorf("expected alias \"n.x\", got %#v", out)
	}
}

// TestNormalizeNodeParametersCredentialEnvelope verifies the credential
// envelope contract: injected keys keep their flat `<credType>_<field>`
// shape, an injected dotted key also becomes reachable under its nested
// path, and nothing is written back into the user-visible parameters.
func TestNormalizeNodeParametersCredentialEnvelope(t *testing.T) {
	params := map[string]interface{}{
		"kafka_brokers": []interface{}{"broker:9092"},
		"response.includeResponseHeadersAndStatus": true,
	}

	out := NormalizeNodeParameters(params)

	if _, ok := out["kafka_brokers"]; !ok {
		t.Errorf("expected the injected credential key to stay at the top level, got %#v", out)
	}
	response, ok := out["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested \"response\" object, got %#v", out["response"])
	}
	if got, ok := response["includeResponseHeadersAndStatus"].(bool); !ok || !got {
		t.Errorf("expected the injected dotted key to become reachable nested, got %#v", response)
	}
	// The user-visible map must not gain the synthesised nested object.
	if _, ok := params["response"]; ok {
		t.Errorf("expected no nested object to leak into the user-visible parameters, got %#v", params)
	}
	if _, ok := params["kafka_brokers"]; !ok {
		t.Errorf("expected the injected credential key to stay untouched, got %#v", params)
	}
}

// TestNormalizeNodeParametersBlockedPath verifies that a non-object value
// blocking a nested path is never replaced.
func TestNormalizeNodeParametersBlockedPath(t *testing.T) {
	params := map[string]interface{}{
		"a":   "text",
		"a.b": "value",
	}

	out := NormalizeNodeParameters(params)

	if got, ok := out["a"].(string); !ok || got != "text" {
		t.Errorf("expected the blocking value to survive, got %#v", out["a"])
	}
	if got, ok := out["a.b"].(string); !ok || got != "value" {
		t.Errorf("expected the flat key to survive, got %#v", out["a.b"])
	}
}

// TestNormalizeNodeParametersInvalidDottedKeys verifies that keys which
// cannot be represented as a nested path are ignored.
func TestNormalizeNodeParametersInvalidDottedKeys(t *testing.T) {
	params := map[string]interface{}{
		"a..b": 1,
		".a":   2,
		"a.":   3,
	}

	out := NormalizeNodeParameters(params)

	if len(out) != len(params) {
		t.Errorf("expected no synthesised keys for invalid dotted keys, got %#v", out)
	}
	for _, key := range []string{"a", "", "a."} {
		if _, ok := out[key]; ok && key != "a." {
			t.Errorf("unexpected key %q in %#v", key, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Regression guards: flat and nested shapes must behave identically
// ---------------------------------------------------------------------------

// flatReaderNode reads a parameter through the literal flat dotted key,
// the way internal/nodes/http/request.go reads
// "response.includeResponseHeadersAndStatus".
type flatReaderNode struct {
	*base.BaseNode
	seen map[string]interface{}
}

func (n *flatReaderNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	n.seen = nodeParams
	value, _ := nodeParams["response.includeResponseHeadersAndStatus"].(bool)
	out := make([]model.DataItem, len(inputData))
	for i := range inputData {
		out[i] = model.DataItem{JSON: map[string]interface{}{"includeMeta": value}}
	}
	return out, nil
}

// paramShapeTrigger forwards the incoming items to the next node.
type paramShapeTrigger struct {
	*base.BaseNode
}

func (t *paramShapeTrigger) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return inputData, nil
}

// runParamShapeWorkflow executes a two node workflow (trigger -> consumer)
// and returns the consumer's output.
func runParamShapeWorkflow(t *testing.T, consumer base.NodeExecutor, consumerParams map[string]interface{}) []model.DataItem {
	t.Helper()

	eng := NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.manualTrigger", &paramShapeTrigger{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Trigger"})})
	eng.RegisterNodeExecutor("n8n-nodes-base.testConsumer", consumer)

	workflow := &model.Workflow{
		ID:   "wf-param-shape",
		Name: "Parameter Shape",
		Nodes: []model.Node{
			{Name: "Start", Type: "n8n-nodes-base.manualTrigger", Position: []int{-300, 0}},
			{Name: "Consumer", Type: "n8n-nodes-base.testConsumer", Parameters: consumerParams, Position: []int{0, 0}},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{{{Node: "Consumer", Type: "main", Index: 0}}}},
		},
	}

	result, err := eng.ExecuteWorkflow(workflow, []model.DataItem{{JSON: map[string]interface{}{"seed": 1}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a non-nil result")
	}
	if result.Error != nil {
		t.Fatalf("workflow error: %v", result.Error)
	}
	return result.Data
}

// TestEngineDeliversFlatAndNestedParameterShapesEqually is the core
// regression guard: a workflow saved with the flat dotted key and one saved
// with the nested shape must reach the executor with the same effective
// value.
func TestEngineDeliversFlatAndNestedParameterShapesEqually(t *testing.T) {
	flatNode := &flatReaderNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Flat Reader"})}
	flatOut := runParamShapeWorkflow(t, flatNode, map[string]interface{}{
		"response.includeResponseHeadersAndStatus": true,
	})
	if len(flatOut) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(flatOut))
	}
	if flatOut[0].JSON["includeMeta"] != true {
		t.Errorf("expected the flat shape to reach the executor, got %#v", flatOut[0].JSON)
	}

	nestedNode := &flatReaderNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Flat Reader"})}
	nestedOut := runParamShapeWorkflow(t, nestedNode, map[string]interface{}{
		"response": map[string]interface{}{
			"includeResponseHeadersAndStatus": true,
		},
	})
	if len(nestedOut) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(nestedOut))
	}
	if nestedOut[0].JSON["includeMeta"] != true {
		t.Errorf("expected the nested shape to reach the executor, got %#v", nestedOut[0].JSON)
	}

	// Both shapes must deliver the very same effective value.
	if !reflect.DeepEqual(flatOut[0].JSON, nestedOut[0].JSON) {
		t.Errorf("flat and nested shapes diverged:\nflat   %#v\nnested %#v", flatOut[0].JSON, nestedOut[0].JSON)
	}
}

// TestEnginePersistedParametersStayN8NClean verifies that the synthesised
// flat aliases never leak back into the persisted workflow parameters: the
// saved JSON must stay in the n8n shape.
func TestEnginePersistedParametersStayN8NClean(t *testing.T) {
	eng := NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.manualTrigger", &paramShapeTrigger{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Trigger"})})
	eng.RegisterNodeExecutor("n8n-nodes-base.testConsumer", &flatReaderNode{BaseNode: base.NewBaseNode(base.NodeDescription{Name: "Flat Reader"})})

	workflow := &model.Workflow{
		ID:   "wf-param-shape-clean",
		Name: "Parameter Shape Clean",
		Nodes: []model.Node{
			{Name: "Start", Type: "n8n-nodes-base.manualTrigger", Position: []int{-300, 0}},
			{
				Name: "Consumer",
				Type: "n8n-nodes-base.testConsumer",
				Parameters: map[string]interface{}{
					"response": map[string]interface{}{
						"includeResponseHeadersAndStatus": true,
					},
				},
				Position: []int{0, 0},
			},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{{{Node: "Consumer", Type: "main", Index: 0}}}},
		},
	}

	if _, err := eng.ExecuteWorkflow(workflow, []model.DataItem{{JSON: map[string]interface{}{"seed": 1}}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	params := workflow.Nodes[1].Parameters
	if _, leaked := params["response.includeResponseHeadersAndStatus"]; leaked {
		t.Errorf("the synthesised flat alias leaked into the persisted parameters: %#v", params)
	}
	nested, ok := params["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected the nested shape to be preserved, got %#v", params["response"])
	}
	if got, ok := nested["includeResponseHeadersAndStatus"].(bool); !ok || !got {
		t.Errorf("expected the nested value to be preserved, got %#v", nested)
	}

	// The persisted workflow must still serialise to the n8n shape.
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to encode the persisted parameters: %v", err)
	}
	if string(encoded) != `{"response":{"includeResponseHeadersAndStatus":true}}` {
		t.Errorf("expected n8n-clean parameters, got %s", encoded)
	}
}

// TestHTTPRequestNodeFlatAndNestedShapesProduceIdenticalOutput exercises the
// real HTTP Request node: the `response.includeResponseHeadersAndStatus`
// option must have the same effect whether it is stored flat or nested.
func TestHTTPRequestNodeFlatAndNestedShapesProduceIdenticalOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "yes")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	baseParams := func() map[string]interface{} {
		return map[string]interface{}{
			"url":                   server.URL,
			"method":                "GET",
			"allowInternalRequests": true,
		}
	}

	flatParams := baseParams()
	flatParams["response.includeResponseHeadersAndStatus"] = true

	nestedParams := baseParams()
	nestedParams["response"] = map[string]interface{}{
		"includeResponseHeadersAndStatus": true,
	}

	flatNode := httpnode.NewHTTPRequestNode()
	flatOut, err := flatNode.Execute([]model.DataItem{{JSON: map[string]interface{}{"seed": 1}}}, NormalizeNodeParameters(flatParams))
	if err != nil {
		t.Fatalf("flat shape failed: %v", err)
	}

	nestedNode := httpnode.NewHTTPRequestNode()
	nestedOut, err := nestedNode.Execute([]model.DataItem{{JSON: map[string]interface{}{"seed": 1}}}, NormalizeNodeParameters(nestedParams))
	if err != nil {
		t.Fatalf("nested shape failed: %v", err)
	}

	if len(flatOut) != 1 || len(nestedOut) != 1 {
		t.Fatalf("expected one output item per shape, got %d and %d", len(flatOut), len(nestedOut))
	}
	if got, ok := flatOut[0].JSON["statusCode"].(int); !ok || got != 200 {
		t.Fatalf("expected the flat shape to enable the response metadata, got %#v", flatOut[0].JSON)
	}
	if got, ok := nestedOut[0].JSON["statusCode"].(int); !ok || got != 200 {
		t.Fatalf("expected the nested shape to enable the response metadata, got %#v", nestedOut[0].JSON)
	}
	if !reflect.DeepEqual(flatOut[0].JSON, nestedOut[0].JSON) {
		t.Errorf("flat and nested shapes diverged:\nflat   %#v\nnested %#v", flatOut[0].JSON, nestedOut[0].JSON)
	}
}
