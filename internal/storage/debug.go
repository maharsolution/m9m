package storage

import (
	"fmt"

	"github.com/neul-labs/m9m/internal/model"
)

// debugBulkPageSize is the number of workflows the generic list + save
// fallback reads per page. The SQL backends never use the fallback (they
// answer with a single UPDATE), so this only matters for the JSON-document
// backends, which are small by nature.
const debugBulkPageSize = 200

// DebugBulkSetter is the optional capability interface for storage
// backends that can flip the per-workflow Debug flag in bulk.
//
// The SQL backends (MySQL, Postgres, SQLite) persist Debug in a dedicated
// `workflows.debug` column, so they answer with a single UPDATE statement.
// The in-memory backend keeps the flag on the Go struct. The JSON-document
// backends (Badger) do NOT implement this interface - ApplyDebugToAll
// falls back to a list + save loop for them, exactly like the n8n
// workflow documents have to be rewritten as a whole.
type DebugBulkSetter interface {
	// SetAllWorkflowsDebug sets Debug on every workflow - optionally
	// scoped to workspaceID, where the empty string means "every
	// workspace" - and returns the number of workflows it changed.
	//
	// The implementation must be idempotent: a workflow that already
	// carries the target value is never written, so a second run
	// reports 0 changed rows.
	SetAllWorkflowsDebug(workspaceID string, enabled bool) (int, error)

	// CountWorkflowsWithDebug counts the workflows whose Debug flag
	// currently equals enabled, optionally scoped to workspaceID.
	CountWorkflowsWithDebug(workspaceID string, enabled bool) (int, error)
}

// DebugBulkResult describes the outcome of a bulk Debug update.
type DebugBulkResult struct {
	// Total is the number of workflows the operation inspected.
	Total int
	// Changed is the number of workflows whose Debug flag flipped.
	// For a dry run this is the number that WOULD flip.
	Changed int
	// Enabled and Disabled count the workflows carrying each value
	// after the operation - before it, for a dry run.
	Enabled  int
	Disabled int
	// DryRun reports that the operation was read-only.
	DryRun bool
}

// ApplyDebugToAll flips the Debug flag of every workflow in store.
//
// The operation is idempotent: a workflow that already carries the target
// value is never written, so a second run changes nothing. dryRun makes
// the call read-only - it reports what would change without touching a
// row.
//
// workspaceID scopes the operation to a single workspace; the empty string
// means "every workspace".
func ApplyDebugToAll(store WorkflowStorage, workspaceID string, enabled bool, dryRun bool) (DebugBulkResult, error) {
	result := DebugBulkResult{DryRun: dryRun}

	if store == nil {
		return result, fmt.Errorf("storage is nil")
	}

	enabledCount, err := countWorkflowsWithDebug(store, workspaceID, true)
	if err != nil {
		return result, err
	}
	disabledCount, err := countWorkflowsWithDebug(store, workspaceID, false)
	if err != nil {
		return result, err
	}

	result.Total = enabledCount + disabledCount
	result.Enabled = enabledCount
	result.Disabled = disabledCount

	// The workflows that do not carry the target value yet are the ones
	// the operation flips.
	if enabled {
		result.Changed = disabledCount
	} else {
		result.Changed = enabledCount
	}

	if dryRun {
		return result, nil
	}

	changed, err := setAllWorkflowsDebug(store, workspaceID, enabled)
	if err != nil {
		return result, err
	}
	result.Changed = changed

	// Refresh the post-state counts so the caller can verify the result
	// instead of trusting the write.
	postEnabled, err := countWorkflowsWithDebug(store, workspaceID, true)
	if err != nil {
		return result, err
	}
	postDisabled, err := countWorkflowsWithDebug(store, workspaceID, false)
	if err != nil {
		return result, err
	}
	result.Enabled = postEnabled
	result.Disabled = postDisabled

	return result, nil
}

// countWorkflowsWithDebug counts the workflows carrying the given flag
// value, using the backend's own counter when it has one.
func countWorkflowsWithDebug(store WorkflowStorage, workspaceID string, enabled bool) (int, error) {
	if setter, ok := store.(DebugBulkSetter); ok {
		count, err := setter.CountWorkflowsWithDebug(workspaceID, enabled)
		if err != nil {
			return 0, fmt.Errorf("failed to count workflows with debug=%t: %w", enabled, err)
		}
		return count, nil
	}

	workflows, err := listAllWorkflows(store, workspaceID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, workflow := range workflows {
		if workflow.Debug == enabled {
			count++
		}
	}
	return count, nil
}

// setAllWorkflowsDebug flips the flag on every workflow, using the
// backend's own bulk statement when it has one.
func setAllWorkflowsDebug(store WorkflowStorage, workspaceID string, enabled bool) (int, error) {
	if setter, ok := store.(DebugBulkSetter); ok {
		changed, err := setter.SetAllWorkflowsDebug(workspaceID, enabled)
		if err != nil {
			return 0, fmt.Errorf("failed to set workflows.debug=%t: %w", enabled, err)
		}
		return changed, nil
	}

	// Fallback for the JSON-document backends: read every workflow, flip
	// the flag on the ones that need it and write them back. A workflow
	// that already carries the target value is never written, so the
	// operation stays idempotent.
	workflows, err := listAllWorkflows(store, workspaceID)
	if err != nil {
		return 0, err
	}

	changed := 0
	for _, workflow := range workflows {
		if workflow.Debug == enabled {
			continue
		}
		workflow.Debug = enabled
		if err := store.UpdateWorkflow(workflow.ID, workflow); err != nil {
			return changed, fmt.Errorf("failed to update workflow %s: %w", workflow.ID, err)
		}
		changed++
	}
	return changed, nil
}

// listAllWorkflows reads every workflow, paging through the backend.
//
// Only the JSON-document backends reach this helper - they iterate their
// key space in a stable order, so the paging is deterministic.
func listAllWorkflows(store WorkflowStorage, workspaceID string) ([]*model.Workflow, error) {
	var all []*model.Workflow

	offset := 0
	for {
		page, _, err := store.ListWorkflows(WorkflowFilters{
			WorkspaceID: workspaceID,
			Offset:      offset,
			Limit:       debugBulkPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list workflows: %w", err)
		}

		all = append(all, page...)
		if len(page) < debugBulkPageSize {
			break
		}
		offset += len(page)
	}

	return all, nil
}
