package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetWorkflowsDebugFlags puts the command flags back into their
// default state so one test cannot leak into the next.
func resetWorkflowsDebugFlags(t *testing.T) {
	t.Helper()

	workflowsDebugEnable = false
	workflowsDebugDisable = false
	workflowsDebugAll = false
	workflowsDebugDryRun = false
	workflowsDebugDBType = ""
	workflowsDebugDB = ""
	workflowsDebugPostgres = ""
	workflowsDebugMySQL = ""
	workflowsDebugMemory = false
}

// TestRunWorkflowsDebug_RequiresAll pins the safety rail: the command
// only ever operates on every existing workflow, so --all is mandatory.
func TestRunWorkflowsDebug_RequiresAll(t *testing.T) {
	resetWorkflowsDebugFlags(t)
	t.Cleanup(func() { resetWorkflowsDebugFlags(t) })

	workflowsDebugEnable = true

	err := runWorkflowsDebug(workflowsDebugCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all is required")
}

// TestRunWorkflowsDebug_RejectsEnableAndDisable pins that the two
// switches cannot be combined.
func TestRunWorkflowsDebug_RejectsEnableAndDisable(t *testing.T) {
	resetWorkflowsDebugFlags(t)
	t.Cleanup(func() { resetWorkflowsDebugFlags(t) })

	workflowsDebugEnable = true
	workflowsDebugDisable = true
	workflowsDebugAll = true

	err := runWorkflowsDebug(workflowsDebugCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// TestRunWorkflowsDebug_MemoryBackend runs the command end to end against
// the in-memory backend - the same code path the MySQL backend uses,
// minus the SQL statement.
func TestRunWorkflowsDebug_MemoryBackend(t *testing.T) {
	resetWorkflowsDebugFlags(t)
	t.Cleanup(func() { resetWorkflowsDebugFlags(t) })

	workflowsDebugEnable = true
	workflowsDebugAll = true
	workflowsDebugMemory = true

	require.NoError(t, runWorkflowsDebug(workflowsDebugCmd, nil))
}
