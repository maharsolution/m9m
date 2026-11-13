package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// maxWorkflowBodyBytes caps the request body the workflow create /
// update handlers read. A workflow document is a JSON graph of nodes and
// connections - a few hundred kilobytes at most - so 8 MiB is generous
// while still refusing an unbounded upload.
const maxWorkflowBodyBytes = 8 << 20

func (s *APIServer) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("active")
	search := r.URL.Query().Get("search")
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset := parseIntParam(offsetStr, 0, 0)
	limit := parseIntParam(limitStr, 20, s.config.MaxPaginationLimit)

	filters := storage.WorkflowFilters{
		Search: search,
		Offset: offset,
		Limit:  limit,
	}

	if active != "" {
		activeBool := active == "true"
		filters.Active = &activeBool
	}

	// Tag filter. The values may be tag IDs or tag names and may be sent
	// as a comma separated list (`?tags=id-1,Production`) or as repeated
	// query parameters (`?tags=id-1&tags=Production`); both carry the same
	// meaning. A workflow matches when it carries ALL of the requested
	// tags (AND semantics).
	tagValues := parseTagIDFilter(r.URL.Query()["tags"])
	if len(tagValues) > 0 {
		tagIDs, err := s.resolveWorkflowTagIDs(tagValues)
		if err != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to list workflows", err)
			return
		}
		filters.TagIDs = tagIDs
	}

	workflows, total, err := s.storage.ListWorkflows(filters)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list workflows", err)
		return
	}

	// A filtered query that matches nothing must still answer with an
	// empty array instead of `null`: the UI (and every other m9m list
	// endpoint) expects `data` / `workflows` to be a JSON array, and
	// `null` breaks strict clients that iterate the list.
	if workflows == nil {
		workflows = []*model.Workflow{}
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":      workflows,
		"workflows": workflows,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
	})
}

func (s *APIServer) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWorkflowBodyBytes))
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	var workflow model.Workflow
	if err := json.Unmarshal(body, &workflow); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	// Resolve the Debug flag of the incoming workflow.
	//
	//   - `?debug=true` is the explicit opt-in for API clients that want
	//     a debug-enabled workflow from the start.
	//   - A payload that carries "debug" states the caller's choice
	//     explicitly - that choice wins.
	//   - A payload WITHOUT "debug" keeps the stored value when the id
	//     already exists (POST is an upsert - SaveWorkflow overwrites an
	//     existing row). The n8n sync bridge POSTs the workflows it
	//     syncs and never mentions "debug", so without this the bulk
	//     enable would be wiped on every sync cycle.
	//   - A payload WITHOUT "debug" for a brand new workflow means OFF:
	//     the production default.
	workflow.Debug = s.resolveCreatedWorkflowDebug(r, body, &workflow)

	if err := validateWorkflow(&workflow); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid workflow", err)
		return
	}

	if err := s.storage.SaveWorkflow(&workflow); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save workflow", err)
		return
	}

	// If the workflow was created already active, register its webhooks so
	// /webhook/{path} works without a separate activate step.
	if s.webhookManager != nil {
		if workflow.Active {
			if regErr := s.webhookManager.RegisterWorkflowWebhooks(&workflow, false); regErr != nil {
				fmt.Printf("[m9m] warning: failed to register webhooks for new workflow %s: %v\n", workflow.ID, regErr)
			}
		}
		// The test URL (/webhook-test/{path}) has to resolve even while the
		// workflow is still inactive — it is what "Test workflow" hits while
		// the flow is being designed.
		s.webhookManager.SyncWorkflowTestHooks(&workflow)
	}

	s.sendJSON(w, http.StatusCreated, workflow)
}

// resolveCreatedWorkflowDebug decides the Debug flag of a workflow that
// POST /api/v1/workflows is about to store.
//
// The rule has two halves:
//
//   - A BRAND NEW workflow starts with Debug OFF. The flag is never
//     inherited from a payload - a copy of an existing workflow, an
//     import envelope or a template would otherwise carry a debug=true
//     flag into the new row. `?debug=true` is the explicit opt-in.
//   - An EXISTING workflow (POST is an upsert) keeps its flag when the
//     payload does not mention "debug" and changes it when the payload
//     does. The n8n sync bridge POSTs the workflows it syncs and never
//     mentions the flag, so without this the bulk enable would be wiped
//     on every sync cycle.
//
// The query parameter wins over everything - it is the explicit request.
func (s *APIServer) resolveCreatedWorkflowDebug(r *http.Request, body []byte, workflow *model.Workflow) bool {
	if raw := strings.TrimSpace(r.URL.Query().Get("debug")); raw != "" {
		return strings.EqualFold(raw, "true") || raw == "1"
	}

	// Only an existing row can keep or change its flag.
	existing, err := s.storage.GetWorkflow(workflow.ID)
	if err != nil {
		// No such workflow yet -> a brand new one -> Debug OFF.
		return false
	}

	if jsonObjectHasKey(body, "debug") {
		return workflow.Debug
	}
	return existing.Debug
}

// jsonObjectHasKey reports whether the raw JSON object carries the given
// top-level key. It tells an explicit "debug": false apart from a payload
// that does not mention the flag at all.
func jsonObjectHasKey(body []byte, key string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return false
	}
	_, ok := fields[key]
	return ok
}

func validateWorkflow(workflow *model.Workflow) error {
	if workflow.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(workflow.Name) > 255 {
		return fmt.Errorf("workflow name too long (max 255 characters)")
	}

	for i, node := range workflow.Nodes {
		if node.Type == "" {
			return fmt.Errorf("node %d: type is required", i)
		}
		if node.Name == "" {
			return fmt.Errorf("node %d: name is required", i)
		}
		if len(node.Name) > 255 {
			return fmt.Errorf("node %d: name too long (max 255 characters)", i)
		}
	}

	return nil
}

func (s *APIServer) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	workflow, err := s.storage.GetWorkflow(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	s.sendJSON(w, http.StatusOK, workflow)
}

func (s *APIServer) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWorkflowBodyBytes))
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	var workflow model.Workflow
	if err := json.Unmarshal(body, &workflow); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	// The Debug flag is an m9m extension that n8n knows nothing about.
	// A client that PUTs a workflow WITHOUT a "debug" key (the n8n sync
	// bridge does exactly that on every sync cycle) must not silently
	// switch the flag OFF - otherwise the bulk enable would be undone
	// within seconds. When the key is absent the stored value wins.
	if !jsonObjectHasKey(body, "debug") {
		if existing, getErr := s.storage.GetWorkflow(id); getErr == nil {
			workflow.Debug = existing.Debug
		}
	}

	if err := s.storage.UpdateWorkflow(id, &workflow); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update workflow", err)
		return
	}

	// Reconcile webhooks: drop any old ones for this workflow, then re-register
	// only if the workflow is currently active. This keeps the cache consistent
	// across path/method/auth edits.
	if s.webhookManager != nil {
		_ = s.webhookManager.UnregisterWorkflowWebhooks(id)
		if workflow.Active {
			if regErr := s.webhookManager.RegisterWorkflowWebhooks(&workflow, false); regErr != nil {
				fmt.Printf("[m9m] warning: failed to register webhooks for updated workflow %s: %v\n", id, regErr)
			}
		}
		// Re-index the test webhooks so /webhook-test/{path} follows the
		// edited path / HTTP method immediately.
		s.webhookManager.SyncWorkflowTestHooks(&workflow)
	}

	s.sendJSON(w, http.StatusOK, workflow)
}

func (s *APIServer) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := s.storage.DeleteWorkflow(id); err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	// Drop the workflow's test webhooks so /webhook-test/{path} stops
	// resolving once the workflow is gone.
	if s.webhookManager != nil {
		s.webhookManager.RemoveWorkflowTestHooks(id)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) ActivateWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := s.storage.ActivateWorkflow(id); err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	// Keep the webhook manager's activeHooks cache in sync so incoming
	// /webhook/{path} requests can be routed immediately after activation.
	if s.webhookManager != nil {
		if wf, getErr := s.storage.GetWorkflow(id); getErr == nil {
			if regErr := s.webhookManager.RegisterWorkflowWebhooks(wf, false); regErr != nil {
				// Log but don't fail the request — the workflow is activated
				// in storage; webhook cache will reconcile on next restart.
				fmt.Printf("[m9m] warning: failed to register webhooks for workflow %s: %v\n", id, regErr)
			}
			// The test index mirrors the workflow's active state (an active
			// workflow wins over an inactive one on a path collision).
			s.webhookManager.SyncWorkflowTestHooks(wf)
		}
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Workflow activated",
		"active":  true,
	})
}

func (s *APIServer) DeactivateWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := s.storage.DeactivateWorkflow(id); err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	// Remove any webhooks for this workflow from the activeHooks cache.
	if s.webhookManager != nil {
		if err := s.webhookManager.UnregisterWorkflowWebhooks(id); err != nil {
			fmt.Printf("[m9m] warning: failed to unregister webhooks for workflow %s: %v\n", id, err)
		}
		// The test URL keeps working while the workflow is inactive, but the
		// index has to learn that the workflow is no longer active (an
		// active workflow wins a path collision).
		if wf, getErr := s.storage.GetWorkflow(id); getErr == nil {
			s.webhookManager.SyncWorkflowTestHooks(wf)
		}
	}

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Workflow deactivated",
		"active":  false,
	})
}

func (s *APIServer) DuplicateWorkflow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	original, err := s.storage.GetWorkflow(id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	duplicate := &model.Workflow{
		Name:   original.Name + " (Copy)",
		Active: false,
		// A duplicate is a NEW workflow: it starts with Debug OFF even
		// when the original had it turned on.
		Debug:       false,
		Nodes:       make([]model.Node, len(original.Nodes)),
		Connections: make(map[string]model.Connections),
		Settings:    original.Settings,
		Tags:        original.Tags,
	}

	if req.Name != "" {
		duplicate.Name = req.Name
	}

	for i, node := range original.Nodes {
		duplicate.Nodes[i] = model.Node{
			ID:          fmt.Sprintf("node-%d", time.Now().UnixNano()+int64(i)),
			Name:        node.Name,
			Type:        node.Type,
			TypeVersion: node.TypeVersion,
			Position:    node.Position,
			Parameters:  node.Parameters,
			Credentials: node.Credentials,
			Disabled:    node.Disabled,
		}
	}

	for key, conn := range original.Connections {
		duplicate.Connections[key] = conn
	}

	if err := s.storage.SaveWorkflow(duplicate); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save duplicate workflow", err)
		return
	}

	// The copy is inactive, but its test URL (/webhook-test/{path}) has to
	// resolve so the duplicate can be exercised while designing.
	if s.webhookManager != nil {
		s.webhookManager.SyncWorkflowTestHooks(duplicate)
	}

	s.sendJSON(w, http.StatusCreated, duplicate)
}

// parseTagIDFilter extracts the tag IDs from the `tags` query parameter.
//
// The parameter accepts both shapes used by clients:
//
//	?tags=id-1,id-2      comma separated
//	?tags=id-1&tags=id-2 repeated parameters
//
// Empty entries are dropped so `?tags=` behaves like "no filter".
func parseTagIDFilter(values []string) []string {
	var ids []string
	seen := make(map[string]struct{})

	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			id := strings.TrimSpace(part)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	return ids
}

// resolveWorkflowTagIDs maps the raw `tags` query values to canonical
// tag IDs.
//
// The parameter accepts tag IDs as well as tag names; names are matched
// case-insensitively. A value that matches neither a known tag ID nor a
// known tag name is passed through unchanged — the storage layer then
// compares it against the tag IDs carried by the workflows, so an
// unknown tag simply matches nothing instead of raising an error.
//
// The resolution costs one ListTags() call per filtered request. The
// tag catalog is small, so this stays cheap; a deployment with
// thousands of tags would want a cached lookup table.
func (s *APIServer) resolveWorkflowTagIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	tags, err := s.storage.ListTags()
	if err != nil {
		return nil, fmt.Errorf("failed to load tags: %w", err)
	}

	ids := make(map[string]struct{}, len(tags))
	names := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		ids[tag.ID] = struct{}{}
		names[strings.ToLower(tag.Name)] = tag.ID
	}

	resolved := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := value
		if _, ok := ids[value]; !ok {
			if mapped, ok := names[strings.ToLower(value)]; ok {
				id = mapped
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}

	return resolved, nil
}
