package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
)

// newExpressionContext builds the node-local execution context the HTTP
// Request node uses while evaluating parameters.
func newExpressionContext(item model.DataItem) *expressions.ExecutionContext {
	return &expressions.ExecutionContext{
		InputData: []model.DataItem{item},
		ItemIndex: 0,
		Variables: map[string]interface{}{
			"json": item.JSON,
		},
	}
}

// TestEvaluateParameters_NestedPath pins the G3 fix: the HTTP Request
// node must resolve a nested path such as
// `{{ $json.data.request.target_url }}`. The legacy (regex) evaluator
// only handled a single level, so the URL stayed unresolved and the
// request failed.
func TestEvaluateParameters_NestedPath(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"data": map[string]interface{}{
			"request": map[string]interface{}{
				"target_url": "https://api.example.com/v1/orders",
			},
		},
	}}

	params := map[string]interface{}{
		"method": "POST",
		"url":    "{{ $json.data.request.target_url }}",
	}

	evaluated, err := node.evaluateParameters(params, newExpressionContext(item))
	if err != nil {
		t.Fatalf("evaluateParameters: %v", err)
	}

	got, _ := evaluated["url"].(string)
	want := "https://api.example.com/v1/orders"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

// TestEvaluateParameters_BracketAccessAndMethod pins bracket access
// plus a JS method call, the shape the upstream Finnet workflows use:
// `{{ $json.headers['Content-Type'].toLowerCase() }}`.
func TestEvaluateParameters_BracketAccessAndMethod(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"headers": map[string]interface{}{
			"Content-Type": "APPLICATION/JSON",
		},
	}}

	params := map[string]interface{}{
		"contentType": "{{ $json.headers['Content-Type'].toLowerCase() }}",
	}

	evaluated, err := node.evaluateParameters(params, newExpressionContext(item))
	if err != nil {
		t.Fatalf("evaluateParameters: %v", err)
	}

	got, _ := evaluated["contentType"].(string)
	if got != "application/json" {
		t.Errorf("contentType = %q, want %q", got, "application/json")
	}
}

// TestEvaluateParameters_InterpolatedString checks that a URL built from
// an expression inside a larger literal still resolves.
func TestEvaluateParameters_InterpolatedString(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"host": "api.example.com",
		"id":   42,
	}}

	params := map[string]interface{}{
		"url": "https://{{ $json.host }}/orders/{{ $json.id }}",
	}

	evaluated, err := node.evaluateParameters(params, newExpressionContext(item))
	if err != nil {
		t.Fatalf("evaluateParameters: %v", err)
	}

	got, _ := evaluated["url"].(string)
	want := "https://api.example.com/orders/42"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

// TestEvaluateParameters_NumberBecomesString pins the n8n behaviour where
// a scalar expression result is stringified for string-typed fields.
// Without it a numeric URL/path silently became empty.
func TestEvaluateParameters_NumberBecomesString(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"id": 42,
	}}

	params := map[string]interface{}{
		"orderId": "{{ $json.id }}",
	}

	evaluated, err := node.evaluateParameters(params, newExpressionContext(item))
	if err != nil {
		t.Fatalf("evaluateParameters: %v", err)
	}

	got, ok := evaluated["orderId"].(string)
	if !ok {
		t.Fatalf("orderId is %T, want string", evaluated["orderId"])
	}
	if got != "42" {
		t.Errorf("orderId = %q, want %q", got, "42")
	}
}

// TestEvaluateParameters_FallsBackToLegacyEvaluator makes sure an
// expression goja cannot compile still gets a chance with the legacy
// evaluator, so older workflows keep working.
//
// `{{ $json.2fa }}` is the probe: a JSON key that starts with a digit is
// invalid JavaScript (`2fa` is not a valid identifier) so goja refuses
// to compile it, while the legacy evaluator resolves it through its
// path lookup.
func TestEvaluateParameters_FallsBackToLegacyEvaluator(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"2fa": "enabled",
	}}

	params := map[string]interface{}{
		"twoFactor": "{{ $json.2fa }}",
	}

	evaluated, err := node.evaluateParameters(params, newExpressionContext(item))
	if err != nil {
		t.Fatalf("evaluateParameters should fall back to the legacy evaluator, got: %v", err)
	}

	got, _ := evaluated["twoFactor"].(string)
	if got != "enabled" {
		t.Errorf("twoFactor = %q, want %q", got, "enabled")
	}
}

// TestEvaluateParameters_UnresolvableExpressionSurfacesError pins the
// other half of the fallback rule: when neither evaluator resolves an
// expression the goja error is surfaced, and the literal
// "{{ ... }}" never reaches the request layer.
func TestEvaluateParameters_UnresolvableExpressionSurfacesError(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{"a": "b"}}

	params := map[string]interface{}{
		"broken": "{{ $json.a. }}",
	}

	_, err := node.evaluateParameters(params, newExpressionContext(item))
	if err == nil {
		t.Fatal("expected an error for an unresolvable expression")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the offending parameter, got: %v", err)
	}
}

// TestHTTPRequestNode_Execute_ResolvesURLFromNestedPath is the
// end-to-end version of the G3 fix: the node must actually call the
// upstream URL that came from a nested expression.
func TestHTTPRequestNode_Execute_ResolvesURLFromNestedPath(t *testing.T) {
	var hitPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{
		"data": map[string]interface{}{
			"request": map[string]interface{}{
				"target_url": server.URL + "/resolved",
			},
		},
	}}

	params := map[string]interface{}{
		"method": "GET",
		"url":    "{{ $json.data.request.target_url }}",
		// SECURITY: allow the loopback test server.
		"allowInternalRequests": true,
	}

	if _, err := node.Execute([]model.DataItem{item}, params); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if hitPath != "/resolved" {
		t.Errorf("upstream path = %q, want %q", hitPath, "/resolved")
	}
}

// TestHTTPRequestNode_Execute_EmptyURLMessage verifies the error text
// tells the operator the expression did not resolve, instead of the
// bare "url parameter cannot be empty".
func TestHTTPRequestNode_Execute_EmptyURLMessage(t *testing.T) {
	node := NewHTTPRequestNode()

	item := model.DataItem{JSON: map[string]interface{}{"missing": nil}}

	params := map[string]interface{}{
		"method": "GET",
		"url":    "{{ $json.data.request.target_url }}",
	}

	_, err := node.Execute([]model.DataItem{item}, params)
	if err == nil {
		t.Fatal("expected an error for an unresolved url expression")
	}
	if !strings.Contains(err.Error(), "expression") {
		t.Errorf("error should mention the expression, got: %v", err)
	}
}

// TestValidateURLForSSRF_ExplainsProblem pins the G3 requirement that
// the SSRF error says what is wrong and shows the offending value.
func TestValidateURLForSSRF_ExplainsProblem(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantSubstr []string
	}{
		{
			name:       "missing scheme",
			url:        "api.example.com/path",
			wantSubstr: []string{"no scheme", "http://", "api.example.com/path"},
		},
		{
			name:       "unsupported scheme",
			url:        "file:///etc/passwd",
			wantSubstr: []string{"only http and https", "file", "/etc/passwd"},
		},
		{
			name:       "loopback host",
			url:        "http://127.0.0.1:8080/admin",
			wantSubstr: []string{"127.0.0.1"},
		},
		{
			name:       "cloud metadata",
			url:        "http://169.254.169.254/latest/meta-data/",
			wantSubstr: []string{"169.254.169.254"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURLForSSRF(tc.url)
			if err == nil {
				t.Fatalf("expected SSRF error for %q", tc.url)
			}
			for _, substr := range tc.wantSubstr {
				if !strings.Contains(err.Error(), substr) {
					t.Errorf("error %q should contain %q", err.Error(), substr)
				}
			}
		})
	}
}

// TestValidateURLForSSRF_AllowsPublicURL guards against over-blocking
// after the message rework.
func TestValidateURLForSSRF_AllowsPublicURL(t *testing.T) {
	if err := validateURLForSSRF("https://api.example.com/v1/orders"); err != nil {
		t.Errorf("public URL should be allowed, got %v", err)
	}
}
