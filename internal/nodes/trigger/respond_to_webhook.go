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
			Description: "Returns the workflow result to the webhook caller (used with Response Mode: Response Node).",
			Category:    "Trigger",
			Group:       []string{"trigger"},
			Properties:  respondToWebhookProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
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
// ResponseHeadersMetaKey is stamped onto the Respond-to-Webhook
// output item after header expressions are evaluated against the
// INPUT item (n8n evaluates `options.responseHeaders` with `$json`
// = the incoming item, e.g. `=application/{{ $json.respond_type }}`).
// The webhook manager reads this key then strips it before writing
// the body — it must not leak into the HTTP payload.
const ResponseHeadersMetaKey = "__m9mResponseHeaders"

func (r *RespondToWebhookNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if nodeParams == nil {
		nodeParams = map[string]interface{}{}
	}
	respondWith, _ := nodeParams["respondWith"].(string)
	headers := r.evaluateResponseHeaders(nodeParams, inputData)

	switch respondWith {
	case "firstIncomingItem":
		if len(inputData) == 0 {
			return []model.DataItem{withResponseHeaders(map[string]interface{}{}, headers)}, nil
		}
		return []model.DataItem{withResponseHeaders(copyJSON(inputData[0].JSON), headers)}, nil

	case "text", "json":
		raw, ok := nodeParams["responseBody"]
		if !ok {
			if len(inputData) == 0 {
				return []model.DataItem{withResponseHeaders(map[string]interface{}{}, headers)}, nil
			}
			return []model.DataItem{withResponseHeaders(copyJSON(inputData[0].JSON), headers)}, nil
		}
		if s, ok := raw.(string); ok && strings.HasPrefix(s, "=") {
			evaluated, err := r.evaluateResponseBody(s, inputData)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate responseBody expression: %w", err)
			}
			raw = evaluated
		}
		if respondWith == "text" {
			var body string
			switch v := raw.(type) {
			case string:
				body = v
			case nil:
				body = ""
			default:
				body = fmt.Sprintf("%v", v)
			}
			return []model.DataItem{withResponseHeaders(map[string]interface{}{"data": body}, headers)}, nil
		}
		body, err := normalizeResponseBody(raw)
		if err != nil {
			if s, ok := raw.(string); ok {
				return []model.DataItem{withResponseHeaders(map[string]interface{}{"data": s}, headers)}, nil
			}
			return nil, fmt.Errorf("invalid responseBody JSON: %w", err)
		}
		return []model.DataItem{withResponseHeaders(body, headers)}, nil

	default:
		if len(inputData) == 0 {
			return []model.DataItem{withResponseHeaders(map[string]interface{}{}, headers)}, nil
		}
		if len(headers) == 0 {
			return inputData, nil
		}
		out := make([]model.DataItem, len(inputData))
		for i, item := range inputData {
			out[i] = withResponseHeaders(copyJSON(item.JSON), headers)
			out[i].Binary = item.Binary
			out[i].PairedItem = item.PairedItem
		}
		return out, nil
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

// evaluateResponseHeaders resolves `options.responseHeaders` against
// the upstream input item. Finnet's Respond to Webhook sets
// Content-Type to `=application/{{ $json.respond_type }}`.
func (r *RespondToWebhookNode) evaluateResponseHeaders(nodeParams map[string]interface{}, inputData []model.DataItem) map[string]string {
	options, _ := nodeParams["options"].(map[string]interface{})
	if options == nil {
		return nil
	}
	rh, _ := options["responseHeaders"].(map[string]interface{})
	if rh == nil {
		return nil
	}
	entries := make([]map[string]string, 0)
	if rawEntries, ok := rh["entries"].([]interface{}); ok {
		for _, e := range rawEntries {
			m, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			value, _ := m["value"].(string)
			if name == "" {
				continue
			}
			entries = append(entries, map[string]string{"name": name, "value": value})
		}
	} else {
		for name, v := range rh {
			vs, _ := v.(string)
			entries = append(entries, map[string]string{"name": name, "value": vs})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		val := e["value"]
		if strings.HasPrefix(val, "=") || (strings.Contains(val, "{{") && strings.Contains(val, "}}")) {
			evaluated, err := r.evaluateResponseBody(val, inputData)
			if err == nil {
				val = fmt.Sprintf("%v", evaluated)
			}
		}
		out[e["name"]] = val
	}
	return out
}

func withResponseHeaders(json map[string]interface{}, headers map[string]string) model.DataItem {
	if json == nil {
		json = map[string]interface{}{}
	}
	if len(headers) > 0 {
		json[ResponseHeadersMetaKey] = headers
	}
	return model.DataItem{JSON: json}
}

func copyJSON(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
