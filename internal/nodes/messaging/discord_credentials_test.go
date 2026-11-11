package messaging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Credential envelope: `discord_webhookUrl` / `discord_botToken`
//
// The credential manager flattens the `discord` credential into the node
// parameters under `discord_<field>`. The node must prefer those keys
// and keep the legacy plain parameters as a fallback.
// ---------------------------------------------------------------

func TestResolveDiscordAuth_ReadsCredentialEnvelope(t *testing.T) {
	webhookURL, botToken := resolveDiscordAuth(map[string]interface{}{
		"discord_webhookUrl": "https://discord.com/api/webhooks/envelope",
		"discord_botToken":   "discord-envelope-token",
	})

	assert.Equal(t, "https://discord.com/api/webhooks/envelope", webhookURL)
	assert.Equal(t, "discord-envelope-token", botToken)
}

func TestResolveDiscordAuth_ReadsLegacyParameters(t *testing.T) {
	webhookURL, botToken := resolveDiscordAuth(map[string]interface{}{
		"webhookUrl": "https://discord.com/api/webhooks/legacy",
		"botToken":   "discord-legacy-token",
	})

	assert.Equal(t, "https://discord.com/api/webhooks/legacy", webhookURL)
	assert.Equal(t, "discord-legacy-token", botToken)
}

func TestResolveDiscordAuth_CredentialEnvelopeWinsOverLegacy(t *testing.T) {
	webhookURL, botToken := resolveDiscordAuth(map[string]interface{}{
		"discord_webhookUrl": "https://discord.com/api/webhooks/envelope",
		"discord_botToken":   "discord-envelope-token",
		"webhookUrl":         "https://discord.com/api/webhooks/legacy",
		"botToken":           "discord-legacy-token",
	})

	assert.Equal(t, "https://discord.com/api/webhooks/envelope", webhookURL)
	assert.Equal(t, "discord-envelope-token", botToken)
}

func TestResolveDiscordAuth_EmptyEnvelopeValueFallsBackToLegacy(t *testing.T) {
	webhookURL, botToken := resolveDiscordAuth(map[string]interface{}{
		"discord_webhookUrl": "",
		"webhookUrl":         "https://discord.com/api/webhooks/legacy",
		"discord_botToken":   "   ",
		"botToken":           "discord-legacy-token",
	})

	assert.Equal(t, "https://discord.com/api/webhooks/legacy", webhookURL)
	assert.Equal(t, "discord-legacy-token", botToken)
}

func TestResolveDiscordAuth_NilParamsAreSafe(t *testing.T) {
	webhookURL, botToken := resolveDiscordAuth(nil)
	assert.Empty(t, webhookURL)
	assert.Empty(t, botToken)
}

func TestDiscordNode_Execute_UsesCredentialEnvelopeWebhookURL(t *testing.T) {
	var capturedAuth string
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	node := NewDiscordNode()
	node.httpClient = server.Client()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"discord_webhookUrl": server.URL,
			"discord_botToken":   "discord-envelope-token",
			"content":            "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, requestCount,
		"the credential envelope webhook URL must be used")
	assert.Equal(t, "Bot discord-envelope-token", capturedAuth,
		"the credential envelope bot token must reach the request")
}

func TestDiscordNode_Execute_UsesLegacyWebhookURL(t *testing.T) {
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	node := NewDiscordNode()
	node.httpClient = server.Client()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"webhookUrl": server.URL,
			"content":    "hello",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, requestCount,
		"the legacy plain webhook URL must keep working")
}

func TestDiscordNode_Execute_RequiresWebhookURL(t *testing.T) {
	node := NewDiscordNode()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"discord_botToken": "discord-envelope-token",
			"content":          "hello",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhookUrl is required")
	assert.NotContains(t, err.Error(), "discord-envelope-token",
		"the error message must never contain the credential value")
}
