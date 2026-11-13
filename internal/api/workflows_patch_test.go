package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// newPatchTestServer wires a router with an in-memory store - no database
// needed.
func newPatchTestServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()

	store := storage.NewMemoryStorage()
	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router, store
}

// patchWorkflowRequest sends a PATCH to /api/v1/workflows/{id} and returns
// the status code plus the decoded response body.
func patchWorkflowRequest(t *testing.T, router http.Handler, id string, body string) (int, *model.Workflow) {
	t.Helper()

	req := httptest.NewRequest("PATCH", "/api/v1/workflows/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var workflow model.Workflow
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workflow))
	}
	return w.Code, &workflow
}

// putWorkflowRequest sends a PUT to /api/v1/workflows/{id} and returns the
// status code plus the decoded response body.
func putWorkflowRequest(t *testing.T, router http.Handler, id string, body string) (int, *model.Workflow) {
	t.Helper()

	req := httptest.NewRequest("PUT", "/api/v1/workflows/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var workflow model.Workflow
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workflow))
	}
	return w.Code, &workflow
}

// seedWorkflow stores a workflow with a name, two nodes and one connection
// so a partial update can be checked against all three.
func seedWorkflow(t *testing.T, store *storage.MemoryStorage, id string) {
	t.Helper()

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:   id,
		Name: "Original Name",
		Nodes: []model.Node{
			{ID: "node-1", Name: "Webhook", Type: "n8n-nodes-base.webhook"},
			{ID: "node-2", Name: "Set", Type: "n8n-nodes-base.set"},
		},
		Connections: map[string]model.Connections{
			"Webhook": {
				Main: [][]model.Connection{
					{{Node: "Set", Type: "main", Index: 0}},
				},
			},
		},
	}))
}

// TestPatchWorkflow_ActiveOnlyKeepsNameNodesAndConnections pins the core
// contract of a partial update: {"active":true} must not touch anything
// else. The old implementation replaced the whole record with the decoded
// body, so the name became "" and every connection was destroyed.
func TestPatchWorkflow_ActiveOnlyKeepsNameNodesAndConnections(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-patch-active")

	code, response := patchWorkflowRequest(t, router, "wf-patch-active", `{"active":true}`)
	require.Equal(t, http.StatusOK, code)

	assert.True(t, response.Active)
	assert.Equal(t, "Original Name", response.Name)
	assert.Len(t, response.Nodes, 2)
	assert.Len(t, response.Connections, 1)

	stored, err := store.GetWorkflow("wf-patch-active")
	require.NoError(t, err)
	assert.True(t, stored.Active)
	assert.Equal(t, "Original Name", stored.Name)
	require.Len(t, stored.Nodes, 2)
	assert.Equal(t, "Webhook", stored.Nodes[0].Name)
	require.Len(t, stored.Connections, 1)
	require.Len(t, stored.Connections["Webhook"].Main, 1)
	assert.Equal(t, "Set", stored.Connections["Webhook"].Main[0][0].Node)
}

// TestPatchWorkflow_NameOnlyKeepsConnectionsAndNodes pins that renaming a
// workflow leaves the graph untouched.
func TestPatchWorkflow_NameOnlyKeepsConnectionsAndNodes(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-patch-name")

	code, response := patchWorkflowRequest(t, router, "wf-patch-name", `{"name":"Renamed"}`)
	require.Equal(t, http.StatusOK, code)

	assert.Equal(t, "Renamed", response.Name)
	assert.Len(t, response.Nodes, 2)
	assert.Len(t, response.Connections, 1)

	stored, err := store.GetWorkflow("wf-patch-name")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.Name)
	assert.False(t, stored.Active)
	require.Len(t, stored.Nodes, 2)
	require.Len(t, stored.Connections, 1)
	assert.Equal(t, "Set", stored.Connections["Webhook"].Main[0][0].Node)
}

// TestPatchWorkflow_ConnectionsReplacesStoredConnections pins that a
// connection map sent via PATCH is applied - and applied completely.
func TestPatchWorkflow_ConnectionsReplacesStoredConnections(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-patch-connections")

	code, response := patchWorkflowRequest(t, router, "wf-patch-connections",
		`{"connections":{"Set":{"main":[[{"node":"Webhook","type":"main","index":0}]]}}}`)
	require.Equal(t, http.StatusOK, code)

	require.Len(t, response.Connections, 1)
	require.Contains(t, response.Connections, "Set")
	require.Len(t, response.Connections["Set"].Main, 1)
	assert.Equal(t, "Webhook", response.Connections["Set"].Main[0][0].Node)

	stored, err := store.GetWorkflow("wf-patch-connections")
	require.NoError(t, err)
	require.Len(t, stored.Connections, 1)
	require.Contains(t, stored.Connections, "Set")
	assert.Equal(t, "Webhook", stored.Connections["Set"].Main[0][0].Node)
	// The rest of the record survives the patch.
	assert.Equal(t, "Original Name", stored.Name)
	assert.Len(t, stored.Nodes, 2)
}

// TestPatchWorkflow_ExplicitEmptyNameIsApplied pins that the presence map
// distinguishes an intentional empty value from an absent field: an
// explicit {"name":""} empties the name.
func TestPatchWorkflow_ExplicitEmptyNameIsApplied(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-patch-empty-name")

	code, _ := patchWorkflowRequest(t, router, "wf-patch-empty-name", `{"name":""}`)
	require.Equal(t, http.StatusOK, code)

	stored, err := store.GetWorkflow("wf-patch-empty-name")
	require.NoError(t, err)
	assert.Equal(t, "", stored.Name)
	assert.Len(t, stored.Nodes, 2)
	assert.Len(t, stored.Connections, 1)
}

// TestPatchWorkflow_UnknownWorkflowIsNotFound pins the error contract of a
// partial update on an unknown id.
func TestPatchWorkflow_UnknownWorkflowIsNotFound(t *testing.T) {
	router, _ := newPatchTestServer(t)

	code, _ := patchWorkflowRequest(t, router, "wf-does-not-exist", `{"active":true}`)
	assert.Equal(t, http.StatusNotFound, code)
}

// TestPatchWorkflow_InvalidJSON pins that a broken body is rejected instead
// of wiping the stored workflow.
func TestPatchWorkflow_InvalidJSON(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-patch-invalid")

	code, _ := patchWorkflowRequest(t, router, "wf-patch-invalid", `{"active":`)
	assert.Equal(t, http.StatusBadRequest, code)

	stored, err := store.GetWorkflow("wf-patch-invalid")
	require.NoError(t, err)
	assert.Equal(t, "Original Name", stored.Name)
	assert.Len(t, stored.Connections, 1)
}

// TestPutWorkflow_ReplacesWholeRecord pins that PUT keeps its full-replace
// semantics: the web UI depends on it.
func TestPutWorkflow_ReplacesWholeRecord(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-put-replace")

	code, response := putWorkflowRequest(t, router, "wf-put-replace", `{"name":"Replaced","active":true}`)
	require.Equal(t, http.StatusOK, code)

	assert.Equal(t, "Replaced", response.Name)
	assert.True(t, response.Active)
	assert.Empty(t, response.Nodes)
	assert.Empty(t, response.Connections)

	stored, err := store.GetWorkflow("wf-put-replace")
	require.NoError(t, err)
	assert.Equal(t, "Replaced", stored.Name)
	assert.True(t, stored.Active)
	assert.Empty(t, stored.Nodes)
	assert.Empty(t, stored.Connections)
}

// TestActivateWorkflow_KeepsWorkflowIntact pins that activating a workflow
// does not wipe its graph.
func TestActivateWorkflow_KeepsWorkflowIntact(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-activate")

	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-activate/activate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := store.GetWorkflow("wf-activate")
	require.NoError(t, err)
	assert.True(t, stored.Active)
	assert.Equal(t, "Original Name", stored.Name)
	assert.Len(t, stored.Nodes, 2)
	require.Len(t, stored.Connections, 1)
	assert.Equal(t, "Set", stored.Connections["Webhook"].Main[0][0].Node)
}

// TestDeactivateWorkflow_KeepsWorkflowIntact pins that deactivating a
// workflow does not wipe its graph either.
func TestDeactivateWorkflow_KeepsWorkflowIntact(t *testing.T) {
	router, store := newPatchTestServer(t)
	seedWorkflow(t, store, "wf-deactivate")

	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-deactivate/deactivate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := store.GetWorkflow("wf-deactivate")
	require.NoError(t, err)
	assert.False(t, stored.Active)
	assert.Equal(t, "Original Name", stored.Name)
	assert.Len(t, stored.Nodes, 2)
	require.Len(t, stored.Connections, 1)
	assert.Equal(t, "Set", stored.Connections["Webhook"].Main[0][0].Node)
}
