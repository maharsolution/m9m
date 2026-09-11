package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProviderCatalog_HasAllExpectedProviders guards against accidental
// removals — if you delete an entry from Providers, this test breaks
// before the UI starts failing at runtime.
func TestProviderCatalog_HasAllExpectedProviders(t *testing.T) {
	want := []Provider{ProviderOpenAI, ProviderAnthropic, ProviderMiniMax, ProviderOllama}
	have := make([]Provider, 0, len(Providers))
	for _, p := range Providers {
		have = append(have, p.ID)
	}
	for _, w := range want {
		assert.Containsf(t, have, w, "provider catalog is missing %q", w)
	}
}

// TestProviderCatalog_EveryEntryIsComplete ensures no entry has empty
// fields that would break the UI dropdown rendering.
func TestProviderCatalog_EveryEntryIsComplete(t *testing.T) {
	for _, p := range Providers {
		assert.NotEmptyf(t, p.ID, "provider has empty id")
		assert.NotEmptyf(t, p.Label, "provider %q has empty label", p.ID)
		assert.NotEmptyf(t, p.Description, "provider %q has empty description", p.ID)
		assert.NotEmptyf(t, p.DefaultBaseURL, "provider %q has empty base URL", p.ID)
		assert.NotEmptyf(t, p.DefaultModel, "provider %q has empty default model", p.ID)
		require.NotEmptyf(t, p.Models, "provider %q has empty model list", p.ID)
		assert.Containsf(t, p.Models, p.DefaultModel, "provider %q default model not in model list", p.ID)
	}
}

// TestProviderCatalog_DefaultModelsAreDistinct catches copy/paste bugs
// in the catalog (e.g. accidentally reusing the OpenAI model for every
// provider).
func TestProviderCatalog_DefaultModelsAreDistinct(t *testing.T) {
	seen := map[string]Provider{}
	for _, p := range Providers {
		if other, dup := seen[p.DefaultModel]; dup {
			t.Fatalf("default model %q is shared by %q and %q", p.DefaultModel, other, p.ID)
		}
		seen[p.DefaultModel] = p.ID
	}
}
