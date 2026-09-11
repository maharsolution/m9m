package ai

import "sync"

// Runtime holds the live AI service and allows callers to swap the
// underlying *AI atomically. In-flight requests grab a reference via
// Current() under RLock and continue using that reference even after
// Apply() installs a new one — so a config change never interrupts a
// running chat.
type Runtime struct {
	mu  sync.RWMutex
	ai  *AI
	cfg Config
}

// NewRuntime wraps a freshly-built AI under a Runtime. The returned
// Runtime owns the AI; callers should only access it through Current().
func NewRuntime(cfg Config) *Runtime {
	return &Runtime{
		ai:  NewAI(&cfg),
		cfg: cfg,
	}
}

// Current returns the live AI service. The returned pointer is stable
// for the lifetime of the current config; the next Apply() will hand
// callers a different pointer on subsequent reads. Treat the result
// as read-only.
func (r *Runtime) Current() *AI {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ai
}

// Config returns the config the live AI was built from. Useful for
// echoing back to the UI in /ai/test responses and for debug logs.
func (r *Runtime) Config() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

// Apply atomically replaces the live AI with a new one built from cfg.
// Callers are responsible for persisting cfg (typically the API
// handler via ConfigStore.Save) before calling Apply.
func (r *Runtime) Apply(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ai = NewAI(&cfg)
	r.cfg = cfg
}
