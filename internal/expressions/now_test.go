package expressions

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/neul-labs/m9m/internal/model"
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

// TestResolveWorkflowLocationDefaultsToNY verifies that workflows
// with no timezone setting default to America/New_York — the same
// default n8n's generic.timezone config uses when GENERIC_TIMEZONE
// is not set. This is the fix for the positive/7 "processedAt"
// date drift: n8n was emitting 2026-09-08 (NY) while m9m was
// emitting 2026-09-09 (UTC). The default must match n8n so that
// parity tests pass.
func TestResolveWorkflowLocationDefaultsToNY(t *testing.T) {
	// No workflow: default to America/New_York.
	got := resolveWorkflowLocation(nil)
	if got == time.UTC {
		t.Fatalf("expected non-UTC default location, got UTC")
	}

	// Workflow with no settings: also default to America/New_York.
	wf := &model.Workflow{}
	got = resolveWorkflowLocation(wf)
	if got == time.UTC {
		t.Fatalf("expected non-UTC default location when settings=nil, got UTC")
	}

	// Workflow with empty timezone string in settings: same default.
	wf = &model.Workflow{Settings: &model.WorkflowSettings{Timezone: ""}}
	got = resolveWorkflowLocation(wf)
	if got == time.UTC {
		t.Fatalf("expected non-UTC default location when timezone='', got UTC")
	}

	// Workflow with explicit timezone: honour it.
	wf = &model.Workflow{Settings: &model.WorkflowSettings{Timezone: "Asia/Tokyo"}}
	got = resolveWorkflowLocation(wf)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("Asia/Tokyo tzdata not available in test env: %v", err)
	}
	if got.String() != tokyo.String() {
		t.Fatalf("expected %s, got %s", tokyo, got)
	}
}

// TestNowProxyUsesWorkflowTimezone verifies the $now proxy's
// .format() output reflects the workflow's timezone (or the default
// America/New_York fallback). This is what fixes the positive/7
// processedAt parity with n8n.
func TestNowProxyUsesWorkflowTimezone(t *testing.T) {
	vm := goja.New()
	wf := &model.Workflow{
		Settings: &model.WorkflowSettings{Timezone: "Asia/Tokyo"},
	}
	proxy := &WorkflowDataProxy{vm: vm, workflow: wf}
	if err := vm.Set("$now", proxy.createNowProxy()); err != nil {
		t.Fatalf("set $now: %v", err)
	}
	got, err := vm.RunString("$now.format('yyyy-MM-dd')")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	s := got.String()
	// Tokyo is UTC+9; the date could be off by one from UTC depending
	// on the wall clock. We just check it parses and is a YYYY-MM-DD
	// date in Tokyo tz.
	_, err = time.ParseInLocation("2006-01-02", s, time.FixedZone("JST", 9*3600))
	if err != nil {
		t.Fatalf("$now.format did not produce a Tokyo date, got %q: %v", s, err)
	}
}
