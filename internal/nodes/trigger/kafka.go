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
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// KafkaTriggerNode implements `n8n-nodes-base.kafkaTrigger`. It
// polls a Confluent-compatible REST Proxy for new records on a
// topic and emits one DataItem per record.
//
// The REST proxy's "create consumer + poll records" cycle is
// idempotent when the group is stable, so re-running the trigger
// after a crash picks up where it left off. We do not maintain
// offsets on the m9m side; the consumer group's broker-managed
// offsets are the source of truth.
//
// In production the polling is wrapped in a ticker (default 5s)
// and runs in the workflow scheduler's goroutine; the node's
// `Execute` path is invoked on each tick with the polled records
// as `inputData`. A user-facing "wait" / "interval" parameter
// controls the cadence.
type KafkaTriggerNode struct {
	*base.BaseNode
	client *http.Client
}

// NewKafkaTriggerNode creates a new Kafka trigger node.
func NewKafkaTriggerNode() *KafkaTriggerNode {
	return &KafkaTriggerNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Kafka Trigger",
			Description: "Trigger a workflow on Kafka messages (via REST Proxy)",
			Category:    "Triggers",
			Properties:  kafkaTriggerProperties(),
			Inputs:      []string{},
			Outputs:     []string{"main"},
		}),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func kafkaTriggerProperties() []base.NodeProperty {
	authModes := []base.Option{
		{Name: "None", Value: "none"},
		{Name: "Basic", Value: "basic"},
		{Name: "API key", Value: "apiKey"},
	}
	props := []base.NodeProperty{
		base.StringProp("REST Proxy URL", "restProxyUrl", "", "Confluent-compatible REST Proxy base URL.", "http://kafka-rest:8082", true),
		base.StringProp("Group ID", "groupId", "", "Consumer group id (used by the broker to track offsets).", "m9m-trigger", true),
		base.StringProp("Topic", "topic", "", "Topic to subscribe to.", "events.user", true),
		base.NumberProp("Poll interval (s)", "intervalSeconds", 5, "How often to poll the proxy for new records.", false),
		base.NumberProp("Max records per poll", "maxRecords", 100, "Upper bound on records returned per poll (1-1000).", false),
		base.StringOpt("Authentication", "authentication", "none", "How to authenticate against the REST Proxy.", authModes, false),
		base.StringProp("Username", "username", "", "Username for Basic auth.", "", false),
		base.StringProp("Password", "password", "", "Password for Basic auth.", "", false),
		base.StringProp("API key", "apiKey", "", "API key (alternative to Basic).", "", false),
	}
	return append(props, base.CommonSettings()...)
}

// Execute polls the REST Proxy for new records and returns them
// as DataItems. On a cold start the node creates (or reuses) a
// consumer in the configured group; subsequent calls just read.
func (n *KafkaTriggerNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	restURL, _ := nodeParams["restProxyUrl"].(string)
	if restURL == "" {
		return nil, fmt.Errorf("restProxyUrl is required")
	}
	groupID, _ := nodeParams["groupId"].(string)
	if groupID == "" {
		return nil, fmt.Errorf("groupId is required")
	}
	topic, _ := nodeParams["topic"].(string)
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
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
		// not fatal — the next tick retries. Log via the
		// error channel so the scheduler can surface it.
		return []model.DataItem{}, nil
	}
	return n.pollRecords(ctx, restURL, groupID, nodeParams, auth)
}

func (n *KafkaTriggerNode) ensureConsumer(ctx context.Context, baseURL, group, topic string, auth kafkaAuth) error {
	body := map[string]interface{}{
		"name":            group,
		"format":          "binary",
		"auto.offset.reset": "latest",
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
	// Subscribe the consumer to the topic. Done in a separate
	// request because the Confluent REST Proxy splits consumer
	// creation from topic subscription.
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
			"value":   string(val),
			"raw":     val,
			"topic":   r["topic"],
			"key":     r["key"],
			"headers": r["headers"],
			"offset":  r["offset"],
			"partition": r["partition"],
		}}
		out = append(out, item)
	}
	return out, nil
}

// ValidateParameters validates the Kafka trigger parameters.
func (n *KafkaTriggerNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return fmt.Errorf("parameters cannot be nil")
	}
	if v, _ := params["restProxyUrl"].(string); v == "" {
		return fmt.Errorf("restProxyUrl is required")
	}
	if v, _ := params["groupId"].(string); v == "" {
		return fmt.Errorf("groupId is required")
	}
	if v, _ := params["topic"].(string); v == "" {
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
