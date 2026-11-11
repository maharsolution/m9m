package webhooks

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The JWT auth path has its own n8n wire shape:
//
//	missing bearer token   → 401 "No token provided"
//	invalid bearer token   → 403 "invalid token"
//	expired bearer token   → 403 "jwt expired"
//	not-yet-valid token    → 403 "jwt not yet valid"
//
// Verified live against http://187.77.113.218:5678/webhook/token_auth on
// 2026-11-11.

// newJWTRouter registers a webhook that expects an HS256 bearer token
// and returns the router that serves it.
func newJWTRouter(t *testing.T, secret string) *mux.Router {
	t.Helper()
	mgr, _, _ := newTestManager()
	wh := &Webhook{
		ID:         "wh-jwt",
		WorkflowID: "wf-1",
		Path:       "/jwt",
		Method:     "POST",
		Active:     true,
		AuthType:   "jwtAuth",
		AuthData: map[string]interface{}{
			"algorithm": "HS256",
			"secret":    secret,
		},
	}
	require.NoError(t, mgr.RegisterWebhook(wh))
	return newHandlerRouter(NewHandler(mgr))
}

func TestHandler_JWT_MissingToken_MatchesN8n(t *testing.T) {
	const secret = "this-is-a-32-char-shared-secret!"
	router := newJWTRouter(t, secret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/jwt",
		bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Type"))
	assert.Equal(t, "No token provided", rec.Body.String())
}

func TestHandler_JWT_InvalidToken_MatchesN8n(t *testing.T) {
	const secret = "this-is-a-32-char-shared-secret!"
	router := newJWTRouter(t, secret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/jwt",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer invalid.token.value")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Type"))
	assert.Equal(t, "invalid token", rec.Body.String())
}

func TestHandler_JWT_ExpiredToken_MatchesN8n(t *testing.T) {
	const secret = "this-is-a-32-char-shared-secret!"
	router := newJWTRouter(t, secret)

	token := makeHS256(t, map[string]interface{}{
		"sub": "alice",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, secret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/jwt",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "jwt expired", rec.Body.String())
}

func TestHandler_JWT_ValidToken_IsAccepted(t *testing.T) {
	const secret = "this-is-a-32-char-shared-secret!"
	router := newJWTRouter(t, secret)

	token := makeHS256(t, map[string]interface{}{
		"sub": "alice",
		"iat": time.Now().Unix(),
	}, secret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/jwt",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// The webhook points at a workflow that does not exist, so the
	// request fails AFTER authentication. The point of the test is
	// that it is not rejected with 401/403.
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
	assert.NotEqual(t, http.StatusForbidden, rec.Code)
}
