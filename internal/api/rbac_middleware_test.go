package api

// Tests for BUG-SEC-2026-10-12-004 — RBAC enforcement
// middleware + persistence.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withAuth returns a context with an `AuthContext` for the
// given user id. Helper for the middleware tests.
func withAuth(ctx context.Context, userID string) context.Context {
	return ContextWithAuth(ctx, &AuthContext{UserID: userID, AuthMethod: "jwt"})
}

func TestRBACMiddleware_RequirePermission_HappyPath(t *testing.T) {
	store := NewRBACStore()
	store.AddUser(User{ID: "u-1", Roles: []string{"editor"}})
	mw := NewRBACMiddleware(store, nil)

	var hit bool
	h := mw.RequirePermission(PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/workflows", nil)
	req = req.WithContext(withAuth(req.Context(), "u-1"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !hit {
		t.Errorf("handler not hit; status=%d", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRBACMiddleware_RequirePermission_Denies(t *testing.T) {
	store := NewRBACStore()
	store.AddUser(User{ID: "u-1", Roles: []string{"viewer"}})
	mw := NewRBACMiddleware(store, nil)

	h := mw.RequirePermission(PermWorkflowWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be hit")
	}))

	req := httptest.NewRequest("POST", "/workflows", nil)
	req = req.WithContext(withAuth(req.Context(), "u-1"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forbidden") {
		t.Errorf("body = %q, want 'forbidden' in body", rec.Body.String())
	}
}

func TestRBACMiddleware_RequirePermission_MissingAuth(t *testing.T) {
	store := NewRBACStore()
	mw := NewRBACMiddleware(store, nil)
	h := mw.RequirePermission(PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be hit")
	}))
	req := httptest.NewRequest("GET", "/workflows", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRBACMiddleware_AdminShortCircuits(t *testing.T) {
	store := NewRBACStore()
	store.AddUser(User{ID: "u-admin", Roles: []string{"admin"}})
	mw := NewRBACMiddleware(store, nil)
	h := mw.RequirePermission(PermCredentialWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/creds", nil)
	req = req.WithContext(withAuth(req.Context(), "u-admin"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("admin status = %d, want 200", rec.Code)
	}
}

func TestRBACMiddleware_RequireAnyPermission(t *testing.T) {
	store := NewRBACStore()
	store.AddUser(User{ID: "u-1", Roles: []string{"viewer"}})
	mw := NewRBACMiddleware(store, nil)

	// viewer has workflow:read but not workflow:execute.
	// RequireAny should allow it.
	var hit int
	h := mw.RequireAnyPermission(PermWorkflowExecute, PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/x", nil)
	req = req.WithContext(withAuth(req.Context(), "u-1"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if hit != 1 {
		t.Errorf("hit = %d, want 1", hit)
	}
}

func TestRBACMiddleware_UserFromContext(t *testing.T) {
	store := NewRBACStore()
	store.AddUser(User{ID: "u-1", Email: "u@x", Roles: []string{"editor"}})
	mw := NewRBACMiddleware(store, nil)
	var got User
	var ok bool
	h := mw.RequirePermission(PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req = req.WithContext(withAuth(req.Context(), "u-1"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !ok {
		t.Fatal("UserFromContext returned ok=false")
	}
	if got.ID != "u-1" || got.Email != "u@x" {
		t.Errorf("user = %+v", got)
	}
}

func TestRBACMiddleware_UnknownUserFailsClosed(t *testing.T) {
	// A user with no entry in the RBAC store gets a
	// fresh `User{ID: "u-unknown"}` (no roles). Every
	// permission check must deny.
	store := NewRBACStore()
	mw := NewRBACMiddleware(store, nil)
	h := mw.RequirePermission(PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be hit for unknown user")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req = req.WithContext(withAuth(req.Context(), "u-unknown"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRBACMiddleware_CustomUserProvider(t *testing.T) {
	store := NewRBACStore()
	// Provider that ignores the auth context and always
	// returns a fixed user. Used to verify the
	// UserProvider seam.
	provider := &staticProvider{user: User{ID: "u-fixed", Roles: []string{"admin"}}}
	mw := NewRBACMiddleware(store, provider)
	h := mw.RequirePermission(PermUserManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Note: no auth context — the static provider supplies the user.
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

type staticProvider struct{ user User }

func (p *staticProvider) UserForRequest(r *http.Request) (User, error) {
	return p.user, nil
}

type errProvider struct{ err error }

func (p *errProvider) UserForRequest(r *http.Request) (User, error) {
	return User{}, p.err
}

func TestRBACMiddleware_ProviderError(t *testing.T) {
	store := NewRBACStore()
	mw := NewRBACMiddleware(store, &errProvider{err: errors.New("user lookup failed")})
	h := mw.RequirePermission(PermWorkflowRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be hit")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRBACPersistence_SnapshotRoundTrip(t *testing.T) {
	store := NewRBACStore()
	_ = store.AddRole(Role{Name: "auditor", Permissions: []Permission{PermWorkflowRead}})
	store.AddUser(User{ID: "u-1", Roles: []string{"editor"}})
	store.AddUser(User{ID: "u-2", Roles: []string{"auditor"}})

	data, err := SnapshotRBAC(store)
	if err != nil {
		t.Fatalf("SnapshotRBAC: %v", err)
	}
	// Sanity: the snapshot has the custom role and both users.
	var snap struct {
		Users []User          `json:"users"`
		Roles map[string]Role `json:"roles"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := snap.Roles["auditor"]; !ok {
		t.Errorf("snapshot missing auditor role")
	}
	if len(snap.Users) != 2 {
		t.Errorf("snapshot users = %d, want 2", len(snap.Users))
	}
	if _, ok := snap.Roles["admin"]; ok {
		t.Errorf("built-in admin role should not be snapshotted")
	}

	// Round-trip into a fresh store.
	fresh := NewRBACStore()
	n, err := LoadRBAC(fresh, data)
	if err != nil {
		t.Fatalf("LoadRBAC: %v", err)
	}
	if n != 3 { // 1 role + 2 users
		t.Errorf("loaded = %d, want 3", n)
	}
	if !fresh.HasPermission(User{ID: "u-1", Roles: []string{"editor"}}, PermWorkflowRead) {
		t.Errorf("editor should have read on fresh store")
	}
	if !fresh.HasPermission(User{ID: "u-2", Roles: []string{"auditor"}}, PermWorkflowRead) {
		t.Errorf("auditor should have read on fresh store")
	}
}

func TestRBACPersistence_LoadRBAC_BadJSON(t *testing.T) {
	store := NewRBACStore()
	if _, err := LoadRBAC(store, []byte("not json")); err == nil {
		t.Fatal("expected error on bad JSON")
	}
}

func TestRBAC_DefaultProvider_MissingAuth(t *testing.T) {
	store := NewRBACStore()
	p := &DefaultUserProvider{Store: store}
	if _, err := p.UserForRequest(httptest.NewRequest("GET", "/x", nil)); err == nil {
		t.Fatal("expected error on missing auth context")
	}
}

func TestRBAC_DefaultProvider_EmptySubject(t *testing.T) {
	store := NewRBACStore()
	p := &DefaultUserProvider{Store: store}
	req := httptest.NewRequest("GET", "/x", nil)
	req = req.WithContext(ContextWithAuth(req.Context(), &AuthContext{}))
	if _, err := p.UserForRequest(req); err == nil {
		t.Fatal("expected error on empty subject")
	}
}
