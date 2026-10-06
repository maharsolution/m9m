package messaging

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// KafkaNativeConfig captures the per-node native-broker wiring.
// It is built from the producer/consumer node parameters when the
// operator selects `mode: native` on the Kafka node. All fields are
// optional except `Brokers`; SASL/TLS are off by default to match
// plain-text dev clusters.
//
// Exported so the `trigger.KafkaTriggerNode` can construct / cache
// a client without duplicating the field set.
type KafkaNativeConfig struct {
	Brokers       []string // bootstrap servers, e.g. ["broker1:9092","broker2:9092"]
	Topic         string
	GroupID       string // only used by the consumer
	SASLMechanism string // "PLAIN" | "SCRAM-SHA-256" | "SCRAM-SHA-512" | ""
	SASLUsername  string
	SASLPassword  string
	TLS           bool   // enable TLS dial
	ClientID      string // optional, surfaced in broker logs
}

// KafkaNativeClient is the segmentio/kafka-go-backed transport for
// the `n8n-nodes-base.kafka` (producer) and
// `n8n-nodes-base.kafkaTrigger` (consumer) nodes. It implements
// the same `kafkaBackend` contract as the REST proxy client so the
// node-level code does not need to branch on transport mode.
//
// One client is created per `Execute` / `poll` call (the
// `kafkaBackend` cache is shared per `mode+brokers+topic+...` key,
// so a workflow that produces 1k items reuses the same Writer
// instead of paying a TCP handshake on every record).
type KafkaNativeClient struct {
	cfg KafkaNativeConfig

	mu     sync.Mutex
	writer *kafka.Writer // producer-side, lazily initialised
	reader *kafka.Reader // consumer-side, lazily initialised
	closed bool
}

// NewKafkaNativeClient constructs a new native Kafka client. It
// validates the configuration eagerly (brokers non-empty, topic
// non-empty) but defers the actual TCP / SASL handshake until the
// first Produce / Poll call.
func NewKafkaNativeClient(cfg KafkaNativeConfig) (*KafkaNativeClient, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka native: at least one broker is required")
	}
	if cfg.Topic == "" {
		return nil, errors.New("kafka native: topic is required")
	}
	for _, b := range cfg.Brokers {
		if b == "" {
			return nil, errors.New("kafka native: empty broker address")
		}
	}
	return &KafkaNativeClient{cfg: cfg}, nil
}

// dialer returns a *kafka.Dialer with the SASL / TLS settings from
// the config. Used both for the initial metadata handshake and for
// each transport connect.
func (c *KafkaNativeClient) dialer() (*kafka.Dialer, error) {
	d := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}
	if c.cfg.ClientID != "" {
		d.ClientID = c.cfg.ClientID
	}

	mech := strings.ToUpper(strings.TrimSpace(c.cfg.SASLMechanism))
	if mech != "" {
		if c.cfg.SASLUsername == "" {
			return nil, errors.New("kafka native: SASL username is required when SASL mechanism is set")
		}
		var saslMech sasl.Mechanism
		switch mech {
		case "PLAIN":
			saslMech = plain.Mechanism{Username: c.cfg.SASLUsername, Password: c.cfg.SASLPassword}
		case "SCRAM-SHA-256":
			saslMech, err = scram.Mechanism(scram.SHA256, c.cfg.SASLUsername, c.cfg.SASLPassword)
			if err != nil {
				return nil, fmt.Errorf("kafka native: scram-sha-256 init: %w", err)
			}
		case "SCRAM-SHA-512":
			saslMech, err = scram.Mechanism(scram.SHA512, c.cfg.SASLUsername, c.cfg.SASLPassword)
			if err != nil {
				return nil, fmt.Errorf("kafka native: scram-sha-512 init: %w", err)
			}
		default:
			return nil, fmt.Errorf("kafka native: unsupported SASL mechanism %q (supported: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)", c.cfg.SASLMechanism)
		}
		d.SASLMechanism = saslMech
	}

	if c.cfg.TLS {
		d.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return d, nil
}

// writerLazy returns (creating on first call) the producer-side
// Writer. The Writer owns the broker connection pool and balances
// produce requests across the partition leaders automatically.
func (c *KafkaNativeClient) writerLazy() (*kafka.Writer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writer != nil {
		return c.writer, nil
	}
	d, err := c.dialer()
	if err != nil {
		return nil, err
	}
	w := &kafka.Writer{
		Addr:                   kafka.TCP(c.cfg.Brokers...),
		Topic:                  c.cfg.Topic,
		// keyBasedBalancer routes by key when Key is non-empty
		// (preserves partitioning for ordered processing) and
		// round-robins when Key is empty. Avoiding
		// `kafka.Hash{}` here because Hash panics on an empty
		// key, which trips up workflows that don't supply one.
		Balancer:               keyBasedBalancer{},
		RequiredAcks:           kafka.RequireOne,
		Async:                  false, // synchronous; surface broker errors to the workflow
		AllowAutoTopicCreation: true,
		WriteTimeout:           10 * time.Second,
		ReadTimeout:            10 * time.Second,
		Transport:              &kafka.Transport{Dialer: d, TLS: d.TLS},
	}
	c.writer = w
	return w, nil
}

// Produce writes one record to Kafka via the segmentio/kafka-go
// Writer. The native transport delivers the same `kafkaProduceResult`
// shape as the REST proxy so the node-level result rendering is
// unchanged.
func (c *KafkaNativeClient) Produce(ctx context.Context, topic string, key, value []byte, headers map[string]string) (kafkaProduceResult, error) {
	if topic != "" && topic != c.cfg.Topic {
		// The native Writer is bound to a single topic at
		// construction. A node that targets multiple topics should
		// either use the REST proxy or open one Writer per topic;
		// for the common case (one topic per node) this is a no-op.
		c.cfg.Topic = topic
	}
	w, err := c.writerLazy()
	if err != nil {
		return kafkaProduceResult{}, err
	}

	msg := kafka.Message{
		Key:     key,
		Value:   value,
		Time:    time.Now(),
		Headers: toKafkaHeaders(headers),
	}
	if err := w.WriteMessages(ctx, msg); err != nil {
		return kafkaProduceResult{}, fmt.Errorf("kafka native produce: %w", err)
	}

	// segmentio/kafka-go's Writer does not surface the assigned
	// partition/offset per-message on a single-shot Write. Return
	// `Partition: -1, Offset: -1` to signal "delivered" without
	// inventing metadata the broker did not give us. The node-level
	// result rendering is unchanged because the field is still
	// emitted on the workflow output.
	return kafkaProduceResult{
		Topic:     c.cfg.Topic,
		Partition: -1,
		Offset:    -1,
	}, nil
}

// Poll blocks for up to `timeout` waiting for new records on the
// configured topic, returning at most `maxRecords` items. Used by
// the consumer trigger node. The Reader is bound to a consumer
// group so multiple m9m replicas coordinate through the broker's
// group coordinator.
func (c *KafkaNativeClient) Poll(ctx context.Context, timeout time.Duration, maxRecords int) ([]KafkaRecord, error) {
	if c.cfg.GroupID == "" {
		return nil, errors.New("kafka native: GroupID is required for the consumer trigger")
	}
	c.mu.Lock()
	if c.reader == nil {
		d, err := c.dialer()
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		c.reader = kafka.NewReader(kafka.ReaderConfig{
			Brokers:        c.cfg.Brokers,
			GroupID:        c.cfg.GroupID,
			Topic:          c.cfg.Topic,
			MinBytes:       1,
			MaxBytes:       1 << 20, // 1 MiB per fetch
			MaxWait:        500 * time.Millisecond,
			Dialer:         d,
			StartOffset:    kafka.LastOffset, // ignore backlog; user can override via seek
			CommitInterval: time.Second,
		})
	}
	r := c.reader
	c.mu.Unlock()

	if maxRecords <= 0 || maxRecords > 1000 {
		maxRecords = 100
	}

	out := make([]KafkaRecord, 0, maxRecords)
	for len(out) < maxRecords {
		pollCtx, cancel := context.WithTimeout(ctx, timeout)
		msg, err := r.FetchMessage(pollCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			// net.OpError / io.EOF inside a poll window just
			// means "no messages in this window"; surface as
			// empty rather than failing the trigger.
			var netErr net.Error
			if errors.As(err, &netErr) {
				break
			}
			return out, fmt.Errorf("kafka native poll: %w", err)
		}
		out = append(out, KafkaRecord{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Headers:   fromKafkaHeaders(msg.Headers),
			Timestamp: msg.Time,
		})
		// Commit the offset so the next poll skips it. Commit
		// failures are non-fatal — the broker will redeliver
		// the record on the next reconnect, which is exactly
		// the "at-least-once" guarantee we want.
		_ = r.CommitMessages(ctx, msg)
	}
	return out, nil
}

// Close releases the Writer / Reader. Safe to call multiple times.
func (c *KafkaNativeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var firstErr error
	if c.writer != nil {
		if err := c.writer.Close(); err != nil {
			firstErr = err
		}
	}
	if c.reader != nil {
		if err := c.reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// KafkaRecord is the transport-agnostic record shape used by both
// the REST and native backends. The consumer trigger's Execute
// method speaks this shape (the REST proxy path constructs the
// same fields inline today; consolidating them here means the
// trigger no longer needs an `if mode == native` branch).
type KafkaRecord struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Timestamp time.Time
}

// toKafkaHeaders converts the public `map[string]string` shape
// (the one the workflow author supplies) into the slice of
// `kafka.Header` the segmentio writer expects.
func toKafkaHeaders(in map[string]string) []kafka.Header {
	if len(in) == 0 {
		return nil
	}
	out := make([]kafka.Header, 0, len(in))
	for k, v := range in {
		out = append(out, kafka.Header{Key: k, Value: []byte(v)})
	}
	return out
}

// fromKafkaHeaders is the inverse of toKafkaHeaders, flattening a
// segmentio header slice into the public map[string]string shape.
func fromKafkaHeaders(in []kafka.Header) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for _, h := range in {
		out[h.Key] = string(h.Value)
	}
	return out
}

// keyBasedBalancer routes by key when a key is present (so
// identical keys land on the same partition and stay ordered) and
// falls back to consistent round-robin when no key is supplied —
// segmentio/kafka-go's `kafka.Hash{}` panics on an empty key, so
// without this fallback every key-less workflow would crash on
// the first record.
type keyBasedBalancer struct{}

func (keyBasedBalancer) Balance(msg kafka.Message, partitions ...int) int {
	if len(partitions) == 0 {
		return 0
	}
	if len(msg.Key) > 0 {
		// FNV-1a 64-bit hash, same algorithm kafka-go uses
		// internally for `Hash{}`. We re-implement it inline
		// (rather than calling Hash{Balance}) because Hash
		// rejects an empty key before reaching Balance.
		var h uint64 = 1469598103934665603
		for _, b := range msg.Key {
			h ^= uint64(b)
			h *= 1099511628211
		}
		return partitions[int(h%uint64(len(partitions)))]
	}
	// Round-robin via a per-Writer counter; we don't have a
	// partition list lock here, but kafka-go invokes the
	// balancer from a single goroutine so the unsynchronised
	// increment is safe in practice.
	return partitions[keyRoundRobin.Add(1)%int64(len(partitions))]
}

var keyRoundRobin atomic.Int64
