package expressions

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestWorkflowDataProxy_NowProxyToISOString verifies the n8n-style
// `$now.toISOString()` expression resolves to an ISO-8601 string.
func TestWorkflowDataProxy_NowProxyToISOString(t *testing.T) {
	vm := goja.New()
	proxy := &WorkflowDataProxy{vm: vm}
	if err := proxy.createNowProxy().ToObject(vm).Set("__set", true); err != nil {
		t.Fatalf("noop: %v", err)
	}
	val := proxy.createNowProxy()
	if err := vm.Set("$now", val); err != nil {
		t.Fatalf("set $now: %v", err)
	}
	got, err := vm.RunString("$now.toISOString()")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	s := got.String()
	// RFC3339 / ISO-8601 sanity check: starts with YYYY-MM-DD.
	if !strings.HasPrefix(s, "20") || len(s) < len("2026-09-05T12:34:56Z") {
		t.Fatalf("$now.toISOString() returned unexpected value: %q", s)
	}
}

// TestWorkflowDataProxy_NowProxyToMillis verifies the n8n-style
// `$now.toMillis()` expression resolves to a numeric timestamp.
func TestWorkflowDataProxy_NowProxyToMillis(t *testing.T) {
	vm := goja.New()
	proxy := &WorkflowDataProxy{vm: vm}
	val := proxy.createNowProxy()
	if err := vm.Set("$now", val); err != nil {
		t.Fatalf("set $now: %v", err)
	}
	got, err := vm.RunString("$now.toMillis()")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got.ToInteger() <= 0 {
		t.Fatalf("expected positive millis, got %v", got)
	}
}

// TestWorkflowDataProxy_NowProxyFormat verifies the n8n-style
// `$now.format('yyyy-MM-dd')` expression resolves to a string
// using the Luxon/Moment-style pattern. This is the helper
// positive/7's body Set node uses to stamp `processedAt` for
// each loop iteration.
func TestWorkflowDataProxy_NowProxyFormat(t *testing.T) {
	vm := goja.New()
	proxy := &WorkflowDataProxy{vm: vm}
	val := proxy.createNowProxy()
	if err := vm.Set("$now", val); err != nil {
		t.Fatalf("set $now: %v", err)
	}
	cases := []struct {
		expr     string
		prefix   string
		length   int
		contains string
	}{
		{`$now.format('yyyy-MM-dd')`, "20", len("2026-09-08"), "-"},
		{`$now.format('yyyy-MM-dd HH:mm:ss')`, "20", len("2026-09-08 12:34:56"), ":"},
		{`$now.format('yyyy')`, "20", 4, ""},
	}
	for _, tc := range cases {
		got, err := vm.RunString(tc.expr)
		if err != nil {
			t.Fatalf("run %q: %v", tc.expr, err)
		}
		s := got.String()
		if !strings.HasPrefix(s, tc.prefix) {
			t.Errorf("%s expected to start with %q, got %q", tc.expr, tc.prefix, s)
		}
		if tc.contains != "" && !strings.Contains(s, tc.contains) {
			t.Errorf("%s expected to contain %q, got %q", tc.expr, tc.contains, s)
		}
		if len(s) != tc.length {
			t.Errorf("%s expected length %d, got %d (%q)", tc.expr, tc.length, len(s), s)
		}
	}
}
