package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

func TestHTTPRequestNodeCreation(t *testing.T) {
	node := NewHTTPRequestNode()
	if node == nil {
		t.Fatal("Expected node to be created, got nil")
	}
	
	desc := node.Description()
	if desc.Name != "HTTP Request" {
		t.Errorf("Expected name 'HTTP Request', got '%s'", desc.Name)
	}
}

func TestHTTPRequestNodeValidateParameters(t *testing.T) {
	node := NewHTTPRequestNode()
	
	// Test with nil params
	err := node.ValidateParameters(nil)
	if err == nil {
		t.Error("Expected error with nil params, got nil")
	}
	
	// Test with empty params
	err = node.ValidateParameters(make(map[string]interface{}))
	if err == nil {
		t.Error("Expected error with empty params, got nil")
	}
	
	// Test with valid params
	params := map[string]interface{}{
		"url":    "https://api.example.com",
		"method": "GET",
	}
	
	err = node.ValidateParameters(params)
	if err != nil {
		t.Errorf("Expected no error with valid params, got %v", err)
	}
	
	// Test with invalid method
	invalidParams := map[string]interface{}{
		"url":    "https://api.example.com",
		"method": "INVALID",
	}
	
	err = node.ValidateParameters(invalidParams)
	if err == nil {
		t.Error("Expected error with invalid method, got nil")
	}
}

func TestHTTPRequestNodeExecuteWithMissingURL(t *testing.T) {
	node := NewHTTPRequestNode()
	
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"method": "GET",
			},
		},
	}
	
	nodeParams := map[string]interface{}{
		"method": "GET",
	}
	
	_, err := node.Execute(inputData, nodeParams)
	if err == nil {
		t.Error("Expected error with missing URL, got nil")
	}
}

func TestHTTPRequestNodeExecuteWithValidRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "success",
			"data":    "test",
		})
	}))
	defer server.Close()
	
	node := NewHTTPRequestNode()
	
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{},
		},
	}
	
	nodeParams := map[string]interface{}{
		"url":    server.URL,
		"method": "GET",
		// SECURITY: Allow internal requests for testing only
		"allowInternalRequests": true,
	}

	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}

	responseData := result[0]

	// Check status code
	statusCode, ok := responseData.JSON["statusCode"]
	if !ok {
		t.Fatal("Expected statusCode to be present")
	}
	
	// Convert to int for comparison
	var statusCodeInt int
	switch v := statusCode.(type) {
	case float64:
		statusCodeInt = int(v)
	case int:
		statusCodeInt = v
	default:
		t.Fatalf("Unexpected status code type: %T", statusCode)
	}
	
	if statusCodeInt != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, statusCodeInt)
	}
	
	// Check that JSON response is parsed
	jsonData, ok := responseData.JSON["json"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected JSON response to be parsed")
	}
	
	message, ok := jsonData["message"].(string)
	if !ok || message != "success" {
		t.Errorf("Expected message 'success', got '%v'", message)
	}
}

func TestHTTPRequestNodeExecutePostRequest(t *testing.T) {
	// Create a test server
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}
		receivedBody = string(body)
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"received": string(body),
		})
	}))
	defer server.Close()
	
	node := NewHTTPRequestNode()
	
	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{},
		},
	}
	
	nodeParams := map[string]interface{}{
		"url":    server.URL,
		"method": "POST",
		"body": map[string]interface{}{
			"name": "test",
			"age":  30,
		},
		// SECURITY: Allow internal requests for testing only
		"allowInternalRequests": true,
	}
	
	result, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	if len(result) != 1 {
		t.Fatalf("Expected 1 result item, got %d", len(result))
	}
	
	// Check that we received the body
	if receivedBody == "" {
		t.Error("Expected to receive body data")
	}
	
	responseData := result[0]
	
	// Check status code
	statusCode, ok := responseData.JSON["statusCode"]
	if !ok {
		t.Fatal("Expected statusCode to be present")
	}
	
	// Convert to int for comparison
	var statusCodeInt int
	switch v := statusCode.(type) {
	case float64:
		statusCodeInt = int(v)
	case int:
		statusCodeInt = v
	default:
		t.Fatalf("Unexpected status code type: %T", statusCode)
	}
	
	if statusCodeInt != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, statusCodeInt)
	}
}

func TestHTTPRequestNodeImplementsNodeExecutor(t *testing.T) {
	var _ base.NodeExecutor = (*HTTPRequestNode)(nil)
}

// TestApplyCredentialAuth_BasicAuth — verifies the engine's
// injected `httpBasicAuth_user` / `httpBasicAuth_password` keys
// are translated into a real `Authorization: Basic ...` header
// on the outgoing request. This is the BUG-2026-09-09-002 fix
// for positive/3 (webhook_callrest).
func TestApplyCredentialAuth_BasicAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpBasicAuth_user":     "alice",
		"httpBasicAuth_password": "s3cret",
	}
	if err := applyCredentialAuth(req, params); err != nil {
		t.Fatalf("applyCredentialAuth: %v", err)
	}
	u, p, ok := req.BasicAuth()
	if !ok {
		t.Fatalf("expected BasicAuth to be set, got none")
	}
	if u != "alice" || p != "s3cret" {
		t.Errorf("got user=%q pass=%q, want alice/s3cret", u, p)
	}
}

// TestApplyCredentialAuth_HeaderAuth — verifies the engine's
// injected `httpHeaderAuth_name` / `httpHeaderAuth_value` keys
// become a real custom header (the most common shape: an API
// key sent as `X-API-Key: ...`).
func TestApplyCredentialAuth_HeaderAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpHeaderAuth_name":  "X-API-Key",
		"httpHeaderAuth_value": "my-key",
	}
	if err := applyCredentialAuth(req, params); err != nil {
		t.Fatalf("applyCredentialAuth: %v", err)
	}
	if got := req.Header.Get("X-API-Key"); got != "my-key" {
		t.Errorf("X-API-Key = %q, want my-key", got)
	}
}

// TestApplyCredentialAuth_ApiKey — verifies the bare
// `apiKey_apiKey` field becomes `Authorization: Bearer ...`.
func TestApplyCredentialAuth_ApiKey(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"apiKey_apiKey": "sk-123",
	}
	if err := applyCredentialAuth(req, params); err != nil {
		t.Fatalf("applyCredentialAuth: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-123" {
		t.Errorf("Authorization = %q, want Bearer sk-123", got)
	}
}

// TestApplyCredentialAuth_NoCredential — anonymous request
// must NOT error. Many HTTP Request nodes legitimately have
// no auth.
func TestApplyCredentialAuth_NoCredential(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	if err := applyCredentialAuth(req, map[string]interface{}{}); err != nil {
		t.Errorf("anonymous request should not error, got %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("anonymous request should not have Authorization header")
	}
}

// TestApplyCredentialAuth_HalfPresentBasicAuth — if the engine
// injects a half-pair (only one of user/password), we want a
// loud failure, not a silent anonymous call.
func TestApplyCredentialAuth_HalfPresentBasicAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://upstream.test/x", nil)
	params := map[string]interface{}{
		"httpBasicAuth_user": "alice",
		// httpBasicAuth_password missing on purpose.
	}
	if err := applyCredentialAuth(req, params); err == nil {
		t.Errorf("expected error for half-present basic auth, got nil")
	}
}

// TestHTTPRequestNode_AppliesBasicAuth — end-to-end check
// against a local test server. Spins up an httptest server that
// requires Basic auth, points an HTTP Request node at it, and
// asserts the request reached it with the right credentials.
func TestHTTPRequestNode_AppliesBasicAuth(t *testing.T) {
	var seenUser, seenPass string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		seenUser, seenPass = u, p
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	node := NewHTTPRequestNode()
	params := map[string]interface{}{
		"url":                     server.URL + "/protected",
		"method":                  "GET",
		"httpBasicAuth_user":      "alice",
		"httpBasicAuth_password":  "s3cret",
	}
	// evaluatedParams is what the engine hands to Execute;
	// in real flows the credential inject happens upstream.
	out, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if seenUser != "alice" || seenPass != "s3cret" {
		t.Errorf("upstream saw user=%q pass=%q, want alice/s3cret", seenUser, seenPass)
	}
}

// TestHTTPRequestNode_AppliesHeaderAuth — same as the basic
// auth test, but exercises the `httpHeaderAuth_name` /
// `httpHeaderAuth_value` path.
func TestHTTPRequestNode_AppliesHeaderAuth(t *testing.T) {
	var seenValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenValue = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	node := NewHTTPRequestNode()
	params := map[string]interface{}{
		"url":                 server.URL + "/protected",
		"method":              "GET",
		"httpHeaderAuth_name": "X-API-Key",
		"httpHeaderAuth_value": "my-key",
	}
	if _, err := node.Execute([]model.DataItem{{JSON: map[string]interface{}{}}}, params); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if seenValue != "my-key" {
		t.Errorf("upstream saw X-API-Key=%q, want my-key", seenValue)
	}
}