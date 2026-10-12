package messaging

// Tests for the engine-injected kafka credential envelope. Covers
// BUG-2026-10-12-002 — the kafka producer + trigger nodes need to
// read the `<credType>_<field>` keys the engine writes from a
// synced `kafka` credential so that a workflow whose only auth lives
// in the credential envelope (no explicit `brokers` / `saslUsername`
// / etc. on the node) actually negotiates SASL with the broker.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestApplyKafkaCredentialAuth_NativeMode_FillsAllFields asserts the
// happy path: a params map with only the `kafka_*` injected keys
// ends up with the explicit-param keys populated, so the existing
// native-broker dispatcher code (which reads `brokers`,
// `saslMechanism`, etc.) keeps working unchanged.
func TestApplyKafkaCredentialAuth_NativeMode_FillsAllFields(t *testing.T) {
	p := map[string]interface{}{
		"kafka_brokers":       "broker1.example.com:9092,broker2.example.com:9092",
		"kafka_saslMechanism": "SCRAM-SHA-256",
		"kafka_saslUsername":  "alice",
		"kafka_saslPassword":  "s3cret",
		"kafka_tls":           "true",
		"kafka_clientId":      "m9m-finnet",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "broker1.example.com:9092,broker2.example.com:9092", p["brokers"])
	assert.Equal(t, "SCRAM-SHA-256", p["saslMechanism"])
	assert.Equal(t, "alice", p["saslUsername"])
	assert.Equal(t, "s3cret", p["saslPassword"])
	assert.Equal(t, "true", p["tls"])
	// `kafka_clientId` is not currently wired into the native config
	// builder; the helper should still leave it untouched for a future
	// pass to consume.
	assert.Equal(t, "m9m-finnet", p["kafka_clientId"])
}

// TestApplyKafkaCredentialAuth_RESTMode_FillsAllFields mirrors the
// native test for the REST-proxy path.
func TestApplyKafkaCredentialAuth_RESTMode_FillsAllFields(t *testing.T) {
	p := map[string]interface{}{
		"kafka_restProxyUrl": "http://kafka-rest:8082",
		"kafka_authentication": "basic",
		"kafka_username":     "alice",
		"kafka_password":     "s3cret",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "http://kafka-rest:8082", p["restProxyUrl"])
	assert.Equal(t, "basic", p["authentication"])
	assert.Equal(t, "alice", p["username"])
	assert.Equal(t, "s3cret", p["password"])
}

// TestApplyKafkaCredentialAuth_NoOverrideWhenExplicitParamSet
// asserts the back-compat path: an explicit `brokers` on the node
// wins over the credential envelope. The credential is only used to
// fill empty slots.
func TestApplyKafkaCredentialAuth_NoOverrideWhenExplicitParamSet(t *testing.T) {
	p := map[string]interface{}{
		"brokers":        "explicit.example.com:9092",
		"kafka_brokers":  "cred.example.com:9092",
		"saslUsername":   "explicit-user",
		"kafka_saslUsername": "cred-user",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "explicit.example.com:9092", p["brokers"], "explicit brokers should win")
	assert.Equal(t, "explicit-user", p["saslUsername"], "explicit username should win")
	// saslMechanism wasn't on the node, so the credential fills it.
	assert.Equal(t, "", p["saslMechanism"], "no credential value for saslMechanism here")
}

// TestApplyKafkaCredentialAuth_NilAndEmptyMap asserts the helper
// doesn't panic on a nil or empty map. Empty maps are a real path
// during dry-run validation before the engine injects credentials.
func TestApplyKafkaCredentialAuth_NilAndEmptyMap(t *testing.T) {
	assert.NotPanics(t, func() { ApplyKafkaCredentialAuth(nil) })
	assert.NotPanics(t, func() { ApplyKafkaCredentialAuth(map[string]interface{}{}) })
}

// TestApplyKafkaCredentialAuth_NoCredentials asserts a no-op when
// the engine did not inject any `kafka_*` keys (i.e. the workflow
// node has no credential envelope).
func TestApplyKafkaCredentialAuth_NoCredentials(t *testing.T) {
	p := map[string]interface{}{
		"brokers":       "standalone.example.com:9092",
		"saslMechanism": "PLAIN",
	}
	before := map[string]interface{}{
		"brokers":       p["brokers"],
		"saslMechanism": p["saslMechanism"],
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, before, p, "helper should not modify explicit-only params")
}

// TestApplyKafkaCredentialAuth_TrimsWhitespace asserts the helper
// trims surrounding whitespace on the credential envelope, which
// the engine-injected JSON path can introduce (e.g. from a UI
// credential form). Without the trim, the broker connect string
// would have a leading/trailing space and the segmentio client
// would reject it.
func TestApplyKafkaCredentialAuth_TrimsWhitespace(t *testing.T) {
	p := map[string]interface{}{
		"kafka_brokers":      "  broker1:9092  ",
		"kafka_saslUsername": "\t bob \n",
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "broker1:9092", p["brokers"])
	assert.Equal(t, "bob", p["saslUsername"])
}

// TestApplyKafkaCredentialAuth_HandlesNonStringTypes asserts the
// helper can also read numeric / bool credentials. The
// credential-injection path normally emits string fields, but
// tests and ad-hoc params (e.g. `kafka_tls` as a literal bool)
// flow through the same code path.
func TestApplyKafkaCredentialAuth_HandlesNonStringTypes(t *testing.T) {
	p := map[string]interface{}{
		"kafka_brokers": []interface{}{"broker1:9092", "broker2:9092"},
		"kafka_tls":     true,
	}
	ApplyKafkaCredentialAuth(p)
	// The slice-form brokers isn't currently consumed by the native
	// dispatcher (which splits a comma-separated string). We only
	// assert that the helper does not panic and that the bool
	// populates the `tls` slot.
	assert.Equal(t, "true", p["tls"])
}

// TestApplyKafkaCredentialAuth_PreservesKebabAndSpecialFields
// asserts the helper does not consume the credential-envelope
// "config" / "additional" / "ssl" fields that are not currently
// consumed by the dispatcher but should be preserved verbatim for
// future wiring.
func TestApplyKafkaCredentialAuth_PreservesUnconsumedFields(t *testing.T) {
	p := map[string]interface{}{
		"kafka_brokers":   "broker1:9092",
		"kafka_clientId":  "m9m-finnet",
		"kafka_partition": float64(3), // pretend the envelope carries this
	}
	ApplyKafkaCredentialAuth(p)
	assert.Equal(t, "m9m-finnet", p["kafka_clientId"], "unconsumed credential fields must be preserved")
	assert.Equal(t, float64(3), p["kafka_partition"], "unconsumed numeric credential fields must be preserved")
}

// TestIsEmpty covers the small helper used by the override policy.
// It's unit-tested in isolation so the rest of the table can stay
// focused on the integration.
func TestIsEmpty(t *testing.T) {
	assert.True(t, isEmpty(map[string]interface{}{}, "missing"))
	assert.True(t, isEmpty(map[string]interface{}{"k": nil}, "k"))
	assert.True(t, isEmpty(map[string]interface{}{"k": ""}, "k"))
	assert.True(t, isEmpty(map[string]interface{}{"k": "   "}, "k"))
	assert.False(t, isEmpty(map[string]interface{}{"k": "v"}, "k"))
	assert.False(t, isEmpty(map[string]interface{}{"k": false}, "k"), "false is not 'empty' — it's a real choice")
	assert.False(t, isEmpty(map[string]interface{}{"k": 0}, "k"), "0 is not 'empty'")
}
