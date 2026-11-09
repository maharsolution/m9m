package messaging

// Regression tests for the Aiven / Confluent Cloud credential shape.
//
// Aiven stores the login as plain `username` + `password` in the
// credential (no `sasl` prefix), while the native transport reads
// `saslUsername` / `saslPassword`. Without the cross-fallback the
// SASL handshake goes out with an empty username and the broker
// replies "[58] SASL Authentication Failed: Invalid username or
// password" — which is exactly what the Aiven sample workflow hit.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestApplyKafkaCredentialAuth_BridgesUsernameToSASL asserts the
// REST-shaped credential fields (`username` / `password`) are
// projected onto the native SASL fields.
func TestApplyKafkaCredentialAuth_BridgesUsernameToSASL(t *testing.T) {
	p := map[string]interface{}{
		"kafka_username": "avnadmin",
		"kafka_password": "AVNS_secret",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "avnadmin", p["saslUsername"])
	assert.Equal(t, "AVNS_secret", p["saslPassword"])
}

// TestApplyKafkaCredentialAuth_BridgesSASLToUsername asserts the
// reverse direction, for credentials that only carry the
// `sasl`-prefixed keys but target a REST proxy.
func TestApplyKafkaCredentialAuth_BridgesSASLToUsername(t *testing.T) {
	p := map[string]interface{}{
		"kafka_saslUsername": "avnadmin",
		"kafka_saslPassword": "AVNS_secret",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "avnadmin", p["username"])
	assert.Equal(t, "AVNS_secret", p["password"])
}

// TestApplyKafkaCredentialAuth_ExplicitSASLUsernameWins asserts the
// cross-fallback never clobbers an explicit node parameter.
func TestApplyKafkaCredentialAuth_ExplicitSASLUsernameWins(t *testing.T) {
	p := map[string]interface{}{
		"kafka_username":     "cred-user",
		"saslUsername":       "node-user",
		"kafka_saslPassword": "cred-pass",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "node-user", p["saslUsername"])
	// No explicit saslPassword on the node, so the credential fills it.
	assert.Equal(t, "cred-pass", p["saslPassword"])
}

// TestApplyKafkaCredentialAuth_CarriesCAMaterial asserts the CA /
// mTLS material survives the folding, so the native dialer can build
// its TLS root pool.
func TestApplyKafkaCredentialAuth_CarriesCAMaterial(t *testing.T) {
	ca := "-----BEGIN CERTIFICATE-----\nMIIE...\n-----END CERTIFICATE-----"
	p := map[string]interface{}{
		"kafka_ca":         ca,
		"kafka_clientCert": "-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----",
		"kafka_clientKey":  "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, ca, p["ca"])
	assert.NotEmpty(t, p["clientCert"])
	assert.NotEmpty(t, p["clientKey"])
}
