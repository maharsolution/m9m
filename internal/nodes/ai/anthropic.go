package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/otel"
	"go.opentelemetry.io/otel/attribute"
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
//
// The endpoint is pluggable: the same wire shape (`/v1/messages`,
// `x-api-key`, `anthropic-version: 2023-06-01`) is spoken by Anthropic
// and by Anthropic-compatible providers such as MiniMax. Resolution
// order for the base URL:
//   1. `baseURL` node parameter (per-call override)
//   2. `provider` node parameter: "anthropic" (default) | "MiniMax"
//      — when set, looks up a built-in default endpoint for that
//      provider unless `baseURL` was also supplied
//   3. `M9M_MINIMAX_BASE_URL` env var (MiniMax-M3 cluster)
//   4. `M9M_ANTHROPIC_BASE_URL` env var (any compatible gateway)
//   5. https://api.anthropic.com/v1
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

	// Resolve the upstream endpoint. Either set `baseURL` directly,
	// or pick a named provider ("MiniMax" / "anthropic") to inherit
	// its default URL. Env-var defaults let operators flip the whole
	// deployment to MiniMax without editing any workflow JSON.
	baseURL := n.resolveBaseURL(nodeParams)

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

		// Open a <agent>.generate span per invocation. The AgentName
		// uses the actual provider so trace viewers can group calls
		// per upstream (anthropic vs. MiniMax) without having to
		// inspect the endpoint URL.
		allowInputs := n.otelManager != nil && n.otelManager.Config().AgentsRecordInputs
		providerTag := "anthropic"
		if isMiniMax(baseURL) {
			providerTag = "MiniMax"
		}
		spanCtx, span := n.otelManager.StartAgentSpan(context.Background(), otel.AgentSpanOptions{
			AgentName:     providerTag,
			ModelID:       providerTag + "/" + modelName,
			Prompt:        prompt,
			InputsAllowed: allowInputs,
		})
		// Tag the span with the resolved endpoint so the trace UI
		// shows where the call actually landed.
		if span != nil {
			span.SetAttributes(
				attribute.String("gen_ai.provider", providerTag),
				attribute.String("m9m.base_url", baseURL),
			)
		}

		// Make API request
		req, err := http.NewRequestWithContext(spanCtx, "POST", baseURL+"/messages", bytes.NewBuffer(jsonData))
		if err != nil {
			endAgentSpan(n.otelManager, span, err)
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		// Anthropic wire shape uses `x-api-key`; MiniMax and several
		// compatible gateways accept either that header or the
		// standard `Authorization: Bearer …`. Send both so the same
		// workflow runs against either provider without changes.
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
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
				"prompt":     prompt,
				"response":   text,
				"model":      modelName,
				"provider":   providerTag,
				"baseUrl":    baseURL,
				"usage":      result["usage"],
				"stopReason": result["stop_reason"],
			},
		}

		results = append(results, resultItem)
	}

	return results, nil
}

// defaultAnthropicBaseURL is the upstream Anthropic API root used
// when neither a node parameter nor an env var override is set.
const defaultAnthropicBaseURL = "https://api.anthropic.com/v1"

// defaultMiniMaxBaseURL is the Anthropic-compatible endpoint exposed
// by MiniMax-M3. The wire shape (`POST /v1/messages`,
// `x-api-key`/`Authorization`, `anthropic-version: 2023-06-01`) is
// identical to Anthropic's, so the same request body works.
const defaultMiniMaxBaseURL = "https://api.MiniMax.chat/v1"

// resolveBaseURL returns the upstream root for this invocation.
// See Execute docstring for the priority order. The string is
// trimmed and any trailing slash is stripped so callers can append
// "/messages" without worrying about double slashes.
func (n *AnthropicNode) resolveBaseURL(nodeParams map[string]interface{}) string {
	// 1. Explicit baseURL wins.
	if raw := strings.TrimSpace(n.GetStringParameter(nodeParams, "baseURL", "")); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	// 2. Named provider — built-in defaults so users only have to
	//    pick "MiniMax" or "anthropic" in the UI to flip the upstream.
	switch strings.ToLower(strings.TrimSpace(n.GetStringParameter(nodeParams, "provider", ""))) {
	case "MiniMax", "minimax":
		if u := strings.TrimSpace(os.Getenv("M9M_MINIMAX_BASE_URL")); u != "" {
			return strings.TrimRight(u, "/")
		}
		return defaultMiniMaxBaseURL
	}
	// 3. Env-var overrides for the default (Anthropic) provider —
	//    lets operators route the whole deployment through a proxy
	//    or a MiniMax gateway without editing any workflow.
	if u := strings.TrimSpace(os.Getenv("M9M_MINIMAX_BASE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(os.Getenv("M9M_ANTHROPIC_BASE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	// 4. Anthropic default.
	return defaultAnthropicBaseURL
}

// isMiniMax reports whether a resolved base URL belongs to MiniMax
// (either the built-in default or an operator-supplied override). The
// match is intentionally case-insensitive and tolerant of trailing
// paths so an override like `https://api.MiniMax.chat/anthropic`
// still routes to the MiniMax branch.
func isMiniMax(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	// Compare against lowercased needles — both sides must be in the
	// same case or the substring match silently misses.
	return strings.Contains(u, "minimax.chat") || strings.Contains(u, "minimax.")
}
