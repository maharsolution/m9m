// Package messaging contains Kafka node implementations.
//
// m9m ships two Kafka nodes that mirror the n8n core catalog:
//
//   - n8n-nodes-base.kafka        — produce a message to a topic
//   - n8n-nodes-base.kafkaTrigger — consume a message from a topic
//
// Each node supports two transport modes, selected via the `mode`
// parameter on the node:
//
//   - `mode: rest` (default) — speaks the Confluent-compatible REST
//     Proxy protocol (`POST /topics/{topic}/produce`,
//     `POST /consumers/{group}`, `GET /consumers/{group}/records`).
//     The proxy is the supported deployment pattern in production
//     Kubernetes / serverless clusters where opening direct broker
//     ports is forbidden; ACLs and schema validation are configured
//     once on the proxy instead of per-client.
//   - `mode: native` — opens TCP connections to one or more
//     `bootstrap.servers` directly via `segmentio/kafka-go`. Supports
//     PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512 SASL and TLS. No proxy
//     hop, no extra sidecar. Use this when the broker is reachable
//     from the m9m container and you want to skip the proxy round-
//     trip.
//
// Both modes share the same `kafkaBackend` contract (see below) so
// the node-level rendering of the produce result / poll output is
// identical regardless of transport.
package messaging

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// kafkaBackend is the dispatch interface shared by the producer
// and consumer nodes. The REST proxy and the native (segmentio/
// kafka-go) client both implement it, so the node-level code does
// not branch on transport mode.
type kafkaBackend interface {
	Produce(ctx context.Context, topic string, key, value []byte, headers map[string]string) (kafkaProduceResult, error)
	// Poll blocks for up to `timeout` waiting for new records on
	// the configured topic, returning at most `maxRecords` items.
	// Used by the consumer trigger node; producer-only clients
	// (REST proxy) return (nil, nil) for it because their
	// consumer path is a different REST shape implemented inside
	// the consumer trigger's `pollRecords` helper.
	Poll(ctx context.Context, timeout time.Duration, maxRecords int) ([]KafkaRecord, error)
	Close() error
}

// kafkaProduceResult captures the broker-side metadata a
// successful produce returns. The producer node surfaces it on
// the workflow output as `{ topic, partition, offset }`.
type kafkaProduceResult struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

// KafkaNode implements `n8n-nodes-base.kafka` — the producer side.
type KafkaNode struct {
	*base.BaseNode
	httpClient *http.Client

	mu      sync.Mutex
	backend kafkaBackend
}

// NewKafkaNode creates a new Kafka producer node.
func NewKafkaNode() *KafkaNode {
	return &KafkaNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Kafka",
			Description: "Produce messages to a Kafka topic via REST Proxy or native TCP.",
			Category:    "Messaging",
			Properties:  kafkaProperties(),
			Inputs:      []string{"main"},
			Outputs:     []string{"main"},
		}),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// kafkaProperties returns the Kafka producer property descriptors.
// Mirrors n8n's `INodeTypeDescription.properties` for
// `n8n-nodes-base.kafka`: a transport switch (rest / native), a
// REST Proxy URL or a comma-separated `bootstrap.servers` list, a
// topic, a message body that defaults to `={{ $json }}`, optional
// key, optional SASL/TLS fields, and optional headers.
func kafkaProperties() []base.NodeProperty {
	modes := []base.Option{
		{Name: "REST Proxy", Value: "rest"},
		{Name: "Native (segmentio/kafka-go)", Value: "native"},
	}
	authModes := []base.Option{
		{Name: "None", Value: "none"},
		{Name: "Basic", Value: "basic"},
		{Name: "API key", Value: "apiKey"},
	}
	saslMechs := []base.Option{
		{Name: "PLAIN", Value: "PLAIN"},
		{Name: "SCRAM-SHA-256", Value: "SCRAM-SHA-256"},
		{Name: "SCRAM-SHA-512", Value: "SCRAM-SHA-512"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Transport", "mode", "rest", "REST Proxy hops through a sidecar proxy; Native opens TCP to the brokers via segmentio/kafka-go.", modes, true),
		// REST Proxy fields (mode=rest)
		base.StringProp("REST Proxy URL", "restProxyUrl", "", "Confluent-compatible REST Proxy base URL (mode=rest).", "http://kafka-rest:8082", false),
		base.StringOpt("Authentication", "authentication", "none", "How to authenticate against the REST Proxy.", authModes, false),
		base.StringProp("Username", "username", "", "Username for Basic auth.", "", false),
		base.StringProp("Password", "password", "", "Password for Basic auth.", "", false),
		base.StringProp("API key", "apiKey", "", "API key (alternative to Basic).", "", false),
		// Native fields (mode=native)
		base.StringProp("Bootstrap Servers", "brokers", "", "Comma-separated broker list for mode=native (e.g. broker1:9092,broker2:9092).", "broker1:9092", false),
		base.StringOpt("SASL Mechanism", "saslMechanism", "", "PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512 — leave empty for no SASL.", saslMechs, false),
		base.StringProp("SASL Username", "saslUsername", "", "Username for SASL (mode=native).", "", false),
		base.StringProp("SASL Password", "saslPassword", "", "Password for SASL (mode=native).", "", false),
		base.BoolProp("TLS", "tls", false, "Enable TLS for the native broker connection (mode=native)."),
		// Shared
		base.StringProp("Topic", "topic", "", "Topic to produce to.", "events.user", true),
		base.StringProp("Key", "key", "", "Optional message key (used for partitioning).", "{{ $json.id }}", false),
		base.StringProp("Value", "value", "", "Message payload — evaluated as an n8n expression, then JSON-encoded.", "={{ $json }}", true),
		base.JsonProp("Headers", "headers", "{}", "Optional headers map attached to every record."),
	}
	return append(props, base.CommonSettings()...)
}

// Execute produces one record per input item, returning a
// per-record metadata array.
func (n *KafkaNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	be, err := n.backendFor(nodeParams)
	if err != nil {
		return nil, n.CreateError(err.Error(), nil)
	}

	headers, _ := nodeParams["headers"].(map[string]interface{})
	headerMap := map[string]string{}
	for k, v := range headers {
		headerMap[k] = fmt.Sprintf("%v", v)
	}

	result := make([]model.DataItem, 0, len(inputData))
	for _, item := range inputData {
		value := materialiseValue(nodeParams["value"], item)
		key := materialiseValue(nodeParams["key"], item)

		// Encode the value as a base64 string (Confluent REST
		// Proxy expects base64-encoded `value` for binary safety,
		// and bare strings for the legacy "raw" form). We use the
		// explicit `value` + `value_bytes` fields so the broker
		// preserves the payload byte-for-byte regardless of the
		// content type.
		var raw []byte
		switch v := value.(type) {
		case string:
			raw = []byte(v)
		case []byte:
			raw = v
		default:
			raw, _ = json.Marshal(value)
		}

		var keyBytes []byte
		if k, ok := key.(string); ok {
			keyBytes = []byte(k)
		} else {
			keyBytes, _ = json.Marshal(key)
		}

		meta, err := be.Produce(context.Background(),
			nodeParams["topic"].(string),
			keyBytes, raw, headerMap)
		if err != nil {
			return nil, n.CreateError(fmt.Sprintf("kafka produce: %v", err), nil)
		}
		result = append(result, model.DataItem{
			JSON: map[string]interface{}{
				"topic":     meta.Topic,
				"partition": meta.Partition,
				"offset":    meta.Offset,
				"key":       string(keyBytes),
				"headers":   headerMap,
			},
		})
	}
	return result, nil
}

// backendFor returns (and caches) the per-node transport client
// for the configured `mode` (rest / native). The REST proxy client
// is a thin HTTP wrapper that signs the request according to the
// configured auth mode; the native client opens TCP connections
// directly to the broker list. Caching is per-instance: a workflow
// that produces 1k items reuses the same Writer / Reader instead of
// paying a TCP handshake on every record.
func (n *KafkaNode) backendFor(params map[string]interface{}) (kafkaBackend, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.backend != nil {
		return n.backend, nil
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", params["mode"])))
	if mode == "" {
		mode = "rest"
	}
	switch mode {
	case "native":
		brokersRaw := strings.TrimSpace(fmt.Sprintf("%v", params["brokers"]))
		if brokersRaw == "" {
			return nil, fmt.Errorf("brokers is required when mode=native")
		}
		var brokers []string
		for _, b := range strings.Split(brokersRaw, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				brokers = append(brokers, b)
			}
		}
		topic := strings.TrimSpace(fmt.Sprintf("%v", params["topic"]))
		client, err := NewKafkaNativeClient(KafkaNativeConfig{
			Brokers:       brokers,
			Topic:         topic,
			SASLMechanism: strings.TrimSpace(fmt.Sprintf("%v", params["saslMechanism"])),
			SASLUsername:  fmt.Sprintf("%v", params["saslUsername"]),
			SASLPassword:  fmt.Sprintf("%v", params["saslPassword"]),
			TLS:           boolParam(params, "tls", false),
			ClientID:      "m9m-kafka-producer",
		})
		if err != nil {
			return nil, err
		}
		n.backend = client
		return n.backend, nil
	default: // "rest" or any unrecognised value (legacy behaviour)
		restURL := strings.TrimSpace(fmt.Sprintf("%v", params["restProxyUrl"]))
		if restURL == "" {
			return nil, fmt.Errorf("restProxyUrl is required when mode=rest")
		}
		auth := kafkaAuth{
			Mode:     fmt.Sprintf("%v", params["authentication"]),
			Username: fmt.Sprintf("%v", params["username"]),
			Password: fmt.Sprintf("%v", params["password"]),
			APIKey:   fmt.Sprintf("%v", params["apiKey"]),
		}
		n.backend = &kafkaRESTProxy{baseURL: strings.TrimRight(restURL, "/"), auth: auth, client: n.httpClient}
		return n.backend, nil
	}
}

// boolParam returns the bool value of `k` from the params map,
// defaulting to `def` when missing or of an unexpected type. Used
// by the native-broker dispatcher so an int(0) or a string("false")
// doesn't silently default the wrong way.
func boolParam(p map[string]interface{}, k string, def bool) bool {
	v, ok := p[k]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off", "":
			return false
		}
	}
	return def
}

// materialiseValue evaluates a value parameter against the
// current item, so n8n-style `={{ $json.x }}` strings resolve to
// the inbound JSON. Plain literals pass through untouched.
func materialiseValue(raw interface{}, item model.DataItem) interface{} {
	s, ok := raw.(string)
	if !ok {
		return raw
	}
	if !strings.HasPrefix(s, "=") && !strings.HasPrefix(s, "{{") {
		return s
	}
	expr := strings.TrimPrefix(s, "=")
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "{{") && strings.HasSuffix(expr, "}}") {
		// Tiny expression resolver: support $json.<path> lookups
		// only. A full n8n expression runtime is overkill for
		// the value/key parameters on a producer.
		expr = strings.TrimSuffix(strings.TrimPrefix(expr, "{{"), "}}")
		expr = strings.TrimSpace(expr)
		if strings.HasPrefix(expr, "$json") {
			path := strings.TrimPrefix(expr, "$json")
			path = strings.TrimPrefix(path, ".")
			cur := item.JSON
			for _, p := range strings.Split(path, ".") {
				if cur == nil {
					return nil
				}
				if m, ok := cur[p].(map[string]interface{}); ok {
					cur = m
				} else {
					return cur[p]
				}
			}
			return cur
		}
	}
	return s
}

// kafkaAuth is the REST Proxy authentication envelope. The Mode
// field decides which of the credential fields are sent: `none`
// sends nothing, `basic` sends Basic auth, `apiKey` sends the
// `X-Api-Key` header (which is what Confluent Cloud uses).
type kafkaAuth struct {
	Mode     string
	Username string
	Password string
	APIKey   string
}

func (a kafkaAuth) apply(req *http.Request) {
	switch a.Mode {
	case "basic":
		if a.Username != "" {
			req.SetBasicAuth(a.Username, a.Password)
		}
	case "apiKey":
		if a.APIKey != "" {
			req.Header.Set("X-Api-Key", a.APIKey)
		}
	}
}

// kafkaRESTProxy is the production Kafka backend. It speaks the
// Confluent REST Proxy v2 `POST /topics/{topic}/produce` shape
// documented at
// https://docs.confluent.io/platform/current/kafka-rest/api.html.
type kafkaRESTProxy struct {
	baseURL string
	auth    kafkaAuth
	client  *http.Client
}

type kafkaProducePayload struct {
	Key struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"key,omitempty"`
	Value struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"value"`
	Headers map[string]string `json:"headers,omitempty"`
}

type kafkaProduceRequest struct {
	Records []kafkaProducePayload `json:"records"`
}

type kafkaProduceResponse struct {
	Offsets []struct {
		Partition int   `json:"partition"`
		Offset    int64 `json:"offset"`
		Error     *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	} `json:"offsets"`
}

func (k *kafkaRESTProxy) Produce(ctx context.Context, topic string, key, value []byte, headers map[string]string) (kafkaProduceResult, error) {
	if topic == "" {
		return kafkaProduceResult{}, fmt.Errorf("topic is required")
	}
	payload := kafkaProduceRequest{Records: make([]kafkaProducePayload, 1)}
	r := &payload.Records[0]
	// Use the `BINARY` type so the broker stores the bytes
	// verbatim; `STRING` would UTF-8-decode and re-encode.
	r.Key.Type = "BINARY"
	r.Key.Value = base64.StdEncoding.EncodeToString(key)
	r.Value.Type = "BINARY"
	r.Value.Value = base64.StdEncoding.EncodeToString(value)
	if len(headers) > 0 {
		r.Headers = headers
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return kafkaProduceResult{}, err
	}
	url := k.baseURL + "/topics/" + topic + "/produce"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return kafkaProduceResult{}, err
	}
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	req.Header.Set("Accept", "application/vnd.kafka.v2+json")
	k.auth.apply(req)

	resp, err := k.client.Do(req)
	if err != nil {
		return kafkaProduceResult{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return kafkaProduceResult{}, fmt.Errorf("kafka rest: status %d: %s", resp.StatusCode, string(respBody))
	}
	var out kafkaProduceResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return kafkaProduceResult{}, fmt.Errorf("kafka rest: decode: %w (body=%s)", err, string(respBody))
	}
	if len(out.Offsets) == 0 {
		return kafkaProduceResult{Topic: topic}, nil
	}
	off := out.Offsets[0]
	if off.Error != nil {
		return kafkaProduceResult{}, fmt.Errorf("kafka rest: %s", off.Error.Message)
	}
	return kafkaProduceResult{
		Topic:     topic,
		Partition: off.Partition,
		Offset:    off.Offset,
	}, nil
}

func (k *kafkaRESTProxy) Close() error { return nil }

// Poll is a no-op for the REST proxy producer client. The
// consumer path on REST Proxy uses a different shape (create
// consumer / subscribe / poll records) that lives on the
// `KafkaTriggerNode` itself rather than on this backend.
func (k *kafkaRESTProxy) Poll(_ context.Context, _ time.Duration, _ int) ([]KafkaRecord, error) {
	return nil, nil
}

// ValidateParameters validates the Kafka producer parameters.
// Mode-aware: `restProxyUrl` is required when mode=rest; `brokers`
// is required when mode=native. `topic` and `value` are always
// required.
func (n *KafkaNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return n.CreateError("parameters cannot be nil", nil)
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", params["mode"])))
	if mode == "" {
		mode = "rest"
	}
	switch mode {
	case "native":
		if b := strings.TrimSpace(fmt.Sprintf("%v", params["brokers"])); b == "" {
			return n.CreateError("brokers is required when mode=native", nil)
		}
	default:
		if rest, _ := params["restProxyUrl"].(string); strings.TrimSpace(rest) == "" {
			return n.CreateError("restProxyUrl is required when mode=rest", nil)
		}
	}
	if topic := strings.TrimSpace(fmt.Sprintf("%v", params["topic"])); topic == "" {
		return n.CreateError("topic is required", nil)
	}
	if v, ok := params["value"]; !ok || fmt.Sprintf("%v", v) == "" {
		return n.CreateError("value is required", nil)
	}
	return nil
}
