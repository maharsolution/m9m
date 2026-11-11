package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------
// CredentialString
// ---------------------------------------------------------------

func TestCredentialString_FirstNonEmptyKeyWins(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": "sk-envelope",
		"apiKey":        "sk-legacy",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "sk-envelope", got, "the envelope key must win over the legacy key")
}

func TestCredentialString_SkipsEmptyString(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": "",
		"apiKey":        "sk-legacy",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "sk-legacy", got, "an empty envelope value must fall through to the legacy key")
}

func TestCredentialString_SkipsWhitespaceOnlyString(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": "   ",
		"apiKey":        "sk-legacy",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "sk-legacy", got, "a whitespace-only envelope value must count as absent")
}

func TestCredentialString_MissingKeyReturnsEmptyString(t *testing.T) {
	params := map[string]interface{}{
		"prompt": "hello",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "", got, "a missing key must yield an empty string")
}

func TestCredentialString_NilMapIsSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		got := CredentialString(nil, "openai_apiKey", "apiKey")
		assert.Equal(t, "", got)
	})
}

func TestCredentialString_NonStringValueIsIgnored(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": 42,
		"apiKey":        "sk-legacy",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "sk-legacy", got, "a non-string value must be ignored, never coerced")
}

func TestCredentialString_NilValueIsIgnored(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": nil,
		"apiKey":        "sk-legacy",
	}

	got := CredentialString(params, "openai_apiKey", "apiKey")
	assert.Equal(t, "sk-legacy", got)
}

func TestCredentialString_NoKeysIsSafe(t *testing.T) {
	params := map[string]interface{}{"apiKey": "sk-legacy"}
	assert.Equal(t, "", CredentialString(params))
}

func TestCredentialString_EmptyKeyIsSkipped(t *testing.T) {
	params := map[string]interface{}{
		"":       "should-not-be-used",
		"apiKey": "sk-legacy",
	}

	got := CredentialString(params, "", "apiKey")
	assert.Equal(t, "sk-legacy", got)
}

func TestCredentialString_ReturnsValueVerbatim(t *testing.T) {
	params := map[string]interface{}{
		"openai_apiKey": "sk-with-spaces ",
	}

	got := CredentialString(params, "openai_apiKey")
	assert.Equal(t, "sk-with-spaces ", got, "the value must be returned byte for byte")
}

// ---------------------------------------------------------------
// CredentialBool
// ---------------------------------------------------------------

func TestCredentialBool_ReadsEnvelopeString(t *testing.T) {
	params := map[string]interface{}{"redis_tls": "true"}
	assert.True(t, CredentialBool(params, "redis_tls"))
}

func TestCredentialBool_FalseEnvelopeString(t *testing.T) {
	params := map[string]interface{}{"redis_tls": "false", "tls": true}
	assert.False(t, CredentialBool(params, "redis_tls", "tls"),
		"an explicit false in the envelope must win over the legacy flag")
}

func TestCredentialBool_ReadsNativeBool(t *testing.T) {
	params := map[string]interface{}{"tls": true}
	assert.True(t, CredentialBool(params, "redis_tls", "tls"))
}

func TestCredentialBool_EmptyStringFallsThrough(t *testing.T) {
	params := map[string]interface{}{"redis_tls": "", "tls": true}
	assert.True(t, CredentialBool(params, "redis_tls", "tls"))
}

func TestCredentialBool_NilMapIsSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.False(t, CredentialBool(nil, "redis_tls", "tls"))
	})
}

func TestCredentialBool_MissingKeyIsFalse(t *testing.T) {
	params := map[string]interface{}{"operation": "get"}
	assert.False(t, CredentialBool(params, "redis_tls"))
}

// ---------------------------------------------------------------
// CredentialInt
// ---------------------------------------------------------------

func TestCredentialInt_ReadsNumericString(t *testing.T) {
	params := map[string]interface{}{"redis_port": "6380", "port": 6379}
	assert.Equal(t, 6380, CredentialInt(params, 6379, "redis_port", "port"))
}

func TestCredentialInt_ReadsNativeInt(t *testing.T) {
	params := map[string]interface{}{"port": 6381}
	assert.Equal(t, 6381, CredentialInt(params, 6379, "redis_port", "port"))
}

func TestCredentialInt_ReadsFloat(t *testing.T) {
	params := map[string]interface{}{"port": float64(6382)}
	assert.Equal(t, 6382, CredentialInt(params, 6379, "redis_port", "port"))
}

func TestCredentialInt_ZeroIsAValue(t *testing.T) {
	params := map[string]interface{}{"redis_db": "0", "db": 3}
	assert.Equal(t, 0, CredentialInt(params, 5, "redis_db", "db"),
		"a numeric zero must be returned, not treated as absent")
}

func TestCredentialInt_MissingKeyUsesFallback(t *testing.T) {
	params := map[string]interface{}{"operation": "get"}
	assert.Equal(t, 6379, CredentialInt(params, 6379, "redis_port", "port"))
}

func TestCredentialInt_NilMapUsesFallback(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.Equal(t, 6379, CredentialInt(nil, 6379, "redis_port"))
	})
}

func TestCredentialInt_NonNumericStringIsSkipped(t *testing.T) {
	params := map[string]interface{}{"redis_port": "not-a-port", "port": 6383}
	assert.Equal(t, 6383, CredentialInt(params, 6379, "redis_port", "port"))
}

// ---------------------------------------------------------------
// The helpers must never panic on hostile input
// ---------------------------------------------------------------

func TestCredentialHelpers_NeverPanic(t *testing.T) {
	hostile := map[string]interface{}{
		"a": nil,
		"b": []interface{}{"x"},
		"c": map[string]interface{}{"nested": true},
		"d": 0,
		"e": "",
	}

	require.NotPanics(t, func() {
		_ = CredentialString(hostile, "a", "b", "c", "d", "e", "missing")
		_ = CredentialBool(hostile, "a", "b", "c", "d", "e", "missing")
		_ = CredentialInt(hostile, 1, "a", "b", "c", "d", "e", "missing")
	})
}
