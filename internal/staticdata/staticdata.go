// Package staticdata implements workflow-scoped key-value storage
// for n8n's "Static Data" feature (BUG-ARCH-2026-10-12-005).
//
// In n8n, a workflow can persist arbitrary JSON across runs
// through `Workflow.getStaticData(type)` / `setStaticData(...)`.
// The data is scoped to the workflow id (global static data)
// OR to a (workflow id, node name) pair (node-level static
// data). It is not visible across workflows.
//
// m9m stores the data on disk under
// `{DataDir}/staticdata/{workflowID}/{nodeName|__global__}.json`.
// The store is goroutine-safe; all writes go through an
// in-process mutex so the file is never read mid-write.
//
// The on-disk format is one JSON object per file. Reads
// return a `map[string]interface{}`; writes replace the
// whole file. This matches n8n's "the whole dict" semantics
// — the workflow code calls `set` on individual keys, but
// the underlying API takes the entire map.
package staticdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store is the workflow-scoped static-data store. It is
// keyed by workflow ID and (optionally) node name.
type Store struct {
	// DataDir is the root under which the per-workflow
	// subdirectories are created. Required.
	DataDir string

	mu sync.Mutex
}

// New returns a Store rooted at `dataDir`. The directory
// is created if it does not exist.
func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("staticdata: data dir is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("staticdata: mkdir %s: %w", dataDir, err)
	}
	return &Store{DataDir: dataDir}, nil
}

// Key returns the on-disk path for a (workflowID, nodeName)
// pair. `nodeName == ""` is the global static data for the
// workflow.
func (s *Store) key(workflowID, nodeName string) string {
	if workflowID == "" {
		// Defensive: m9m never calls us with an empty
		// workflowID (the engine supplies it). The error
		// path is a clear 500 instead of a silent file at
		// the root.
		return filepath.Join(s.DataDir, "_invalid", "_workflow.json")
	}
	node := nodeName
	if node == "" {
		node = "__global__"
	}
	return filepath.Join(s.DataDir, workflowID, node+".json")
}

// Get returns the static-data map for the given scope. If
// the file does not exist, returns an empty (non-nil) map —
// n8n's "first call returns {}" semantics.
func (s *Store) Get(workflowID, nodeName string) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readFile(s.key(workflowID, nodeName))
}

// Set replaces the static-data map for the given scope.
// The whole map is persisted (atomic via temp-file +
// rename) so a partial write can never leave the on-disk
// JSON malformed.
func (s *Store) Set(workflowID, nodeName string, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.key(workflowID, nodeName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("staticdata: mkdir %s: %w", filepath.Dir(path), err)
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("staticdata: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("staticdata: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("staticdata: rename %s: %w", path, err)
	}
	return nil
}

// Update applies `mutator` to the current static data and
// persists the result. The mutator receives the current
// map (or an empty one) and may mutate it in place. This
// is the read-modify-write helper the engine uses when a
// node calls `set` on a single key.
func (s *Store) Update(workflowID, nodeName string, mutator func(map[string]interface{})) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.key(workflowID, nodeName)
	current, err := s.readFile(path)
	if err != nil {
		return err
	}
	mutator(current)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("staticdata: mkdir %s: %w", filepath.Dir(path), err)
	}
	body, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("staticdata: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("staticdata: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("staticdata: rename %s: %w", path, err)
	}
	return nil
}

// Delete removes the static data for the given scope. No
// error if the file does not exist.
func (s *Store) Delete(workflowID, nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.key(workflowID, nodeName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("staticdata: remove %s: %w", path, err)
	}
	return nil
}

// readFile is the helper that loads a JSON map from disk.
// `os.IsNotExist` returns an empty (non-nil) map with no
// error — matches n8n.
func (s *Store) readFile(path string) (map[string]interface{}, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("staticdata: read %s: %w", path, err)
	}
	if len(body) == 0 {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("staticdata: parse %s: %w", path, err)
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}
