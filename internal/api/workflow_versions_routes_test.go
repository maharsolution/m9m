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
	"github.com/neul-labs/m9m/internal/versions"
)

// newVersionRoutesTestServer wires a router with the workflow-version
// routes registered against an in-memory store — no database needed.
//
// The version manager is installed through the same package-level setter
// cmd/m9m/commands/serve.go uses, so the test exercises the production
// wiring path.
func newVersionRoutesTestServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()

	store := storage.NewMemoryStorage()
	manager := versions.NewVersionManager(versions.NewPersistentVersionStorage(store), store)

	SetVersionManager(manager)
	t.Cleanup(func() { SetVersionManager(nil) })

	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router, store
}

// saveVersionTestWorkflow stores a workflow so the version routes can
// resolve it.
func saveVersionTestWorkflow(t *testing.T, store *storage.MemoryStorage, id string) *model.Workflow {
	t.Helper()

	workflow := &model.Workflow{ID: id, Name: "Version Test"}
	require.NoError(t, store.SaveWorkflow(workflow))
	return workflow
}

// versionListEnvelope is the JSON body of GET
// /api/v1/workflows/{id}/versions.
type versionListEnvelope struct {
	Data   []versions.WorkflowVersion `json:"data"`
	Total  int                        `json:"total"`
	Count  int                        `json:"count"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

// routeIndex returns the registration index of a path template in the
// router's route table. gorilla/mux resolves routes in registration
// order, so the index is the ordering assertion the task asks for.
//
// The template may be given with or without the "/api/v1" prefix — the
// subrouter prefixes are not part of the route's own template.
//
// -1 means the route was not registered at all.
func routeIndex(router *mux.Router, pathTemplate string) int {
	index := -1
	position := 0

	_ = router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		template, err := route.GetPathTemplate()
		if err == nil && index == -1 &&
			(template == pathTemplate || strings.HasSuffix(template, pathTemplate)) {
			index = position
		}
		position++
		return nil
	})

	return index
}

// TestVersionRoutesRegistered asserts the version routes exist — the
// live server answered 404 for them because they were never wired.
func TestVersionRoutesRegistered(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-versions")

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-versions/versions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope versionListEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Empty(t, envelope.Data)
	assert.Equal(t, 0, envelope.Total)
	assert.Equal(t, 0, envelope.Count)
	assert.Equal(t, 50, envelope.Limit)
	assert.Equal(t, 0, envelope.Offset)
}

// TestVersionRoutesRegisteredBeforeWorkflowWildcard asserts the route
// ordering: the "/workflows/{id}/versions" collection must be
// registered before the "/workflows/{id}" wildcard, otherwise gorilla/mux
// would answer with the workflow lookup instead of the version list.
func TestVersionRoutesRegisteredBeforeWorkflowWildcard(t *testing.T) {
	router, _ := newVersionRoutesTestServer(t)

	versionsIndex := routeIndex(router, "/workflows/{id}/versions")
	wildcardIndex := routeIndex(router, "/workflows/{id}")

	require.NotEqual(t, -1, versionsIndex, "version collection route must be registered")
	require.NotEqual(t, -1, wildcardIndex, "workflow wildcard route must be registered")
	assert.Less(t, versionsIndex, wildcardIndex,
		"the version collection route has to be registered before the workflow wildcard")
}

// TestVersionListNotShadowedByWorkflowRoute asserts the ordering
// behaviourally: a known workflow answers with the version envelope on
// /versions and with the workflow document on the wildcard path.
func TestVersionListNotShadowedByWorkflowRoute(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-shadow")

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-shadow/versions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\"")
	assert.NotContains(t, w.Body.String(), "\"nodes\"")

	// The wildcard route still serves the workflow itself.
	req = httptest.NewRequest("GET", "/api/v1/workflows/wf-shadow", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "wf-shadow")
}

// TestVersionCompareRouteNotShadowedByVersionIDRoute asserts the
// static "/versions/compare" segment wins over the "{versionId}"
// wildcard: without the ordering the compare request would be read as
// a version whose id is "compare" and answer 404 instead of 400.
func TestVersionCompareRouteNotShadowedByVersionIDRoute(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-compare")

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-compare/versions/compare", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Contains(t, envelope["message"], "from")
}

// TestVersionListUnknownWorkflow asserts a 404 (not an empty list) for
// a workflow the store does not know.
func TestVersionListUnknownWorkflow(t *testing.T) {
	router, _ := newVersionRoutesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/workflows/does-not-exist/versions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusNotFound), envelope["code"])
	assert.Contains(t, envelope["message"], "Workflow not found")
}

// TestVersionGetUnknownVersion asserts a 404 for an unknown version id.
func TestVersionGetUnknownVersion(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-single")

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-single/versions/ver_missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Contains(t, envelope["message"], "Version not found")
}

// TestVersionCreateListAndGet covers the full snapshot round trip the
// UI needs: create a snapshot, list the snapshots, fetch one.
func TestVersionCreateListAndGet(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-roundtrip")

	body := strings.NewReader(`{"versionTag":"v1.0.0","description":"first"}`)
	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-roundtrip/versions", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var created versions.WorkflowVersion
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "wf-roundtrip", created.WorkflowID)
	assert.Equal(t, "v1.0.0", created.VersionTag)
	assert.Equal(t, 1, created.VersionNum)

	// List.
	req = httptest.NewRequest("GET", "/api/v1/workflows/wf-roundtrip/versions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope versionListEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, 1, envelope.Total)
	assert.Equal(t, created.ID, envelope.Data[0].ID)

	// Single.
	req = httptest.NewRequest("GET", "/api/v1/workflows/wf-roundtrip/versions/"+created.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var single versions.WorkflowVersion
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &single))
	assert.Equal(t, created.ID, single.ID)
}

// TestVersionRestore asserts the restore endpoint answers with the
// envelope the frontend expects: {message, restoredFrom, backupCreated}.
func TestVersionRestore(t *testing.T) {
	router, store := newVersionRoutesTestServer(t)
	saveVersionTestWorkflow(t, store, "wf-restore")

	body := strings.NewReader(`{"versionTag":"v1","description":"snapshot"}`)
	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-restore/versions", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created versions.WorkflowVersion
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	req = httptest.NewRequest("POST", "/api/v1/workflows/wf-restore/versions/"+created.ID+"/restore",
		strings.NewReader(`{"createBackup":true}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Message       string                   `json:"message"`
		RestoredFrom  versions.WorkflowVersion `json:"restoredFrom"`
		BackupCreated bool                     `json:"backupCreated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.NotEmpty(t, envelope.Message)
	assert.Equal(t, created.ID, envelope.RestoredFrom.ID)
	assert.True(t, envelope.BackupCreated)
}

// TestVersionRoutesWithoutManager asserts the graceful degradation: the
// routes exist even when no manager was wired, and answer 503 instead
// of panicking.
func TestVersionRoutesWithoutManager(t *testing.T) {
	SetVersionManager(nil)

	store := storage.NewMemoryStorage()
	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-1/versions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusServiceUnavailable), envelope["code"])
}
