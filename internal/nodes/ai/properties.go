package ai

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the AI nodes (OpenAI, Anthropic). Each helper mirrors n8n's
// `INodeTypeDescription.properties` for the corresponding
// `@n8n/n8n-nodes-langchain.<node>` type.

func openAIProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "Chat", Value: "chat"},
		{Name: "Text Completion", Value: "completion"},
		{Name: "Embedding", Value: "embedding"},
		{Name: "Image", Value: "image"},
		{Name: "Audio", Value: "audio"},
		{Name: "File", Value: "file"},
		{Name: "Assistant", Value: "assistant"},
	}
	models := []base.Option{
		{Name: "gpt-4o", Value: "gpt-4o"},
		{Name: "gpt-4o-mini", Value: "gpt-4o-mini"},
		{Name: "gpt-4-turbo", Value: "gpt-4-turbo"},
		{Name: "gpt-3.5-turbo", Value: "gpt-3.5-turbo"},
		{Name: "text-embedding-3-small", Value: "text-embedding-3-small"},
		{Name: "dall-e-3", Value: "dall-e-3"},
		{Name: "whisper-1", Value: "whisper-1"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "chat", "OpenAI resource to use.", resources, true),
		base.StringOpt("Operation", "operation", "message", "Operation on the resource.", []base.Option{
			{Name: "Create", Value: "create"},
			{Name: "Message", Value: "message"},
			{Name: "Generate", Value: "generate"},
		}, true),
		base.StringOpt("Model", "model", "gpt-4o-mini", "Model to call.", models, true),
		base.TextProp("System Message", "systemMessage", "", "Optional system message to set the assistant's behaviour.", "", 4),
		base.TextProp("User Message", "userMessage", "", "User message / prompt.", "Hello from m9m!", 6),
		base.NumberProp("Temperature", "temperature", 0.7, "Sampling temperature (0.0-2.0).", false),
		base.NumberProp("Max Tokens", "maxTokens", 1024, "Upper bound on tokens in the response.", false),
		base.NumberProp("Top P", "topP", 1, "Nucleus sampling (0.0-1.0).", false),
		base.BoolProp("Stream", "stream", false, "Stream the response token-by-token (only chat / completion)."),
		base.JsonProp("Tools", "tools", "[]", "Tool definitions for tool-calling (chat only)."),
	}
	return append(props, base.CommonSettings()...)
}

func anthropicProperties() []base.NodeProperty {
	models := []base.Option{
		{Name: "claude-3-5-sonnet", Value: "claude-3-5-sonnet"},
		{Name: "claude-3-5-haiku", Value: "claude-3-5-haiku"},
		{Name: "claude-3-opus", Value: "claude-3-opus"},
		{Name: "claude-3-sonnet", Value: "claude-3-sonnet"},
		{Name: "claude-3-haiku", Value: "claude-3-haiku"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Model", "model", "claude-3-5-sonnet", "Anthropic model to call.", models, true),
		base.TextProp("System Prompt", "systemPrompt", "", "System prompt to set the assistant's behaviour.", "", 4),
		base.TextProp("User Message", "userMessage", "", "User message / prompt.", "Hello from m9m!", 6),
		base.NumberProp("Max Tokens", "maxTokens", 1024, "Upper bound on tokens in the response (required by Anthropic).", true),
		base.NumberProp("Temperature", "temperature", 1.0, "Sampling temperature (0.0-1.0).", false),
		base.NumberProp("Top P", "topP", 1, "Nucleus sampling (0.0-1.0).", false),
		base.NumberProp("Top K", "topK", 0, "Top-K sampling (0 = off).", false),
		base.BoolProp("Stream", "stream", false, "Stream the response token-by-token."),
		base.JsonProp("Tools", "tools", "[]", "Tool definitions for tool-calling."),
	}
	return append(props, base.CommonSettings()...)
}
