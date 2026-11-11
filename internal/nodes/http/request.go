/*
Package http provides HTTP-related node implementations for m9m.
*/
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
	"github.com/neul-labs/m9m/internal/otel"
	"go.opentelemetry.io/otel/propagation"
)

// SSRF protection: blocked IP ranges and hosts
var (
	// blockedIPRanges contains private and special-use IP ranges that should not be accessed
	blockedIPRanges = []struct {
		network *net.IPNet
		name    string
	}{
		{mustParseCIDR("10.0.0.0/8"), "Private (10.x.x.x)"},
		{mustParseCIDR("172.16.0.0/12"), "Private (172.16-31.x.x)"},
		{mustParseCIDR("192.168.0.0/16"), "Private (192.168.x.x)"},
		{mustParseCIDR("127.0.0.0/8"), "Loopback"},
		{mustParseCIDR("169.254.0.0/16"), "Link-local / Cloud Metadata"},
		{mustParseCIDR("0.0.0.0/8"), "Current network"},
		{mustParseCIDR("100.64.0.0/10"), "Shared address space"},
		{mustParseCIDR("192.0.0.0/24"), "IETF Protocol"},
		{mustParseCIDR("192.0.2.0/24"), "TEST-NET-1"},
		{mustParseCIDR("198.51.100.0/24"), "TEST-NET-2"},
		{mustParseCIDR("203.0.113.0/24"), "TEST-NET-3"},
		{mustParseCIDR("224.0.0.0/4"), "Multicast"},
		{mustParseCIDR("240.0.0.0/4"), "Reserved"},
		{mustParseCIDR("255.255.255.255/32"), "Broadcast"},
		// IPv6 blocked ranges
		{mustParseCIDR("::1/128"), "IPv6 Loopback"},
		{mustParseCIDR("fc00::/7"), "IPv6 Unique local"},
		{mustParseCIDR("fe80::/10"), "IPv6 Link-local"},
		{mustParseCIDR("ff00::/8"), "IPv6 Multicast"},
	}

	// blockedHosts contains specific hostnames that should not be accessed
	blockedHosts = map[string]bool{
		"localhost":                            true,
		"metadata.google.internal":             true, // GCP metadata
		"metadata.goog":                        true, // GCP metadata
		"169.254.169.254":                      true, // AWS/Azure/GCP metadata
		"169.254.170.2":                        true, // AWS ECS metadata
		"fd00:ec2::254":                        true, // AWS EC2 IPv6 metadata
		"[fd00:ec2::254]":                      true,
		"instance-data":                        true, // OpenStack metadata
		"metadata":                             true,
		"kubernetes.default":                   true, // Kubernetes API
		"kubernetes.default.svc":               true,
		"kubernetes.default.svc.cluster.local": true,
	}
)

func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR: %s", s))
	}
	return network
}

// isBlockedIP checks if an IP address is in a blocked range
func isBlockedIP(ip net.IP) (bool, string) {
	for _, blocked := range blockedIPRanges {
		if blocked.network.Contains(ip) {
			return true, blocked.name
		}
	}
	return false, ""
}

// validateURLForSSRF checks if a URL is safe to request (not internal/metadata)
// SECURITY: Prevents Server-Side Request Forgery attacks
func validateURLForSSRF(urlStr string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("only http and https schemes are allowed, got: %s", parsedURL.Scheme)
	}

	// Extract hostname (without port)
	hostname := parsedURL.Hostname()

	// Check if hostname is in blocked list
	if blockedHosts[strings.ToLower(hostname)] {
		return fmt.Errorf("access to host '%s' is blocked for security reasons", hostname)
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Allow the request to proceed - DNS resolution will fail at request time
		// This handles cases where the hostname is valid but DNS is temporarily unavailable
		log.Printf("SECURITY WARNING: Could not resolve hostname '%s' for SSRF check: %v", hostname, err)
		return nil
	}

	// Check each resolved IP against blocked ranges
	for _, ip := range ips {
		if blocked, reason := isBlockedIP(ip); blocked {
			return fmt.Errorf("access to IP %s (%s) is blocked for security reasons: %s", ip.String(), hostname, reason)
		}
	}

	return nil
}

// HTTPRequestNode implements the HTTP Request node functionality
type HTTPRequestNode struct {
	*base.BaseNode
	client *http.Client
}

// NewHTTPRequestNode creates a new HTTP Request node
func NewHTTPRequestNode() *HTTPRequestNode {
	description := base.NodeDescription{
		Name:        "HTTP Request",
		Description: "Makes HTTP requests to REST APIs and other web services",
		Category:    "HTTP",
		Group:       []string{"transform"},
		Properties:  httpRequestProperties(),
		Inputs:      []string{"main"},
		Outputs:     []string{"main"},
		Credentials: []base.NodeCredentialSpec{
			{Type: "httpBasicAuth", DisplayName: "Basic Auth", Required: false},
			{Type: "httpHeaderAuth", DisplayName: "Header Auth", Required: false},
			{Type: "apiKey", DisplayName: "API Key", Required: false},
		},
	}

	return &HTTPRequestNode{
		BaseNode: base.NewBaseNode(description),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// httpRequestProperties returns the property descriptors for the
// n8n-style HTTP Request Parameters tab. Mirrors n8n's
// `INodeTypeDescription.properties` so the m9m NDV-style UI can
// render the same form n8n's editor does.
//
// Every name below is a key the runtime actually reads:
// `method`, `url`, `headers`, `headerParameters`, `specifyBody`,
// `jsonBody`, `body`, `rawContentType`, `contentType`,
// `allowInternalRequests` and `includeResponseHeadersAndStatus`.
// Authentication is supplied through the node's credential (see
// NodeDescription.Credentials), not through a node parameter.
func httpRequestProperties() []base.NodeProperty {
	methods := []base.Option{
		{Name: "DELETE", Value: "DELETE"},
		{Name: "GET", Value: "GET"},
		{Name: "HEAD", Value: "HEAD"},
		{Name: "OPTIONS", Value: "OPTIONS"},
		{Name: "PATCH", Value: "PATCH"},
		{Name: "POST", Value: "POST"},
		{Name: "PUT", Value: "PUT"},
	}
	bodyTypes := []base.Option{
		{Name: "None", Value: ""},
		{Name: "JSON", Value: "json"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Method", "method", "GET", "HTTP method to use.", methods, true),
		nodeprops.Str("URL", "url", "", "Endpoint to call. Expressions are evaluated.", "https://example.com/path", true),
		nodeprops.JSON("Headers", "headers", "{}", "Header name/value map added to the request."),
		nodeprops.Collection("Header Parameters", "headerParameters", "n8n-style list of {name, value} header rows. Accepts a plain array or n8n's `{parameters: [...]}` wrapper.", nil),
		nodeprops.Opt("Body Content Type", "specifyBody", "", "How the request body is supplied. `json` sends the JSON body field.", bodyTypes, false),
		nodeprops.JSON("Body (JSON)", "jsonBody", "{}", "JSON body sent when Body Content Type is JSON.").
			WithDisplayOptions(nodeprops.Show("specifyBody", "json")),
		nodeprops.JSON("Body", "body", "{}", "Raw request body. Sent for POST, PUT and PATCH requests."),
		nodeprops.Str("Raw Content-Type", "rawContentType", "", "Content-Type header used for the raw body.", "application/json", false),
		nodeprops.Str("Content-Type", "contentType", "", "Legacy alias for Raw Content-Type.", "application/json", false),
		nodeprops.Bool("Allow Internal Requests", "allowInternalRequests", false, "Allow requests to private / loopback addresses. Disabled by default because of SSRF protection."),
		nodeprops.Bool("Include Response Headers and Status", "includeResponseHeadersAndStatus", false, "Add the response status code and headers to the output item."),
	}
	return append(props, nodeprops.Settings()...)
}

// Description returns the node description
func (h *HTTPRequestNode) Description() base.NodeDescription {
	return h.BaseNode.Description()
}

// ValidateParameters validates HTTP Request node parameters
func (h *HTTPRequestNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return h.CreateError("parameters cannot be nil", nil)
	}

	// Check if url exists
	url := h.GetStringParameter(params, "url", "")
	if url == "" {
		return h.CreateError("url parameter is required", nil)
	}

	// Check if method exists
	method := h.GetStringParameter(params, "method", "GET")

	// Allow expressions in method parameter
	if expressions.IsExpression(method) {
		// Method is an expression, so we can't validate it at this stage
		// It will be validated during execution after expression evaluation
		return nil
	}

	// Validate method if it's not an expression
	validMethods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"PATCH":   true,
		"DELETE":  true,
		"HEAD":    true,
		"OPTIONS": true,
	}

	if !validMethods[strings.ToUpper(method)] {
		return h.CreateError(fmt.Sprintf("invalid HTTP method: %s", method), nil)
	}

	return nil
}

// Execute processes the HTTP Request node operation
func (h *HTTPRequestNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return h.ExecuteWithContext(context.Background(), inputData, nodeParams)
}

// ExecuteWithContext is the context-aware variant. The parent ctx
// carries the active OTel SpanContext; we inject its traceparent onto
// the outbound HTTP request when the OTel pipeline is enabled. When
// tracing is disabled the global propagator is a no-op so this call is
// free.
func (h *HTTPRequestNode) ExecuteWithContext(ctx context.Context, inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Create execution context for expression evaluation
	execContext := &expressions.ExecutionContext{
		InputData: inputData,
		ItemIndex: 0,
		Variables: make(map[string]interface{}),
	}

	// Process each input data item
	result := make([]model.DataItem, len(inputData))

	for i, item := range inputData {
		// Update execution context for current item
		execContext.ItemIndex = i
		execContext.Variables["json"] = item.JSON

		// Evaluate node parameters with expression context
		evaluatedParams, err := h.evaluateParameters(nodeParams, execContext)
		if err != nil {
			return nil, h.CreateError(fmt.Sprintf("failed to evaluate parameters: %v", err), nil)
		}

		// Get required parameters from evaluated parameters
		requestURL := h.GetStringParameter(evaluatedParams, "url", "")
		if requestURL == "" {
			return nil, h.CreateError("url parameter cannot be empty", nil)
		}

		// SECURITY: Validate URL against SSRF attacks
		// Check if allowInternalRequests is explicitly enabled (disabled by default)
		allowInternal := h.GetBoolParameter(evaluatedParams, "allowInternalRequests", false)
		if !allowInternal {
			if err := validateURLForSSRF(requestURL); err != nil {
				return nil, h.CreateError(fmt.Sprintf("SSRF protection: %v", err), nil)
			}
		} else {
			log.Printf("SECURITY WARNING: SSRF protection disabled for request to %s", requestURL)
		}

		method := h.GetStringParameter(evaluatedParams, "method", "GET")

		// Validate method after evaluation
		validMethods := map[string]bool{
			"GET":     true,
			"POST":    true,
			"PUT":     true,
			"PATCH":   true,
			"DELETE":  true,
			"HEAD":    true,
			"OPTIONS": true,
		}

		if !validMethods[strings.ToUpper(method)] {
			return nil, h.CreateError(fmt.Sprintf("invalid HTTP method: %s", method), nil)
		}

		// Create HTTP request
		req, err := http.NewRequest(strings.ToUpper(method), requestURL, nil)
		if err != nil {
			return nil, h.CreateError(fmt.Sprintf("failed to create request: %v", err), nil)
		}

		// Add headers if provided. n8n's HTTP Request node exports
		// headers in two shapes:
		//
		//   1. Flat map:    `headers: { "X-Foo": "bar" }`
		//   2. Spec list:   `headerParameters: { parameters: [
		//                       { name: "X-Foo", value: "bar" }, ... ] }`
		//
		// Older m9m only recognised shape (1), which broke every
		// workflow authored in the n8n UI for typeVersion >= 4.
		// Accept both so the `headerParameters` shape the webhook
		// parity tests rely on flows through to the outgoing
		// request.
		if headers, ok := evaluatedParams["headers"].(map[string]interface{}); ok {
			for key, value := range headers {
				if strValue, ok := value.(string); ok {
					req.Header.Set(key, strValue)
				}
			}
		}
		if hp, ok := evaluatedParams["headerParameters"]; ok {
			applyHeaderParameters(req, hp)
		}

		// Apply the per-node credential envelope (if any). The
		// engine writes keys shaped `<credType>_<field>` into
		// `evaluatedParams` before Execute is called; we map
		// those to the matching HTTP auth scheme. Runs BEFORE
		// the explicit headers so a node-level `Authorization`
		// header (rare) can still override the credential one
		// — but the common case is "no explicit header, just the
		// credential", so this branch fills the gap.
		if err := applyCredentialAuth(req, evaluatedParams); err != nil {
			return nil, h.CreateError(fmt.Sprintf("credential auth: %v", err), nil)
		}

		// Add body for POST, PUT, PATCH requests.
		// n8n HTTP Request v4 uses `jsonBody` + `specifyBody: "json"`
		// (Finnet ServiceRoutePlans / Call SvcRule). Older m9m only
		// read `body`, so those nodes posted an empty payload and the
		// downstream IF (`$json.data.attributes.route`) fell to false.
		if method == "POST" || method == "PUT" || method == "PATCH" {
			bodyBytes, contentType, bodyErr := resolveRequestBody(evaluatedParams)
			if bodyErr != nil {
				return nil, h.CreateError(fmt.Sprintf("failed to build request body: %v", bodyErr), nil)
			}
			if len(bodyBytes) > 0 {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				if contentType != "" && req.Header.Get("Content-Type") == "" {
					req.Header.Set("Content-Type", contentType)
				}
			}
		}

		// Set default user agent if not provided
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "m9m/1.0")
		}

		// Inject the active traceparent into the outbound request
		// header so downstream services can continue the trace. The
		// global propagator is initialised to a no-op by the OTel SDK
		// when tracing is disabled, so this call is free.
		otel.GlobalInject(ctx, propagation.HeaderCarrier(req.Header))

		// Execute request. Derive the timeout from the parent ctx so
		// node cancellation propagates; fall back to a fresh ctx when
		// the parent is nil (legacy callers that still use Execute).
		parent := ctx
		if parent == nil {
			parent = context.Background()
		}
		callCtx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()

		req = req.WithContext(callCtx)

		resp, err := h.client.Do(req)
		if err != nil {
			return nil, h.CreateError(fmt.Sprintf("request failed: %v", err), nil)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, h.CreateError(fmt.Sprintf("failed to read response body: %v", err), nil)
		}

		// Parse response headers
		headers := make(map[string]interface{})
		for key, values := range resp.Header {
			if len(values) > 0 {
				headers[key] = values[0] // Take first value
			}
		}

		// Build the n8n-shaped response item.
		//
		// n8n HTTP Request (default): `$json` IS the parsed response body
		// (object fields at the top level). Workflows like Finnet
		// post_balance read `$json.data.attributes.route` directly.
		//
		// Only when `response.includeResponseHeadersAndStatus` (or the
		// legacy fullResponse flag) is true do we also surface
		// `headers` / `statusCode` as top-level keys alongside the body.
		includeMeta := h.responseIncludesMeta(evaluatedParams)

		responseData := model.DataItem{
			JSON: h.shapeHTTPResponse(resp.StatusCode, headers, body, resp.Header.Get("Content-Type"), includeMeta),
		}

		// Copy binary data if present
		if item.Binary != nil {
			responseData.Binary = make(map[string]model.BinaryData)
			for k, v := range item.Binary {
				responseData.Binary[k] = v
			}
		}

		// Copy paired item data if present
		if item.PairedItem != nil {
			responseData.PairedItem = item.PairedItem
		}

		result[i] = responseData
	}

	return result, nil
}

// evaluateParameters evaluates node parameters with expression context
func (h *HTTPRequestNode) evaluateParameters(params map[string]interface{}, context *expressions.ExecutionContext) (map[string]interface{}, error) {
	evaluator := expressions.NewExpressionEvaluator()

	evaluatedParams := make(map[string]interface{})

	for key, value := range params {
		switch v := value.(type) {
		case string:
			// Check if this is an expression
			if expressions.IsExpression(v) {
				// Evaluate the expression
				evaluatedValue, err := evaluator.Evaluate(v, context)
				if err != nil {
					return nil, fmt.Errorf("failed to evaluate expression for parameter %s: %v", key, err)
				}
				evaluatedParams[key] = evaluatedValue
			} else {
				// Use the literal value
				evaluatedParams[key] = v
			}
		case map[string]interface{}:
			// Recursively evaluate nested maps
			nested, err := h.evaluateParameters(v, context)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate nested parameters for %s: %v", key, err)
			}
			evaluatedParams[key] = nested
		default:
			// Use the literal value
			evaluatedParams[key] = v
		}
	}

	return evaluatedParams, nil
}

// applyHeaderParameters reads the n8n-style header spec (a list of
// `{name, value}` records, possibly nested under
// `headerParameters.parameters`) and applies every entry to the
// outgoing request. Empty/whitespace names are skipped so a
// half-filled UI row does not crash the request, and non-string
// values are coerced via fmt.Sprintf so numeric IDs from upstream
// expressions still flow through.
func applyHeaderParameters(req *http.Request, raw interface{}) {
	entries := flattenHeaderSpec(raw)
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		value, ok := entry["value"].(string)
		if !ok {
			value = fmt.Sprintf("%v", entry["value"])
		}
		req.Header.Set(name, value)
	}
}

func flattenHeaderSpec(raw interface{}) []map[string]interface{} {
	switch v := raw.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]interface{}:
		// n8n nests the list under `parameters`:
		// `headerParameters: { parameters: [...] }`.
		if list, ok := v["parameters"].([]interface{}); ok {
			return flattenHeaderSpec(list)
		}
		// Some workflows omit the wrapper and put a list-shaped value
		// under the key directly.
		if list, ok := v["values"].([]interface{}); ok {
			return flattenHeaderSpec(list)
		}
	}
	return nil
}

// resolveRequestBody picks the outbound body from n8n-shaped params.
// Priority:
//  1. `jsonBody` when `specifyBody` is "json" (or jsonBody is set and
//     body is empty) — Finnet ServiceRoutePlans / Call SvcRule
//  2. `body` string / map (Call External Svc raw XML, legacy shapes)
//
// Content-Type defaults to application/json for jsonBody; raw bodies
// honour `rawContentType` / `contentType` when provided.
func resolveRequestBody(params map[string]interface{}) ([]byte, string, error) {
	specify, _ := params["specifyBody"].(string)
	jsonBody := params["jsonBody"]
	body := params["body"]

	useJSONBody := false
	switch {
	case specify == "json" && jsonBody != nil:
		useJSONBody = true
	case jsonBody != nil && (body == nil || body == ""):
		useJSONBody = true
	}

	if useJSONBody {
		switch v := jsonBody.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				return nil, "", nil
			}
			// After expression eval, `={...}` becomes a JSON text
			// string. Validate lightly; still send even if not strict
			// JSON so upstream can report its own parse error.
			return []byte(s), "application/json", nil
		case map[string]interface{}, []interface{}:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, "", err
			}
			return b, "application/json", nil
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, "", err
			}
			return b, "application/json", nil
		}
	}

	contentType := ""
	if ct, ok := params["rawContentType"].(string); ok && ct != "" {
		contentType = ct
	} else if ct, ok := params["contentType"].(string); ok && ct != "" && ct != "raw" && ct != "json" {
		contentType = ct
	}

	switch v := body.(type) {
	case string:
		if v == "" {
			return nil, contentType, nil
		}
		if contentType == "" {
			contentType = "application/json"
		}
		return []byte(v), contentType, nil
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, "", err
		}
		if contentType == "" {
			contentType = "application/json"
		}
		return b, contentType, nil
	case nil:
		return nil, contentType, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, "", err
		}
		if contentType == "" {
			contentType = "application/json"
		}
		return b, contentType, nil
	}
}

// responseIncludesMeta reports whether the HTTP Request node should
// surface response headers/status alongside the body. Mirrors n8n's
// `response.includeResponseHeadersAndStatus` / fullResponse flags.
func (h *HTTPRequestNode) responseIncludesMeta(params map[string]interface{}) bool {
	if h.GetBoolParameter(params, "response.includeResponseHeadersAndStatus", false) {
		return true
	}
	if h.GetBoolParameter(params, "includeResponseHeadersAndStatus", false) {
		return true
	}
	// Nested options.response.fullResponse (n8n v3/v4 export shape).
	if opts, ok := params["options"].(map[string]interface{}); ok {
		if resp, ok := opts["response"].(map[string]interface{}); ok {
			if full, ok := resp["fullResponse"].(bool); ok && full {
				return true
			}
			if include, ok := resp["includeResponseHeadersAndStatus"].(bool); ok && include {
				return true
			}
		}
	}
	return false
}

// shapeHTTPResponse builds the n8n-compatible `$json` payload for an
// HTTP Request node output.
//
// Default (includeMeta=false): parsed JSON object fields become the
// top-level `$json` keys — so `$json.data.attributes.route` works for
// Finnet ServiceRoutePlans → isAsync. Non-object JSON and non-JSON
// bodies are wrapped as `{ "data": <value> }` (n8n's text/array shape).
//
// includeMeta=true: also attach `headers` and `statusCode` as top-level
// keys (and keep a `body` string for raw access).
func (h *HTTPRequestNode) shapeHTTPResponse(statusCode int, headers map[string]interface{}, body []byte, contentType string, includeMeta bool) map[string]interface{} {
	var out map[string]interface{}

	isJSON := strings.Contains(strings.ToLower(contentType), "application/json") ||
		strings.Contains(strings.ToLower(contentType), "+json")
	if isJSON || json.Valid(body) {
		var parsed interface{}
		if err := json.Unmarshal(body, &parsed); err == nil {
			switch v := parsed.(type) {
			case map[string]interface{}:
				out = v
			default:
				out = map[string]interface{}{"data": v}
			}
		}
	}
	if out == nil {
		out = map[string]interface{}{"data": string(body)}
	}

	if includeMeta {
		// Copy so we don't mutate a shared parsed map unexpectedly when
		// callers hold references; also avoid colliding with a body key
		// named "headers"/"statusCode" by preferring response meta only
		// when the flag is set (n8n does the same under fullResponse).
		meta := make(map[string]interface{}, len(out)+3)
		for k, v := range out {
			meta[k] = v
		}
		meta["headers"] = headers
		meta["statusCode"] = statusCode
		meta["body"] = string(body)
		return meta
	}
	return out
}

// applyCredentialAuth looks at the evaluated node params for the
// four credential-injected key patterns the engine writes for
// per-node credentials, and applies the matching auth scheme to
// the outgoing request.
//
// Why this exists: the engine's `InjectCredentialsIntoNodeParameters`
// writes the resolved credential envelope into the params map
// under keys shaped `<credType>_<field>` — for example
// `httpBasicAuth_user` / `httpBasicAuth_password` for an
// `httpBasicAuth` credential, or `httpHeaderAuth_name` /
// `httpHeaderAuth_value` for `httpHeaderAuth`. Without this
// helper the credentials are silently dropped and the request
// goes out anonymous — which is exactly the BUG-2026-09-09-002
// parity gap (`positive/3 webhook_callrest` returned the
// upstream's anonymous-echo body, not the authed one).
//
// Supported schemes:
//   - `httpBasicAuth`: sets `Authorization: Basic ...` via
//     `req.SetBasicAuth(user, password)`. Both fields required.
//   - `httpHeaderAuth`: sets `<name>: <value>` directly. Both
//     fields required.
//   - `apiKey`: sets `Authorization: Bearer <apiKey>`. n8n's
//     `apiKey` credential type is usually wired as a header
//     pair (name + value); the most common convention is the
//     simple "Authorization: Bearer ..." pattern. We honour
//     the bare `apiKey` field here.
//   - All others (oauth2, jwt, etc.): no-op. The oauth2 flow
//     needs a refresh-token round-trip; the jwt case is for
//     inbound webhooks, not outbound requests.
//
// Returns an error only if a credential is attached but its
// fields are missing (silent-fail would be a worse outcome than
// a clear 4xx — the user can see the missing-field message in
// the execution record).
//
// The envelope keys are the only source of auth material for this
// node (there is no inline auth parameter in the n8n HTTP Request
// shape), so no legacy fallback key is needed here. The values are
// read through base.CredentialString, which treats a present but
// empty value as absent and ignores non-string values.
func stringFromMap(params map[string]interface{}, key string) string {
	return base.CredentialString(params, key)
}

func applyCredentialAuth(req *http.Request, params map[string]interface{}) error {
	user := stringFromMap(params, "httpBasicAuth_user")
	pass := stringFromMap(params, "httpBasicAuth_password")
	if user != "" || pass != "" {
		// Both halves required; the engine's inject writes
		// both or neither for a given credential, so a
		// half-present pair means the credential id was
		// dangling.
		if user == "" || pass == "" {
			return fmt.Errorf("httpBasicAuth credential is missing one of user/password")
		}
		req.SetBasicAuth(user, pass)
		return nil
	}

	name := stringFromMap(params, "httpHeaderAuth_name")
	value := stringFromMap(params, "httpHeaderAuth_value")
	if name != "" || value != "" {
		if name == "" || value == "" {
			return fmt.Errorf("httpHeaderAuth credential is missing one of name/value")
		}
		req.Header.Set(name, value)
		return nil
	}

	if apiKey := stringFromMap(params, "apiKey_apiKey"); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}

	// No credential attached — anonymous request. Don't error;
	// many HTTP Request nodes legitimately have no auth
	// (public APIs, same-origin calls, etc.).
	return nil
}
