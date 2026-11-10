package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/transform"
)

// Finnet post_balance reads `$json.data.attributes.route` off the
// ServiceRoutePlans HTTP Request output. That only works when the
// parsed JSON body is flattened into `$json` (n8n default).
func TestHTTPRequest_FlattensJSONBody_ForIsAsync(t *testing.T) {
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"matched":     true,
			"target_rule": "service-rules-order",
			"attributes":  map[string]interface{}{"route": "async"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	node := NewHTTPRequestNode()
	out, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, map[string]interface{}{
		"url":                   server.URL,
		"method":                "GET",
		"allowInternalRequests": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	if _, nested := out[0].JSON["json"]; nested {
		t.Fatalf("must not nest body under .json; got %v", out[0].JSON)
	}
	data, _ := out[0].JSON["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	if attrs["route"] != "async" {
		t.Fatalf("expected flattened data.attributes.route=async, got %v", out[0].JSON)
	}

	ifNode := transform.NewIfNode()
	ifOut, err := ifNode.Execute(out, map[string]interface{}{
		"conditions": map[string]interface{}{
			"combinator": "or",
			"conditions": []interface{}{map[string]interface{}{
				"leftValue":  "={{ $json.data.attributes.route.toLowerCase() }}",
				"operator":   map[string]interface{}{"operation": "equals", "type": "string"},
				"rightValue": "async",
			}},
			"options": map[string]interface{}{"caseSensitive": true, "typeValidation": "strict", "version": 3},
		},
	})
	if err != nil {
		t.Fatalf("IF: %v", err)
	}
	if len(ifOut) != 1 || ifOut[0].JSON["_ifResult"] != true {
		t.Fatalf("isAsync should be true on flattened HTTP output, got %v", ifOut)
	}
}
