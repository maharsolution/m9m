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

	"github.com/neul-labs/m9m/internal/connections"
	"github.com/neul-labs/m9m/internal/credentials"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// ExecutionResult represents the result of a workflow execution
type ExecutionResult struct {
	Data  []model.DataItem `json:"data"`
	Error error            `json:"error,omitempty"`
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

	// Check for cycles in workflow
	hasCycles, err := e.connectionRouter.HasCycles(workflow)
	if err != nil {
		return nil, fmt.Errorf("error checking for workflow cycles: %w", err)
	}

	if hasCycles {
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

// stripRoutingMetadata returns a copy of items with internal routing
// metadata removed (currently the IF node's `_ifResult` tag). The
// connection router consumes this metadata to partition items across
// branches; once that has happened the tag must not leak into
// downstream node inputs or webhook responses.
//
// Items are shallow-copied only when they actually contain a routing
// field, so the no-op case (no IF involved) is allocation-free.
func stripRoutingMetadata(items []model.DataItem) []model.DataItem {
	clean := false
	for i := range items {
		if _, ok := items[i].JSON["_ifResult"]; ok {
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
				if k == "_ifResult" {
					continue
				}
				newJSON[k] = v
			}
			out[i].JSON = newJSON
		}
	}
	return out
}
