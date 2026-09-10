package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecutor struct {
	result *WorkflowResult
	err    error
}

func (m *mockExecutor) ExecuteWorkflowWithContext(ctx context.Context, workflow *model.Workflow, inputData []model.DataItem) (*WorkflowResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockLookup struct {
	wfs map[string]*model.Workflow
	err error
}

func (m *mockLookup) GetWorkflow(id string) (*model.Workflow, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.wfs[id], nil
}

func writeTestWorkflow(t *testing.T, dir string) string {
	t.Helper()
	wf := model.Workflow{
		Name: "sub-workflow",
		Nodes: []model.Node{
			{Name: "Start", Type: "n8n-nodes-base.start", Position: []int{0, 0}},
		},
	}
	data, _ := json.Marshal(wf)
	path := filepath.Join(dir, "sub.json")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func TestExecuteWorkflowNode_Execute(t *testing.T) {
	t.Run("executes sub-workflow via path", func(t *testing.T) {
		dir := t.TempDir()
		wfPath := writeTestWorkflow(t, dir)

		executor := &mockExecutor{
			result: &WorkflowResult{
				Data: []model.DataItem{{JSON: map[string]interface{}{"result": "ok"}}},
			},
		}

		node := NewExecuteWorkflowNode(executor, nil)
		params := map[string]interface{}{
			"source":       "localFile",
			"workflowPath": wfPath,
		}

		input := []model.DataItem{{JSON: map[string]interface{}{"input": "data"}}}
		result, err := node.Execute(input, params)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "ok", result[0].JSON["result"])
	})

	t.Run("executes sub-workflow via workflowId string", func(t *testing.T) {
		executor := &mockExecutor{
			result: &WorkflowResult{
				Data: []model.DataItem{{JSON: map[string]interface{}{"hello": "world"}}},
			},
		}
		lookup := &mockLookup{wfs: map[string]*model.Workflow{
			"child-1": {Name: "child", Nodes: []model.Node{{Name: "Start"}}},
		}}
		node := NewExecuteWorkflowNode(executor, lookup)

		result, err := node.Execute(nil, map[string]interface{}{
			"workflowId": "child-1",
		})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "world", result[0].JSON["hello"])
	})

	t.Run("executes sub-workflow via resource-locator object", func(t *testing.T) {
		executor := &mockExecutor{
			result: &WorkflowResult{Data: []model.DataItem{{JSON: map[string]interface{}{"ok": true}}}},
		}
		lookup := &mockLookup{wfs: map[string]*model.Workflow{
			"abc": {Name: "abc"},
		}}
		node := NewExecuteWorkflowNode(executor, lookup)

		_, err := node.Execute(nil, map[string]interface{}{
			"workflowId": map[string]interface{}{
				"__rl":     true,
				"value":    "abc",
				"mode":     "list",
				"cachedResultName": "abc",
			},
		})
		require.NoError(t, err)
	})

	t.Run("workflowId without lookup fails clearly", func(t *testing.T) {
		executor := &mockExecutor{}
		node := NewExecuteWorkflowNode(executor, nil)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowId": "any-id",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflowId is set but no workflow lookup is configured")
	})

	t.Run("workflowId not found", func(t *testing.T) {
		executor := &mockExecutor{}
		lookup := &mockLookup{wfs: map[string]*model.Workflow{}}
		node := NewExecuteWorkflowNode(executor, lookup)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowId": "missing",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("lookup error surfaces", func(t *testing.T) {
		executor := &mockExecutor{}
		lookup := &mockLookup{err: errors.New("boom")}
		node := NewExecuteWorkflowNode(executor, lookup)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowId": "anything",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("no executor", func(t *testing.T) {
		node := NewExecuteWorkflowNode(nil, nil)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowPath": "/tmp/test.json",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no workflow engine")
	})

	t.Run("max depth exceeded", func(t *testing.T) {
		dir := t.TempDir()
		wfPath := writeTestWorkflow(t, dir)

		executor := &mockExecutor{
			result: &WorkflowResult{Data: []model.DataItem{}},
		}

		node := NewExecuteWorkflowNode(executor, nil)
		node.depth = 10
		params := map[string]interface{}{
			"workflowPath": wfPath,
			"maxDepth":     10,
		}

		_, err := node.Execute(nil, params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "maximum recursion depth")
	})

	t.Run("missing workflow id and path", func(t *testing.T) {
		executor := &mockExecutor{}
		node := NewExecuteWorkflowNode(executor, nil)
		_, err := node.Execute(nil, map[string]interface{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflowId is required")
	})

	t.Run("file not found", func(t *testing.T) {
		executor := &mockExecutor{}
		node := NewExecuteWorkflowNode(executor, nil)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowPath": "/nonexistent/workflow.json",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read workflow file")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))

		executor := &mockExecutor{}
		node := NewExecuteWorkflowNode(executor, nil)
		_, err := node.Execute(nil, map[string]interface{}{
			"workflowPath": path,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid workflow JSON")
	})

	t.Run("unsupported source", func(t *testing.T) {
		// source is now validated up-front in ValidateParameters, not
		// at Execute time. Cover the rejection at the validation layer
		// so the contract is documented in tests.
		node := NewExecuteWorkflowNode(nil, nil)
		err := node.ValidateParameters(map[string]interface{}{
			"source":       "remote",
			"workflowPath": "/tmp/test.json",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported source")
	})
}

func TestExecuteWorkflowNode_ValidateParameters(t *testing.T) {
	node := NewExecuteWorkflowNode(nil, nil)

	assert.NoError(t, node.ValidateParameters(nil))
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"workflowPath": "",
	}))
	assert.NoError(t, node.ValidateParameters(map[string]interface{}{
		"workflowPath": "/some/path.json",
	}))
	assert.Error(t, node.ValidateParameters(map[string]interface{}{
		"source":       "remote",
		"workflowPath": "/some/path.json",
	}))

	// workflowId string + lookup
	withLookup := NewExecuteWorkflowNode(nil, &mockLookup{})
	assert.NoError(t, withLookup.ValidateParameters(map[string]interface{}{
		"workflowId": "abc",
	}))
	// workflowId as resource-locator object
	assert.NoError(t, withLookup.ValidateParameters(map[string]interface{}{
		"workflowId": map[string]interface{}{"__rl": true, "value": "abc"},
	}))
	// workflowId with empty value falls through to path-check, fails.
	assert.Error(t, withLookup.ValidateParameters(map[string]interface{}{
		"workflowId": "",
	}))
	// workflowId object with empty value falls through.
	assert.Error(t, withLookup.ValidateParameters(map[string]interface{}{
		"workflowId": map[string]interface{}{"__rl": true, "value": ""},
	}))
}

func TestExecuteWorkflowNode_Description(t *testing.T) {
	node := NewExecuteWorkflowNode(nil, nil)
	desc := node.Description()
	assert.Equal(t, "Execute Workflow", desc.Name)
	assert.Equal(t, "Core", desc.Category)
}
