package trigger

import (
	"strings"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestRespondToWebhook_EvaluatesResponseBodyExpression verifies that a
// `responseBody` parameter starting with `=` is run through the
// expression evaluator instead of being returned verbatim. This is
// what makes the `webhook_xml` parity test pass — n8n writes
// `={{ $json.data.toJsonString() }}` and expects the workflow's XML
// string back, not the literal expression text.
func TestRespondToWebhook_EvaluatesResponseBodyExpression(t *testing.T) {
	node := NewRespondToWebhookNode()

	xml := `<?xml version="1.0"?><buku><id>001</id><judul>Belajar</judul></buku>`
	input := []model.DataItem{
		{JSON: map[string]interface{}{"data": xml}},
	}
	params := map[string]interface{}{
		"respondWith":  "json",
		"responseBody": "={{ $json.data.toJsonString() }}",
	}

	out, err := node.Execute(input, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out))
	}
	got, ok := out[0].JSON["data"].(string)
	if !ok {
		t.Fatalf("expected data to be a string, got %T (%v)", out[0].JSON["data"], out[0].JSON["data"])
	}
	// n8n's `toJsonString()` on a string returns the raw string
	// (no JSON quoting). The webhook manager reads `data` out of
	// NodeOutputs and forwards it verbatim as the response body,
	// so the XML has to land here unwrapped.
	if !strings.Contains(got, "<buku>") || !strings.Contains(got, "<id>001</id>") {
		t.Fatalf("expected raw XML in data, got %q", got)
	}
	if strings.HasPrefix(got, `"`) {
		t.Fatalf("expected raw XML (no JSON quoting), got %q", got)
	}
}

// TestRespondToWebhook_NonExpressionResponseBodyPassesThrough ensures
// that a literal JSON-shaped `responseBody` (no `=` prefix) still
// flows through unchanged. The XML fallback branch already covers
// raw XML strings; this guards the more common "user pasted a JSON
// object into the editor" path.
func TestRespondToWebhook_NonExpressionResponseBodyPassesThrough(t *testing.T) {
	node := NewRespondToWebhookNode()
	input := []model.DataItem{
		{JSON: map[string]interface{}{"foo": "bar"}},
	}
	params := map[string]interface{}{
		"respondWith":  "json",
		"responseBody": map[string]interface{}{"hello": "world"},
	}

	out, err := node.Execute(input, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := out[0].JSON["hello"]; got != "world" {
		t.Fatalf("expected hello=world, got %v", got)
	}
}

// TestRespondToWebhook_ExpressionWithEmptyInput makes sure the
// expression path doesn't crash when the upstream produced no items
// at all. n8n still evaluates the expression but `$json` resolves to
// null, and the result should propagate without panicking.
func TestRespondToWebhook_ExpressionWithEmptyInput(t *testing.T) {
	node := NewRespondToWebhookNode()
	params := map[string]interface{}{
		"respondWith":  "json",
		"responseBody": "={{ $json }}",
	}

	out, err := node.Execute(nil, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out))
	}
	if out[0].JSON == nil {
		t.Fatalf("expected non-nil JSON map")
	}
}
