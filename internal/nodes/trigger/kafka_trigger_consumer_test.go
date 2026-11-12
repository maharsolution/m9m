/*
Package trigger — tests for the Kafka trigger's execution model.

n8n runs one consumer *daemon* per active workflow: the Kafka
trigger keeps polling the broker and pushes every record into the
workflow, whether or not a request is in flight.

m9m does not do that today. The Kafka trigger is a *poll
primitive*: `Execute` reads whatever the broker has right now and
returns the records as items. During a webhook-driven execution the
engine never calls it — the node is skipped with an explicit warning
(see `isConsumerTriggerType` in internal/engine/engine.go) — so a
workflow that waits for a Kafka message while another trigger serves
the request will never see that message under m9m.

These tests pin that documented behaviour:

  - `Execute` performs exactly ONE poll cycle and returns; it does
    not leave a consumer running in the background.
  - The node exposes no consumer-daemon lifecycle API (no
    Start/Stop/Run), so nothing can start a background poll.
  - The poll itself is a well-behaved one-shot: it creates the
    consumer, subscribes it, reads the records and decodes them.
*/
package trigger

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

// kafkaRESTServer is a minimal Confluent-compatible REST proxy used
// to observe what the trigger does during a poll.
type kafkaRESTServer struct {
	mu       sync.Mutex
	requests []string

	records []map[string]interface{}
	// consumerExists makes the create-consumer call answer 409
	// ("consumer already exists"), like a proxy that keeps the
	// consumer between polls.
	consumerExists bool
}

func (s *kafkaRESTServer) record(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, path)
}

func (s *kafkaRESTServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *kafkaRESTServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/consumers/", func(w http.ResponseWriter, r *http.Request) {
		s.record(r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/consumers/m9m-test-group":
			if s.consumerExists {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/consumers/m9m-test-group/subscription":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/consumers/m9m-test-group/records":
			w.Header().Set("Content-Type", "application/vnd.kafka.v2+json")
			json.NewEncoder(w).Encode(s.records)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return mux
}

// TestKafkaTrigger_ExecuteIsOneShotPoll proves the trigger performs a
// single poll cycle and returns — it does not keep a consumer
// running in the background the way n8n's daemon does.
//
// This is the behaviour that makes the Finnet post_balance workflow
// diagnosable: m9m will not deliver a Kafka record that arrives
// *after* the poll, and it does not pretend otherwise by holding the
// request open.
func TestKafkaTrigger_ExecuteIsOneShotPoll(t *testing.T) {
	srv := &kafkaRESTServer{records: []map[string]interface{}{}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	node := NewKafkaTriggerNode()
	params := map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": ts.URL,
		"topic":        "finnet-queue",
		"groupId":      "m9m-test-group",
	}

	items, err := node.Execute(nil, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items: want 0, got %d", len(items))
	}

	// One poll cycle = create consumer + subscribe + read records.
	paths := srv.paths()
	if len(paths) != 3 {
		t.Fatalf("REST calls during one poll: want 3, got %d (%v)", len(paths), paths)
	}
	if paths[0] != "/consumers/m9m-test-group" {
		t.Errorf("first call: want the consumer creation, got %q", paths[0])
	}
	if paths[1] != "/consumers/m9m-test-group/subscription" {
		t.Errorf("second call: want the subscription, got %q", paths[1])
	}
	if paths[2] != "/consumers/m9m-test-group/records" {
		t.Errorf("third call: want the record poll, got %q", paths[2])
	}

	// No background polling: the trigger must not keep talking to
	// the broker after Execute returned.
	time.Sleep(250 * time.Millisecond)
	if after := len(srv.paths()); after != 3 {
		t.Fatalf("REST calls after Execute returned: want 3 (no daemon), got %d (%v)", after, srv.paths())
	}
}

// TestKafkaTrigger_PollDecodesRecords proves the one-shot poll
// returns the broker's records as workflow items.
func TestKafkaTrigger_PollDecodesRecords(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("<balance>1</balance>"))
	srv := &kafkaRESTServer{records: []map[string]interface{}{
		{
			"value":     payload,
			"topic":     "finnet-queue",
			"key":       "exec_key",
			"headers":   map[string]interface{}{"content-type": "application/json"},
			"offset":    42,
			"partition": 0,
		},
	}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	node := NewKafkaTriggerNode()
	items, err := node.Execute(nil, map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": ts.URL,
		"topic":        "finnet-queue",
		"groupId":      "m9m-test-group",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items: want 1, got %d", len(items))
	}
	if got := items[0].JSON["value"]; got != "<balance>1</balance>" {
		t.Errorf("value: want %q, got %v", "<balance>1</balance>", got)
	}
	if got := items[0].JSON["topic"]; got != "finnet-queue" {
		t.Errorf("topic: want %q, got %v", "finnet-queue", got)
	}
	if got := items[0].JSON["key"]; got != "exec_key" {
		t.Errorf("key: want %q, got %v", "exec_key", got)
	}
}

// TestKafkaTrigger_PollHonoursMaxRecords proves the poll is bounded
// by `maxRecords`.
func TestKafkaTrigger_PollHonoursMaxRecords(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("x"))
	records := []map[string]interface{}{
		{"value": payload, "topic": "finnet-queue", "key": "k1", "offset": 1, "partition": 0},
		{"value": payload, "topic": "finnet-queue", "key": "k2", "offset": 2, "partition": 0},
		{"value": payload, "topic": "finnet-queue", "key": "k3", "offset": 3, "partition": 0},
	}
	srv := &kafkaRESTServer{records: records}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	node := NewKafkaTriggerNode()
	items, err := node.Execute(nil, map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": ts.URL,
		"topic":        "finnet-queue",
		"groupId":      "m9m-test-group",
		"maxRecords":   2,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items: want 2 (maxRecords), got %d", len(items))
	}
}

// TestKafkaTrigger_NoConsumerDaemonAPI proves the node exposes no
// consumer-daemon lifecycle surface. n8n's trigger is started and
// stopped by the runtime; m9m's trigger is a poll primitive, so
// there is deliberately no Start/Stop/Run to call — nothing can
// leave a consumer running in the background.
func TestKafkaTrigger_NoConsumerDaemonAPI(t *testing.T) {
	typ := reflect.TypeOf(&KafkaTriggerNode{})
	for _, name := range []string{"Start", "Stop", "Run", "Consume", "StartConsumer", "StopConsumer"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("Kafka trigger exposes a %q method — m9m must not run an always-on consumer daemon", name)
		}
	}
}

// TestKafkaTrigger_ExecuteRequiresTopicAndGroup proves the poll
// primitive refuses to run without the two parameters it needs.
func TestKafkaTrigger_ExecuteRequiresTopicAndGroup(t *testing.T) {
	node := NewKafkaTriggerNode()

	if _, err := node.Execute(nil, map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": "http://127.0.0.1:1",
		"groupId":      "m9m-test-group",
	}); err == nil {
		t.Fatal("expected an error when the topic is missing")
	}

	if _, err := node.Execute(nil, map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": "http://127.0.0.1:1",
		"topic":        "finnet-queue",
	}); err == nil {
		t.Fatal("expected an error when the groupId is missing")
	}
}

// TestKafkaTrigger_ConsumerExistsIsTolerated proves a poll against a
// proxy that already knows the consumer (409) still reads records —
// the poll is idempotent across ticks.
func TestKafkaTrigger_ConsumerExistsIsTolerated(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("payload"))
	srv := &kafkaRESTServer{
		consumerExists: true,
		records: []map[string]interface{}{
			{"value": payload, "topic": "finnet-queue", "key": "k", "offset": 7, "partition": 0},
		},
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	node := NewKafkaTriggerNode()
	items, err := node.Execute(nil, map[string]interface{}{
		"mode":         "rest",
		"restProxyUrl": ts.URL,
		"topic":        "finnet-queue",
		"groupId":      "m9m-test-group",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items: want 1, got %d", len(items))
	}
	if got := items[0].JSON["value"]; got != "payload" {
		t.Errorf("value: want %q, got %v", "payload", got)
	}
}
