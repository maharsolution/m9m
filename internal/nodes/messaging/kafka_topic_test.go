package messaging

// Regression test for the producer's `topic` parameter.
//
// The topic can be an expression (`={{ $json.topic }}`). Before the
// fix the topic was read once, outside the per-item loop, and the
// raw expression string was sent to the broker — which created (or
// wrote to) a topic literally named "={{ $json.topic }}".

import (
	"testing"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaterialiseValue_TopicExpression asserts the topic expression
// resolves to the item's field.
func TestMaterialiseValue_TopicExpression(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"topic": "finnet-queue"}}

	got := materialiseValue("={{ $json.topic }}", item)
	topic, ok := got.(string)
	require.True(t, ok, "expected a string, got %T", got)
	assert.Equal(t, "finnet-queue", topic)
}

// TestMaterialiseValue_LiteralTopic asserts a literal topic passes
// through untouched.
func TestMaterialiseValue_LiteralTopic(t *testing.T) {
	item := model.DataItem{JSON: map[string]interface{}{"topic": "other"}}
	assert.Equal(t, "finnet-queue", materialiseValue("finnet-queue", item))
}
