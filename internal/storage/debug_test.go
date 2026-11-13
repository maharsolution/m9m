package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/model"
)

// seedDebugWorkflows stores n workflows with the given Debug flag.
func seedDebugWorkflows(t *testing.T, store WorkflowStorage, prefix string, debug bool, count int) []*model.Workflow {
	t.Helper()

	workflows := make([]*model.Workflow, 0, count)
	for i := 0; i < count; i++ {
		workflow := &model.Workflow{
			ID:    prefix + "-" + string(rune('a'+i)),
			Name:  prefix + " " + string(rune('A'+i)),
			Debug: debug,
		}
		require.NoError(t, store.SaveWorkflow(workflow))
		workflows = append(workflows, workflow)
	}
	return workflows
}

// assertWorkflowDebug reads every workflow back and asserts the flag.
func assertWorkflowDebug(t *testing.T, store WorkflowStorage, expected bool) {
	t.Helper()

	workflows, total, err := store.ListWorkflows(WorkflowFilters{})
	require.NoError(t, err)
	require.Equal(t, len(workflows), total)

	for _, workflow := range workflows {
		stored, err := store.GetWorkflow(workflow.ID)
		require.NoError(t, err)
		assert.Equal(t, expected, stored.Debug, "workflow %s", workflow.ID)
	}
}

// TestApplyDebugToAll_MemoryEnablesEveryWorkflow pins the bulk enable:
// every stored workflow ends up with Debug=true and a second run is a
// no-op.
func TestApplyDebugToAll_MemoryEnablesEveryWorkflow(t *testing.T) {
	store := NewMemoryStorage()
	seedDebugWorkflows(t, store, "wf", false, 3)

	result, err := ApplyDebugToAll(store, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Changed)
	assert.Equal(t, 3, result.Enabled)
	assert.Equal(t, 0, result.Disabled)
	assert.False(t, result.DryRun)

	assertWorkflowDebug(t, store, true)

	// Second run: nothing changes.
	result, err = ApplyDebugToAll(store, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 0, result.Changed)
	assert.Equal(t, 3, result.Enabled)
	assert.Equal(t, 0, result.Disabled)

	assertWorkflowDebug(t, store, true)
}

// TestApplyDebugToAll_MemoryDisablesEveryWorkflow is the inverse: the
// command can also turn the flag back off.
func TestApplyDebugToAll_MemoryDisablesEveryWorkflow(t *testing.T) {
	store := NewMemoryStorage()
	seedDebugWorkflows(t, store, "wf", true, 3)

	result, err := ApplyDebugToAll(store, "", false, false)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Changed)
	assert.Equal(t, 0, result.Enabled)
	assert.Equal(t, 3, result.Disabled)

	assertWorkflowDebug(t, store, false)

	// Second run: nothing changes.
	result, err = ApplyDebugToAll(store, "", false, false)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Changed)
	assertWorkflowDebug(t, store, false)
}

// TestApplyDebugToAll_DryRunChangesNothing pins the read-only mode: the
// summary reports what would change, but no row is touched.
func TestApplyDebugToAll_DryRunChangesNothing(t *testing.T) {
	store := NewMemoryStorage()
	seedDebugWorkflows(t, store, "wf", false, 4)

	result, err := ApplyDebugToAll(store, "", true, true)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 4, result.Total)
	assert.Equal(t, 4, result.Changed, "the dry run reports what would change")
	assert.Equal(t, 0, result.Enabled)
	assert.Equal(t, 4, result.Disabled)

	assertWorkflowDebug(t, store, false)
}

// TestApplyDebugToAll_WorkspaceScope pins that the operator can limit
// the bulk update to a single workspace.
func TestApplyDebugToAll_WorkspaceScope(t *testing.T) {
	store := NewMemoryStorage()

	scoped := &model.Workflow{ID: "wf-scoped", Name: "Scoped", WorkspaceID: "ws-1"}
	other := &model.Workflow{ID: "wf-other", Name: "Other", WorkspaceID: "ws-2"}
	require.NoError(t, store.SaveWorkflow(scoped))
	require.NoError(t, store.SaveWorkflow(other))

	result, err := ApplyDebugToAll(store, "ws-1", true, false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Changed)

	scopedStored, err := store.GetWorkflow("wf-scoped")
	require.NoError(t, err)
	assert.True(t, scopedStored.Debug)

	otherStored, err := store.GetWorkflow("wf-other")
	require.NoError(t, err)
	assert.False(t, otherStored.Debug, "the other workspace is untouched")
}

// TestApplyDebugToAll_MixedStateOnlyChangesTheOutliers pins that a
// workflow already carrying the target value is never rewritten.
func TestApplyDebugToAll_MixedStateOnlyChangesTheOutliers(t *testing.T) {
	store := NewMemoryStorage()
	seedDebugWorkflows(t, store, "on", true, 2)
	seedDebugWorkflows(t, store, "off", false, 3)

	result, err := ApplyDebugToAll(store, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, 5, result.Total)
	assert.Equal(t, 3, result.Changed)
	assert.Equal(t, 5, result.Enabled)
	assert.Equal(t, 0, result.Disabled)

	assertWorkflowDebug(t, store, true)
}

// TestStorageBackendsImplementDebugBulkSetter pins that the backends
// expose the bulk capability - the SQL ones persist the flag in the
// dedicated `workflows.debug` column.
func TestStorageBackendsImplementDebugBulkSetter(t *testing.T) {
	var _ DebugBulkSetter = (*SQLiteStorage)(nil)
	var _ DebugBulkSetter = (*MySQLStorage)(nil)
	var _ DebugBulkSetter = (*PostgresStorage)(nil)
	var _ DebugBulkSetter = (*MemoryStorage)(nil)
}

// newSQLiteDebugTestStore opens a throwaway SQLite database. The test is
// skipped when the binary was built without cgo - go-sqlite3 needs it.
func newSQLiteDebugTestStore(t *testing.T) *SQLiteStorage {
	t.Helper()

	store, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "debug.db"))
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skipf("sqlite3 needs cgo: %v", err)
		}
		t.Fatalf("failed to open the sqlite storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}

// TestApplyDebugToAll_SQLiteEnablesEveryWorkflow runs the bulk enable
// against a real SQL backend with its own `workflows.debug` column.
func TestApplyDebugToAll_SQLiteEnablesEveryWorkflow(t *testing.T) {
	store := newSQLiteDebugTestStore(t)
	seedDebugWorkflows(t, store, "wf", false, 3)

	result, err := ApplyDebugToAll(store, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Changed)
	assert.Equal(t, 3, result.Enabled)

	assertWorkflowDebug(t, store, true)

	// Second run: nothing changes.
	result, err = ApplyDebugToAll(store, "", true, false)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Changed)
	assert.Equal(t, 3, result.Enabled)

	assertWorkflowDebug(t, store, true)
}

// TestApplyDebugToAll_SQLiteDryRunChangesNothing pins the read-only mode
// on the SQL backend.
func TestApplyDebugToAll_SQLiteDryRunChangesNothing(t *testing.T) {
	store := newSQLiteDebugTestStore(t)
	seedDebugWorkflows(t, store, "wf", false, 2)

	result, err := ApplyDebugToAll(store, "", true, true)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 2, result.Changed)
	assert.Equal(t, 0, result.Enabled)

	assertWorkflowDebug(t, store, false)
}

// TestMemoryStorage_NewWorkflowKeepsDebugOff pins the default of the
// storage layer: a workflow that is stored without the flag comes back
// with it off.
func TestMemoryStorage_NewWorkflowKeepsDebugOff(t *testing.T) {
	store := NewMemoryStorage()

	require.NoError(t, store.SaveWorkflow(&model.Workflow{ID: "wf-new", Name: "New"}))

	stored, err := store.GetWorkflow("wf-new")
	require.NoError(t, err)
	assert.False(t, stored.Debug)
}
