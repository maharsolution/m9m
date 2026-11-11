package database

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Credential envelope: `redis_host` / `redis_port` / `redis_password`
// / `redis_db` / `redis_tls`
//
// The credential manager flattens the `redis` credential into the node
// parameters under `redis_<field>` — with every value stringified. The
// node must prefer those keys and keep the legacy plain parameters as
// a fallback.
// ---------------------------------------------------------------

func TestResolveRedisConnection_ReadsCredentialEnvelope(t *testing.T) {
	host, port, db, useTLS, username, password := resolveRedisConnection(map[string]interface{}{
		"redis_host":     "redis.internal",
		"redis_port":     "6380",
		"redis_db":       "3",
		"redis_tls":      "true",
		"redis_username": "acl-user",
		"redis_password": "redis-secret",
	})

	assert.Equal(t, "redis.internal", host)
	assert.Equal(t, 6380, port)
	assert.Equal(t, 3, db)
	assert.True(t, useTLS)
	assert.Equal(t, "acl-user", username)
	assert.Equal(t, "redis-secret", password)
}

func TestResolveRedisConnection_ReadsLegacyParameters(t *testing.T) {
	host, port, db, useTLS, username, password := resolveRedisConnection(map[string]interface{}{
		"host":     "legacy.internal",
		"port":     6381,
		"db":       4,
		"tls":      true,
		"username": "legacy-user",
		"password": "legacy-secret",
	})

	assert.Equal(t, "legacy.internal", host)
	assert.Equal(t, 6381, port)
	assert.Equal(t, 4, db)
	assert.True(t, useTLS)
	assert.Equal(t, "legacy-user", username)
	assert.Equal(t, "legacy-secret", password)
}

func TestResolveRedisConnection_CredentialEnvelopeWinsOverLegacy(t *testing.T) {
	host, port, db, useTLS, _, password := resolveRedisConnection(map[string]interface{}{
		"redis_host":     "redis.internal",
		"redis_port":     "6380",
		"redis_db":       "3",
		"redis_tls":      "true",
		"redis_password": "redis-secret",
		"host":           "legacy.internal",
		"port":           6381,
		"db":             4,
		"tls":            false,
		"password":       "legacy-secret",
	})

	assert.Equal(t, "redis.internal", host)
	assert.Equal(t, 6380, port)
	assert.Equal(t, 3, db)
	assert.True(t, useTLS)
	assert.Equal(t, "redis-secret", password)
}

func TestResolveRedisConnection_EmptyEnvelopeValueFallsBackToLegacy(t *testing.T) {
	host, port, _, _, _, password := resolveRedisConnection(map[string]interface{}{
		"redis_host":     "",
		"redis_port":     "",
		"redis_password": "",
		"host":           "legacy.internal",
		"port":           6381,
		"password":       "legacy-secret",
	})

	assert.Equal(t, "legacy.internal", host)
	assert.Equal(t, 6381, port)
	assert.Equal(t, "legacy-secret", password)
}

func TestResolveRedisConnection_DefaultsArePreserved(t *testing.T) {
	host, port, db, useTLS, _, password := resolveRedisConnection(nil)

	assert.Equal(t, "localhost", host)
	assert.Equal(t, 6379, port)
	assert.Equal(t, 0, db)
	assert.False(t, useTLS)
	assert.Empty(t, password)
}

// The HTTP backend must authenticate with the credential password.
func TestRedisNode_Execute_HTTPBackendUsesCredentialEnvelopePassword(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"OK"`))
	}))
	defer server.Close()

	node := NewRedisNodeWithClient(server.Client())

	_, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, map[string]interface{}{
		"mode":           "http",
		"baseUrl":        server.URL,
		"operation":      "set",
		"key":            "mykey",
		"value":          "myvalue",
		"redis_password": "redis-envelope-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer redis-envelope-secret", capturedAuth,
		"the credential envelope password must reach the request")
}

func TestRedisNode_Execute_HTTPBackendUsesLegacyPassword(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"OK"`))
	}))
	defer server.Close()

	node := NewRedisNodeWithClient(server.Client())

	_, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, map[string]interface{}{
		"mode":      "http",
		"baseUrl":   server.URL,
		"operation": "set",
		"key":       "mykey",
		"value":     "myvalue",
		"password":  "legacy-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer legacy-secret", capturedAuth,
		"the legacy plain password must keep working")
}
