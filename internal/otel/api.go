package otel

import (
	"context"
	"encoding/json"
	"net/http"
)

// APIHandler exposes the OTel config to the UI over HTTP. It is owned by
// the api package (registered as /api/v1/otel/{get,put}) but the handler
// logic lives here so the otel package stays the single source of truth
// for config semantics.
//
// The handler is a thin wrapper: it decodes a ConfigOverride, validates
// it, persists it via the ConfigStore, and asks the Manager to reload.
// The Manager is optional — when the binary has no Manager (e.g. a CLI
// tool that just persists overrides), the handler still works for GET.
type APIHandler struct {
	store   *ConfigStore
	manager *Manager
}

// NewAPIHandler wires a handler. manager may be nil (the handler will
// persist settings but not actually rebuild the tracer provider).
func NewAPIHandler(store *ConfigStore, manager *Manager) *APIHandler {
	return &APIHandler{store: store, manager: manager}
}

// Get returns the current view (env + override + effective) to the UI.
func (h *APIHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "otel storage not initialised")
		return
	}

	env := LoadConfigFromEnv()
	override, err := h.store.LoadOrZero()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "otel: load override: "+err.Error())
		return
	}
	view := ConfigView{
		Env:       env,
		Override:  override,
		Effective: MergeConfig(env, override),
	}
	writeJSON(w, http.StatusOK, view)
}

// Put accepts a ConfigOverride body and persists + applies it.
func (h *APIHandler) Put(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "otel storage not initialised")
		return
	}

	var override ConfigOverride
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&override); err != nil {
		writeError(w, http.StatusBadRequest, "otel: invalid body: "+err.Error())
		return
	}
	if err := override.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.Save(override); err != nil {
		writeError(w, http.StatusInternalServerError, "otel: save override: "+err.Error())
		return
	}

	// Reload the tracer provider so the change takes effect immediately.
	// The Manager is allowed to be nil (CLI-only contexts); we skip the
	// reload in that case but still report success — the persisted
	// override will be picked up the next time the binary restarts.
	if h.manager != nil {
		cfg := EffectiveConfig(override)
		if err := h.manager.Reload(r.Context(), cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "otel: reload: "+err.Error())
			return
		}
	}

	env := LoadConfigFromEnv()
	writeJSON(w, http.StatusOK, ConfigView{
		Env:       env,
		Override:  override,
		Effective: MergeConfig(env, override),
	})
}

// Delete clears the persisted override, restoring env-driven defaults.
func (h *APIHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "otel storage not initialised")
		return
	}
	if err := h.store.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, "otel: clear override: "+err.Error())
		return
	}
	if h.manager != nil {
		cfg := EffectiveConfig(ConfigOverride{})
		if err := h.manager.Reload(r.Context(), cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "otel: reload: "+err.Error())
			return
		}
	}
	env := LoadConfigFromEnv()
	writeJSON(w, http.StatusOK, ConfigView{
		Env:       env,
		Override:  ConfigOverride{},
		Effective: env,
	})
}

// TestTrace sends a no-op span to the collector so the UI can confirm
// connectivity. The collector returns success via the OTLP ack; we don't
// need to wait — the user can refresh the trace view to confirm. The
// span is recorded but does not count toward any sampling budget because
// it runs only when the user clicks the button.
func (h *APIHandler) TestTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "otel manager not initialised")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	_, span := h.manager.Tracer().Start(ctx, "m9m.otel.test")
	span.SetAttributes()
	span.End()
	w.WriteHeader(http.StatusNoContent)
}

// writeJSON / writeError are tiny local helpers so this file doesn't
// need to import the api package (which would create a cycle — the api
// package already imports otel for the handler to mount).
func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
