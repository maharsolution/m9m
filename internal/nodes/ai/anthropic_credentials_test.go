package ai

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// Credential envelope: `anthropic_apiKey`
//
// The credential manager flattens the `anthropic` credential into the
// node parameters under `anthropic_apiKey`. The node must use that
// key; the legacy plain `apiKey` parameter stays supported as a
// fallback.
// ---------------------------------------------------------------

func TestAnthropicNode_Execute_UsesCredentialEnvelopeAPIKey(t *testing.T) {
	t.Setenv("M9M_MINIMAX_BASE_URL", "")
	t.Setenv("M9M_ANTHROPIC_BASE_URL", "")

	srv, getReq, _ := fakeAnthropicServer(t)
	defer srv.Close()

	node := newAnthropicNodePointingAt(t, srv)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"anthropic_apiKey": "sk-ant-envelope",
			"prompt":           "hello",
		},
	)
	require.NoError(t, err)

	req := getReq()
	require.NotNil(t, req)
	assert.Equal(t, "sk-ant-envelope", req.Header.Get("x-api-key"),
		"the credential envelope key must reach the request")
	assert.Equal(t, "Bearer sk-ant-envelope", req.Header.Get("Authorization"))
}

func TestAnthropicNode_Execute_UsesLegacyAPIKey(t *testing.T) {
	t.Setenv("M9M_MINIMAX_BASE_URL", "")
	t.Setenv("M9M_ANTHROPIC_BASE_URL", "")

	srv, getReq, _ := fakeAnthropicServer(t)
	defer srv.Close()

	node := newAnthropicNodePointingAt(t, srv)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"apiKey": "sk-ant-legacy",
			"prompt": "hello",
		},
	)
	require.NoError(t, err)

	req := getReq()
	require.NotNil(t, req)
	assert.Equal(t, "sk-ant-legacy", req.Header.Get("x-api-key"),
		"the legacy plain key must keep working")
}

func TestAnthropicNode_Execute_CredentialEnvelopeWinsOverLegacyKey(t *testing.T) {
	t.Setenv("M9M_MINIMAX_BASE_URL", "")
	t.Setenv("M9M_ANTHROPIC_BASE_URL", "")

	srv, getReq, _ := fakeAnthropicServer(t)
	defer srv.Close()

	node := newAnthropicNodePointingAt(t, srv)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"anthropic_apiKey": "sk-ant-envelope",
			"apiKey":           "sk-ant-legacy",
			"prompt":           "hello",
		},
	)
	require.NoError(t, err)

	req := getReq()
	require.NotNil(t, req)
	assert.Equal(t, "sk-ant-envelope", req.Header.Get("x-api-key"),
		"the envelope key must win over the legacy key")
}

func TestAnthropicNode_Execute_EmptyEnvelopeKeyFallsBackToLegacyKey(t *testing.T) {
	t.Setenv("M9M_MINIMAX_BASE_URL", "")
	t.Setenv("M9M_ANTHROPIC_BASE_URL", "")

	srv, getReq, _ := fakeAnthropicServer(t)
	defer srv.Close()

	node := newAnthropicNodePointingAt(t, srv)

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"anthropic_apiKey": "",
			"apiKey":           "sk-ant-legacy",
			"prompt":           "hello",
		},
	)
	require.NoError(t, err)

	req := getReq()
	require.NotNil(t, req)
	assert.Equal(t, "sk-ant-legacy", req.Header.Get("x-api-key"))
}

func TestAnthropicNode_Execute_MissingAPIKeyErrorDoesNotLeakSecret(t *testing.T) {
	node := NewAnthropicNode()

	_, err := node.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{}}},
		map[string]interface{}{
			"anthropic_apiKey": "   ",
			"prompt":           "hello",
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey is required")
	assert.NotContains(t, err.Error(), "sk-ant-",
		"the error message must never contain the credential value")
}
