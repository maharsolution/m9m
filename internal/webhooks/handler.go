package webhooks

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// Handler handles incoming webhook HTTP requests
type Handler struct {
	manager *WebhookManager
}

// NewHandler creates a new webhook handler
func NewHandler(manager *WebhookManager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// RegisterRoutes registers webhook routes on the router
func (h *Handler) RegisterRoutes(router *mux.Router) {
	// Test webhooks (for workflow testing)
	router.HandleFunc("/api/v1/webhooks/test/{path:.*}", h.HandleTestWebhook).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS")

	// Production webhooks
	router.HandleFunc("/webhook/{path:.*}", h.HandleProductionWebhook).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS")
	router.HandleFunc("/webhook-test/{path:.*}", h.HandleTestWebhook).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS")

	// Webhook management API
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/webhooks", h.ListWebhooks).Methods("GET", "OPTIONS")
	api.HandleFunc("/webhooks", h.CreateWebhook).Methods("POST", "OPTIONS")
	api.HandleFunc("/webhooks/{id}", h.GetWebhook).Methods("GET", "OPTIONS")
	api.HandleFunc("/webhooks/{id}", h.DeleteWebhook).Methods("DELETE", "OPTIONS")
}

// HandleProductionWebhook handles production webhook requests
func (h *Handler) HandleProductionWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhookRequest(w, r, false)
}

// HandleTestWebhook handles test webhook requests
func (h *Handler) HandleTestWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhookRequest(w, r, true)
}

// Webhook error response wire shape — mirrors n8n's webhook surface so
// callers can parse m9m errors with the same code they already use for
// n8n. n8n emits three keys for its webhook 404:
//
//	{"code":404,"message":"...","hint":"..."}
//
// Verified live against http://187.77.113.218:5678/webhook/<missing>
// 2026-09-03. We use the same keys (and key order) for 401/400 so
// callers that only special-case the `code` field keep working, and the
// `hint` field can carry actionable remediation text without forcing a
// new schema on the wire.
type webhookErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// writeJSONError serialises a {code,message,hint} error body and writes
// it with the requested HTTP status. It deliberately does NOT call
// http.Error (which writes text/plain) so callers always receive a
// JSON-parseable error body — matching n8n's webhook surface.
//
// The body is written with json.Marshal (not json.Encoder.Encode) so
// the trailing newline that Encoder appends does not break byte-equal
// diffing with n8n's wire output.
func writeJSONError(w http.ResponseWriter, status int, body webhookErrorBody) {
	data, err := json.Marshal(body)
	if err != nil {
		// Fall back to a minimal valid JSON body so the caller always
		// gets parseable output even if marshalling fails (it should
		// never fail for this struct shape, but defensive fallback is
		// cheap).
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"code":500,"message":"Internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// handleWebhookRequest processes a webhook request (test or production)
func (h *Handler) handleWebhookRequest(w http.ResponseWriter, r *http.Request, isTest bool) {
	// Extract path from URL
	vars := mux.Vars(r)
	path := "/" + vars["path"]

	log.Printf("📥 Webhook request: %s %s (test=%v)", r.Method, path, isTest)

	// Find webhook
	webhook, err := h.manager.GetWebhookByPath(path, r.Method, isTest)
	if err != nil {
		log.Printf("⚠️  Webhook not found: %s %s (test=%v)", r.Method, path, isTest)
		writeJSONError(w, http.StatusNotFound, webhookErrorBody{
			Code: http.StatusNotFound,
			Message: fmt.Sprintf("The requested webhook \"%s %s\" is not registered.",
				r.Method, strings.TrimPrefix(path, "/")),
			Hint: "The workflow must be active for a production URL to run successfully. " +
				"You can activate the workflow using the toggle in the top-right of the editor. " +
				"Note that unlike test URL calls, production URL calls aren't shown on the canvas " +
				"(only in the executions list)",
		})
		return
	}

	// Authenticate request
	if err := h.authenticateRequest(r, webhook); err != nil {
		log.Printf("⚠️  Authentication failed for webhook %s: %v", webhook.ID, err)
		writeJSONError(w, http.StatusUnauthorized, webhookErrorBody{
			Code:    http.StatusUnauthorized,
			Message: "Authentication failed",
			Hint:    "Verify the credentials configured on the Webhook node match what this " +
				"request is sending (Basic auth header, X-API-Key header, or custom header auth).",
		})
		return
	}

	// Parse request
	webhookRequest, err := h.parseRequest(r)
	if err != nil {
		log.Printf("⚠️  Failed to parse webhook request: %v", err)
		writeJSONError(w, http.StatusBadRequest, webhookErrorBody{
			Code:    http.StatusBadRequest,
			Message: "Invalid request",
			Hint:    "Check that the request body matches the expected Content-Type and is " +
				"well-formed (valid JSON for application/json, valid URL-encoded form for " +
				"application/x-www-form-urlencoded).",
		})
		return
	}

	// Execute the workflow. n8n's default (and our implicit default for any
	// legacy webhook that does not explicitly opt in to a blocking mode)
	// is to acknowledge the caller immediately with
	// `{"message":"Workflow was started"}` and run the workflow in the
	// background — callers like Zapier, CI hooks, etc. expect this
	// fire-and-forget contract. Only `responseMode: lastNode` and
	// `responseMode: responseNode` block on the engine result so the
	// caller can read the workflow output inline.
	if IsAsyncResponseMode(webhook.ResponseMode) {
		// Fire-and-forget: kick off the goroutine, ack the caller, done.
		h.manager.ExecuteWebhookAsync(webhook, webhookRequest)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(DefaultAsyncAckBody))

		log.Printf("✅ Webhook accepted (async): %s %s", r.Method, path)
		return
	}

	// Blocking response modes (lastNode / responseNode): preserve the
	// pre-existing behaviour so callers that read the workflow output
	// inline do not regress.
	response, err := h.manager.ExecuteWebhook(webhook, webhookRequest)
	if err != nil {
		log.Printf("⚠️  Webhook execution failed: %v", err)
		http.Error(w, fmt.Sprintf("Webhook execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	h.sendResponse(w, response)

	log.Printf("✅ Webhook executed successfully: %s %s", r.Method, path)
}

// ListWebhooks lists all webhooks
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	workflowID := r.URL.Query().Get("workflowId")

	webhooks, err := h.manager.ListWebhooks(workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  webhooks,
		"count": len(webhooks),
	})
}

// CreateWebhook creates a new webhook
func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var webhook Webhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.manager.RegisterWebhook(&webhook); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(webhook)
}

// GetWebhook retrieves a webhook by ID
func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	webhookID := vars["id"]

	webhook, err := h.manager.GetWebhook(webhookID)
	if err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webhook)
}

// DeleteWebhook deletes a webhook
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	webhookID := vars["id"]

	if err := h.manager.UnregisterWebhook(webhookID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper methods

func (h *Handler) parseRequest(r *http.Request) (*WebhookRequest, error) {
	// Copy the headers so we can inject the synthetic `Host` entry
	// that n8n surfaces on its webhook trigger output. Go's
	// http.Request separates `Host` out from `r.Header` (it lives on
	// `r.Host`), so without this copy the host header would be missing
	// from the trigger output — diverging from n8n's wire shape.
	headers := make(http.Header, len(r.Header)+1)
	for k, v := range r.Header {
		headers[k] = append([]string(nil), v...)
	}
	if r.Host != "" {
		headers.Set("Host", r.Host)
	}

	request := &WebhookRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Query:   r.URL.Query(),
	}

	// Parse body based on content type
	if r.Method != "GET" && r.Method != "DELETE" {
		contentType := r.Header.Get("Content-Type")

		if strings.Contains(contentType, "application/json") {
			var body interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to parse JSON body: %w", err)
			}
			request.Body = body
		} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			if err := r.ParseForm(); err != nil {
				return nil, fmt.Errorf("failed to parse form data: %w", err)
			}
			formData := make(map[string]interface{})
			for key, values := range r.PostForm {
				if len(values) == 1 {
					formData[key] = values[0]
				} else {
					formData[key] = values
				}
			}
			request.Body = formData
		} else {
			// Read raw body
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %w", err)
			}
			request.Body = string(bodyBytes)
		}
	}

	return request, nil
}

func (h *Handler) authenticateRequest(r *http.Request, webhook *Webhook) error {
	switch webhook.AuthType {
	case "none", "":
		return nil

	case "basic":
		// Basic authentication
		username, password, ok := r.BasicAuth()
		if !ok {
			return fmt.Errorf("basic auth required")
		}

		expectedUsername := getAuthData(webhook.AuthData, "username", "")
		expectedPassword := getAuthData(webhook.AuthData, "password", "")

		// SECURITY: Use constant-time comparison to prevent timing attacks
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUsername)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) == 1

		if !usernameMatch || !passwordMatch {
			return fmt.Errorf("invalid credentials")
		}

	case "apiKey":
		// API key authentication
		// SECURITY: Only accept API key from header, NOT from query parameters
		// Query parameters are logged in server logs, browser history, proxy caches, and referrer headers
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// SECURITY: Log warning but don't accept query parameter API keys
			if r.URL.Query().Get("apiKey") != "" {
				log.Printf("SECURITY WARNING: API key provided in query parameter (rejected). Use X-API-Key header instead.")
			}
			return fmt.Errorf("API key required in X-API-Key header")
		}

		expectedKey := getAuthData(webhook.AuthData, "apiKey", "")

		// SECURITY: Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(expectedKey)) != 1 {
			return fmt.Errorf("invalid API key")
		}

	case "header":
		// Custom header authentication
		headerName := getAuthData(webhook.AuthData, "headerName", "Authorization")
		headerValue := getAuthData(webhook.AuthData, "headerValue", "")

		actualValue := r.Header.Get(headerName)

		// SECURITY: Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(actualValue), []byte(headerValue)) != 1 {
			return fmt.Errorf("invalid header authentication")
		}

	default:
		return fmt.Errorf("unsupported auth type: %s", webhook.AuthType)
	}

	return nil
}

func (h *Handler) sendResponse(w http.ResponseWriter, response *WebhookResponse) {
	// Set headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	// Set status code
	w.WriteHeader(response.StatusCode)

	// Write body. json.Marshal is used instead of json.NewEncoder
	// (and its mandatory trailing newline) so the wire shape matches
	// n8n byte-for-byte for responseData: firstEntryJson and allEntries
	// payloads.
	if response.Body != nil {
		switch body := response.Body.(type) {
		case string:
			_, _ = w.Write([]byte(body))
		case []byte:
			_, _ = w.Write(body)
		default:
			data, mErr := json.Marshal(body)
			if mErr == nil {
				_, _ = w.Write(data)
			}
		}
	}
}

func getAuthData(authData map[string]interface{}, key, defaultValue string) string {
	if authData == nil {
		return defaultValue
	}
	if val, ok := authData[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}
