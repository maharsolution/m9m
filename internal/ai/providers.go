package ai

// ProviderInfo describes an AI provider: the user-facing label, a list
// of well-known model IDs (so the Settings UI can offer a dropdown),
// and the default base URL the runtime falls back to when no override
// is supplied.
//
// We deliberately hand-maintain this catalog rather than fetch it from
// each provider's `/models` endpoint: (a) the UI needs it offline
// before it knows whether the operator has a working API key, (b) the
// set of supported models changes much more slowly than a live call
// would imply, (c) it keeps the runtime free of additional outbound
// calls during Settings load.
type ProviderInfo struct {
	ID             Provider `json:"id"`
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	DefaultBaseURL string   `json:"defaultBaseUrl"`
	DefaultModel   string   `json:"defaultModel"`
	Models         []string `json:"models"`
	RequiresKey    bool     `json:"requiresKey"`
}

// Providers is the canonical list of providers the Settings UI should
// surface, in display order. Order matters: the first entry is the
// default for new users and the first option in dropdowns.
//
// To add a new provider:
//   1. Add a Provider constant to ai.go
//   2. Add an entry here (with the right `DefaultBaseURL` and at least
//      one model)
//   3. Add a `case ProviderX:` to callLLM in ai.go that knows how to
//      talk to it
//   4. (Optional) Add any provider-specific env vars to
//      LoadConfigFromEnv in config.go
//   5. Add a test in api_test.go covering the new provider id
//   6. Mirror the entry in web/src/api/ai.ts → PROVIDER_CATALOG so the
//      UI dropdown stays in sync
var Providers = []ProviderInfo{
	{
		ID:             ProviderOpenAI,
		Label:          "OpenAI",
		Description:    "GPT-4o, GPT-4.1, o1, o3 — OpenAI's flagship chat models.",
		DefaultBaseURL: "https://api.openai.com/v1",
		DefaultModel:   "gpt-4o",
		Models: []string{
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4.1",
			"gpt-4.1-mini",
			"gpt-4-turbo",
			"o1",
			"o1-mini",
			"o3",
			"o3-mini",
			"gpt-3.5-turbo",
		},
		RequiresKey: true,
	},
	{
		ID:             ProviderAnthropic,
		Label:          "Anthropic (Claude)",
		Description:    "Claude Opus 4.1, Sonnet 4.5, Haiku 4.5 — Anthropic's flagship chat models.",
		DefaultBaseURL: "https://api.anthropic.com/v1",
		DefaultModel:   "claude-sonnet-4-5",
		Models: []string{
			"claude-opus-4-1",
			"claude-sonnet-4-5",
			"claude-haiku-4-5",
			"claude-3-7-sonnet-latest",
			"claude-3-5-sonnet-latest",
			"claude-3-5-haiku-latest",
			"claude-3-opus-latest",
		},
		RequiresKey: true,
	},
	{
		ID:             ProviderMiniMax,
		Label:          "MiniMax (Anthropic-compatible)",
		Description:    "Anthropic-compatible endpoint exposed by MiniMax; choose the model id MiniMax assigned.",
		DefaultBaseURL: "https://api.minimax.chat/v1",
		DefaultModel:   "MiniMax-M3",
		Models: []string{
			"MiniMax-M3",
			"minimax-text-01",
		},
		RequiresKey: true,
	},
	{
		ID:             ProviderOllama,
		Label:          "Ollama (local)",
		Description:    "Run any open-source model locally; no API key required.",
		DefaultBaseURL: "http://localhost:11434",
		DefaultModel:   "llama3.2",
		Models: []string{
			"llama3.2",
			"llama3.1",
			"qwen2.5",
			"mistral",
			"mixtral",
			"gemma2",
			"phi3",
			"codellama",
		},
		RequiresKey: false,
	},
}

// ProviderInfoByID returns the catalog entry for provider id, or a
// zero-value ProviderInfo + false when the id is not known. Used by
// the API handler to attach human-readable labels to the ConfigView
// payload.
func ProviderInfoByID(p Provider) (ProviderInfo, bool) {
	for _, info := range Providers {
		if info.ID == p {
			return info, true
		}
	}
	return ProviderInfo{}, false
}

// DefaultModelFor returns the catalog's default model id for provider
// p, falling back to the package-wide DefaultConfig model when the
// provider is unknown. Used by the Settings UI to pre-fill the model
// field after the operator picks a provider.
func DefaultModelFor(p Provider) string {
	if info, ok := ProviderInfoByID(p); ok && info.DefaultModel != "" {
		return info.DefaultModel
	}
	return DefaultConfig().Model
}

// DefaultBaseURLFor returns the catalog's default base URL for
// provider p, or "" when the provider is unknown. The caller is
// expected to use this only for display — the actual URL the runtime
// hits is computed inside *AI based on the resolved Config.
func DefaultBaseURLFor(p Provider) string {
	if info, ok := ProviderInfoByID(p); ok {
		return info.DefaultBaseURL
	}
	return ""
}

// RequiresAPIKeyFor reports whether the provider needs an API key.
// Ollama returns false; everything else returns true. The UI uses
// this to gate the "API key required" hint and to skip the field for
// local Ollama setups.
func RequiresAPIKeyFor(p Provider) bool {
	if info, ok := ProviderInfoByID(p); ok {
		return info.RequiresKey
	}
	return true
}
