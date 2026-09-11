package ai

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// ConfigOverride captures a single field override loaded from persistent
// storage. The DB layer stores each user-edited field as a key/value
// pair; a nil (zero) pointer means "use the env default".
//
// Field names mirror the public Config field names in lowerCamelCase
// (Provider, BaseURL, MaxTokens, ...) so the JSON the UI sends round
// trips 1:1.
type ConfigOverride struct {
	// Enabled tri-state: nil means "follow env", true/false override
	// the env default.
	Enabled *bool `json:"enabled,omitempty"`

	Provider    *string  `json:"provider,omitempty"`    // openai|anthropic|minimax|ollama
	APIKey      *string  `json:"apiKey,omitempty"`       // never logged
	BaseURL     *string  `json:"baseUrl,omitempty"`
	Model       *string  `json:"model,omitempty"`
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TimeoutMs   *int     `json:"timeoutMs,omitempty"`

	// Source describes where the override came from — "ui" or "api".
	// Filled in by the API layer; not consumed by the loader.
	Source string `json:"source,omitempty"`
}

// Validate returns an error when the override holds values that don't
// pass basic sanity checks. We don't validate URLs here so the UI can
// stage half-finished changes.
func (o ConfigOverride) Validate() error {
	if o.Provider != nil {
		p := strings.ToLower(strings.TrimSpace(*o.Provider))
		switch Provider(p) {
		case ProviderOpenAI, ProviderAnthropic, ProviderMiniMax, ProviderOllama:
			// OK
		default:
			return errors.New("ai: provider must be one of openai|anthropic|minimax|ollama")
		}
	}
	if o.MaxTokens != nil {
		if *o.MaxTokens < 1 || *o.MaxTokens > 200000 {
			return errors.New("ai: maxTokens must be between 1 and 200000")
		}
	}
	if o.Temperature != nil {
		if *o.Temperature < 0 || *o.Temperature > 2 {
			return errors.New("ai: temperature must be between 0 and 2")
		}
	}
	if o.TimeoutMs != nil {
		if *o.TimeoutMs < 100 || *o.TimeoutMs > 600000 {
			return errors.New("ai: timeoutMs must be between 100 and 600000 (ms)")
		}
	}
	return nil
}

// IsEmpty reports whether the override has no fields set.
func (o ConfigOverride) IsEmpty() bool {
	return o.Enabled == nil && o.Provider == nil && o.APIKey == nil &&
		o.BaseURL == nil && o.Model == nil && o.MaxTokens == nil &&
		o.Temperature == nil && o.TimeoutMs == nil
}

// MarshalOverride is the canonical JSON encoding used by the DB layer
// when storing the override blob in a single TEXT column. Keeping the
// format private means we can change the storage layout later (e.g.
// migrate to one-row-per-field) without breaking on-disk payloads.
func MarshalOverride(o ConfigOverride) ([]byte, error) {
	if o.IsEmpty() {
		return []byte("{}"), nil
	}
	return json.Marshal(o)
}

// UnmarshalOverride parses the JSON form back into an override. A nil
// or empty byte slice decodes to a zero ConfigOverride — interpreted as
// "no override, fall back to env".
func UnmarshalOverride(b []byte) (ConfigOverride, error) {
	var o ConfigOverride
	if len(b) == 0 {
		return o, nil
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return o, err
	}
	return o, nil
}

// ApplyDefaults fills in defaults for fields that are still zero. The
// defaults match what LoadConfigFromEnv produces with no env vars set.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = ProviderOpenAI
	}
	if c.Model == "" {
		c.Model = "gpt-4"
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 4096
	}
	if c.Temperature == 0 {
		c.Temperature = 0.7
	}
	if c.Timeout == 0 {
		c.Timeout = 60 * time.Second
	}
}

// MergeConfig returns env augmented with the override. The merge order is
// env -> override, so a non-nil override field always wins. A nil pointer
// is the documented way to say "use the env default", which matches the
// n8n behaviour where empty env vars preserve the prior value.
func MergeConfig(env Config, override ConfigOverride) Config {
	out := env
	if override.Enabled != nil {
		out.Enabled = *override.Enabled
	}
	if override.Provider != nil {
		out.Provider = Provider(strings.ToLower(strings.TrimSpace(*override.Provider)))
	}
	if override.APIKey != nil {
		out.APIKey = strings.TrimSpace(*override.APIKey)
	}
	if override.BaseURL != nil {
		out.BaseURL = strings.TrimSpace(*override.BaseURL)
	}
	if override.Model != nil {
		out.Model = strings.TrimSpace(*override.Model)
	}
	if override.MaxTokens != nil {
		out.MaxTokens = *override.MaxTokens
	}
	if override.Temperature != nil {
		out.Temperature = *override.Temperature
	}
	if override.TimeoutMs != nil {
		out.Timeout = time.Duration(*override.TimeoutMs) * time.Millisecond
	}
	out.ApplyDefaults()
	return out
}

// EffectiveConfig returns the merged config the runtime should use. It is
// the single helper callers should reach for; LoadConfigFromEnv stays
// for tests that don't want to fake a storage layer.
func EffectiveConfig(override ConfigOverride) Config {
	return MergeConfig(LoadConfigFromEnv(), override)
}

// ConfigView is what the GET /api/v1/ai endpoint returns to the UI.
// It contains three layers: the env-derived defaults (so the UI can
// show "what would happen with no override"), the user-edited override
// (so the UI can highlight which fields diverge from defaults), and the
// effective value (for read-only display / debugging).
//
// Providers is the full catalog the UI uses to render the provider
// dropdown + per-provider model dropdown. It is included on every GET
// so the UI never needs a second round-trip to render Settings → AI.
type ConfigView struct {
	Env       Config         `json:"env"`
	Override  ConfigOverride `json:"override"`
	Effective Config         `json:"effective"`
	Providers []ProviderInfo `json:"providers"`
}

// LoadConfigFromEnv parses the M9M_AI_* env vars into a Config, with
// fallback to the deprecated M9M_COPILOT_* names for back-compat. The
// function never returns an error: unparseable numbers fall back to
// defaults, unknown providers fall back to OpenAI. A Config with
// Enabled=false is returned when M9M_AI_ENABLED is not "true" so the
// rest of the codebase can treat AI as off without nil-checking
// Config.
func LoadConfigFromEnv() Config {
	provider := strings.ToLower(strings.TrimSpace(firstNonEmpty("M9M_AI_PROVIDER", "M9M_COPILOT_PROVIDER")))
	if provider == "" {
		provider = string(ProviderOpenAI)
	}
	cfg := Config{
		Enabled:    envBool("M9M_AI_ENABLED", false),
		Provider:   Provider(provider),
		APIKey:     strings.TrimSpace(firstNonEmpty("M9M_AI_API_KEY", "M9M_COPILOT_API_KEY")),
		BaseURL:    strings.TrimSpace(firstNonEmpty("M9M_AI_BASE_URL", "M9M_COPILOT_BASE_URL")),
		Model:      strings.TrimSpace(firstNonEmpty("M9M_AI_MODEL", "M9M_COPILOT_MODEL")),
		MaxTokens:  envInt("M9M_AI_MAX_TOKENS", 4096),
		Temperature: envFloat("M9M_AI_TEMPERATURE", 0.7),
		Timeout:    time.Duration(envInt("M9M_AI_TIMEOUT_MS", 60000)) * time.Millisecond,
	}
	// Validate the provider we got from env; unknown values fall back
	// to OpenAI rather than panicking later.
	switch cfg.Provider {
	case ProviderOpenAI, ProviderAnthropic, ProviderMiniMax, ProviderOllama:
		// OK
	default:
		cfg.Provider = ProviderOpenAI
	}
	cfg.ApplyDefaults()
	return cfg
}

// firstNonEmpty returns the first non-empty value from a list of env
// var names. Used to honour both the new M9M_AI_* names and the
// deprecated M9M_COPILOT_* names so existing `.env` files keep
// working.
func firstNonEmpty(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

func envBool(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(name string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
