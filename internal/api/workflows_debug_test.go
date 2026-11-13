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

// newWorkflowDebugTestServer wires a router with an in-memory store - no
// database needed.
func newWorkflowDebugTestServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()

	store := storage.NewMemoryStorage()
	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router, store
}

// postWorkflow POSTs a workflow document and returns the status code plus
// the stored workflow.
func postWorkflow(t *testing.T, router http.Handler, query string, body string) (int, *model.Workflow) {
	t.Helper()

	req := httptest.NewRequest("POST", "/api/v1/workflows"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var workflow model.Workflow
	if w.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workflow))
	}
	return w.Code, &workflow
}

// putWorkflow PUTs a workflow document and returns the status code.
func putWorkflow(t *testing.T, router http.Handler, id string, body string) int {
	t.Helper()

	req := httptest.NewRequest("PUT", "/api/v1/workflows/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w.Code
}

// TestCreateWorkflow_DebugDefaultsToOff pins the production default: a
// newly created workflow starts with Debug OFF.
func TestCreateWorkflow_DebugDefaultsToOff(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	code, created := postWorkflow(t, router, "", `{"name":"Fresh","nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusCreated, code)
	assert.False(t, created.Debug)

	stored, err := store.GetWorkflow(created.ID)
	require.NoError(t, err)
	assert.False(t, stored.Debug, "a new workflow is stored with Debug OFF")
}

// TestCreateWorkflow_NeverInheritsDebugFromPayload pins that a new
// workflow never inherits the flag from the payload - a copy of a
// debug=true workflow must not land as debug=true.
func TestCreateWorkflow_NeverInheritsDebugFromPayload(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	code, created := postWorkflow(t, router, "", `{"name":"Copied","debug":true,"nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusCreated, code)
	assert.False(t, created.Debug)

	stored, err := store.GetWorkflow(created.ID)
	require.NoError(t, err)
	assert.False(t, stored.Debug)
}

// TestCreateWorkflow_ExplicitDebugOptIn pins the explicit opt-in for API
// clients that really want a debug-enabled workflow from the start.
func TestCreateWorkflow_ExplicitDebugOptIn(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	code, created := postWorkflow(t, router, "?debug=true", `{"name":"Explicit","nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusCreated, code)
	assert.True(t, created.Debug)

	stored, err := store.GetWorkflow(created.ID)
	require.NoError(t, err)
	assert.True(t, stored.Debug)
}

// TestCreateWorkflow_ExistingWorkflowKeepsDebugWhenOmitted pins the
// upsert contract: POSTing an existing workflow without a "debug" key
// keeps the stored flag. The n8n sync bridge POSTs every synced
// workflow and never mentions the flag.
func TestCreateWorkflow_ExistingWorkflowKeepsDebugWhenOmitted(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-existing", Name: "Existing", Debug: true,
	}))

	code, _ := postWorkflow(t, router, "", `{"id":"wf-existing","name":"Renamed","nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusCreated, code)

	stored, err := store.GetWorkflow("wf-existing")
	require.NoError(t, err)
	assert.True(t, stored.Debug, "the stored flag survives a payload without a debug key")
}

// TestUpdateWorkflow_PreservesDebugWhenOmitted pins the same contract for
// the PUT path.
func TestUpdateWorkflow_PreservesDebugWhenOmitted(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-existing", Name: "Existing", Debug: true,
	}))

	code := putWorkflow(t, router, "wf-existing", `{"name":"Renamed","nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusOK, code)

	stored, err := store.GetWorkflow("wf-existing")
	require.NoError(t, err)
	assert.True(t, stored.Debug, "the stored flag survives a payload without a debug key")
}

// TestUpdateWorkflow_AppliesExplicitDebug pins that an explicit value -
// including an explicit false - is applied.
func TestUpdateWorkflow_AppliesExplicitDebug(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-existing", Name: "Existing", Debug: true,
	}))

	code := putWorkflow(t, router, "wf-existing", `{"name":"Renamed","debug":false,"nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusOK, code)

	stored, err := store.GetWorkflow("wf-existing")
	require.NoError(t, err)
	assert.False(t, stored.Debug, "an explicit debug=false is applied")

	code = putWorkflow(t, router, "wf-existing", `{"name":"Renamed","debug":true,"nodes":[],"connections":{}}`)
	require.Equal(t, http.StatusOK, code)

	stored, err = store.GetWorkflow("wf-existing")
	require.NoError(t, err)
	assert.True(t, stored.Debug)
}

// TestDuplicateWorkflow_DebugDefaultsToOff pins that a duplicate of a
// debug=true workflow is a new workflow and starts with Debug OFF.
func TestDuplicateWorkflow_DebugDefaultsToOff(t *testing.T) {
	router, store := newWorkflowDebugTestServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-original", Name: "Original", Debug: true,
	}))

	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-original/duplicate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var duplicate model.Workflow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &duplicate))
	assert.False(t, duplicate.Debug)

	stored, err := store.GetWorkflow(duplicate.ID)
	require.NoError(t, err)
	assert.False(t, stored.Debug)
}
