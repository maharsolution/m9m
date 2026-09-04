package webhooks

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// jsonUnmarshal is a thin wrapper around json.Unmarshal so the
// lastNodeResponseBody helper stays small and the alias can be
// swapped out for tolerant parsers in the future.
var jsonUnmarshal = func(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// WebhookManager manages webhook registration and execution
type WebhookManager struct {
	storage         WebhookStorage
	workflowStorage storage.WorkflowStorage
	engine          engine.WorkflowEngine
	activeHooks     map[string]*Webhook // path:method:test -> webhook
	mu              sync.RWMutex
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(webhookStorage WebhookStorage, workflowStorage storage.WorkflowStorage, engine engine.WorkflowEngine) *WebhookManager {
	return &WebhookManager{
		storage:         webhookStorage,
		workflowStorage: workflowStorage,
		engine:          engine,
		activeHooks:     make(map[string]*Webhook),
	}
}

// RegisterWebhook registers a webhook for a workflow node
func (m *WebhookManager) RegisterWebhook(webhook *Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate webhook
	if err := m.validateWebhook(webhook); err != nil {
		return fmt.Errorf("invalid webhook: %w", err)
	}

	// Generate ID if not provided
	if webhook.ID == "" {
		webhook.ID = generateWebhookID()
	}

	// Normalize path (ensure leading slash)
	webhook.Path = normalizePath(webhook.Path)

	// Set defaults
	if webhook.Method == "" {
		webhook.Method = "POST"
	}
	if webhook.ResponseMode == "" {
		webhook.ResponseMode = "onReceived"
	}
	if webhook.ResponseData == "" {
		webhook.ResponseData = "firstEntryJson"
	}

	// Save to storage
	if err := m.storage.SaveWebhook(webhook); err != nil {
		return fmt.Errorf("failed to save webhook: %w", err)
	}

	// Add to active hooks if active
	if webhook.Active {
		key := makeWebhookKey(webhook.Path, webhook.Method, webhook.IsTest)
		m.activeHooks[key] = webhook
		log.Printf("✅ Webhook registered: %s %s (workflow=%s, test=%v)",
			webhook.Method, webhook.Path, webhook.WorkflowID, webhook.IsTest)
	}

	return nil
}

// UnregisterWebhook removes a webhook
func (m *WebhookManager) UnregisterWebhook(webhookID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, err := m.storage.GetWebhook(webhookID)
	if err != nil {
		return err
	}

	// Remove from active hooks
	key := makeWebhookKey(webhook.Path, webhook.Method, webhook.IsTest)
	delete(m.activeHooks, key)

	// Delete from storage
	if err := m.storage.DeleteWebhook(webhookID); err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	log.Printf("❌ Webhook unregistered: %s %s", webhook.Method, webhook.Path)
	return nil
}

// GetWebhook retrieves a webhook by ID
func (m *WebhookManager) GetWebhook(webhookID string) (*Webhook, error) {
	return m.storage.GetWebhook(webhookID)
}

// GetWebhookByPath retrieves a webhook by path and method
func (m *WebhookManager) GetWebhookByPath(path string, method string, isTest bool) (*Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path = normalizePath(path)
	key := makeWebhookKey(path, method, isTest)

	webhook, exists := m.activeHooks[key]
	if !exists {
		return nil, fmt.Errorf("webhook not found: %s %s (test=%v)", method, path, isTest)
	}

	return webhook, nil
}

// ListWebhooks lists all webhooks for a workflow
func (m *WebhookManager) ListWebhooks(workflowID string) ([]*Webhook, error) {
	return m.storage.ListWebhooks(workflowID)
}

// RegisterWorkflowWebhooks registers all webhooks for a workflow
func (m *WebhookManager) RegisterWorkflowWebhooks(workflow *model.Workflow, isTest bool) error {
	// Find all webhook nodes in the workflow
	for _, node := range workflow.Nodes {
		if isWebhookNode(node.Type) {
			webhook := m.createWebhookFromNode(workflow, &node, isTest)
			if err := m.RegisterWebhook(webhook); err != nil {
				log.Printf("⚠️  Failed to register webhook for node %s: %v", node.Name, err)
			}
		}
	}

	return nil
}

// UnregisterWorkflowWebhooks removes all webhooks for a workflow
func (m *WebhookManager) UnregisterWorkflowWebhooks(workflowID string) error {
	webhooks, err := m.storage.ListWebhooks(workflowID)
	if err != nil {
		return err
	}

	for _, webhook := range webhooks {
		if err := m.UnregisterWebhook(webhook.ID); err != nil {
			log.Printf("⚠️  Failed to unregister webhook %s: %v", webhook.ID, err)
		}
	}

	return nil
}

// ExecuteWebhook executes a webhook and returns the response
func (m *WebhookManager) ExecuteWebhook(webhook *Webhook, request *WebhookRequest) (*WebhookResponse, error) {
	startTime := time.Now()

	// Get workflow
	workflow, err := m.workflowStorage.GetWorkflow(webhook.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %s", webhook.WorkflowID)
	}

	// Prepare execution input from webhook request
	inputData := m.prepareInputData(request)

	// Execute workflow
	executionID := generateExecutionID()
	result, err := m.engine.ExecuteWorkflow(workflow, inputData)
	executionErr := engine.ResolveExecutionError(result, err)

	// Create execution record
	execution := &WebhookExecution{
		ID:          generateWebhookExecutionID(),
		WebhookID:   webhook.ID,
		ExecutionID: executionID,
		Request:     request,
		CreatedAt:   time.Now(),
		Duration:    time.Since(startTime).Milliseconds(),
	}

	if executionErr != nil {
		execution.Status = "failed"
		execution.Error = executionErr.Error()
		_ = m.storage.SaveWebhookExecution(execution)
		return nil, fmt.Errorf("workflow execution failed: %w", executionErr)
	}

	execution.Status = "success"

	// Prepare response based on webhook configuration. The full-fat
	// variant receives the workflow + trigger node so it can honour
	// `responseMode: responseNode` by reading the Respond-to-Webhook
	// node's output out of `result.NodeOutputs`. Without the
	// workflow + trigger context the manager would fall back to
	// `lastNode` semantics and respond with whatever the last
	// executed node produced — diverging from n8n's wire shape for
	// the `webhook_code` workflow (`V4432EsGIkpqIZx9`).
	response := m.prepareResponseWithContext(webhook, result, workflow, webhook.NodeID)
	execution.Response = response

	// Save execution record
	if err := m.storage.SaveWebhookExecution(execution); err != nil {
		log.Printf("⚠️  Failed to save webhook execution: %v", err)
	}

	return response, nil
}

// DefaultAsyncAckBody is the wire body returned to clients when a webhook
// is processed in fire-and-forget mode. n8n responds with this exact
// payload + 200 OK so external callers (Zapier-style triggers, CI
// hooks, etc.) can treat m9m as a drop-in.
const DefaultAsyncAckBody = `{"message":"Workflow was started"}`

// IsAsyncResponseMode reports whether the webhook should respond
// immediately with an acknowledgment (true) or block on the engine
// result (false). n8n's default — and the implicit default for any
// legacy webhook that doesn't explicitly opt into
// `responseMode: lastNode` / `responseNode` — is asynchronous.
func IsAsyncResponseMode(responseMode string) bool {
	switch responseMode {
	case "lastNode", "responseNode":
		return false
	default:
		// "" (legacy) and "onReceived" both mean "ack immediately".
		return true
	}
}

// ExecuteWebhookAsync runs ExecuteWebhook in a goroutine. It returns
// immediately so the HTTP handler can ack the caller with 200 OK and
// `{"message":"Workflow was started"}`. Errors during the background
// run are logged but never propagated to the caller (caller is already
// gone). The goroutine is wrapped in defer recover() so a panic inside
// the engine or its nodes cannot crash the server.
func (m *WebhookManager) ExecuteWebhookAsync(webhook *Webhook, request *WebhookRequest) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 panic in async webhook execution (webhook=%s, workflow=%s): %v",
					webhook.ID, webhook.WorkflowID, r)
			}
		}()

		if _, err := m.ExecuteWebhook(webhook, request); err != nil {
			log.Printf("⚠️  Async webhook execution failed (webhook=%s, workflow=%s): %v",
				webhook.ID, webhook.WorkflowID, err)
		}
	}()
}

// LoadActiveWebhooks loads all active webhooks into memory
func (m *WebhookManager) LoadActiveWebhooks() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhooks, err := m.storage.ListWebhooks("")
	if err != nil {
		return fmt.Errorf("failed to load webhooks: %w", err)
	}

	count := 0
	for _, webhook := range webhooks {
		if webhook.Active {
			key := makeWebhookKey(webhook.Path, webhook.Method, webhook.IsTest)
			m.activeHooks[key] = webhook
			count++
		}
	}

	log.Printf("📡 Loaded %d active webhooks", count)
	return nil
}

// Helper methods

func (m *WebhookManager) validateWebhook(webhook *Webhook) error {
	if webhook.WorkflowID == "" {
		return fmt.Errorf("workflow ID is required")
	}
	if webhook.Path == "" {
		return fmt.Errorf("path is required")
	}
	return nil
}

func (m *WebhookManager) createWebhookFromNode(workflow *model.Workflow, node *model.Node, isTest bool) *Webhook {
	// Extract webhook configuration from node parameters. n8n's Webhook
	// node exposes responseData as a top-level parameter; without
	// honouring it the wire shape diverges from n8n whenever the user
	// has explicitly chosen `allEntries` (which expects a JSON array
	// response) vs `firstEntryJson` (which expects a bare object).
	path := getStringParam(node.Parameters, "path", "")
	method := strings.ToUpper(getStringParam(node.Parameters, "httpMethod", "POST"))
	authType := getStringParam(node.Parameters, "authentication", "none")
	responseMode := getStringParam(node.Parameters, "responseMode", "onReceived")
	responseData := getStringParam(node.Parameters, "responseData", "firstEntryJson")

	return &Webhook{
		WorkflowID:   workflow.ID,
		NodeID:       node.Name,
		Path:         normalizePath(path),
		Method:       method,
		IsTest:       isTest,
		Active:       workflow.Active && !isTest,
		AuthType:     authType,
		ResponseMode: responseMode,
		ResponseData: responseData,
	}
}

// prepareInputData builds the engine's input data from the parsed
// HTTP request. The `headers` and `query` / `params` maps are passed
// through in their canonical Go shape (`map[string][]string`); the
// Webhook trigger node normalises them to n8n's wire shape
// (lowercased keys + scalar string values) downstream so that
// expressions like `$json.headers["content-type"]` resolve the same
// way they do on n8n.
//
// We also stamp `executionMode` ("production" / "test") and
// `webhookUrl` here — at the manager level — so the values are
// available to the engine even if a workflow has been simplified to
// skip the trigger node (e.g. a workflow that wires straight into a
// Set node still needs `$json.executionMode` populated, the same way
// n8n populates it on the trigger output).
func (m *WebhookManager) prepareInputData(request *WebhookRequest) []model.DataItem {
	data := map[string]interface{}{
		"headers":       request.Headers,
		"params":        request.Query,
		"query":         request.Query,
		"body":          request.Body,
		"method":        request.Method,
		"path":          request.Path,
		"webhookUrl":    resolveRequestWebhookURL(request),
		"executionMode": resolveExecutionMode(request),
	}

	return []model.DataItem{
		{
			JSON: data,
		},
	}
}

// resolveExecutionMode returns the n8n-compatible executionMode label
// for an inbound webhook request. Test webhooks are served from
// `/webhook-test/<path>`; production webhooks are served from
// `/webhook/<path>` — the path prefix is the only reliable signal in
// the inbound request because n8n exposes the same literal strings
// in its trigger output.
func resolveExecutionMode(request *WebhookRequest) string {
	if strings.HasPrefix(request.Path, "/webhook-test/") {
		return "test"
	}
	return "production"
}

// resolveRequestWebhookURL returns the public URL clients should use
// to hit this webhook. The lookup order matches the trigger-node
// resolver so both layers agree:
//  1. `M9M_WEBHOOK_URL` (preferred — lets operators point at a proxy).
//  2. `WEBHOOK_URL` (n8n-compatible env var).
//  3. `M9M_HOST` + `M9M_PORT`, defaulting to `http://localhost:8080`.
//
// The returned URL has the inbound path appended so callers can echo
// it back via `{{ $json.webhookUrl }}` exactly like n8n does.
func resolveRequestWebhookURL(request *WebhookRequest) string {
	path := request.Path
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

func (m *WebhookManager) prepareResponse(webhook *Webhook, result *engine.ExecutionResult) *WebhookResponse {
	return m.prepareResponseWithContext(webhook, result, nil, "")
}

// respondToWebhookNodeType is the n8n node-type identifier m9m
// registers the "Respond to Webhook" trigger under. Workflows that
// opt into `responseMode: responseNode` on the Webhook trigger must
// include exactly one node of this type on the webhook's downstream
// chain; the webhook manager walks the connection graph from the
// trigger node to find it and reads its output out of the engine's
// NodeOutputs map.
const respondToWebhookNodeType = "n8n-nodes-base.respondToWebhook"

// findRespondToWebhookNode walks the workflow's connection graph
// starting from `triggerNode` and returns the name of the first
// downstream node whose `Type` matches `respondToWebhookNodeType`.
// Returns "" when no Respond-to-Webhook node is reachable from the
// trigger (caller should fall back to the last-node output).
//
// BFS is used (rather than recursive DFS) so that the *closest*
// Respond-to-Webhook node to the trigger wins — this matches n8n's
// own execution order, which runs the trigger, then the nodes
// immediately connected to it, before anything further downstream.
// Cycles are guarded by the visited set even though n8n rejects
// workflows with cycles at import time.
func findRespondToWebhookNode(workflow *model.Workflow, triggerNode string) string {
	if workflow == nil || triggerNode == "" {
		return ""
	}

	// Build the node-name → node-type lookup so we can recognise
	// the respond-to-webhook node without iterating over the full
	// node slice at every BFS step.
	typeByName := make(map[string]string, len(workflow.Nodes))
	for _, n := range workflow.Nodes {
		typeByName[n.Name] = n.Type
	}

	visited := make(map[string]struct{})
	queue := []string{triggerNode}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}

		if typeByName[current] == respondToWebhookNodeType {
			return current
		}

		conns, ok := workflow.Connections[current]
		if !ok {
			continue
		}
		for _, branch := range conns.Main {
			for _, c := range branch {
				if c.Node == "" {
					continue
				}
				if _, seen := visited[c.Node]; seen {
					continue
				}
				queue = append(queue, c.Node)
			}
		}
	}
	return ""
}

// findRespondToWebhookNodeByType scans the workflow's nodes for any
// node whose `Type` is the respond-to-webhook node type, returning
// its name. This is the fallback path used when the workflow has no
// incoming-connection index for the trigger (e.g. the trigger node
// was renamed but its connections survived): the manager picks the
// first respond-to-webhook node it sees and lets the engine decide
// whether it actually received the data — if it didn't, NodeOutputs
// lookup simply returns nil and the caller falls back to the
// last-node body.
func findRespondToWebhookNodeByType(workflow *model.Workflow) string {
	if workflow == nil {
		return ""
	}
	for _, n := range workflow.Nodes {
		if n.Type == respondToWebhookNodeType {
			return n.Name
		}
	}
	return ""
}

// extractResponseNodeData reads the output the Respond-to-Webhook
// node produced and returns it. Returns nil when the node produced
// no data, or when the engine did not populate per-node tracking
// for this run (older engine paths). Callers MUST treat a nil
// return as "fall back to last-node output".
func extractResponseNodeData(workflow *model.Workflow, triggerNode string, result *engine.ExecutionResult) []model.DataItem {
	if result == nil || result.NodeOutputs == nil {
		return nil
	}

	nodeName := findRespondToWebhookNode(workflow, triggerNode)
	if nodeName == "" {
		nodeName = findRespondToWebhookNodeByType(workflow)
	}
	if nodeName == "" {
		return nil
	}

	data, ok := result.NodeOutputs[nodeName]
	if !ok || len(data) == 0 {
		return nil
	}
	return data
}

// prepareResponseWithContext is the full-fat version of
// prepareResponse that also accepts the workflow + trigger-node name
// so it can implement `responseMode: responseNode`. The manager
// passes the workflow it just executed; the legacy single-arg
// prepareResponse is preserved for backwards compatibility with
// callers (and the unit-test suite) that don't have a workflow
// handy.
func (m *WebhookManager) prepareResponseWithContext(webhook *Webhook, result *engine.ExecutionResult, workflow *model.Workflow, triggerNode string) *WebhookResponse {
	response := &WebhookResponse{
		StatusCode: 200,
		Headers:    webhook.ResponseHeaders,
	}

	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}
	response.Headers["Content-Type"] = "application/json"

	// `responseMode: responseNode` overrides `responseData`. n8n's
	// behaviour: the workflow author wires the trigger to a
	// "Respond to Webhook" node, and that node's output is returned
	// verbatim — regardless of `responseData`. We locate the
	// Respond-to-Webhook node by walking the connection graph from
	// the trigger, fall back to a flat node-type scan when the
	// trigger was renamed (workflows in the wild sometimes drop the
	// `connections` index for renamed nodes), and finally fall back
	// to `lastNode` semantics if there is no Respond-to-Webhook
	// node at all (or the engine did not populate NodeOutputs —
	// older engine paths, parallel workers, etc.).
	if webhook.ResponseMode == "responseNode" {
		if data := extractResponseNodeData(workflow, triggerNode, result); data != nil {
			switch webhook.ResponseData {
			case "allEntries":
				entries := make([]map[string]interface{}, len(data))
				for i, item := range data {
					entries[i] = item.JSON
				}
				response.Body = entries
			case "noData":
				response.Body = map[string]interface{}{"message": "success"}
			case "firstEntryJson", "":
				fallthrough
			default:
				// n8n honours the Respond-to-Webhook node's own
				// `respondWith` shape; the most common setting is
				// `firstIncomingItem`, which is exactly what we
				// return here. If `respondWith=json` the upstream
				// node already produced a single item with the
				// literal body, so read that item verbatim.
				if len(data) == 0 {
					response.Body = map[string]interface{}{"message": "success"}
				} else {
					response.Body = unwrapRespondToWebhookBody(data[0].JSON)
				}
			}
			return response
		}
		// No Respond-to-Webhook node found (or no per-node
		// tracking) — fall through to the responseData-driven
		// branch so callers that expected `lastNode` semantics
		// still get a useful response.
	}

	// Based on response mode. n8n's wire shape for the Webhook response
	// is *workflow-shape dependent*, not uniform across responseData
	// values:
	//
	//   - `firstEntryJson` (n8n's default when responseData is unset):
	//     emit the first item's JSON as a bare object
	//     (`{"total":15}`). Verified live against
	//     http://187.77.113.218:5678/webhook/simple_webhook (no Set
	//     node downstream) — n8n returns a bare object, not an array.
	//
	//   - `allEntries`: emit every item's JSON as a JSON array
	//     (`[{...},{...},...]`). Verified live against
	//     http://187.77.113.218:5678/webhook/bocahtuanakal
	//     (responseData=allEntries) — n8n returns an array.
	//
	//   - `noData`: emit a 200 with an empty body. n8n's `noData`
	//     mode literally responds with an empty body; we mirror that
	//     by emitting `{"message":"success"}` so clients still see a
	//     valid JSON body (this matches m9m's historical behaviour
	//     and the existing test suite).
	//
	// The Set node downstream may have merged upstream webhook fields
	// (body, headers, params, etc.) into its output — that is fine;
	// n8n returns those fields too, and downstream callers rely on
	// seeing them.
	switch webhook.ResponseData {
	case "firstEntryJson":
		if len(result.Data) == 0 {
			response.Body = map[string]interface{}{"message": "success"}
		} else {
			response.Body = lastNodeResponseBody(workflow, result)
		}
	case "allEntries":
		entries := make([]map[string]interface{}, len(result.Data))
		for i, item := range result.Data {
			entries[i] = item.JSON
		}
		response.Body = entries
	case "noData":
		response.Body = map[string]interface{}{"message": "success"}
	default:
		// Unknown / empty responseData: fall through to firstEntryJson
		// (bare object) — this matches n8n's default for the field when
		// it is unset.
		if len(result.Data) == 0 {
			response.Body = map[string]interface{}{"message": "success"}
		} else {
			response.Body = lastNodeResponseBody(workflow, result)
		}
	}

	return response
}

// Helper functions

// lastNodeResponseBody returns the wire-shape body for the `lastNode`
// response mode. n8n's HTTP Request, Webhook and other transport-style
// nodes wrap their real payload in `{body, headers, statusCode, json?}`
// — the wrapper exists for downstream pipelines, but when the node is
// the webhook's last node n8n emits the JSON body to the caller
// instead. Detect that shape here so parity tests don't see the raw
// wrapper.
func lastNodeResponseBody(workflow *model.Workflow, result *engine.ExecutionResult) map[string]interface{} {
	if result == nil || len(result.Data) == 0 {
		return map[string]interface{}{"message": "success"}
	}
	item := result.Data[0].JSON

	// Detect n8n's HTTPRequest-shaped payload by checking for the
	// well-known wrapper keys. We only unwrap when ALL three of
	// `body`/`headers`/`statusCode` are present — partial matches
	// are normally user data, not a transport wrapper.
	_, hasBody := item["body"]
	_, hasHeaders := item["headers"]
	_, hasStatus := item["statusCode"]
	if hasBody && hasHeaders && hasStatus {
		if jsonBody, ok := item["json"]; ok {
			if unwrapped, ok := jsonBody.(map[string]interface{}); ok {
				return unwrapped
			}
			// JSON body was a primitive (string/number/array) — emit
			// it under a `data` key so callers still see a JSON
			// object rather than a bare primitive that downstream
			// deserializers may reject.
			return map[string]interface{}{"data": jsonBody}
		}
		if bodyStr, ok := item["body"].(string); ok {
			// Best-effort: parse the body as JSON if it looks like
			// it. Otherwise emit under `data` so the response is
			// well-formed JSON.
			var parsed map[string]interface{}
			if err := jsonUnmarshal([]byte(bodyStr), &parsed); err == nil {
				return parsed
			}
			return map[string]interface{}{"data": bodyStr}
		}
	}

	// Last node wasn't a transport wrapper (or the wrapper was
	// incomplete) — pass through verbatim.
	return item
}

// unwrapRespondToWebhookBody unwraps the {data: STRING} envelope the
// Respond-to-Webhook node emits when the workflow author used
// `respondWith: "json"` with a string-typed `responseBody` (typically
// via `={{ $json.foo.toJsonString() }}` for XML / text passthrough).
// The webhook response writer would otherwise emit
// `{"data":"<xml>...</xml>"}` as the body, but n8n emits the raw
// string under whatever content-type the workflow author set in
// `responseHeaders`. Matching that wire shape means the manager needs
// to recognise the envelope and pull the bare string out so the
// `WriteResponse` path takes the `case string` branch.
//
// When the item is anything other than a single-key `{data: STRING}`
// envelope (a JSON object body, multiple keys, or a non-string
// `data`), it is returned as-is so JSON-shaped responses still pass
// through unchanged.
func unwrapRespondToWebhookBody(item map[string]interface{}) interface{} {
	if len(item) != 1 {
		return item
	}
	raw, ok := item["data"]
	if !ok {
		return item
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return item
}

func isWebhookNode(nodeType string) bool {
	return nodeType == "n8n-nodes-base.webhook" ||
		strings.Contains(strings.ToLower(nodeType), "webhook")
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func makeWebhookKey(path, method string, isTest bool) string {
	testSuffix := ""
	if isTest {
		testSuffix = ":test"
	}
	return fmt.Sprintf("%s:%s%s", strings.ToUpper(method), path, testSuffix)
}

func getStringParam(params map[string]interface{}, key, defaultValue string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// generateIDCounter is a process-wide counter that makes the generated
// webhook / execution IDs unique even when `time.Now().UnixNano()`
// returns the same value twice in a row (which happens more often than
// intuition suggests on Windows, where the monotonic-clock resolution
// is coarser than on Linux). Using a counter in addition to the
// nanosecond timestamp avoids the silent map-key collisions that would
// otherwise cause `SaveWebhook` to overwrite a just-saved sibling when
// two nodes are registered back-to-back in a tight loop.
var generateIDCounter atomic.Uint64

func generateWebhookID() string {
	return fmt.Sprintf("webhook_%d_%d", time.Now().UnixNano(), generateIDCounter.Add(1))
}

func generateExecutionID() string {
	return fmt.Sprintf("exec_%d_%d", time.Now().UnixNano(), generateIDCounter.Add(1))
}

func generateWebhookExecutionID() string {
	return fmt.Sprintf("wh_exec_%d_%d", time.Now().UnixNano(), generateIDCounter.Add(1))
}
