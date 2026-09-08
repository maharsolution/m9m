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

	// Per-execution context. Populated at the start of every
	// ExecuteWorkflow call and read by executeNodeWithContext so the
	// RunAwareNodeExecutor interface can hand the workflow + live
	// runExecutionData to nodes that need it (Code, primarily, for
	// `$("OtherNode").item.json` lookups).
	workflow        *model.Workflow
	runExecutionData *expressions.RunExecutionData
	runIndex         int
	itemIndex        int
}

// NewWorkflowEngine creates a new workflow engine
func NewWorkflowEngine() WorkflowEngine {
	return &workflowEngineImpl{
		nodeRegistry:     make(NodeRegistry),
		connectionRouter: connections.NewConnectionRouter(),
	}
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

	// Set run context on the engine so that RunAwareNodeExecutor
	// implementations (e.g. Code) can pull sibling node outputs from
	// the live runExecutionData. Both fields are cleared at the end
	// to keep the engine re-entrant across concurrent workflows.
	e.workflow = workflow
	e.runExecutionData = &expressions.RunExecutionData{
		ExecutionData: &expressions.ExecutionData{},
		ResultData: &expressions.RunData{
			NodeData: make(map[string][]expressions.NodeExecutionResult),
		},
	}
	defer func() {
		e.workflow = nil
		e.runExecutionData = nil
		e.runIndex = 0
		e.itemIndex = 0
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

	if hasCycles && !connections.WorkflowUsesSplitInBatches(workflow) {
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

	// Build node lookup map for O(1) access (instead of O(n) linear search)
	nodeMap := make(map[string]*model.Node, len(workflow.Nodes))
	for i := range workflow.Nodes {
		nodeMap[workflow.Nodes[i].Name] = &workflow.Nodes[i]
	}

	// Initialize with input data for starting nodes (nodes with no incoming connections)
	startingNodes := e.findStartingNodes(workflow)

	// Set input data for starting nodes
	for _, nodeName := range startingNodes {
		nodeResults[nodeName] = inputData
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
		// chain. These are executed by the engine's loop driver
		// when the splitInBatches node itself runs; the top-level
		// topological loop would otherwise execute them a second
		// time and double-apply their assignments.
		if nodesHandledByLoopDriver[nodeName] {
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
			if isTriggerOnlyNodeType(node.Type) {
				nodeResults[nodeName] = []model.DataItem{}
				continue
			}
			return nil, fmt.Errorf("failed to get executor for node %s: %w", node.Name, err)
		}

		// Validate parameters
		if err := executor.ValidateParameters(node.Parameters); err != nil {
			return nil, fmt.Errorf("invalid parameters for node %s: %w", node.Name, err)
		}

		// Get input data for this node
		inputDataForNode := nodeResults[nodeName]
		if inputDataForNode == nil {
			// No specific input data, use empty input
			inputDataForNode = []model.DataItem{{JSON: make(map[string]interface{})}}
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
				e.runExecutionData,
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

		// Execute the node, passing the input data and node parameters
		outputData, err := e.executeNodeWithContext(ctx, executor, inputDataForNode, finalNodeParams)
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
		// node each get their own slot.
		if e.runExecutionData != nil && e.runExecutionData.ResultData != nil {
			results := e.runExecutionData.ResultData.NodeData[nodeName]
			for len(results) <= e.runIndex {
				results = append(results, expressions.NodeExecutionResult{})
			}
			results[e.runIndex] = expressions.NodeExecutionResult{
				Data:      stripRoutingMetadata(outputData),
				StartTime: time.Now(),
			}
			e.runExecutionData.ResultData.NodeData[nodeName] = results
		}

		// Route data to connected nodes
		routedData, err := e.connectionRouter.RouteData(nodeName, workflow, outputData)
		if err != nil {
			return nil, fmt.Errorf("error routing data from node %s: %w", node.Name, err)
		}

		// Add routed data to node results
		for targetNode, data := range routedData {
			if nodeResults[targetNode] == nil {
				nodeResults[targetNode] = data
			} else {
				// Append data if node already has data
				nodeResults[targetNode] = append(nodeResults[targetNode], data...)
			}
		}
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

	// Prefer the run-aware executor when available — Code node
	// snippets use `$("OtherNode").item.json` to pull sibling node
	// outputs, which only resolves when the executor gets the live
	// runExecutionData. Without this, the Code node builds its own
	// context internally and silently swallows fields under spread
	// operators (`...$(Loop).item.json`).
	if runExecutor, ok := executor.(base.RunAwareNodeExecutor); ok {
		return runExecutor.ExecuteWithRun(e.workflow, e.runExecutionData, e.runIndex, e.itemIndex, inputData, nodeParams)
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
		if err := executor.ValidateParameters(bodyNode.Parameters); err != nil {
			return nil, fmt.Errorf("body node %q invalid parameters: %w", name, err)
		}

		// Resolve credentials (same as top-level path).
		finalParams := bodyNode.Parameters
		if e.credentialManager != nil {
			finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(bodyNode.ID, bodyNode.Parameters)
			if err != nil {
				return nil, fmt.Errorf("body node %q credential injection: %w", name, err)
			}
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
		e.runIndex = iterRunIndex
		outputData, err := e.executeNodeWithContext(ctx, executor, input, finalParams)
		e.runIndex = prevRunIndex
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
		if err := executor.ValidateParameters(doneNode.Parameters); err != nil {
			return nil, fmt.Errorf("done node %q invalid parameters: %w", name, err)
		}

		finalParams := doneNode.Parameters
		if e.credentialManager != nil {
			finalParams, err = e.credentialManager.InjectCredentialsIntoNodeParameters(doneNode.ID, doneNode.Parameters)
			if err != nil {
				return nil, fmt.Errorf("done node %q credential injection: %w", name, err)
			}
		}

		input := nodeResults[name]
		if len(input) == 0 {
			nodeResults[name] = []model.DataItem{}
			continue
		}

		outputData, err := e.executeNodeWithContext(ctx, executor, input, finalParams)
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
