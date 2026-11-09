package messaging

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	CACert        string // PEM-encoded CA cert chain (Aiven / Confluent Cloud)
	ClientCert    string // PEM-encoded client certificate (mTLS)
	ClientKey     string // PEM-encoded client private key (mTLS, must pair with ClientCert)
	ClientID      string // optional, surfaced in broker logs
	StartOffset   int64  // consumer group start position: kafka.FirstOffset / kafka.LastOffset (default LastOffset)
}

// joinGrace is the extra time the first fetch of a new consumer
// group is allowed to spend waiting for the group join. Aiven's
// coordinator takes ~3-4 s to answer FindCoordinator + JoinGroup +
// SyncGroup, which regularly exceeds a 5 s poll window once the
// TLS + SASL handshake is included.
const joinGrace = 20 * time.Second

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
			s, sErr := scram.Mechanism(scram.SHA256, c.cfg.SASLUsername, c.cfg.SASLPassword)
			if sErr != nil {
				return nil, fmt.Errorf("kafka native: scram-sha-256 init: %w", sErr)
			}
			saslMech = s
		case "SCRAM-SHA-512":
			s, sErr := scram.Mechanism(scram.SHA512, c.cfg.SASLUsername, c.cfg.SASLPassword)
			if sErr != nil {
				return nil, fmt.Errorf("kafka native: scram-sha-512 init: %w", sErr)
			}
			saslMech = s
		default:
			return nil, fmt.Errorf("kafka native: unsupported SASL mechanism %q (supported: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)", c.cfg.SASLMechanism)
		}
		d.SASLMechanism = saslMech
	}

	if c.cfg.TLS {
		// Build a TLS config with the minimum protocol version
		// pinned to TLS 1.2 (Aiven's brokers reject TLS 1.0/1.1
		// handshakes) and leave `ServerName` unset so segmentio/
		// kafka-go's pool fills it in from the broker address on
		// first dial (matches the behaviour of `tls.Dial` to a
		// `host:port` string). Cert verification is on by default;
		// the Aiven brokers use public CA-signed certs, so this
		// works without an extra root-pool file.
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if c.cfg.CACert != "" {
			pool := x509.NewCertPool()
			pemBytes := normalizePEM(c.cfg.CACert)
			if !pool.AppendCertsFromPEM(pemBytes) {
				return nil, errors.New("kafka native: CACert is set but did not contain any valid PEM blocks")
			}
			tlsCfg.RootCAs = pool
		}
		// mTLS: load a client cert+key pair from the credential
		// envelope. Both halves must be present; partial pairs
		// fail closed with a clear error rather than silently
		// falling back to one-way TLS.
		if c.cfg.ClientCert != "" || c.cfg.ClientKey != "" {
			if c.cfg.ClientCert == "" || c.cfg.ClientKey == "" {
				return nil, errors.New("kafka native: ClientCert and ClientKey must both be set for mTLS")
			}
			cert, err := tls.X509KeyPair(
				normalizePEM(c.cfg.ClientCert),
				normalizePEM(c.cfg.ClientKey),
			)
			if err != nil {
				return nil, fmt.Errorf("kafka native: mTLS client cert load: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		d.TLS = tlsCfg
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
	// Build a custom Dial function that does the TLS handshake
	// inline (rather than relying on `kafka.Transport.TLS` to do
	// it after the plaintext dial). The two-step approach in
	// segmentio's transport pool has historically been a source
	// of "unexpected EOF: broker appears to be expecting TLS"
	// errors on Aiven / Confluent Cloud brokers when the
	// underlying connection from `defaultDialer.DialContext`
	// completes the TCP handshake but is then re-wrapped with
	// `tls.Client` on a connection that the broker has already
	// partially closed for being plaintext. Doing the TLS dial
	// ourselves guarantees the very first byte on the wire is
	// the TLS ClientHello.
	dialFn := func(ctx context.Context, network, address string) (net.Conn, error) {
		rawConn, dialErr := (&net.Dialer{
			Timeout:   d.Timeout,
			DualStack: true,
		}).DialContext(ctx, network, address)
		if dialErr != nil {
			return nil, dialErr
		}
		if d.TLS == nil {
			return rawConn, nil
		}
		// Clone so we can fill in ServerName without mutating
		// the shared *tls.Config that other Writer instances
		// may also reference.
		tlsCfg := d.TLS.Clone()
		if tlsCfg.ServerName == "" {
			host, _, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				host = address
			}
			tlsCfg.ServerName = host
		}
		return tls.Client(rawConn, tlsCfg), nil
	}
	w := &kafka.Writer{
		Addr: kafka.TCP(c.cfg.Brokers...),
		// `Topic` is deliberately left EMPTY. kafka-go treats
		// `Writer.Topic` and `Message.Topic` as mutually exclusive:
		// whichever is set wins (`Writer.chooseTopic`). Setting
		// `Writer.Topic` here pinned the writer to the FIRST topic
		// the node ever produced to, so a later run with a
		// different `topic` parameter silently published to the
		// OLD topic while the node reported the NEW one — data
		// landing on the wrong topic with no error.
		//
		// The per-message topic is set in `Produce` instead, which
		// makes the writer topic-agnostic and correct for nodes
		// whose topic is an expression resolved per item.
		//
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
		// Transport carries the SASL / ClientID metadata. We
		// own the actual dial + TLS handshake via the custom
		// `Dial` function above, so we leave `TLS` nil here
		// (otherwise the pool would re-wrap our already-TLS
		// connection in another `tls.Client` and double-encrypt
		// the handshake).
		Transport: &kafka.Transport{
			Dial:        dialFn,
			DialTimeout: d.Timeout,
			SASL:        d.SASLMechanism,
			ClientID:    d.ClientID,
		},
	}
	// `Completion` is invoked by kafka-go once the broker has
	// acknowledged the batch. It is the only hook that carries the
	// broker-assigned partition / offset back to the caller —
	// `WriteMessages` itself returns only an error. We stash the
	// metadata on the message via `WriterData` so `Produce` can
	// surface a real partition / offset instead of the -1
	// placeholder (which is what n8n's Kafka node returns).
	//
	// `WriterData` is a pointer so that the mutation performed here
	// is visible to the caller: kafka-go copies the `Message` struct
	// into its internal batch, but the pointer payload survives the
	// copy.
	w.Completion = func(msgs []kafka.Message, err error) {
		if err != nil {
			return
		}
		for i := range msgs {
			meta, ok := msgs[i].WriterData.(*produceMeta)
			if !ok || meta == nil {
				continue
			}
			meta.Partition = msgs[i].Partition
			meta.Offset = msgs[i].Offset
		}
	}
	c.writer = w
	return w, nil
}

// produceMeta carries the broker-assigned partition / offset back
// from the kafka-go `Completion` hook to the `Produce` caller. It
// is passed by pointer through `kafka.Message.WriterData` because
// kafka-go copies the `Message` struct into its write batch.
type produceMeta struct {
	Partition int
	Offset    int64
}

// Produce writes one record to Kafka via the segmentio/kafka-go
// Writer. The native transport delivers the same `kafkaProduceResult`
// shape as the REST proxy so the node-level result rendering is
// unchanged.
func (c *KafkaNativeClient) Produce(ctx context.Context, topic string, key, value []byte, headers map[string]string) (kafkaProduceResult, error) {
	if topic == "" {
		return kafkaProduceResult{}, errors.New("kafka native: topic is required")
	}
	w, err := c.writerLazy()
	if err != nil {
		return kafkaProduceResult{}, err
	}

	meta := &produceMeta{Partition: -1, Offset: -1}
	msg := kafka.Message{
		// The topic travels WITH the message (the writer itself is
		// topic-agnostic — see `writerLazy`). Setting `msg.Topic`
		// per record is what lets one node publish to different
		// topics across runs / per item without the writer pinning
		// itself to the first topic it ever saw.
		Topic:      topic,
		Key:        key,
		Value:      value,
		Time:       time.Now(),
		Headers:    toKafkaHeaders(headers),
		WriterData: meta,
	}
	if err := w.WriteMessages(ctx, msg); err != nil {
		return kafkaProduceResult{}, fmt.Errorf("kafka native produce: %w", err)
	}

	// The `Completion` hook (installed in `writerLazy`) fills `meta`
	// with the broker-assigned partition / offset. Fall back to
	// -1 / -1 only when the hook did not run (which happens on older
	// kafka-go versions) so the caller can still tell "delivered but
	// unknown position" from "delivered at offset N".
	res := kafkaProduceResult{
		Topic:     topic,
		Partition: meta.Partition,
		Offset:    meta.Offset,
	}
	return res, nil
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
	justJoined := false
	if c.reader == nil {
		d, err := c.dialer()
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		// `StartOffset` only applies when the consumer group has no
		// committed offset for the partition yet. n8n exposes this
		// as the trigger's `startPosition` option:
		//   first  -> replay the whole topic from the beginning
		//   last   -> only receive messages produced after the join
		// Defaulting to `last` matches n8n, but a fresh group then
		// never sees messages that were produced *before* the
		// trigger started — which is the common "produce then
		// consume" smoke-test case.
		startOffset := c.cfg.StartOffset
		if startOffset == 0 {
			startOffset = kafka.LastOffset
		}
		c.reader = kafka.NewReader(kafka.ReaderConfig{
			Brokers:        c.cfg.Brokers,
			GroupID:        c.cfg.GroupID,
			Topic:          c.cfg.Topic,
			MinBytes:       1,
			MaxBytes:       1 << 20, // 1 MiB per fetch
			MaxWait:        500 * time.Millisecond,
			Dialer:         d,
			StartOffset:    startOffset,
			CommitInterval: time.Second,
		})
		justJoined = true
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
		if err != nil && justJoined {
			// The very first fetch of a brand-new consumer group
			// has to wait for the group join (FindCoordinator +
			// JoinGroup + SyncGroup), which routinely takes longer
			// than the operator's poll interval. Without this
			// allowance the first poll always returns zero records
			// and the backlog is only visible one tick later —
			// which made the "produce then consume" smoke test
			// look flaky.
			justJoined = false
			joinCtx, joinCancel := context.WithTimeout(ctx, joinGrace)
			msg, err = r.FetchMessage(joinCtx)
			joinCancel()
		}
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

// normalizePEM takes a PEM-encoded certificate (possibly with
// arbitrary whitespace inserted between base64 characters — the
// n8n credential envelope ships it that way after JSON-encoding
// a multi-line string) and returns a clean, line-broken form that
// `x509.CertPool.AppendCertsFromPEM` will accept.
//
// Without this, segments like "MIIERDCCA qygAwIBAgIU" get rejected
// because the parser only honours newlines / carriage-returns /
// tabs as whitespace inside the base64 payload, not plain spaces.
func normalizePEM(pem string) []byte {
	// Strip all whitespace (spaces, tabs, newlines).
	var stripped strings.Builder
	stripped.Grow(len(pem))
	for _, r := range pem {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		stripped.WriteRune(r)
	}
	// Re-emit with a CRLF after each BEGIN/END header so the parser
	// sees a well-formed block boundary.
	clean := strings.ReplaceAll(stripped.String(), "-----BEGINCERTIFICATE-----", "-----BEGIN CERTIFICATE-----\n")
	clean = strings.ReplaceAll(clean, "-----ENDCERTIFICATE-----", "\n-----END CERTIFICATE-----\n")
	clean = strings.ReplaceAll(clean, "-----BEGINPRIVATEKEY-----", "-----BEGIN PRIVATE KEY-----\n")
	clean = strings.ReplaceAll(clean, "-----ENDPRIVATEKEY-----", "\n-----END PRIVATE KEY-----\n")
	return []byte(clean)
}
