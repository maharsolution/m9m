package expressions

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/neul-labs/m9m/internal/model"
)

// parityC3Context builds the n8n-shaped expression context used by the
// C3 parity contract. It mirrors what the engine hands to a node
// executor: one inbound item, one upstream node result and the
// execution metadata.
func parityC3Context() *ExpressionContext {
	workflow := &model.Workflow{
		ID:   "parity-c3-workflow",
		Name: "Parity C3 Workflow",
		Nodes: []model.Node{
			{Name: "Webhook", Type: "n8n-nodes-base.webhook"},
		},
	}

	run := &RunExecutionData{
		ResultData: &RunData{
			NodeData: map[string][]NodeExecutionResult{
				"Webhook": {
					{Data: []model.DataItem{{JSON: map[string]interface{}{"seed": 1}}}},
				},
			},
		},
		ExecutionMode: ModeManual,
		StartedAt:     time.Now(),
	}

	return &ExpressionContext{
		Workflow:         workflow,
		RunExecutionData: run,
		RunIndex:         0,
		ItemIndex:        0,
		ActiveNodeName:   "Webhook",
		ConnectionInputData: []model.DataItem{{JSON: map[string]interface{}{
			"name": "John",
			"user": map[string]interface{}{
				"profile": map[string]interface{}{"name": "Jane"},
			},
			"headers": map[string]interface{}{
				"Content-Type": "application/json",
			},
			"items": []interface{}{"first", "second"},
		}}},
		Mode: ModeManual,
		AdditionalKeys: &AdditionalKeys{
			ExecutionId:           "exec-parity-c3",
			RestApiUrl:            "http://127.0.0.1:5678/api/v1",
			InstanceBaseUrl:       "http://127.0.0.1:5678",
			WebhookBaseUrl:        "http://127.0.0.1:5678/webhook",
			WebhookWaitingBaseUrl: "http://127.0.0.1:5678/webhook-waiting",
			WebhookTestBaseUrl:    "http://127.0.0.1:5678/webhook-test",
		},
	}
}

// parityC3Numeric reports whether a value is a number and returns it as
// float64 so numeric results can be compared regardless of the exact Go
// type goja produced (int64 vs float64).
func parityC3Numeric(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

// TestParityC3ExpressionContract pins the C3 expression contract of the
// parity plan: nested path access, bracket access, method invocation and
// the $json / $input / $now / $node / $execution context variables.
//
// The plan asks for an executable check; this file is that check. It
// only reads runtime behaviour — no production code is modified.
func TestParityC3ExpressionContract(t *testing.T) {
	evaluator := NewGojaExpressionEvaluator(DefaultEvaluatorConfig())
	context := parityC3Context()

	stringCases := []struct {
		name       string
		expression string
		expected   string
	}{
		{"json_root", "{{ $json.name }}", "John"},
		{"nested_path", "{{ $json.user.profile.name }}", "Jane"},
		{"bracket_access", "{{ $json.headers['Content-Type'] }}", "application/json"},
		{"bracket_method_call", "{{ $json.headers['Content-Type'].toLowerCase() }}", "application/json"},
		{"input_item_property", "{{ $input.item.json.name }}", "John"},
		{"input_first", "{{ $input.first().json.name }}", "John"},
		{"input_all_length", "{{ $input.all().length }}", "1"},
		{"node_reference", "{{ $node('Webhook').item.json.seed }}", "1"},
		{"execution_mode", "{{ $execution.mode }}", "manual"},
		{"execution_id", "{{ $execution.id }}", "exec-parity-c3"},
	}

	for _, tc := range stringCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := evaluator.EvaluateExpression(tc.expression, context)
			if err != nil {
				t.Fatalf("expression %q failed: %v", tc.expression, err)
			}
			text, ok := result.(string)
			if !ok {
				// Numeric expressions are compared numerically below; a
				// non-string, non-number result is a contract break.
				if number, isNumber := parityC3Numeric(result); isNumber {
					if fmt.Sprintf("%v", number) != tc.expected {
						t.Fatalf("expression %q = %v, want %s", tc.expression, number, tc.expected)
					}
					return
				}
				t.Fatalf("expression %q returned %T (%v), want string %q", tc.expression, result, result, tc.expected)
			}
			if text != tc.expected {
				t.Fatalf("expression %q = %q, want %q", tc.expression, text, tc.expected)
			}
		})
	}

	t.Run("now_is_iso_string", func(t *testing.T) {
		result, err := evaluator.EvaluateExpression("{{ $now.toISOString() }}", context)
		if err != nil {
			t.Fatalf("$now.toISOString() failed: %v", err)
		}
		text, ok := result.(string)
		if !ok {
			t.Fatalf("$now.toISOString() returned %T, want string", result)
		}
		if !strings.HasPrefix(text, "20") || len(text) < len("2026-01-01T00:00:00Z") {
			t.Fatalf("$now.toISOString() = %q, want an ISO-8601 timestamp", text)
		}
	})

	t.Run("now_to_millis_is_numeric", func(t *testing.T) {
		result, err := evaluator.EvaluateExpression("{{ $now.toMillis() }}", context)
		if err != nil {
			t.Fatalf("$now.toMillis() failed: %v", err)
		}
		millis, ok := parityC3Numeric(result)
		if !ok {
			t.Fatalf("$now.toMillis() returned %T, want a number", result)
		}
		if millis < 1_600_000_000_000 {
			t.Fatalf("$now.toMillis() = %v, want a millisecond epoch", millis)
		}
	})

	t.Run("input_item_array_index", func(t *testing.T) {
		result, err := evaluator.EvaluateExpression("{{ $json.items[1] }}", context)
		if err != nil {
			t.Fatalf("$json.items[1] failed: %v", err)
		}
		if result != "second" {
			t.Fatalf("$json.items[1] = %v, want 'second'", result)
		}
	})
}
