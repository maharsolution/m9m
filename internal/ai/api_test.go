package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/storage"
)

func TestAPIHandler_GetReturnsConfigView(t *testing.T) {
	h := NewAPIHandler(NewConfigStore(storage.NewMemoryStorage()), NewRuntime(*DefaultConfig()))

	req := httptest.NewRequest(http.MethodGet, "/ai", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var view ConfigView
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &view))
	assert.NotEmpty(t, view.Providers, "provider catalog must be included")
	assert.NotNil(t, view.Env, view.Override, view.Effective)
	assert.Equal(t, ProviderOpenAI, view.Env.Provider)
}

func TestAPIHandler_GetRejectsNonGet(t *testing.T) {
	h := NewAPIHandler(NewConfigStore(storage.NewMemoryStorage()), NewRuntime(*DefaultConfig()))

	req := httptest.NewRequest(http.MethodPost, "/ai", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestAPIHandler_PutPersistsOverrideAndReloadsRuntime(t *testing.T) {
	store := NewConfigStore(storage.NewMemoryStorage())
	runtime := NewRuntime(*DefaultConfig())
	h := NewAPIHandler(store, runtime)

	override := ConfigOverride{
		Provider: ptrString("anthropic"),
		Model:    ptrString("claude-sonnet-4-5"),
		Enabled:  ptrBool(true),
	}
	body, err := json.Marshal(override)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/ai", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Put(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Round-trip via the store
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", derefString(loaded.Provider))
	assert.Equal(t, "claude-sonnet-4-5", derefString(loaded.Model))

	// Runtime picked up the change
	cfg := runtime.Config()
	assert.Equal(t, ProviderAnthropic, cfg.Provider)
	assert.Equal(t, "claude-sonnet-4-5", cfg.Model)
}

func TestAPIHandler_PutRejectsInvalidBody(t *testing.T) {
	h := NewAPIHandler(NewConfigStore(storage.NewMemoryStorage()), NewRuntime(*DefaultConfig()))

	req := httptest.NewRequest(http.MethodPut, "/ai", bytes.NewReader([]byte("{not-json")))
	rr := httptest.NewRecorder()
	h.Put(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAPIHandler_PutRejectsUnknownProvider(t *testing.T) {
	h := NewAPIHandler(NewConfigStore(storage.NewMemoryStorage()), NewRuntime(*DefaultConfig()))

	bad := "totally-fake"
	body, _ := json.Marshal(ConfigOverride{Provider: &bad})
	req := httptest.NewRequest(http.MethodPut, "/ai", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Put(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "provider")
}

func TestAPIHandler_DeleteClearsOverrideAndReloadsRuntime(t *testing.T) {
	store := NewConfigStore(storage.NewMemoryStorage())
	runtime := NewRuntime(*DefaultConfig())

	// Seed an override so the delete is meaningful
	seed := ConfigOverride{Provider: ptrString("anthropic")}
	require.NoError(t, store.Save(seed))
	runtime.Apply(MergeConfig(LoadConfigFromEnv(), seed))

	h := NewAPIHandler(store, runtime)

	req := httptest.NewRequest(http.MethodDelete, "/ai", nil)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Store should now report ErrNotFound
	_, err := store.Load()
	assert.Error(t, err)

	// Runtime should have reverted to defaults
	cfg := runtime.Config()
	assert.Equal(t, ProviderOpenAI, cfg.Provider, "delete restores env defaults")
}

func TestAPIHandler_GetReturnsServiceUnavailableWhenStoreNil(t *testing.T) {
	h := NewAPIHandler(nil, NewRuntime(*DefaultConfig()))

	req := httptest.NewRequest(http.MethodGet, "/ai", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestAPIHandler_TestChatReturnsMessage(t *testing.T) {
	// Spin up a fake OpenAI-compatible server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from fake"}}]}`))
	}))
	defer srv.Close()

	store := NewConfigStore(storage.NewMemoryStorage())
	cfg := *DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.APIKey = "test-key"
	runtime := NewRuntime(cfg)
	h := NewAPIHandler(store, runtime)

	body, _ := json.Marshal(map[string]string{"prompt": "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/ai/test", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.TestChat(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var out map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	assert.Equal(t, "hello from fake", out["message"])
	assert.NotEmpty(t, out["elapsed"])
}

func TestAPIHandler_TestChatReturnsServiceUnavailableWhenRuntimeNil(t *testing.T) {
	h := NewAPIHandler(NewConfigStore(storage.NewMemoryStorage()), nil)

	req := httptest.NewRequest(http.MethodPost, "/ai/test", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	h.TestChat(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func ptrBool(b bool) *bool { return &b }
