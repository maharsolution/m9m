/*
Package engine provides the core workflow execution engine for m9m.
*/
package engine

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/neul-labs/m9m/internal/connections"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/expressions"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/otel"
)

// ExecutionResult represents the result of a workflow execution
type ExecutionResult struct {
	Data  []model.DataItem `json:"data"`
	Error error            `json:"error,omitempty"`

	// NodeOutputs carries the per-node output produced during this
	// execution. The map is keyed by node name (the same string used
	// in `workflow.Connections`) and the value is the exact slice of
	// DataItems that node emitted at the end of its run (i.e. after
	// routing-metadata stripping).
	//
	// This is what powers `responseMode: responseNode` on the Webhook
	// trigger — when a workflow ships a "Respond to Webhook" node,
	// the webhook manager reads the respond-to-webhook node's output
	// out of this map and returns it verbatim instead of returning
	// the last-node output. Without this map the manager would have
	// to re-traverse `workflow.Connections` and re-execute the
	// respond-to-webhook lookup, which would diverge from n8n's
	// wire shape.
	//
	// For workflows where per-node tracking wasn't populated (older
	// engine paths, the streaming/parallel engines, or `Reliable`
	// engine) the map is nil and consumers must fall back to the
	// legacy `Data` field (the last-node output).
	NodeOutputs map[string][]model.DataItem `json:"nodeOutputs,omitempty"`

	// EdgesTaken records which connections in the workflow actually
	// carried at least one item during this execution. The key format
	// is "<sourceNodeID>:<outputIndex>:<targetNodeID>:<inputIndex>"
	// — the same shape used by the frontend's
	// `buildEdgeId(sourceNodeId, sourceOutput, targetNodeId,
	// targetInput)` helper, minus the leading "workflow-edge:"
	// prefix and URL-encoding (those are presentation concerns owned
	// by the UI; the engine emits the raw tuple).
	//
	// Why this exists: a user reading an execution should be able to
	// tell which branches of an IF/Switch actually carried items
	// versus which were skipped, without us having to persist every
	// node's full I/O snapshot. Storing one bool per edge is O(E)
	// (typically < 10 even for branching workflows), which is two
	// orders of magnitude smaller than the per-node snapshots the
	// Debug=OFF helper otherwise records.
	//
	// Populated by the engine's execution loop as it routes data
	// from node to node. Empty for workflows without branching.
	EdgesTaken map[string]bool `json:"edgesTaken,omitempty"`
}

// NodeRegistry maps node types to their executors
type NodeRegistry map[string]base.NodeExecutor

// WorkflowEngine is the interface for workflow execution
type WorkflowEngine interface {
	// ExecuteWorkflow executes a complete workflow
	ExecuteWorkflow(workflow *model.Workflow, inputData []model.DataItem) (*ExecutionResult, error)

	// ExecuteWorkflowParallel executes multiple workflows in parallel
	ExecuteWorkflowParallel(workflows []*model.Workflow, inputData [][]model.DataItem) ([]*ExecutionResult, error)

	// RegisterNodeExecutor registers a node executor for a node type
	RegisterNodeExecutor(nodeType string, executor base.NodeExecutor)

	// GetNodeExecutor retrieves a node executor for a node type
	GetNodeExecutor(nodeType string) (base.NodeExecutor, error)

	// SetCredentialManager sets the credential manager for the engine
	SetCredentialManager(credentialManager *credentials.CredentialManager)

	// SetConnectionRouter sets the connection router for the engine
	SetConnectionRouter(connectionRouter connections.ConnectionRouter)

	// GetRegisteredNodeTypes returns metadata for all registered node executors.
	GetRegisteredNodeTypes() []NodeTypeInfo
}

// NodeTypeInfo describes a registered node type for catalogs and APIs.
type NodeTypeInfo struct {
	TypeID      string              `json:"name"`
	DisplayName string              `json:"displayName"`
	Description string              `json:"description"`
	Category    string              `json:"category"`
	Version     int                 `json:"version"`
	Inputs      []string            `json:"inputs"`
	Outputs     []string            `json:"outputs"`
	Properties  []base.NodeProperty `json:"properties,omitempty"`
}

// workflowEngineImpl is the concrete implementation of WorkflowEngine
type workflowEngineImpl struct {
	nodeRegistry      NodeRegistry
	credentialManager *credentials.CredentialManager
	connectionRouter  connections.ConnectionRouter
	otelManager       *otel.Manager

	// Per-execution context. Prefer the execFrame stored on ctx
	// (see withExecFrame) so concurrent webhook + Kafka runs on the
	// same engine cannot clobber each other's runExecutionData.
	// The fields below remain as a fallback for older call paths /
	// tests that invoke executeNodeWithContext without a frame.
	workflow         *model.Workflow
	runExecutionData *expressions.RunExecutionData
	runIndex         int
	itemIndex        int

	// edgesTaken is the accumulator the engine populates as it
	// routes items between nodes. Each entry is keyed by
	// `<sourceNodeID>:<outputIndex>:<targetNodeID>:<inputIndex>`
	// (matching the frontend's edge ID minus the
	// "workflow-edge:" prefix and URL-encoding) and the value is
	// true when at least one item flowed across that connection.
	// Reset to a fresh map at the start of every ExecuteWorkflow
	// call so the previous run's data never leaks across runs.
	edgesTaken map[string]bool
}

// execFrame holds the per-run state that used to live on the engine
// singleton. Storing it on ctx means Finnet's webhook poller and the
// Kafka consumer can run concurrently without clobbering
// `$('Async Subworkflow').item.json.key` mid-Find.
type execFrame struct {
	workflow         *model.Workflow
	runExecutionData *expressions.RunExecutionData
	runIndex         int
	itemIndex        int
}

type execFrameKey struct{}

func withExecFrame(ctx context.Context, f *execFrame) context.Context {
	return context.WithValue(ctx, execFrameKey{}, f)
}

func execFrameFrom(ctx context.Context) *execFrame {
	if ctx == nil {
		return nil
	}
	f, _ := ctx.Value(execFrameKey{}).(*execFrame)
	return f
}

func (e *workflowEngineImpl) activeRunData(ctx context.Context) *expressions.RunExecutionData {
	if f := execFrameFrom(ctx); f != nil && f.runExecutionData != nil {
		return f.runExecutionData
	}
	return e.runExecutionData
}

func (e *workflowEngineImpl) setActiveRunIndex(ctx context.Context, idx int) {
	if f := execFrameFrom(ctx); f != nil {
		f.runIndex = idx
	}
	e.runIndex = idx
}

// NewWorkflowEngine creates a new workflow engine
func NewWorkflowEngine() WorkflowEngine {
	return &workflowEngineImpl{
		nodeRegistry:     make(NodeRegistry),
		connectionRouter: connections.NewConnectionRouter(),
	}
}

// SetOTelManager wires the OpenTelemetry Manager so the engine emits
// workflow.execute / node.execute spans. Calling this is optional;
// when manager is nil the engine runs without instrumentation.
func (e *workflowEngineImpl) SetOTelManager(manager *otel.Manager) {
	e.otelManager = manager
}

// RegisterNodeExecutor registers a node executor for a node type
func (e *workflowEngineImpl) RegisterNodeExecutor(nodeType string, executor base.NodeExecutor) {
	e.nodeRegistry[nodeType] = executor
}

// GetNodeExecutor retrieves a node executor for a node type
func (e *workflowEngineImpl) GetNodeExecutor(nodeType string) (base.NodeExecutor, error) {
	executor, exists := e.nodeRegistry[nodeType]
	if !exists {
		return nil, fmt.Errorf("no executor registered for node type: %s", nodeType)
	}
	return executor, nil
}

// SetCredentialManager sets the credential manager for the engine
func (e *workflowEngineImpl) SetCredentialManager(credentialManager *credentials.CredentialManager) {
	e.credentialManager = credentialManager
}

// SetConnectionRouter sets the connection router for the engine
func (e *workflowEngineImpl) SetConnectionRouter(connectionRouter connections.ConnectionRouter) {
	e.connectionRouter = connectionRouter
}

// GetRegisteredNodeTypes returns metadata for all registered node executors.
func (e *workflowEngineImpl) GetRegisteredNodeTypes() []NodeTypeInfo {
	var result []NodeTypeInfo
	for typeID, executor := range e.nodeRegistry {
		desc := executor.Description()
		inputs := desc.Inputs
		if len(inputs) == 0 {
			inputs = []string{"main"}
		}
		outputs := desc.Outputs
		if len(outputs) == 0 {
			outputs = []string{"main"}
		}
		result = append(result, NodeTypeInfo{
			TypeID:      typeID,
			DisplayName: desc.Name,
			Description: desc.Description,
			Category:    desc.Category,
			Version:     1,
			Inputs:      inputs,
			Outputs:     outputs,
			Properties:  desc.Properties,
		})
	}
	return result
}

// ExecuteWorkflow executes a complete workflow
func (e *workflowEngineImpl) ExecuteWorkflow(workflow *model.Workflow, inputData []model.DataItem) (*ExecutionResult, error) {
	return e.ExecuteWorkflowWithContext(context.Background(), workflow, inputData)
}

// ExecuteWorkflowWithContext executes a workflow honoring caller cancellation.
func (e *workflowEngineImpl) ExecuteWorkflowWithContext(ctx context.Context, workflow *model.Workflow, inputData []model.DataItem) (*ExecutionResult, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Open the workflow.execute span. The function-level defer below
	// records the final status and ends the span; we read mode from
	// the ctx-injected value the caller set up (set by the webhook
	// handler for webhook-driven runs, by the scheduler for cron
	// runs, by the MCP/API layer for manual runs).
	mode := workflowExecutionMode(ctx)
	execID := workflowExecutionID(ctx)
	isRetry := workflowExecutionIsRetry(ctx)
	retryOf := workflowExecutionRetryOf(ctx)

	wfAttrs := otel.WorkflowAttrs{
		WorkflowID:        workflow.ID,
		WorkflowName:      workflow.Name,
		WorkflowVersionID: workflow.VersionID,
		NodeCount:         len(workflow.Nodes),
		ProjectID:         workflow.ProjectID,
		ExecutionID:       execID,
		ExecutionMode:     mode,
		ExecutionStatus:   "running",
		IsRetry:           isRetry,
		RetryOf:           retryOf,
		WorkflowCustom:    workflow.CustomSpanAttributes,
	}

	// Open the workflow.execute span. The OTel tracer.Start convention
	// attaches the new span to ctx so any subsequent tracer.Start
	// (including the per-node spans opened by the loop below) chain
	// off it automatically.
	ctx, wfSpan := e.otelManager.StartWorkflowSpan(ctx, wfAttrs)

	// Hook a continuation link if the workflow resumed after a Wait
	// (the engine attaches the previous SpanContext on ctx before
	// calling us).
	if prev := workflowContinuationSpan(ctx); prev.IsValid() {
		otel.AddSpanLink(wfSpan, prev, otel.AttrContinuationReason.String("after_wait"))
	}

	// Keep a handle on the workflow span so the deferred cleanup can
	// record the final status. The same span is also reachable via
	// trace.SpanFromContext for downstream code.
	ctx = withWorkflowSpan(ctx, wfSpan)
	wfSpanFinal := wfSpan

	defer func() {
		if wfSpanFinal != nil {
			// The status was set to "running" at start; if the
			// engine reports an error result, replace it. The
			// engine itself updates status to "completed" /
			// "failed" / "cancelled" via the helper below.
			otel.EndWorkflowSpan(wfSpanFinal, "completed", nil)
		}
	}()

	// Save + reset the EdgesTaken accumulator before every run so a
	// re-used engine (long-lived process) never leaks edges from
	// the previous workflow into the current one. The map is
	// small (O(E)) and the allocation cost is negligible.
	//
	// For the SUB-workflow case (an ExecuteWorkflow node recursing
	// back into ExecuteWorkflowWithContext with a different
	// workflow), `parentEdgesTaken` holds the caller's accumulated
	// edges. Without merging them back at the end of this call,
	// the inner run would clobber the caller's accumulator — so
	// when the OUTER (parent) workflow later returns its result,
	// its EdgesTaken would only contain edges recorded AFTER the
	// sub-workflow returned, not the parent's own edges. That's
	// the bug behind "execution view shows grey lines on the
	// parent workflow that contains an Execute Workflow node":
	// the persisted EdgesTaken only had sub-workflow keys, so the
	// frontend's `parseEdgeId(parentId:...)` lookups all missed
	// and fell through to the default slate-400 stroke. The merge
	// happens just before the result is constructed below.
	parentEdgesTaken := e.edgesTaken
	e.edgesTaken = make(map[string]bool)
	if execID == "" {
		execID = fmt.Sprintf("exec_%d", time.Now().UnixNano())
	}
	frame := &execFrame{
		workflow: workflow,
		runExecutionData: &expressions.RunExecutionData{
			ExecutionData: &expressions.ExecutionData{
				ContextData: make(map[string]interface{}),
			},
			ResultData: &expressions.RunData{
				NodeData: make(map[string][]expressions.NodeExecutionResult),
			},
			ExecutionID: execID,
			StartedAt:   time.Now(),
		},
	}
	// Sub-workflow inputs (Finnet Async Sub Workflow → Execute
	// Workflow Trigger) carry the Payload `key`. Stash it on the
	// run so Find Respond can recover when
	// `$('Async Subworkflow').item.json.key` resolves to JS
	// undefined during the If-retry poll.
	if IsSubWorkflowStart(ctx) && len(inputData) > 0 {
		if k, ok := inputData[0].JSON["key"].(string); ok && k != "" {
			frame.runExecutionData.ExecutionData.ContextData["subworkflowKey"] = k
		}
	}
	ctx = withExecFrame(ctx, frame)
	// Mirror onto the engine fields for fallbacks / tests, but the
	// live path always reads from the ctx frame so concurrent runs
	// stay isolated.
	e.workflow = frame.workflow
	e.runExecutionData = frame.runExecutionData
	e.runIndex = 0
	e.itemIndex = 0
	defer func() {
		// Only clear the engine fields when this frame is still the
		// one mirrored there — a concurrent run may have already
		// overwritten them with its own frame.
		if e.runExecutionData == frame.runExecutionData {
			e.workflow = nil
			e.runExecutionData = nil
			e.runIndex = 0
			e.itemIndex = 0
		}
	}()

	// Handle empty workflow
	if len(workflow.Nodes) == 0 {
		return &ExecutionResult{
			Data: inputData,
		}, nil
	}

	// Validate workflow connections
	if err := e.connectionRouter.ValidateConnections(workflow); err != nil {
		return nil, fmt.Errorf("invalid workflow connections: %w", err)
	}

	// Check for cycles in workflow. We tolerate cycles when the
	// workflow uses n8n's `splitInBatches` (Loop) node, because the
	// pattern `Process Item → Loop Over Items → Process Item` is the
	// canonical n8n loop construction: `splitInBatches` sends the
	// next batch to its `main[1]` branch each iteration, then sends
	// the final "done" payload to `main[0]`. The engine handles the
	// iteration internally, so the connection-level cycle is benign.
	hasCycles, err := e.connectionRouter.HasCycles(workflow)
	if err != nil {
		return nil, fmt.Errorf("error checking for workflow cycles: %w", err)
	}

	if hasCycles && !connections.WorkflowUsesSplitInBatches(workflow) && !connections.WorkflowUsesIfRetryLoop(workflow) {
		return nil, fmt.Errorf("workflow contains cycles - cannot execute")
	}

	// Resolve workflow credentials if credential manager is available
	if e.credentialManager != nil {
		err := e.credentialManager.ResolveWorkflowCredentials(workflow)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve workflow credentials: %w", err)
		}
	}

	// Get execution order for nodes
	executionOrder, err := e.connectionRouter.GetExecutionOrder(workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to determine execution order: %w", err)
	}

	// Execute nodes in order
	nodeResults := make(map[string][]model.DataItem)
	// Per-target input-index buckets. n8n Merge chooseBranch
	// (Finnet post_balance) waits for both inputs then emits
	// ONLY `useDataOfInput` (default 1 = Payload, which carries
	// `key`). Flattening both inputs into nodeResults made the
	// Async Subworkflow see the ServiceRoutePlans item without
	// `key`, so Find Respond queried `{key:"undefined"}` and the
	// webhook returned an empty body even though mongo had the
	// doc under the real exec_* key.
	nodeInputResults := make(map[string]map[int][]model.DataItem)

	// Build node lookup map for O(1) access (instead of O(n) linear search)
	nodeMap := make(map[string]*model.Node, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nodeMap[workflow.Nodes[i].Name] = &workflow.Nodes[i]
	}

	// Initialize with input data for starting nodes (nodes with no incoming connections).
	// Sub-workflow invocations (Execute Workflow → same/other workflow)
	// must start ONLY at executeWorkflowTrigger — otherwise every
	// Webhook/Kafka trigger is re-seeded and Finnet post_balance
	// recurses until the HTTP client times out.
	startingNodes := e.findStartingNodes(workflow)
	if IsSubWorkflowStart(ctx) {
		ewtStarts := e.findExecuteWorkflowTriggerStarts(workflow)
		if len(ewtStarts) > 0 {
			// Seed non-EWT entry points with empty input so the
			// engine's nil→workflow-input fallback does not re-run
			// Webhook/Kafka and recurse into Execute Workflow.
			ewtSet := make(map[string]bool, len(ewtStarts))
			for _, n := range ewtStarts {
				ewtSet[n] = true
			}
			for _, n := range startingNodes {
				if !ewtSet[n] {
					nodeResults[n] = []model.DataItem{}
				}
			}
			startingNodes = ewtStarts
		}
	} else if mode := workflowExecutionMode(ctx); mode == "webhook" {
		// Webhook invocations must start ONLY at webhook nodes —
		// otherwise executeWorkflowTrigger / Kafka Trigger are also
		// seeded and Finnet's Set Counter→Find Respond poll runs on
		// the parent before finnet-queue has published.
		webhookStarts := e.findWebhookStarts(workflow)
		if len(webhookStarts) > 0 {
			whSet := make(map[string]bool, len(webhookStarts))
			for _, n := range webhookStarts {
				whSet[n] = true
			}
			for _, n := range startingNodes {
				if !whSet[n] {
					nodeResults[n] = []model.DataItem{}
				}
			}
			startingNodes = webhookStarts
		}
	}

	// Set input data for starting nodes
	for _, nodeName := range startingNodes {
		nodeResults[nodeName] = inputData
	}

	// Pre-seed downstream nodes (those with incoming connections) with
	// an empty input slice so the engine's "no routed data → fallback
	// to a single empty item" path doesn't accidentally fire for
	// nodes that sit on the unselected branch of an IF/Switch.
	//
	// Background: ConnectionRouter.RouteData only populates routedData
	// for targets that actually receive items. If the upstream is an
	// IF/Switch and the routing tag diverts every item to a sibling
	// branch, the unselected branch's downstream nodes are NEVER
	// present in routedData — so nodeResults[name] remains nil, and
	// the engine's nil-check (`if inputDataForNode == nil`) substitutes
	// a single placeholder item. That placeholder then flows through
	// the unselected branch (e.g. into a database query that
	// shouldn't run at all for this input), which is exactly what
	// was happening with the db_quiry workflow: db_type=mysql routed
	// through the IF's main[0] (MySQL Switch → MySQL node) AND the
	// database query node on the PostgreSQL branch was still being
	// invoked once with the placeholder item, triggering an SSL
	// connection failure.
	//
	// Seeding nodeResults with [] for every node that has incoming
	// connections fixes this: when routing returns 0 items, the node
	// sees len(inputDataForNode) == 0 and the engine skips it (same
	// behaviour as if upstream routed an empty payload).
	for _, conn := range workflow.Connections {
		for _, branches := range conn.Main {
			for _, c := range branches {
				if c.Node == "" {
					continue
				}
				if _, already := nodeResults[c.Node]; !already {
					nodeResults[c.Node] = []model.DataItem{}
				}
			}
		}
	}

	// Pre-compute the body-chain and done-chain orderings for every
	// splitInBatches node in the workflow. The engine drives
	// iteration for these nodes itself (instead of calling
	// splitInBatches.Execute as a normal node) because n8n's loop
	// pattern relies on the back-edge from the body chain to the
	// splitInBatches node — a connection-level cycle that the
	// per-node topological loop cannot iterate on its own. Each
	// batch's body chain is executed with an incrementing runIndex
	// so `$("Process Item").all()` and friends resolve correctly
	// during the iteration and again after the loop is done.
	loopBodyOrder := map[string][]string{}
	loopDoneOrder := map[string][]string{}
	// nodesHandledByLoopDriver is the union of every node that
	// belongs to *some* splitInBatches loop's body or done chain.
	// The main engine loop skips these nodes (they're executed
	// by the loop driver when the splitInBatches node itself
	// runs). Without this skip the body chain would execute
	// twice: once inside the loop driver and again from the
	// top-level topological loop, producing duplicate `processedAt`
	// assignments and (for the done branch) duplicate `response`
	// fields.
	nodesHandledByLoopDriver := make(map[string]bool)
	for _, n := range workflow.Nodes {
		if n.Type != "n8n-nodes-base.splitInBatches" {
			continue
		}
		bodyOrder := connections.GetLoopIterationOrder(workflow, n.Name)
		doneOrder := connections.GetLoopDoneOrder(workflow, n.Name)
		loopBodyOrder[n.Name] = bodyOrder
		loopDoneOrder[n.Name] = doneOrder
		for _, name := range bodyOrder {
			nodesHandledByLoopDriver[name] = true
		}
		for _, name := range doneOrder {
			nodesHandledByLoopDriver[name] = true
		}
	}

	// Detect If-retry loops (`attempt < maxAttempts` guard
	// with a back-edge into the body). Like splitInBatches,
	// the body nodes need to be skipped from the top-level
	// topological loop — the engine drives the iteration when
	// it reaches the body entry (BackEdgeTarget) with seeded
	// input, or the guard node.
	ifRetryLoops := connections.DetectIfRetryLoops(workflow)
	for i := range ifRetryLoops {
		for _, name := range ifRetryLoops[i].BodyNodes {
			nodesHandledByLoopDriver[name] = true
		}
	}
	ifRetryDriven := make(map[string]bool, len(ifRetryLoops))

	// Execute each node in order
	for _, nodeName := range executionOrder {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get the node by name using O(1) lookup
		node := nodeMap[nodeName]
		if node == nil {
			return nil, fmt.Errorf("node %s not found in workflow", nodeName)
		}

		// Skip nodes that belong to a splitInBatches body or done
		// chain / If-retry body. These are executed by the engine's
		// loop driver. Exception: when the If-retry body entry
		// (BackEdgeTarget) already has items from Set Counter, drive
		// the loop here — the guard If sits AFTER the body and would
		// otherwise see empty input and never start (H18).
		if nodesHandledByLoopDriver[nodeName] {
			if loop := connections.IfRetryLoopForNode(workflow, nodeName); loop != nil &&
				nodeName == loop.BackEdgeTarget &&
				len(nodeResults[nodeName]) > 0 &&
				!ifRetryDriven[loop.GuardNode] {
				guardNode := nodeMap[loop.GuardNode]
				if guardNode != nil {
					ifRetryDriven[loop.GuardNode] = true
					if err := e.executeIfRetryLoop(
						ctx,
						workflow,
						guardNode,
						*loop,
						nodeResults,
						e.activeRunData(ctx),
						nodeResults[nodeName],
					); err != nil {
						return &ExecutionResult{
							Data:  nil,
							Error: fmt.Errorf("error executing If-retry loop %s: %w", loop.GuardNode, err),
						}, nil
					}
				}
			}
			continue
		}

		// Get the executor for this node type
		executor, err := e.GetNodeExecutor(node.Type)
		if err != nil {
			// n8n exports frequently include decorative nodes (Sticky
			// Notes, Canvas Notes, etc.) that are not part of the
			// execution graph. They are canvas annotations that show
			// up in the editor but never run. Treat them as no-ops
			// during execution so a single decorative node does not
			// poison an otherwise-executable workflow.
			if isDecorativeNodeType(node.Type) {
				nodeResults[nodeName] = []model.DataItem{}
				continue
			}
			// Trigger-only nodes (e.g. Manual Trigger) are UI-side
			// start buttons: they have no executable side. When the
			// workflow runs via a real trigger (Webhook, Cron, etc.)
			// these should be skipped silently, not abort execution.
			//
			// The trigger still has to PASS ITS INPUT DOWNSTREAM.
			// n8n's Manual Trigger emits one item, and the nodes
			// after it are what actually do the work. Clearing
			// `nodeResults` here (the old behaviour) emptied the
			// item the starting-node seeding had just put there,
			// so every downstream node was skipped with
			// "SKIP empty input" and the workflow produced no
			// output at all — a Manual Trigger → Set → Kafka
			// workflow silently published nothing.
			if isTriggerOnlyNodeType(node.Type) {
				if len(nodeResults[nodeName]) == 0 {
					nodeResults[nodeName] = inputData
				}
				// Route the trigger's items to its downstream nodes
				// exactly like any other node. Without this the
				// Manual Trigger's item never left the trigger and
				// every node after it was skipped as "empty input".
				triggerOutput := nodeResults[nodeName]
				routedData, routeErr := e.connectionRouter.RouteData(nodeName, workflow, triggerOutput)
				if routeErr != nil {
					return nil, fmt.Errorf("error routing data from node %s: %w", node.Name, routeErr)
				}
				for targetNode, data := range routedData {
					if nodeResults[targetNode] == nil {
						nodeResults[targetNode] = data
					} else {
						nodeResults[targetNode] = append(nodeResults[targetNode], data...)
					}
				}
				recordEdgesTaken(e.edgesTaken, node, triggerOutput, workflow)
				continue
			}
			return nil, fmt.Errorf("failed to get executor for node %s: %w", node.Name, err)
		}

		// Get input data for this node
		inputDataForNode := nodeResults[nodeName]
		if inputDataForNode == nil {
			// No specific input data. Historically we substituted a
			// single empty placeholder item here; that broke IF/Switch
			// routing because nodes downstream of an unselected branch
			// would still execute with that placeholder (e.g. a
			// PostgreSQL query running for an input that the IF routed
			// to the MySQL branch). Now we seed every downstream node
			// with []model.DataItem{} up front (see the pre-seed loop
			// above), so reaching this nil-check means the workflow
			// definition has a node with no incoming connections AND
			// the trigger didn't seed it. Fall back to the webhook
			// input data in that single case.
			inputDataForNode = inputData
		}
		// Merge chooseBranch: n8n emits ONLY the selected input's
		// items (default useDataOfInput=1 → Payload with `key` on
		// Finnet). Replace the flattened multi-input slice before
		// the empty-input guard so we don't feed ServiceRoutePlans
		// into Kafka / Async Sub Workflow.
		if selected, ok := chooseBranchInput(node, nodeInputResults[nodeName]); ok {
			inputDataForNode = selected
		}
		// Skip nodes that received no input. This mirrors n8n's
		// "0 items → node doesn't run" behaviour and is the only
		// thing that prevents the unselected IF/Switch branch from
		// firing (e.g. running the PostgreSQL query for a
		// db_type=mysql input just because we synthesised an empty
		// placeholder item for it).
		//
		// Trigger-style nodes (webhook, schedule, etc.) are an
		// exception — they get their data from outside the
		// connection graph and start the workflow even when their
		// nodeResults entry happens to be empty at this point.
		if len(inputDataForNode) == 0 && !isTriggerOnlyNodeType(node.Type) {
			nodeResults[nodeName] = []model.DataItem{}
			continue
		}

		// splitInBatches is n8n's loop construct. The connection
		// graph contains a back-edge from the body chain back to
		// this node, which the per-node topological sort cannot
		// iterate on its own. The engine therefore takes over the
		// node's execution: it splits the input into batches,
		// executes the body chain once per batch (with an
		// incrementing runIndex), then executes the once-per-loop
		// done branch exactly once with the aggregated output.
		// The splitInBatches node itself only contributes its
		// nodeParameters to this loop (batchSize, options).
		if node.Type == "n8n-nodes-base.splitInBatches" {
			if err := e.executeSplitInBatchesLoop(
				ctx,
				workflow,
				node,
				nodeResults,
				e.activeRunData(ctx),
				inputDataForNode,
				loopBodyOrder[node.Name],
				loopDoneOrder[node.Name],
			); err != nil {
				return &ExecutionResult{
					Data:  nil,
					Error: fmt.Errorf("error executing splitInBatches loop %s: %w", node.Name, err),
				}, nil
			}
			continue
		}

		// If-retry loop guard: this If node's condition is
		// `attempt < maxAttempts` and its true branch points
		// back into the loop body. The body nodes have already
		// been removed from the top-level loop via
		// `nodesHandledByLoopDriver`, so the engine needs to
		// drive the body here. We run the body up to
		// MaxIterations times, stopping early when the guard's
		// condition is false (i.e. the If would route to its
		// main[1] "false" branch on this iteration).
		if loop := connections.IfRetryLoopForGuard(workflow, node.Name); loop != nil {
			if ifRetryDriven[loop.GuardNode] {
				continue
			}
			ifRetryDriven[loop.GuardNode] = true
			if err := e.executeIfRetryLoop(
				ctx,
				workflow,
				node,
				*loop,
				nodeResults,
				e.activeRunData(ctx),
				inputDataForNode,
			); err != nil {
				return &ExecutionResult{
					Data:  nil,
					Error: fmt.Errorf("error executing If-retry loop %s: %w", node.Name, err),
				}, nil
			}
			continue
		}

		// Skip nodes that received no items. This matches n8n's
		// execution semantics: a node is only invoked when its
		// upstream routed at least one item into it. Without this
		// guard, the engine executes every node in topological
		// order with a synthetic empty item, which causes
		// downstream-side-effect nodes (notably Respond-to-Webhook)
		// to fire with `$json` resolved to an empty object and
		// overwrite the *correct* response from the branch the item
		// actually took. For example, an IF node that routes a
		// valid item to "Loop Over Orders" and an invalid item to
		// "Respond Invalid Input" — when a valid item is processed,
		// the IF routes it to Loop, but m9m still invokes Respond
		// Invalid Input with an empty item, producing a stale
		// `{"error": "<undefined>", "status": "REJECTED"}` response
		// body that overrides the legitimate Respond Processed
		// Summary's body.
		//
		// The starting-node branch above already seeds non-empty
		// data, and triggers emit their own items, so legitimate
		// starts are unaffected. Decorative / trigger-only nodes
		// are skipped earlier in the loop.
		if len(inputDataForNode) == 0 {
			nodeResults[nodeName] = []model.DataItem{}
			continue
		}

		// Prepare node parameters with credentials if credential manager is available
		finalNodeParams := node.Parameters
		if e.credentialManager != nil {
			var err error
			finalNodeParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(node.ID, node.Parameters)
			if err != nil {
				return &ExecutionResult{
					Data:  nil,
					Error: fmt.Errorf("error injecting credentials for node %s: %w", node.Name, err),
				}, nil // Return as result.Error, not as function error
			}
		}

		// Validate the post-injection parameters so credential-only
		// nodes (e.g. an n8n MySQL node whose connection settings live
		// entirely on the synced credential envelope) don't trip the
		// "host is required" check at validate time. The original
		// `node.Parameters` validation ran before injection and
		// rejected every synced credential-only workflow.
		if err := executor.ValidateParameters(finalNodeParams); err != nil {
			return nil, fmt.Errorf("invalid parameters for node %s: %w", node.Name, err)
		}

		// Execute the node, passing the input data and node parameters.
		// Wrap the call in a node.execute span so the trace tree mirrors
		// the workflow DAG. Custom attributes from model.Node and from
		// the (optional) MetadataAwareNodeExecutor interface are attached
		// before EndNodeSpan so the failure-path recording picks them up.
		nodeCtx, nodeSpan := e.otelManager.StartNodeSpan(ctx, otel.NodeAttrs{
			NodeID:          node.ID,
			NodeName:        node.Name,
			NodeType:        node.Type,
			NodeTypeVersion: node.TypeVersion,
			ItemsInput:      len(inputDataForNode),
			Custom:          mergeNodeCustomAttrs(node),
		})
		// Always attach programmatic metadata on the span right after
		// the node finishes — SetAttributes is cheaper than a second
		// SetAttributes call.
		outputData, err := e.executeNodeWithContext(nodeCtx, executor, inputDataForNode, finalNodeParams)
		// Re-attach input/output counts now that we know the output.
		if nodeSpan != nil && nodeSpan.SpanContext().IsValid() {
			nodeSpan.SetAttributes(
				otel.AttrNodeItemsOutput.Int(len(outputData)),
			)
			// Programmatic metadata from the executor (n8n's setMetadata
			// analogue). Programmatic values override the static config
			// attributes to match the documented n8n behaviour.
			if md, ok := executor.(base.MetadataAwareNodeExecutor); ok {
				if progMD := md.SpanMetadata(); len(progMD) > 0 {
					merged := mergeProgrammaticNodeAttrs(node.CustomSpanAttributes, progMD)
					for k, v := range merged {
						nodeSpan.SetAttributes(attributeFromValue("m9m.node.custom."+k, v))
					}
				}
			}
		}
		otel.EndNodeSpan(nodeSpan, err)
		if err != nil {
			return &ExecutionResult{
				Data:  nil,
				Error: fmt.Errorf("error executing node %s: %w", node.Name, err),
			}, nil // Return as result.Error, not as function error
		}

		// Store output data for this node. Strip internal routing
		// metadata (e.g. IF's `_ifResult`) so it never leaks into
		// downstream node inputs or webhook responses.
		nodeResults[nodeName] = stripRoutingMetadata(outputData)

		// Also store in runExecutionData so RunAwareNodeExecutor
		// implementations (Code node, primarily) can pull sibling
		// node outputs via `$("OtherNode").item.json`. The slice
		// is keyed by runIndex so multiple iterations of a Loop
		// node each get their own slot. Prefer the ctx execFrame
		// so concurrent runs stay isolated.
		if runData := e.activeRunData(ctx); runData != nil && runData.ResultData != nil {
			runIndex := e.runIndex
			if f := execFrameFrom(ctx); f != nil {
				runIndex = f.runIndex
			}
			results := runData.ResultData.NodeData[nodeName]
			for len(results) <= runIndex {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[runIndex] = expressions.NodeExecutionResult{
				Data:      stripRoutingMetadata(outputData),
				StartTime: time.Now(),
			}
			runData.ResultData.NodeData[nodeName] = results
		}

		// Route data to connected nodes
		routedData, err := e.connectionRouter.RouteData(nodeName, workflow, outputData)
		if err != nil {
			return nil, fmt.Errorf("error routing data from node %s: %w", node.Name, err)
		}

		// Add routed data to node results, also bucketing by the
		// target's input index so Merge chooseBranch can pick a
		// single input the way n8n does.
		for targetNode, data := range routedData {
			if nodeResults[targetNode] == nil {
				nodeResults[targetNode] = data
			} else {
				// Append data if node already has data
				nodeResults[targetNode] = append(nodeResults[targetNode], data...)
			}
			inIdx := incomingInputIndex(workflow, nodeName, targetNode)
			if nodeInputResults[targetNode] == nil {
				nodeInputResults[targetNode] = make(map[int][]model.DataItem)
			}
			nodeInputResults[targetNode][inIdx] = append(nodeInputResults[targetNode][inIdx], data...)
		}

		// Record which connections actually carried items in this
		// routing step. We walk workflow.Connections[source] instead
		// of using routedData alone because the latter is collapsed
		// to (target → items) — the branch (outputIndex) and
		// inputIndex are lost. For non-routing nodes (the common
		// case), every connection in the source's connections map
		// gets the full output, so we mark every edge as taken.
		//
		// For routing nodes (IF, Switch), partitionByRoutingMetadata
		// inside the router partitioned items per branch. A target
		// in branch k that received >0 items means that specific
		// edge was taken; other edges in the same source are not.
		// We approximate this by re-running the partition logic
		// against the raw outputData: branchable nodes tag items
		// with _ifResult / _switchRuleIndex, non-routing nodes
		// don't, in which case every edge is marked taken.
		recordEdgesTaken(e.edgesTaken, node, outputData, workflow)
	}

	// Return the result from the last node in execution order that
	// actually produced data. For workflows with branching (e.g. IF
	// nodes), the last node in topological order may not have received
	// any items if all items routed to a sibling branch; in that case
	// fall back to the previous node in execution order that did
	// produce output. This mirrors n8n's `responseMode: lastNode`
	// semantics, which key off the last executed node, not the last
	// node in the topological sort.
	var finalResult []model.DataItem
	for i := len(executionOrder) - 1; i >= 0; i-- {
		nodeName := executionOrder[i]
		if data, ok := nodeResults[nodeName]; ok && len(data) > 0 {
			finalResult = data
			break
		}
	}

	// Merge this run's EdgesTaken back into the parent's
	// accumulator (if any) and restore the field so the rest of
	// the parent's execution loop keeps accumulating against the
	// combined map. For the OUTERMOST call `parentEdgesTaken` is
	// nil — that's the "top-level run" case, and we just keep
	// this run's map as-is. See the parentEdgesTaken capture at
	// the top of this function for the full rationale.
	if parentEdgesTaken != nil {
		for k, v := range e.edgesTaken {
			parentEdgesTaken[k] = v
		}
		e.edgesTaken = parentEdgesTaken
	}

	return &ExecutionResult{
		Data: finalResult,
		// Publish the per-node output map so external callers (the
		// webhook manager in particular) can implement
		// `responseMode: responseNode` by reading the Respond-to-
		// Webhook node's output directly. Without this the manager
		// would only have access to `finalResult`, which is the
		// last-node output and is the wrong shape for responseNode
		// workflows.
		NodeOutputs: nodeResults,
		// Publish the EdgesTaken accumulator so the API and
		// webhook manager can persist a per-edge "did this branch
		// carry items?" signal back to the execution record.
		// Without this the UI can colour edges green/grey only when
		// Debug=ON (because the per-node snapshots let it infer the
		// path); with this set the same colouring works under
		// Debug=OFF, satisfying the "different colour per edge"
		// requirement without bloating the DB. After the merge
		// above, `e.edgesTaken` contains both this run's edges and
		// (for nested sub-workflows) every inner run's edges, so
		// the persisted value reflects the full execution tree.
		EdgesTaken: e.edgesTaken,
	}, nil
}

func (e *workflowEngineImpl) executeNodeWithContext(
	ctx context.Context,
	executor base.NodeExecutor,
	inputData []model.DataItem,
	nodeParams map[string]interface{},
) ([]model.DataItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Prefer the per-run execFrame on ctx so concurrent webhook +
	// Kafka executions cannot clobber each other's runExecutionData.
	wf := e.workflow
	runData := e.runExecutionData
	runIndex := e.runIndex
	itemIndex := e.itemIndex
	if f := execFrameFrom(ctx); f != nil {
		wf = f.workflow
		runData = f.runExecutionData
		runIndex = f.runIndex
		itemIndex = f.itemIndex
	}

	// Prefer the run-aware executor when available — Code node
	// snippets use `$("OtherNode").item.json` to pull sibling node
	// outputs, which only resolves when the executor gets the live
	// runExecutionData. Without this, the Code node builds its own
	// context internally and silently swallows fields under spread
	// operators (`...$(Loop).item.json`).
	if runExecutor, ok := executor.(base.RunAwareNodeExecutor); ok {
		return runExecutor.ExecuteWithRun(wf, runData, runIndex, itemIndex, inputData, nodeParams)
	}

	if contextExecutor, ok := executor.(base.ContextAwareNodeExecutor); ok {
		return contextExecutor.ExecuteWithContext(ctx, inputData, nodeParams)
	}

	type nodeExecutionResult struct {
		output []model.DataItem
		err    error
	}
	resultChan := make(chan nodeExecutionResult, 1)
	go func() {
		outputData, err := executor.Execute(inputData, nodeParams)
		resultChan <- nodeExecutionResult{output: outputData, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result.output, result.err
	}
}

// executeSplitInBatchesLoop drives the iteration for a single
// splitInBatches (Loop) node. The contract is:
//
//  1. `inputData` (the items that routed into this node) is split
//     into batches of `batchSize` (default 10).
//  2. For each batch, the per-iteration body chain (the nodes
//     reachable from `main[1]`, NOT including the back-edge that
//     targets this splitInBatches node) executes in topological
//     order with the batch as its input. Each batch gets its own
//     `runIndex` slot so `$("Process Item").all()` inside the body
//     resolves correctly during iteration.
//  3. The "anchor" output for each batch (the deepest body node's
//     output after routing) is appended to an accumulator.
//  4. After every batch has been processed, the once-per-loop done
//     branch (nodes reachable from `main[0]`) executes exactly once
//     with the accumulator as its input. This matches n8n's
//     invariant: "fire Done / Output Final once, after every batch
//     has run".
//  5. The aggregator's items are written back to `nodeResults`
//     under the splitInBatches node name so downstream readers
//     (e.g. Respond to Webhook) see the same shape as n8n.
//
// The splitInBatches node itself does NOT need an executor call -
// the engine handles iteration entirely, so this method never
// invokes `splitInBatchesNode.Execute`. (That node is now a
// pass-through so other engine paths can still call it safely.)
func (e *workflowEngineImpl) executeSplitInBatchesLoop(
	ctx context.Context,
	workflow *model.Workflow,
	node *model.Node,
	nodeResults map[string][]model.DataItem,
	runData *expressions.RunExecutionData,
	inputData []model.DataItem,
	bodyOrder []string,
	doneOrder []string,
) error {
	if len(inputData) == 0 {
		// No items to iterate. Mark this node as having
		// produced no output so downstream "Skip if empty"
		// semantics apply. We do NOT execute the done branch
		// when there were no batches because n8n doesn't fire
		// "Done" when the upstream had nothing to split.
		nodeResults[node.Name] = []model.DataItem{}
		return nil
	}

	// Read batchSize (default 10) from the node's parameters.
	batchSize := 10
	if node.Parameters != nil {
		if v, ok := node.Parameters["batchSize"]; ok {
			switch n := v.(type) {
			case int:
				if n > 0 {
					batchSize = n
				}
			case float64:
				if n > 0 {
					batchSize = int(n)
				}
			}
		}
	}

	// Build the list of batches. Each batch is a sub-slice of the
	// original input items so per-iteration state (JSON, Binary,
	// PairedItem) flows through unchanged.
	batches := splitIntoBatches(inputData, batchSize)

	// For every batch, run the body chain and capture the body's
	// final output. The aggregated output across all batches is
	// what the done branch will see.
	aggregated := make([]model.DataItem, 0, len(inputData))
	for batchIdx, batch := range batches {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Use a per-iteration runIndex slot. The top-level
		// `e.runIndex` is preserved (so the rest of the
		// workflow still indexes off it), but every batch
		// pushes one slot into each body node's result so
		// `$("Process Item").all()` returns the running
		// accumulator instead of just the most recent batch.
		iterRunIndex := e.runIndex + 1 + batchIdx

		// Publish the *current* batch at the splitInBatches
		// node's iteration slot before the body chain runs.
		// Body-chain nodes that read `$("Loop Over Items").item.json`
		// (e.g. Code snippets in positive/8's `Merge Inventory
		// & Price` node) need this lookup to return the items
		// the loop is currently processing, not the aggregated
		// output across all batches (which is published later
		// at the top-level slot). Without this publish the
		// Code node sees `undefined.item` and crashes.
		if runData != nil && runData.ResultData != nil {
			results := runData.ResultData.NodeData[node.Name]
			for len(results) <= iterRunIndex {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[iterRunIndex] = expressions.NodeExecutionResult{
				Data:      batch,
				StartTime: time.Now(),
			}
			runData.ResultData.NodeData[node.Name] = results
		}

		batchOutput, err := e.runLoopBodyChain(
			ctx,
			workflow,
			node,
			nodeResults,
			runData,
			bodyOrder,
			batch,
			iterRunIndex,
		)
		if err != nil {
			return fmt.Errorf("batch %d: %w", batchIdx, err)
		}
		aggregated = append(aggregated, batchOutput...)
	}

	// Publish the per-iteration results so the done branch (and
	// any node that resolves `$("Loop Over Items").all()`) can
	// see the aggregated state. The splitInBatches node itself
	// is also tagged here - its "output" is the aggregated list
	// - so the `last-node` resolution downstream picks up the
	// right payload.
	nodeResults[node.Name] = aggregated
	if runData != nil && runData.ResultData != nil {
		results := runData.ResultData.NodeData[node.Name]
		for len(results) <= e.runIndex {
			results = append(results, expressions.NodeExecutionResult{})
		}
		results[e.runIndex] = expressions.NodeExecutionResult{
			Data:      aggregated,
			StartTime: time.Now(),
		}
		runData.ResultData.NodeData[node.Name] = results
	}

	// Now run the once-per-loop done branch with the aggregated
	// items as input. The done branch receives the full set of
	// processed items (not just the last batch's), which is the
	// n8n invariant: "Done / Output Final fires after every
	// iteration, with the accumulator visible to it".
	if len(doneOrder) == 0 {
		return nil
	}
	if _, err := e.runLoopDoneChain(
		ctx,
		workflow,
		node,
		nodeResults,
		runData,
		doneOrder,
		aggregated,
	); err != nil {
		return err
	}
	return nil
}

// runLoopBodyChain executes the per-iteration body chain
// (splitInBatches's main[1] descendants) for a single batch and
// returns the chain's terminal output. The terminal node is the
// last node in `bodyOrder` (the deepest body node); its output is
// what the engine aggregates across batches.
//
// The body chain is intentionally a straight-line sequence in
// execution order - n8n's Loop body does not branch. If a
// downstream node *does* branch (e.g. an IF inside the body),
// each branch's output is preserved in `nodeResults` so the next
// iteration (or the done branch) can read it via
// `$("Node").all()`. The return value of this helper is just the
// terminal node's output, used by the loop driver to build the
// accumulator.
func (e *workflowEngineImpl) runLoopBodyChain(
	ctx context.Context,
	workflow *model.Workflow,
	loopNode *model.Node,
	nodeResults map[string][]model.DataItem,
	runData *expressions.RunExecutionData,
	bodyOrder []string,
	batch []model.DataItem,
	iterRunIndex int,
) ([]model.DataItem, error) {
	if len(bodyOrder) == 0 {
		// No body chain - the loop had a splitInBatches node
		// with nothing connected to main[1]. Pass the batch
		// through unchanged so the aggregator at least sees
		// the input items.
		return batch, nil
	}

	// Seed the first body node's input with the batch.
	currentInput := batch
	currentInputFor := bodyOrder[0]
	nodeResults[currentInputFor] = currentInput

	// We re-use the top-level engine executor / credential
	// injection helpers. Build a temporary "nodeMap" view that
	// resolves workflow nodes by name without rebuilding it on
	// every iteration (the caller already built it).
	nodeMap := make(map[string]*model.Node, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nodeMap[workflow.Nodes[i].Name] = &workflow.Nodes[i]
	}

	// Execute each body node in order, routing its output to
	// downstream body nodes as we go. The routing step uses the
	// SAME connection router as the top-level engine so
	// per-item routing metadata (IF's `_ifResult`, Switch's
	// `_switchRuleIndex`) is honoured.
	for _, name := range bodyOrder {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bodyNode := nodeMap[name]
		if bodyNode == nil {
			return nil, fmt.Errorf("body node %q not found in workflow", name)
		}

		// Skip decorative / trigger-only nodes inside the body
		// chain (defensive - the connection walker already
		// drops them, but a future change could re-add them).
		if isDecorativeNodeType(bodyNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}
		if isTriggerOnlyNodeType(bodyNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}

		executor, err := e.GetNodeExecutor(bodyNode.Type)
		if err != nil {
			return nil, fmt.Errorf("body node %q: %w", name, err)
		}

		// Resolve credentials (same as top-level path) BEFORE
		// validation so credential-only nodes don't fail the
		// pre-execution check.
		finalParams := bodyNode.Parameters
		if e.credentialManager != nil {
			finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(bodyNode.ID, bodyNode.Parameters)
			if err != nil {
				return nil, fmt.Errorf("body node %q credential injection: %w", name, err)
			}
		}
		if err := executor.ValidateParameters(finalParams); err != nil {
			return nil, fmt.Errorf("body node %q invalid parameters: %w", name, err)
		}

		// Skip body nodes that received no items (matches the
		// top-level skip-empty-input guard).
		input := nodeResults[name]
		if len(input) == 0 {
			nodeResults[name] = []model.DataItem{}
			continue
		}

		// Run the body node with the iteration's runIndex so
		// sibling-node lookups (e.g. `$("Other Body Node")`)
		// resolve against the same iteration slot.
		prevRunIndex := e.runIndex
		if f := execFrameFrom(ctx); f != nil {
			prevRunIndex = f.runIndex
		}
		e.setActiveRunIndex(ctx, iterRunIndex)
		nodeCtx, nodeSpan := e.otelManager.StartNodeSpan(ctx, otel.NodeAttrs{
			NodeID:          bodyNode.ID,
			NodeName:        bodyNode.Name,
			NodeType:        bodyNode.Type,
			NodeTypeVersion: bodyNode.TypeVersion,
			ItemsInput:      len(input),
			Custom:          mergeNodeCustomAttrs(bodyNode),
		})
		outputData, err := e.executeNodeWithContext(nodeCtx, executor, input, finalParams)
		if nodeSpan != nil && nodeSpan.SpanContext().IsValid() {
			nodeSpan.SetAttributes(otel.AttrNodeItemsOutput.Int(len(outputData)))
		}
		otel.EndNodeSpan(nodeSpan, err)
		e.setActiveRunIndex(ctx, prevRunIndex)
		if err != nil {
			return nil, fmt.Errorf("body node %q: %w", name, err)
		}

		// Store the cleaned output. Strip routing metadata so
		// back-edge iteration does not propagate `_ifResult`
		// etc. into the next batch.
		cleaned := stripRoutingMetadata(outputData)
		nodeResults[name] = cleaned

		// Publish to runData at this iteration's slot.
		if runData != nil && runData.ResultData != nil {
			results := runData.ResultData.NodeData[name]
			for len(results) <= iterRunIndex {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[iterRunIndex] = expressions.NodeExecutionResult{
				Data:      cleaned,
				StartTime: time.Now(),
			}
			runData.ResultData.NodeData[name] = results
		}

		// Route the output to downstream body nodes (and to
		// the splitInBatches back-edge, which we ignore here).
		// The router already filters out back-edges to
		// splitInBatches when the workflow uses the loop
		// pattern.
		routed, err := e.connectionRouter.RouteData(name, workflow, outputData)
		if err != nil {
			return nil, fmt.Errorf("body node %q routing: %w", name, err)
		}
		for target, data := range routed {
			// Don't let body-chain output leak back into the
			// splitInBatches node's input (the loop driver
			// controls that flow). The connection router
			// already drops edges that target a
			// splitInBatches node, but we double-check
			// defensively.
			if target == loopNode.Name {
				continue
			}
			if nodeResults[target] == nil {
				nodeResults[target] = data
			} else {
				nodeResults[target] = append(nodeResults[target], data...)
			}
		}
	}

	// The terminal output is the deepest body node's last
	// result. We use the last node in bodyOrder as the
	// terminal, which matches the connection router's
	// "deepest process branch" anchor.
	terminal := bodyOrder[len(bodyOrder)-1]
	return nodeResults[terminal], nil
}

// executeIfRetryLoop drives the iteration for an
// `attempt < maxAttempts` retry loop. The guard is an `If` node
// whose main[0] (true) branch is a back-edge to the body entry;
// the main[1] (false) branch is the loop's "give up" exit (e.g.
// "Respond Failed" in the Finnet workflow).
//
// Algorithm:
//
//  1. Run the body chain from the back-edge target up to (but
//     not including) the guard. The body nodes' outputs feed
//     back into the body's entry on each iteration via
//     `nodeResults[backEdgeTarget]`.
//  2. After each iteration, evaluate the guard's condition
//     against the body's terminal output. If false (i.e. the
//     condition no longer holds — e.g. a response document was
//     found in Mongo) we stop iterating.
//  3. Cap iterations at `loop.MaxIterations` (default 10) to
//     prevent runaway loops.
//
// The body is executed with a per-iteration `runIndex` slot so
// expressions like `$("Set Counter").all()` reflect the
// running iteration count.
//
// The guard's main[0] (true) target is treated as the "re-loop"
// edge; the engine deliberately does NOT route the body's
// terminal output back into `nodeResults[backEdgeTarget]`
// between iterations via the connection router (which would
// also re-route to the false-branch targets). Instead the
// driver re-seeds the body entry directly so the next
// iteration starts with the same input shape.
func (e *workflowEngineImpl) executeIfRetryLoop(
	ctx context.Context,
	workflow *model.Workflow,
	guardNode *model.Node,
	loop connections.IfRetryLoop,
	nodeResults map[string][]model.DataItem,
	runData *expressions.RunExecutionData,
	inputData []model.DataItem,
) error {
	if loop.MaxIterations <= 0 {
		loop.MaxIterations = 10
	}

	// Build a node lookup so we can resolve each body node
	// quickly.
	nodeMap := make(map[string]*model.Node, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nodeMap[workflow.Nodes[i].Name] = &workflow.Nodes[i]
	}

	// Seed the body entry with the input data the first time
	// around. On subsequent iterations, the body's own output
	// is what we want to feed back in; we re-publish it to
	// `nodeResults[backEdgeTarget]` below. Prefer an already-
	// seeded BackEdgeTarget (Set Counter → Find Respond) when
	// the guard itself has no direct input.
	if len(nodeResults[loop.BackEdgeTarget]) == 0 {
		if len(inputData) == 0 {
			nodeResults[guardNode.Name] = []model.DataItem{}
			return nil
		}
		nodeResults[loop.BackEdgeTarget] = inputData
	}

	// The body chain, in execution order. We use the
	// pre-computed order from the detector (canvas position
	// order, which is stable and matches the natural left-to-
	// right reading order of n8n's UI).
	bodyOrder := loop.BodyNodes

	for iter := 0; iter < loop.MaxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Per-iteration runIndex slot. The first iteration
		// reuses the top-level slot; subsequent iterations
		// push one slot so `$("Set Counter").all()` reflects
		// the running history.
		iterRunIndex := e.runIndex + 1 + iter

		// Run the body chain.
		bodyOutput, err := e.runIfRetryBodyChain(
			ctx,
			workflow,
			guardNode,
			loop,
			nodeResults,
			runData,
			bodyOrder,
			nodeMap,
			iterRunIndex,
		)
		if err != nil {
			return fmt.Errorf("iteration %d: %w", iter, err)
		}

		// Evaluate the guard's condition. If it returns
		// false (i.e. the loop should stop), break.
		shouldContinue, err := e.evaluateIfRetryGuard(ctx, workflow, guardNode, bodyOutput)
		if err != nil {
			return fmt.Errorf("iteration %d guard evaluation: %w", iter, err)
		}
		if !shouldContinue {
			nodeResults[guardNode.Name] = bodyOutput
			return nil
		}

		// Loop again: seed the body entry with the
		// iteration's terminal output so the next
		// iteration's first node sees the right input.
		//
		// IMPORTANT: when Find Respond misses, the body
		// returns no items. Re-seeding that empty slice
		// into BackEdgeTarget makes the next iteration
		// skip Find entirely (empty-input guard), so the
		// poll never observes the Kafka/mongo write.
		// Keep the previous seed (Set Counter / prior
		// Find input) across empty misses.
		// Sleep briefly between polls so the Kafka consumer
		// path can write the mongo reply (Finnet sticky:
		// "Counter Max 10 retry" over ~20s wall clock).
		if iter < loop.MaxIterations-1 {
			if len(bodyOutput) > 0 {
				nodeResults[loop.BackEdgeTarget] = bodyOutput
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}

	// Exhausted all iterations without the guard going
	// false. Return the last body output and let the engine
	// continue (the false-branch downstream — e.g. "Respond
	// Failed" — will not fire because we never published
	// anything for it; the engine's existing per-node
	// "received no items → skip" guard handles this).
	if nodeResults[loop.BackEdgeTarget] != nil {
		nodeResults[guardNode.Name] = nodeResults[loop.BackEdgeTarget]
	} else {
		nodeResults[guardNode.Name] = inputData
	}
	return nil
}

// runIfRetryBodyChain executes the body of an If-retry loop for
// one iteration. The body is `loop.BodyNodes` in canvas order;
// each node's output is routed via the connection router. The
// loop's back-edge (the guard's main[0] target) is filtered
// out — the driver re-seeds the body entry between iterations
// rather than letting the connection router auto-loop.
func (e *workflowEngineImpl) runIfRetryBodyChain(
	ctx context.Context,
	workflow *model.Workflow,
	guardNode *model.Node,
	loop connections.IfRetryLoop,
	nodeResults map[string][]model.DataItem,
	runData *expressions.RunExecutionData,
	bodyOrder []string,
	nodeMap map[string]*model.Node,
	runIndexSlot int,
) ([]model.DataItem, error) {
	if len(bodyOrder) == 0 {
		return nil, nil
	}

	var lastTaggedOutput []model.DataItem
	for _, name := range bodyOrder {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bodyNode := nodeMap[name]
		if bodyNode == nil {
			return nil, fmt.Errorf("body node %q not found in workflow", name)
		}

		// Publish the *current* iteration input at this
		// node's per-iteration slot before the node runs.
		// This is what `$("Find Respond").item.json` should
		// see during the body — the live items the loop is
		// currently processing, not the cumulative history.
		inputSnapshot := nodeResults[name]
		if runData != nil && runData.ResultData != nil {
			results := runData.ResultData.NodeData[name]
			for len(results) <= runIndexSlot {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[runIndexSlot] = expressions.NodeExecutionResult{
				Data:      inputSnapshot,
				StartTime: time.Now(),
			}
			runData.ResultData.NodeData[name] = results
		}

		// Skip decorative / trigger-only nodes.
		if isDecorativeNodeType(bodyNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}
		if isTriggerOnlyNodeType(bodyNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}

		// Look up the executor.
		executor, err := e.GetNodeExecutor(bodyNode.Type)
		if err != nil {
			return nil, fmt.Errorf("body node %q: %w", name, err)
		}

		// Inject credentials.
		finalParams := bodyNode.Parameters
		if e.credentialManager != nil {
			finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(bodyNode.ID, bodyNode.Parameters)
			if err != nil {
				return nil, fmt.Errorf("body node %q credential injection: %w", name, err)
			}
		}
		if err := executor.ValidateParameters(finalParams); err != nil {
			return nil, fmt.Errorf("body node %q validation: %w", name, err)
		}

		// Execute the node via the context-aware path so
		// expressions like $('Async Subworkflow').item.json.key
		// and $('Set Counter').item.json.attempt resolve against
		// the live runExecutionData (plain Execute cannot).
		if len(inputSnapshot) == 0 && !isTriggerOnlyNodeType(bodyNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}
		prevRunIndex := e.runIndex
		if f := execFrameFrom(ctx); f != nil {
			prevRunIndex = f.runIndex
		}
		e.setActiveRunIndex(ctx, runIndexSlot)
		outputData, err := e.executeNodeWithContext(ctx, executor, inputSnapshot, finalParams)
		e.setActiveRunIndex(ctx, prevRunIndex)
		if err != nil {
			return nil, fmt.Errorf("body node %q execution: %w", name, err)
		}

		// Store a cleaned copy for downstream body inputs / final
		// results, but RouteData MUST see the tagged output so IF
		// nodes (Check Key Exists1) can fan out to IsRespondJson vs
		// the guard. Stripping first left IsRespondJson empty and
		// the parent Respond to Webhook got the raw mongo doc.
		//
		// CRITICAL (Finnet post_balance): the BackEdgeTarget is
		// Find Respond. When Find misses it returns []. Writing that
		// empty slice into nodeResults[Find Respond] wipes the Set
		// Counter seed. The post-iteration reseed only runs when
		// bodyOutput is non-empty, so the next iteration sees
		// len(input)==0 and SKIPS Find entirely — the poll sleeps
		// for ~20s never querying again while Atlas already has the
		// doc. Keep the seed whenever the body entry itself misses.
		cleaned := stripRoutingMetadata(outputData)
		if name == loop.BackEdgeTarget && len(cleaned) == 0 && len(inputSnapshot) > 0 {
			nodeResults[name] = inputSnapshot
		} else {
			nodeResults[name] = cleaned
		}
		lastTaggedOutput = outputData

		// Route the output to all downstream nodes, except
		// the back-edge target (the body entry). The
		// driver re-seeds the body entry explicitly between
		// iterations, so the connection router should not
		// also auto-loop.
		routed, err := e.connectionRouter.RouteData(name, workflow, outputData)
		if err != nil {
			return nil, fmt.Errorf("body node %q routing: %w", name, err)
		}
		for target, data := range routed {
			if target == loop.BackEdgeTarget {
				// Suppress the back-edge: the driver
				// controls body re-entry. Without this,
				// the connection router would re-seed
				// the body entry and the driver would
				// see the loop as having completed
				// prematurely.
				continue
			}
			if nodeResults[target] == nil {
				nodeResults[target] = data
			} else {
				nodeResults[target] = append(nodeResults[target], data...)
			}
		}
	}

	// Guard evaluation needs IF routing tags on the terminal body
	// output so filterIfRetryGuardItems can distinguish exit vs retry.
	if lastTaggedOutput != nil {
		return lastTaggedOutput, nil
	}
	return nodeResults[bodyOrder[len(bodyOrder)-1]], nil
}

// isInBody reports whether `target` is a node in the body chain.
// Retained for potential future use (the body chain driver
// currently allows routing to non-body nodes so the main engine
// can pick up items routed out of the body, e.g. into the
// "Respond to Webhook" exit branch).
func isInBody(bodyOrder []string, target string) bool {
	for _, n := range bodyOrder {
		if n == target {
			return true
		}
	}
	return false
}

// evaluateIfRetryGuard evaluates the guard's condition against
// the items produced by the most recent body iteration. Returns
// true when the loop should continue (the guard's true branch
// would be taken).
//
// The body's terminal node is typically an `If` node itself
// (e.g. `Check Key Exists1` in the Finnet workflow) which tags
// items with `_ifResult`. We pre-filter to only the items that
// would be routed to the guard (i.e. `_ifResult=false` — the
// items the body explicitly routed into the loop's back-edge),
// so the guard's `attempt < maxAttempts` check is run against
// the right set of items.
func (e *workflowEngineImpl) evaluateIfRetryGuard(
	ctx context.Context,
	workflow *model.Workflow,
	guardNode *model.Node,
	bodyOutput []model.DataItem,
) (bool, error) {
	if guardNode.Type != "n8n-nodes-base.if" {
		return false, fmt.Errorf("guard %q is not an If node", guardNode.Name)
	}

	// Filter to items the body routed to the guard (those
	// that didn't take the body's exit branch). The body's
	// terminal node tags every item with `_ifResult`; the
	// items with `_ifResult=false` are the ones routed to
	// the guard.
	// Empty body output means Find Respond / upstream returned
	// no items (mongo miss). Keep polling — do NOT treat this
	// as "exit branch taken".
	if len(bodyOutput) == 0 {
		return true, nil
	}
	guardItems := filterIfRetryGuardItems(bodyOutput)
	if len(guardItems) == 0 {
		// The body found what it was looking for (routed
		// every item to the exit branch). Don't loop.
		return false, nil
	}

	// The IF node executor is registered under its n8n type.
	executor, err := e.GetNodeExecutor(guardNode.Type)
	if err != nil {
		return false, err
	}
	finalParams := guardNode.Parameters
	if e.credentialManager != nil {
		finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(guardNode.ID, guardNode.Parameters)
		if err != nil {
			return false, err
		}
	}
	if err := executor.ValidateParameters(finalParams); err != nil {
		return false, err
	}

	// Evaluate via the run-aware path so
	// `$('Set Counter').item.json.attempt` resolves against
	// the live runExecutionData. Plain Execute builds an
	// ExpressionContext without RunExecutionData, so the
	// Finnet guard always saw undefined/undefined → false
	// and aborted the poll after the first miss.
	output, err := e.executeNodeWithContext(ctx, executor, guardItems, finalParams)
	if err != nil {
		return false, err
	}
	for _, item := range output {
		if v, ok := item.JSON["_ifResult"].(bool); ok && v {
			return true, nil
		}
	}
	return false, nil
}

// filterIfRetryGuardItems returns the items from `items` that
// the body's terminal `If` node routed to the guard (i.e. the
// items with `_ifResult=false`). When no items carry an
// `_ifResult` tag (the body's terminal node wasn't an If node),
// we fall back to returning all items — the guard will then
// evaluate the body's full output.
func filterIfRetryGuardItems(items []model.DataItem) []model.DataItem {
	if len(items) == 0 {
		return nil
	}
	hasTags := false
	for _, item := range items {
		if _, ok := item.JSON["_ifResult"]; ok {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return items
	}
	var out []model.DataItem
	for _, item := range items {
		if v, ok := item.JSON["_ifResult"].(bool); ok && !v {
			out = append(out, item)
		}
	}
	return out
}

// runLoopDoneChain executes the once-per-loop done branch
// (splitInBatches's main[0] descendants) with `aggregated` as the
// input. Unlike the body chain, the done branch runs exactly once
// per loop (not per batch), and it always sees the *full*
// aggregated output across all batches.
//
// Internally it reuses the same per-node execution helpers as
// the body chain so expression evaluation, credential injection,
// and routing metadata all behave identically.
func (e *workflowEngineImpl) runLoopDoneChain(
	ctx context.Context,
	workflow *model.Workflow,
	loopNode *model.Node,
	nodeResults map[string][]model.DataItem,
	runData *expressions.RunExecutionData,
	doneOrder []string,
	aggregated []model.DataItem,
) ([]model.DataItem, error) {
	if len(doneOrder) == 0 {
		return nil, nil
	}

	nodeMap := make(map[string]*model.Node, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nodeMap[workflow.Nodes[i].Name] = &workflow.Nodes[i]
	}

	// Seed the first done-branch node's input with the
	// aggregated items.
	nodeResults[doneOrder[0]] = aggregated

	for _, name := range doneOrder {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doneNode := nodeMap[name]
		if doneNode == nil {
			return nil, fmt.Errorf("done node %q not found in workflow", name)
		}
		if isDecorativeNodeType(doneNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}
		if isTriggerOnlyNodeType(doneNode.Type) {
			nodeResults[name] = []model.DataItem{}
			continue
		}
		executor, err := e.GetNodeExecutor(doneNode.Type)
		if err != nil {
			return nil, fmt.Errorf("done node %q: %w", name, err)
		}

		finalParams := doneNode.Parameters
		if e.credentialManager != nil {
			finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(doneNode.ID, doneNode.Parameters)
			if err != nil {
				return nil, fmt.Errorf("done node %q credential injection: %w", name, err)
			}
		}
		if err := executor.ValidateParameters(finalParams); err != nil {
			return nil, fmt.Errorf("done node %q invalid parameters: %w", name, err)
		}

		input := nodeResults[name]
		if len(input) == 0 {
			nodeResults[name] = []model.DataItem{}
			continue
		}

		nodeCtx, nodeSpan := e.otelManager.StartNodeSpan(ctx, otel.NodeAttrs{
			NodeID:          doneNode.ID,
			NodeName:        doneNode.Name,
			NodeType:        doneNode.Type,
			NodeTypeVersion: doneNode.TypeVersion,
			ItemsInput:      len(input),
			Custom:          mergeNodeCustomAttrs(doneNode),
		})
		outputData, err := e.executeNodeWithContext(nodeCtx, executor, input, finalParams)
		if nodeSpan != nil && nodeSpan.SpanContext().IsValid() {
			nodeSpan.SetAttributes(otel.AttrNodeItemsOutput.Int(len(outputData)))
		}
		otel.EndNodeSpan(nodeSpan, err)
		if err != nil {
			return nil, fmt.Errorf("done node %q: %w", name, err)
		}

		cleaned := stripRoutingMetadata(outputData)
		nodeResults[name] = cleaned

		if runData != nil && runData.ResultData != nil {
			results := runData.ResultData.NodeData[name]
			for len(results) <= e.runIndex {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[e.runIndex] = expressions.NodeExecutionResult{
				Data:      cleaned,
				StartTime: time.Now(),
			}
			runData.ResultData.NodeData[name] = results
		}

		// Route the done-branch output to its downstream
		// (typically Respond to Webhook). We do NOT skip the
		// splitInBatches target here because done-branch
		// targets don't have a back-edge to the loop node.
		routed, err := e.connectionRouter.RouteData(name, workflow, outputData)
		if err != nil {
			return nil, fmt.Errorf("done node %q routing: %w", name, err)
		}
		for target, data := range routed {
			if target == loopNode.Name {
				continue
			}
			if nodeResults[target] == nil {
				nodeResults[target] = data
			} else {
				nodeResults[target] = append(nodeResults[target], data...)
			}
		}
	}

	// The terminal done-branch node is the last in doneOrder.
	terminal := doneOrder[len(doneOrder)-1]
	return nodeResults[terminal], nil
}

// splitIntoBatches splits `items` into batches of at most
// `batchSize` (>=1). Returns an empty slice when `items` is empty
// or `batchSize` is non-positive. Each batch is a fresh slice so
// downstream callers can mutate items without aliasing the
// upstream input.
func splitIntoBatches(items []model.DataItem, batchSize int) [][]model.DataItem {
	if len(items) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	numBatches := (len(items) + batchSize - 1) / batchSize
	batches := make([][]model.DataItem, 0, numBatches)
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := make([]model.DataItem, end-i)
		copy(batch, items[i:end])
		batches = append(batches, batch)
	}
	return batches
}

// ExecuteWorkflowParallel executes multiple workflows in parallel
func (e *workflowEngineImpl) ExecuteWorkflowParallel(workflows []*model.Workflow, inputData [][]model.DataItem) ([]*ExecutionResult, error) {
	if len(workflows) == 0 {
		return []*ExecutionResult{}, nil
	}

	if len(workflows) != len(inputData) {
		return nil, fmt.Errorf("number of workflows (%d) must match number of input data arrays (%d)", len(workflows), len(inputData))
}

	// Create channel for results
	results := make([]*ExecutionResult, len(workflows))

	// Create wait group to track completion
	var wg sync.WaitGroup
	wg.Add(len(workflows))

	// Execute each workflow in a separate goroutine with panic recovery
	for i, workflow := range workflows {
		go func(index int, wf *model.Workflow, data []model.DataItem) {
			defer wg.Done()

			// Recover from panics to prevent goroutine crashes from blocking WaitGroup
			defer func() {
				if r := recover(); r != nil {
					log.Printf("PANIC in workflow execution %s: %v\n%s", wf.Name, r, debug.Stack())
					results[index] = &ExecutionResult{
						Data:  nil,
						Error: fmt.Errorf("panic during workflow execution: %v", r),
					}
				}
			}()

			// Execute the workflow
			result, err := e.ExecuteWorkflow(wf, data)
			if err != nil {
				results[index] = &ExecutionResult{
					Data:  nil,
					Error: fmt.Errorf("error executing workflow %s: %w", wf.Name, err),
				}
			} else {
				results[index] = result
			}
		}(i, workflow, inputData[i])
	}

	// Wait for all workflows to complete
	wg.Wait()

	return results, nil
}

// findExecuteWorkflowTriggerStarts returns every node whose type is
// `n8n-nodes-base.executeWorkflowTrigger`. Used when a parent Execute
// Workflow node invokes this workflow as a child.
func (e *workflowEngineImpl) findExecuteWorkflowTriggerStarts(workflow *model.Workflow) []string {
	if workflow == nil {
		return nil
	}
	var starts []string
	for _, node := range workflow.Nodes {
		if node.Type == "n8n-nodes-base.executeWorkflowTrigger" {
			starts = append(starts, node.Name)
		}
	}
	return starts
}

// findWebhookStarts returns every webhook trigger node. Used when
// execution mode is "webhook" so sibling triggers (Kafka, EWT) are
// not also seeded with the HTTP payload.
func (e *workflowEngineImpl) findWebhookStarts(workflow *model.Workflow) []string {
	if workflow == nil {
		return nil
	}
	var starts []string
	for _, node := range workflow.Nodes {
		if node.Type == "n8n-nodes-base.webhook" {
			starts = append(starts, node.Name)
		}
	}
	return starts
}

// findStartingNodes finds nodes that have no incoming connections
func (e *workflowEngineImpl) findStartingNodes(workflow *model.Workflow) []string {
	if workflow == nil || workflow.Connections == nil {
		// If no connections, all nodes are starting nodes
		var allNodes []string
		for _, node := range workflow.Nodes {
			allNodes = append(allNodes, node.Name)
		}
		return allNodes
	}

	// Create a set of all node names
	allNodes := make(map[string]bool)
	for _, node := range workflow.Nodes {
		allNodes[node.Name] = true
	}

	// Find nodes that are targets of connections (have incoming connections)
	nodesWithIncomingConnections := make(map[string]bool)
	for _, connections := range workflow.Connections {
		// Check main connections
		for _, typeConnections := range connections.Main {
			// typeConnections is []Connection
			for _, connection := range typeConnections {
				// connection is a Connection struct
				nodesWithIncomingConnections[connection.Node] = true
			}
		}
	}

	// Starting nodes are those that are not targets of any connection
	var startingNodes []string
	for nodeName := range allNodes {
		if !nodesWithIncomingConnections[nodeName] {
			startingNodes = append(startingNodes, nodeName)
		}
	}

	return startingNodes
}

// isDecorativeNodeType reports whether a node type is decorative only
// and never participates in execution. n8n's UI exports include these
// nodes alongside real workflow nodes so the editor can render canvas
// annotations (sticky notes, comments, etc.) but they have no executor
// and no connections. They should be skipped during execution rather
// than aborting the whole workflow.
func isDecorativeNodeType(nodeType string) bool {
	switch nodeType {
	case "n8n-nodes-base.stickyNote",
		"n8n-nodes-base.note",
		"@n8n/n8n-nodes-langchain.note":
		return true
	}
	return false
}

// isTriggerOnlyNodeType reports whether a node type is a UI-side
// trigger that has no executable side (it just kicks off the workflow
// when a developer clicks "Test workflow"). When the workflow is run
// via a different real trigger (e.g. Webhook) these nodes should be
// silently skipped — otherwise the engine aborts with
// "no executor registered for node type".
func isTriggerOnlyNodeType(nodeType string) bool {
	switch nodeType {
	case "n8n-nodes-base.manualTrigger":
		return true
	}
	return false
}

// stripRoutingMetadata returns a copy of items with internal routing
// metadata removed (currently the IF node's `_ifResult`, the Switch
// node's `_switchRuleIndex`, and the splitInBatches node's
// `_loopDone` tags). The connection router consumes these to
// partition items across branches; once that has happened the tag
// must not leak into downstream node inputs or webhook responses.
//
// Items are shallow-copied only when they actually contain a routing
// field, so the no-op case (no IF/Switch/Loop involved) is
// allocation-free.
func stripRoutingMetadata(items []model.DataItem) []model.DataItem {
	clean := false
	for i := range items {
		if _, ok := items[i].JSON["_ifResult"]; ok {
			clean = true
			break
		}
		if _, ok := items[i].JSON["_switchRuleIndex"]; ok {
			clean = true
			break
		}
		if _, ok := items[i].JSON["_loopDone"]; ok {
			clean = true
			break
		}
	}
	if !clean {
		return items
	}
	out := make([]model.DataItem, len(items))
	for i := range items {
		out[i] = items[i]
		if out[i].JSON != nil {
			// Copy on first write to avoid mutating the source.
			newJSON := make(map[string]interface{}, len(out[i].JSON))
			for k, v := range out[i].JSON {
				if k == "_ifResult" || k == "_switchRuleIndex" || k == "_loopDone" {
					continue
				}
				newJSON[k] = v
			}
			out[i].JSON = newJSON
		}
	}
	return out
}

// routingKind returns a coarse classification of the source node's
// per-item routing metadata so the EdgesTaken accumulator can decide
// which branches were actually exercised.
//
// The detection mirrors the partitionByRoutingMetadata helper in
// internal/connections/router.go — we don't import that helper because
// it lives in a sibling package and the cost of re-implementing the
// check is one switch over known tags.
//
// Returned values:
//
//	"if"     — every emitted item carries `_ifResult`; the engine
//	           routes true items to main[0], false items to main[1].
//	"switch" — every emitted item carries `_switchRuleIndex`; the
//	           engine routes items with tag k to main[k], with
//	           unmatched items falling through to main[last].
//	""       — no routing tags found; the engine forwards every
//	           emitted item to every downstream target on every
//	           branch.
func routingKind(items []model.DataItem) string {
	for _, item := range items {
		if _, ok := item.JSON["_ifResult"]; ok {
			return "if"
		}
		if _, ok := item.JSON["_switchRuleIndex"]; ok {
			return "switch"
		}
	}
	return ""
}

// recordEdgesTaken marks every connection leaving `sourceNode` that
// carried at least one item from `outputData` into the accumulator
// `taken`. The key format matches the frontend's `buildEdgeId` minus
// the "workflow-edge:" prefix and URL-encoding — see ExecutionResult
// .EdgesTaken.
//
// For non-routing nodes (no `_ifResult` / `_switchRuleIndex` tags)
// every connection leaving the source is recorded as taken: that's
// the historic behaviour the engine already implements and matches
// n8n's "every branch forwards every item" model for plain pass-
// through nodes.
//
// For routing nodes, only the branches that received at least one
// matching item are recorded. The function uses the source node's
// own `Connections` map as the source of truth for which (outputIndex,
// targetNode, inputIndex) tuples exist — the router has already
// collapsed this into per-target slices in `routedData`, which we
// intentionally do NOT consult here because the per-branch breakdown
// is gone by then.
//
// This function is a no-op when the workflow has no connections
// leaving the source node (a terminal node, or a workflow with no
// edges at all).
func recordEdgesTaken(taken map[string]bool, sourceNode *model.Node, outputData []model.DataItem, workflow *model.Workflow) {
	if workflow == nil || sourceNode == nil || workflow.Connections == nil {
		return
	}
	conns, ok := workflow.Connections[sourceNode.Name]
	if !ok || conns.Main == nil {
		return
	}

	// Build the set of branches that received at least one item, so
	// we can decide per-edge whether to mark it taken. For
	// non-routing sources the set is "all branches" — every
	// emitted item was forwarded to every branch.
	branchWithItems := make(map[int]bool, len(conns.Main))
	kind := routingKind(outputData)
	switch kind {
	case "if":
		for _, item := range outputData {
			v, ok := item.JSON["_ifResult"]
			if !ok {
				continue
			}
			b, ok := v.(bool)
			if !ok {
				continue
			}
			// main[0] = true, main[1] = false.
			if b {
				branchWithItems[0] = true
			} else {
				branchWithItems[1] = true
			}
		}
	case "switch":
		lastBranch := len(conns.Main) - 1
		for _, item := range outputData {
			v, ok := item.JSON["_switchRuleIndex"]
			if !ok {
				continue
			}
			n, ok := toInt(v)
			if !ok {
				continue
			}
			// SwitchNode.Execute emits `_switchRuleIndex: len(rules)`
			// when no rule matched AND fallbackToLast is set —
			// that's an out-of-range index relative to
			// `connections.Main` (the number of downstream branches
			// is normally equal to the number of rules, but a
			// trimmed workflow with a missing rule still leaves the
			// tag at len(rules)). The router collapses this to
			// `main[lastBranch]` per the fallbackToLast contract;
			// mirror that here so the recorded edges match the
			// items the router actually delivered.
			if n >= len(conns.Main) {
				n = lastBranch
			}
			if n < 0 {
				continue
			}
			branchWithItems[n] = true
		}
		// NOTE: previously this branch also did
		// `branchWithItems[len(conns.Main)-1] = true` to mirror the
		// router's `fallbackToLast` behaviour. That was wrong:
		// fallback-to-last is already handled inside SwitchNode.Execute
		// — when no rule matches and `fallbackToLast` is set, the
		// node itself emits an item with `_switchRuleIndex: len(rules)`
		// (which lands in the `for _, item := range outputData` loop
		// above, gets clamped to `len(conns.Main)-1` by the
		// out-of-range check, and marks the last branch). Adding the
		// unconditional `true` here caused the UI to grey-out the
		// wrong path: for a 3-branch Switch that routed a matched
		// item to branch 0, edgesTaken still contained branch 2's
		// edge as `true`, which the frontend painted green even
		// though no items actually flowed down main[2]. Removed in
		// 4e18bf9 follow-up.
		//
		// If `fallbackToLast=false` AND no rule matched, the Switch
		// emits zero items — every branch should be absent from
		// `taken`, which matches what the router actually delivered.
	default:
		for i := range conns.Main {
			branchWithItems[i] = true
		}
	}

	// Resolve sourceID → nodeID for the key. node.ID is the
	// Vue Flow node ID assigned by the editor; the frontend's
	// buildEdgeId uses the same field, so matching is direct.
	// Resolve targetName → targetID by linear scan; for typical
	// workflows (< 100 nodes) this is far cheaper than maintaining
	// a per-run name index.
	nameToID := make(map[string]string, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nameToID[workflow.Nodes[i].Name] = workflow.Nodes[i].ID
	}

	sourceID := nameToID[sourceNode.Name]

	for outputIndex, outputs := range conns.Main {
		if !branchWithItems[outputIndex] {
			continue
		}
		for _, c := range outputs {
			targetID := nameToID[c.Node]
			if targetID == "" {
				// The target was renamed out from under us or
				// doesn't exist; skip rather than write a key
				// the UI can't resolve.
				continue
			}
			taken[edgeKey(sourceID, outputIndex, targetID, c.Index)] = true
		}
	}
}

// edgeKey builds the `<sourceID>:<outputIndex>:<targetID>:<inputIndex>`
// tuple the engine and frontend share. Centralised here so the
// accumulator (above) and any future consumer (e.g. a metrics
// publisher) speak the same dialect.
func edgeKey(sourceID string, outputIndex int, targetID string, inputIndex int) string {
	return fmt.Sprintf("%s:%d:%s:%d", sourceID, outputIndex, targetID, inputIndex)
}

// incomingInputIndex returns the target input index for the first
// connection from source→target in the workflow. Defaults to 0 when
// the edge is missing (legacy single-input nodes).
func incomingInputIndex(workflow *model.Workflow, source, target string) int {
	if workflow == nil || workflow.Connections == nil {
		return 0
	}
	conns, ok := workflow.Connections[source]
	if !ok {
		return 0
	}
	for _, branch := range conns.Main {
		for _, c := range branch {
			if c.Node == target {
				if c.Index < 0 {
					return 0
				}
				return c.Index
			}
		}
	}
	return 0
}

// chooseBranchInput returns the items from the Merge input selected
// by n8n's chooseBranch `useDataOfInput` (1-based, default 1) when
// the node is a chooseBranch Merge. ok is false for every other
// node type / mode so callers keep the flattened input slice.
func chooseBranchInput(node *model.Node, byInput map[int][]model.DataItem) ([]model.DataItem, bool) {
	if node == nil || node.Type != "n8n-nodes-base.merge" || byInput == nil {
		return nil, false
	}
	mode, _ := node.Parameters["mode"].(string)
	if mode != "chooseBranch" {
		return nil, false
	}
	use := 1
	if raw, ok := node.Parameters["useDataOfInput"]; ok {
		if n, ok := toInt(raw); ok && n >= 1 {
			use = n
		}
	}
	data, ok := byInput[use-1]
	if !ok || len(data) == 0 {
		// Finnet: input 0 is Payload (has key) and input 1 is the
		// ServiceRoutePlans branch. If the configured input is empty
		// but another input carries `key`, prefer that so Kafka /
		// Async Sub Workflow still get the mongo lookup key.
		for idx, items := range byInput {
			if idx == use-1 {
				continue
			}
			for _, it := range items {
				if k, ok := it.JSON["key"].(string); ok && k != "" {
					return items, true
				}
			}
		}
		if !ok {
			return nil, false
		}
	}
	return data, true
}

// toInt coerces arbitrary JSON-decoded numerics into an int. n8n's
// Switch node writes the rule index as a number; depending on the
// decode path it can arrive as int, int64, float64, or json.Number.
// Centralising the coercion avoids subtle off-by-one branch bugs in
// the EdgesTaken accumulator when one path emits `_switchRuleIndex`
// as float64(2) but another emits int64(2).
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	}
	return 0, false
}
