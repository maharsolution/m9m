package messaging

import (
	"encoding/json"
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
)

// TestResolveProduceValue_NilValueReturnsUpstreamJSON covers the
// n8n parity case where the operator leaves the producer's `value`
// field empty: m9m should forward the entire upstream item to the
// topic, not send an empty record. Without this fallback, a synced
// workflow that relied on the empty-value-as-upstream convention
// (the user's "Finnet - Webhook use Redist - Kafka/Rabbit" on the
// `finnet-queue` node) would publish a zero-length Kafka record and
// the consumer-side `Set Counter` / `Find Respond` retry loop
// would spin on a missing key.
func TestResolveProduceValue_NilValueReturnsUpstreamJSON(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"id": "C1119", "balance": 10000}}
	got := resolveProduceValue(nil, item)
	b, ok := got.([]byte)
	assert.True(t, ok, "resolveProduceValue(nil, item) should return []byte, got %T", got)
	var back map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, "C1119", back["id"])
	assert.Equal(t, float64(10000), back["balance"])
}

// TestResolveProduceValue_EmptyStringValueReturnsUpstreamJSON
// covers the same case but with `value: ""` (literal empty
// string) instead of nil. The n8n UI emits an empty string when
// the operator leaves the field blank; a synced workflow therefore
// arrives at m9m with `value: ""`, not `value: nil`. Both must
// trigger the upstream-forward fallback.
func TestResolveProduceValue_EmptyStringValueReturnsUpstreamJSON(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"nama": "erwan"}}
	got := resolveProduceValue("", item)
	b, ok := got.([]byte)
	assert.True(t, ok, "resolveProduceValue(\"\", item) should return []byte, got %T", got)
	var back map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, "erwan", back["nama"])
}

// TestResolveProduceValue_WhitespaceStringReturnsUpstreamJSON
// guards against a UI that emits a single space (or similar) when
// the operator saves the node with an untouched value field.
func TestResolveProduceValue_WhitespaceStringReturnsUpstreamJSON(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"k": "v"}}
	got := resolveProduceValue("   ", item)
	_, ok := got.([]byte)
	assert.True(t, ok, "whitespace value should also trigger the upstream fallback, got %T", got)
}

// TestResolveProduceValue_ExpressionEvaluatesAgainstItem covers
// the case where the operator does set a value, but as an n8n
// expression `={{ $json.x }}` referencing the upstream item.
// This is the case `materialiseValue` already handled before
// this fix; verify the new helper still honours it.
func TestResolveProduceValue_ExpressionEvaluatesAgainstItem(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"name": "erwan"}}
	got := resolveProduceValue("={{ $json.name }}", item)
	assert.Equal(t, "erwan", got, "expression value should still resolve to the upstream field")
}

// TestResolveProduceValue_LiteralStringPassesThrough covers the
// case where the operator sets a literal string value. m9m should
// not silently overwrite it with the upstream item; that would
// change the wire payload the broker sees.
func TestResolveProduceValue_LiteralStringPassesThrough(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"id": "C1119"}}
	got := resolveProduceValue("hello world", item)
	assert.Equal(t, "hello world", got, "literal value should pass through unchanged")
}

// TestResolveProduceValue_NonStringLiteralPassesThrough covers
// the case where the operator sets a non-string value (e.g. a
// JSON number or bool). m9m should not coerce; the producer
// marshals it according to its type and ships the bytes to the
// broker.
func TestResolveProduceValue_NonStringLiteralPassesThrough(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"id": "C1119"}}
	got := resolveProduceValue(float64(42), item)
	assert.Equal(t, float64(42), got)
}
