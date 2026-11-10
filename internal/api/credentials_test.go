package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/storage"
)

func newTestServer(t *testing.T) *APIServer {
	t.Helper()
	store := storage.NewMemoryStorage()
	return &APIServer{
		storage: store,
		config:  DefaultAPIServerConfig(),
	}
}

// withVars attaches mux path variables to the request so handlers that
// call mux.Vars can find them. We rebuild the request rather than
// touching r in-place to keep the function side-effect free.
func withVars(req *http.Request, vars map[string]string) *http.Request {
	return mux.SetURLVars(req, vars)
}

func doJSON(t *testing.T, srv *APIServer, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	switch {
	case method == http.MethodGet && path == "/api/v1/credentials":
		srv.ListCredentials(w, req)
	case method == http.MethodPost && path == "/api/v1/credentials":
		srv.CreateCredential(w, req)
	case method == http.MethodGet && path == "/api/v1/credentials/types":
		srv.ListCredentialTypes(w, req)
	case method == http.MethodPost && path == "/api/v1/credentials/test":
		srv.TestCredentialPayload(w, req)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/credentials/schema/"):
		credType := strings.TrimPrefix(path, "/api/v1/credentials/schema/")
		srv.GetCredentialSchema(w, withVars(req, map[string]string{"type": credType}))
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/credentials/") && !strings.Contains(path, "/test"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.GetCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPost && strings.HasSuffix(path, "/test"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		id = strings.TrimSuffix(id, "/test")
		srv.TestCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPut && strings.HasSuffix(path, "/transfer"):
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		id = strings.TrimSuffix(id, "/transfer")
		srv.TransferCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPatch:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.PatchCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodPut:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.UpdateCredential(w, withVars(req, map[string]string{"id": id}))
	case method == http.MethodDelete:
		id := strings.TrimPrefix(path, "/api/v1/credentials/")
		srv.DeleteCredential(w, withVars(req, map[string]string{"id": id}))
	}
	return w
}

func TestCreateAndGetCredential(t *testing.T) {
	srv := newTestServer(t)

	createBody := map[string]any{
		"name": "mybasic",
		"type": "httpBasicAuth",
		"data": map[string]any{"user": "admin", "password": "secret"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", w.Code, w.Body.String())
	}

	var created CredentialResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" || created.Name != "mybasic" || created.Type != "httpBasicAuth" {
		t.Fatalf("unexpected envelope: %+v", created)
	}

	// GET of a single credential returns the stored `data` so the UI's
	// edit form can be pre-filled with the real values (n8n editor
	// parity). The LIST endpoint stays data-free.
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials/"+created.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: status %d", w.Code)
	}
	var fetched struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched: %v", err)
	}
	if fetched.Data["user"] != "admin" || fetched.Data["password"] != "secret" {
		t.Fatalf("single GET must expose data fields, got %+v", fetched.Data)
	}

	// The list envelope must NOT carry the secrets.
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials", nil)
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("data leaked in LIST response: %s", w.Body.String())
	}
}

// TestCredentialPatchMergesDataFields guards the "data fields are
// honoured" contract: a PATCH that only carries the fields the user
// touched must not wipe the fields they left alone.
func TestCredentialPatchMergesDataFields(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "pg",
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"database": "prod",
			"user":     "admin",
			"password": "secret",
		},
	})
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Patch only the password.
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+created.ID,
		map[string]any{"data": map[string]any{"password": "new-secret"}})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status %d body %s", w.Code, w.Body.String())
	}
	var patched struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.Data["password"] != "new-secret" {
		t.Fatalf("password not updated: %+v", patched.Data)
	}
	if patched.Data["user"] != "admin" || patched.Data["host"] != "db.example.com" ||
		patched.Data["database"] != "prod" {
		t.Fatalf("untouched fields were wiped: %+v", patched.Data)
	}

	// An empty data map must not erase the stored secrets either.
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+created.ID,
		map[string]any{"name": "pg-renamed", "data": map[string]any{}})
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.Data["password"] != "new-secret" || patched.Data["user"] != "admin" {
		t.Fatalf("empty data patch wiped the credential: %+v", patched.Data)
	}
}

// TestCredentialTestPayloadEndpoint validates an unsaved payload —
// the UI's "Test connection" button must check the values currently in
// the form, not the ones still in storage.
func TestCredentialTestPayloadEndpoint(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials/test", map[string]any{
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"database": "prod",
			"user":     "admin",
			"password": "secret",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("test payload: status %d body %s", w.Code, w.Body.String())
	}
	var result testCredentialResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "OK" {
		t.Fatalf("expected OK, got %+v", result)
	}

	// Missing required field -> Error.
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/test", map[string]any{
		"type": "postgres",
		"data": map[string]any{"host": "db.example.com", "database": "prod"},
	})
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" {
		t.Fatalf("expected Error for missing user/password, got %+v", result)
	}
}

// TestListCredentialTypes makes sure the UI's type chooser is driven
// by the backend — mongoDb and kafka were missing from the hardcoded
// frontend list, so those credentials could not be created at all.
func TestListCredentialTypes(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodGet, "/api/v1/credentials/types", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("types: status %d body %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []credentialTypeSummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, t := range resp.Data {
		seen[t.Type] = true
	}
	for _, want := range []string{"mongoDb", "kafka", "postgres", "mysql", "apiKey"} {
		if !seen[want] {
			t.Fatalf("credential type %q missing from types list: %+v", want, resp.Data)
		}
	}
}

func TestListCredentialsEnvelope(t *testing.T) {
	srv := newTestServer(t)

	createBody := map[string]any{
		"name": "cred-a",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-1"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d", w.Code)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status %d", w.Code)
	}

	var resp struct {
		Data       []CredentialResponse `json:"data"`
		NextCursor any                  `json:"nextCursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "cred-a" {
		t.Fatalf("expected 1 credential named cred-a, got: %+v", resp.Data)
	}
}

func TestCredentialSchema(t *testing.T) {
	srv := newTestServer(t)

	w := doJSON(t, srv, http.MethodGet, "/api/v1/credentials/schema/httpBasicAuth", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("schema: status %d, body %s", w.Code, w.Body.String())
	}
	var schema CredentialTypeSchema
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema.Type != "httpBasicAuth" {
		t.Fatalf("schema type: got %s", schema.Type)
	}
	wantRequired := map[string]bool{"user": true, "password": true}
	for _, r := range schema.Required {
		delete(wantRequired, r)
	}
	if len(wantRequired) > 0 {
		t.Fatalf("missing required: %+v, full schema: %+v", wantRequired, schema)
	}

	// Unknown type -> 404
	w = doJSON(t, srv, http.MethodGet, "/api/v1/credentials/schema/thisTypeDoesNotExist", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown type: status %d, want 404", w.Code)
	}
}

func TestCredentialTestEndpoint(t *testing.T) {
	srv := newTestServer(t)

	createBody := map[string]any{
		"name": "pg-valid",
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"database": "prod",
			"user":     "admin",
			"password": "secret",
		},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d", w.Code)
	}
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/"+created.ID+"/test", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("test: status %d", w.Code)
	}
	var result testCredentialResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "OK" {
		t.Fatalf("expected OK, got %+v", result)
	}

	createBody2 := map[string]any{
		"name": "pg-invalid",
		"type": "postgres",
		"data": map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"database": "prod",
			"password": "secret",
		},
	}
	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody2)
	if w.Code != http.StatusCreated {
		t.Fatalf("create2: status %d", w.Code)
	}
	var created2 CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created2)

	w = doJSON(t, srv, http.MethodPost, "/api/v1/credentials/"+created2.ID+"/test", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.Status != "Error" {
		t.Fatalf("expected Error for missing user, got %+v", result)
	}
}

func TestCredentialTransferEndpoint(t *testing.T) {
	srv := newTestServer(t)
	createBody := map[string]any{
		"name": "x",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	var c CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &c)

	w = doJSON(t, srv, http.MethodPut, "/api/v1/credentials/"+c.ID+"/transfer",
		map[string]any{"destinationProjectId": "p-2"})
	if w.Code != http.StatusOK {
		t.Fatalf("transfer: status %d body %s", w.Code, w.Body.String())
	}
	var resp CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID != c.ID {
		t.Fatalf("transfer returned different id: %s vs %s", resp.ID, c.ID)
	}
}

func TestCredentialPatchPartial(t *testing.T) {
	srv := newTestServer(t)
	createBody := map[string]any{
		"name": "old",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k1"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	var c CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &c)

	w = doJSON(t, srv, http.MethodPatch, "/api/v1/credentials/"+c.ID,
		map[string]any{"name": "renamed"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status %d body %s", w.Code, w.Body.String())
	}
	var updated CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Name != "renamed" {
		t.Fatalf("expected renamed, got %s", updated.Name)
	}
	if updated.Type != "apiKey" {
		t.Fatalf("type changed unexpectedly: %s", updated.Type)
	}
}

// TestCredentialWriteThroughToEngine verifies the fix for the
// user-reported bug: credentials created/updated/deleted via the
// UI must be visible to the workflow engine immediately, without
// requiring a server restart or a manual LoadFromStorage refresh.
// We stand up a tiny CredentialManager (the same one the engine
// reads), wire it through SetCredentialManager, and assert that
// UpsertCredential / RemoveCredential are invoked by the handlers.
func TestCredentialWriteThroughToEngine(t *testing.T) {
	// The credential store requires M9M_DEV_MODE=true (or a
	// real N8N_ENCRYPTION_KEY in the environment) to spin up
	// without prod-mode encryption-key gating. Tests run in dev
	// mode so we set it explicitly here and clean it up.
	t.Setenv("M9M_DEV_MODE", "true")

	srv := newTestServer(t)
	cm, err := credentials.NewCredentialManager()
	if err != nil {
		t.Fatalf("NewCredentialManager: %v", err)
	}
	srv.SetCredentialManager(cm)

	// Create — manager should now have the credential.
	createBody := map[string]any{
		"name": "write-thru",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-create"},
	}
	w := doJSON(t, srv, http.MethodPost, "/api/v1/credentials", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d body %s", w.Code, w.Body.String())
	}
	var created CredentialResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if _, err := cm.GetCredential(created.ID); err != nil {
		t.Fatalf("credential not in manager after Create: %v", err)
	}

	// Update — manager's data must reflect the new value.
	updateBody := map[string]any{
		"name": "write-thru",
		"type": "apiKey",
		"data": map[string]any{"apiKey": "k-updated"},
	}
	w = doJSON(t, srv, http.MethodPut, "/api/v1/credentials/"+created.ID, updateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("update: status %d body %s", w.Code, w.Body.String())
	}
	c, err := cm.GetCredential(created.ID)
	if err != nil {
		t.Fatalf("GetCredential after update: %v", err)
	}
	if got := c.Data["apiKey"]; got != "k-updated" {
		t.Fatalf("expected apiKey=k-updated, got %v", got)
	}

	// Delete — manager must drop it.
	w = doJSON(t, srv, http.MethodDelete, "/api/v1/credentials/"+created.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d body %s", w.Code, w.Body.String())
	}
	if _, err := cm.GetCredential(created.ID); err == nil {
		t.Fatalf("credential still in manager after Delete")
	}
}
