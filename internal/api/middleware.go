package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/neul-labs/m9m/internal/auth"
)

// SecurityHeadersMiddleware adds security headers to all responses
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS filter
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Strict Transport Security (for HTTPS)
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Cache control for API responses
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

		// Referrer Policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Remove server identification
		w.Header().Del("Server")

		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = allowedOrigin
			}

			// Allow specific origin or wildcard
			if allowedOrigin == "*" || origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Requested-With")
			w.Header().Set("Access-Control-Expose-Headers", "Link, Location")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrappedWriter := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Call the next handler
		next.ServeHTTP(wrappedWriter, r)

		// Log the request
		duration := time.Since(start)
		log.Printf(
			"%s %s %d %s [%s]",
			r.Method,
			r.RequestURI,
			wrappedWriter.statusCode,
			duration,
			r.RemoteAddr,
		)
	})
}

// RecoveryMiddleware recovers from panics and returns a 500 error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				log.Printf("PANIC: %v\n%s", err, debug.Stack())

				// Return 500 error
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{
					"error": true,
					"message": "Internal server error",
					"code": 500
				}`))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying WriteHeader
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RateLimitMiddleware implements basic rate limiting (optional - can be enhanced)
func RateLimitMiddleware(requestsPerMinute int) func(http.Handler) http.Handler {
	type client struct {
		requests  int
		resetTime time.Time
	}

	clients := make(map[string]*client)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			now := time.Now()

			// Get or create client
			c, exists := clients[ip]
			if !exists {
				clients[ip] = &client{
					requests:  0,
					resetTime: now.Add(time.Minute),
				}
				c = clients[ip]
			}

			// Reset if time window has passed
			if now.After(c.resetTime) {
				c.requests = 0
				c.resetTime = now.Add(time.Minute)
			}

			// Check rate limit
			if c.requests >= requestsPerMinute {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{
					"error": true,
					"message": "Rate limit exceeded",
					"code": 429
				}`))
				return
			}

			c.requests++
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware validates authentication (basic implementation)
// NOTE: This middleware requires authentication by default. Use AuthMiddlewareWithManager
// for full authentication support with JWT and API keys.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks and public endpoints
		if isPublicEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if authentication context exists (set by AuthMiddlewareWithManager)
		authCtx := AuthFromContext(r.Context())
		if authCtx == nil {
			// No auth context - return 401 Unauthorized
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{
				"error": true,
				"message": "Authentication required",
				"code": 401
			}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AuthOptions configures the authentication middleware.
type AuthOptions struct {
	// StaticAPIKeys are the automation keys configured through
	// M9M_API_KEY. They are matched against the SHA-256 digest of the
	// presented key in constant time.
	StaticAPIKeys []string

	// StaticPermissions are the scopes a static API key gets. When it
	// is empty, DefaultStaticKeyPermissions is used.
	StaticPermissions []string
}

// DefaultStaticKeyPermissions are the scopes an automation key
// (M9M_API_KEY) gets.
//
// The key can drive the workflows and the credential envelopes, but it
// can NOT read the secret payload of a credential and it can NOT export
// credentials - both of those need an explicit grant (a session of an
// editor/admin, or a key with the matching permission).
var DefaultStaticKeyPermissions = []string{
	"workflow:read",
	"workflow:write",
	"workflow:execute",
	"execution:read",
	"credential:read",
	"credential:write",
}

// AuthMiddlewareWithManager creates an authentication middleware with a configured AuthManager.
//
// The manager is the ONE auth system of m9m (internal/auth): it owns
// the users, their roles, their API keys and the JWT format. The
// deprecated second implementation in internal/api/auth_manager.go is
// not used.
func AuthMiddlewareWithManager(authManager *auth.AuthManager) func(http.Handler) http.Handler {
	return AuthMiddlewareWithOptions(authManager, AuthOptions{})
}

// AuthMiddlewareWithOptions creates the authentication middleware with
// the static automation keys.
//
// Two credentials are accepted:
//
//	Authorization: Bearer <jwt>   a session token of a logged-in user
//	X-API-Key: <key>              an automation key (static or stored)
//
// A key in the query string is NOT accepted: query strings end up in
// access logs, proxy caches and browser history.
func AuthMiddlewareWithOptions(authManager *auth.AuthManager, opts AuthOptions) func(http.Handler) http.Handler {
	staticPermissions := opts.StaticPermissions
	if len(staticPermissions) == 0 {
		staticPermissions = DefaultStaticKeyPermissions
	}

	// Pre-compute the digests of the static keys once.
	staticDigests := make([][]byte, 0, len(opts.StaticAPIKeys))
	for _, key := range opts.StaticAPIKeys {
		if key == "" {
			continue
		}
		staticDigests = append(staticDigests, []byte(auth.HashAPIKey(key)))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for public endpoints
			if isPublicEndpoint(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			var user *auth.User
			var authContext *AuthContext
			var err error
			var authMethod string

			// Try Bearer token first
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				user, err = authManager.ValidateSessionToken(token)
				if err != nil {
					log.Printf("JWT validation failed: %v", err)
				} else {
					authMethod = "jwt"
					// A session carries the identity and the role of the
					// user. The permissions of the role are filled in
					// below, when the context is completed.
					authContext = &AuthContext{
						UserID:     user.ID,
						AuthMethod: authMethod,
						Role:       user.Role,
					}
				}
			}

			// Try the API key header if no valid JWT
			if user == nil {
				apiKey := r.Header.Get("X-API-Key")
				if apiKey == "" {
					// Also check Authorization header for API key
					if strings.HasPrefix(authHeader, "Api-Key ") {
						apiKey = strings.TrimPrefix(authHeader, "Api-Key ")
					}
				}

				if apiKey != "" {
					// A static (configured) automation key wins over the
					// stored ones - it is the credential the operator
					// configured for this instance.
					if matchesStaticKey(apiKey, staticDigests) {
						authContext = &AuthContext{
							APIKeyID:            "static",
							Permissions:         staticPermissions,
							ExplicitPermissions: staticPermissions,
							AuthMethod:          "api_key",
						}
						ctx := ContextWithAuth(r.Context(), authContext)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}

					var key *auth.APIKey
					user, key, err = authManager.ValidateAPIKeyWithPermissions(apiKey)
					if err != nil {
						log.Printf("API key validation failed: %v", err)
					} else {
						authMethod = "api_key"
						permissions := key.Permissions
						if len(permissions) == 0 {
							// A key without explicit scopes inherits the
							// permissions of its owner's role.
							permissions = getPermissionsForRole(auth.UserRole(user.Role))
						}
						authContext = &AuthContext{
							UserID:              user.ID,
							APIKeyID:            key.ID,
							Permissions:         permissions,
							ExplicitPermissions: key.Permissions,
							AuthMethod:          authMethod,
							Role:                user.Role,
						}
					}
				} else if r.URL.Query().Get("apiKey") != "" || r.URL.Query().Get("api_key") != "" {
					// Query-parameter keys stay rejected on purpose.
					log.Printf("SECURITY WARNING: API key provided in query parameter (rejected). Use the X-API-Key header instead.")
				}
			}

			// If no valid authentication found, return 401
			if user == nil || authContext == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{
					"error": true,
					"message": "Unauthorized: valid authentication required",
					"code": 401
				}`))
				return
			}

			if authContext.UserID == "" {
				authContext.UserID = user.ID
			}
			if authContext.Role == "" {
				authContext.Role = user.Role
			}

			// Build permissions from role
			if len(authContext.Permissions) == 0 {
				userRole := auth.UserRole(user.Role)
				if userRole == auth.RoleAdmin {
					authContext.Permissions = []string{"admin"}
				} else {
					authContext.Permissions = getPermissionsForRole(userRole)
				}
			}

			// Add auth context to request
			ctx := ContextWithAuth(r.Context(), authContext)
			ctx = auth.ContextWithUser(ctx, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// matchesStaticKey compares the presented key against the digests of
// the configured static keys in constant time.
func matchesStaticKey(presented string, digests [][]byte) bool {
	if presented == "" || len(digests) == 0 {
		return false
	}

	candidate := []byte(auth.HashAPIKey(presented))
	matched := 0
	for _, digest := range digests {
		matched |= subtle.ConstantTimeCompare(candidate, digest)
	}
	return matched == 1
}

// getPermissionsForRole returns the permissions for a given role
func getPermissionsForRole(role auth.UserRole) []string {
	switch role {
	case auth.RoleAdmin:
		return []string{"admin"}
	case auth.RoleMember:
		return []string{
			"workflow:read", "workflow:write", "workflow:execute",
			"execution:read", "credential:read", "credential:write",
		}
	case auth.RoleViewer:
		return []string{"workflow:read", "execution:read"}
	default:
		return []string{}
	}
}

// publicPaths is THE public allowlist of the m9m API.
//
// It is the single place that decides which requests reach a handler
// without a credential. Everything else under /api/v1 needs a bearer
// token or an X-API-Key header.
//
//	/health, /healthz, /ready          liveness + readiness probes
//	/api/v1/auth/login                login
//	/api/v1/auth/register             first-boot registration
//	/api/v1/auth/invites/accept       invitation acceptance
//	/webhook/*                        webhook triggers (own auth)
//	/api/v1/webhooks/*                webhook management for nodes
//
// The registration endpoint is only open while the instance has no
// user at all - the auth handler enforces that, the allowlist only
// decides that the request may reach it.
var publicPaths = []string{
	"/health",
	"/healthz",
	"/ready",
	"/api/v1/auth/login",
	"/api/v1/auth/register",
	"/api/v1/auth/invites/accept",
	"/api/v1/auth/password/reset",
	"/api/v1/auth/password/reset/confirm",
}

// isPublicEndpoint checks if a path is a public endpoint that doesn't require authentication
func isPublicEndpoint(path string) bool {
	for _, p := range publicPaths {
		if path == p {
			return true
		}
	}

	// Also allow webhook endpoints (they have their own authentication)
	if strings.HasPrefix(path, "/webhook/") || strings.HasPrefix(path, "/api/v1/webhooks/") {
		return true
	}

	return false
}

// RequireAuthMiddleware creates a middleware that requires authentication
func RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx := AuthFromContext(r.Context())
		if authCtx == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{
				"error": true,
				"message": "Authentication required",
				"code": 401
			}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequirePermissionMiddleware creates a middleware that requires a specific permission
func RequirePermissionMiddleware(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := AuthFromContext(r.Context())
			if authCtx == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{
					"error": true,
					"message": "Authentication required",
					"code": 401
				}`))
				return
			}

			// Check if user has the required permission
			if !authCtx.HasPermission(permission) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{
					"error": true,
					"message": "Permission denied: ` + permission + ` required",
					"code": 403
				}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdminMiddleware creates a middleware that requires admin role
func RequireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx := AuthFromContext(r.Context())
		if authCtx == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{
				"error": true,
				"message": "Authentication required",
				"code": 401
			}`))
			return
		}

		// Check if user has admin permission
		if !authCtx.HasPermission("admin") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{
				"error": true,
				"message": "Admin access required",
				"code": 403
			}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
