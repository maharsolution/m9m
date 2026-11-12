package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/auth"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/storage"
)

// authTestSecret is the JWT secret of the test instances. It is a
// throwaway value that only exists inside the test process.
const authTestSecret = "m9m-test-jwt-secret-value-at-least-32-chars"

// authTestPassword is the password of the test accounts. It never
// leaves the test process.
const authTestPassword = "Str0ng!TestPassw0rd"

// authTestHarness is one wired API server with the auth stack on.
type authTestHarness struct {
	router  *mux.Router
	server  *APIServer
	manager *auth.AuthManager
	store   *storage.MemoryStorage
	audit   *bytes.Buffer
}

// newAuthTestHarness builds an API server with the full auth stack:
// the storage-backed user store, the auth manager, the auth handler
// and the authentication middleware on the /api/v1 subrouter.
func newAuthTestHarness(t *testing.T, staticKeys ...string) *authTestHarness {
	t.Helper()

	// The credential export/import paths need an instance encryption
	// key - the same one the parity fixtures use.
	t.Setenv("N8N_ENCRYPTION_KEY", credentialExportTestKey)
	t.Setenv("N8N_ENCRYPTION_SALT", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("M9M_DATA_DIR", t.TempDir())

	store := storage.NewMemoryStorage()
	userStorage := auth.NewPersistentUserStorage(store)
	manager := auth.NewAuthManager(userStorage, authTestSecret, time.Hour)
	handler := auth.NewAuthHandler(manager)

	server := NewAPIServer(nil, nil, store)
	audit := &bytes.Buffer{}
	server.SetAuditWriter(audit)
	server.SetAuth(manager, handler,
		AuthMiddlewareWithOptions(manager, AuthOptions{StaticAPIKeys: staticKeys}))

	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return &authTestHarness{
		router:  router,
		server:  server,
		manager: manager,
		store:   store,
		audit:   audit,
	}
}

// createUser stores a user with the given role and returns it.
func (h *authTestHarness) createUser(t *testing.T, id, email, role string) *auth.User {
	t.Helper()

	hasher := auth.NewPasswordHasher()
	hashed, err := hasher.HashPassword(authTestPassword)
	require.NoError(t, err)

	user := &auth.User{
		ID:        id,
		Email:     email,
		Password:  hashed,
		Role:      role,
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, auth.NewPersistentUserStorage(h.store).SaveUser(user))
	return user
}

// login returns the session token of a user.
func (h *authTestHarness) login(t *testing.T, email string) string {
	t.Helper()

	token, _, err := h.manager.Login(&auth.UserCredentials{
		Email:    email,
		Password: authTestPassword,
	})
	require.NoError(t, err)
	return token
}

// do performs a request against the router.
func (h *authTestHarness) do(method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// TestAuthMiddleware_NoCredentialIs401 asserts that an anonymous API
// call is rejected.
func TestAuthMiddleware_NoCredentialIs401(t *testing.T) {
	h := newAuthTestHarness(t)

	rec := h.do(http.MethodGet, "/api/v1/credentials", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_WrongKeyIs401 asserts that a bogus API key is
// rejected.
func TestAuthMiddleware_WrongKeyIs401(t *testing.T) {
	h := newAuthTestHarness(t, "static-key-value-1234567890")

	rec := h.do(http.MethodGet, "/api/v1/credentials", map[string]string{
		"X-API-Key": "totally-wrong-key-value",
	})

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_StaticKeyIs200 asserts that the configured
// automation key opens the API.
func TestAuthMiddleware_StaticKeyIs200(t *testing.T) {
	h := newAuthTestHarness(t, "static-key-value-1234567890")

	rec := h.do(http.MethodGet, "/api/v1/credentials", map[string]string{
		"X-API-Key": "static-key-value-1234567890",
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAuthMiddleware_StoredKeyIs200 asserts that a key created through
// the API authenticates as well.
func TestAuthMiddleware_StoredKeyIs200(t *testing.T) {
	h := newAuthTestHarness(t)
	user := h.createUser(t, "user-1", "owner@example.com", string(auth.RoleAdmin))

	_, plainKey, err := h.manager.CreateAPIKeyWithPermissions(user.ID, "automation", nil, nil)
	require.NoError(t, err)

	rec := h.do(http.MethodGet, "/api/v1/credentials", map[string]string{
		"X-API-Key": plainKey,
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAuthMiddleware_JWTSessionIs200 asserts that a session token opens
// the API.
func TestAuthMiddleware_JWTSessionIs200(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-1", "owner@example.com", string(auth.RoleAdmin))
	token := h.login(t, "owner@example.com")

	rec := h.do(http.MethodGet, "/api/v1/credentials", map[string]string{
		"Authorization": "Bearer " + token,
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAuthMiddleware_PublicPathsBypass asserts that the liveness
// probes and the login endpoint stay reachable without a credential.
func TestAuthMiddleware_PublicPathsBypass(t *testing.T) {
	h := newAuthTestHarness(t)

	for _, path := range []string{"/health", "/healthz"} {
		rec := h.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}

	// The readiness probe answers 503 while the instance has no queue -
	// what matters here is that it is REACHED (and not rejected with
	// 401 by the auth middleware).
	rec := h.do(http.MethodGet, "/ready", nil)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)

	// The login endpoint is public - it answers 401 for a wrong
	// password, but it is reached (it is not a 401 "no credential"
	// from the middleware).
	rec = h.do(http.MethodPost, "/api/v1/auth/login", nil)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_QueryParamKeyIsRejected asserts that a key in the
// query string does not authenticate.
func TestAuthMiddleware_QueryParamKeyIsRejected(t *testing.T) {
	h := newAuthTestHarness(t, "static-key-value-1234567890")

	rec := h.do(http.MethodGet, "/api/v1/credentials?apiKey=static-key-value-1234567890", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_RoleMatrix asserts the role/permission matrix of
// the API middleware.
func TestAuthMiddleware_RoleMatrix(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-admin", "admin@example.com", string(auth.RoleAdmin))
	h.createUser(t, "user-member", "member@example.com", string(auth.RoleMember))
	h.createUser(t, "user-viewer", "viewer@example.com", string(auth.RoleViewer))

	adminToken := h.login(t, "admin@example.com")
	memberToken := h.login(t, "member@example.com")
	viewerToken := h.login(t, "viewer@example.com")

	// Every authenticated role reaches the read-only endpoint.
	for name, token := range map[string]string{
		"admin":  adminToken,
		"member": memberToken,
		"viewer": viewerToken,
	} {
		rec := h.do(http.MethodGet, "/api/v1/credentials", map[string]string{
			"Authorization": "Bearer " + token,
		})
		assert.Equal(t, http.StatusOK, rec.Code, name)
	}

	// The role decides which scopes a session carries.
	adminCtx := h.authContextFor(t, adminToken)
	assert.True(t, adminCtx.HasPermission("admin"))
	assert.Equal(t, "admin", adminCtx.Role)

	memberCtx := h.authContextFor(t, memberToken)
	assert.False(t, memberCtx.HasPermission("admin"))
	assert.True(t, memberCtx.HasPermission("workflow:write"))
	assert.Equal(t, "member", memberCtx.Role)

	viewerCtx := h.authContextFor(t, viewerToken)
	assert.False(t, viewerCtx.HasPermission("workflow:write"))
	assert.True(t, viewerCtx.HasPermission("workflow:read"))
	assert.Equal(t, "viewer", viewerCtx.Role)
}

// authContextFor runs a request through the middleware and returns the
// auth context the handler saw.
func (h *authTestHarness) authContextFor(t *testing.T, token string) *AuthContext {
	t.Helper()

	var captured *AuthContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = AuthFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := AuthMiddlewareWithOptions(h.manager, AuthOptions{})
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, captured)
	return captured
}

// ---------------------------------------------------------------------------
// Credential secret protection
// ---------------------------------------------------------------------------

// TestGetCredential_DataStrippedForPlainKey asserts that an automation
// key without the explicit grant never sees the secret payload.
func TestGetCredential_DataStrippedForPlainKey(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-admin", "admin@example.com", string(auth.RoleAdmin))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"database": "billing", "password": "super-secret"},
	}))

	_, plainKey, err := h.manager.CreateAPIKeyWithPermissions("user-admin", "automation", nil, nil)
	require.NoError(t, err)

	rec := h.do(http.MethodGet, "/api/v1/credentials/cred-1", map[string]string{
		"X-API-Key": plainKey,
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "super-secret")

	var envelope CredentialResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Nil(t, envelope.Data, "the data payload must be stripped")
	assert.Equal(t, "MongoDB account", envelope.Name)
}

// TestGetCredential_DataPresentForEditorSession asserts that the
// session of an editor role sees the secret payload.
func TestGetCredential_DataPresentForEditorSession(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-member", "member@example.com", string(auth.RoleMember))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"database": "billing", "password": "super-secret"},
	}))

	token := h.login(t, "member@example.com")
	rec := h.do(http.MethodGet, "/api/v1/credentials/cred-1", map[string]string{
		"Authorization": "Bearer " + token,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope CredentialResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.NotNil(t, envelope.Data, "an editor session must see the data payload")

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "super-secret", data["password"])
}

// TestGetCredential_DataPresentWithExplicitGrant asserts that a key
// with the explicit "credentials:read-data" permission sees the
// payload.
func TestGetCredential_DataPresentWithExplicitGrant(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-member", "member@example.com", string(auth.RoleMember))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"password": "super-secret"},
	}))

	_, plainKey, err := h.manager.CreateAPIKeyWithPermissions(
		"user-member", "reader", []string{auth.PermissionCredentialReadData}, nil)
	require.NoError(t, err)

	rec := h.do(http.MethodGet, "/api/v1/credentials/cred-1", map[string]string{
		"X-API-Key": plainKey,
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "super-secret")
}

// TestGetCredential_ViewerSessionHasNoData asserts that a viewer is
// not an editor: the payload stays stripped.
func TestGetCredential_ViewerSessionHasNoData(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-viewer", "viewer@example.com", string(auth.RoleViewer))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"password": "super-secret"},
	}))

	token := h.login(t, "viewer@example.com")
	rec := h.do(http.MethodGet, "/api/v1/credentials/cred-1", map[string]string{
		"Authorization": "Bearer " + token,
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "super-secret")
}

// TestCredentialExport_RequiresAdmin asserts that the export endpoint
// is closed for everybody without the admin grant.
func TestCredentialExport_RequiresAdmin(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-member", "member@example.com", string(auth.RoleMember))
	h.createUser(t, "user-admin", "admin@example.com", string(auth.RoleAdmin))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"password": "super-secret"},
	}))

	memberToken := h.login(t, "member@example.com")
	rec := h.do(http.MethodGet, "/api/v1/credentials/export", map[string]string{
		"Authorization": "Bearer " + memberToken,
	})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// An automation key is not an admin either.
	_, plainKey, err := h.manager.CreateAPIKeyWithPermissions("user-admin", "automation", nil, nil)
	require.NoError(t, err)
	rec = h.do(http.MethodGet, "/api/v1/credentials/export", map[string]string{
		"X-API-Key": plainKey,
	})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// The admin session may export.
	adminToken := h.login(t, "admin@example.com")
	rec = h.do(http.MethodGet, "/api/v1/credentials/export", map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestCredentialExport_BodyIsEncrypted asserts that the export body
// carries the encrypted n8n envelope and never the plaintext.
func TestCredentialExport_BodyIsEncrypted(t *testing.T) {
	t.Setenv("N8N_ENCRYPTION_KEY", credentialExportTestKey)
	t.Setenv("N8N_ENCRYPTION_SALT", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("M9M_DATA_DIR", t.TempDir())

	h := newAuthTestHarness(t)
	h.createUser(t, "user-admin", "admin@example.com", string(auth.RoleAdmin))

	credMgr, err := credentials.NewCredentialManager()
	require.NoError(t, err)
	h.server.SetCredentialManager(credMgr)

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"database": "billing", "password": "super-secret"},
	}))

	adminToken := h.login(t, "admin@example.com")
	rec := h.do(http.MethodGet, "/api/v1/credentials/export", map[string]string{
		"Authorization": "Bearer " + adminToken,
	})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "super-secret")

	var exported []credentials.ExportCredential
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exported))
	require.Len(t, exported, 1)
	assert.True(t, strings.HasPrefix(exported[0].Data, "U2FsdGVkX1"),
		"the exported data must be the encrypted n8n envelope")
}

// TestCredentialExport_WritesAuditLine asserts that an export writes a
// structured audit line that carries the credential NAME but never a
// secret value.
func TestCredentialExport_WritesAuditLine(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-admin", "admin@example.com", string(auth.RoleAdmin))

	require.NoError(t, h.store.SaveCredential(&storage.Credential{
		ID:   "cred-1",
		Name: "MongoDB account",
		Type: "mongoDb",
		Data: map[string]interface{}{"password": "super-secret"},
	}))

	adminToken := h.login(t, "admin@example.com")
	rec := h.do(http.MethodGet, "/api/v1/credentials/export", map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	auditLine := h.audit.String()
	require.NotEmpty(t, auditLine)

	var event map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(auditLine)), &event))
	assert.Equal(t, AuditActionCredentialExport, event["action"])
	assert.Equal(t, "allowed", event["result"])
	assert.Equal(t, "user-admin", event["userId"])
	assert.NotContains(t, auditLine, "super-secret")

	names, ok := event["names"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, names, "MongoDB account")
}

// TestCredentialImport_RequiresAdmin asserts that the import endpoint
// is closed for a non-admin caller.
func TestCredentialImport_RequiresAdmin(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-member", "member@example.com", string(auth.RoleMember))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/credentials/import",
		strings.NewReader(`[{"id":"cred-1","name":"One","type":"mongoDb","data":"U2FsdGVkX1+x"}]`))
	req.Header.Set("Authorization", "Bearer "+h.login(t, "member@example.com"))
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ---------------------------------------------------------------------------
// Auth routes
// ---------------------------------------------------------------------------

// TestRegister_ClosesAfterTheFirstUser asserts that the public
// registration window accepts exactly one account.
func TestRegister_ClosesAfterTheFirstUser(t *testing.T) {
	h := newAuthTestHarness(t)

	body := `{"email":"owner@example.com","password":"` + authTestPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// The second registration is closed.
	body = `{"email":"second@example.com","password":"` + authTestPassword + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec = httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestLogin_ReturnsTokenAndUser asserts the login contract of the
// web UI.
func TestLogin_ReturnsTokenAndUser(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-1", "owner@example.com", string(auth.RoleAdmin))

	body := `{"email":"owner@example.com","password":"` + authTestPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var payload struct {
		Token string         `json:"token"`
		User  auth.User      `json:"user"`
		Extra map[string]any `json:"-"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.NotEmpty(t, payload.Token)
	assert.Equal(t, "owner@example.com", payload.User.Email)
	assert.Empty(t, payload.User.Password, "the password must never be part of the answer")
}

// TestCreateAPIKey_ShowsTheSecretOnce asserts that the key is handed
// out exactly once and that the listing never repeats it.
func TestCreateAPIKey_ShowsTheSecretOnce(t *testing.T) {
	h := newAuthTestHarness(t)
	h.createUser(t, "user-1", "owner@example.com", string(auth.RoleAdmin))
	token := h.login(t, "owner@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/api-keys",
		strings.NewReader(`{"name":"automation","permissions":["workflow:read"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created struct {
		Key struct {
			ID         string   `json:"id"`
			Prefix     string   `json:"prefix"`
			Permission []string `json:"permissions"`
		} `json:"apiKey"`
		Plain string `json:"key"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	plain := created.Plain
	require.NotEmpty(t, plain, "the plaintext key must be returned once")
	assert.NotEmpty(t, created.Key.ID)
	assert.NotEmpty(t, created.Key.Prefix)
	assert.Equal(t, []string{"workflow:read"}, created.Key.Permission)

	// The stored digest is never part of the answer.
	assert.NotContains(t, rec.Body.String(), auth.HashAPIKey(plain))

	// The listing never repeats the secret.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), plain)
}
