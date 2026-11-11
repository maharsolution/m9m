package http

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Credential envelope reads through the shared base helper
// (base.CredentialString).
//
// The HTTP Request node has no inline auth parameter in the n8n shape
// — the credential envelope (`httpBasicAuth_user`, …) is the only
// source of auth material — so no legacy fallback key is needed. What
// matters here is that a present-but-empty envelope value counts as
// absent instead of poisoning the auth scheme.
// ---------------------------------------------------------------

func TestApplyCredentialAuth_EmptyEnvelopeValueCountsAsAbsent(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpBasicAuth_user":     "",
		"httpBasicAuth_password": "",
	}

	require.NoError(t, applyCredentialAuth(req, params),
		"an empty credential must not be treated as attached")
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestApplyCredentialAuth_WhitespaceEnvelopeValueCountsAsAbsent(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpBasicAuth_user":     "   ",
		"httpBasicAuth_password": "   ",
	}

	require.NoError(t, applyCredentialAuth(req, params))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestApplyCredentialAuth_HalfPresentBasicAuthStillFails(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpBasicAuth_user":     "alice",
		"httpBasicAuth_password": "",
	}

	err := applyCredentialAuth(req, params)
	require.Error(t, err, "a half-present credential must fail loudly")
	assert.NotContains(t, err.Error(), "alice",
		"the error message must never contain the credential value")
}

func TestApplyCredentialAuth_NonStringEnvelopeValueIsIgnored(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"apiKey_apiKey": 12345,
	}

	require.NoError(t, applyCredentialAuth(req, params),
		"a non-string credential value must be ignored")
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestApplyCredentialAuth_NilParamsAreSafe(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	require.NoError(t, applyCredentialAuth(req, nil))
	assert.Empty(t, req.Header.Get("Authorization"))
}
