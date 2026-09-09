package webhooks

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
	"github.com/stretchr/testify/assert"
)

// newTestStore returns a real in-memory storage backend so we can
// exercise the full storage.WorkflowStorage interface without
// hand-rolling each method.
func newTestStore(t *testing.T) storage.WorkflowStorage {
	t.Helper()
	return storage.NewMemoryStorage()
}

// seedCredential stores a credential envelope directly in the store,
// bypassing the API layer (which would reject unencrypted data in
// dev mode). Returns the credential id so tests can wire it into
// the node's Credentials map.
func seedCredential(t *testing.T, ws storage.WorkflowStorage, credType, user, pass string) string {
	t.Helper()
	cred := &storage.Credential{
		ID:   "test-" + credType,
		Name: "test " + credType,
		Type: credType,
		Data: map[string]interface{}{"user": user, "password": pass},
	}
	assert.NoError(t, ws.SaveCredential(cred))
	return cred.ID
}

func TestResolveCredentialAuthData_BasicAuth(t *testing.T) {
	ws := newTestStore(t)
	id := seedCredential(t, ws, "httpBasicAuth", "admin", "admin123")

	node := &model.Node{
		Name: "Webhook",
		Credentials: map[string]model.Credential{
			"httpBasicAuth": {ID: id, Name: "basic_credential", Type: "httpBasicAuth"},
		},
		Parameters: map[string]interface{}{"authentication": "basicAuth"},
	}

	got := resolveCredentialAuthData(ws, node)
	assert.NotNil(t, got)
	assert.Equal(t, "admin", got["username"])
	assert.Equal(t, "admin123", got["password"])
}

func TestResolveCredentialAuthData_HeaderAuth(t *testing.T) {
	ws := newTestStore(t)
	cred := &storage.Credential{
		ID: "test-header", Name: "test header", Type: "httpHeaderAuth",
		Data: map[string]interface{}{"name": "x-api-key", "value": "admin123"},
	}
	assert.NoError(t, ws.SaveCredential(cred))

	node := &model.Node{
		Credentials: map[string]model.Credential{
			"httpHeaderAuth": {ID: cred.ID, Name: "test header", Type: "httpHeaderAuth"},
		},
		Parameters: map[string]interface{}{"authentication": "headerAuth"},
	}

	got := resolveCredentialAuthData(ws, node)
	assert.NotNil(t, got)
	assert.Equal(t, "x-api-key", got["headerName"])
	assert.Equal(t, "admin123", got["headerValue"])
}

func TestResolveCredentialAuthData_NoAuth(t *testing.T) {
	ws := newTestStore(t)
	node := &model.Node{
		Parameters: map[string]interface{}{"authentication": "none"},
	}
	got := resolveCredentialAuthData(ws, node)
	assert.Nil(t, got)
}

func TestResolveCredentialAuthData_MissingFromStore(t *testing.T) {
	ws := newTestStore(t)
	node := &model.Node{
		Credentials: map[string]model.Credential{
			"httpBasicAuth": {ID: "ghost", Type: "httpBasicAuth"},
		},
		Parameters: map[string]interface{}{"authentication": "basicAuth"},
	}
	got := resolveCredentialAuthData(ws, node)
	assert.Nil(t, got, "missing credential must NOT abort webhook registration; fall back to legacy 'any well-formed header' check")
}

func TestResolveCredentialAuthData_EmptyCredData(t *testing.T) {
	ws := newTestStore(t)
	cred := &storage.Credential{
		ID: "test-empty", Name: "empty", Type: "httpBasicAuth",
		Data: map[string]interface{}{},
	}
	assert.NoError(t, ws.SaveCredential(cred))

	node := &model.Node{
		Credentials: map[string]model.Credential{
			"httpBasicAuth": {ID: cred.ID, Type: "httpBasicAuth"},
		},
		Parameters: map[string]interface{}{"authentication": "basicAuth"},
	}
	got := resolveCredentialAuthData(ws, node)
	assert.Nil(t, got, "empty credential data must not produce an empty AuthData envelope")
}
