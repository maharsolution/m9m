package messaging

// Regression tests for the producer's transport cache and the native
// writer's topic binding.
//
// Two bugs are covered:
//
//  1. The producer node's transport cache was a single slot shared
//     by the whole engine process. The first workflow to produce
//     anything pinned the transport, so a later workflow with a
//     different topic published to the FIRST workflow's topic while
//     the node reported the requested one.
//
//  2. The native `kafka.Writer` was created with a fixed `Topic`,
//     and kafka-go treats `Writer.Topic` and `Message.Topic` as
//     mutually exclusive — the writer's topic always won. A node
//     whose topic is an expression (or changes between runs) wrote
//     to the wrong topic with no error.

import (
	"context"
	"testing"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackendFor_CachesPerConfiguration asserts two different
// configurations get two different transports (the single-slot
// cache used to hand the second caller the first caller's client).
func TestBackendFor_CachesPerConfiguration(t *testing.T) {
	n := NewKafkaNode()

	first, err := n.backendFor(map[string]interface{}{
		"mode":     "native",
		"brokers":  "broker-a:9092",
		"topic":    "topic-a",
		"tls":      true,
		"ca":       "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----",
		"username": "u1",
		"password": "p1",
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := n.backendFor(map[string]interface{}{
		"mode":     "native",
		"brokers":  "broker-b:9092",
		"topic":    "topic-b",
		"tls":      true,
		"ca":       "-----BEGIN CERTIFICATE-----\nBBB\n-----END CERTIFICATE-----",
		"username": "u2",
		"password": "p2",
	})
	require.NoError(t, err)
	require.NotNil(t, second)

	assert.NotSame(t, first, second,
		"a different broker/credential set must not reuse the previous transport")

	// The same configuration must still hit the cache (the reuse
	// optimisation the cache exists for).
	again, err := n.backendFor(map[string]interface{}{
		"mode":     "native",
		"brokers":  "broker-a:9092",
		"topic":    "topic-a",
		"tls":      true,
		"ca":       "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----",
		"username": "u1",
		"password": "p1",
	})
	require.NoError(t, err)
	assert.Same(t, first, again, "identical config should reuse the cached transport")
}

// TestBackendFor_TopicNotPartOfCacheKey asserts that only the
// topic differs, the transport is shared — the native writer is
// topic-agnostic because the topic travels with each message.
func TestBackendFor_TopicNotPartOfCacheKey(t *testing.T) {
	n := NewKafkaNode()

	base := map[string]interface{}{
		"mode":     "native",
		"brokers":  "broker-a:9092",
		"tls":      true,
		"ca":       "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----",
		"username": "u1",
		"password": "p1",
	}

	withTopicA := map[string]interface{}{}
	for k, v := range base {
		withTopicA[k] = v
	}
	withTopicA["topic"] = "topic-a"

	withTopicB := map[string]interface{}{}
	for k, v := range base {
		withTopicB[k] = v
	}
	withTopicB["topic"] = "topic-b"

	first, err := n.backendFor(withTopicA)
	require.NoError(t, err)
	second, err := n.backendFor(withTopicB)
	require.NoError(t, err)

	assert.Same(t, first, second,
		"a topic-only difference must not spawn a second transport")
}

// TestNativeProduce_TopicTravelsWithMessage asserts the producer
// reports the topic it actually published to, and that the native
// client accepts a per-call topic (the writer no longer pins one).
func TestNativeProduce_TopicTravelsWithMessage(t *testing.T) {
	// A fake backend records the topic it was asked to publish to.
	rec := &recordingBackend{}
	n := &KafkaNode{backends: map[string]kafkaBackend{
		backendCacheKey("native", map[string]interface{}{"brokers": "b:9092"}): rec,
	}}

	out, err := n.Execute(
		[]model.DataItem{{JSON: map[string]interface{}{"v": 1}}},
		map[string]interface{}{
			"mode":    "native",
			"brokers": "b:9092",
			"topic":   "the-real-topic",
		},
	)
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.Equal(t, []string{"the-real-topic"}, rec.topics,
		"the topic handed to the broker must be the one requested")
	assert.Equal(t, "the-real-topic", out[0].JSON["topic"],
		"the reported topic must match the topic actually published to")
}

// recordingBackend is a kafkaBackend stub that records the topics
// Produce was called with.
type recordingBackend struct {
	topics []string
}

func (r *recordingBackend) Produce(ctx context.Context, topic string, key, value []byte, headers map[string]string) (kafkaProduceResult, error) {
	r.topics = append(r.topics, topic)
	return kafkaProduceResult{Topic: topic, Partition: 0, Offset: 1}, nil
}

func (r *recordingBackend) Poll(ctx context.Context, timeout time.Duration, maxRecords int) ([]KafkaRecord, error) {
	return nil, nil
}

func (r *recordingBackend) Close() error { return nil }
