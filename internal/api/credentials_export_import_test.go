package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// credentialExportTestKey is the instance key of the test instance. It
// is the same 32 character key n8n uses in its own test fixtures, so
// the known n8n blob below can be decrypted by the import path.
const credentialExportTestKey = "Default_Secure_Key_Change_Me_!!!"

// n8nKnownCredentialBlob is a credential blob that was extracted live
// from an n8n database (the n8n interoperability canary).
const n8nKnownCredentialBlob = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="

// setupCredentialExportServer builds an API server with the credential
// export/import routes and an empty credential store.
func setupCredentialExportServer(t *testing.T) (*mux.Router, *storage.MemoryStorage, *credentials.CredentialManager) {
	t.Helper()
	t.Setenv("N8N_ENCRYPTION_KEY", credentialExportTestKey)
	t.Setenv("N8N_ENCRYPTION_SALT", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("M9M_DATA_DIR", t.TempDir())

	credMgr, err := credentials.NewCredentialManager()
	require.NoError(t, err)

	store := storage.NewMemoryStorage()
	server := NewAPIServer(nil, nil, store)
	server.SetCredentialManager(credMgr)

	router := mux.NewRouter()
	// The static credential routes are registered BEFORE the standard
	// routes - exactly like the production wiring does.
	router.HandleFunc("/api/v1/credentials/export", server.ExportCredentials).Methods("GET", "POST", "OPTIONS")
	router.HandleFunc("/api/v1/credentials/import", server.ImportCredentials).Methods("POST", "OPTIONS")
	server.RegisterRoutes(router)

	return router, store, credMgr
}

// bundleWorkflowJSON is an n8n workflow document that references a
// credential by id and name.
const bundleWorkflowJSON = `{
  "id": "wf-bundle-1",
  "name": "Bundle Import",
  "active": true,
  "nodes": [
    {
      "id": "n1",
      "name": "Webhook",
      "type": "n8n-nodes-base.webhook",
      "typeVersion": 2,
      "position": [0, 0],
      "parameters": {"path": "bundle-test", "httpMethod": "POST"},
      "webhookId": "hook-bundle"
    },
    {
      "id": "n2",
      "name": "Mongo",
      "type": "n8n-nodes-base.mongoDb",
      "typeVersion": 1,
      "position": [0, 0],
      "parameters": {},
      "credentials": {
        "mongoDb": {"id": "cred-src", "name": "MongoDB account"}
      }
    }
  ],
  "connections": {
    "Webhook": {"main": [[{"node": "Mongo", "type": "main", "index": 0}]]}
  },
  "settings": {},
  "staticData": {},
  "pinData": {},
  "meta": {},
  "tags": [],
  "versionId": "v-bundle-1"
}`

// TestExportCredentials_N8NShape asserts that the export endpoint
// answers n8n-shaped credential objects whose data field carries the
// encrypted payload - and never the plaintext secret.
func TestExportCredentials_N8NShape(t *testing.T) {
	router, store, _ := setupCredentialExportServer(t)

	require.NoError(t, store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{
			"database": "billing",
			"password": "super-secret-value",
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/credentials/export", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "credentials-export.json")

	// The plaintext secret must never leave the instance.
	assert.NotContains(t, rec.Body.String(), "super-secret-value")

	var exported []credentials.ExportCredential
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
	assert.Equal(t, "cred-1", exported[0].ID)
	assert.Equal(t, "MongoDB account", exported[0].Name)
	assert.Equal(t, "mongoDb", exported[0].Type)
	require.NotEmpty(t, exported[0].Data)
	assert.True(t, strings.HasPrefix(exported[0].Data, "U2FsdGVkX1"),
		"the exported data must be the n8n Salted__ envelope")

	// The blob really carries the credential data.
	decrypted, err := credentials.DecryptCredentialData(
		credentials.NewN8NCipher(credentialExportTestKey), exported[0].Data)
	require.NoError(t, err)
	assert.Equal(t, "super-secret-value", decrypted["password"])
	assert.Equal(t, "billing", decrypted["database"])
}

// TestExportCredentials_IdFilter asserts that only the requested
// credentials are exported.
func TestExportCredentials_IdFilter(t *testing.T) {
	router, store, _ := setupCredentialExportServer(t)

	require.NoError(t, store.SaveCredential(&storage.Credential{ID: "cred-1", Name: "One", Type: "mongoDb", Data: map[string]interface{}{"a": "1"}}))
	require.NoError(t, store.SaveCredential(&storage.Credential{ID: "cred-2", Name: "Two", Type: "kafka", Data: map[string]interface{}{"b": "2"}}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/credentials/export?ids=cred-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var exported []credentials.ExportCredential
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
	assert.Equal(t, "cred-2", exported[0].ID)
	assert.Equal(t, "kafka", exported[0].Type)
}

// TestImportCredentials_EncryptedBundle imports the n8n array form and
// asserts that the credential is stored with the right name, type and
// data - and that the engine sees it.
func TestImportCredentials_EncryptedBundle(t *testing.T) {
	router, store, credMgr := setupCredentialExportServer(t)

	blob, err := credentials.EncryptCredentialData(
		credentials.NewN8NCipher(credentialExportTestKey),
		map[string]interface{}{"database": "billing", "password": "secret-value"})
	require.NoError(t, err)

	payload := []map[string]interface{}{
		{"id": "cred-imported", "name": "MongoDB account", "type": "mongoDb", "data": blob},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import", strings.NewReader(mustJSON(t, payload)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var summary struct {
		Imported    int                      `json:"imported"`
		Failed      int                      `json:"failed"`
		Credentials []credentialImportResult `json:"credentials"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
	assert.Equal(t, 1, summary.Imported)
	assert.Equal(t, 0, summary.Failed)
	require.Len(t, summary.Credentials, 1)
	assert.Equal(t, "MongoDB account", summary.Credentials[0].Name)
	assert.Equal(t, "mongoDb", summary.Credentials[0].Type)

	// The secret must not be part of the response.
	assert.NotContains(t, rec.Body.String(), "secret-value")

	stored, err := store.GetCredential(summary.Credentials[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "MongoDB account", stored.Name)
	assert.Equal(t, "mongoDb", stored.Type)
	assert.Equal(t, "billing", stored.Data["database"])
	assert.Equal(t, "secret-value", stored.Data["password"])

	// The engine (credential manager) must see the credential.
	fromManager, err := credMgr.GetCredential(summary.Credentials[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "secret-value", fromManager.Data["password"])
}

// TestImportCredentials_N8NFixtureShape imports the credential shape
// n8n documents: a single object and the array form. The payload is a
// blob that was extracted live from an n8n database.
func TestImportCredentials_N8NFixtureShape(t *testing.T) {
	router, store, _ := setupCredentialExportServer(t)

	single := map[string]interface{}{
		"id":   "n8n-cred-1",
		"name": "n8n basic auth",
		"type": "httpBasicAuth",
		"data": n8nKnownCredentialBlob,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import", strings.NewReader(mustJSON(t, single)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	stored, err := store.GetCredential("n8n-cred-1")
	require.NoError(t, err)
	assert.Equal(t, "n8n basic auth", stored.Name)
	assert.Equal(t, "httpBasicAuth", stored.Type)
	assert.Equal(t, "admin", stored.Data["user"])
	assert.Equal(t, "admin123", stored.Data["password"])

	// The array form of the same fixture.
	array := []map[string]interface{}{
		{"id": "n8n-cred-2", "name": "n8n basic auth 2", "type": "httpBasicAuth", "data": n8nKnownCredentialBlob},
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import", strings.NewReader(mustJSON(t, array)))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	stored, err = store.GetCredential("n8n-cred-2")
	require.NoError(t, err)
	assert.Equal(t, "admin", stored.Data["user"])
}

// TestImportCredentials_PlainDataNeedsExplicitFlag makes sure a plain
// JSON payload is never accepted silently - the caller has to ask for
// it explicitly.
func TestImportCredentials_PlainDataNeedsExplicitFlag(t *testing.T) {
	router, store, _ := setupCredentialExportServer(t)

	plain := map[string]interface{}{
		"id":   "plain-cred",
		"name": "Plain",
		"type": "mongoDb",
		"data": map[string]interface{}{"database": "billing"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import", strings.NewReader(mustJSON(t, plain)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "decrypted")

	_, err := store.GetCredential("plain-cred")
	assert.Error(t, err, "the plain credential must not have been stored")

	// With the explicit flag the very same payload is accepted.
	plain["decrypted"] = true
	req = httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import", strings.NewReader(mustJSON(t, plain)))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	stored, err := store.GetCredential("plain-cred")
	require.NoError(t, err)
	assert.Equal(t, "billing", stored.Data["database"])
}

// TestImportWorkflows_BundleResolvesCredentials imports a bundle into
// an empty instance: the workflow is imported and active, the
// credential reference is resolved (no warning) and the credential is
// usable by the node.
func TestImportWorkflows_BundleResolvesCredentials(t *testing.T) {
	router, store, credMgr := setupCredentialExportServer(t)

	blob, err := credentials.EncryptCredentialData(
		credentials.NewN8NCipher(credentialExportTestKey),
		map[string]interface{}{"database": "billing", "password": "secret-value"})
	require.NoError(t, err)

	bundle := map[string]interface{}{
		"workflows":   []json.RawMessage{json.RawMessage(bundleWorkflowJSON)},
		"credentials": []map[string]interface{}{{"id": "cred-src", "name": "MongoDB account", "type": "mongoDb", "data": blob}},
		"activeState": "fromJson",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/import", strings.NewReader(mustJSON(t, bundle)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var summary struct {
		Imported    int                      `json:"imported"`
		ActiveState string                   `json:"activeState"`
		Workflows   []WorkflowImportItem     `json:"workflows"`
		Credentials *credentialImportSummary `json:"credentials"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
	require.Equal(t, 1, summary.Imported)
	assert.Equal(t, "fromJson", summary.ActiveState)
	require.Len(t, summary.Workflows, 1)
	assert.True(t, summary.Workflows[0].Active, "the workflow must be active")
	for _, warning := range summary.Workflows[0].Warnings {
		assert.NotContains(t, warning, "could not be resolved",
			"the credential of the bundle must resolve")
	}

	// The credential of the bundle was imported first.
	require.NotNil(t, summary.Credentials)
	assert.Equal(t, 1, summary.Credentials.Imported)
	importedID := summary.Credentials.Credentials[0].ID

	storedCredential, err := store.GetCredential(importedID)
	require.NoError(t, err)
	assert.Equal(t, "MongoDB account", storedCredential.Name)
	assert.Equal(t, "mongoDb", storedCredential.Type)
	assert.Equal(t, "secret-value", storedCredential.Data["password"])

	// The workflow reference points at the imported credential.
	workflow, err := store.GetWorkflow(summary.Workflows[0].ID)
	require.NoError(t, err)
	for _, node := range workflow.Nodes {
		if node.Name != "Mongo" {
			continue
		}
		ref, ok := node.Credentials["mongoDb"]
		require.True(t, ok, "the node must keep its credential reference")
		assert.Equal(t, importedID, ref.ID, "the credential reference must be resolved")
		assert.Equal(t, "MongoDB account", ref.Name)
	}

	// The credential is usable by the node: the credential manager
	// injects the credential values into the node parameters.
	require.NoError(t, credMgr.ResolveWorkflowCredentials(workflow))
	params, err := credMgr.InjectCredentialsIntoNodeParameters("n2", map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "billing", params["mongoDb_database"])
	assert.Equal(t, "secret-value", params["mongoDb_password"])
}

// TestImportWorkflows_MissingCredentialStillWarns makes sure a
// credential that is neither in the bundle nor in the local store
// still produces the existing warning.
func TestImportWorkflows_MissingCredentialStillWarns(t *testing.T) {
	router, _, _ := setupCredentialExportServer(t)

	bundle := map[string]interface{}{
		"workflows":   []json.RawMessage{json.RawMessage(bundleWorkflowJSON)},
		"activeState": "fromJson",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/import", strings.NewReader(mustJSON(t, bundle)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var summary struct {
		Workflows []WorkflowImportItem `json:"workflows"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
	require.Len(t, summary.Workflows, 1)

	joined := strings.Join(summary.Workflows[0].Warnings, "\n")
	assert.Contains(t, joined, "could not be resolved")
	assert.Contains(t, joined, "MongoDB account")
}

// TestExportWorkflow_IncludeCredentials asserts the bundle mode of the
// workflow export: the workflow document plus the encrypted
// credentials it references.
func TestExportWorkflow_IncludeCredentials(t *testing.T) {
	router, store, _ := setupCredentialExportServer(t)

	require.NoError(t, store.SaveCredential(&storage.Credential{
		ID:   "cred-src",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"database": "billing", "password": "secret-value"},
	}))

	workflow := &model.Workflow{
		ID:     "wf-bundle-1",
		Name:   "Bundle Import",
		Active: true,
		Nodes: []model.Node{
			{
				ID: "n1", Name: "Webhook", Type: "n8n-nodes-base.webhook", TypeVersion: 2,
				Position: []int{0, 0}, Parameters: map[string]interface{}{},
			},
			{
				ID: "n2", Name: "Mongo", Type: "n8n-nodes-base.mongoDb", TypeVersion: 1,
				Position: []int{0, 0}, Parameters: map[string]interface{}{},
				Credentials: map[string]model.Credential{
					"mongoDb": {ID: "cred-src", Name: "MongoDB account", Type: "mongoDb"},
				},
			},
		},
		Connections: map[string]model.Connections{},
		VersionID:   "v-bundle-1",
	}
	require.NoError(t, store.SaveWorkflow(workflow))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/wf-bundle-1/export?includeCredentials=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "secret-value")

	var bundle workflowCredentialBundle
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &bundle))
	require.Len(t, bundle.Workflows, 1)
	require.Len(t, bundle.Credentials, 1)
	assert.Equal(t, "cred-src", bundle.Credentials[0].ID)
	assert.Equal(t, "MongoDB account", bundle.Credentials[0].Name)
	assert.Equal(t, "mongoDb", bundle.Credentials[0].Type)
	assert.True(t, strings.HasPrefix(bundle.Credentials[0].Data, "U2FsdGVkX1"))
}

// mustJSON encodes a value as JSON.
func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
