package webhooks

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/trigger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newWebhookWorkflow builds the smallest workflow shape the test-webhook
// index has to understand: a single n8n Webhook trigger node.
func newWebhookWorkflow(id, path string, active bool) *model.Workflow {
	return &model.Workflow{
		ID:     id,
		Name:   id,
		Active: active,
		Nodes: []model.Node{{
			Name: "Webhook",
			Type: "n8n-nodes-base.webhook",
			Parameters: map[string]interface{}{
				"path":         path,
				"httpMethod":   "POST",
				"responseMode": "lastNode",
			},
		}},
	}
}

// newTestRouter wires the webhook handler onto a fresh mux router.
func newTestRouter(mgr *WebhookManager) *mux.Router {
	router := mux.NewRouter()
	NewHandler(mgr).RegisterRoutes(router)
	return router
}

// ---------------------------------------------------------------------------
// Test-webhook index tests
// ---------------------------------------------------------------------------

// TestWebhookManager_TestPath_ResolvesInactiveWorkflow is the core of the
// n8n parity fix: `/webhook-test/<path>` has to resolve the workflow even
// when it is NOT active, while `/webhook/<path>` (production) keeps
// 404-ing until the workflow is activated.
func TestWebhookManager_TestPath_ResolvesInactiveWorkflow(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := newWebhookWorkflow("wf-inactive", "webhook_xml", false)
	require.NoError(t, ws.SaveWorkflow(wf))
	require.NoError(t, mgr.RefreshTestHooks())

	hook, err := mgr.GetWebhookByPath("/webhook_xml", "POST", true)
	require.NoError(t, err, "test URL must resolve an inactive workflow")
	assert.Equal(t, "wf-inactive", hook.WorkflowID)
	assert.Equal(t, "Webhook", hook.NodeID, "test URL must point at the same webhook node")
	assert.True(t, hook.IsTest)
	assert.False(t, hook.Active, "an inactive workflow stays inactive; only the URL differs")

	_, err = mgr.GetWebhookByPath("/webhook_xml", "POST", false)
	assert.Error(t, err, "production URL must not resolve an inactive workflow")
}

// TestWebhookManager_TestPath_MapsToSameWorkflowAsProduction pins the
// "both URLs address the same webhook node / workflow" contract.
func TestWebhookManager_TestPath_MapsToSameWorkflowAsProduction(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := newWebhookWorkflow("wf-active", "webhook_xml", true)
	require.NoError(t, ws.SaveWorkflow(wf))
	require.NoError(t, mgr.RegisterWorkflowWebhooks(wf, false))
	require.NoError(t, mgr.RefreshTestHooks())

	prod, err := mgr.GetWebhookByPath("/webhook_xml", "POST", false)
	require.NoError(t, err)

	test, err := mgr.GetWebhookByPath("/webhook_xml", "POST", true)
	require.NoError(t, err)

	assert.Equal(t, prod.WorkflowID, test.WorkflowID)
	assert.Equal(t, prod.NodeID, test.NodeID)
	assert.Equal(t, prod.Path, test.Path)
	assert.Equal(t, prod.Method, test.Method)
}

// TestWebhookManager_TestPath_PrefersActiveWorkflow makes the tie-break
// deterministic when two workflows expose the same path: the active one
// wins (its production URL is live as well).
func TestWebhookManager_TestPath_PrefersActiveWorkflow(t *testing.T) {
	mgr, _, ws := newTestManager()

	inactive := newWebhookWorkflow("wf-inactive", "shared_path", false)
	active := newWebhookWorkflow("wf-active", "shared_path", true)
	require.NoError(t, ws.SaveWorkflow(inactive))
	require.NoError(t, ws.SaveWorkflow(active))
	require.NoError(t, mgr.RefreshTestHooks())

	hook, err := mgr.GetWebhookByPath("/shared_path", "POST", true)
	require.NoError(t, err)
	assert.Equal(t, "wf-active", hook.WorkflowID, "the active workflow must win the tie")
}

// TestWebhookManager_TestPath_PrefersRecentlyUpdatedWorkflow breaks the
// remaining ties by update time.
func TestWebhookManager_TestPath_PrefersRecentlyUpdatedWorkflow(t *testing.T) {
	mgr, _, ws := newTestManager()

	older := newWebhookWorkflow("wf-aaa", "shared_path", false)
	newer := newWebhookWorkflow("wf-bbb", "shared_path", false)
	require.NoError(t, ws.SaveWorkflow(older))
	require.NoError(t, ws.SaveWorkflow(newer))
	older.UpdatedAt = time.Now().Add(-time.Hour)
	newer.UpdatedAt = time.Now()

	mgr.SyncWorkflowTestHooks(older)
	mgr.SyncWorkflowTestHooks(newer)

	hook, err := mgr.GetWebhookByPath("/shared_path", "POST", true)
	require.NoError(t, err)
	assert.Equal(t, "wf-bbb", hook.WorkflowID, "the most recently updated workflow must win")
}

// TestWebhookManager_TestPath_BreaksTieByWorkflowID keeps the choice
// deterministic when neither the active flag nor the update time
// separates two candidates.
func TestWebhookManager_TestPath_BreaksTieByWorkflowID(t *testing.T) {
	mgr, _, ws := newTestManager()

	stamp := time.Now()
	low := newWebhookWorkflow("wf-aaa", "shared_path", false)
	high := newWebhookWorkflow("wf-zzz", "shared_path", false)
	require.NoError(t, ws.SaveWorkflow(low))
	require.NoError(t, ws.SaveWorkflow(high))
	low.UpdatedAt = stamp
	high.UpdatedAt = stamp

	mgr.SyncWorkflowTestHooks(low)
	mgr.SyncWorkflowTestHooks(high)

	hook, err := mgr.GetWebhookByPath("/shared_path", "POST", true)
	require.NoError(t, err)
	assert.Equal(t, "wf-zzz", hook.WorkflowID)
}

// TestWebhookManager_SyncWorkflowTestHooks_AfterPathChange covers the
// editor flow: renaming the path of a Webhook node must move the test
// URL instead of leaving the old one behind.
func TestWebhookManager_SyncWorkflowTestHooks_AfterPathChange(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := newWebhookWorkflow("wf-rename", "old_path", false)
	require.NoError(t, ws.SaveWorkflow(wf))
	mgr.SyncWorkflowTestHooks(wf)

	_, err := mgr.GetWebhookByPath("/old_path", "POST", true)
	require.NoError(t, err)

	wf.Nodes[0].Parameters["path"] = "new_path"
	require.NoError(t, ws.UpdateWorkflow(wf.ID, wf))
	mgr.SyncWorkflowTestHooks(wf)

	_, err = mgr.GetWebhookByPath("/old_path", "POST", true)
	assert.Error(t, err, "the old test path must stop resolving after a rename")

	hook, err := mgr.GetWebhookByPath("/new_path", "POST", true)
	require.NoError(t, err)
	assert.Equal(t, "wf-rename", hook.WorkflowID)
}

// TestWebhookManager_SyncWorkflowTestHooks_AfterMethodChange mirrors the
// rename case for the HTTP method.
func TestWebhookManager_SyncWorkflowTestHooks_AfterMethodChange(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := newWebhookWorkflow("wf-method", "method_path", false)
	require.NoError(t, ws.SaveWorkflow(wf))
	mgr.SyncWorkflowTestHooks(wf)

	_, err := mgr.GetWebhookByPath("/method_path", "POST", true)
	require.NoError(t, err)

	wf.Nodes[0].Parameters["httpMethod"] = "GET"
	require.NoError(t, ws.UpdateWorkflow(wf.ID, wf))
	mgr.SyncWorkflowTestHooks(wf)

	_, err = mgr.GetWebhookByPath("/method_path", "POST", true)
	assert.Error(t, err, "the old method must stop resolving")

	hook, err := mgr.GetWebhookByPath("/method_path", "GET", true)
	require.NoError(t, err)
	assert.Equal(t, "wf-method", hook.WorkflowID)
}

// TestWebhookManager_RemoveWorkflowTestHooks covers workflow deletion.
func TestWebhookManager_RemoveWorkflowTestHooks(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := newWebhookWorkflow("wf-deleted", "deleted_path", false)
	require.NoError(t, ws.SaveWorkflow(wf))
	mgr.SyncWorkflowTestHooks(wf)

	_, err := mgr.GetWebhookByPath("/deleted_path", "POST", true)
	require.NoError(t, err)

	// The API deletes the workflow first, then drops its test webhooks.
	require.NoError(t, ws.DeleteWorkflow(wf.ID))
	mgr.RemoveWorkflowTestHooks(wf.ID)

	_, err = mgr.GetWebhookByPath("/deleted_path", "POST", true)
	assert.Error(t, err, "a deleted workflow must stop resolving its test URL")
}

// TestWebhookManager_RefreshTestHooks_IgnoresWorkflowsWithoutWebhookNode
// keeps non-webhook workflows out of the index.
func TestWebhookManager_RefreshTestHooks_IgnoresWorkflowsWithoutWebhookNode(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := &model.Workflow{
		ID:     "wf-no-hook",
		Name:   "no-hook",
		Active: true,
		Nodes: []model.Node{{
			Name:       "Set",
			Type:       "n8n-nodes-base.set",
			Parameters: map[string]interface{}{},
		}},
	}
	require.NoError(t, ws.SaveWorkflow(wf))
	require.NoError(t, mgr.RefreshTestHooks())

	_, err := mgr.GetWebhookByPath("/no-hook", "POST", true)
	assert.Error(t, err)
}

// TestWebhookManager_RefreshTestHooks_DoesNotPersistWebhooks guards the
// webhook storage shape: the test index is derived, never persisted, so
// `/api/v1/webhooks` keeps listing production registrations only.
func TestWebhookManager_RefreshTestHooks_DoesNotPersistWebhooks(t *testing.T) {
	mgr, _, ws := newTestManager()

	require.NoError(t, ws.SaveWorkflow(newWebhookWorkflow("wf-derived", "derived_path", false)))
	require.NoError(t, mgr.RefreshTestHooks())

	all, err := mgr.ListWebhooks("")
	require.NoError(t, err)
	assert.Empty(t, all, "test webhooks must not be persisted")
}

// TestWebhookManager_TestPath_LazyRefreshPicksUpStoreChanges covers
// workflows that are created outside the REST API (e.g. through the MCP
// tools): a lookup miss triggers a rate-limited rebuild of the index.
func TestWebhookManager_TestPath_LazyRefreshPicksUpStoreChanges(t *testing.T) {
	mgr, _, ws := newTestManager()

	require.NoError(t, mgr.RefreshTestHooks())

	// Simulate the index being older than the refresh interval.
	mgr.mu.Lock()
	mgr.testHooksBuiltAt = time.Now().Add(-testHooksRefreshInterval)
	mgr.mu.Unlock()

	require.NoError(t, ws.SaveWorkflow(newWebhookWorkflow("wf-mcp", "mcp_path", false)))

	hook, err := mgr.GetWebhookByPath("/mcp_path", "POST", true)
	require.NoError(t, err, "a stale index must be rebuilt on a lookup miss")
	assert.Equal(t, "wf-mcp", hook.WorkflowID)
}

// ---------------------------------------------------------------------------
// HTTP-level tests
// ---------------------------------------------------------------------------

// TestHandler_TestPath_ExecutesInactiveWorkflow is the end-to-end HTTP
// contract: the test URL runs an inactive workflow, the production URL
// keeps answering 404 until the workflow is activated.
func TestHandler_TestPath_ExecutesInactiveWorkflow(t *testing.T) {
	mgr, _, ws := newTestManager()
	mgr.engine.RegisterNodeExecutor("n8n-nodes-base.webhook", trigger.NewWebhookNode())

	wf := newWebhookWorkflow("wf-http", "webhook_xml", false)
	require.NoError(t, ws.SaveWorkflow(wf))
	require.NoError(t, mgr.RefreshTestHooks())

	router := newTestRouter(mgr)

	// Test URL: works while the flow is still being designed.
	req := httptest.NewRequest(http.MethodPost, "/webhook-test/webhook_xml",
		bytes.NewReader([]byte(`{"hello":"world"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "test URL must run the inactive workflow")
	assert.Contains(t, rec.Body.String(), `"executionMode":"test"`,
		"the trigger output must report the test execution mode")

	// Production URL: still 404 for an inactive workflow.
	req = httptest.NewRequest(http.MethodPost, "/webhook/webhook_xml",
		bytes.NewReader([]byte(`{"hello":"world"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"production URL must not run an inactive workflow")
}

// TestHandler_BothPaths_RunTheSameActiveWorkflow asserts that an active
// workflow answers on both URLs and that the two responses carry the
// same payload — only the execution mode differs.
func TestHandler_BothPaths_RunTheSameActiveWorkflow(t *testing.T) {
	mgr, _, ws := newTestManager()
	mgr.engine.RegisterNodeExecutor("n8n-nodes-base.webhook", trigger.NewWebhookNode())

	wf := newWebhookWorkflow("wf-http-active", "both_paths", true)
	require.NoError(t, ws.SaveWorkflow(wf))
	require.NoError(t, mgr.RegisterWorkflowWebhooks(wf, false))
	require.NoError(t, mgr.RefreshTestHooks())

	router := newTestRouter(mgr)

	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path,
			bytes.NewReader([]byte(`{"hello":"world"}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	testRec := post("/webhook-test/both_paths")
	require.Equal(t, http.StatusOK, testRec.Code)
	assert.Contains(t, testRec.Body.String(), `"executionMode":"test"`)

	prodRec := post("/webhook/both_paths")
	require.Equal(t, http.StatusOK, prodRec.Code)
	assert.Contains(t, prodRec.Body.String(), `"executionMode":"production"`)

	assert.NotEqual(t, testRec.Body.String(), prodRec.Body.String(),
		"the two URLs must differ in execution mode, not in payload")
}

// TestHandler_TestPath_NotFound_KeepsN8nErrorShape keeps the 404 wire
// shape of an unknown test URL unchanged (n8n-compatible JSON envelope).
func TestHandler_TestPath_NotFound_KeepsN8nErrorShape(t *testing.T) {
	mgr, _, _ := newTestManager()
	router := newTestRouter(mgr)

	req := httptest.NewRequest(http.MethodGet, "/webhook-test/missing_path", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":404`)
	assert.Contains(t, rec.Body.String(), "is not registered")
}
