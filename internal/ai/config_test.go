package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigOverride_ValidateRejectsUnknownProvider(t *testing.T) {
	bad := "not-a-provider"
	o := ConfigOverride{Provider: &bad}
	err := o.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestConfigOverride_ValidateAcceptsKnownProviders(t *testing.T) {
	for _, p := range []string{"openai", "anthropic", "minimax", "ollama", "OPENAI", " Anthropic "} {
		pp := p
		o := ConfigOverride{Provider: &pp}
		assert.NoErrorf(t, o.Validate(), "expected %q to validate", p)
	}
}

func TestConfigOverride_ValidateRejectsOutOfRangeMaxTokens(t *testing.T) {
	bad := 0
	o := ConfigOverride{MaxTokens: &bad}
	require.Error(t, o.Validate())

	bad = 500000
	o = ConfigOverride{MaxTokens: &bad}
	require.Error(t, o.Validate())
}

func TestConfigOverride_ValidateRejectsOutOfRangeTemperature(t *testing.T) {
	bad := -0.1
	o := ConfigOverride{Temperature: &bad}
	require.Error(t, o.Validate())

	bad = 2.5
	o = ConfigOverride{Temperature: &bad}
	require.Error(t, o.Validate())
}

func TestConfigOverride_ValidateRejectsOutOfRangeTimeout(t *testing.T) {
	bad := 50
	o := ConfigOverride{TimeoutMs: &bad}
	require.Error(t, o.Validate())

	bad = 700000
	o = ConfigOverride{TimeoutMs: &bad}
	require.Error(t, o.Validate())
}

func TestConfigOverride_IsEmpty(t *testing.T) {
	assert.True(t, ConfigOverride{}.IsEmpty())
	assert.False(t, ConfigOverride{Provider: ptrString("openai")}.IsEmpty())
}

func TestMergeConfig_OverrideWinsOverEnv(t *testing.T) {
	env := Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4",
		MaxTokens: 1024,
	}
	override := ConfigOverride{
		Model:     ptrString("claude-sonnet-4-5"),
		MaxTokens: ptrInt(8192),
	}
	got := MergeConfig(env, override)
	assert.Equal(t, ProviderOpenAI, got.Provider, "provider not in override keeps env value")
	assert.Equal(t, "claude-sonnet-4-5", got.Model)
	assert.Equal(t, 8192, got.MaxTokens)
}

func TestMergeConfig_NormalisesProviderCase(t *testing.T) {
	upper := "Anthropic"
	env := Config{Provider: ProviderOpenAI, Model: "x"}
	got := MergeConfig(env, ConfigOverride{Provider: &upper})
	assert.Equal(t, ProviderAnthropic, got.Provider)
}

func TestMergeConfig_AppliesDefaultsForZeroFields(t *testing.T) {
	got := MergeConfig(Config{}, ConfigOverride{})
	assert.Equal(t, ProviderOpenAI, got.Provider)
	assert.Equal(t, "gpt-4", got.Model)
	assert.Equal(t, 4096, got.MaxTokens)
	assert.InDelta(t, 0.7, got.Temperature, 1e-9)
	assert.Equal(t, 60_000_000_000, int(got.Timeout))
}

func TestLoadConfigFromEnv_DefaultsWhenUnset(t *testing.T) {
	// Clear any pre-existing AI env so the test is hermetic.
	for _, n := range []string{"M9M_AI_PROVIDER", "M9M_AI_API_KEY", "M9M_AI_BASE_URL", "M9M_AI_MODEL", "M9M_AI_MAX_TOKENS", "M9M_AI_TEMPERATURE", "M9M_AI_TIMEOUT_MS", "M9M_AI_ENABLED"} {
		t.Setenv(n, "")
	}
	cfg := LoadConfigFromEnv()
	assert.Equal(t, ProviderOpenAI, cfg.Provider)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "gpt-4", cfg.Model)
}

func TestLoadConfigFromEnv_HonoursLegacyCopilotEnv(t *testing.T) {
	t.Setenv("M9M_AI_PROVIDER", "")
	t.Setenv("M9M_COPILOT_PROVIDER", "anthropic")
	t.Setenv("M9M_COPILOT_MODEL", "claude-sonnet-4-5")

	cfg := LoadConfigFromEnv()
	assert.Equal(t, ProviderAnthropic, cfg.Provider)
	assert.Equal(t, "claude-sonnet-4-5", cfg.Model)
}

func TestLoadConfigFromEnv_PrefersNewAIEnvOverLegacy(t *testing.T) {
	t.Setenv("M9M_AI_PROVIDER", "openai")
	t.Setenv("M9M_COPILOT_PROVIDER", "anthropic")

	cfg := LoadConfigFromEnv()
	assert.Equal(t, ProviderOpenAI, cfg.Provider, "M9M_AI_* should win over M9M_COPILOT_*")
}

func TestLoadConfigFromEnv_FallsBackOnUnknownProvider(t *testing.T) {
	t.Setenv("M9M_AI_PROVIDER", "totally-fake-provider")
	cfg := LoadConfigFromEnv()
	assert.Equal(t, ProviderOpenAI, cfg.Provider, "unknown provider falls back to OpenAI")
}

func TestProviderInfoByID(t *testing.T) {
	for _, p := range Providers {
		got, ok := ProviderInfoByID(p.ID)
		require.Truef(t, ok, "expected %q to be in catalog", p.ID)
		assert.Equal(t, p.ID, got.ID)
		assert.NotEmpty(t, got.Models, "provider %q has no model catalog", p.ID)
	}
}

func TestDefaultModelFor(t *testing.T) {
	assert.Equal(t, "gpt-4o", DefaultModelFor(ProviderOpenAI))
	assert.Equal(t, "claude-sonnet-4-5", DefaultModelFor(ProviderAnthropic))
	assert.Equal(t, "MiniMax-M3", DefaultModelFor(ProviderMiniMax))
	assert.Equal(t, "llama3.2", DefaultModelFor(ProviderOllama))
}

func TestRequiresAPIKeyFor(t *testing.T) {
	assert.True(t, RequiresAPIKeyFor(ProviderOpenAI))
	assert.True(t, RequiresAPIKeyFor(ProviderAnthropic))
	assert.True(t, RequiresAPIKeyFor(ProviderMiniMax))
	assert.False(t, RequiresAPIKeyFor(ProviderOllama))
}

func TestDefaultBaseURLFor(t *testing.T) {
	assert.True(t, strings.HasPrefix(DefaultBaseURLFor(ProviderOpenAI), "https://"))
	assert.True(t, strings.HasPrefix(DefaultBaseURLFor(ProviderAnthropic), "https://"))
	assert.Equal(t, "https://api.minimax.chat/v1", DefaultBaseURLFor(ProviderMiniMax))
	assert.Equal(t, "http://localhost:11434", DefaultBaseURLFor(ProviderOllama))
}

func ptrInt(n int) *int { return &n }

// make sure os is not reported unused on systems that strip test files
var _ = os.Getenv
