package webhooks

import (
	"reflect"
	"testing"
)

// TestUnwrapRespondToWebhookBody pins the contract that the
// Respond-to-Webhook node's `{data: STRING}` envelope is unwrapped
// before being written to the HTTP response. This is what makes
// n8n-style XML / text passthrough (a workflow that runs
// `respondWith: "json"` with `={{ $json.data.toJsonString() }}`)
// emit the raw string body instead of a JSON-wrapped envelope.
func TestUnwrapRespondToWebhookBody(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want interface{}
	}{
		{
			name: "single string data key unwraps to the raw string",
			in:   map[string]interface{}{"data": "<xml/>"},
			want: "<xml/>",
		},
		{
			name: "multi-key object is returned as-is",
			in:   map[string]interface{}{"foo": "bar", "data": "ignored"},
			want: map[string]interface{}{"foo": "bar", "data": "ignored"},
		},
		{
			name: "non-string data is returned as-is",
			in:   map[string]interface{}{"data": map[string]interface{}{"nested": true}},
			want: map[string]interface{}{"data": map[string]interface{}{"nested": true}},
		},
		{
			name: "object without data key is returned as-is",
			in:   map[string]interface{}{"hello": "world"},
			want: map[string]interface{}{"hello": "world"},
		},
		{
			name: "empty object is returned as-is",
			in:   map[string]interface{}{},
			want: map[string]interface{}{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unwrapRespondToWebhookBody(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unwrapRespondToWebhookBody(%v) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}
