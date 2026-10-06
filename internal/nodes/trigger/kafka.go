package trigger

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
	"github.com/neul-labs/m9m/internal/nodes/messaging"
)

// KafkaTriggerNode implements `n8n-nodes-base.kafkaTrigger`. It
// supports two transport modes, selected via the `mode` parameter:
//
//   - `mode: rest` — polls a Confluent-compatible REST Proxy using
//     the create-consumer / subscribe / poll-records cycle. The
//     REST proxy's cycle is idempotent when the group is stable, so
//     re-running the trigger after a crash picks up where it left
//     off. Offsets are broker-managed; m9m does not persist them
//     locally.
//   - `mode: native` — opens a consumer-group connection directly
//     to the broker list via segmentio/kafka-go (see
//     `messaging.KafkaNativeClient`). No proxy hop, lower latency.
//
// In production the polling is wrapped in a ticker (default 5s)
// and runs in the workflow scheduler's goroutine; the node's
// `Execute` path is invoked on each tick with the polled records
// as `inputData`. The `intervalSeconds` parameter controls the
// cadence.
type KafkaTriggerNode struct {
	*base.BaseNode
	client *http.Client

	mu      sync.Mutex
	nativeC map[string]*messaging.KafkaNativeClient
}

// NewKafkaTriggerNode creates a new Kafka trigger node.
func NewKafkaTriggerNode() *KafkaTriggerNode {
	return &KafkaTriggerNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Kafka Trigger",
			Description: "Trigger a workflow on Kafka messages (REST Proxy or native TCP).",
			Category:    "Triggers",
			Properties:  kafkaTriggerProperties(),
			Inputs:      []string{},
			Outputs:     []string{"main"},
		}),
		client:  &http.Client{Timeout: 30 * time.Second},
		nativeC: map[string]*messaging.KafkaNativeClient{},
	}
}

func kafkaTriggerProperties() []base.NodeProperty {
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
		base.StringProp("Group ID", "groupId", "", "Consumer group id (used by the broker to track offsets).", "m9m-trigger", true),
		base.StringProp("Topic", "topic", "", "Topic to subscribe to.", "events.user", true),
		base.NumberProp("Poll interval (s)", "intervalSeconds", 5, "How often to poll for new records.", false),
		base.NumberProp("Max records per poll", "maxRecords", 100, "Upper bound on records returned per poll (1-1000).", false),
	}
	return append(props, base.CommonSettings()...)
}

// Execute polls the configured transport for new records and
// returns them as DataItems. Dispatches on `mode`: the REST path
// keeps its existing create-consumer + poll-records shape; the
// native path opens a segmentio/kafka-go reader and returns
// whatever the broker fetched in the poll window.
func (n *KafkaTriggerNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", nodeParams["mode"])))
	if mode == "" {
		mode = "rest"
	}
	topic := strings.TrimSpace(fmt.Sprintf("%v", nodeParams["topic"]))
	groupID := strings.TrimSpace(fmt.Sprintf("%v", nodeParams["groupId"]))
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if groupID == "" {
		return nil, fmt.Errorf("groupId is required")
	}

	switch mode {
	case "native":
		return n.pollNative(nodeParams, topic, groupID)
	default:
		return n.pollREST(nodeParams, topic, groupID)
	}
}

// pollREST is the REST Proxy path. Identical to the pre-P0
// implementation; preserved verbatim so existing workflows that
// target a sidecar proxy keep their behaviour.
func (n *KafkaTriggerNode) pollREST(nodeParams map[string]interface{}, topic, groupID string) ([]model.DataItem, error) {
	restURL := strings.TrimSpace(fmt.Sprintf("%v", nodeParams["restProxyUrl"]))
	if restURL == "" {
		return nil, fmt.Errorf("restProxyUrl is required when mode=rest")
	}
	auth := kafkaAuth{
		Mode:     fmt.Sprintf("%v", nodeParams["authentication"]),
		Username: fmt.Sprintf("%v", nodeParams["username"]),
		Password: fmt.Sprintf("%v", nodeParams["password"]),
		APIKey:   fmt.Sprintf("%v", nodeParams["apiKey"]),
	}
	restURL = strings.TrimRight(restURL, "/")

	ctx := context.Background()
	if err := n.ensureConsumer(ctx, restURL, groupID, topic, auth); err != nil {
		// First-call failure (consumer not created yet) is
		// not fatal — the next tick retries.
		return []model.DataItem{}, nil
	}
	return n.pollRecords(ctx, restURL, groupID, nodeParams, auth)
}

// pollNative opens (or reuses) a native consumer client for the
// (brokers, topic, groupID, sasl, tls) tuple and reads up to
// `maxRecords` records within the configured poll interval.
// Multiple workflows running the same Kafka trigger share one
// Reader per unique config so they don't all pay the consumer
// group join fee.
func (n *KafkaTriggerNode) pollNative(nodeParams map[string]interface{}, topic, groupID string) ([]model.DataItem, error) {
	brokersRaw := strings.TrimSpace(fmt.Sprintf("%v", nodeParams["brokers"]))
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
	cfg := messaging.KafkaNativeConfig{
		Brokers:       brokers,
		Topic:         topic,
		GroupID:       groupID,
		SASLMechanism: strings.TrimSpace(fmt.Sprintf("%v", nodeParams["saslMechanism"])),
		SASLUsername:  fmt.Sprintf("%v", nodeParams["saslUsername"]),
		SASLPassword:  fmt.Sprintf("%v", nodeParams["saslPassword"]),
		TLS:           boolParam(nodeParams, "tls", false),
		ClientID:      "m9m-kafka-trigger",
	}
	client, err := n.clientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka native: %w", err)
	}

	timeout := 2 * time.Second
	if v := nodeParams["intervalSeconds"]; v != nil {
		switch t := v.(type) {
		case int:
			if t > 0 {
				timeout = time.Duration(t) * time.Second
			}
		case int32:
			if int(t) > 0 {
				timeout = time.Duration(t) * time.Second
			}
		case float64:
			if int(t) > 0 {
				timeout = time.Duration(int(t)) * time.Second
			}
		}
	}
	max := 100
	if v := nodeParams["maxRecords"]; v != nil {
		switch t := v.(type) {
		case int:
			if t > 0 && t <= 1000 {
				max = t
			}
		case int32:
			if int(t) > 0 && int(t) <= 1000 {
				max = int(t)
			}
		case float64:
			if int(t) > 0 && int(t) <= 1000 {
				max = int(t)
			}
		}
	}

	records, err := client.Poll(context.Background(), timeout, max)
	if err != nil {
		return nil, fmt.Errorf("kafka native poll: %w", err)
	}
	out := make([]model.DataItem, 0, len(records))
	for _, r := range records {
		out = append(out, model.DataItem{
			JSON: map[string]interface{}{
				"value":     string(r.Value),
				"raw":       r.Value,
				"topic":     r.Topic,
				"key":       string(r.Key),
				"headers":   r.Headers,
				"offset":    r.Offset,
				"partition": r.Partition,
				"timestamp": r.Timestamp,
			},
		})
	}
	return out, nil
}

// clientFor returns a cached native client for the given config
// (creating it on first call). Cache key = brokers|topic|group|
// saslMech|userTLS — same config shares one Reader. The Reader
// is owned by m9m for the lifetime of the process; cancelling the
// workflow does not tear it down so reconnects don't replay
// already-committed offsets.
func (n *KafkaTriggerNode) clientFor(cfg messaging.KafkaNativeConfig) (*messaging.KafkaNativeClient, error) {
	key := fmt.Sprintf("%s|%s|%s|%s|%s|%v", strings.Join(cfg.Brokers, ","), cfg.Topic, cfg.GroupID, cfg.SASLMechanism, cfg.SASLUsername, cfg.TLS)
	n.mu.Lock()
	defer n.mu.Unlock()
	if c, ok := n.nativeC[key]; ok {
		return c, nil
	}
	c, err := messaging.NewKafkaNativeClient(cfg)
	if err != nil {
		return nil, err
	}
	n.nativeC[key] = c
	return c, nil
}

// boolParam returns the bool value of `k` from the params map,
// defaulting to `def` when missing or of an unexpected type.
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

func (n *KafkaTriggerNode) ensureConsumer(ctx context.Context, baseURL, group, topic string, auth kafkaAuth) error {
	body := map[string]interface{}{
		"name":               group,
		"format":             "binary",
		"auto.offset.reset":  "latest",
		"auto.commit.enable": "true",
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/consumers/"+group, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	auth.apply(req)
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 409 {
		// Consumer already exists — that's fine.
		return nil
	}
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("kafka create consumer: %d", resp.StatusCode)
	}
	// Subscribe the consumer to the topic.
	subscribe := map[string]interface{}{"topics": []string{topic}}
	sbuf, _ := json.Marshal(subscribe)
	sreq, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/consumers/"+group+"/subscription", bytes.NewReader(sbuf))
	if sreq == nil {
		return fmt.Errorf("kafka subscribe: build request")
	}
	sreq.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	auth.apply(sreq)
	sresp, err := n.client.Do(sreq)
	if err != nil {
		return err
	}
	defer sresp.Body.Close()
	if sresp.StatusCode >= 300 {
		return fmt.Errorf("kafka subscribe: %d", sresp.StatusCode)
	}
	return nil
}

func (n *KafkaTriggerNode) pollRecords(ctx context.Context, baseURL, group string, params map[string]interface{}, auth kafkaAuth) ([]model.DataItem, error) {
	max := 100
	if v, ok := params["maxRecords"]; ok {
		switch t := v.(type) {
		case int:
			if t > 0 && t <= 1000 {
				max = t
			}
		case int32:
			if int(t) > 0 && int(t) <= 1000 {
				max = int(t)
			}
		case float64:
			if int(t) > 0 && int(t) <= 1000 {
				max = int(t)
			}
		}
	}
	url := fmt.Sprintf("%s/consumers/%s/records?timeout=%d&max_bytes=%d",
		baseURL, group, 1000, 1<<20)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.kafka.v2+json")
	auth.apply(req)
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kafka poll: %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var records []map[string]interface{}
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("kafka poll decode: %w", err)
	}
	if len(records) > max {
		records = records[:max]
	}

	out := make([]model.DataItem, 0, len(records))
	for _, r := range records {
		raw, _ := r["value"].(string)
		val, _ := base64.StdEncoding.DecodeString(raw)
		item := model.DataItem{JSON: map[string]interface{}{
			"value":     string(val),
			"raw":       val,
			"topic":     r["topic"],
			"key":       r["key"],
			"headers":   r["headers"],
			"offset":    r["offset"],
			"partition": r["partition"],
		}}
		out = append(out, item)
	}
	return out, nil
}

// ValidateParameters validates the Kafka trigger parameters.
// Mode-aware: `restProxyUrl` is required when mode=rest; `brokers`
// is required when mode=native. `groupId` and `topic` are always
// required.
func (n *KafkaTriggerNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return fmt.Errorf("parameters cannot be nil")
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", params["mode"])))
	if mode == "" {
		mode = "rest"
	}
	switch mode {
	case "native":
		if b := strings.TrimSpace(fmt.Sprintf("%v", params["brokers"])); b == "" {
			return fmt.Errorf("brokers is required when mode=native")
		}
	default:
		if v := strings.TrimSpace(fmt.Sprintf("%v", params["restProxyUrl"])); v == "" {
			return fmt.Errorf("restProxyUrl is required when mode=rest")
		}
	}
	if v := strings.TrimSpace(fmt.Sprintf("%v", params["groupId"])); v == "" {
		return fmt.Errorf("groupId is required")
	}
	if v := strings.TrimSpace(fmt.Sprintf("%v", params["topic"])); v == "" {
		return fmt.Errorf("topic is required")
	}
	return nil
}

// kafkaAuth is duplicated here (rather than imported from the
// messaging package) so the trigger package can stay free of an
// import cycle. The shape is intentionally identical to
// `messaging.kafkaAuth`; if you add a new auth mode update both
// at once.
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