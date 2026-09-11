package ai

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/storage"
)

func TestConfigStore_LoadReturnsErrNotFoundWhenEmpty(t *testing.T) {
	s := NewConfigStore(storage.NewMemoryStorage())
	_, err := s.Load()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound), "expected ErrNotFound, got %v", err)
}

func TestConfigStore_LoadOrZeroReturnsZeroOnEmpty(t *testing.T) {
	s := NewConfigStore(storage.NewMemoryStorage())
	o, err := s.LoadOrZero()
	require.NoError(t, err)
	assert.True(t, o.IsEmpty())
}

func TestConfigStore_SaveThenLoadRoundtrips(t *testing.T) {
	s := NewConfigStore(storage.NewMemoryStorage())

	want := ConfigOverride{
		Provider: ptrString("openai"),
		Model:    ptrString("gpt-4o"),
	}
	require.NoError(t, s.Save(want))

	got, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, "openai", derefString(got.Provider))
	assert.Equal(t, "gpt-4o", derefString(got.Model))
}

func TestConfigStore_SaveEmptyDeletesExistingOverride(t *testing.T) {
	s := NewConfigStore(storage.NewMemoryStorage())

	require.NoError(t, s.Save(ConfigOverride{Provider: ptrString("anthropic")}))
	_, err := s.Load()
	require.NoError(t, err)

	require.NoError(t, s.Save(ConfigOverride{})) // empty
	_, err = s.Load()
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestConfigStore_ClearRemovesOverride(t *testing.T) {
	s := NewConfigStore(storage.NewMemoryStorage())

	require.NoError(t, s.Save(ConfigOverride{Provider: ptrString("openai")}))
	require.NoError(t, s.Clear())

	_, err := s.Load()
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestConfigStore_LoadReportsInvalidJSON(t *testing.T) {
	raw := storage.NewMemoryStorage()
	require.NoError(t, raw.SaveRaw(StorageKey, []byte("{not json")))
	s := NewConfigStore(raw)
	_, err := s.Load()
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
}

// helpers for tests that need to construct *string values
func ptrString(s string) *string { return &s }
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
