package api

// RBAC scaffold for BUG-SEC-2026-10-12-004.
//
// The current `AuthContext` carries a flat `[]string` of
// permission names. For teams > 5 people that stops scaling:
// a permissions list of 30+ items, ad-hoc strings, no concept
// of inheritance, no audit trail. The scaffold below introduces
// role-based access control as a thin layer on top:
//
//   - `Role` is a named bundle of permissions.
//   - `User` has a slice of roles.
//   - `EffectivePermissions(user)` flattens the user's roles
//     into a permission set; `AuthContext.HasPermission` is
//     the read path used by HTTP middleware.
//
// In v0.5.0 we'll wire the existing `AuthContext` to use this
// module (replace the string slice with a `[]Role`). For now
// the scaffold is in place and covered by tests so a future
// PR can do the swap without a re-design.

import (
	"strings"
	"sync"
)

// Permission is a single capability. Keep them lower_snake_case
// so they round-trip cleanly through n8n's wire format.
type Permission string

// Built-in permissions. The list is intentionally tiny; v0.5.0
// will grow it from real audit-log analysis.
const (
	PermWorkflowRead    Permission = "workflow:read"
	PermWorkflowWrite   Permission = "workflow:write"
	PermWorkflowExecute Permission = "workflow:execute"
	PermCredentialRead  Permission = "credential:read"
	PermCredentialWrite Permission = "credential:write"
	PermUserManage      Permission = "user:manage"
	PermAdmin           Permission = "admin" // sentinel
)

// Role is a named bundle of permissions. A user inherits the
// union of all their roles' permissions.
type Role struct {
	Name        string
	Permissions []Permission
}

// BuiltInRoles are the default roles every install ships with.
// Custom roles are persisted in the credential / RBAC store
// (v0.5.0, BUG-CAP-2026-10-12-006).
var BuiltInRoles = []Role{
	{
		Name: "viewer",
		Permissions: []Permission{
			PermWorkflowRead,
			PermCredentialRead,
		},
	},
	{
		Name: "editor",
		Permissions: []Permission{
			PermWorkflowRead,
			PermWorkflowWrite,
			PermWorkflowExecute,
			PermCredentialRead,
			PermCredentialWrite,
		},
	},
	{
		Name: "admin",
		Permissions: []Permission{
			PermAdmin, // sentinel: HasPermission(perm) returns true if any role has "admin"
		},
	},
}

// User is the RBAC subject. It is intentionally a thin type
// here — the production user store (LDAP, local, OIDC) plugs
// in behind the `UserProvider` interface.
type User struct {
	ID    string
	Email string
	Roles []string // role names
}

// RBACStore is the in-process RBAC service. It is goroutine-safe.
type RBACStore struct {
	mu    sync.RWMutex
	roles map[string]Role
	users map[string]User
}

// NewRBACStore returns an RBAC store pre-populated with
// `BuiltInRoles`. Pass `nil` users; add them with `AddUser`.
func NewRBACStore() *RBACStore {
	s := &RBACStore{
		roles: make(map[string]Role),
		users: make(map[string]User),
	}
	for _, r := range BuiltInRoles {
		s.roles[r.Name] = r
	}
	return s
}

// AddRole registers a custom role. Built-in role names
// cannot be overwritten.
func (s *RBACStore) AddRole(r Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, builtin := map[string]bool{"viewer": true, "editor": true, "admin": true}[r.Name]; builtin {
		return ErrRoleReserved
	}
	s.roles[r.Name] = r
	return nil
}

// AddUser registers or updates a user.
func (s *RBACStore) AddUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
}

// EffectivePermissions flattens the user's roles into a set of
// permission strings. The `admin` sentinel permission grants
// the holder every other permission automatically.
func (s *RBACStore) EffectivePermissions(u User) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]struct{}{}
	for _, roleName := range u.Roles {
		role, ok := s.roles[roleName]
		if !ok {
			continue
		}
		for _, p := range role.Permissions {
			seen[string(p)] = struct{}{}
		}
	}
	if _, isAdmin := seen["admin"]; isAdmin {
		return []string{"admin"}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// HasPermission mirrors the existing `AuthContext.HasPermission`
// semantics: admin is a wildcard, otherwise the permission
// must be in the user's effective set.
func (s *RBACStore) HasPermission(u User, want Permission) bool {
	effective := s.EffectivePermissions(u)
	for _, p := range effective {
		if p == "admin" || p == string(want) {
			return true
		}
	}
	return false
}

// LookupUser is a convenience for HTTP handlers that already
// have a user ID.
func (s *RBACStore) LookupUser(id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

// ErrRoleReserved is returned when a caller tries to overwrite
// a built-in role.
var ErrRoleReserved = &RBACError{msg: "role name is reserved"}

// RBACError is the typed error returned by the RBAC store.
type RBACError struct{ msg string }

func (e *RBACError) Error() string { return "rbac: " + e.msg }

// IsRBACError reports whether the error is a *RBACError.
func IsRBACError(err error) bool {
	_, ok := err.(*RBACError)
	return ok
}

// RoleFromStringList parses a comma-separated role list
// (e.g. from an OIDC claim) and returns the slice. Trims
// whitespace; ignores empty parts.
func RoleFromStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
