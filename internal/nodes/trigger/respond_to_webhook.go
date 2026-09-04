package trigger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// RespondToWebhookNode implements the "Respond to Webhook" node that
// n8n uses together with `responseMode: responseNode` on a Webhook
// trigger. The workflow runtime wires the trigger's response output
// to whatever the Respond-to-Webhook node produced, so the
// downstream caller reads the exact body the workflow produced.
//
// m9m executes this node as a pass-through (it forwards its input
// verbatim) so the webhook manager can read the produced body out of
// the engine's `ExecutionResult.NodeOutputs` map (keyed by node
// name) and return it as the HTTP response. This matches n8n's wire
// shape — n8n's Respond-to-Webhook node is a "marker" that tells
// the runtime "use my output as the webhook body", not a transformer
// that mutates the data.
//
// Parameter compatibility: n8n writes the user-facing response body
// into a `respondWith` parameter that can be one of:
//
//   - "allIncomingItems" (default) — emit every input item verbatim
//   - "firstIncomingItem"           — emit only the first input item
//   - "json"                        — emit a literal JSON body
//     (n8n wires the user-supplied JSON into `responseBody`)
//   - "binary" / "text"             — emit raw bytes (out of scope
//     for the simple parity tests)
//
// For the parity fix that landed first (workflow
// `V4432EsGIkpqIZx9` — "Simple Webhook - Code"), n8n writes
// `respondWith: "json"` plus a `responseBody` JSON object whose
// keys mirror the JSON the workflow produced upstream. The body
// value is parsed once at Execute time and surfaced as the
// single-output DataItem so `prepareResponse` reads it back out of
// `NodeOutputs`.
//
// Expression handling: when `responseBody` is a string starting with
// `=` (the n8n convention for "this is an expression, evaluate it")
// the value is run through the Goja expression evaluator using the
// upstream input data as `$json` context. This is required for the
// `webhook_xml` parity test, where the workflow uses
// `={{ $json.data.toJsonString() }}` to stringify an XML payload
// before it is returned to the caller. Without this step the literal
// expression string would be emitted instead of the evaluated value.
type RespondToWebhookNode struct {
	*base.BaseNode
	evaluator *expressions.GojaExpressionEvaluator
}

// NewRespondToWebhookNode creates a new Respond to Webhook node.
func NewRespondToWebhookNode() *RespondToWebhookNode {
	return &RespondToWebhookNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Respond to Webhook",
			Description: "Returns data from the workflow back to the webhook caller (responseMode: responseNode)",
			Category:    "Trigger",
		}),
		evaluator: expressions.NewGojaExpressionEvaluator(expressions.DefaultEvaluatorConfig()),
	}
}

// ValidateParameters accepts any (or no) parameter set. The
// Respond-to-Webhook node is a marker; invalid parameter values
// (e.g. `respondWith: "json"` without a `responseBody`) surface as
// execution-time errors in Execute, not validation errors, so the
// workflow editor can still save an in-progress workflow.
func (r *RespondToWebhookNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	if rw, ok := params["respondWith"]; ok {
		if _, ok := rw.(string); !ok {
			return fmt.Errorf("respondWith must be a string")
		}
	}
	return nil
}

// Execute returns the body the workflow wants to send back to the
// webhook caller. The shape of the output depends on `respondWith`:
//
//   - "" / "allIncomingItems": pass-through (one output item per input)
//   - "firstIncomingItem": first input item only (matches n8n's wire shape)
//   - "json": parse `responseBody` into a JSON object and emit it
//     as a single DataItem — the literal body n8n would write back
//     to the webhook caller
//
// Anything else (e.g. "binary", "text") falls back to pass-through
// so the workflow stays alive; the webhook manager will still emit
// the right shape because it reads NodeOutputs directly.
func (r *RespondToWebhookNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if nodeParams == nil {
		nodeParams = map[string]interface{}{}
	}
	respondWith, _ := nodeParams["respondWith"].(string)

	switch respondWith {
	case "firstIncomingItem":
		if len(inputData) == 0 {
			return []model.DataItem{{JSON: map[string]interface{}{}}}, nil
		}
		return []model.DataItem{inputData[0]}, nil

	case "json":
		raw, ok := nodeParams["responseBody"]
		if !ok {
			// No explicit body — pass-through so the caller still
			// gets a valid JSON object instead of an empty 200.
			if len(inputData) == 0 {
				return []model.DataItem{{JSON: map[string]interface{}{}}}, nil
			}
			return inputData, nil
		}
		// If the user supplied `responseBody` as a string starting with
		// `=`, n8n interprets the rest as a JavaScript expression. Run
		// it through the Goja evaluator against the upstream `$json`
		// context so the response body reflects the actual data the
		// workflow produced (e.g. `={{ $json.data.toJsonString() }}`)
		// instead of returning the literal expression text.
		if s, ok := raw.(string); ok && strings.HasPrefix(s, "=") {
			evaluated, err := r.evaluateResponseBody(s, inputData)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate responseBody expression: %w", err)
			}
			raw = evaluated
		}
		body, err := normalizeResponseBody(raw)
		if err != nil {
			// Non-JSON string body (XML, plain text, etc.). n8n
			// evaluates the `responseBody` expression and emits the
			// literal result back to the webhook caller regardless of
			// whether it's valid JSON. Mirror that behaviour so XML
			// and text responses work even when the workflow author
			// used `respondWith: "json"` out of habit. The webhook
			// manager reads `data` back out of NodeOutputs and writes
			// it as the raw response body when the value is a string.
			if s, ok := raw.(string); ok {
				return []model.DataItem{{JSON: map[string]interface{}{"data": s}}}, nil
			}
			return nil, fmt.Errorf("invalid responseBody JSON: %w", err)
		}
		return []model.DataItem{{JSON: body}}, nil

	default:
		// "" or "allIncomingItems" or unknown → pass-through.
		if len(inputData) == 0 {
			return []model.DataItem{{JSON: map[string]interface{}{}}}, nil
		}
		return inputData, nil
	}
}

// evaluateResponseBody runs an n8n `={{ ... }}` expression against the
// given input items. The leading `=` is stripped before evaluation
// because Goja's expression parser treats the rest as a pure
// expression (n8n's `=` prefix means "no further interpolation, the
// whole thing is code"). The first input item is exposed as `$json`
// to match the parity test convention — webhook flows run a single
// item at a time and the workflows that depend on this code path
// reference upstream output by field rather than by item index.
//
// Returns the evaluated value coerced to a string when the result
// isn't already JSON-shaped. The respond-to-webhook contract is
// either a JSON object (one Map entry per top-level field) or a
// string (wrapped under `data` for the manager to forward verbatim
// as the raw HTTP body), so anything that survives JSON.stringify is
// fine here — the value is forwarded through `data` so XML/text
// bodies stay intact.
func (r *RespondToWebhookNode) evaluateResponseBody(expr string, inputData []model.DataItem) (interface{}, error) {
	// Build the ExpressionContext. We pass all upstream items through
	// so n8n-style helpers like `$first()` / `$last()` work, but the
	// helper-set in the Goja evaluator defaults to `$json` resolving
	// to the first item which is what `responseBody` expressions
	// expect on a webhook responseNode.
	if len(inputData) == 0 {
		// Empty upstream — n8n still evaluates the expression but
		// `$json` resolves to `null`. Surface a stable empty item so
		// the evaluator can still produce a deterministic value.
		inputData = []model.DataItem{{JSON: map[string]interface{}{}}}
	}

	context := &expressions.ExpressionContext{
		ActiveNodeName:      "Respond to Webhook",
		RunIndex:            0,
		ItemIndex:           0,
		Mode:                expressions.ModeManual,
		ConnectionInputData: inputData,
		Workflow: &model.Workflow{
			Name: "Respond to Webhook",
		},
		AdditionalKeys: &expressions.AdditionalKeys{
			ExecutionId: "respond-to-webhook",
		},
	}

	evaluated, err := r.evaluator.EvaluateExpression(strings.TrimPrefix(expr, "="), context)
	if err != nil {
		return nil, err
	}

	// If the expression already returned a structured value (map,
	// slice, etc.), hand it through as-is so `normalizeResponseBody`
	// keeps the shape. Otherwise stringify primitives so they flow
	// through the `data` wrapper as a plain string body.
	switch v := evaluated.(type) {
	case map[string]interface{}, []interface{}, nil:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// normalizeResponseBody coerces the `responseBody` parameter into a
// JSON-shaped map. n8n's UI ships it as either a parsed JSON object
// (when the user typed JSON in the editor) or a string (when the
// workflow was exported from the legacy editor or hand-written).
// Both shapes need to produce the same output so the wire byte
// shape matches n8n.
func normalizeResponseBody(raw interface{}) (map[string]interface{}, error) {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v, nil
	case map[string]string:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			out[k] = val
		}
		return out, nil
	case string:
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported responseBody type %T", raw)
	}
}
