package api

// Tests for BUG-SEC-2026-10-12-004 — RBAC scaffold.

import (
	"testing"
)

func TestRBAC_BuiltInRoles(t *testing.T) {
	s := NewRBACStore()
	cases := []struct {
		roles []string
		perm  Permission
		want  bool
	}{
		{[]string{"viewer"}, PermWorkflowRead, true},
		{[]string{"viewer"}, PermWorkflowWrite, false},
		{[]string{"editor"}, PermWorkflowWrite, true},
		{[]string{"editor"}, PermUserManage, false},
		{[]string{"admin"}, PermCredentialWrite, true}, // admin wildcard
		{[]string{"admin"}, PermWorkflowRead, true},    // admin wildcard
		{[]string{"viewer", "editor"}, PermUserManage, false},
		{nil, PermWorkflowRead, false},
	}
	for _, c := range cases {
		u := User{ID: "u1", Roles: c.roles}
		if got := s.HasPermission(u, c.perm); got != c.want {
			t.Errorf("roles=%v, perm=%q: got %v, want %v", c.roles, c.perm, got, c.want)
		}
	}
}

func TestRBAC_AddCustomRole(t *testing.T) {
	s := NewRBACStore()
	if err := s.AddRole(Role{
		Name: "auditor",
		Permissions: []Permission{
			PermWorkflowRead,
			PermCredentialRead,
		},
	}); err != nil {
		t.Fatalf("AddRole: %v", err)
	}
	u := User{ID: "a", Roles: []string{"auditor"}}
	if !s.HasPermission(u, PermWorkflowRead) {
		t.Errorf("auditor should have workflow:read")
	}
	if s.HasPermission(u, PermWorkflowWrite) {
		t.Errorf("auditor should NOT have workflow:write")
	}
}

func TestRBAC_ReservedRoleRejected(t *testing.T) {
	s := NewRBACStore()
	err := s.AddRole(Role{Name: "admin", Permissions: []Permission{PermWorkflowRead}})
	if !IsRBACError(err) {
		t.Fatalf("expected RBACError, got %v", err)
	}
}

func TestRBAC_MultipleRolesInherit(t *testing.T) {
	s := NewRBACStore()
	_ = s.AddRole(Role{
		Name: "billing",
		Permissions: []Permission{"billing:read"},
	})
	_ = s.AddRole(Role{
		Name: "deployer",
		Permissions: []Permission{"workflow:execute"},
	})
	u := User{ID: "u", Roles: []string{"billing", "deployer"}}
	if !s.HasPermission(u, "billing:read") {
		t.Errorf("expected billing:read via billing role")
	}
	if !s.HasPermission(u, "workflow:execute") {
		t.Errorf("expected workflow:execute via deployer role")
	}
	if s.HasPermission(u, "user:manage") {
		t.Errorf("expected denial of user:manage")
	}
}

func TestRBAC_AdminWildcardShortCircuits(t *testing.T) {
	// The effective permission set for an admin user is just
	// ["admin"] — every other perm is short-circuited by the
	// HasPermission loop. This keeps the auth middleware
	// cheap (one map lookup, not 30).
	s := NewRBACStore()
	u := User{ID: "a", Roles: []string{"admin"}}
	got := s.EffectivePermissions(u)
	if len(got) != 1 || got[0] != "admin" {
		t.Errorf("admin effective = %v, want [admin]", got)
	}
}

func TestRBAC_RoleFromStringList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"editor", []string{"editor"}},
		{"editor, admin", []string{"editor", "admin"}},
		{"  viewer  ,   editor  ", []string{"viewer", "editor"}},
		{",,", nil},
	}
	for _, c := range cases {
		got := RoleFromStringList(c.in)
		if !equalStringSlices(got, c.want) {
			t.Errorf("RoleFromStringList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
