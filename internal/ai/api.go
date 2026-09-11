package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// APIHandler exposes the AI config to the UI over HTTP. It is owned by
// the api package (registered as /api/v1/ai/{get,put,delete}) but the
// handler logic lives here so the ai package stays the single source of
// truth for config semantics.
//
// The handler is a thin wrapper: it decodes a ConfigOverride, validates
// it, persists it via the ConfigStore, and asks the Runtime to reload.
// The Runtime is optional — when the binary has no Runtime (e.g. a CLI
// tool that just persists overrides), the handler still works for GET.
type APIHandler struct {
	store   *ConfigStore
	runtime *Runtime
}

// NewAPIHandler wires a handler. runtime may be nil (the handler will
// persist settings but not actually rebuild the AI service).
func NewAPIHandler(store *ConfigStore, runtime *Runtime) *APIHandler {
	return &APIHandler{store: store, runtime: runtime}
}

// Get returns the current view (env + override + effective) to the UI.
func (h *APIHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "ai storage not initialised")
		return
	}

	env := LoadConfigFromEnv()
	override, err := h.store.LoadOrZero()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "ai: load override: "+err.Error())
		return
	}
	view := ConfigView{
		Env:       env,
		Override:  override,
		Effective: MergeConfig(env, override),
		Providers: Providers,
	}
	writeAPIJSON(w, http.StatusOK, view)
}

// Put accepts a ConfigOverride body and persists + applies it.
func (h *APIHandler) Put(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "ai storage not initialised")
		return
	}

	var override ConfigOverride
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&override); err != nil {
		writeAPIError(w, http.StatusBadRequest, "ai: invalid body: "+err.Error())
		return
	}
	if err := override.Validate(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.Save(override); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "ai: save override: "+err.Error())
		return
	}

	// Reload the live AI service so the change takes effect immediately.
	// The Runtime is allowed to be nil (CLI-only contexts); we skip the
	// reload in that case but still report success — the persisted
	// override will be picked up the next time the binary restarts.
	if h.runtime != nil {
		cfg := EffectiveConfig(override)
		h.runtime.Apply(cfg)
	}

	env := LoadConfigFromEnv()
	writeAPIJSON(w, http.StatusOK, ConfigView{
		Env:       env,
		Override:  override,
		Effective: MergeConfig(env, override),
		Providers: Providers,
	})
}

// Delete clears the persisted override, restoring env-driven defaults.
func (h *APIHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "ai storage not initialised")
		return
	}
	if err := h.store.Clear(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "ai: clear override: "+err.Error())
		return
	}
	if h.runtime != nil {
		cfg := EffectiveConfig(ConfigOverride{})
		h.runtime.Apply(cfg)
	}
	env := LoadConfigFromEnv()
	writeAPIJSON(w, http.StatusOK, ConfigView{
		Env:       env,
		Override:  ConfigOverride{},
		Effective: env,
		Providers: Providers,
	})
}

// TestChat sends a one-shot chat prompt to the configured AI service so
// the UI can confirm the provider, key, and base URL all line up. The
// handler returns the LLM's reply verbatim — the UI shows it in a
// dialog so the operator knows whether their config works.
func (h *APIHandler) TestChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.runtime == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "ai runtime not initialised")
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "ai: invalid body: "+err.Error())
		return
	}
	if body.Prompt == "" {
		body.Prompt = "Reply with exactly: m9m AI assistant ready."
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.runtime.Config().Timeout)
	defer cancel()

	start := time.Now()
	reply, err := h.runtime.Current().Chat(ctx, &ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: body.Prompt}},
	})
	elapsed := time.Since(start)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "ai: test chat failed: "+err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]string{
		"message": reply.Message,
		"elapsed": elapsed.String(),
	})
}

// writeAPIJSON / writeAPIError are tiny local helpers so this file
// doesn't need to import the api package (which would create a cycle —
// the api package already imports ai for the handler to mount).
func writeAPIJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeAPIJSON(w, status, map[string]string{"error": msg})
}
