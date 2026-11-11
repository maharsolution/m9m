package credentials

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/storage"
)

// newAccessTokenTestManager builds a credential manager whose salt and
// data directory live in a temp folder so the test never touches the
// developer's real credential store.
func newAccessTokenTestManager(t *testing.T) *CredentialManager {
	t.Helper()

	t.Setenv("N8N_ENCRYPTION_SALT", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("N8N_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("M9M_DATA_DIR", t.TempDir())

	cm, err := NewCredentialManager()
	require.NoError(t, err)
	return cm
}

// TestAccessTokenForType_ReadsOAuthTokenData asserts that the token is
// taken from the n8n `oauthTokenData.access_token` envelope.
func TestAccessTokenForType_ReadsOAuthTokenData(t *testing.T) {
	cm := newAccessTokenTestManager(t)
	cm.UpsertCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "Sheets",
		Type: "googleSheetsOAuth2Api",
		Data: map[string]interface{}{
			"oauthTokenData": map[string]interface{}{
				"access_token": "ya29.token",
			},
		},
	})

	token, err := cm.AccessTokenForType("googleSheetsOAuth2Api")
	require.NoError(t, err)
	assert.Equal(t, "ya29.token", token)
}

// TestAccessTokenForType_ReadsEncodedOAuthTokenData asserts that the
// JSON-encoded envelope (the shape the credential store persists) is
// understood as well.
func TestAccessTokenForType_ReadsEncodedOAuthTokenData(t *testing.T) {
	cm := newAccessTokenTestManager(t)
	cm.UpsertCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "Sheets",
		Type: "googleSheetsOAuth2Api",
		Data: map[string]interface{}{
			"oauthTokenData": `{"access_token":"ya29.encoded"}`,
		},
	})

	token, err := cm.AccessTokenForType("googleSheetsOAuth2Api")
	require.NoError(t, err)
	assert.Equal(t, "ya29.encoded", token)
}

// TestAccessTokenForType_ReadsFlatField asserts the flat `accessToken`
// fallback.
func TestAccessTokenForType_ReadsFlatField(t *testing.T) {
	cm := newAccessTokenTestManager(t)
	cm.UpsertCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "Sheets",
		Type: "googleSheetsOAuth2Api",
		Data: map[string]interface{}{"accessToken": "ya29.flat"},
	})

	token, err := cm.AccessTokenForType("googleSheetsOAuth2Api")
	require.NoError(t, err)
	assert.Equal(t, "ya29.flat", token)
}

// TestAccessTokenForType_ResolvesEnvironmentPlaceholder asserts that a
// token stored as an environment reference is resolved before it is
// handed out.
func TestAccessTokenForType_ResolvesEnvironmentPlaceholder(t *testing.T) {
	cm := newAccessTokenTestManager(t)
	t.Setenv("M9M_TEST_GOOGLE_TOKEN", "ya29.env")

	cm.UpsertCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "Sheets",
		Type: "googleSheetsOAuth2Api",
		Data: map[string]interface{}{"accessToken": "${M9M_TEST_GOOGLE_TOKEN}"},
	})

	token, err := cm.AccessTokenForType("googleSheetsOAuth2Api")
	require.NoError(t, err)
	assert.Equal(t, "ya29.env", token)
}

// TestAccessTokenForType_UnknownType asserts that a missing credential
// is an error, not an empty token.
func TestAccessTokenForType_UnknownType(t *testing.T) {
	cm := newAccessTokenTestManager(t)

	_, err := cm.AccessTokenForType("googleSheetsOAuth2Api")
	require.Error(t, err)

	_, err = cm.AccessTokenForType("")
	require.Error(t, err)
}
