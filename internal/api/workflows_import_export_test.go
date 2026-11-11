package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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

// catalogEngine exposes a node catalogue so the import can flag node
// types m9m does not implement.
type catalogEngine struct {
	MockWorkflowEngine
	types []engine.NodeTypeInfo
}

func (c *catalogEngine) GetRegisteredNodeTypes() []engine.NodeTypeInfo {
	return c.types
}

func setupImportExportServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()
	store := storage.NewMemoryStorage()
	server := NewAPIServer(nil, nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)
	return router, store
}

// n8nWorkflowJSON is a small but realistic n8n workflow document.
const n8nWorkflowJSON = `{
  "id": "n8n-source-id",
  "name": "Daily Report",
  "active": true,
  "nodes": [
    {
      "id": "node-1",
      "name": "Manual Trigger",
      "type": "n8n-nodes-base.manualTrigger",
      "typeVersion": 1,
      "position": [100, 200],
      "parameters": {}
    },
    {
      "id": "node-2",
      "name": "HTTP Request",
      "type": "n8n-nodes-base.httpRequest",
      "typeVersion": 2.1,
      "position": [300, 200],
      "parameters": {
        "url": "={{ $json.url }}",
        "method": "GET"
      },
      "notes": "fetch data",
      "disabled": true,
      "onError": "continueRegularOutput",
      "retryOnFail": true,
      "maxTries": 3,
      "waitBetweenTries": 1000,
      "continueOnFail": true,
      "alwaysOutputData": true,
      "executeOnce": false,
      "webhookId": "hook-1",
      "root": "root-1",
      "credentials": {
        "httpBasicAuth": {
          "id": "cred-1",
          "name": "My Cred"
        }
      }
    }
  ],
  "connections": {
    "Manual Trigger": {
      "main": [
        [
          {
            "node": "HTTP Request",
            "type": "main",
            "index": 0
          }
        ]
      ]
    }
  },
  "settings": {
    "executionOrder": "v1"
  },
  "staticData": {},
  "pinData": {},
  "tags": [
    { "id": "tag-1", "name": "prod" },
    "legacy-tag"
  ],
  "versionId": "old-version-id",
  "meta": {}
}`

func TestExportWorkflow_SingleObject(t *testing.T) {
	router, store := setupImportExportServer(t)

	workflow := &model.Workflow{
		ID:     "wf-1",
		Name:   "Daily Report",
		Active: true,
		Nodes: []model.Node{
			{ID: "n1", Name: "Start", Type: "n8n-nodes-base.manualTrigger", TypeVersion: 1, Position: []int{0, 0}, Parameters: map[string]interface{}{}},
		},
		Connections: map[string]model.Connections{},
		VersionID:   "v-1",
	}
	require.NoError(t, store.SaveWorkflow(workflow))

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-1/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "daily-report.json")

	var exported map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))

	// A single export is an object (n8n editor parity), not an array.
	assert.Equal(t, "Daily Report", exported["name"])
	assert.Equal(t, true, exported["active"])
	assert.Equal(t, "v-1", exported["versionId"])
	assert.Len(t, exported["nodes"], 1)
	assert.Contains(t, exported, "connections")
	assert.Contains(t, exported, "settings")
	assert.Contains(t, exported, "staticData")
	assert.Contains(t, exported, "pinData")
	assert.Contains(t, exported, "meta")
	assert.Contains(t, exported, "tags")
}

func TestExportWorkflow_NotFound(t *testing.T) {
	router, _ := setupImportExportServer(t)

	req := httptest.NewRequest("GET", "/api/v1/workflows/missing/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestExportWorkflow_ContainsAllN8nFields pins the export contract:
// the document carries every field n8n writes.
func TestExportWorkflow_ContainsAllN8nFields(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "wf-fields",
		Name:        "Field check",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-fields/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var exported map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))

	for _, field := range []string{
		"id", "name", "active", "nodes", "connections", "settings",
		"staticData", "pinData", "meta", "tags", "versionId",
	} {
		assert.Contains(t, exported, field)
	}

	// A workflow that never went through an import has no version id
	// of its own, but n8n always writes one.
	assert.NotEmpty(t, exported["versionId"])
}

// TestExportWorkflow_FallbackFileName pins the file-name fallback for
// a workflow whose name carries no usable characters: the workflow id
// takes over.
func TestExportWorkflow_FallbackFileName(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "wf-odd",
		Name:        "###",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-odd/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), `filename="wf-odd.json"`)
}

// TestExportWorkflows_BulkUsesStableFileName pins the bulk file name:
// a bulk export always answers with an array, so it always carries the
// same file name - also for a single id.
func TestExportWorkflows_BulkUsesStableFileName(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "wf-1",
		Name:        "Daily Report",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	req := httptest.NewRequest("GET", "/api/v1/workflows/export?ids=wf-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), `filename="workflows-export.json"`)
}

// TestRoutes_StaticWorkflowPathsAreNotWorkflowIDs pins the route
// order: the static paths win over the "{id}" wildcard.
func TestRoutes_StaticWorkflowPathsAreNotWorkflowIDs(t *testing.T) {
	router, store := setupImportExportServer(t)

	// A workflow whose id collides with the static paths.
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "export",
		Name:        "Trap",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	// GET /workflows/export is the bulk export, not "get workflow export".
	req := httptest.NewRequest("GET", "/api/v1/workflows/export?ids=export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var exported []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
	assert.Equal(t, "Trap", exported[0]["name"])

	// POST /workflows/import is the import endpoint, not "import" as an id.
	req = httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestExportWorkflows_BulkAlwaysReturnsArray(t *testing.T) {
	router, store := setupImportExportServer(t)

	for _, id := range []string{"wf-1", "wf-2"} {
		require.NoError(t, store.SaveWorkflow(&model.Workflow{
			ID:          id,
			Name:        "Workflow " + id,
			Nodes:       []model.Node{},
			Connections: map[string]model.Connections{},
		}))
	}

	// Two ids -> array of two.
	req := httptest.NewRequest("GET", "/api/v1/workflows/export?ids=wf-1,wf-2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var exported []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))
	require.Len(t, exported, 2)

	// One id -> still an array (n8n CLI parity).
	req = httptest.NewRequest("GET", "/api/v1/workflows/export?ids=wf-1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	exported = nil
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
}

func TestExportWorkflows_POSTWithIDs(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "wf-1",
		Name:        "Workflow",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	body := bytes.NewBufferString(`{"ids":["wf-1"]}`)
	req := httptest.NewRequest("POST", "/api/v1/workflows/export", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var exported []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
	assert.Equal(t, "wf-1", exported[0]["id"])
}

func TestExportWorkflows_UnknownIDIsNotFound(t *testing.T) {
	router, _ := setupImportExportServer(t)

	req := httptest.NewRequest("GET", "/api/v1/workflows/export?ids=missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestImportWorkflow_SingleObject(t *testing.T) {
	router, store := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, 1, summary.Imported)
	assert.Equal(t, 0, summary.Failed)
	require.Len(t, summary.Workflows, 1)
	assert.Equal(t, "Daily Report", summary.Workflows[0].Name)
	assert.Equal(t, "n8n-source-id", summary.Workflows[0].SourceID)
	assert.NotEmpty(t, summary.Workflows[0].VersionID)
	assert.NotEqual(t, "old-version-id", summary.Workflows[0].VersionID)

	stored, err := store.GetWorkflow("n8n-source-id")
	require.NoError(t, err)
	assert.Equal(t, "Daily Report", stored.Name)
	// Imported workflows are inactive unless the request asks otherwise.
	assert.False(t, stored.Active)
	assert.Equal(t, []string{"prod", "legacy-tag"}, stored.Tags)
	require.Len(t, stored.Nodes, 2)

	second := stored.Nodes[1]
	assert.Equal(t, "HTTP Request", second.Name)
	assert.Equal(t, 2, second.TypeVersion)
	assert.Equal(t, "={{ $json.url }}", second.Parameters["url"])
	assert.Equal(t, "fetch data", second.Notes)
	require.NotNil(t, second.Disabled)
	assert.True(t, *second.Disabled)
	assert.Equal(t, "continueRegularOutput", second.OnError)
	assert.True(t, second.RetryOnFail)
	assert.Equal(t, 3, second.MaxTries)
	assert.Equal(t, 1000, second.WaitBetweenTries)
	assert.True(t, second.ContinueOnFail)
	assert.True(t, second.AlwaysOutputData)
	assert.Equal(t, "hook-1", second.WebhookID)
	assert.Equal(t, "root-1", second.Root)

	// Connections are keyed by node name, like n8n.
	require.Contains(t, stored.Connections, "Manual Trigger")
	require.Len(t, stored.Connections["Manual Trigger"].Main, 1)
	require.Len(t, stored.Connections["Manual Trigger"].Main[0], 1)
	assert.Equal(t, "HTTP Request", stored.Connections["Manual Trigger"].Main[0][0].Node)
}

func TestImportWorkflow_Array(t *testing.T) {
	router, _ := setupImportExportServer(t)

	payload := `[` + n8nWorkflowJSON + `,` + n8nWorkflowJSON + `]`
	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, 2, summary.Imported)
	assert.Len(t, summary.Workflows, 2)
	// The second entry collides with the first id, so it gets a new one.
	assert.NotEqual(t, summary.Workflows[0].ID, summary.Workflows[1].ID)
}

func TestImportWorkflow_MultipartUpload(t *testing.T) {
	router, store := setupImportExportServer(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "workflow.json")
	require.NoError(t, err)
	_, err = part.Write([]byte(n8nWorkflowJSON))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, 1, summary.Imported)

	_, err = store.GetWorkflow("n8n-source-id")
	assert.NoError(t, err)
}

func TestImportWorkflow_MalformedJSON(t *testing.T) {
	router, _ := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(`{"nodes":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportWorkflow_MissingNodesIsRejected(t *testing.T) {
	router, store := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(`{"name":"No nodes","connections":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, response["message"], "nodes")

	workflows, _, err := store.ListWorkflows(storage.WorkflowFilters{})
	require.NoError(t, err)
	assert.Empty(t, workflows)
}

func TestImportWorkflow_MissingConnectionsIsRejected(t *testing.T) {
	router, _ := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(`{"name":"No connections","nodes":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, response["message"], "connections")
}

func TestImportWorkflow_BatchKeepsGoodEntries(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `[` + n8nWorkflowJSON + `,{"name":"Broken","nodes":[]}]`
	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// One bad entry does not abort the batch.
	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, 1, summary.Imported)
	assert.Equal(t, 1, summary.Failed)
	require.Len(t, summary.Errors, 1)
	assert.Equal(t, "Broken", summary.Errors[0].Name)
	assert.Contains(t, summary.Errors[0].Message, "connections")

	_, err := store.GetWorkflow("n8n-source-id")
	assert.NoError(t, err)
}

func TestImportWorkflow_UnknownNodeTypeIsAWarning(t *testing.T) {
	store := storage.NewMemoryStorage()
	server := NewAPIServer(&catalogEngine{types: []engine.NodeTypeInfo{{TypeID: "n8n-nodes-base.manualTrigger"}}}, nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	require.NotEmpty(t, summary.Workflows[0].Warnings)
	assert.Contains(t, strings.Join(summary.Workflows[0].Warnings, " | "), "httpRequest")
}

func TestImportWorkflow_CredentialsAreNormalized(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "My Cred",
		Type: "httpBasicAuth",
	}))

	payload := `{
      "id": "cred-flow",
      "name": "Credential flow",
      "nodes": [
        {
          "id": "n1",
          "name": "HTTP Request",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "credentials": { "httpBasicAuth": { "id": "cred-1", "name": "My Cred" } }
        },
        {
          "id": "n2",
          "name": "Legacy",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "credentials": { "httpBasicAuth": "My Cred" }
        },
        {
          "id": "n3",
          "name": "Unknown",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "credentials": { "httpBasicAuth": { "id": "gone", "name": "Missing Cred" } }
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("cred-flow")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 3)

	assert.Equal(t, "cred-1", stored.Nodes[0].Credentials["httpBasicAuth"].ID)
	assert.Equal(t, "My Cred", stored.Nodes[0].Credentials["httpBasicAuth"].Name)
	// The legacy string shape resolves by name + type.
	assert.Equal(t, "cred-1", stored.Nodes[1].Credentials["httpBasicAuth"].ID)
	// An unresolvable credential keeps the name and reports a warning.
	assert.Empty(t, stored.Nodes[2].Credentials["httpBasicAuth"].ID)
	assert.Equal(t, "Missing Cred", stored.Nodes[2].Credentials["httpBasicAuth"].Name)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	require.NotEmpty(t, summary.Workflows[0].Warnings)
	assert.Contains(t, summary.Workflows[0].Warnings[0], "Missing Cred")
}

func TestImportWorkflow_ActiveFlagNeedsExplicitRequest(t *testing.T) {
	router, store := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import?active=fromJson", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("n8n-source-id")
	require.NoError(t, err)
	assert.True(t, stored.Active)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, "fromJson", summary.ActiveState)
}

func TestImportWorkflow_CollisionKeepsExistingWorkflow(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:          "n8n-source-id",
		Name:        "Existing workflow",
		Nodes:       []model.Node{},
		Connections: map[string]model.Connections{},
	}))

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	assert.Equal(t, "n8n-source-id", summary.Workflows[0].SourceID)
	assert.NotEqual(t, "n8n-source-id", summary.Workflows[0].ID)

	// The existing workflow is untouched.
	existing, err := store.GetWorkflow("n8n-source-id")
	require.NoError(t, err)
	assert.Equal(t, "Existing workflow", existing.Name)

	// The import landed under a fresh id.
	imported, err := store.GetWorkflow(summary.Workflows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Daily Report", imported.Name)
}

func TestImportWorkflow_NodeWithoutIDGetsOne(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "no-node-id",
      "name": "Node without id",
      "nodes": [
        {
          "name": "Start",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {}
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("no-node-id")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 1)
	assert.NotEmpty(t, stored.Nodes[0].ID)
}

func TestImportExport_RoundTrip(t *testing.T) {
	router, store := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(n8nWorkflowJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	req = httptest.NewRequest("GET", "/api/v1/workflows/n8n-source-id/export", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var exported map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exported))

	stored, err := store.GetWorkflow("n8n-source-id")
	require.NoError(t, err)

	assert.Equal(t, stored.Name, exported["name"])
	assert.Equal(t, stored.Active, exported["active"])
	assert.Len(t, exported["nodes"], len(stored.Nodes))

	exportedNodes := exported["nodes"].([]interface{})
	for i, raw := range exportedNodes {
		node := raw.(map[string]interface{})
		assert.Equal(t, stored.Nodes[i].Name, node["name"])
		assert.Equal(t, stored.Nodes[i].Type, node["type"])
	}

	exportedConnections := exported["connections"].(map[string]interface{})
	manual := exportedConnections["Manual Trigger"].(map[string]interface{})
	main := manual["main"].([]interface{})
	branch := main[0].([]interface{})
	first := branch[0].(map[string]interface{})
	assert.Equal(t, "HTTP Request", first["node"])

	// The exported credentials use the modern n8n shape.
	httpNode := exportedNodes[1].(map[string]interface{})
	credentials := httpNode["credentials"].(map[string]interface{})
	basic := credentials["httpBasicAuth"].(map[string]interface{})
	assert.Equal(t, "My Cred", basic["name"])
	// The credential does not exist in m9m, so the export writes the
	// n8n shape with a null id.
	assert.Nil(t, basic["id"])
	assert.NotEmpty(t, exported["versionId"])
}

// TestImportWorkflow_DropsNodesWithoutType pins the n8n editor rule:
// a node without a type is unusable, so it is dropped and the count
// is reported as a warning.
func TestImportWorkflow_DropsNodesWithoutType(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "no-node-type",
      "name": "Workflow with a broken node",
      "nodes": [
        {
          "id": "good",
          "name": "Start",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {}
        },
        {
          "id": "broken",
          "name": "Broken",
          "typeVersion": 1,
          "position": [10, 10],
          "parameters": {}
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("no-node-type")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 1)
	assert.Equal(t, "Start", stored.Nodes[0].Name)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	require.NotEmpty(t, summary.Workflows[0].Warnings)
	assert.Contains(t, strings.Join(summary.Workflows[0].Warnings, " "), `without a "type"`)
}

// TestImportWorkflow_GeneratesMissingNodeName pins the n8n editor
// rule: a node without a name gets a unique one.
func TestImportWorkflow_GeneratesMissingNodeName(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "no-node-name",
      "name": "Workflow with unnamed nodes",
      "nodes": [
        {
          "id": "a",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {}
        },
        {
          "id": "b",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {}
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("no-node-name")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 2)
	assert.NotEmpty(t, stored.Nodes[0].Name)
	assert.NotEmpty(t, stored.Nodes[1].Name)
	assert.NotEqual(t, stored.Nodes[0].Name, stored.Nodes[1].Name)
}

// TestImportWorkflow_NormalizesInvalidPosition pins the n8n rule:
// a malformed position falls back to [0, 0].
func TestImportWorkflow_NormalizesInvalidPosition(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "bad-position",
      "name": "Workflow with a broken position",
      "nodes": [
        {
          "id": "a",
          "name": "Start",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": "nonsense",
          "parameters": {}
        },
        {
          "id": "b",
          "name": "Second",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [1],
          "parameters": {}
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("bad-position")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 2)
	assert.Equal(t, []int{0, 0}, stored.Nodes[0].Position)
	assert.Equal(t, []int{0, 0}, stored.Nodes[1].Position)
}

// TestImportWorkflow_RegeneratesWebhookID pins the n8n editor rule:
// the webhook id of a webhook node is regenerated and the webhook
// path follows it when it referenced the old id.
func TestImportWorkflow_RegeneratesWebhookID(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "webhook-id",
      "name": "Workflow with a webhook",
      "nodes": [
        {
          "id": "a",
          "name": "Webhook",
          "type": "n8n-nodes-base.webhook",
          "typeVersion": 2,
          "position": [0, 0],
          "webhookId": "old-hook-id",
          "parameters": { "path": "old-hook-id" }
        },
        {
          "id": "b",
          "name": "Webhook 2",
          "type": "n8n-nodes-base.webhook",
          "typeVersion": 2,
          "position": [0, 0],
          "webhookId": "second-hook-id",
          "parameters": { "options": { "path": "second-hook-id" } }
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("webhook-id")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 2)

	assert.NotEqual(t, "old-hook-id", stored.Nodes[0].WebhookID)
	assert.NotEmpty(t, stored.Nodes[0].WebhookID)
	assert.Equal(t, stored.Nodes[0].WebhookID, stored.Nodes[0].Parameters["path"])

	assert.NotEqual(t, "second-hook-id", stored.Nodes[1].WebhookID)
	options, ok := stored.Nodes[1].Parameters["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, stored.Nodes[1].WebhookID, options["path"])
}

// TestImportWorkflow_DropsConnectionsOfUnknownNode pins the n8n
// editor rule: connections are sanitised against the real node names.
func TestImportWorkflow_DropsConnectionsOfUnknownNode(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "unknown-source",
      "name": "Workflow with a dangling connection",
      "nodes": [
        {
          "id": "a",
          "name": "Start",
          "type": "n8n-nodes-base.manualTrigger",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {}
        }
      ],
      "connections": {
        "Ghost": {
          "main": [[{ "node": "Start", "type": "main", "index": 0 }]]
        }
      }
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("unknown-source")
	require.NoError(t, err)
	assert.NotContains(t, stored.Connections, "Ghost")

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	assert.Contains(t, strings.Join(summary.Workflows[0].Warnings, " "), "Ghost")
}

// TestExportImport_ReimportsExportedFile pins the UI requirement: a
// downloaded export file is valid JSON that m9m can re-import.
func TestExportImport_ReimportsExportedFile(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID:     "wf-round-trip",
		Name:   "Round trip",
		Active: false,
		Nodes: []model.Node{
			{ID: "n1", Name: "Start", Type: "n8n-nodes-base.manualTrigger", TypeVersion: 1, Position: []int{10, 20}, Parameters: map[string]interface{}{}},
			{ID: "n2", Name: "HTTP Request", Type: "n8n-nodes-base.httpRequest", TypeVersion: 1, Position: []int{30, 20}, Parameters: map[string]interface{}{"url": "https://example.com"}},
		},
		Connections: map[string]model.Connections{
			"Start": {Main: [][]model.Connection{{{Node: "HTTP Request", Type: "main", Index: 0}}}},
		},
		Tags: []string{"prod"},
	}))

	// Download the workflow.
	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-round-trip/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	exported := w.Body.Bytes()

	// Feed the file back into the import endpoint.
	req = httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewReader(exported))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	assert.Equal(t, "Round trip", summary.Workflows[0].Name)

	imported, err := store.GetWorkflow(summary.Workflows[0].ID)
	require.NoError(t, err)
	require.Len(t, imported.Nodes, 2)
	assert.Equal(t, "n8n-nodes-base.manualTrigger", imported.Nodes[0].Type)
	assert.Equal(t, "n8n-nodes-base.httpRequest", imported.Nodes[1].Type)
	require.Contains(t, imported.Connections, "Start")
	require.Len(t, imported.Connections["Start"].Main, 1)
	require.Len(t, imported.Connections["Start"].Main[0], 1)
	assert.Equal(t, "HTTP Request", imported.Connections["Start"].Main[0][0].Node)
	assert.Equal(t, []string{"prod"}, imported.Tags)
}

// TestImportWorkflow_FailedBatchReportsErrorsAtTopLevel pins the
// failure response: the per-item errors are readable without digging
// into the nested summary object.
func TestImportWorkflow_FailedBatchReportsErrorsAtTopLevel(t *testing.T) {
	router, _ := setupImportExportServer(t)

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(`[{"name":"Broken","nodes":[]}]`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(0), response["imported"])
	assert.Equal(t, float64(1), response["failed"])

	errors, ok := response["errors"].([]interface{})
	require.True(t, ok, "the per-item errors must be readable at the top level")
	require.Len(t, errors, 1)
	entry, ok := errors[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Broken", entry["name"])
	assert.Contains(t, entry["message"], "connections")
}

// TestImportWorkflow_PayloadTooLargeIsRejected pins the payload cap:
// an oversized body is refused with 413 instead of being truncated
// into invalid JSON.
func TestImportWorkflow_PayloadTooLargeIsRejected(t *testing.T) {
	router, _ := setupImportExportServer(t)

	original := maxImportPayloadBytes
	maxImportPayloadBytes = 64
	defer func() { maxImportPayloadBytes = original }()

	payload := `{"name":"Too big","nodes":[],"connections":{},"padding":"` + strings.Repeat("x", 256) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

// TestImportWorkflow_CredentialIDMustMatchType pins the n8n lookup
// rule: a credential id is only reused when the credential type
// matches as well (`findOneBy({ id, type })`).
func TestImportWorkflow_CredentialIDMustMatchType(t *testing.T) {
	router, store := setupImportExportServer(t)

	// cred-1 is an httpHeaderAuth credential, the node asks for an
	// httpBasicAuth one.
	require.NoError(t, store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "Header Cred",
		Type: "httpHeaderAuth",
	}))

	payload := `{
      "id": "cred-type-flow",
      "name": "Credential type flow",
      "nodes": [
        {
          "id": "n1",
          "name": "HTTP Request",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "credentials": { "httpBasicAuth": { "id": "cred-1", "name": "Header Cred" } }
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("cred-type-flow")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 1)
	// The id belongs to another credential type, so it is not reused.
	assert.Empty(t, stored.Nodes[0].Credentials["httpBasicAuth"].ID)
	assert.Equal(t, "Header Cred", stored.Nodes[0].Credentials["httpBasicAuth"].Name)
}

// TestImportWorkflow_AmbiguousCredentialNameIsNotResolved pins n8n's
// "exactly one match wins" rule.
func TestImportWorkflow_AmbiguousCredentialNameIsNotResolved(t *testing.T) {
	router, store := setupImportExportServer(t)

	require.NoError(t, store.SaveCredential(&storage.Credential{ID: "c1", Name: "Twin", Type: "httpBasicAuth"}))
	require.NoError(t, store.SaveCredential(&storage.Credential{ID: "c2", Name: "Twin", Type: "httpBasicAuth"}))

	payload := `{
      "id": "cred-ambiguous",
      "name": "Ambiguous credential",
      "nodes": [
        {
          "id": "n1",
          "name": "HTTP Request",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "credentials": { "httpBasicAuth": "Twin" }
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("cred-ambiguous")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 1)
	assert.Empty(t, stored.Nodes[0].Credentials["httpBasicAuth"].ID)
	assert.Equal(t, "Twin", stored.Nodes[0].Credentials["httpBasicAuth"].Name)
}

// TestImportWorkflow_DisabledNodeCredentialIsNormalized pins the m9m
// rule: every node is normalised the same way, so a disabled node
// does not keep a dangling credential reference.
func TestImportWorkflow_DisabledNodeCredentialIsNormalized(t *testing.T) {
	router, store := setupImportExportServer(t)

	payload := `{
      "id": "cred-disabled",
      "name": "Disabled credential",
      "nodes": [
        {
          "id": "n1",
          "name": "HTTP Request",
          "type": "n8n-nodes-base.httpRequest",
          "typeVersion": 1,
          "position": [0, 0],
          "parameters": {},
          "disabled": true,
          "credentials": { "httpBasicAuth": { "id": "gone", "name": "Missing Cred" } }
        }
      ],
      "connections": {}
    }`

	req := httptest.NewRequest("POST", "/api/v1/workflows/import", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	stored, err := store.GetWorkflow("cred-disabled")
	require.NoError(t, err)
	require.Len(t, stored.Nodes, 1)
	assert.Empty(t, stored.Nodes[0].Credentials["httpBasicAuth"].ID)
	assert.Equal(t, "Missing Cred", stored.Nodes[0].Credentials["httpBasicAuth"].Name)

	var summary WorkflowImportSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)
	assert.Contains(t, strings.Join(summary.Workflows[0].Warnings, " "), "Missing Cred")
}
