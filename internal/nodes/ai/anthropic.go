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
)

// AnthropicNode interacts with Anthropic (Claude) API
type AnthropicNode struct {
	*base.BaseNode
	httpClient  *http.Client
	otelManager *otel.Manager
}

// SetOTelManager wires the OTel Manager into the node so the per-
// invocation <agent>.generate span can be emitted.
func (n *AnthropicNode) SetOTelManager(m *otel.Manager) {
	n.otelManager = m
}

// NewAnthropicNode creates a new Anthropic node
func NewAnthropicNode() *AnthropicNode {
	return &AnthropicNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Anthropic",
			Description: "Use Anthropic's Claude models for text generation and analysis",
			Category:    "ai",
		}),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Execute processes input with Anthropic API. Spans emitted on the call:
//   - "anthropic.generate" — the messages/create call
func (n *AnthropicNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Get parameters
	apiKey := n.GetStringParameter(nodeParams, "apiKey", "")
	modelName := n.GetStringParameter(nodeParams, "model", "claude-3-5-sonnet-20241022")
	prompt := n.GetStringParameter(nodeParams, "prompt", "")
	maxTokens := n.GetIntParameter(nodeParams, "maxTokens", 1024)
	temperature := nodeParams["temperature"]
	if temperature == nil {
		temperature = 1.0
	}

	if apiKey == "" {
		return nil, fmt.Errorf("apiKey is required")
	}

	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	var results []model.DataItem

	for range inputData {
		// Prepare request payload for Anthropic API
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

		// Open a <agent>.generate span per invocation.
		allowInputs := n.otelManager != nil && n.otelManager.Config().AgentsRecordInputs
		spanCtx, span := n.otelManager.StartAgentSpan(context.Background(), otel.AgentSpanOptions{
			AgentName:     "anthropic",
			ModelID:       "anthropic/" + modelName,
			Prompt:        prompt,
			InputsAllowed: allowInputs,
		})

		// Make API request
		req, err := http.NewRequestWithContext(spanCtx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
		if err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		// Inject trace context so Anthropic-side traces (when their
		// endpoints expose one) chain onto ours. With a global
		// no-op propagator this is essentially free.
		otel.GlobalInject(spanCtx, otel.HeaderCarrier(req.Header))

		resp, err := n.httpClient.Do(req)
		if err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			apiErr := fmt.Errorf("anthropic API returned status %d", resp.StatusCode)
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
		content, ok := result["content"].([]interface{})
		if !ok || len(content) == 0 {
			noContent := fmt.Errorf("no content in Anthropic response")
			endAgentSpan(n.otelManager, span, noContent)
			return nil, noContent
		}

		firstContent := content[0].(map[string]interface{})
		text, _ := firstContent["text"].(string)

		// Stamp usage tokens on the agent span when the API returns
		// a usage block.
		if usage, ok := result["usage"].(map[string]interface{}); ok {
			if it, ok := usage["input_tokens"].(float64); ok {
				if ot, ok := usage["output_tokens"].(float64); ok {
					otel.RecordUsage(span, int(it), int(ot))
				}
			}
		}
		endAgentSpan(n.otelManager, span, nil)

		// Create result item
		resultItem := model.DataItem{
			JSON: map[string]interface{}{
				"prompt":      prompt,
				"response":    text,
				"model":       modelName,
				"usage":       result["usage"],
				"stopReason":  result["stop_reason"],
			},
		}

		results = append(results, resultItem)
	}

	return results, nil
}
