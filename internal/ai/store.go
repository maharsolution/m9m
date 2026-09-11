package ai

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/neul-labs/m9m/internal/storage"
)

// StorageKey is the canonical raw-storage key under which the AI
// config override is persisted. The dot prefix mimics the n8n "config"
// namespace (n8n uses `config.<key>` for similar override blobs).
const StorageKey = "ai.config"

// ErrNotFound is returned when no override has been saved yet. Callers
// should treat it as "no override, fall back to env".
var ErrNotFound = errors.New("ai: no override stored")

// ConfigStore persists the AI config override in the underlying
// WorkflowStorage's raw KV. It is safe for concurrent use.
//
// We deliberately wrap storage.WorkflowStorage rather than introduce a
// dedicated interface: the existing SaveRaw/GetRaw/DeleteRaw primitives
// already provide the semantics we need, and the alternative (a per-
// backend typed Settings table) would mean touching four storage
// implementations before we get a single working endpoint.
type ConfigStore struct {
	store storage.WorkflowStorage
	mu    sync.RWMutex
}

// NewConfigStore constructs a ConfigStore. The supplied store must be
// non-nil; the caller is expected to ensure the storage layer was
// initialised successfully.
func NewConfigStore(store storage.WorkflowStorage) *ConfigStore {
	return &ConfigStore{store: store}
}

// Load reads the persisted override. Returns (zero, ErrNotFound) when
// nothing has been stored yet; (zero, err) on transport errors.
//
// Different storage backends disagree about how to signal "missing":
// MemoryStorage returns a typed error string, Postgres / SQLite return
// sql.ErrNoRows-shaped errors. We treat any "no such key" error as
// ErrNotFound so callers don't need to special-case each backend.
func (s *ConfigStore) Load() (ConfigOverride, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.store == nil {
		return ConfigOverride{}, ErrNotFound
	}
	b, err := s.store.GetRaw(StorageKey)
	if err != nil {
		if isMissingKeyError(err) {
			return ConfigOverride{}, ErrNotFound
		}
		return ConfigOverride{}, fmt.Errorf("ai: read config override: %w", err)
	}
	if len(b) == 0 {
		return ConfigOverride{}, ErrNotFound
	}
	o, err := UnmarshalOverride(b)
	if err != nil {
		return ConfigOverride{}, fmt.Errorf("ai: parse config override: %w", err)
	}
	return o, nil
}

// isMissingKeyError returns true for the handful of error shapes the
// in-tree storage backends produce when GetRaw hits a missing key. We
// match by error message rather than type because each backend
// wraps its own error differently.
func isMissingKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return true
	case strings.Contains(msg, "no rows"):
		return true
	case strings.Contains(msg, "key not found"):
		return true
	}
	return false
}

// Save persists the override. An empty override (no fields set) deletes
// the stored record so subsequent loads fall back to env defaults.
func (s *ConfigStore) Save(o ConfigOverride) error {
	if err := o.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return errors.New("ai: storage not initialised")
	}
	if o.IsEmpty() {
		return s.store.DeleteRaw(StorageKey)
	}
	b, err := MarshalOverride(o)
	if err != nil {
		return fmt.Errorf("ai: marshal config override: %w", err)
	}
	if err := s.store.SaveRaw(StorageKey, b); err != nil {
		return fmt.Errorf("ai: write config override: %w", err)
	}
	return nil
}

// Clear removes the persisted override, restoring env-driven defaults.
func (s *ConfigStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return nil
	}
	return s.store.DeleteRaw(StorageKey)
}

// LoadOrZero is the convenience wrapper used by the runtime bootstrap:
// returns the override on hit, a zero override on ErrNotFound, and
// surfaces any other error.
func (s *ConfigStore) LoadOrZero() (ConfigOverride, error) {
	o, err := s.Load()
	if errors.Is(err, ErrNotFound) {
		return ConfigOverride{}, nil
	}
	return o, err
}
