package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/storage"
)

// mysqlRawKeyLimit is the length limit of the MySQL raw store key:
// `raw_data`.`key` is VARCHAR(255).
const mysqlRawKeyLimit = 255

// mysqlKeyLimitStorage wraps a storage backend and rejects raw keys that
// are longer than the MySQL column allows.
//
// It reproduces the production failure: the MySQL backend answers the
// INSERT of an over-long key with "Data too long for column 'key'", so
// the row never reaches the table.
type mysqlKeyLimitStorage struct {
	*storage.MemoryStorage
}

func (s *mysqlKeyLimitStorage) SaveRaw(key string, value []byte) error {
	if len(key) > mysqlRawKeyLimit {
		return fmt.Errorf("Error 1406 (22001): Data too long for column 'key' at row 1")
	}
	return s.MemoryStorage.SaveRaw(key, value)
}

// failingSessionStorage is a user store whose session writes always fail -
// the situation the MySQL backend ran into with the over-long JWT key.
type failingSessionStorage struct {
	*MemoryUserStorage
}

func (s *failingSessionStorage) SaveSession(session *Session) error {
	return errors.New("Error 1406 (22001): Data too long for column 'key' at row 1")
}

// TestPersistentUserStorage_LongJWTSessionRoundTrip asserts that a
// production-sized JWT (313+ characters) survives a round-trip through
// the persistent store.
//
// Before the fix the session was stored under the raw token, so the key
// was ~321 characters and the MySQL INSERT failed with "Data too long for
// column 'key'". The test fails on the old code and passes on the fixed
// code.
func TestPersistentUserStorage_LongJWTSessionRoundTrip(t *testing.T) {
	backend := &mysqlKeyLimitStorage{MemoryStorage: storage.NewMemoryStorage()}
	store := NewPersistentUserStorage(backend)

	jwtManager := NewJWTManager(persistenceTestSecret, time.Hour, "m9m")
	token, err := jwtManager.GenerateToken(&User{
		ID:    "user_1757700000000000000",
		Email: "admin@m9m.local",
		Role:  string(RoleAdmin),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(token), 313, "the test needs a production-sized JWT")

	// The old key format is what breaks MySQL.
	assert.Greater(t, len("session:"+token), mysqlRawKeyLimit,
		"the raw token does not fit into the MySQL raw key column")

	session := &Session{
		ID:        "sess_long_jwt",
		UserID:    "user_1757700000000000000",
		Token:     token,
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}

	require.NoError(t, store.SaveSession(session), "the session must be storable")

	got, err := store.GetSession(token)
	require.NoError(t, err, "the session must be readable again")
	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, session.UserID, got.UserID)
	assert.Equal(t, token, got.Token)
	assert.Equal(t, session.ExpiresAt.Unix(), got.ExpiresAt.Unix())
}

// TestSessionStorageKey_FitsIntoMySQLColumn asserts the properties of the
// new session key: it keeps the "session:" prefix (so the prefix scan of
// DeleteUserSessions still finds it) and it always fits into the MySQL
// raw key column.
func TestSessionStorageKey_FitsIntoMySQLColumn(t *testing.T) {
	tokens := []string{
		"short",
		strings.Repeat("a", 313),
		strings.Repeat("b", 4096),
	}

	for _, token := range tokens {
		key := sessionStorageKey(token)
		assert.True(t, strings.HasPrefix(key, "session:"), "the prefix must stay 'session:'")
		assert.LessOrEqual(t, len(key), mysqlRawKeyLimit, "the key must fit into the MySQL column")
		assert.NotContains(t, key, token, "the raw token must not be part of the key")
	}

	// The same token always maps to the same key, a different token to a
	// different one.
	assert.Equal(t, sessionStorageKey("token-a"), sessionStorageKey("token-a"))
	assert.NotEqual(t, sessionStorageKey("token-a"), sessionStorageKey("token-b"))
}

// TestPersistentUserStorage_DeleteUserSessionsWithDigestedKeys asserts
// that the prefix scan of DeleteUserSessions still finds the sessions
// after the key format changed.
func TestPersistentUserStorage_DeleteUserSessionsWithDigestedKeys(t *testing.T) {
	backend := &mysqlKeyLimitStorage{MemoryStorage: storage.NewMemoryStorage()}
	store := NewPersistentUserStorage(backend)

	sessions := []*Session{
		{ID: "s1", UserID: "user_x", Token: strings.Repeat("t", 313), ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s2", UserID: "user_y", Token: strings.Repeat("u", 313), ExpiresAt: time.Now().Add(time.Hour)},
	}
	for _, session := range sessions {
		require.NoError(t, store.SaveSession(session))
	}

	require.NoError(t, store.DeleteUserSessions("user_x"))

	_, err := store.GetSession(sessions[0].Token)
	assert.Error(t, err, "the session of the deleted user must be gone")

	got, err := store.GetSession(sessions[1].Token)
	require.NoError(t, err, "the session of another user must survive")
	assert.Equal(t, "user_y", got.UserID)

	// DeleteSession must use the same key function.
	require.NoError(t, store.DeleteSession(sessions[1].Token))
	_, err = store.GetSession(sessions[1].Token)
	assert.Error(t, err)
}

// TestAuthManager_Login_SessionPersistenceFailure asserts that a login
// which cannot store its session returns an error and NO token: a token
// without a session row is rejected by every later request.
func TestAuthManager_Login_SessionPersistenceFailure(t *testing.T) {
	store := &failingSessionStorage{MemoryUserStorage: NewMemoryUserStorage(storage.NewMemoryStorage())}
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	_, err := manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)

	token, user, err := manager.Login(&UserCredentials{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.Error(t, err, "a login without a stored session must fail")
	assert.Empty(t, token, "no token may be handed out")
	assert.Nil(t, user)
	assert.True(t, errors.Is(err, ErrSessionUnavailable), "the sentinel error must identify the failure")

	// The returned error stays generic - no storage detail in the answer.
	assert.NotContains(t, err.Error(), "raw_data")
	assert.NotContains(t, err.Error(), "Data too long")
}

// TestAuthHandler_Login_SessionPersistenceFailureIs500 asserts that the
// HTTP layer answers 500 when the session could not be persisted and
// 401 when the credentials are wrong.
func TestAuthHandler_Login_SessionPersistenceFailureIs500(t *testing.T) {
	store := &failingSessionStorage{MemoryUserStorage: NewMemoryUserStorage(storage.NewMemoryStorage())}
	manager := NewAuthManager(store, persistenceTestSecret, time.Hour)

	_, err := manager.Register(&UserRegistration{
		Email:    "owner@example.com",
		Password: persistenceTestPassword,
	})
	require.NoError(t, err)

	handler := NewAuthHandler(manager)

	t.Run("session persistence failure answers 500", func(t *testing.T) {
		body, err := json.Marshal(UserCredentials{
			Email:    "owner@example.com",
			Password: persistenceTestPassword,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
		rec := httptest.NewRecorder()
		handler.Login(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "raw_data", "no storage detail may leak")
		assert.NotContains(t, rec.Body.String(), "Data too long")
		assert.NotContains(t, rec.Body.String(), "token")
	})

	t.Run("wrong credentials answer 401", func(t *testing.T) {
		body, err := json.Marshal(UserCredentials{
			Email:    "owner@example.com",
			Password: "Wr0ng!Passw0rd",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
		rec := httptest.NewRecorder()
		handler.Login(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// TestValidateEmail_RejectsOverLongEmail asserts that an email which
// would push the raw key "user_email:<email>" beyond the MySQL column
// limit is rejected.
func TestValidateEmail_RejectsOverLongEmail(t *testing.T) {
	t.Run("over-long email is rejected", func(t *testing.T) {
		email := strings.Repeat("a", MaxEmailLength+1) + "@example.com"
		require.Greater(t, len(email), MaxEmailLength)

		err := ValidateEmail(email)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "240")
	})

	t.Run("email at the limit is accepted", func(t *testing.T) {
		email := strings.Repeat("a", MaxEmailLength-len("@example.com")) + "@example.com"
		require.Len(t, email, MaxEmailLength)

		assert.NoError(t, ValidateEmail(email))
	})
}
