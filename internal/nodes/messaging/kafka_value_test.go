package messaging

// Regression tests for the producer's `value` parameter resolution.
//
// `={{ $json }}` must forward the whole upstream item. Before the
// fix the path split produced [""] and the lookup returned nil, so
// the producer published the literal payload "null" to the topic.

import (
	"encoding/json"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kafkaTestItem() model.DataItem {
	return model.DataItem{JSON: map[string]interface{}{
		"marker": "HELLO",
		"n":      float64(7),
	}}
}

// TestMaterialiseValue_WholeJSON asserts `={{ $json }}` resolves to
// the complete item.
func TestMaterialiseValue_WholeJSON(t *testing.T) {
	got := materialiseValue("={{ $json }}", kafkaTestItem())
	require.NotNil(t, got, "whole-item expression must not resolve to nil")

	m, ok := got.(map[string]interface{})
	require.True(t, ok, "expected a map, got %T", got)
	assert.Equal(t, "HELLO", m["marker"])
	assert.Equal(t, float64(7), m["n"])
}

// TestMaterialiseValue_JSONField asserts `={{ $json.marker }}`
// resolves to the single field.
func TestMaterialiseValue_JSONField(t *testing.T) {
	got := materialiseValue("={{ $json.marker }}", kafkaTestItem())
	assert.Equal(t, "HELLO", got)
}

// TestResolveProduceValue_WholeJSON asserts the producer's `value`
// parameter with a whole-item expression resolves to the item's
// content (not to nil / "null"). `Execute` JSON-marshals a non
// string, non-[]byte value, so the map is the expected shape here.
func TestResolveProduceValue_WholeJSON(t *testing.T) {
	got := resolveProduceValue("={{ $json }}", kafkaTestItem())
	require.NotNil(t, got, "whole-item expression must not resolve to nil")

	m, ok := got.(map[string]interface{})
	require.True(t, ok, "expected a map, got %T", got)
	assert.Equal(t, "HELLO", m["marker"])

	// The bytes the broker actually receives.
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"marker":"HELLO","n":7}`, string(raw))
}

// TestResolveProduceValue_EmptyForwardsUpstream asserts an empty
// `value` forwards the upstream item (n8n parity).
func TestResolveProduceValue_EmptyForwardsUpstream(t *testing.T) {
	got := resolveProduceValue("", kafkaTestItem())
	b, ok := got.([]byte)
	require.True(t, ok, "expected []byte, got %T", got)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, "HELLO", decoded["marker"])
}

// TestMaterialiseValue_PlainLiteral asserts a literal string is
// passed through untouched.
func TestMaterialiseValue_PlainLiteral(t *testing.T) {
	assert.Equal(t, "plain-string", materialiseValue("plain-string", kafkaTestItem()))
}
