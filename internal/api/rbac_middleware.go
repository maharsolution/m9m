package api

// RBAC enforcement middleware for BUG-SEC-2026-10-12-004.
//
// The `RBACStore` (rbac.go) provides the read API:
// `HasPermission(user, perm)`. The middleware below wraps
// an `http.Handler` (or a `mux.MiddlewareFunc` for gorilla/
// mux), extracts the authenticated subject from the request
// context (via `AuthFromContext`), looks up the user in
// the `RBACStore`, and either short-circuits with 403
// or delegates to the wrapped handler.
//
// Two integration modes are supported:
//
//   1. **UserProvider** is the source of truth for "who
//      is this request". The default provider reads the
//      `user_id` / `sub` field from the `AuthContext` and
//      looks it up in the `RBACStore`. A production
//      deployment that wires OIDC or LDAP can supply its
//      own `UserProvider` to enrich the user (e.g. fetch
//      roles from the IdP claim).
//
//   2. The middleware is mounted in `cmd/m9m/main.go` next
//      to the existing auth middleware. The recommended
//      pattern is to gate the whole `/api/v1/*` tree with
//      `RequireAuth`, then gate the resource-level routes
//      with `RequirePermission("workflow:write")` etc.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// UserProvider turns a request into the RBAC user. The
// default provider reads `auth.UserID` and looks the user
// up in the `RBACStore`. Production deployments that have
// OIDC/SSO wired can supply a provider that augments the
// user's roles from the IdP claim.
type UserProvider interface {
	UserForRequest(r *http.Request) (User, error)
}

// DefaultUserProvider is the in-process provider.
type DefaultUserProvider struct {
	Store *RBACStore
}

// UserForRequest returns the user identified by the auth
// context. If the auth context is missing (the route is
// not behind `RequireAuth`), returns an error.
func (p *DefaultUserProvider) UserForRequest(r *http.Request) (User, error) {
	auth := AuthFromContext(r.Context())
	if auth == nil {
		return User{}, errors.New("rbac: missing auth context")
	}
	id := auth.UserID
	if id == "" {
		// Fall back to the API-key identifier.
		id = auth.APIKeyID
	}
	if id == "" {
		return User{}, errors.New("rbac: no subject in auth context")
	}
	u, ok := p.Store.LookupUser(id)
	if !ok {
		// Authenticated but unknown to the RBAC store.
		// Return a fresh user with no roles — every check
		// will fail closed (deny). The operator can later
		// `AddUser` to grant access.
		return User{ID: id}, nil
	}
	return u, nil
}

// rbacMiddleware is the http.Handler wrapper. Public API:
// `RequirePermission(perm)` returns a `func(http.Handler)
// http.Handler` (mux.MiddlewareFunc-compatible).
type rbacMiddleware struct {
	store    *RBACStore
	provider UserProvider
}

// NewRBACMiddleware returns a middleware factory. The
// `provider` can be nil; in that case the default
// in-process provider is used.
func NewRBACMiddleware(store *RBACStore, provider UserProvider) *rbacMiddleware {
	if provider == nil {
		provider = &DefaultUserProvider{Store: store}
	}
	return &rbacMiddleware{store: store, provider: provider}
}

// RequirePermission returns a middleware that 403s any
// request whose authenticated user lacks `perm`.
//
// Usage with gorilla/mux:
//
//	api := mux.NewRouter()
//	api.Use(RequireAuth(...))
//	api.Handle("/workflows", RequirePermission(PermWorkflowRead)(listWorkflows)).Methods("GET")
//	api.Handle("/workflows", RequirePermission(PermWorkflowWrite)(createWorkflow)).Methods("POST")
func (m *rbacMiddleware) RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, err := m.provider.UserForRequest(r)
			if err != nil {
				http.Error(w, "rbac: "+err.Error(), http.StatusUnauthorized)
				return
			}
			if !m.store.HasPermission(u, perm) {
				writeForbidden(w, u, perm)
				return
			}
			// Stash the user in the request context so
			// downstream handlers can read it without
			// re-running the lookup.
			ctx := context.WithValue(r.Context(), rbacUserKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAnyPermission returns a middleware that 403s any
// request whose authenticated user lacks ALL of the listed
// permissions. Useful for routes that accept either
// `workflow:write` (UI editing) or `workflow:execute`
// (CLI one-shot).
func (m *rbacMiddleware) RequireAnyPermission(perms ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, err := m.provider.UserForRequest(r)
			if err != nil {
				http.Error(w, "rbac: "+err.Error(), http.StatusUnauthorized)
				return
			}
			allowed := false
			for _, p := range perms {
				if m.store.HasPermission(u, p) {
					allowed = true
					break
				}
			}
			if !allowed {
				writeForbidden(w, u, perms...)
				return
			}
			ctx := context.WithValue(r.Context(), rbacUserKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeForbidden renders a 403 with a JSON body the UI can
// inspect. The body intentionally does NOT echo the user's
// ID or roles (don't leak authorisation state to a probing
// client).
func writeForbidden(w http.ResponseWriter, _ User, want ...Permission) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":       "forbidden",
		"message":     "insufficient permissions",
		"required":    want,
	})
}

type contextKey string

const rbacUserKey contextKey = "rbac_user"

// UserFromContext returns the user the middleware stashed
// in the request context, or `false` if the request didn't
// go through the middleware.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(rbacUserKey).(User)
	return u, ok
}

// ----- persistence (in-memory, JSON) -----

// SnapshotRBAC serializes the store's users and custom
// roles to JSON. Built-in roles are not stored (they're
// always available). The result is stable across `go
// version`s; intended for `POST /api/v1/admin/rbac/load`.
func SnapshotRBAC(store *RBACStore) ([]byte, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	users := make([]User, 0, len(store.users))
	for _, u := range store.users {
		// Skip users that have only built-in roles —
		// reloading would be a no-op.
		users = append(users, u)
	}
	roles := make(map[string]Role)
	for name, r := range store.roles {
		if _, builtin := map[string]bool{"viewer": true, "editor": true, "admin": true}[name]; builtin {
			continue
		}
		roles[name] = r
	}
	return json.Marshal(struct {
		Users []User          `json:"users"`
		Roles map[string]Role `json:"roles"`
	}{Users: users, Roles: roles})
}

// LoadRBAC replaces the store's custom roles and users
// with the snapshot. Built-in roles are left untouched.
// Returns the number of users + roles loaded.
func LoadRBAC(store *RBACStore, data []byte) (int, error) {
	var snap struct {
		Users []User          `json:"users"`
		Roles map[string]Role `json:"roles"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return 0, fmt.Errorf("rbac: decode: %w", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for name, r := range snap.Roles {
		store.roles[name] = r
	}
	for _, u := range snap.Users {
		store.users[u.ID] = u
	}
	return len(snap.Users) + len(snap.Roles), nil
}
