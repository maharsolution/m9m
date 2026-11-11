package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Credential envelope: `openai_apiKey`
//
// The credential manager flattens the `openai` credential into the
// node parameters under `openai_apiKey`. The node must use that key;
// the legacy plain `apiKey` parameter stays supported as a fallback.
// ---------------------------------------------------------------

// openAICredentialServer starts a stub OpenAI endpoint, wires a node
// to it and returns a getter for the captured Authorization header.
func openAICredentialServer(t *testing.T) (*OpenAINode, func() string) {
	t.Helper()

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message":       map[string]interface{}{"content": "ok"},
					"finish_reason": "stop",
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	node := NewOpenAINode()
	node.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	return node, func() string { return authorization }
}

func TestOpenAINode_Execute_UsesCredentialEnvelopeAPIKey(t *testing.T) {
	node, authorization := openAICredentialServer(t)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"openai_apiKey": "sk-envelope-key",
			"prompt":        "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-envelope-key", authorization(),
		"the credential envelope key must reach the request")
}

func TestOpenAINode_Execute_UsesLegacyAPIKey(t *testing.T) {
	node, authorization := openAICredentialServer(t)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"apiKey": "sk-legacy-key",
			"prompt": "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-legacy-key", authorization(),
		"the legacy plain key must keep working")
}

func TestOpenAINode_Execute_CredentialEnvelopeWinsOverLegacyKey(t *testing.T) {
	node, authorization := openAICredentialServer(t)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"openai_apiKey": "sk-envelope-key",
			"apiKey":        "sk-legacy-key",
			"prompt":        "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-envelope-key", authorization(),
		"the envelope key must win over the legacy key")
}

func TestOpenAINode_Execute_EmptyEnvelopeKeyFallsBackToLegacyKey(t *testing.T) {
	node, authorization := openAICredentialServer(t)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"openai_apiKey": "",
			"apiKey":        "sk-legacy-key",
			"prompt":        "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-legacy-key", authorization(),
		"an empty envelope value must fall through to the legacy key")
}

func TestOpenAINode_Execute_MissingAPIKeyErrorDoesNotLeakSecret(t *testing.T) {
	node := NewOpenAINode()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"openai_apiKey": "   ",
			"prompt":        "hello",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey is required")
	assert.NotContains(t, err.Error(), "sk-",
		"the error message must never contain the credential value")
}
