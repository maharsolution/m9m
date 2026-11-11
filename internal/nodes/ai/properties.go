package ai

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// openAIProperties describes the OpenAI node.
//
// ai.OpenAINode.Execute reads `apiKey`, `model`, `prompt`,
// `maxTokens` and `temperature`. The old panel advertised n8n's
// resource/operation split (resource, operation, systemMessage,
// userMessage, topP, stream, tools) — none of those keys are read by
// the executor, so they were removed rather than kept as noise.
func openAIProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Secret("API key", "apiKey", "",
			"OpenAI API key. Falls back to the OPENAI_API_KEY environment variable when the engine injects one.",
			"sk-…", true),
		nodeprops.Str("Model", "model", "gpt-3.5-turbo",
			"Model that serves the completion.",
			"gpt-4o-mini", true),
		nodeprops.Required(nodeprops.Text("Prompt", "prompt", "",
			"Prompt sent to the model as the single user message.",
			"Hello from m9m!", 6)),
		nodeprops.Num("Max tokens", "maxTokens", 1000,
			"Upper bound on the tokens in the response.", false),
		nodeprops.Num("Temperature", "temperature", 0.7,
			"Sampling temperature (0.0-2.0). Higher values are more random.", false),
	}
	return append(props, nodeprops.Settings()...)
}

// anthropicProperties describes the Anthropic node.
//
// ai.AnthropicNode.Execute reads `apiKey`, `model`, `prompt`,
// `maxTokens`, `temperature` (raw node parameter), `provider` and
// `baseURL`. `provider` picks the built-in upstream
// ("anthropic" | "MiniMax"); `baseURL` overrides it per execution.
func anthropicProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Secret("API key", "apiKey", "",
			"Anthropic API key for the selected provider.",
			"sk-ant-…", true),
		nodeprops.Opt("Provider", "provider", "anthropic",
			"Which upstream serves the request.",
			[]base.Option{
				{Name: "Anthropic", Value: "anthropic"},
				{Name: "MiniMax", Value: "MiniMax"},
			}, false),
		nodeprops.Str("Base URL", "baseURL", "",
			"Override the upstream endpoint. Empty = the provider's built-in URL (or the M9M_MINIMAX_BASE_URL / M9M_ANTHROPIC_BASE_URL environment variables).",
			"https://api.anthropic.com/v1", false),
		nodeprops.Str("Model", "model", "claude-3-5-sonnet-20241022",
			"Model that serves the completion.",
			"claude-3-5-sonnet-20241022", true),
		nodeprops.Required(nodeprops.Text("Prompt", "prompt", "",
			"Prompt sent to the model as the single user message.",
			"Hello from m9m!", 6)),
		nodeprops.Num("Max tokens", "maxTokens", 1024,
			"Upper bound on the tokens in the response.", false),
		nodeprops.Num("Temperature", "temperature", 1.0,
			"Sampling temperature (0.0-2.0). Higher values are more random.", false),
	}
	return append(props, nodeprops.Settings()...)
}
