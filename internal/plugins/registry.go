package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/neul-labs/m9m/internal/engine"
)

// Registry holds the loaded plugins and threads them through the
// engine. The map is keyed by node type (e.g.
// `n8n-nodes-base.my-node`) so registration with the engine is a
// straight assignment.
//
// All methods are safe for concurrent use; the only mutable state
// is the plugin map and the engine pointer.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
	dirs    []string
}

// NewRegistry returns an empty registry. Call `LoadDir` once
// (typically during `m9m serve` startup) and `RegisterAll` to
// install the plugins with the engine.
func NewRegistry() *Registry {
	return &Registry{plugins: map[string]*Plugin{}}
}

// LoadDir walks `dir` for `*.js` files and loads each one. Plugin
// load errors are collected and returned as a single multi-error so
// the operator can see every problem at once; a single broken
// plugin does not abort the entire startup.
func (r *Registry) LoadDir(dir string) error {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("plugin dir %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read plugin dir %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var errs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		p, err := Load(full)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		r.mu.Lock()
		// Last-write-wins: if two plugins resolve to the same
		// nodeType, the later file in alphabetical order replaces
		// the earlier one. This keeps `m9m node list` deterministic
		// for `m9m plugin install` workflows that drop multiple
		// files into the directory.
		if existing, ok := r.plugins[p.NodeType]; ok {
			errs = append(errs, fmt.Sprintf("plugin %s: node type %q already registered from %s (replaced by %s)",
				e.Name(), p.NodeType, existing.FilePath, full))
		}
		r.plugins[p.NodeType] = p
		r.mu.Unlock()
	}
	r.dirs = append(r.dirs, dir)

	if len(errs) > 0 {
		return fmt.Errorf("plugin load errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// RegisterAll wires every loaded plugin with the engine as a
// `NodeExecutor` wrapper. The wrapper serialises `execute` calls
// because Goja is not re-entrant — concurrent invocations of the
// same plugin would otherwise corrupt the VM.
func (r *Registry) RegisterAll(eng engine.WorkflowEngine) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for nodeType, p := range r.plugins {
		eng.RegisterNodeExecutor(nodeType, &Wrapper{plugin: p})
	}
	return nil
}

// List returns the loaded plugins, sorted by node type for
// deterministic `m9m node list` output.
func (r *Registry) List() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeType < out[j].NodeType
	})
	return out
}

// Count returns the number of loaded plugins.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// Reload re-reads a single plugin file. Used by the file watcher
// to hot-reload on change. Returns the new plugin (or an error) so
// the caller can decide whether to swap it in.
func (r *Registry) Reload(nodeType string) (*Plugin, error) {
	r.mu.Lock()
	existing, ok := r.plugins[nodeType]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("plugin %q is not loaded", nodeType)
	}
	p, err := Load(existing.FilePath)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.plugins[nodeType] = p
	r.mu.Unlock()
	return p, nil
}
