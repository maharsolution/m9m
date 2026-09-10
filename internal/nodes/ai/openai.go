package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OpenAINode interacts with OpenAI API
type OpenAINode struct {
	*base.BaseNode
	httpClient   *http.Client
	otelManager  *otel.Manager
}

// SetOTelManager wires the OTel Manager into the node so the per-execution
// <agent>.generate span can be emitted. When manager is nil the node
// skips instrumentation. Calling after node construction is safe; the
// manager is only read on the request hot path.
func (n *OpenAINode) SetOTelManager(m *otel.Manager) {
	n.otelManager = m
}

// NewOpenAINode creates a new OpenAI node
func NewOpenAINode() *OpenAINode {
	return &OpenAINode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "OpenAI",
			Description: "Use OpenAI's GPT models for text generation and completion",
			Category:    "ai",
		}),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Execute processes input with OpenAI API. Spans emitted on the call:
//   - "<provider>.generate"  — the chat completion itself
//   - "execute_tool <name>"  — when the model returns tool_calls (rare
//     for OpenAI chat-completions; surfaced when the user wires the
//     responses/agents endpoint in the future).
func (n *OpenAINode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Get parameters
	apiKey := n.GetStringParameter(nodeParams, "apiKey", "")
	modelName := n.GetStringParameter(nodeParams, "model", "gpt-3.5-turbo")
	prompt := n.GetStringParameter(nodeParams, "prompt", "")
	maxTokens := n.GetIntParameter(nodeParams, "maxTokens", 1000)
	temperature := nodeParams["temperature"]
	if temperature == nil {
		temperature = 0.7
	}

	if apiKey == "" {
		return nil, fmt.Errorf("apiKey is required")
	}

	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	var results []model.DataItem

	for range inputData {
		// Prepare request payload
		messages := []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		}

		payload := map[string]interface{}{
			"model":       modelName,
			"messages":    messages,
			"max_tokens":  maxTokens,
			"temperature": temperature,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %v", err)
		}

		// Open a <provider>.generate span per invocation. The span
		// name follows the OpenTelemetry GenAI semantic convention
		// and uses `openai.generate` so trace viewers group calls
		// per-provider. Inputs / outputs are recorded conditionally
		// on the AgentsRecordInputs / AgentsRecordOutputs flags,
		// which the OTEL manager exposes on its Config.
		allowInputs := n.otelManager != nil && n.otelManager.Config().AgentsRecordInputs
		spanCtx, span := n.otelManager.StartAgentSpan(context.Background(), otel.AgentSpanOptions{
			AgentName:     "openai",
			ModelID:       "openai/" + modelName,
			Prompt:        prompt,
			InputsAllowed: allowInputs,
		})
		// Also stamp gen_ai.usage.* on the node span when the API
		// returns a usage block, so cost dashboards without per-call
		// parsing get totals for free.

		// Make API request
		req, err := http.NewRequestWithContext(spanCtx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		// Propagate the trace context to OpenAI's /v1/chat/completions
		// endpoint so the trace continues across the upstream call. The
		// GlobalInject helper routes through the SDK global propagator,
		// which is initialised to a no-op when tracing is disabled.
		otel.GlobalInject(spanCtx, otel.HeaderCarrier(req.Header))

		resp, err := n.httpClient.Do(req)
		if err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			apiErr := fmt.Errorf("openai API returned status %d", resp.StatusCode)
			endAgentSpan(n.otelManager, span, apiErr)
			return nil, apiErr
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to decode response: %v", err)
		}

		// Extract the generated text
		choices, ok := result["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			noChoices := fmt.Errorf("no choices in OpenAI response")
			endAgentSpan(n.otelManager, span, noChoices)
			return nil, noChoices
		}

		choice := choices[0].(map[string]interface{})
		message := choice["message"].(map[string]interface{})
		content, _ := message["content"].(string)

		// Stamp usage tokens on the agent span (if present).
		if usage, ok := result["usage"].(map[string]interface{}); ok {
			if pt, ok := usage["prompt_tokens"].(float64); ok {
				if ct, ok := usage["completion_tokens"].(float64); ok {
					otel.RecordUsage(span, int(pt), int(ct))
				}
			}
		}

		endAgentSpan(n.otelManager, span, nil)

		// Create result item
		resultItem := model.DataItem{
			JSON: map[string]interface{}{
				"prompt":      prompt,
				"response":    content,
				"model":       modelName,
				"usage":       result["usage"],
				"finishReason": choice["finish_reason"],
			},
		}

		results = append(results, resultItem)
	}

	return results, nil
}

// toFloat is a small helper that coerces the user-supplied temperature
// (which is json-unmarshalled as interface{} with possible float64,
// int, int64, or string) into a float64 suitable for the
// gen_ai.request.temperature attribute.
func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

// endAgentSpan is a tiny nil-safe wrapper around *Manager.EndAgentSpan so
// AI nodes that weren't initialised with a manager (e.g. embedded in
// unit tests, or used before serve.go called SetOTelManager) don't
// panic. The Manager method itself is already nil-safe; this wrapper
// protects the node struct's pointer too.
func endAgentSpan(m *otel.Manager, span oteltrace.Span, err error) {
	if m == nil {
		return
	}
	m.EndAgentSpan(span, err)
}
