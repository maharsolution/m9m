package messaging

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
// Credential envelope: `slack_accessToken` / `slack_webhookUrl`
//
// The credential manager flattens the `slack` credential into the node
// parameters under `slack_<field>`. The node must prefer those keys
// and keep the legacy plain parameters as a fallback.
// ---------------------------------------------------------------

func TestResolveSlackAuth_ReadsCredentialEnvelope(t *testing.T) {
	webhookURL, token := resolveSlackAuth(map[string]interface{}{
		"slack_accessToken": "xoxb-envelope",
		"slack_webhookUrl":  "https://hooks.slack.com/services/envelope",
	})

	assert.Equal(t, "https://hooks.slack.com/services/envelope", webhookURL)
	assert.Equal(t, "xoxb-envelope", token)
}

func TestResolveSlackAuth_ReadsLegacyParameters(t *testing.T) {
	webhookURL, token := resolveSlackAuth(map[string]interface{}{
		"accessToken": "xoxb-legacy",
		"webhookUrl":  "https://hooks.slack.com/services/legacy",
		"token":       "xoxb-legacy-token",
	})

	assert.Equal(t, "https://hooks.slack.com/services/legacy", webhookURL)
	assert.Equal(t, "xoxb-legacy-token", token)
}

func TestResolveSlackAuth_CredentialEnvelopeWinsOverLegacy(t *testing.T) {
	webhookURL, token := resolveSlackAuth(map[string]interface{}{
		"slack_accessToken": "xoxb-envelope",
		"slack_webhookUrl":  "https://hooks.slack.com/services/envelope",
		"token":             "xoxb-legacy",
		"webhookUrl":        "https://hooks.slack.com/services/legacy",
	})

	assert.Equal(t, "https://hooks.slack.com/services/envelope", webhookURL)
	assert.Equal(t, "xoxb-envelope", token)
}

func TestResolveSlackAuth_EmptyEnvelopeValueFallsBackToLegacy(t *testing.T) {
	webhookURL, token := resolveSlackAuth(map[string]interface{}{
		"slack_accessToken": "",
		"token":             "xoxb-legacy",
		"slack_webhookUrl":  "  ",
		"webhookUrl":        "https://hooks.slack.com/services/legacy",
	})

	assert.Equal(t, "https://hooks.slack.com/services/legacy", webhookURL)
	assert.Equal(t, "xoxb-legacy", token)
}

func TestResolveSlackAuth_NilParamsAreSafe(t *testing.T) {
	webhookURL, token := resolveSlackAuth(nil)
	assert.Empty(t, webhookURL)
	assert.Empty(t, token)
}

// The token path must actually authenticate the outgoing request.
func TestSlackNode_Execute_UsesCredentialEnvelopeToken(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	node := NewSlackNode()
	node.httpClient = &http.Client{
		Transport: &rewriteTransport{
			base:    server.Client().Transport,
			baseURL: server.URL,
		},
	}

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"slack_accessToken": "xoxb-envelope-token",
			"channel":           "#general",
			"text":              "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer xoxb-envelope-token", capturedAuth,
		"the credential envelope token must reach the request")
}

func TestSlackNode_Execute_UsesLegacyToken(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	node := NewSlackNode()
	node.httpClient = &http.Client{
		Transport: &rewriteTransport{
			base:    server.Client().Transport,
			baseURL: server.URL,
		},
	}

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"token":   "xoxb-legacy-token",
			"channel": "#general",
			"text":    "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer xoxb-legacy-token", capturedAuth,
		"the legacy plain token must keep working")
}

func TestSlackNode_Execute_UsesCredentialEnvelopeWebhookURL(t *testing.T) {
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	node := NewSlackNode()
	node.httpClient = server.Client()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"slack_webhookUrl": server.URL,
			"text":             "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, requestCount,
		"the credential envelope webhook URL must be used")
}
