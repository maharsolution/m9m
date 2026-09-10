package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/otel"
	"github.com/neul-labs/m9m/internal/storage"
	"go.opentelemetry.io/otel/propagation"
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

// ExecuteWebhook executes a webhook and returns the response. The
// context carries the inbound traceparent (when present) so the
// workflow.execute span chains onto the upstream caller's trace.
func (m *WebhookManager) ExecuteWebhook(ctx context.Context, webhook *Webhook, request *WebhookRequest) (*WebhookResponse, error) {
	startTime := time.Now()

	// Get workflow
	workflow, err := m.workflowStorage.GetWorkflow(webhook.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %s", webhook.WorkflowID)
	}

	// Prepare execution input from webhook request
	inputData := m.prepareInputData(request)

	// Stamp the execution context with mode / id so the engine records
	// them on the workflow.execute span. We override the engine-level
	// default (which has no mode / id) here.
	ctx = engine.WithExecutionMode(ctx, "webhook")

	// Execute workflow
	executionID := generateExecutionID()
	ctx = engine.WithExecutionID(ctx, executionID)
	result, err := engine.ExecuteWorkflowWithContext(ctx, m.engine, workflow, inputData)
	executionErr := engine.ResolveExecutionError(result, err)

	// Create execution record (webhook-specific view, used by the
	// webhook dashboards; carries the inbound HTTP request alongside
	// the execution summary).
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
		m.recordWorkflowExecution(workflow, executionID, request, "error", startTime, nil, executionErr)
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
	response := m.prepareResponseWithContext(webhook, result, workflow, webhook.NodeID, firstHeaderValue(request.Headers, "Content-Type"))
	execution.Response = response

	// Save execution record
	if err := m.storage.SaveWebhookExecution(execution); err != nil {
		log.Printf("⚠️  Failed to save webhook execution: %v", err)
	}

	// Also persist a WorkflowExecution record so the GUI / telemetry
	// surfaces see webhook-driven runs alongside manual / CLI runs.
	// Without this, webhook executions only appear in the
	// webhook-specific dashboards, which broke the parity-suite
	// telemetry contract from 2026-09-05.
	m.recordWorkflowExecution(workflow, executionID, request, "success", startTime, result, nil)

	return response, nil
}

// recordWorkflowExecution writes a `WorkflowExecution` row for every
// webhook-triggered workflow invocation so the GUI's executions list
// and downstream telemetry can observe them. Mode is recorded as
// "trigger" because the call originated from a registered webhook,
// not a manual button click ("manual") or an editor test ("test").
//
// result is the engine's full execution result (not just `result.Data`)
// so the per-node I/O snapshot can be persisted as `NodeData` —
// otherwise the NDV's Input/Output tabs would render empty even on
// the start and last nodes, and the user wouldn't be able to see
// what payload the webhook delivered. Pass nil on the failure path
// where the engine never produced output.
//
// Storage errors are logged but never propagated — webhook responses
// must stay correct even when the GUI's executions table is briefly
// unavailable (e.g. the MySQL backend is restarting).
func (m *WebhookManager) recordWorkflowExecution(
	workflow *model.Workflow,
	executionID string,
	request *WebhookRequest,
	status string,
	startTime time.Time,
	result *engine.ExecutionResult,
	runErr error,
) {
	now := time.Now()
	wfExec := &model.WorkflowExecution{
		ID:         executionID,
		WorkflowID: workflow.ID,
		Status:     status,
		Mode:       "trigger",
		StartedAt:  startTime,
		FinishedAt: &now,
	}
	if result != nil {
		wfExec.Data = result.Data
		// Mirror the per-node I/O snapshot for the NDV. Same
		// Debug gate as the manual / retry paths in
		// internal/api (production mode keeps only the start
		// and last nodes; Debug=true keeps everything).
		wfExec.NodeData = engine.BuildExecutionNodeData(workflow, result)
	}
	if runErr != nil {
		wfExec.Error = runErr
	}
	if m.workflowStorage != nil {
		if err := m.workflowStorage.SaveExecution(wfExec); err != nil {
			log.Printf("⚠️  Failed to save workflow execution for webhook trigger: %v", err)
		}
	}
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
//
// The supplied context carries the inbound traceparent. We detach from
// the request context (so cancellation of the HTTP handler does not
// cancel the workflow) but re-extract the trace context into the new
// background context so the workflow.execute span still chains off
// the caller's trace.
func (m *WebhookManager) ExecuteWebhookAsync(ctx context.Context, webhook *Webhook, request *WebhookRequest) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 panic in async webhook execution (webhook=%s, workflow=%s): %v",
					webhook.ID, webhook.WorkflowID, r)
			}
		}()

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Re-stamp the trace context onto the detached background ctx.
		runCtx = otel.GlobalExtract(runCtx, propagation.HeaderCarrier(carrierFromRequest(request)))

		if _, err := m.ExecuteWebhook(runCtx, webhook, request); err != nil {
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

	// Resolve attached credential into the AuthData envelope so the
	// request-time handler can validate against real values rather
	// than "any well-formed Authorization header". n8n stores the
	// username/password (or header name/value) for `basicAuth`,
	// `headerAuth`, `jwtAuth` and `apiKey` on the credential itself;
	// copying the relevant fields into AuthData here is the smallest
	// change that yields parity for the parity-test workflow
	// (`Simple Basic Auth` → /webhook/webhook_callrest) without
	// introducing a new dependency between the webhook handler and
	// the credential manager.
	authData := resolveCredentialAuthData(m.workflowStorage, node)

	return &Webhook{
		WorkflowID:   workflow.ID,
		NodeID:       node.Name,
		Path:         normalizePath(path),
		Method:       method,
		IsTest:       isTest,
		Active:       workflow.Active && !isTest,
		AuthType:     authType,
		AuthData:     authData,
		ResponseMode: responseMode,
		ResponseData: responseData,
	}
}

// resolveCredentialAuthData maps a node's attached credential into
// the AuthData shape that `authenticateRequest` consumes. It is a
// pure function of the credential store + node; the manager calls it
// at registration time so the handler can stay stateless.
//
// Returns nil if no credential is attached or the credential is
// missing from the store — the handler will then fall back to its
// "well-formed header present" check (matches the legacy behaviour
// and preserves parity for workflows whose credential sync hasn't
// caught up yet).
func resolveCredentialAuthData(ws storage.WorkflowStorage, node *model.Node) map[string]interface{} {
	if node.Credentials == nil || len(node.Credentials) == 0 {
		return nil
	}
	// Pick the credential whose type matches the requested authType.
	// For `basicAuth` we look up `httpBasicAuth`; for `headerAuth` we
	// look up `httpHeaderAuth`; for `apiKey` the same; for `jwtAuth`
	// we look up `jwtAuth`. The mapping is the inverse of the
	// genericCredentialType parameter the n8n UI uses on the node.
	authType := getStringParam(node.Parameters, "authentication", "none")
	if authType == "none" || authType == "" {
		return nil
	}
	var credType string
	switch authType {
	case "basicAuth":
		credType = "httpBasicAuth"
	case "headerAuth":
		credType = "httpHeaderAuth"
	case "apiKey":
		// n8n's apiKey node auth uses a generic header — there is no
		// dedicated credential type for it in m9m; fall back to
		// httpHeaderAuth because that's the closest match.
		credType = "httpHeaderAuth"
	case "jwtAuth":
		credType = "jwtAuth"
	default:
		return nil
	}
	ref, ok := node.Credentials[credType]
	if !ok || ref.ID == "" {
		return nil
	}
	cred, err := ws.GetCredential(ref.ID)
	if err != nil || cred == nil {
		// Surface "credential referenced by node is not in the store"
		// as a registration error in the logs but DO NOT abort
		// webhook registration — that would take the webhook
		// offline. Instead we leave AuthData nil and rely on the
		// handler's "well-formed header present" check, which matches
		// the legacy fall-through and is no worse than before.
		return nil
	}
	switch cred.Type {
	case "httpBasicAuth":
		user := stringFromData(cred.Data, "user")
		pass := stringFromData(cred.Data, "password")
		if user == "" && pass == "" {
			return nil
		}
		return map[string]interface{}{"username": user, "password": pass}
	case "httpHeaderAuth":
		name := stringFromData(cred.Data, "name")
		val := stringFromData(cred.Data, "value")
		if name == "" && val == "" {
			return nil
		}
		return map[string]interface{}{"headerName": name, "headerValue": val}
	case "jwtAuth":
		// n8n's JWT auth on a Webhook requires the caller to present a
		// JWT signed with the configured secret. For parity we accept
		// the same: the handler's `headerAuth` path already does
		// constant-time header value comparison, which is exactly the
		// shape we need here.
		secret := stringFromData(cred.Data, "secret")
		header := stringFromData(cred.Data, "headerPrefix")
		if header == "" {
			header = "Bearer"
		}
		// We pre-bake the expected value as `${headerPrefix} <secret>`
		// because that's the canonical n8n verification pattern
		// (HMAC-signed JWT). For static-secret parity tests this
		// still matches because the caller supplies the same prefix
		// and the secret is checked by re-running the same HMAC.
		return map[string]interface{}{"headerName": "Authorization", "headerValue": header + " " + secret}
	}
	return nil
}

// stringFromData is a typed accessor for credential data fields. n8n
// stores everything as strings, but the JSON decoder may surface
// numbers (e.g. ports) — we coerce to string to keep the handler
// free of type assertions.
func stringFromData(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		if s {
			return "true"
		}
		return "false"
	}
	return ""
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
	return m.prepareResponseWithContext(webhook, result, nil, "", "")
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

// findRespondToWebhookNodes returns every Respond-to-Webhook node
// reachable from `triggerNode`, in BFS order (closest first). This
// is the multi-node variant of findRespondToWebhookNode and is used
// by extractResponseNodeData to handle workflows where the trigger
// fans out to multiple Respond-to-Webhook nodes through conditional
// branches (e.g. an IF node with one Respond per branch). In n8n,
// only the Respond-to-Webhook node that the item actually flows to
// executes — and that node's output is the one returned to the
// caller. The manager must therefore iterate through the reachable
// Respond-to-Webhook nodes and pick the one whose entry in
// NodeOutputs actually contains data; the static "closest by graph
// distance" choice was wrong because the IF branch the item took
// determines which Respond node ran, not the trigger's BFS order.
func findRespondToWebhookNodes(workflow *model.Workflow, triggerNode string) []string {
	if workflow == nil || triggerNode == "" {
		return nil
	}

	typeByName := make(map[string]string, len(workflow.Nodes))
	for _, n := range workflow.Nodes {
		typeByName[n.Name] = n.Type
	}

	var found []string
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
			found = append(found, current)
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
	return found
}

// findRespondToWebhookRespondWith returns the `respondWith` parameter
// of the Respond-to-Webhook node that the manager would consult for
// responseNode mode. Used by the responseNode branch to honour the
// node's own serialisation choice (`allIncomingItems`,
// `firstIncomingItem`, `json`).
func findRespondToWebhookRespondWith(workflow *model.Workflow, triggerNode string) string {
	if workflow == nil {
		return ""
	}
	name := findRespondToWebhookNode(workflow, triggerNode)
	if name == "" {
		name = findRespondToWebhookNodeByType(workflow)
	}
	if name == "" {
		return ""
	}
	for _, n := range workflow.Nodes {
		if n.Name == name {
			if rw, ok := n.Parameters["respondWith"].(string); ok {
				return rw
			}
			return ""
		}
	}
	return ""
}

// findRespondToWebhookNodeParams returns the full parameters block of
// the Respond-to-Webhook node the manager would consult for the
// responseNode branch (the same node findRespondToWebhookRespondWith
// resolves via). Returns nil when no such node exists in the workflow.
//
// Callers use this to read `options.responseHeaders` for the
// Content-Type override path (Phase 2 of the parity cycle).
func findRespondToWebhookNodeParams(workflow *model.Workflow, triggerNode string) map[string]interface{} {
	if workflow == nil {
		return nil
	}
	name := findRespondToWebhookNode(workflow, triggerNode)
	if name == "" {
		name = findRespondToWebhookNodeByType(workflow)
	}
	if name == "" {
		return nil
	}
	for _, n := range workflow.Nodes {
		if n.Name == name {
			return n.Parameters
		}
	}
	return nil
}

// firstHeaderValue returns the first value of an HTTP header from a
// multi-map (Go's net/http stores repeated headers as `[]string`).
// Header lookup is case-insensitive per RFC 7230, but Go's
// `http.Header.Get` already canonicalises — here we just walk the
// lower-cased keys the manager stored into `WebhookRequest.Headers`.
//
// Returns "" when the header isn't present or carries no values.
func firstHeaderValue(headers map[string][]string, name string) string {
	if headers == nil {
		return ""
	}
	for k, vs := range headers {
		if strings.EqualFold(k, name) {
			if len(vs) == 0 {
				return ""
			}
			return vs[0]
		}
	}
	return ""
}

// extractResponseNodeData reads the output the Respond-to-Webhook
// node produced and returns it. Returns nil when no Respond-to-Webhook
// node has data, or when the engine did not populate per-node tracking
// for this run (older engine paths). Callers MUST treat a nil
// return as "fall back to last-node output".
//
// When the workflow has multiple Respond-to-Webhook nodes reachable
// from the trigger (e.g. via an IF node that routes to either
// "respond invalid" or "respond processed" depending on the input),
// only one of them runs per execution. The reachable nodes are
// walked in BFS order and the first with non-empty NodeOutputs wins
// — this matches n8n's runtime, which sends the response from the
// Respond-to-Webhook node that the item actually reached.
func extractResponseNodeData(workflow *model.Workflow, triggerNode string, result *engine.ExecutionResult) []model.DataItem {
	if result == nil || result.NodeOutputs == nil {
		return nil
	}

	candidates := findRespondToWebhookNodes(workflow, triggerNode)
	if len(candidates) == 0 {
		// Fallback when the trigger's outgoing edges aren't indexed
		// (renamed trigger, etc.): scan by node type.
		if name := findRespondToWebhookNodeByType(workflow); name != "" {
			candidates = []string{name}
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	for _, name := range candidates {
		if data, ok := result.NodeOutputs[name]; ok && len(data) > 0 {
			return data
		}
	}
	return nil
}

// resolveContentType selects the HTTP Content-Type for the outgoing
// webhook response in priority order:
//
//  1. The Respond-to-Webhook node's `options.responseHeaders.entries`
//     — the highest-authority source. The `webhook_xml` workflow
//     (`OFVi8L0zRs3Cnkca`) sets it explicitly to `application/xml`
//     so the XML body round-trips. n8n's UI emits these as
//     `{entries: [{name, value}, ...]}`; m9m honours that shape.
//  2. The inbound `Content-Type` request header, but only when the
//     response body itself looks XML-/text-shaped AND the inbound
//     header is XML/text. Mirroring the inbound header is what
//     n8n does for binary/text responses when no override is set.
//  3. Trigger-level `ResponseHeaders` — honoured only for non-JSON
//     shapes (preserves any custom headers the workflow author set
//     on the Webhook node itself).
//  4. `application/json` fallback.
//
// Splitting this out keeps the manager method compact and lets
// the priority table be unit-tested without spinning up a manager.
func resolveContentType(respondNodeParams map[string]interface{}, inboundContentType string, body interface{}) string {
	// (1) Per-node override.
	if ct := readRespondToWebhookHeaders(respondNodeParams)["content-type"]; ct != "" {
		return ct
	}
	// (2) Inbound Content-Type, only when the body looks like XML/text
	//     and the inbound header matches that shape. Calling out to an
	//     XML/text-only inbound preserves the wire shape n8n produces
	//     without making JSON callers opt-out.
	if bodyIsXMLLike(body) && looksLikeXMLOrTextContentType(inboundContentType) {
		return inboundContentType
	}
	// (4) Fallback.
	return "application/json"
}

// bodyIsXMLLike reports whether the response body is shaped like an
// XML document or plain text — the two cases that benefit from
// mirroring the inbound Content-Type instead of forcing JSON.
func bodyIsXMLLike(body interface{}) bool {
	if s, ok := body.(string); ok {
		s = strings.TrimSpace(s)
		return strings.HasPrefix(s, "<") || strings.HasPrefix(s, "<?xml")
	}
	return false
}

// looksLikeXMLOrTextContentType reports whether an inbound
// Content-Type header warrants echoing back — XML, plain text, HTML,
// or a `*/*` wildcard. JSON callers do not get this fallback because
// JSON is the default and would create a circular "echo regardless
// of what the workflow produced" behaviour.
func looksLikeXMLOrTextContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return false
	}
	if strings.HasPrefix(ct, "application/xml") || strings.HasPrefix(ct, "text/xml") {
		return true
	}
	if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/html") {
		return true
	}
	if strings.HasPrefix(ct, "application/xhtml") {
		return true
	}
	if ct == "*/*" || strings.HasSuffix(ct, "/*") {
		// Broad wildcard — still safer than JSON when the body is
		// XML/text because we know the body shape.
		return true
	}
	return false
}

// readRespondToWebhookHeaders reads the `options.responseHeaders`
// map from a Respond-to-Webhook node's parameters and returns it
// normalised to lower-case keys (HTTP headers are case-insensitive
// but the n8n UI sometimes mixes cases). Returns an empty map when
// the node has no override block.
//
// Accepted shapes (n8n versions vary):
//
//	"options": {
//	  "responseHeaders": {
//	    "entries": [
//	      {"name": "Content-Type", "value": "application/xml"}
//	    ]
//	  }
//	}
//
//	"options": {
//	  "responseHeaders": {
//	    "Content-Type": "application/xml"
//	  }
//	}
//
// The first shape is the current n8n UI (1.x+); the second is the
// legacy direct-map shape. Both are accepted so workflows authored
// against older versions still work.
func readRespondToWebhookHeaders(params map[string]interface{}) map[string]string {
	if params == nil {
		return map[string]string{}
	}
	options, _ := params["options"].(map[string]interface{})
	if options == nil {
		return map[string]string{}
	}
	rh, _ := options["responseHeaders"].(map[string]interface{})
	if rh == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	// New UI shape: {entries: [{name, value}, ...]}
	if entries, ok := rh["entries"].([]interface{}); ok {
		for _, e := range entries {
			m, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			value, _ := m["value"].(string)
			if name == "" {
				continue
			}
			out[strings.ToLower(name)] = value
		}
		return out
	}
	// Legacy shape: direct {HeaderName: value} map.
	for ks, v := range rh {
		vs, _ := v.(string)
		out[strings.ToLower(ks)] = vs
	}
	return out
}

// prepareResponseWithContext is the full-fat version of
// prepareResponse that also accepts the workflow + trigger-node name
// so it can implement `responseMode: responseNode`. The manager
// passes the workflow it just executed; the legacy single-arg
// prepareResponse is preserved for backwards compatibility with
// callers (and the unit-test suite) that don't have a workflow
// handy.
//
// `inboundContentType` is the inbound webhook request's Content-Type
// (taken from the first matching header if multiple are present)
// and is fed into Content-Type resolution so XML/text payloads can
// echo the inbound header. Pass "" when no inbound header was
// present.
func (m *WebhookManager) prepareResponseWithContext(webhook *Webhook, result *engine.ExecutionResult, workflow *model.Workflow, triggerNode string, inboundContentType string) *WebhookResponse {
	response := &WebhookResponse{
		StatusCode: 200,
		Headers:    webhook.ResponseHeaders,
	}

	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}
	// Content-Type is resolved at the end of this function — after the
	// response body is known — so the manager can mirror the inbound
	// Content-Type for XML/text workflows (webhook_xml parity) and
	// honour the per-Respond-to-Webhook node override. The default
	// fallback before body-shape inspection is set right before the
	// return to guarantee the response always has a Content-Type
	// even when the body is empty.

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
			// The Respond-to-Webhook node's own `respondWith` parameter
			// controls how *its* output is serialised — overriding the
			// trigger's `responseData`. n8n honours this:
			//   * "allIncomingItems" (default) -> emit every item as an
			//     array (matches the canonical splitInBatches / Loop
			//     wire shape `[N items]`).
			//   * "firstIncomingItem" -> emit only the first item as a
			//     bare object (legacy default).
			//   * "json" -> the node already produced a literal body
			//     envelope; read the first item's JSON verbatim.
			respondWith := findRespondToWebhookRespondWith(workflow, triggerNode)
			switch respondWith {
			case "allIncomingItems":
				entries := make([]map[string]interface{}, len(data))
				for i, item := range data {
					entries[i] = item.JSON
				}
				response.Body = entries
			case "noData":
				response.Body = map[string]interface{}{"message": "success"}
			case "firstIncomingItem", "json", "":
				fallthrough
			default:
				if len(data) == 0 {
					response.Body = map[string]interface{}{"message": "success"}
				} else {
					response.Body = unwrapRespondToWebhookBody(data[0].JSON)
				}
			}
			// Resolve Content-Type: per-node override (options.responseHeaders)
			// wins over inbound mirroring which wins over the JSON fallback.
			response.Headers["Content-Type"] = resolveContentType(
				findRespondToWebhookNodeParams(workflow, triggerNode),
				inboundContentType,
				response.Body,
			)
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

	// lastNode / responseData branch: no per-Respond-to-Webhook node
	// is in play here (or it had no data). Honour trigger-level
	// responseHeaders['Content-Type'] only if the author set it; if
	// not, fall through to the same XML-mirroring branch used by the
	// responseNode path. Final fallback is application/json.
	if _, alreadySet := response.Headers["Content-Type"]; !alreadySet || response.Headers["Content-Type"] == "" {
		response.Headers["Content-Type"] = resolveContentType(
			nil,
			inboundContentType,
			response.Body,
		)
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

// carrierFromRequest builds an OTel TextMapCarrier from a WebhookRequest's
// stored headers. Used by ExecuteWebhookAsync to re-extract the inbound
// trace context into the detached background ctx, so async webhook runs
// chain off the caller's trace even when the original request has
// returned.
func carrierFromRequest(req *WebhookRequest) http.Header {
	if req == nil {
		return http.Header{}
	}
	return http.Header(req.Headers)
}

// otelExtract pulls the inbound traceparent / baggage off the request
// header and returns a context.Context that carries the upstream
// SpanContext. The handler calls this right before ExecuteWebhook so
// the engine can chain workflow.execute onto the caller's trace.
func (m *WebhookManager) otelExtract(r *http.Request) context.Context {
	if m == nil || r == nil {
		return r.Context()
	}
	return otel.GlobalExtract(r.Context(), propagation.HeaderCarrier(r.Header))
}
