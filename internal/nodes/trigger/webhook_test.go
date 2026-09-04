package trigger

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestNormalizeWebhookHeaders pins down the lowercase / scalar-value
// shape we emit on the Webhook trigger output. n8n's webhook payload
// uses `{"content-type":"application/json"}` (lowercase key, scalar
// value); m9m previously emitted `{"Content-Type":["application/json"]}`.
// Downstream Set/IF/Function nodes that read `$json.headers.content-type`
// would silently miss because of the casing + shape mismatch.
func TestNormalizeWebhookHeaders(t *testing.T) {
	in := map[string][]string{
		"Content-Type":    {"application/json"},
		"X-Custom":        {"abc", "def"}, // multi-value: first wins
		"Accept-Encoding": {},
	}
	got := normalizeWebhookHeaders(in)

	if len(got) != 2 {
		t.Fatalf("Expected 2 entries (empty Accept-Encoding dropped), got %d: %#v", len(got), got)
	}
	if v, ok := got["content-type"]; !ok || v != "application/json" {
		t.Errorf("Expected lowercase content-type='application/json', got %#v", got)
	}
	if v, ok := got["x-custom"]; !ok || v != "abc" {
		t.Errorf("Expected x-custom='abc' (first value wins), got %#v", got)
	}
	if _, ok := got["Accept-Encoding"]; ok {
		t.Errorf("Expected no Accept-Encoding key (empty value dropped)")
	}
}

func TestNormalizeWebhookHeaders_Nil(t *testing.T) {
	got := normalizeWebhookHeaders(nil)
	if got == nil {
		t.Fatalf("Expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("Expected empty map, got %#v", got)
	}
}

func TestNormalizeHeaderLikeMap(t *testing.T) {
	in := map[string][]string{
		"page":  {"1"},
		"tags":  {"a", "b"},
		"empty": {},
	}
	got := normalizeHeaderLikeMap(in)
	if v, ok := got["page"].(string); !ok || v != "1" {
		t.Errorf("Expected page='1' as scalar, got %#v", got["page"])
	}
	if v, ok := got["tags"].([]string); !ok || len(v) != 2 {
		t.Errorf("Expected tags to remain an array, got %#v", got["tags"])
	}
	if _, ok := got["empty"]; ok {
		t.Errorf("Expected empty value dropped")
	}
}

func TestResolveWebhookURL_Default(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "")
	t.Setenv("WEBHOOK_URL", "")
	got := resolveWebhookURL("/foo")
	if got != "http://localhost:8080/foo" {
		t.Errorf("Expected default URL, got %q", got)
	}
}

func TestResolveWebhookURL_M9MEnvWins(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "https://hook.example.com")
	t.Setenv("WEBHOOK_URL", "https://other.example.com")
	got := resolveWebhookURL("/bocahtuanakal")
	if got != "https://hook.example.com/bocahtuanakal" {
		t.Errorf("Expected M9M_WEBHOOK_URL to win, got %q", got)
	}
}

func TestResolveWebhookURL_LegacyEnvFallback(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "")
	t.Setenv("WEBHOOK_URL", "https://legacy.example.com")
	got := resolveWebhookURL("/x")
	if got != "https://legacy.example.com/x" {
		t.Errorf("Expected legacy WEBHOOK_URL fallback, got %q", got)
	}
}

func TestResolveWebhookURL_StripsTrailingSlash(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "https://hook.example.com/")
	t.Setenv("WEBHOOK_URL", "")
	got := resolveWebhookURL("/x")
	if got != "https://hook.example.com/x" {
		t.Errorf("Expected trailing slash stripped, got %q", got)
	}
}

func TestResolveWebhookURL_NoLeadingSlash(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "")
	t.Setenv("WEBHOOK_URL", "")
	got := resolveWebhookURL("noslash")
	if got != "http://localhost:8080/noslash" {
		t.Errorf("Expected leading slash added, got %q", got)
	}
}

// TestWebhookNodeExecute_NormalizesHeadersAndAddsWebhookUrl exercises the
// full Execute path that the manager's engine run takes — verifying the
// trigger output is byte-equivalent to n8n's webhook payload shape
// (lowercase keys, scalar string values, webhookUrl field present).
func TestWebhookNodeExecute_NormalizesHeadersAndAddsWebhookUrl(t *testing.T) {
	t.Setenv("M9M_WEBHOOK_URL", "http://187.77.113.218:8080")
	t.Setenv("WEBHOOK_URL", "")

	node := NewWebhookNode()

	params := map[string]interface{}{
		"path":       "bocahtuanakal",
		"httpMethod": "POST",
	}

	// Simulate the input item the manager's prepareInputData builds.
	in := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"headers": map[string][]string{
					"Content-Type": {"application/json"},
					"User-Agent":   {"curl/8.21.0"},
				},
				"params": map[string][]string{},
				"query":  map[string][]string{},
				"body": map[string]interface{}{
					"varA": "5",
					"varB": "11110",
				},
				"method": "POST",
				"path":   "bocahtuanakal",
			},
		},
	}

	out, err := node.Execute(in, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(out))
	}

	item := out[0].JSON

	// Headers normalised: lowercase keys, scalar string values.
	headers, ok := item["headers"].(map[string]string)
	if !ok {
		t.Fatalf("Expected headers to be map[string]string, got %T", item["headers"])
	}
	if headers["content-type"] != "application/json" {
		t.Errorf("Expected lowercase content-type='application/json', got %#v", headers)
	}
	if headers["user-agent"] != "curl/8.21.0" {
		t.Errorf("Expected lowercase user-agent, got %#v", headers)
	}

	// Body passed through unchanged.
	body, ok := item["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected body to be map[string]interface{}, got %T", item["body"])
	}
	if body["varA"] != "5" || body["varB"] != "11110" {
		t.Errorf("Expected body passthrough, got %#v", body)
	}

	// webhookUrl present and full public URL.
	if url, ok := item["webhookUrl"].(string); !ok || url != "http://187.77.113.218:8080/bocahtuanakal" {
		t.Errorf("Expected webhookUrl='http://187.77.113.218:8080/bocahtuanakal', got %v", item["webhookUrl"])
	}

	// n8n parity: `method` and `path` are NOT exposed on the Webhook
	// trigger output (they are surfaced via `$env` instead). Verify
	// the trigger omits them.
	if _, hasMethod := item["method"]; hasMethod {
		t.Errorf("trigger output must not expose `method` (n8n parity), got %v", item["method"])
	}
	if _, hasPath := item["path"]; hasPath {
		t.Errorf("trigger output must not expose `path` (n8n parity), got %v", item["path"])
	}
}

// TestAuthHeadersLookup_AllShapes exercises the auth helper against
// the three header shapes that may be present on the manager's input:
//   - canonical http.Header (map[string][]string)
//   - already-normalised (map[string]string)
//   - mixed (map[string]interface{} with string or []string value)
//
// validateAuthentication is called *before* the trigger normalises
// headers, so it must read any of these shapes without choking.
func TestAuthHeadersLookup_AllShapes(t *testing.T) {
	cases := []struct {
		name    string
		headers interface{}
		key     string
		want    string
		wantOK  bool
	}{
		{
			name:    "http.Header shape, exact case",
			headers: map[string][]string{"Authorization": {"Basic abc"}},
			key:     "Authorization",
			want:    "Basic abc",
			wantOK:  true,
		},
		{
			name:    "http.Header shape, mixed case lookup",
			headers: map[string][]string{"X-Token": {"s3cret"}},
			key:     "x-token",
			want:    "s3cret",
			wantOK:  true,
		},
		{
			name:    "normalised string map",
			headers: map[string]string{"authorization": "Basic xyz"},
			key:     "Authorization",
			want:    "Basic xyz",
			wantOK:  true,
		},
		{
			name:    "interface map with string value",
			headers: map[string]interface{}{"X-Token": "s3cret"},
			key:     "x-token",
			want:    "s3cret",
			wantOK:  true,
		},
		{
			name:    "interface map with []string value",
			headers: map[string]interface{}{"X-Token": []string{"s3cret", "other"}},
			key:     "x-token",
			want:    "s3cret",
			wantOK:  true,
		},
		{
			name:    "missing key returns not-ok",
			headers: map[string]string{},
			key:     "X-Token",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "no headers key in request map",
			headers: nil,
			key:     "X-Token",
			want:    "",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := map[string]interface{}{}
			if tc.headers != nil {
				req["headers"] = tc.headers
			}
			got, ok := authHeadersLookup(req, tc.key)
			if ok != tc.wantOK {
				t.Errorf("authHeadersLookup ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("authHeadersLookup = %q, want %q", got, tc.want)
			}
		})
	}
}
