package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authManager *AuthManager
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authManager *AuthManager) *AuthHandler {
	return &AuthHandler{
		authManager: authManager,
	}
}

// RegisterRoutes registers the authentication routes.
//
// It is called with the /api/v1 subrouter of the main router, so the
// paths below end up as:
//
//	POST   /api/v1/auth/login
//	POST   /api/v1/auth/register
//	POST   /api/v1/auth/logout
//	GET    /api/v1/auth/me
//	PATCH  /api/v1/auth/me
//	GET    /api/v1/auth/users
//	POST   /api/v1/auth/users
//	GET    /api/v1/auth/users/{id}
//	PATCH  /api/v1/auth/users/{id}
//	DELETE /api/v1/auth/users/{id}
//	POST   /api/v1/auth/invites
//	POST   /api/v1/auth/invites/accept
//	GET    /api/v1/auth/api-keys
//	POST   /api/v1/auth/api-keys
//	DELETE /api/v1/auth/api-keys/{id}
//
// The login and register endpoints are the only public ones - the API
// middleware lets them through (see the public allowlist in
// internal/api/middleware.go); every other route below authenticates
// the caller itself.
func (h *AuthHandler) RegisterRoutes(api *mux.Router) {
	// Public endpoints (no auth required)
	api.HandleFunc("/auth/login", h.Login).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/register", h.Register).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/invites/accept", h.AcceptInvite).Methods("POST", "OPTIONS")

	// Authenticated endpoints (require JWT)
	api.HandleFunc("/auth/logout", h.AuthMiddleware(h.Logout)).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/me", h.AuthMiddleware(h.GetCurrentUser)).Methods("GET", "OPTIONS")
	api.HandleFunc("/auth/me", h.AuthMiddleware(h.UpdateCurrentUser)).Methods("PATCH", "OPTIONS")

	// User management (admin only)
	api.HandleFunc("/auth/users", h.AuthMiddleware(h.ListUsers)).Methods("GET", "OPTIONS")
	api.HandleFunc("/auth/users", h.AuthMiddleware(h.CreateUser)).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/users/{id}", h.AuthMiddleware(h.GetUser)).Methods("GET", "OPTIONS")
	api.HandleFunc("/auth/users/{id}", h.AuthMiddleware(h.UpdateUser)).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/auth/users/{id}", h.AuthMiddleware(h.DeleteUser)).Methods("DELETE", "OPTIONS")

	// Invitations (admin/member)
	api.HandleFunc("/auth/invites", h.AuthMiddleware(h.InviteUser)).Methods("POST", "OPTIONS")

	// API keys
	api.HandleFunc("/auth/api-keys", h.AuthMiddleware(h.ListAPIKeys)).Methods("GET", "OPTIONS")
	api.HandleFunc("/auth/api-keys", h.AuthMiddleware(h.CreateAPIKey)).Methods("POST", "OPTIONS")
	api.HandleFunc("/auth/api-keys/{id}", h.AuthMiddleware(h.DeleteAPIKey)).Methods("DELETE", "OPTIONS")
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var credentials UserCredentials
	if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, user, err := h.authManager.Login(&credentials)
	if err != nil {
		// A session that could not be persisted is a server-side
		// failure, not a wrong credential: the credentials were fine,
		// the store was not. The message stays generic - the storage
		// detail is logged by the auth manager.
		if errors.Is(err, ErrSessionUnavailable) {
			http.Error(w, "Session store unavailable", http.StatusInternalServerError)
			return
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// Register handles user registration.
//
// Public registration is only open while the instance has NO users at
// all (the first account becomes the owner). Once an owner exists the
// endpoint answers 403 - further accounts are created by an admin
// through an invitation token.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var registration UserRegistration
	if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// An invitation token is the only way in once an owner exists.
	if strings.TrimSpace(registration.InviteToken) == "" {
		allowed, err := h.authManager.PublicRegistrationOpen()
		if err != nil {
			http.Error(w, "Failed to read the user store", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "Registration is closed - this instance already has an owner", http.StatusForbidden)
			return
		}
	}

	user, err := h.authManager.Register(&registration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		http.Error(w, "No token provided", http.StatusUnauthorized)
		return
	}

	if err := h.authManager.Logout(token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetCurrentUser returns the current authenticated user
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// UpdateCurrentUser updates the current user's information
func (h *AuthHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	var update UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.authManager.UpdateUser(user.ID, &update, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get updated user
	updatedUser, err := h.authManager.userStorage.GetUser(user.ID)
	if err != nil {
		http.Error(w, "Failed to fetch updated user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedUser.Sanitize())
}

// ListUsers lists all users (admin only)
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if user.Role != string(RoleAdmin) {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	users, err := h.authManager.userStorage.ListUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanitize all users
	sanitized := make([]*User, len(users))
	for i, u := range users {
		sanitized[i] = u.Sanitize()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  sanitized,
		"count": len(sanitized),
	})
}

// CreateUser creates a new user (admin only)
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if user.Role != string(RoleAdmin) {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	var registration UserRegistration
	if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	newUser, err := h.authManager.Register(&registration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newUser)
}

// GetUser gets a user by ID (admin only)
func (h *AuthHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)

	if actor.Role != string(RoleAdmin) {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	userID := vars["id"]

	user, err := h.authManager.userStorage.GetUser(userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user.Sanitize())
}

// UpdateUser updates a user (admin only)
func (h *AuthHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)

	vars := mux.Vars(r)
	userID := vars["id"]

	var update UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.authManager.UpdateUser(userID, &update, actor.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get updated user
	updatedUser, err := h.authManager.userStorage.GetUser(userID)
	if err != nil {
		http.Error(w, "Failed to fetch updated user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedUser.Sanitize())
}

// DeleteUser deletes a user (admin only)
func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)

	vars := mux.Vars(r)
	userID := vars["id"]

	if err := h.authManager.DeleteUser(userID, actor.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// InviteUser creates an invitation (admin/member)
func (h *AuthHandler) InviteUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	invite, err := h.authManager.InviteUser(req.Email, req.Role, actor.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(invite)
}

// AcceptInvite accepts an invitation
func (h *AuthHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.authManager.AcceptInvite(req.Token, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

// CreateAPIKey creates a new API key.
//
// The plaintext key is returned exactly once, in this response. The
// store only ever keeps its SHA-256 digest.
//
// The optional `permissions` field narrows the key down to explicit
// scopes (e.g. "credentials:read-data"). Without it the key inherits
// the permissions of the owner's role.
func (h *AuthHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	var req struct {
		Name        string     `json:"name"`
		Permissions []string   `json:"permissions,omitempty"`
		ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	apiKey, plainKey, err := h.authManager.CreateAPIKeyWithPermissions(user.ID, req.Name, req.Permissions, req.ExpiresAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The stored digest never leaves the instance - the record is
	// copied and stripped before it is answered.
	safe := *apiKey
	safe.Key = ""

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"apiKey": safe,
		"key":    plainKey, // Only shown once!
	})
}

// ListAPIKeys lists the user's API keys.
//
// The stored digest is never part of the answer - only the id, the
// name, the prefix, the permissions and the lifecycle timestamps.
func (h *AuthHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	keys, err := h.authManager.userStorage.ListUserAPIKeys(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	safe := make([]*APIKey, 0, len(keys))
	for _, key := range keys {
		if key == nil {
			continue
		}
		copy := *key
		copy.Key = "" // never expose the digest
		safe = append(safe, &copy)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  safe,
		"count": len(safe),
	})
}

// DeleteAPIKey deletes an API key
func (h *AuthHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	keyID := vars["id"]

	if err := h.authManager.userStorage.DeleteAPIKey(keyID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AuthMiddleware authenticates the caller of an auth route.
//
// The API middleware (internal/api) already validated the caller and
// stored the authenticated user in the request context; that user wins.
// Without it (the auth routes mounted on their own) the bearer token
// is validated here.
func (h *AuthHandler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if user, ok := UserFromContext(r.Context()); ok {
			next(w, r.WithContext(ContextWithUser(r.Context(), user)))
			return
		}

		token := extractToken(r)
		if token == "" {
			http.Error(w, "No authorization token provided", http.StatusUnauthorized)
			return
		}

		user, err := h.authManager.GetCurrentUser(token)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add user to context
		ctx := ContextWithUser(r.Context(), user)
		r = r.WithContext(ctx)

		next(w, r)
	}
}

// currentUser returns the authenticated user of the request. The
// handlers below only run behind AuthMiddleware, so the user is always
// present.
func currentUser(r *http.Request) *User {
	if user, ok := UserFromContext(r.Context()); ok {
		return user
	}
	return &User{}
}

// extractToken extracts the JWT token from the Authorization header.
//
// The query-parameter fallback of the previous implementation was
// removed on purpose: query strings end up in access logs, proxy
// caches and browser history, so a token must never travel that way.
func extractToken(r *http.Request) string {
	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	return ""
}
