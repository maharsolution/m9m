package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// ErrSessionUnavailable is returned when a login session could not be
// persisted in the user store.
//
// A token whose session row is missing is rejected by every later
// request ("session expired or revoked"), so a login must never hand
// out a token whose session could not be stored. The underlying storage
// error is logged in full; the returned error stays generic so no
// database detail leaks into an HTTP response.
var ErrSessionUnavailable = errors.New("session store unavailable")

// AuthManager handles authentication and user management
type AuthManager struct {
	userStorage    UserStorage
	jwtManager     *JWTManager
	passwordHasher *PasswordHasher

	// registrationOpen controls the public registration endpoint. It
	// starts open (a fresh instance accepts exactly one registration -
	// the owner) and is closed by the operator as soon as an owner
	// account exists.
	registrationOpen bool
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(userStorage UserStorage, jwtSecretKey string, tokenDuration time.Duration) *AuthManager {
	return &AuthManager{
		userStorage:      userStorage,
		jwtManager:       NewJWTManager(jwtSecretKey, tokenDuration, "m9m"),
		passwordHasher:   NewPasswordHasher(),
		registrationOpen: true,
	}
}

// Login authenticates a user and returns a JWT token
func (m *AuthManager) Login(credentials *UserCredentials) (string, *User, error) {
	// Validate credentials
	if credentials.Email == "" || credentials.Password == "" {
		return "", nil, errors.New("email and password are required")
	}

	// Get user by email
	user, err := m.userStorage.GetUserByEmail(credentials.Email)
	if err != nil {
		log.Printf("Login failed for %s: user not found", credentials.Email)
		return "", nil, errors.New("invalid email or password")
	}

	// Check if user is active
	if !user.Active {
		return "", nil, errors.New("user account is deactivated")
	}

	// Verify password
	if err := m.passwordHasher.VerifyPassword(credentials.Password, user.Password); err != nil {
		log.Printf("Login failed for %s: invalid password", credentials.Email)
		return "", nil, errors.New("invalid email or password")
	}

	// Generate JWT token
	token, err := m.jwtManager.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Create session. The session lives in the user store, so it
	// survives a restart of the server - and a logout revokes the
	// token immediately.
	session := &Session{
		ID:        generateID("sess"),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(m.jwtManager.tokenDuration),
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	if err := m.userStorage.SaveSession(session); err != nil {
		// The session MUST be in the store: the token is only accepted
		// while its session row exists. A token without a session is
		// rejected by every later request, so the login fails instead
		// of handing out a dead token.
		log.Printf("Login failed for %s: failed to persist session: %v", user.Email, err)
		return "", nil, ErrSessionUnavailable
	}

	log.Printf("User logged in: %s (%s)", user.Email, user.ID)

	return token, user.Sanitize(), nil
}

// Register creates a new user account
func (m *AuthManager) Register(registration *UserRegistration) (*User, error) {
	// Validate email
	if err := ValidateEmail(registration.Email); err != nil {
		return nil, fmt.Errorf("invalid email: %w", err)
	}

	// Validate password
	if err := ValidatePassword(registration.Password); err != nil {
		return nil, fmt.Errorf("invalid password: %w", err)
	}

	// Check if user already exists
	if _, err := m.userStorage.GetUserByEmail(registration.Email); err == nil {
		return nil, fmt.Errorf("user with email %s already exists", registration.Email)
	}

	// Hash password
	hashedPassword, err := m.passwordHasher.HashPassword(registration.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Determine role (first user is admin, others are members)
	role := string(RoleMember)
	users, _ := m.userStorage.ListUsers()
	if len(users) == 0 {
		role = string(RoleAdmin)
		log.Println("First user registration - assigning admin role")
	}

	// Create user
	user := &User{
		ID:        generateID("user"),
		Email:     registration.Email,
		Password:  hashedPassword,
		FirstName: registration.FirstName,
		LastName:  registration.LastName,
		Role:      role,
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.userStorage.SaveUser(user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	log.Printf("User registered: %s (%s) - role: %s", user.Email, user.ID, user.Role)

	return user.Sanitize(), nil
}

// ValidateToken validates a JWT token and returns the claims
func (m *AuthManager) ValidateToken(tokenString string) (*JWTClaims, error) {
	claims, err := m.jwtManager.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Verify user still exists and is active
	user, err := m.userStorage.GetUser(claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if !user.Active {
		return nil, errors.New("user account is deactivated")
	}

	return claims, nil
}

// Logout invalidates a user session
func (m *AuthManager) Logout(token string) error {
	return m.userStorage.DeleteSession(token)
}

// GetCurrentUser returns the current user from a token
func (m *AuthManager) GetCurrentUser(token string) (*User, error) {
	claims, err := m.ValidateToken(token)
	if err != nil {
		return nil, err
	}

	user, err := m.userStorage.GetUser(claims.UserID)
	if err != nil {
		return nil, err
	}

	return user.Sanitize(), nil
}

// UpdateUser updates user information
func (m *AuthManager) UpdateUser(userID string, update *UserUpdate, actingUserID string) error {
	// Get acting user to check permissions
	actingUser, err := m.userStorage.GetUser(actingUserID)
	if err != nil {
		return errors.New("unauthorized")
	}

	// Check permissions
	if userID != actingUserID && actingUser.Role != string(RoleAdmin) {
		return errors.New("insufficient permissions")
	}

	// If changing password, hash it
	if update.Password != nil {
		if err := ValidatePassword(*update.Password); err != nil {
			return fmt.Errorf("invalid password: %w", err)
		}

		hashedPassword, err := m.passwordHasher.HashPassword(*update.Password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		update.Password = &hashedPassword
	}

	// Only admins can change roles
	if update.Role != nil && actingUser.Role != string(RoleAdmin) {
		return errors.New("only administrators can change user roles")
	}

	return m.userStorage.UpdateUser(userID, update)
}

// DeleteUser deletes a user account
func (m *AuthManager) DeleteUser(userID string, actingUserID string) error {
	// Only admins can delete users
	actingUser, err := m.userStorage.GetUser(actingUserID)
	if err != nil {
		return errors.New("unauthorized")
	}

	if actingUser.Role != string(RoleAdmin) {
		return errors.New("only administrators can delete users")
	}

	// Cannot delete self
	if userID == actingUserID {
		return errors.New("cannot delete your own account")
	}

	// Delete user sessions
	if err := m.userStorage.DeleteUserSessions(userID); err != nil {
		log.Printf("Warning: failed to delete user sessions: %v", err)
	}

	return m.userStorage.DeleteUser(userID)
}

// CreateAPIKey creates a new API key for a user.
//
// The key inherits the permissions of the owner's role.
func (m *AuthManager) CreateAPIKey(userID, name string, expiresAt *time.Time) (*APIKey, string, error) {
	return m.CreateAPIKeyWithPermissions(userID, name, nil, expiresAt)
}

// CreateAPIKeyWithPermissions creates a new API key for a user with an
// explicit permission list.
//
// The plaintext key is generated here, hashed with SHA-256 and only
// the digest is stored. The plaintext is returned to the caller exactly
// once - there is no way to recover it from the store.
//
// An empty permission list means "inherit the permissions of the
// owner's role"; a non-empty list narrows the key down to exactly
// those scopes (e.g. "credentials:read-data").
func (m *AuthManager) CreateAPIKeyWithPermissions(userID, name string, permissions []string, expiresAt *time.Time) (*APIKey, string, error) {
	// Generate API key
	key, prefix, err := GenerateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the key for storage. API keys are high-entropy random
	// strings, so a fast digest is the right primitive here (bcrypt
	// would only slow the hot path down).
	hashedKey := HashAPIKey(key)

	apiKey := &APIKey{
		ID:          generateID("apik"),
		UserID:      userID,
		Name:        name,
		Key:         hashedKey,
		Prefix:      prefix,
		Permissions: append([]string(nil), permissions...),
		ExpiresAt:   expiresAt,
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.userStorage.SaveAPIKey(apiKey); err != nil {
		return nil, "", fmt.Errorf("failed to save API key: %w", err)
	}

	log.Printf("API key created for user %s: %s (%s)", userID, name, prefix)

	// Return the plain key (only shown once)
	return apiKey, key, nil
}

// ValidateAPIKey validates an API key and returns the user.
func (m *AuthManager) ValidateAPIKey(key string) (*User, error) {
	user, _, err := m.ValidateAPIKeyWithPermissions(key)
	return user, err
}

// ValidateAPIKeyWithPermissions validates an API key and returns the
// owning user plus the key record.
//
// The lookup goes through the key prefix (the first 8 characters of
// the plaintext key) and is confirmed with a constant-time comparison
// of the SHA-256 digests - the plaintext key is never stored and never
// compared in constant time against a stored secret.
func (m *AuthManager) ValidateAPIKeyWithPermissions(key string) (*User, *APIKey, error) {
	if len(key) < 8 {
		return nil, nil, errors.New("invalid API key format")
	}

	prefix := key[:8]
	apiKey, err := m.userStorage.GetAPIKeyByPrefix(prefix)
	if err != nil {
		// Fallback to the hash lookup (a key whose prefix index is
		// missing still authenticates when its digest is known).
		apiKey, err = m.userStorage.GetAPIKey(HashAPIKey(key))
		if err != nil {
			return nil, nil, errors.New("invalid API key")
		}
	}

	// Verify key
	if subtle.ConstantTimeCompare([]byte(HashAPIKey(key)), []byte(apiKey.Key)) != 1 {
		return nil, nil, errors.New("invalid API key")
	}

	// Check if active
	if !apiKey.Active {
		return nil, nil, errors.New("API key is inactive")
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, nil, errors.New("API key expired")
	}

	// Get user
	user, err := m.userStorage.GetUser(apiKey.UserID)
	if err != nil {
		return nil, nil, errors.New("user not found")
	}

	if !user.Active {
		return nil, nil, errors.New("user account is deactivated")
	}

	// Update last used timestamp
	now := time.Now()
	apiKey.LastUsed = &now
	if err := m.userStorage.SaveAPIKey(apiKey); err != nil {
		log.Printf("Warning: failed to update API key last used: %v", err)
	}

	return user, apiKey, nil
}

// PublicRegistrationOpen reports whether the public registration
// endpoint may still create an account.
//
// Public registration is only open while the instance has no users at
// all: the first account becomes the owner. Once an owner exists the
// endpoint is closed and further accounts can only be created by an
// admin (directly or through an invitation token).
func (m *AuthManager) PublicRegistrationOpen() (bool, error) {
	if !m.registrationOpen {
		return false, nil
	}

	users, err := m.userStorage.ListUsers()
	if err != nil {
		return false, err
	}

	return len(users) == 0, nil
}

// SetRegistrationOpen opens or closes the public registration endpoint.
// Closing it is what an operator does when the instance is bootstrapped
// with an owner account: the first-boot registration window closes.
func (m *AuthManager) SetRegistrationOpen(open bool) {
	m.registrationOpen = open
}

// BootstrapOwner creates the owner account of a fresh instance.
//
// It is called on every boot with the M9M_ADMIN_EMAIL /
// M9M_ADMIN_PASSWORD pair. The call is idempotent: an instance that
// already has users is never touched, so restarting the server never
// resets a password.
func (m *AuthManager) BootstrapOwner(email, password string) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return false, errors.New("email and password are required")
	}

	if err := ValidateEmail(email); err != nil {
		return false, fmt.Errorf("invalid admin email: %w", err)
	}
	if err := ValidatePassword(password); err != nil {
		return false, fmt.Errorf("invalid admin password: %w", err)
	}

	users, err := m.userStorage.ListUsers()
	if err != nil {
		return false, err
	}
	if len(users) > 0 {
		// An owner already exists - never overwrite it on restart.
		return false, nil
	}

	if _, err := m.userStorage.GetUserByEmail(email); err == nil {
		return false, nil
	}

	hashedPassword, err := m.passwordHasher.HashPassword(password)
	if err != nil {
		return false, fmt.Errorf("failed to hash the admin password: %w", err)
	}

	owner := &User{
		ID:        generateID("user"),
		Email:     email,
		Password:  hashedPassword,
		FirstName: "Owner",
		Role:      string(RoleAdmin),
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.userStorage.SaveUser(owner); err != nil {
		return false, fmt.Errorf("failed to save the owner account: %w", err)
	}

	log.Printf("Bootstrapped owner account %s (%s)", owner.Email, owner.ID)

	return true, nil
}

// ValidateSessionToken validates a bearer token AND requires a live
// session for it.
//
// The session lives in the user store, so it survives a restart of the
// server and a logout revokes the token immediately.
func (m *AuthManager) ValidateSessionToken(tokenString string) (*User, error) {
	claims, err := m.jwtManager.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	user, err := m.userStorage.GetUser(claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if !user.Active {
		return nil, errors.New("user account is deactivated")
	}

	if _, err := m.userStorage.GetSession(tokenString); err != nil {
		return nil, errors.New("session expired or revoked")
	}

	return user.Sanitize(), nil
}

// InviteUser creates an invitation token for a new user
func (m *AuthManager) InviteUser(email, role, invitedBy string) (*InviteToken, error) {
	// Validate email
	if err := ValidateEmail(email); err != nil {
		return nil, fmt.Errorf("invalid email: %w", err)
	}

	// Validate role
	if !UserRole(role).IsValid() {
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	// Check if user already exists
	if _, err := m.userStorage.GetUserByEmail(email); err == nil {
		return nil, fmt.Errorf("user with email %s already exists", email)
	}

	// Generate token
	token, err := GenerateRandomToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate invite token: %w", err)
	}

	invite := &InviteToken{
		ID:        generateID("inv"),
		Email:     email,
		Token:     token,
		Role:      role,
		InvitedBy: invitedBy,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7 days
		CreatedAt: time.Now(),
	}

	if err := m.userStorage.SaveInviteToken(invite); err != nil {
		return nil, fmt.Errorf("failed to save invite token: %w", err)
	}

	log.Printf("User invited: %s (role: %s, by: %s)", email, role, invitedBy)

	return invite, nil
}

// AcceptInvite accepts an invitation and creates a user account
func (m *AuthManager) AcceptInvite(token, password string) (*User, error) {
	// Get invite
	invite, err := m.userStorage.GetInviteToken(token)
	if err != nil {
		return nil, errors.New("invalid or expired invitation")
	}

	// Create user
	registration := &UserRegistration{
		Email:    invite.Email,
		Password: password,
	}

	user, err := m.Register(registration)
	if err != nil {
		return nil, err
	}

	// Update user role from invite
	roleStr := invite.Role
	update := &UserUpdate{
		Role: &roleStr,
	}
	if err := m.userStorage.UpdateUser(user.ID, update); err != nil {
		log.Printf("Warning: failed to update user role: %v", err)
	}

	// Mark invite as used
	now := time.Now()
	invite.UsedAt = &now
	if err := m.userStorage.SaveInviteToken(invite); err != nil {
		log.Printf("Warning: failed to update invite token: %v", err)
	}

	return user, nil
}

// Helper function to generate IDs
func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
