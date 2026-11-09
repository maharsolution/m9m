package trigger

import (
	"testing"
)

// TestKafkaTriggerValidate_AutoDetectNativeFromBrokers covers the
// credential shape an Aiven / Confluent Cloud Kafka credential
// produces: `brokers` + `username` + `password` + `ca`, with no
// `mode` and no `restProxyUrl`.
//
// Before the fix the trigger defaulted to `mode=rest` and the
// validation rejected the node with "restProxyUrl is required when
// mode=rest" — which is exactly the error the "Integrasi Aiven"
// workflow failed with.
func TestKafkaTriggerValidate_AutoDetectNativeFromBrokers(t *testing.T) {
	node := NewKafkaTriggerNode()

	params := map[string]interface{}{
		"kafka_brokers":  "kafka.example.aivencloud.com:20475",
		"kafka_username": "avnadmin",
		"kafka_password": "secret",
		"kafka_ca":       "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
		"topic":          "topic-data-example",
		"groupId":        "n8n-kafka",
		"options":        map[string]interface{}{"autoCommitInterval": 0, "fromBeginning": true},
	}

	// The credential envelope must be folded into the explicit param
	// shape before validation, exactly like at Execute time.
	if err := node.ValidateParameters(withKafkaEnvelope(params)); err != nil {
		t.Fatalf("expected broker-only credential to validate, got: %v", err)
	}
}

// TestKafkaTriggerValidate_RestStillRequiresProxy guards the
// REST-proxy path: with neither brokers nor a restProxyUrl the node
// must still be rejected.
func TestKafkaTriggerValidate_RestStillRequiresProxy(t *testing.T) {
	node := NewKafkaTriggerNode()

	params := map[string]interface{}{
		"topic":   "topic-data-example",
		"groupId": "n8n-kafka",
	}

	if err := node.ValidateParameters(params); err == nil {
		t.Fatal("expected error when neither brokers nor restProxyUrl is set")
	}
}

// TestStartOffsetFromParams_FromBeginningOption covers the older n8n
// export shape that carries `options.fromBeginning: true` instead of
// the newer `startPosition` parameter.
func TestStartOffsetFromParams_FromBeginningOption(t *testing.T) {
	first := startOffsetFromParams(map[string]interface{}{
		"options": map[string]interface{}{"fromBeginning": true},
	})
	if first != kafkaFirstOffset {
		t.Errorf("expected fromBeginning=true to map to FirstOffset, got %d", first)
	}

	last := startOffsetFromParams(map[string]interface{}{
		"options": map[string]interface{}{"fromBeginning": false},
	})
	if last != kafkaLastOffset {
		t.Errorf("expected fromBeginning=false to map to LastOffset, got %d", last)
	}
}

// TestStartOffsetFromParams_StartPositionWins verifies the explicit
// `startPosition` parameter still takes precedence over the legacy
// `options.fromBeginning` flag.
func TestStartOffsetFromParams_StartPositionWins(t *testing.T) {
	got := startOffsetFromParams(map[string]interface{}{
		"startPosition": "last",
		"options":       map[string]interface{}{"fromBeginning": true},
	})
	if got != kafkaLastOffset {
		t.Errorf("expected startPosition=last to win, got %d", got)
	}
}

// withKafkaEnvelope mimics the engine's credential injection: it
// writes the credential fields under the `kafka_<field>` keys that
// `messaging.ApplyKafkaCredentialAuth` folds back in.
func withKafkaEnvelope(params map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		out[k] = v
	}
	for k, v := range params {
		if k == "brokers" || k == "username" || k == "password" || k == "ca" {
			out["kafka_"+k] = v
			delete(out, k)
		}
	}
	return out
}
