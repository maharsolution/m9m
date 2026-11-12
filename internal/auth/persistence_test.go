package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/storage"
)

// persistenceTestSecret is the JWT secret of the test instances. It is
// a throwaway value that only exists inside the test process.
const persistenceTestSecret = "m9m-test-jwt-secret-value-at-least-32-chars"

// persistenceTestPassword is the password of the test accounts. It
// never leaves the test process.
const persistenceTestPassword = "Str0ng!TestPassw0rd"

// TestPersistentUserStorage_SurvivesReopen asserts that the users, the
// API keys and the sessions live in the storage backend and are still
// there after the store is reopened - which is what a restart of m9m
// does.
func TestPersistentUserStorage_SurvivesReopen(t *testing.T) {
	backend := storage.NewMemoryStorage()

	// First "boot": the owner registers and logs in.
	first := NewPersistentUserStorage(backend)
	manager := NewAuthManager(first, persistenceTestSecret, time.Hour)

	user, err := manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, string(RoleAdmin), user.Role, "the first account is the owner")

	token, _, err := manager.Login(&UserCredentials{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	_, plainKey, err := manager.CreateAPIKeyWithPermissions(user.ID, "automation", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plainKey)

	// Second "boot": a fresh storage handle over the same backend.
	second := NewPersistentUserStorage(backend)
	reopened := NewAuthManager(second, persistenceTestSecret, time.Hour)

	users, err := reopened.userStorage.ListUsers()
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "owner@example.com", users[0].Email)

	// The session token still authenticates.
	sessionUser, err := reopened.ValidateSessionToken(token)
	require.NoError(t, err, "the session must survive a restart")
	assert.Equal(t, user.ID, sessionUser.ID)

	// The API key still authenticates.
	keyUser, err := reopened.ValidateAPIKey(plainKey)
	require.NoError(t, err, "the API key must survive a restart")
	assert.Equal(t, user.ID, keyUser.ID)
}

// TestPersistentUserStorage_RevokedKeyIsRejected asserts that a
// revoked key stops working immediately.
func TestPersistentUserStorage_RevokedKeyIsRejected(t *testing.T) {
	backend := storage.NewMemoryStorage()
	store := NewPersistentUserStorage(backend)
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	user, err := manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)

	apiKey, plainKey, err := manager.CreateAPIKeyWithPermissions(user.ID, "automation", nil, nil)
	require.NoError(t, err)

	_, _, err = manager.ValidateAPIKeyWithPermissions(plainKey)
	require.NoError(t, err)

	// Revoke it.
	require.NoError(t, store.DeleteAPIKey(apiKey.ID))

	_, _, err = manager.ValidateAPIKeyWithPermissions(plainKey)
	assert.Error(t, err, "a revoked key must be rejected")
}

// TestPersistentUserStorage_FirstRegisterClosesTheWindow asserts that
// the public registration window accepts exactly one account.
func TestPersistentUserStorage_FirstRegisterClosesTheWindow(t *testing.T) {
	backend := storage.NewMemoryStorage()
	store := NewPersistentUserStorage(backend)
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	open, err := manager.PublicRegistrationOpen()
	require.NoError(t, err)
	assert.True(t, open, "a fresh instance accepts the first registration")

	_, err = manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)

	open, err = manager.PublicRegistrationOpen()
	require.NoError(t, err)
	assert.False(t, open, "the window closes after the first account")
}

// TestPersistentUserStorage_BootstrapOwnerIsIdempotent asserts that a
// restart never overwrites the owner account.
func TestPersistentUserStorage_BootstrapOwnerIsIdempotent(t *testing.T) {
	backend := storage.NewMemoryStorage()
	store := NewPersistentUserStorage(backend)
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	created, err := manager.BootstrapOwner("owner@example.com", persistenceTestPassword)
	require.NoError(t, err)
	assert.True(t, created)

	// A second boot with the same credentials must not touch the
	// account.
	created, err = manager.BootstrapOwner("owner@example.com", persistenceTestPassword)
	require.NoError(t, err)
	assert.False(t, created)

	users, err := store.ListUsers()
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, string(RoleAdmin), users[0].Role)
}

// TestPersistentUserStorage_APIKeyIsHashed asserts that the plaintext
// key never reaches the store.
func TestPersistentUserStorage_APIKeyIsHashed(t *testing.T) {
	backend := storage.NewMemoryStorage()
	store := NewPersistentUserStorage(backend)
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	user, err := manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)

	apiKey, plainKey, err := manager.CreateAPIKeyWithPermissions(user.ID, "automation", nil, nil)
	require.NoError(t, err)

	// The stored record holds the digest, never the plaintext.
	assert.Equal(t, HashAPIKey(plainKey), apiKey.Key)
	assert.NotEqual(t, plainKey, apiKey.Key)

	stored, err := store.GetAPIKey(HashAPIKey(plainKey))
	require.NoError(t, err)
	assert.Equal(t, HashAPIKey(plainKey), stored.Key)
	assert.NotEqual(t, plainKey, stored.Key)
}
