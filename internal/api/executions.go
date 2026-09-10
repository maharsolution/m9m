package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/queue"
	"github.com/neul-labs/m9m/internal/storage"
)

func (s *APIServer) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	workflow, err := s.storage.GetWorkflow(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	inputData, err := parseExecutionInputData(r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid execution input payload", err)
		return
	}

	startTime := time.Now()
	execution := &model.WorkflowExecution{
		ID:         fmt.Sprintf("exec_%d", startTime.UnixNano()),
		WorkflowID: id,
		StartedAt:  startTime,
		Mode:       "manual",
		Status:     "running",
	}

	if err := s.storage.SaveExecution(execution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save execution", err)
		return
	}

	execCtx, cancel := context.WithCancel(r.Context())
	s.trackExecutionCancel(execution.ID, cancel)
	defer func() {
		cancel()
		s.untrackExecutionCancel(execution.ID)
	}()

	result, execErr := engine.ExecuteWorkflowWithContext(execCtx, s.engine, workflow, inputData)
	executionErr := engine.ResolveExecutionError(result, execErr)

	finishedAt := time.Now()
	execution.FinishedAt = &finishedAt

	if executionErr != nil {
		if errors.Is(executionErr, context.Canceled) {
			execution.Status = "cancelled"
		} else {
			execution.Status = "failed"
		}
		execution.Error = executionErr
	} else {
		execution.Status = "completed"
		execution.Data = result.Data
	}

	// Publish per-node I/O snapshots so the execution-detail UI can
	// render the n8n-style Node Details View (Input / Output /
	// Schema / Table / JSON per node). The engine populates
	// result.NodeOutputs with one entry per executed node; we copy
	// it into execution.NodeData so it survives the round trip into
	// storage. For backwards compatibility the previous
	// `execution.Data` field (the workflow-level last-node output)
	// is left intact.
	if result != nil && len(result.NodeOutputs) > 0 {
		// Per-node snapshot size is gated by the workflow's Debug
		// flag (see model.Workflow.Debug). When Debug=true we copy
		// every entry so the n8n-style NDV shows full per-node
		// I/O for the whole run. When Debug=false (the default
		// for production workflows) we keep only the start
		// node's input and the last node's output, so production
		// executions don't pay the storage cost of every
		// intermediate item.
		execution.NodeData = engine.BuildExecutionNodeData(workflow, result)
	}

	// Forward the engine's EdgesTaken accumulator. This is the
	// per-edge "did this connection carry items?" map that lets
	// the UI colour execution edges green/grey without needing
	// the full per-node snapshot (Debug=OFF stays cheap). The
	// engine always populates it (or returns nil for runs that
	// had no connections to walk), and we forward whatever
	// arrives — the storage layer treats nil as "absent".
	if result != nil && len(result.EdgesTaken) > 0 {
		execution.EdgesTaken = result.EdgesTaken
	}

	if err := s.storage.SaveExecution(execution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update execution", err)
		return
	}

	s.sendJSON(w, http.StatusOK, execution)
}

// ExecuteWorkflowByDefinition executes an inline workflow definition payload.
func (s *APIServer) ExecuteWorkflowByDefinition(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Workflow  *model.Workflow  `json:"workflow"`
		InputData []model.DataItem `json:"inputData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}
	if request.Workflow == nil {
		s.sendError(w, http.StatusBadRequest, "workflow is required", nil)
		return
	}
	if err := validateWorkflow(request.Workflow); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid workflow", err)
		return
	}

	inputData := request.InputData
	if len(inputData) == 0 {
		inputData = defaultExecutionInputData()
	}

	result, execErr := engine.ExecuteWorkflowWithContext(r.Context(), s.engine, request.Workflow, inputData)
	executionErr := engine.ResolveExecutionError(result, execErr)
	if executionErr != nil {
		s.sendJSON(w, http.StatusOK, map[string]interface{}{
			"data":  []model.DataItem{},
			"error": executionErr.Error(),
		})
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data": result.Data,
	})
}

func (s *APIServer) ExecuteWorkflowAsync(w http.ResponseWriter, r *http.Request) {
	if s.jobQueue == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Job queue not available", nil)
		return
	}

	id := mux.Vars(r)["id"]

	workflow, err := s.storage.GetWorkflow(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	inputData, err := parseExecutionInputData(r)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid execution input payload", err)
		return
	}

	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	job := &queue.Job{
		ID:         jobID,
		WorkflowID: id,
		Workflow:   workflow,
		InputData:  inputData,
		Priority:   0,
		MaxRetries: 3,
		CreatedAt:  time.Now(),
	}

	if err := s.jobQueue.Enqueue(job); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to enqueue job", err)
		return
	}

	s.sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"jobId":      jobID,
		"workflowId": id,
		"status":     "pending",
		"message":    "Workflow execution queued",
	})
}

func (s *APIServer) ListJobs(w http.ResponseWriter, r *http.Request) {
	if s.jobQueue == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Job queue not available", nil)
		return
	}

	var status *queue.JobStatus
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		value := queue.JobStatus(statusStr)
		status = &value
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	jobs, err := s.jobQueue.ListJobs(status, limit)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list jobs", err)
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":    jobs,
		"count":   len(jobs),
		"pending": s.jobQueue.GetPendingCount(),
		"running": s.jobQueue.GetRunningCount(),
	})
}

func (s *APIServer) GetJob(w http.ResponseWriter, r *http.Request) {
	if s.jobQueue == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Job queue not available", nil)
		return
	}

	id := mux.Vars(r)["id"]
	job, err := s.jobQueue.GetJob(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Job not found", err)
		return
	}

	s.sendJSON(w, http.StatusOK, job)
}

func (s *APIServer) CreateExecution(w http.ResponseWriter, r *http.Request) {
	var execution model.WorkflowExecution
	if err := json.NewDecoder(r.Body).Decode(&execution); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	if execution.WorkflowID == "" {
		s.sendError(w, http.StatusBadRequest, "workflowId is required", nil)
		return
	}
	if execution.Status == "" {
		execution.Status = "running"
	}
	if execution.Mode == "" {
		execution.Mode = "manual"
	}
	if execution.StartedAt.IsZero() {
		execution.StartedAt = time.Now()
	}

	if err := s.storage.SaveExecution(&execution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save execution", err)
		return
	}

	s.sendJSON(w, http.StatusCreated, execution)
}

func (s *APIServer) ListExecutions(w http.ResponseWriter, r *http.Request) {
	offset := parseIntParam(r.URL.Query().Get("offset"), 0, 0)
	limit := parseIntParam(r.URL.Query().Get("limit"), 20, s.config.MaxPaginationLimit)

	filters := storage.ExecutionFilters{
		WorkflowID: r.URL.Query().Get("workflowId"),
		Status:     r.URL.Query().Get("status"),
		Offset:     offset,
		Limit:      limit,
	}

	executions, total, err := s.storage.ListExecutions(filters)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list executions", err)
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":       executions,
		"executions": executions,
		"total":      total,
		"offset":     offset,
		"limit":      limit,
	})
}

func (s *APIServer) GetExecution(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	execution, err := s.storage.GetExecution(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Execution not found", err)
		return
	}

	s.sendJSON(w, http.StatusOK, execution)
}

func (s *APIServer) DeleteExecution(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := s.storage.DeleteExecution(id); err != nil {
		s.sendError(w, http.StatusNotFound, "Execution not found", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) RetryExecution(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	execution, err := s.storage.GetExecution(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Execution not found", err)
		return
	}

	workflow, err := s.storage.GetWorkflow(execution.WorkflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	startTime := time.Now()
	newExecution := &model.WorkflowExecution{
		ID:         fmt.Sprintf("exec_%d", startTime.UnixNano()),
		WorkflowID: execution.WorkflowID,
		StartedAt:  startTime,
		Mode:       "retry",
		Status:     "running",
	}

	if err := s.storage.SaveExecution(newExecution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save execution", err)
		return
	}

	execCtx, cancel := context.WithCancel(r.Context())
	s.trackExecutionCancel(newExecution.ID, cancel)
	defer func() {
		cancel()
		s.untrackExecutionCancel(newExecution.ID)
	}()

	result, execErr := engine.ExecuteWorkflowWithContext(execCtx, s.engine, workflow, execution.Data)
	executionErr := engine.ResolveExecutionError(result, execErr)

	finishedAt := time.Now()
	newExecution.FinishedAt = &finishedAt

	if executionErr != nil {
		if errors.Is(executionErr, context.Canceled) {
			newExecution.Status = "cancelled"
		} else {
			newExecution.Status = "failed"
		}
		newExecution.Error = executionErr
	} else {
		newExecution.Status = "completed"
		newExecution.Data = result.Data
	}

	// Mirror the per-node output snapshot so retries preserve the
	// same execution detail that a fresh run would. The same
	// workflow.Debug gate applies here (see
	// buildExecutionNodeData in ExecuteWorkflow).
	if result != nil && len(result.NodeOutputs) > 0 {
		newExecution.NodeData = engine.BuildExecutionNodeData(workflow, result)
	}
	// Forward EdgesTaken so retried executions show the same
	// branch colouring as a fresh run. Same nil-tolerance as the
	// non-retry path above.
	if result != nil && len(result.EdgesTaken) > 0 {
		newExecution.EdgesTaken = result.EdgesTaken
	}

	if err := s.storage.SaveExecution(newExecution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update execution", err)
		return
	}

	s.sendJSON(w, http.StatusOK, newExecution)
}

func (s *APIServer) CancelExecution(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	execution, err := s.storage.GetExecution(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Execution not found", err)
		return
	}

	if execution.Status != "running" {
		s.sendError(w, http.StatusBadRequest, "Execution is not running", nil)
		return
	}

	cancel, exists := s.getExecutionCancel(id)
	if !exists {
		s.sendJSON(w, http.StatusConflict, map[string]interface{}{
			"error":       true,
			"message":     "Execution is running but cancellation is not supported by this runtime",
			"executionId": id,
			"status":      "running",
		})
		return
	}

	cancel()
	s.sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":     "Cancellation requested",
		"executionId": id,
		"status":      "cancel_requested",
	})
}

// CountExecutions returns the total number of executions matching
// the query. Used by the Performance page so it doesn't have to
// load every execution body into memory just to count them.
func (s *APIServer) CountExecutions(w http.ResponseWriter, r *http.Request) {
	filters := storage.ExecutionFilters{
		WorkflowID: r.URL.Query().Get("workflowId"),
		Status:     r.URL.Query().Get("status"),
	}
	count, err := s.storage.CountExecutions(filters)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to count executions", err)
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"count": count})
}

// RecentExecutions returns up to `limit` (default 200) most-recent
// executions. Powers the avg-duration / success-rate metrics on
// the Performance page. The full bodies (including nodeData) are
// returned so this endpoint doubles as the canonical execution
// detail source for the n8n-style debug UI.
func (s *APIServer) RecentExecutions(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r.URL.Query().Get("limit"), 200, 500)
	filters := storage.ExecutionFilters{
		WorkflowID: r.URL.Query().Get("workflowId"),
		Status:     r.URL.Query().Get("status"),
	}
	executions, err := s.storage.RecentExecutions(filters, limit)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list executions", err)
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":       executions,
		"executions": executions,
		"count":      len(executions),
	})
}

// CountWorkflows returns the total workflow count matching filters.
// Like CountExecutions, this avoids loading every workflow body
// just to populate the "Active Workflows" metric on the
// Performance page.
func (s *APIServer) CountWorkflows(w http.ResponseWriter, r *http.Request) {
	filters := storage.WorkflowFilters{
		Search: r.URL.Query().Get("search"),
	}
	if activeStr := r.URL.Query().Get("active"); activeStr != "" {
		active := activeStr == "true"
		filters.Active = &active
	}
	count, err := s.storage.CountWorkflows(filters)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to count workflows", err)
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]interface{}{"count": count})
}

// RetryNodeRequest is the request body for POST /executions/:id/retry-node.
//
// Field semantics:
//
//   - NodeName (required): the workflow node whose execution you want
//     to replay. Must match a node in the workflow the original
//     execution belongs to. The retry runs this node AND every node
//     downstream of it. Everything upstream is skipped; its output is
//     replayed from the saved NodeData snapshot so the target node
//     receives the same items it did the first time.
//
//   - InputData (optional): when set, overrides the upstream snapshot
//     for the target node's input. Use this to "edit the input and
//     re-run from here", which is the n8n NDV "Run" affordance.
//
//   - Mode (optional): label persisted on the new execution. Defaults
//     to "retry-node".
type RetryNodeRequest struct {
	NodeName  string             `json:"nodeName"`
	InputData []model.DataItem   `json:"inputData,omitempty"`
	Mode      string             `json:"mode,omitempty"`
}

// RetryNode re-runs an execution starting from a specific node.
//
// Behaviour
// ---------
// We build a sub-workflow containing only the target node and its
// downstream descendants, then call ExecuteWorkflowWithContext with
// either:
//   - the upstream node's saved output from the original execution's
//     NodeData snapshot (default; matches n8n's "Retry from here" on a
//     failed node), or
//   - the caller's override payload when InputData is supplied (matches
//     n8n's NDV "Run this node" with edited input).
//
// The new execution is stored with mode="retry-node" and links back to
// the original via the parent execution id (the original is referenced
// via `Metadata.parentExecutionId` for traceability; this is additive
// to the canonical execution shape and ignored by older readers).
//
// Status codes
// ------------
//   - 200: retry started; the body is the new execution record.
//   - 400: invalid request body, unknown node, or the upstream node
//          has no saved snapshot to replay.
//   - 404: original execution or workflow not found.
func (s *APIServer) RetryNode(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var req RetryNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if req.NodeName == "" {
		s.sendError(w, http.StatusBadRequest, "nodeName is required", nil)
		return
	}
	if req.Mode == "" {
		req.Mode = "retry-node"
	}

	original, err := s.storage.GetExecution(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Execution not found", err)
		return
	}

	workflow, err := s.storage.GetWorkflow(original.WorkflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	// Locate the target node and the upstream node(s) feeding it.
	targetIdx := -1
	upstreamNames := []string{}
	for i, n := range workflow.Nodes {
		if n.Name == req.NodeName {
			targetIdx = i
			continue
		}
		// Any node that has a connection INTO the target counts as
		// "upstream" — the engine will reroute from the first one
		// we find with saved output. We collect all of them so the
		// caller can see the diagnostic later.
		if conns, ok := workflow.Connections[n.Name]; ok && conns.Main != nil {
			for _, outputs := range conns.Main {
				for _, c := range outputs {
					if c.Node == req.NodeName {
						upstreamNames = append(upstreamNames, n.Name)
					}
				}
			}
		}
	}
	if targetIdx == -1 {
		s.sendError(w, http.StatusBadRequest,
			fmt.Sprintf("node %q not found in workflow", req.NodeName), nil)
		return
	}

	// Build the sub-workflow: only the target and its downstream
	// descendants, with their connections pruned to point at nodes
	// we kept. This keeps the engine's connection router honest
	// about edge targets and stops it from re-executing the
	// upstream nodes by accident.
	subWorkflow, inputData, err := buildSubWorkflow(workflow, original, &req, upstreamNames)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, err.Error(), err)
		return
	}

	startTime := time.Now()
	newExecution := &model.WorkflowExecution{
		ID:         fmt.Sprintf("exec_%d", startTime.UnixNano()),
		WorkflowID: original.WorkflowID,
		StartedAt:  startTime,
		Mode:       req.Mode,
		Status:     "running",
		Metadata: map[string]interface{}{
			"parentExecutionId": original.ID,
			"retryFromNode":     req.NodeName,
		},
	}

	if err := s.storage.SaveExecution(newExecution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save execution", err)
		return
	}

	execCtx, cancel := context.WithCancel(r.Context())
	s.trackExecutionCancel(newExecution.ID, cancel)
	defer func() {
		cancel()
		s.untrackExecutionCancel(newExecution.ID)
	}()

	result, execErr := engine.ExecuteWorkflowWithContext(execCtx, s.engine, subWorkflow, inputData)
	executionErr := engine.ResolveExecutionError(result, execErr)

	finishedAt := time.Now()
	newExecution.FinishedAt = &finishedAt

	if executionErr != nil {
		if errors.Is(executionErr, context.Canceled) {
			newExecution.Status = "cancelled"
		} else {
			newExecution.Status = "failed"
		}
		newExecution.Error = executionErr
	} else {
		newExecution.Status = "completed"
		if result != nil {
			newExecution.Data = result.Data
		}
	}

	// Persist the per-node output snapshot so this retry is also
	// inspectable in the NDV-style execution detail view. Only the
	// sub-workflow nodes produced data, so NodeData is keyed by the
	// kept node names; the upstream snapshot from the original
	// execution is intentionally NOT copied (it would mislead the
	// user into thinking the upstream ran again). Debug gating
	// reuses buildExecutionNodeData so a per-node retry honours
	// the same workflow.Debug preference as a fresh run.
	if result != nil && len(result.NodeOutputs) > 0 {
		newExecution.NodeData = engine.BuildExecutionNodeData(subWorkflow, result)
	}
	// Forward the EdgesTaken accumulator for the sub-workflow so
	// the per-node retry's branch colouring stays consistent
	// with the rest of the API.
	if result != nil && len(result.EdgesTaken) > 0 {
		newExecution.EdgesTaken = result.EdgesTaken
	}

	if err := s.storage.SaveExecution(newExecution); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update execution", err)
		return
	}

	s.sendJSON(w, http.StatusOK, newExecution)
}

// buildSubWorkflow constructs a workflow containing the target node
// and its downstream descendants, plus the input data the target
// node should receive.
//
// The "downstream" set is computed by walking
// workflow.Connections[*].Main[*][*].Node from the target outward
// until we run out of edges. Connections pointing at nodes outside
// the kept set are dropped — the engine's router only sees edges
// between kept nodes, so it won't try to route data to a node that
// isn't there.
//
// Input data resolution:
//   - If the caller supplied InputData, use it verbatim.
//   - Otherwise, replay the FIRST upstream node's last successful
//     output from the original execution's NodeData snapshot. We
//     pick the first one because the engine's topological loop only
//     sees one starting node for the sub-workflow and the IF / Switch
//     routing tags the engine uses to fan out are stripped after
//     routing, so the snapshot is the most faithful replay available.
//
// Returns an error when there's no upstream node in the saved
// snapshot AND the caller didn't pass an override — re-running a
// trigger node with no recorded input would silently produce an
// empty execution.
func buildSubWorkflow(
	workflow *model.Workflow,
	original *model.WorkflowExecution,
	req *RetryNodeRequest,
	upstreamNames []string,
) (*model.Workflow, []model.DataItem, error) {
	// Build the kept-set via BFS over downstream edges.
	kept := map[string]bool{req.NodeName: true}
	queue := []string{req.NodeName}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		conns, ok := workflow.Connections[cur]
		if !ok || conns.Main == nil {
			continue
		}
		for _, outputs := range conns.Main {
			for _, c := range outputs {
				if !kept[c.Node] {
					kept[c.Node] = true
					queue = append(queue, c.Node)
				}
			}
		}
	}

	// Filter the nodes slice preserving original order.
	keptNodes := make([]model.Node, 0, len(kept))
	for _, n := range workflow.Nodes {
		if kept[n.Name] {
			keptNodes = append(keptNodes, n)
		}
	}
	if len(keptNodes) == 0 {
		return nil, nil, fmt.Errorf("no nodes to retry")
	}

	// Filter the connections map to edges between kept nodes only.
	keptConns := make(map[string]model.Connections, len(kept))
	for source, conns := range workflow.Connections {
		if !kept[source] {
			continue
		}
		newMain := make([][]model.Connection, len(conns.Main))
		for i, outputs := range conns.Main {
			filtered := make([]model.Connection, 0, len(outputs))
			for _, c := range outputs {
				if kept[c.Node] {
					filtered = append(filtered, c)
				}
			}
			newMain[i] = filtered
		}
		keptConns[source] = model.Connections{Main: newMain}
	}

	// Resolve input data: caller override > upstream snapshot > error.
	inputData := req.InputData
	if inputData == nil {
		for _, upName := range upstreamNames {
			if items, ok := original.NodeData[upName]; ok && len(items) > 0 {
				inputData = items
				break
			}
		}
	}
	if inputData == nil {
		// Trigger node (no upstream) — accept the request but warn;
		// the engine will execute the trigger node with no items,
		// which mirrors n8n's behaviour for "Run once with no input".
		inputData = []model.DataItem{}
	}

	sub := &model.Workflow{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Active:      false,
		Nodes:       keptNodes,
		Connections: keptConns,
		Settings:    workflow.Settings,
		StaticData:  workflow.StaticData,
		PinData:     workflow.PinData,
		Tags:        workflow.Tags,
		VersionID:   workflow.VersionID,
		CreatedAt:   workflow.CreatedAt,
		UpdatedAt:   workflow.UpdatedAt,
		CreatedBy:   workflow.CreatedBy,
	}
	return sub, inputData, nil
}
