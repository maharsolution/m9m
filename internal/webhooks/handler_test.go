package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin down the wire shape of the HTTP response body the
// Webhook handler writes for the three cardinal cases n8n users hit:
//   - 1 item  → `[{...}]`
//   - N items → `[{...},{...},...]`
//   - 0 items → `[]` (an empty array)
//
// n8n's Webhook response contract always serialises the last node's
// output as a JSON array — even with `responseData: firstEntryJson` the
// body is a single-element array. Earlier m9m shipped the body as a
// bare object (e.g. `{"body":...,"headers":...,"total":15}`) which
// breaks every n8n client that does `JSON.parse(...) as Array`. The
// helper below exercises the same code path as
// `handler.handleWebhookRequest` for the response-writing half, so a
// regression in either `prepareResponse` or `sendResponse` shows up
// here.

// runHandlerForResult drives the response-writing half of the webhook
// pipeline against an in-memory recorder. It uses the real Manager's
// `prepareResponse` (which builds the WebhookResponse from an
// ExecutionResult) and the real Handler's `sendResponse` (which writes
// to an http.ResponseWriter), so the assertions below catch issues in
// either half. Auth, request parsing and storage are bypassed — those
// are covered by webhooks_test.go.
func runHandlerForResult(t *testing.T, wh *Webhook, result *engine.ExecutionResult) (*httptest.ResponseRecorder, []byte) {
	t.Helper()

	mgr, _, _ := newTestManager()
	h := NewHandler(mgr)

	resp := mgr.prepareResponse(wh, result)

	rec := httptest.NewRecorder()
	h.sendResponse(rec, resp)

	body := rec.Body.Bytes()
	return rec, body
}

func TestHandler_ResponseShape_FirstEntryJson_WrapsAsArray(t *testing.T) {
	// Simulate the "My workflow" Set-after-Webhook case from the bug
	// report. The Set node at the end of the workflow accumulates all
	// upstream Webhook trigger fields (body/headers/params/...) plus its
	// own assignment (total). The handler must wrap this object in a
	// single-element array so the wire shape matches n8n's `[{...}]`.
	wh := &Webhook{ResponseData: "firstEntryJson"}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{
				JSON: map[string]interface{}{
					"body":    map[string]interface{}{"varA": "5", "varB": "10"},
					"headers": map[string]interface{}{"Content-Type": "application/json"},
					"params":  map[string]interface{}{},
					"query":   map[string]interface{}{},
					"method":  "POST",
					"path":    "/webhook/my",
					"total":   15.0,
				},
			},
		},
	}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// Body must parse as a JSON array of length 1.
	var arr []map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &arr), "response body must be valid JSON")
	require.Len(t, arr, 1, "firstEntryJson must wrap output in a single-element JSON array")

	// The merged upstream fields MUST be preserved (n8n keeps them too)
	// and the final node's assignment (`total`) must be present.
	item := arr[0]
	assert.Equal(t, 15.0, item["total"], "final Set node's `total` field must be preserved")
	assert.NotNil(t, item["body"], "webhook-internal `body` field must not be stripped")
	assert.NotNil(t, item["headers"], "webhook-internal `headers` field must not be stripped")
}

func TestHandler_ResponseShape_AllEntries_WrapsAsArray(t *testing.T) {
	// When the workflow yields multiple items the handler must return
	// every one of them in a single JSON array, preserving order.
	wh := &Webhook{ResponseData: "allEntries"}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{JSON: map[string]interface{}{"i": 0, "payload": "first"}},
			{JSON: map[string]interface{}{"i": 1, "payload": "second"}},
			{JSON: map[string]interface{}{"i": 2, "payload": "third"}},
		},
	}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var arr []map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &arr), "response body must be valid JSON")
	require.Len(t, arr, 3, "allEntries must return every item")

	for idx, item := range arr {
		assert.Equal(t, float64(idx), item["i"])
	}
}

func TestHandler_ResponseShape_Empty_FirstEntryJson_ReturnsSuccessObject(t *testing.T) {
	// With zero items and firstEntryJson, n8n returns
	// `{"message":"Workflow started"}`-style success object — we follow
	// the existing convention of returning `{"message":"success"}`. The
	// test pins this behaviour so any future change is deliberate.
	wh := &Webhook{ResponseData: "firstEntryJson"}
	result := &engine.ExecutionResult{Data: []model.DataItem{}}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &obj), "response body must be valid JSON")
	assert.Equal(t, "success", obj["message"])
}

func TestHandler_ResponseShape_PreservesCustomHeaders(t *testing.T) {
	// The handler must still forward any custom response headers that the
	// Webhook node configured — wrapping the body in an array is a body
	// change, not a headers change.
	wh := &Webhook{
		ResponseData:    "firstEntryJson",
		ResponseHeaders: map[string]string{"X-Custom": "abc", "X-Trace": "xyz"},
	}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{JSON: map[string]interface{}{"total": 3.0}},
		},
	}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "abc", rec.Header().Get("X-Custom"))
	assert.Equal(t, "xyz", rec.Header().Get("X-Trace"))

	// Body must still be a JSON array despite the custom headers.
	var arr []map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &arr))
	require.Len(t, arr, 1)
	assert.Equal(t, 3.0, arr[0]["total"])
}

func TestHandler_ResponseShape_DefaultUnknownResponseData_WrapsAsArray(t *testing.T) {
	// When the ResponseData field holds an unrecognised value, we fall
	// through to the default branch — and that default must also wrap in
	// an array to stay consistent with n8n's contract.
	wh := &Webhook{ResponseData: "someUnknownMode"}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{JSON: map[string]interface{}{"ok": true}},
		},
	}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)

	var arr []map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &arr), "default branch must still produce a JSON array")
	require.Len(t, arr, 1)
	assert.Equal(t, true, arr[0]["ok"])
}