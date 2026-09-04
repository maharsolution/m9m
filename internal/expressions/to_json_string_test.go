package expressions

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
)

// TestToJsonStringPolyfill pins the contract that the goja runtime
// exposes n8n-compatible `toJsonString()` on every value. n8n wraps
// its JSON values in a Proxy that exposes this helper for
// XML/text passthrough expressions like
// `{{ $json.data.toJsonString() }}`. Without the polyfill those
// expressions fail with `TypeError: ... .toJsonString is not a
// function` and the upstream webhook XML test case fails.
func TestToJsonStringPolyfill(t *testing.T) {
	rt := NewSecureGojaRuntime(DefaultRuntimeConfig())

	ctx := &ExpressionContext{
		ConnectionInputData: []model.DataItem{
			{JSON: map[string]interface{}{"data": "<?xml version=\"1.0\"?><buku><id>001</id></buku>"}},
		},
		ItemIndex: 0,
	}
	if err := rt.SetupDataProxy(NewWorkflowDataProxy(ctx, rt.vm)); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Confirm the polyfill landed on the prototypes — direct
	// runtime exercise rather than going through the parser, since
	// the parser could rewrite or escape the literal value.
	cases := []struct {
		code string
		want string
	}{
		// n8n's string pass-through: `toJsonString()` on a string is
		// the raw string, NOT a JSON-quoted form. This matches the
		// upstream `webhook_xml` flow where `$json.data` is an XML
		// payload and the workflow expects the raw XML back from
		// `toJsonString()` rather than the JSON-encoded form.
		{`$json.data.toJsonString()`, `<?xml version="1.0"?><buku><id>001</id></buku>`},
		{`"<raw xml>".toJsonString()`, `<raw xml>`},
		{`(1).toJsonString()`, `1`},
		// Booleans come back as the literal n8n wire form.
		{`true.toJsonString()`, `true`},
		// Structured values keep the existing `JSON.stringify`
		// behaviour so other parity tests that rely on JSON-wrapped
		// objects continue to work.
		{`({a:1, b:"x"}).toJsonString()`, `{"a":1,"b":"x"}`},
	}

	for _, tc := range cases {
		v, err := rt.vm.RunString(tc.code)
		if err != nil {
			t.Errorf("code %q failed: %v", tc.code, err)
			continue
		}
		gs := v.Export()
		s, ok := gs.(string)
		if !ok {
			t.Errorf("code %q returned %T, want string", tc.code, gs)
			continue
		}
		if s != tc.want {
			t.Errorf("code %q = %q, want %q", tc.code, s, tc.want)
		}
	}
}
