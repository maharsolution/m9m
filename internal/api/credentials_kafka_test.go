package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kafkaProbeStub replaces the broker metadata probe with a fake that
// mimics a broker: it only answers a metadata request when the SASL
// credentials are the expected ones. No live Kafka broker is needed.
type kafkaProbeStub struct {
	calls     []sasl.Mechanism
	wantUser  string
	wantPass  string
	failNames map[string]bool
	anonOK    bool
}

func (s *kafkaProbeStub) install(t *testing.T) {
	t.Helper()
	orig := kafkaMetadataFunc
	t.Cleanup(func() { kafkaMetadataFunc = orig })
	kafkaMetadataFunc = func(brokers []string, _ *tls.Config, mech sasl.Mechanism, user, password string) (int, error) {
		s.calls = append(s.calls, mech)
		if mech == nil {
			// An anonymous metadata request — the broker may or may not
			// allow it. The stub only allows it when asked to.
			if s.anonOK {
				return 3, nil
			}
			return 0, kafka.SASLAuthenticationFailed
		}
		if s.failNames[mech.Name()] {
			return 0, kafka.SASLAuthenticationFailed
		}
		if user != s.wantUser || password != s.wantPass {
			return 0, kafka.SASLAuthenticationFailed
		}
		return 3, nil
	}
}

// TestTestKafkaConnection_WrongUsernameReportsAuthenticationFailure is the
// regression test for the reported bug: a wrong username used to report
// "OK" while a wrong password correctly reported an error.
//
// Root cause: testKafkaConnection read the login from
// `dataString(data, "user")`, which only knows the generic alias map
// (`username` / `login` / `loginName`) and never the Kafka credential's
// own `saslUsername` / `saslPassword` keys. With those keys the tester
// saw an empty user, skipped the SASL handshake entirely and fell back
// to an unauthenticated metadata request the broker happens to allow —
// a misleading "OK". On top of that a stale alias key could shadow the
// username the operator had just edited.
func TestTestKafkaConnection_WrongUsernameReportsAuthenticationFailure(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "PLAIN",
		"saslUsername":  "mallory",
		"saslPassword":  "s3cret",
	})

	require.True(t, attempted, "the Kafka credential test must attempt a connection")
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "authentication failed")

	// The probe must have run with a SASL mechanism — never as an
	// anonymous metadata request.
	require.Len(t, stub.calls, 1)
	assert.NotNil(t, stub.calls[0], "the probe must not fall back to an unauthenticated request")
}

// TestTestKafkaConnection_WrongPasswordReportsAuthenticationFailure locks
// in the behaviour that already worked.
func TestTestKafkaConnection_WrongPasswordReportsAuthenticationFailure(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "PLAIN",
		"saslUsername":  "alice",
		"saslPassword":  "wrong-password",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "authentication failed")
	require.Len(t, stub.calls, 1)
	assert.NotNil(t, stub.calls[0])
}

// TestTestKafkaConnection_CanonicalUsernamePasswordAuthenticate asserts the
// canonical n8n-parity field names are honoured.
func TestTestKafkaConnection_CanonicalUsernamePasswordAuthenticate(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "PLAIN",
		"username":      "alice",
		"password":      "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1)
	pm, ok := stub.calls[0].(plain.Mechanism)
	require.True(t, ok)
	assert.Equal(t, "alice", pm.Username)
	assert.Equal(t, "s3cret", pm.Password)
}

// TestTestKafkaConnection_LegacySASLAliasesAuthenticate asserts the legacy
// `saslUsername` / `saslPassword` spellings still work for credentials
// stored by an older build.
func TestTestKafkaConnection_LegacySASLAliasesAuthenticate(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "PLAIN",
		"saslUsername":  "alice",
		"saslPassword":  "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1)
	pm, ok := stub.calls[0].(plain.Mechanism)
	require.True(t, ok)
	assert.Equal(t, "alice", pm.Username)
	assert.Equal(t, "s3cret", pm.Password)
}

// TestTestKafkaConnection_CanonicalUsernameWinsOverStaleAlias asserts a
// stale alias key left over from an earlier save cannot shadow the
// username the operator just edited.
func TestTestKafkaConnection_CanonicalUsernameWinsOverStaleAlias(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":      "broker.example.com:9092",
		"username":     "mallory",
		"saslUsername": "alice",
		"password":     "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "authentication failed")
}

// TestTestKafkaConnection_MissingUsernameIsReportedExplicitly asserts the
// tester never falls back to an unauthenticated probe when SASL is
// configured but no username is stored.
func TestTestKafkaConnection_MissingUsernameIsReportedExplicitly(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "PLAIN",
		"saslPassword":  "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "username")
	assert.Empty(t, stub.calls, "no probe may run when the username is missing")
}

// TestTestKafkaConnection_SelectedMechanismIsUsedExclusively asserts the
// mechanism picked by the operator is the only one tried.
func TestTestKafkaConnection_SelectedMechanismIsUsedExclusively(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "SCRAM-SHA-256",
		"username":      "alice",
		"password":      "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1, "an explicitly selected mechanism must not be mixed with the others")
	assert.Equal(t, "SCRAM-SHA-256", stub.calls[0].Name())
}

// TestTestKafkaConnection_EmptyMechanismFallsBackPlainThenScram asserts the
// documented fallback order when no mechanism is configured.
func TestTestKafkaConnection_EmptyMechanismFallsBackPlainThenScram(t *testing.T) {
	stub := &kafkaProbeStub{
		wantUser:  "alice",
		wantPass:  "s3cret",
		failNames: map[string]bool{"PLAIN": true},
	}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":  "broker.example.com:9092",
		"username": "alice",
		"password": "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 2)
	assert.Equal(t, "PLAIN", stub.calls[0].Name())
	assert.Equal(t, "SCRAM-SHA-256", stub.calls[1].Name())
}

// TestTestKafkaConnection_N8nLowercaseMechanismIsAccepted asserts the
// lowercase mechanism spelling n8n stores is understood.
func TestTestKafkaConnection_N8nLowercaseMechanismIsAccepted(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":       "broker.example.com:9092",
		"saslMechanism": "plain",
		"username":      "alice",
		"password":      "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1)
	assert.Equal(t, "PLAIN", stub.calls[0].Name())
}

// TestTestKafkaConnection_SyncedN8nCredentialUsesNativeSASL asserts a
// credential synced from n8n (`authentication` is a boolean there) keeps
// using the native SASL path.
func TestTestKafkaConnection_SyncedN8nCredentialUsesNativeSASL(t *testing.T) {
	stub := &kafkaProbeStub{wantUser: "alice", wantPass: "s3cret"}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":        "broker.example.com:9092",
		"authentication": true,
		"username":       "alice",
		"password":       "s3cret",
		"saslMechanism":  "plain",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1)
	assert.Equal(t, "PLAIN", stub.calls[0].Name())
}

// TestTestKafkaConnection_AnonymousProbeOnlyWithoutCredentials asserts the
// unauthenticated metadata request is still used when the credential
// stores no login at all.
func TestTestKafkaConnection_AnonymousProbeOnlyWithoutCredentials(t *testing.T) {
	stub := &kafkaProbeStub{anonOK: true}
	stub.install(t)

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers": "broker.example.com:9092",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
	require.Len(t, stub.calls, 1)
	assert.Nil(t, stub.calls[0])
}

// TestTestKafkaConnection_ErrorMessageDoesNotLeakSecrets asserts the
// username and the password are scrubbed from the error message.
func TestTestKafkaConnection_ErrorMessageDoesNotLeakSecrets(t *testing.T) {
	orig := kafkaMetadataFunc
	t.Cleanup(func() { kafkaMetadataFunc = orig })
	kafkaMetadataFunc = func(_ []string, _ *tls.Config, _ sasl.Mechanism, _ string, _ string) (int, error) {
		return 0, fmt.Errorf("SASL authentication failed: rejected login alice with password s3cret")
	}

	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":  "broker.example.com:9092",
		"username": "alice",
		"password": "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.NotContains(t, res.Message, "s3cret", "the password must be scrubbed")
	assert.NotContains(t, res.Message, "alice", "the username must be scrubbed")
}

// newKafkaRESTProxy starts a stub REST Proxy that only answers with 200
// when the expected Basic auth credentials are supplied.
func newKafkaRESTProxy(t *testing.T, wantUser, wantPass string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != wantUser || pass != wantPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`["topic-a"]`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestTestKafkaConnection_RESTProxyRejectsWrongUsername asserts the REST
// proxy path reports a wrong username as an authentication failure.
func TestTestKafkaConnection_RESTProxyRejectsWrongUsername(t *testing.T) {
	srv := newKafkaRESTProxy(t, "alice", "s3cret")

	res, attempted := testKafkaConnection(map[string]interface{}{
		"restProxyUrl":   srv.URL,
		"authentication": "basic",
		"username":       "mallory",
		"password":       "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "authentication failed")
}

// TestTestKafkaConnection_RESTProxyRejectsWrongPassword asserts the REST
// proxy path reports a wrong password as an authentication failure.
func TestTestKafkaConnection_RESTProxyRejectsWrongPassword(t *testing.T) {
	srv := newKafkaRESTProxy(t, "alice", "s3cret")

	res, attempted := testKafkaConnection(map[string]interface{}{
		"restProxyUrl":   srv.URL,
		"authentication": "basic",
		"username":       "alice",
		"password":       "wrong-password",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "authentication failed")
}

// TestTestKafkaConnection_RESTProxyAcceptsCorrectCredentials asserts the
// REST proxy path succeeds with the right login.
func TestTestKafkaConnection_RESTProxyAcceptsCorrectCredentials(t *testing.T) {
	srv := newKafkaRESTProxy(t, "alice", "s3cret")

	res, attempted := testKafkaConnection(map[string]interface{}{
		"restProxyUrl":   srv.URL,
		"authentication": "basic",
		"username":       "alice",
		"password":       "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
}

// TestTestKafkaConnection_RESTProxyAcceptsLegacySASLAliases asserts a
// credential that only stores the legacy `saslUsername` / `saslPassword`
// still authenticates against the REST proxy.
func TestTestKafkaConnection_RESTProxyAcceptsLegacySASLAliases(t *testing.T) {
	srv := newKafkaRESTProxy(t, "alice", "s3cret")

	res, attempted := testKafkaConnection(map[string]interface{}{
		"restProxyUrl":   srv.URL,
		"authentication": "basic",
		"saslUsername":   "alice",
		"saslPassword":   "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "OK", res.Status)
}

// TestTestKafkaConnection_RESTProxyRequiresURL asserts an explicit Basic
// auth mode without a REST Proxy URL is reported instead of silently
// falling back to the native path.
func TestTestKafkaConnection_RESTProxyRequiresURL(t *testing.T) {
	res, attempted := testKafkaConnection(map[string]interface{}{
		"brokers":        "broker.example.com:9092",
		"authentication": "basic",
		"username":       "alice",
		"password":       "s3cret",
	})

	require.True(t, attempted)
	assert.Equal(t, "Error", res.Status)
	assert.Contains(t, strings.ToLower(res.Message), "restproxyurl")
}

// TestKafkaCredentialUserPass_Resolution asserts the alias resolution of
// the single username / password pair.
func TestKafkaCredentialUserPass_Resolution(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		wantUser string
		wantPass string
	}{
		{
			name:     "canonical keys",
			data:     map[string]interface{}{"username": "alice", "password": "s3cret"},
			wantUser: "alice",
			wantPass: "s3cret",
		},
		{
			name:     "legacy sasl keys",
			data:     map[string]interface{}{"saslUsername": "alice", "saslPassword": "s3cret"},
			wantUser: "alice",
			wantPass: "s3cret",
		},
		{
			name:     "legacy user alias",
			data:     map[string]interface{}{"user": "alice", "password": "s3cret"},
			wantUser: "alice",
			wantPass: "s3cret",
		},
		{
			name:     "canonical wins over stale alias",
			data:     map[string]interface{}{"username": "mallory", "saslUsername": "alice", "password": "s3cret"},
			wantUser: "mallory",
			wantPass: "s3cret",
		},
		{
			name:     "password alias fallback",
			data:     map[string]interface{}{"username": "alice", "saslPassword": "s3cret"},
			wantUser: "alice",
			wantPass: "s3cret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pass := kafkaCredentialUserPass(tt.data)
			assert.Equal(t, tt.wantUser, user)
			assert.Equal(t, tt.wantPass, pass)
		})
	}
}

// TestKafkaCredentialSchema_DeclaresExactlyOneUsernameAndOnePassword is the
// schema-shape regression test: the duplicated username / password
// properties must not come back.
func TestKafkaCredentialSchema_DeclaresExactlyOneUsernameAndOnePassword(t *testing.T) {
	schema, ok := credentialSchemas["kafka"]
	require.True(t, ok, "the kafka credential schema must exist")

	var userProps, passProps []string
	for _, p := range schema.Properties {
		switch p.Name {
		case "username", "saslUsername", "user", "login", "loginName":
			userProps = append(userProps, p.Name)
		case "password", "saslPassword", "pass", "pwd":
			passProps = append(passProps, p.Name)
		}
	}
	assert.Equal(t, []string{"username"}, userProps,
		"the Kafka schema must declare exactly one username property")
	assert.Equal(t, []string{"password"}, passProps,
		"the Kafka schema must declare exactly one password property")
}

// TestKafkaCredentialSchema_KeepsAuthSelectors asserts the remaining auth
// selectors stay coherent after the de-duplication.
func TestKafkaCredentialSchema_KeepsAuthSelectors(t *testing.T) {
	schema, ok := credentialSchemas["kafka"]
	require.True(t, ok)

	names := map[string]SchemaProperty{}
	for _, p := range schema.Properties {
		names[p.Name] = p
	}
	require.Contains(t, names, "saslMechanism")
	require.Contains(t, names, "authentication")
	require.Contains(t, names, "apiKey")
	assert.NotContains(t, names, "saslUsername")
	assert.NotContains(t, names, "saslPassword")
	assert.Equal(t, "password", names["password"].Type)
	assert.Equal(t, "string", names["username"].Type)
}
