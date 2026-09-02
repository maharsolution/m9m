package trigger

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// WebhookNode implements the Webhook trigger node
type WebhookNode struct {
	*base.BaseNode
	server *http.Server
}

// NewWebhookNode creates a new Webhook node
func NewWebhookNode() *WebhookNode {
	description := base.NodeDescription{
		Name:        "Webhook",
		Description: "Receives HTTP webhook requests",
		Category:    "Trigger",
	}

	return &WebhookNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// normalizeWebhookHeaders converts a `map[string][]string` (the
// canonical Go http.Header shape) into the `map[string]string` shape
// n8n emits on its Webhook trigger output: lowercase keys, scalar
// string values, taking the first value when a header was repeated.
//
// n8n's webhook output (`{"headers":{"content-type":"application/json", …}}`)
// uses lowercase keys and string values. m9m previously passed the raw
// `http.Header` map through, which gave us PascalCase keys
// (`Content-Type`) and `[]string` values
// (`{"Content-Type":["application/json"]}`). Downstream nodes that
// compared `$json.headers["content-type"]` then silently failed because
// the key casing and shape diverged. This helper closes that gap.
func normalizeWebhookHeaders(raw map[string][]string) map[string]string {
	if raw == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for name, values := range raw {
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(name)] = values[0]
	}
	return out
}

// resolveWebhookURL returns the public URL clients should use to hit
// this webhook. n8n writes this verbatim into the trigger output so
// downstream expressions like `={{ $json.webhookUrl }}` can echo it
// back to the caller. The lookup order is:
//
//  1. The explicit `M9M_WEBHOOK_URL` env var (preferred — it lets
//     operators point the value at a proxy / tunnel host).
//  2. The legacy `WEBHOOK_URL` env var (n8n-compatible).
//  3. The `path` joined to the configured host (`M9M_HOST` + `M9M_PORT`,
//     defaulting to `http://localhost:8080`).
//
// When the env var is set but doesn't already end with the webhook's
// path, the path is appended — matching n8n's behaviour of returning
// the full hit URL.
func resolveWebhookURL(path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u := strings.TrimRight(os.Getenv("M9M_WEBHOOK_URL"), "/"); u != "" {
		return u + path
	}
	if u := strings.TrimRight(os.Getenv("WEBHOOK_URL"), "/"); u != "" {
		return u + path
	}
	host := strings.TrimSpace(os.Getenv("M9M_HOST"))
	if host == "" {
		host = "localhost"
	}
	port := strings.TrimSpace(os.Getenv("M9M_PORT"))
	if port == "" {
		port = "8080"
	}
	return "http://" + host + ":" + port + path
}

// Execute processes the Webhook node
func (w *WebhookNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Webhook nodes are typically triggered externally
	// This method handles the webhook data processing

	// Extract webhook configuration
	path, _ := nodeParams["path"].(string)
	method, _ := nodeParams["httpMethod"].(string)
	if method == "" {
		method = "POST"
	}

	// For execution context, we assume webhook data is already provided in inputData
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Process authentication if configured (uses the original item so
	// the auth code can keep reading the canonical http.Header shape).
	authType, _ := nodeParams["authentication"].(string)
	if authType != "" {
		authenticated, err := w.validateAuthentication(inputData[0].JSON, nodeParams)
		if err != nil {
			return nil, fmt.Errorf("webhook authentication failed: %w", err)
		}
		if !authenticated {
			return nil, fmt.Errorf("webhook authentication failed: invalid credentials")
		}
	}

	var outputData []model.DataItem

	for _, item := range inputData {
		// Normalize the header shape. m9m's prepareInputData drops the
		// raw http.Header map onto `item.JSON["headers"]`; we coerce it
		// into n8n's wire shape (lowercase keys, scalar string values)
		// here so the downstream Set / Function / IF nodes see the same
		// `$json.headers.content-type` value they would on n8n.
		rawHeaders, _ := item.JSON["headers"].(map[string][]string)
		normalizedHeaders := normalizeWebhookHeaders(rawHeaders)

		// Build the trigger output. The exact field set mirrors n8n's
		// webhook trigger schema:
		//
		//   headers, params, query, body, webhookUrl, executionMode
		//
		// Note: n8n's `params` is a synonym for `query` (both hold the
		// URL query parameters) and `executionMode` is the literal
		// string `"production"` for production webhooks. We pass through
		// whatever the manager populated upstream; this node is only
		// responsible for normalising headers and adding webhookUrl.
		rawParams, _ := item.JSON["params"].(map[string][]string)
		normalizedParams := normalizeHeaderLikeMap(rawParams)

		rawQuery, _ := item.JSON["query"].(map[string][]string)
		normalizedQuery := normalizeHeaderLikeMap(rawQuery)

		webhookData := map[string]interface{}{
			"headers": normalizedHeaders,
			"params":  normalizedParams,
			"query":   normalizedQuery,
			"body":    item.JSON["body"],
			"method":  method,
			"path":    path,
		}

		// webhookUrl is added last so callers can echo it back via
		// `={{ $json.webhookUrl }}` expressions. It is the full public
		// URL n8n would expose for the same workflow.
		webhookData["webhookUrl"] = resolveWebhookURL(path)

		outputItem := model.DataItem{
			JSON: webhookData,
		}

		outputData = append(outputData, outputItem)
	}

	return outputData, nil
}

// normalizeHeaderLikeMap coerces a `map[string][]string` (the canonical
// URL query / params shape in Go) into the n8n wire shape:
//
//   - string keys (preserved as-is — query keys are case-sensitive in n8n)
//   - scalar string values, first-element-wins when a key was repeated
//
// n8n returns query/params as `{"page":["1","2"]}` style maps for repeated
// keys and as a single-element array even when there is only one value;
// m9m keeps the same convention so downstream code can keep working.
func normalizeHeaderLikeMap(raw map[string][]string) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(raw))
	for name, values := range raw {
		if len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			out[name] = values[0]
			continue
		}
		out[name] = values
	}
	return out
}

// validateAuthentication validates webhook authentication
func (w *WebhookNode) validateAuthentication(requestData map[string]interface{}, nodeParams map[string]interface{}) (bool, error) {
	authType, _ := nodeParams["authentication"].(string)

	switch authType {
	case "basicAuth":
		// Basic Auth validation
		return w.validateBasicAuth(requestData, nodeParams)
	case "headerAuth":
		// Header-based authentication
		return w.validateHeaderAuth(requestData, nodeParams)
	case "none":
		return true, nil
	default:
		return true, nil // No authentication required
	}
}

// authHeadersLookup extracts a header value from the request map in a
// case-insensitive way, accepting either the canonical `http.Header`
// shape (`map[string][]string`, used by the manager when it assembles
// the engine input) or the already-normalized n8n shape
// (`map[string]string` / `map[string]interface{}` of strings). This
// matters because m9m normalizes the trigger output *after* auth, but
// `validateAuthentication` runs against the raw input the manager
// prepared — so it must read either shape without choking.
func authHeadersLookup(requestData map[string]interface{}, name string) (string, bool) {
	raw, ok := requestData["headers"]
	if !ok {
		return "", false
	}
	lcName := strings.ToLower(name)
	switch h := raw.(type) {
	case map[string][]string:
		for k, v := range h {
			if strings.ToLower(k) == lcName && len(v) > 0 {
				return v[0], true
			}
		}
	case map[string]string:
		for k, v := range h {
			if strings.ToLower(k) == lcName {
				return v, true
			}
		}
	case map[string]interface{}:
		for k, v := range h {
			if strings.ToLower(k) != lcName {
				continue
			}
			switch s := v.(type) {
			case string:
				return s, true
			case []string:
				if len(s) > 0 {
					return s[0], true
				}
			case []interface{}:
				if len(s) > 0 {
					if str, ok := s[0].(string); ok {
						return str, true
					}
				}
			}
		}
	}
	return "", false
}

// validateBasicAuth validates basic authentication
func (w *WebhookNode) validateBasicAuth(requestData map[string]interface{}, nodeParams map[string]interface{}) (bool, error) {
	authHeader, ok := authHeadersLookup(requestData, "authorization")
	if !ok {
		return false, fmt.Errorf("no authorization header found")
	}

	if !strings.HasPrefix(strings.ToLower(authHeader), "basic ") {
		return false, fmt.Errorf("invalid authorization header format")
	}

	// In a real implementation, you would decode and validate the credentials
	// For now, we'll assume valid if the header is present
	return true, nil
}

// validateHeaderAuth validates header-based authentication
func (w *WebhookNode) validateHeaderAuth(requestData map[string]interface{}, nodeParams map[string]interface{}) (bool, error) {
	headerName, _ := nodeParams["headerName"].(string)
	expectedValue, _ := nodeParams["headerValue"].(string)

	if headerName == "" {
		return false, fmt.Errorf("header name not configured")
	}

	headerValue, ok := authHeadersLookup(requestData, headerName)
	if !ok {
		return false, fmt.Errorf("required header %s not found", headerName)
	}

	return headerValue == expectedValue, nil
}

// ValidateParameters validates Webhook node parameters
func (w *WebhookNode) ValidateParameters(params map[string]interface{}) error {
	// Path is required
	if _, ok := params["path"]; !ok {
		return fmt.Errorf("webhook path is required")
	}

	// Validate HTTP method if specified
	if method, ok := params["httpMethod"].(string); ok {
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "DELETE": true,
			"PATCH": true, "HEAD": true, "OPTIONS": true,
		}
		if !validMethods[strings.ToUpper(method)] {
			return fmt.Errorf("invalid HTTP method: %s", method)
		}
	}

	return nil
}

// GetWebhookInfo returns webhook configuration for external webhook server
func (w *WebhookNode) GetWebhookInfo(params map[string]interface{}) map[string]interface{} {
	path, _ := params["path"].(string)
	method, _ := params["httpMethod"].(string)
	if method == "" {
		method = "POST"
	}

	return map[string]interface{}{
		"path":   path,
		"method": strings.ToUpper(method),
		"type":   "webhook",
	}
}