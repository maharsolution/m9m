package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

func TestHTTPRequest_UsesJsonBody_SpecifyBodyJSON(t *testing.T) {
	var gotBody string
	var gotCT string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"attributes":{"route":"async"},"target_rule":"service-rules-order"}}`)
	}))
	defer server.Close()

	node := NewHTTPRequestNode()
	out, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, map[string]interface{}{
		"url":                   server.URL,
		"method":                "POST",
		"sendBody":              true,
		"specifyBody":           "json",
		"jsonBody":              `{"plan_code":"service-rule-sanity","parameters":{"url":"http://example/webhook/post_balance"}}`,
		"allowInternalRequests": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotBody == "" {
		t.Fatal("expected jsonBody to be posted; got empty body")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("posted body not JSON: %v (%q)", err, gotBody)
	}
	if parsed["plan_code"] != "service-rule-sanity" {
		t.Fatalf("unexpected posted body: %v", parsed)
	}
	if gotCT == "" || gotCT[:16] != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", gotCT)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out))
	}
	data, _ := out[0].JSON["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	if attrs["route"] != "async" {
		t.Fatalf("expected flattened route=async in output, got %v", out[0].JSON)
	}
}
