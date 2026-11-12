package api

import (
	"context"
)

// Context keys for storing authentication information
type contextKey string

const (
	authContextKey contextKey = "auth_context"
)

// AuthContext contains authentication information.
//
// It is filled by the authentication middleware (see
// AuthMiddlewareWithManager in middleware.go) and read by the handlers
// that need to know WHO is calling and WHAT they may do.
//
// Role is the n8n-style role of the authenticated user ("admin",
// "member", "viewer") and is only set for session (JWT) callers. An
// API key has no role - its scopes live in Permissions.
type AuthContext struct {
	UserID      string
	SessionID   string
	APIKeyID    string
	Permissions []string
	AuthMethod  string // "jwt", "api_key"
	Role        string // "admin", "member", "viewer" (session callers only)

	// ExplicitPermissions are the scopes that were attached to the
	// credential ITSELF - the scopes of an API key.
	//
	// They are deliberately kept apart from Permissions: Permissions
	// may hold the scopes a key INHERITS from the role of its owner,
	// and an inherited scope must never unlock a secret-bearing
	// operation (credential secrets, credential export).
	ExplicitPermissions []string
}

// ContextWithAuth adds authentication context to a context
func ContextWithAuth(ctx context.Context, auth *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey, auth)
}

// AuthFromContext retrieves authentication context from a context
func AuthFromContext(ctx context.Context) *AuthContext {
	auth, ok := ctx.Value(authContextKey).(*AuthContext)
	if !ok {
		return nil
	}
	return auth
}

// RequireAuthFromContext retrieves authentication context and panics if not found
// This should only be used in handlers that are protected by authentication middleware
func RequireAuthFromContext(ctx context.Context) *AuthContext {
	auth := AuthFromContext(ctx)
	if auth == nil {
		panic("authentication context not found - ensure authentication middleware is applied")
	}
	return auth
}

// HasPermission checks if the authentication context has a specific permission
func (ac *AuthContext) HasPermission(permission string) bool {
	if ac == nil {
		return false
	}

	for _, perm := range ac.Permissions {
		if perm == permission || perm == "admin" {
			return true
		}
	}
	return false
}

// hasExplicitPermission reports whether the permission list contains the
// permission EXACTLY.
//
// The "admin" wildcard of HasPermission does NOT count here: the
// fine-grained grants (credentials:read-data, credentials:export) have
// to be attached on purpose, so a key that inherits the scopes of its
// owner never gets them by accident.
func (ac *AuthContext) hasExplicitPermission(permission string) bool {
	if ac == nil {
		return false
	}

	for _, perm := range ac.ExplicitPermissions {
		if perm == permission {
			return true
		}
	}
	return false
}

// IsAPIKey returns true if the authentication was done via API key
func (ac *AuthContext) IsAPIKey() bool {
	return ac != nil && ac.AuthMethod == "api_key"
}

// IsJWT returns true if the authentication was done via JWT token
func (ac *AuthContext) IsJWT() bool {
	return ac != nil && ac.AuthMethod == "jwt"
}

// GetUserID returns the user ID if available
func (ac *AuthContext) GetUserID() string {
	if ac == nil {
		return ""
	}
	return ac.UserID
}

// GetIdentifier returns a unique identifier for the authenticated entity
func (ac *AuthContext) GetIdentifier() string {
	if ac == nil {
		return "anonymous"
	}

	if ac.UserID != "" {
		return ac.UserID
	}

	if ac.APIKeyID != "" {
		return "api_key:" + ac.APIKeyID
	}

	return "unknown"
}
