package auth

import "context"

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// ContextWithUser stores an authenticated user in the context.
//
// The API middleware (internal/api) calls this after it validated a
// bearer token or an API key, so the auth handler and every other
// handler can read the identity of the caller without re-parsing the
// token.
func ContextWithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext returns the authenticated user stored in the context,
// if any.
func UserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey).(*User)
	if !ok || user == nil {
		return nil, false
	}
	return user, true
}
