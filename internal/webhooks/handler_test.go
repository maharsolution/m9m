package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin down the wire shape of the HTTP response body the
// Webhook handler writes for the cardinal cases n8n users hit:
//   - firstEntryJson → bare object (`{...}`)
//   - allEntries     → array (`[{...},{...},...]`)
//   - 0 items        → `{"message":"success"}` (success object)
//   - noData         → `{"message":"success"}` (success object)
//
// n8n's Webhook response contract is workflow-shape dependent:
//   - With responseData unset (defaults to firstEntryJson) n8n emits
//     the first item's JSON as a bare object — verified live against
//     http://187.77.113.218:5678/webhook/simple_webhook.
//   - With responseData="allEntries" n8n emits every item as a JSON
//     array — verified live against
//     http://187.77.113.218:5678/webhook/bocahtuanakal.
//   - With responseData="noData" n8n emits an empty 200 body.
//
// Earlier m9m always wrapped in a single-element array `[{...}]`,
// which mismatched n8n for workflows without downstream Set nodes.
// The helper below exercises the same code path as
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

func TestHandler_ResponseShape_FirstEntryJson_EmitsBareObject(t *testing.T) {
	// Simulate the "My workflow" Set-after-Webhook case. The Set node
	// at the end of the workflow accumulates all upstream Webhook
	// trigger fields (body/headers/params/...) plus its own
	// assignment (total). With `responseData: firstEntryJson` the
	// handler must emit the first item's JSON as a bare object —
	// matching n8n's wire shape for the default responseData.
	wh := &Webhook{ResponseData: "firstEntryJson"}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{
				JSON: map[string]interface{}{
					"body":    map[string]interface{}{"varA": "5", "varB": "10"},
					"headers": map[string]interface{}{"content-type": "application/json"},
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

	// Body must parse as a bare JSON object (NOT a single-element
	// array). firstEntryJson's contract is to emit the first item's
	// JSON verbatim.
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &obj), "response body must be valid JSON")
	require.Empty(t, obj[""], "firstEntryJson must NOT wrap the item in an array")

	// The merged upstream fields MUST be preserved (n8n keeps them too)
	// and the final node's assignment (`total`) must be present.
	assert.Equal(t, 15.0, obj["total"], "final Set node's `total` field must be preserved")
	assert.NotNil(t, obj["body"], "webhook-internal `body` field must not be stripped")
	assert.NotNil(t, obj["headers"], "webhook-internal `headers` field must not be stripped")
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
	// Webhook node configured — switching the body shape (bare object vs
	// array) is a body change, not a headers change.
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

	// Body must be a bare JSON object despite the custom headers.
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &obj))
	assert.Equal(t, 3.0, obj["total"])
}

func TestHandler_ResponseShape_DefaultUnknownResponseData_EmitsBareObject(t *testing.T) {
	// When the ResponseData field holds an unrecognised value, we fall
	// through to the default branch — and that default must emit a bare
	// object to match n8n's firstEntryJson default.
	wh := &Webhook{ResponseData: "someUnknownMode"}
	result := &engine.ExecutionResult{
		Data: []model.DataItem{
			{JSON: map[string]interface{}{"ok": true}},
		},
	}

	rec, body := runHandlerForResult(t, wh, result)

	assert.Equal(t, http.StatusOK, rec.Code)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &obj), "default branch must produce a bare JSON object")
	assert.Equal(t, true, obj["ok"])
}

// ---------------------------------------------------------------------------
// Error response wire shape — pins the gap #2 cosmetic fix.
// ---------------------------------------------------------------------------
//
// n8n's webhook surface returns JSON errors with the shape
//
//	{"code":<status>,"message":"...","hint":"..."}
//
// so callers can branch on the numeric code without parsing the
// message. m9m previously used http.Error which produced
// text/plain bodies — diverging from n8n and breaking JSON clients.
// These tests pin the wire shape of m9m's 404 / 401 / 400 responses
// against the router path, which is the only path real HTTP requests
// exercise in production (mux.Vars is only populated when the request
// flows through the router).

// TestHandler_NotFound_ReturnsJSONMatchingN8n asserts that a POST to
// an unknown webhook path returns the n8n-shaped JSON 404, not the
// pre-fix plain-text body. Verified against the live
// http://187.77.113.218:5678/webhook/nonexistent_path response.
func TestHandler_NotFound_ReturnsJSONMatchingN8n(t *testing.T) {
	mgr, _, _ := newTestManager()
	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/nonexistent_path",
		bytes.NewReader([]byte(`{"x":1}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"),
		"404 must use application/json, not text/plain (gap #2 fix)")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"404 body must be valid JSON")

	// Field presence + types — the three fields n8n emits.
	require.Contains(t, body, "code")
	require.Contains(t, body, "message")
	require.Contains(t, body, "hint")

	assert.Equal(t, float64(http.StatusNotFound), body["code"],
		"code field must equal the HTTP status numerically")
	assert.Equal(t, "string", fmt.Sprintf("%T", body["message"]),
		"message field must be a string")
	assert.Equal(t, "string", fmt.Sprintf("%T", body["hint"]),
		"hint field must be a string")

	// Message must reference the method + path the caller requested,
	// matching n8n's wire format: `"POST nonexistent_path"`.
	assert.Contains(t, body["message"], "POST nonexistent_path",
		"message must reference the request method and path")
}

// TestHandler_NotFound_TestPath_AlsoReturnsJSON ensures the test
// webhook route (/webhook-test/...) emits the same JSON 404 shape as
// the production route — cosmetic consistency, but a regression here
// would surface in the editor's "Listen for Test Event" panel.
func TestHandler_NotFound_TestPath_AlsoReturnsJSON(t *testing.T) {
	mgr, _, _ := newTestManager()
	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhook-test/missing",
		nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusNotFound), body["code"])
	assert.Contains(t, body["message"], "GET missing")
}

// TestHandler_Unauthorized_ReturnsJSON pins the 401 wire shape for
// webhooks that have authType="basic" with a configured username and
// password — when the caller omits the Authorization header the
// handler must surface a JSON 401, not text/plain.
func TestHandler_Unauthorized_ReturnsJSON(t *testing.T) {
	mgr, _, _ := newTestManager()

	wh := &Webhook{
		ID:         "wh-basic",
		WorkflowID: "wf-1",
		Path:       "/basic",
		Method:     "POST",
		Active:     true,
		AuthType:   "basic",
		AuthData: map[string]interface{}{
			"username": "user",
			"password": "pass",
		},
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	// No Authorization header at all — handler must 401.
	req := httptest.NewRequest(http.MethodPost, "/webhook/basic",
		bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"),
		"401 must use application/json, not text/plain")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusUnauthorized), body["code"])
	assert.Equal(t, "Authentication failed", body["message"])
	assert.NotEmpty(t, body["hint"], "hint must provide remediation guidance")
}

// TestHandler_Unauthorized_WrongPassword_ReturnsJSON exercises the
// same wire shape for the case where credentials are provided but
// invalid. Without this, callers parsing m9m 401s would silently
// treat "credentials were wrong" and "credentials were missing" as
// the same text/plain body.
func TestHandler_Unauthorized_WrongPassword_ReturnsJSON(t *testing.T) {
	mgr, _, _ := newTestManager()

	wh := &Webhook{
		ID:         "wh-basic-bad",
		WorkflowID: "wf-1",
		Path:       "/basic-bad",
		Method:     "POST",
		Active:     true,
		AuthType:   "basic",
		AuthData: map[string]interface{}{
			"username": "user",
			"password": "pass",
		},
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/basic-bad",
		bytes.NewReader([]byte(`{}`)))
	req.SetBasicAuth("user", "wrongpass")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusUnauthorized), body["code"])
	assert.Equal(t, "Authentication failed", body["message"])
}

// TestHandler_Unauthorized_HeaderAuth_ReturnsJSON covers the
// `authType: header` (custom header auth) path — the same JSON 401
// must come back whether the caller uses basic auth or a custom
// header. n8n's webhook 401 wire shape is the same regardless of
// which auth scheme failed.
func TestHandler_Unauthorized_HeaderAuth_ReturnsJSON(t *testing.T) {
	mgr, _, _ := newTestManager()

	wh := &Webhook{
		ID:         "wh-header",
		WorkflowID: "wf-1",
		Path:       "/hdr",
		Method:     "POST",
		Active:     true,
		AuthType:   "header",
		AuthData: map[string]interface{}{
			"headerName":  "X-Custom-Secret",
			"headerValue": "s3cret",
		},
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/hdr",
		bytes.NewReader([]byte(`{}`)))
	// No X-Custom-Secret header — handler must 401.
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusUnauthorized), body["code"])
}

// TestHandler_BadRequest_MalformedJSON_ReturnsJSON pins the 400 wire
// shape for callers that POST a body claiming Content-Type:
// application/json but whose body is not valid JSON. n8n responds
// with a JSON 400 — m9m must match.
func TestHandler_BadRequest_MalformedJSON_ReturnsJSON(t *testing.T) {
	mgr, _, ws := newTestManager()

	// Need a real workflow so the handler reaches the parseRequest
	// path (auth passes first because AuthType="" by default).
	wf := &model.Workflow{ID: "wf-bad-json", Name: "bad-json", Active: true, Nodes: []model.Node{}}
	require.NoError(t, ws.SaveWorkflow(wf))

	wh := &Webhook{
		ID:         "wh-bad-json",
		WorkflowID: wf.ID,
		Path:       "/bad-json",
		Method:     "POST",
		Active:     true,
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	// Valid Content-Type header so the parseRequest branch routes to
	// the JSON decoder, but the body is malformed JSON.
	req := httptest.NewRequest(http.MethodPost, "/webhook/bad-json",
		bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"),
		"400 must use application/json, not text/plain")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusBadRequest), body["code"])
	assert.Equal(t, "Invalid request", body["message"])
	assert.NotEmpty(t, body["hint"], "hint must provide remediation guidance")
}

// TestHandler_BadRequest_MalformedForm_ReturnsJSON covers the
// application/x-www-form-urlencoded bad-body path — same wire shape
// as the JSON 400.
func TestHandler_BadRequest_MalformedForm_ReturnsJSON(t *testing.T) {
	mgr, _, ws := newTestManager()

	wf := &model.Workflow{ID: "wf-bad-form", Name: "bad-form", Active: true, Nodes: []model.Node{}}
	require.NoError(t, ws.SaveWorkflow(wf))

	wh := &Webhook{
		ID:         "wh-bad-form",
		WorkflowID: wf.ID,
		Path:       "/bad-form",
		Method:     "POST",
		Active:     true,
	}
	require.NoError(t, mgr.RegisterWebhook(wh))

	h := NewHandler(mgr)
	router := newHandlerRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhook/bad-form",
		bytes.NewReader([]byte(`a=%`))) // % is an invalid percent-encoded byte
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(http.StatusBadRequest), body["code"])
	assert.Equal(t, "Invalid request", body["message"])
}