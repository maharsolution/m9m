package transform

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestSetNodeRawModeReplacesItemBody covers the n8n Set node in
// "raw" mode: the whole item body comes from the `jsonOutput`
// parameter (a JSON *string*) and the upstream fields are dropped.
//
// This is the shape n8n writes for the "Integrasi Aiven" workflow:
//
//	{"mode":"raw","jsonOutput":"{\n  \"my_field_1\": \"1\",\n  \"my_field_2\": 1000\n}\n","options":{}}
//
// Before the fix the node fell through to the assignment-based path,
// found no `assignments` key, and emitted an empty `{}` item — so
// the downstream Kafka producer published `{}` instead of the
// configured payload.
func TestSetNodeRawModeReplacesItemBody(t *testing.T) {
	node := NewSetNode()

	params := map[string]interface{}{
		"mode":       "raw",
		"jsonOutput": "{\n  \"my_field_1\": \"1\",\n  \"my_field_2\": 1000\n}\n",
		"options":    map[string]interface{}{},
	}

	if err := node.ValidateParameters(params); err != nil {
		t.Fatalf("raw mode should validate: %v", err)
	}

	input := []model.DataItem{{JSON: map[string]interface{}{"upstream": "should be dropped"}}}

	out, err := node.Execute(input, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}

	if got, ok := out[0].JSON["my_field_1"].(string); !ok || got != "1" {
		t.Errorf("expected my_field_1 == \"1\", got %#v", out[0].JSON["my_field_1"])
	}
	if got, ok := out[0].JSON["my_field_2"].(float64); !ok || got != 1000 {
		t.Errorf("expected my_field_2 == 1000, got %#v", out[0].JSON["my_field_2"])
	}
	if _, present := out[0].JSON["upstream"]; present {
		t.Errorf("raw mode must replace the upstream body, got %#v", out[0].JSON)
	}
}

// TestSetNodeRawModeAcceptsObjectJSONOutput covers the variant where
// `jsonOutput` is already an object (programmatic workflows) rather
// than a JSON string.
func TestSetNodeRawModeAcceptsObjectJSONOutput(t *testing.T) {
	node := NewSetNode()

	params := map[string]interface{}{
		"mode": "raw",
		"jsonOutput": map[string]interface{}{
			"my_field_1": "1",
		},
	}

	out, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out[0].JSON["my_field_1"] != "1" {
		t.Errorf("expected my_field_1 == \"1\", got %#v", out[0].JSON)
	}
}

// TestSetNodeRawModeInvalidJSONFails makes sure a malformed
// `jsonOutput` is reported instead of silently producing an empty
// item.
func TestSetNodeRawModeInvalidJSONFails(t *testing.T) {
	node := NewSetNode()

	params := map[string]interface{}{
		"mode":       "raw",
		"jsonOutput": "{not json",
	}

	if err := node.ValidateParameters(params); err == nil {
		t.Fatal("expected validation error for malformed jsonOutput")
	}
	if _, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, params); err == nil {
		t.Fatal("expected execution error for malformed jsonOutput")
	}
}

// TestSetNodeRawModeInferredFromJSONOutput covers the n8n export
// shape where `mode` is absent but `jsonOutput` is present — the
// raw-mode path must still be taken.
func TestSetNodeRawModeInferredFromJSONOutput(t *testing.T) {
	node := NewSetNode()

	params := map[string]interface{}{
		"jsonOutput": `{"hello":"world"}`,
	}

	out, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{"old": true}}}, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out[0].JSON["hello"] != "world" {
		t.Errorf("expected hello == world, got %#v", out[0].JSON)
	}
}
