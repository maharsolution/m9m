package webhooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveContentType_NodeOverrideWins verifies that an explicit
// `options.responseHeaders.entries` content-type on the
// Respond-to-Webhook node takes precedence over everything else,
// including inbound Content-Type. This is the path that makes the
// `webhook_xml` workflow (`OFVi8L0zRs3Cnkca`) round-trip an XML body
// back to the caller with the same wire shape n8n emits.
func TestResolveContentType_NodeOverrideWins(t *testing.T) {
	params := map[string]interface{}{
		"respondWith": "json",
		"options": map[string]interface{}{
			"responseHeaders": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"name":  "content-type",
						"value": "application/xml",
					},
				},
			},
		},
	}
	// Even if the inbound says JSON, the node override wins.
	got := resolveContentType(params, "application/json", `<?xml version="1.0"?><root/>`)
	assert.Equal(t, "application/xml", got)
}

// TestResolveContentType_InboundXMLMirrored verifies that, when no
// node override is present, an inbound `application/xml` is echoed
// for an XML-shaped body. This is the parity fix for the user's
// manual curl on 2026-09-09 where n8n echoed application/xml while
// m9m hardcoded application/json.
func TestResolveContentType_InboundXMLMirrored(t *testing.T) {
	got := resolveContentType(
		nil,
		"application/xml",
		`<?xml version="1.0" encoding="UTF-8"?><buku><id>001</id></buku>`,
	)
	assert.Equal(t, "application/xml", got)
}

// TestResolveContentType_InboundXMLMirrored_LegacyShape is the same
// as above but for the legacy direct-map shape n8n's older
// versions emit: `responseHeaders = { "Content-Type":
// "application/xml" }` instead of the `entries[]` form.
func TestResolveContentType_InboundXMLMirrored_TextPlain(t *testing.T) {
	got := resolveContentType(
		map[string]interface{}{
			"options": map[string]interface{}{
				"responseHeaders": map[string]interface{}{
					"Content-Type": "text/plain",
				},
			},
		},
		"text/plain",
		"plain text body without angle bracket",
	)
	assert.Equal(t, "text/plain", got)
}

// TestResolveContentType_JSONFallbackUnaffected verifies the JSON
// fallback path: when the response body is JSON-shaped (a map or
// nil) and there is no per-node override, the function returns
// application/json even if the inbound said XML. JSON callers must
// keep the same wire shape they had before this fix.
func TestResolveContentType_JSONFallbackUnaffected(t *testing.T) {
	got := resolveContentType(
		nil,
		"application/xml",
		map[string]interface{}{"result": 1.0},
	)
	assert.Equal(t, "application/json", got)
}

// TestResolveContentType_NodeOverridesJSON ensures the node-level
// override path can ALSO produce non-JSON response Content-Type
// for a JSON-shaped body — the explicit per-node header is the
// highest authority even if the body is JSON.
func TestResolveContentType_NodeOverridesJSON(t *testing.T) {
	params := map[string]interface{}{
		"options": map[string]interface{}{
			"responseHeaders": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"name":  "Content-Type",
						"value": "text/csv",
					},
				},
			},
		},
	}
	got := resolveContentType(params, "application/json", map[string]interface{}{"foo": "bar"})
	assert.Equal(t, "text/csv", got)
}

// TestResolveContentType_EmptyBodyDefaultsToJSON verifies the
// degenerate case: empty body, no override, no inbound header.
// JSON fallback should kick in so callers always see a parseable
// response shape.
func TestResolveContentType_EmptyBodyDefaultsToJSON(t *testing.T) {
	got := resolveContentType(nil, "", map[string]interface{}{"message": "success"})
	assert.Equal(t, "application/json", got)
}

// TestReadRespondToWebhookHeaders_Empty verifies graceful handling
// of the absent-options and absent-responseHeaders branches.
func TestReadRespondToWebhookHeaders_Empty(t *testing.T) {
	assert.Equal(t, map[string]string{}, readRespondToWebhookHeaders(nil))
	assert.Equal(t, map[string]string{}, readRespondToWebhookHeaders(map[string]interface{}{}))
	optsOnly := map[string]interface{}{"options": map[string]interface{}{}}
	assert.Equal(t, map[string]string{}, readRespondToWebhookHeaders(optsOnly))
}

// TestReadRespondToWebhookHeaders_LegacyShape verifies the direct-map
// shape is accepted (older n8n versions emit it).
func TestReadRespondToWebhookHeaders_LegacyShape(t *testing.T) {
	h := readRespondToWebhookHeaders(map[string]interface{}{
		"options": map[string]interface{}{
			"responseHeaders": map[string]interface{}{
				"Content-Type": "application/xml",
				"X-Foo":        "bar",
			},
		},
	})
	assert.Equal(t, "application/xml", h["content-type"])
	assert.Equal(t, "bar", h["x-foo"])
}

// TestFirstHeaderValueCaseInsensitive verifies the helper normalises
// header lookup so callers can pass "Content-Type" rather than
// remembering Go's canonical-case map key. Duplicate values collapse
// to the first.
func TestFirstHeaderValueCaseInsensitive(t *testing.T) {
	headers := map[string][]string{
		"Content-Type": {"application/xml", "application/json"},
	}
	assert.Equal(t, "application/xml", firstHeaderValue(headers, "Content-Type"))
	assert.Equal(t, "application/xml", firstHeaderValue(headers, "content-type"))
	assert.Equal(t, "", firstHeaderValue(headers, "X-Missing"))
	assert.Equal(t, "", firstHeaderValue(nil, "Content-Type"))
}
