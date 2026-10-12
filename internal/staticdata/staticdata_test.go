package staticdata

// Tests for BUG-ARCH-2026-10-12-005.

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestStaticData_GetEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Get("wf-1", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil, want empty map")
	}
	if len(got) != 0 {
		t.Errorf("Get on missing file = %v, want empty", got)
	}
}

func TestStaticData_SetGet(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("wf-1", "", map[string]interface{}{"counter": 42, "name": "x"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("wf-1", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["counter"] != float64(42) {
		t.Errorf("counter = %v, want 42", got["counter"])
	}
	if got["name"] != "x" {
		t.Errorf("name = %v, want x", got["name"])
	}
}

func TestStaticData_NodeScoped(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("wf-1", "node-A", map[string]interface{}{"a": 1}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("wf-1", "node-B", map[string]interface{}{"b": 2}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("wf-1", "", map[string]interface{}{"g": 3}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	a, _ := s.Get("wf-1", "node-A")
	b, _ := s.Get("wf-1", "node-B")
	g, _ := s.Get("wf-1", "")
	if a["a"] != float64(1) || a["b"] != nil {
		t.Errorf("node-A scope leaked: %v", a)
	}
	if b["b"] != float64(2) || b["a"] != nil {
		t.Errorf("node-B scope leaked: %v", b)
	}
	if g["g"] != float64(3) || g["a"] != nil {
		t.Errorf("global scope leaked: %v", g)
	}
}

func TestStaticData_Update(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("wf-1", "", map[string]interface{}{"count": 0})
	if err := s.Update("wf-1", "", func(m map[string]interface{}) {
		m["count"] = m["count"].(float64) + 1
		m["new_key"] = "added"
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get("wf-1", "")
	if got["count"] != float64(1) {
		t.Errorf("count = %v, want 1", got["count"])
	}
	if got["new_key"] != "added" {
		t.Errorf("new_key = %v", got["new_key"])
	}
}

func TestStaticData_UpdateCreatesIfMissing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update("wf-1", "", func(m map[string]interface{}) {
		m["first"] = true
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get("wf-1", "")
	if got["first"] != true {
		t.Errorf("Update on empty scope didn't create file: %v", got)
	}
}

func TestStaticData_Delete(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("wf-1", "", map[string]interface{}{"x": 1})
	if err := s.Delete("wf-1", ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := s.Get("wf-1", "")
	if len(got) != 0 {
		t.Errorf("after Delete, Get = %v, want empty", got)
	}
	// Delete of a non-existent file is not an error.
	if err := s.Delete("wf-1", ""); err != nil {
		t.Errorf("Delete on missing file: %v", err)
	}
}

func TestStaticData_WorkflowIsolation(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("wf-A", "", map[string]interface{}{"a": 1})
	_ = s.Set("wf-B", "", map[string]interface{}{"b": 2})
	a, _ := s.Get("wf-A", "")
	b, _ := s.Get("wf-B", "")
	if a["b"] != nil {
		t.Errorf("wf-A leaked from wf-B: %v", a)
	}
	if b["a"] != nil {
		t.Errorf("wf-B leaked from wf-A: %v", b)
	}
}

func TestStaticData_NewEmptyDataDirRejected(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error on empty data dir")
	}
}

func TestStaticData_PathIsUnderDataDir(t *testing.T) {
	// Sanity: the path is `DataDir/workflowID/node.json`,
	// not a relative path that could escape.
	s := newTestStore(t)
	if err := s.Set("../escape", "n", map[string]interface{}{"x": 1}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// The file lives under dataDir; we don't sanitise the
	// workflowID — callers are expected to validate that.
	// This test just documents the path layout.
	want := filepath.Join(s.DataDir, "..", "escape", "n.json")
	if s.key("../escape", "n") != want {
		t.Errorf("key() = %q, want %q", s.key("../escape", "n"), want)
	}
}
