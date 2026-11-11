package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// defaultTagPageSize is the page size used for the tag list when the
// caller does not pass ?limit=.
const defaultTagPageSize = 50

// tagCreateRequest is the inbound body of POST /api/v1/tags.
//
// Only `name` and `color` are accepted — the id and the timestamps are
// assigned by the server, exactly like the credential create endpoint
// does.
type tagCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// tagUpdateRequest is the inbound body of PUT/PATCH
// /api/v1/tags/{id}. Both fields are optional: an omitted field keeps
// the stored value.
type tagUpdateRequest struct {
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// workflowTagsRequest is the inbound body of POST/PUT
// /api/v1/workflows/{id}/tags. `tagIds` is the canonical field; `tags`
// is accepted as an alias so a client that mirrors the workflow model
// (`workflow.tags`) works too.
type workflowTagsRequest struct {
	TagIDs []string `json:"tagIds,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}

// ListTags returns every tag in the store.
//
// GET /api/v1/tags
//
//	{
//	  "data":  [{"id":"tag_1","name":"Production","color":"#FF5733","createdAt":"…","updatedAt":"…"}],
//	  "tags":  [ … same array … ],
//	  "total": 1,
//	  "count": 1,
//	  "limit": 50,
//	  "offset": 0
//	}
//
// The envelope mirrors the one every other m9m list endpoint uses
// ("data" + a domain alias, see /workflows and /executions) and the
// workflow-version envelope (see /workflows/{id}/versions), so the UI
// can share one list component for both features.
func (s *APIServer) ListTags(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	tags, err := s.storage.ListTags()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list tags", err)
		return
	}

	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	filtered := make([]*storage.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(tag.Name), search) {
			continue
		}
		filtered = append(filtered, tag)
	}
	sortTags(filtered)

	total := len(filtered)
	offset := parseIntParam(r.URL.Query().Get("offset"), 0, 0)
	limit := parseIntParam(r.URL.Query().Get("limit"), defaultTagPageSize, s.maxPaginationLimit())

	page := paginateTags(filtered, offset, limit)

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":   page,
		"tags":   page,
		"total":  total,
		"count":  len(page),
		"limit":  limit,
		"offset": offset,
	})
}

// CreateTag stores a new tag.
//
// POST /api/v1/tags  {"name":"Production","color":"#FF5733"}
//
// 201 → {"id":"tag_…","name":"Production","color":"#FF5733","createdAt":"…","updatedAt":"…"}
func (s *APIServer) CreateTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	var request tagCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		s.sendError(w, http.StatusBadRequest, "name is required", nil)
		return
	}

	// The `tags` table has a UNIQUE constraint on `name`; check it up
	// front so the client gets a readable 409 instead of a 500 from the
	// driver.
	existing, err := s.findTagByName(name)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to list tags", err)
		return
	}
	if existing != nil {
		s.sendError(w, http.StatusConflict, "tag name already exists", nil)
		return
	}

	tag := &storage.Tag{
		Name:  name,
		Color: strings.TrimSpace(request.Color),
	}
	if err := s.storage.SaveTag(tag); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to save tag", err)
		return
	}

	s.sendJSON(w, http.StatusCreated, tag)
}

// GetTag returns a single tag.
//
// GET /api/v1/tags/{id}
// 200 -> {"id":"tag_…","name":"Production",…}
// 404 -> {"error":true,"message":"Tag not found","code":404}
func (s *APIServer) GetTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	id := mux.Vars(r)["id"]
	tag, err := s.storage.GetTag(id)
	if err != nil {
		if isTagNotFound(err) {
			s.sendError(w, http.StatusNotFound, "Tag not found", err)
		} else {
			s.sendError(w, http.StatusInternalServerError, "Failed to get tag", err)
		}
		return
	}

	s.sendJSON(w, http.StatusOK, tag)
}

// UpdateTag renames and/or recolours a tag.
//
// PUT|PATCH /api/v1/tags/{id} with body {"name":"Staging","color":"#00FF00"}
// 200 -> {"id":"tag_…","name":"Staging","color":"#00FF00","createdAt":"…","updatedAt":"…"}
// 404 -> unknown tag
func (s *APIServer) UpdateTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	id := mux.Vars(r)["id"]

	var request tagUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	existing, err := s.storage.GetTag(id)
	if err != nil {
		if isTagNotFound(err) {
			s.sendError(w, http.StatusNotFound, "Tag not found", err)
		} else {
			s.sendError(w, http.StatusInternalServerError, "Failed to get tag", err)
		}
		return
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = existing.Name
	}
	color := strings.TrimSpace(request.Color)
	if color == "" {
		color = existing.Color
	}

	if !strings.EqualFold(name, existing.Name) {
		duplicate, err := s.findTagByName(name)
		if err != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to list tags", err)
			return
		}
		if duplicate != nil && duplicate.ID != id {
			s.sendError(w, http.StatusConflict, "tag name already exists", nil)
			return
		}
	}

	// Carry the original creation timestamp over — the SQL backends
	// re-save the whole row on update and would otherwise reset it.
	tag := &storage.Tag{
		ID:        id,
		Name:      name,
		Color:     color,
		CreatedAt: existing.CreatedAt,
	}
	if err := s.storage.UpdateTag(id, tag); err != nil {
		if isTagNotFound(err) {
			s.sendError(w, http.StatusNotFound, "Tag not found", err)
		} else {
			s.sendError(w, http.StatusInternalServerError, "Failed to update tag", err)
		}
		return
	}

	s.sendJSON(w, http.StatusOK, tag)
}

// DeleteTag removes a tag.
//
// DELETE /api/v1/tags/{id} → 204 (no body)
//
//	→ 404 unknown tag
func (s *APIServer) DeleteTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	id := mux.Vars(r)["id"]
	if err := s.storage.DeleteTag(id); err != nil {
		if isTagNotFound(err) {
			s.sendError(w, http.StatusNotFound, "Tag not found", err)
		} else {
			s.sendError(w, http.StatusInternalServerError, "Failed to delete tag", err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetWorkflowTags returns the tags assigned to a workflow.
//
// GET /api/v1/workflows/{id}/tags
//
//	{
//	  "data":   [{"id":"tag_1","name":"Production",…}],
//	  "count":  1,
//	  "tagIds": ["tag_1"]
//	}
//
// The workflow stores tag IDs (the same IDs the `tags` table holds);
// this endpoint resolves them into full tag objects. Unknown IDs are
// skipped instead of failing the whole request.
func (s *APIServer) GetWorkflowTags(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	workflowID := mux.Vars(r)["id"]
	workflow, err := s.storage.GetWorkflow(workflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	tags, tagIDs := s.resolveWorkflowTags(workflow)

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":   tags,
		"count":  len(tags),
		"tagIds": tagIDs,
	})
}

// SetWorkflowTags replaces the tag list of a workflow.
//
// POST|PUT /api/v1/workflows/{id}/tags  {"tagIds":["tag_1","tag_2"]}
//
// 200 → {"data":[…],"count":N,"tagIds":[…]}
func (s *APIServer) SetWorkflowTags(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	workflowID := mux.Vars(r)["id"]
	workflow, err := s.storage.GetWorkflow(workflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	var request workflowTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	requested := request.TagIDs
	if len(requested) == 0 {
		requested = request.Tags
	}

	tagIDs, err := s.validateTagIDs(requested)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	workflow.Tags = tagIDs
	if err := s.storage.UpdateWorkflow(workflowID, workflow); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update workflow", err)
		return
	}

	tags, _ := s.resolveWorkflowTags(workflow)

	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":   tags,
		"count":  len(tags),
		"tagIds": tagIDs,
	})
}

// AddWorkflowTag attaches a single tag to a workflow.
//
// POST /api/v1/workflows/{id}/tags/{tagId} → 200 {"data":[…],"count":N,"tagIds":[…]}
func (s *APIServer) AddWorkflowTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	vars := mux.Vars(r)
	workflowID := vars["id"]
	tagID := vars["tagId"]

	workflow, err := s.storage.GetWorkflow(workflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	if _, err := s.storage.GetTag(tagID); err != nil {
		if isTagNotFound(err) {
			s.sendError(w, http.StatusNotFound, "Tag not found", err)
		} else {
			s.sendError(w, http.StatusInternalServerError, "Failed to get tag", err)
		}
		return
	}

	for _, existing := range workflow.Tags {
		if existing == tagID {
			// Already attached — answer with the current state.
			tags, tagIDs := s.resolveWorkflowTags(workflow)
			s.sendJSON(w, http.StatusOK, map[string]interface{}{
				"data":   tags,
				"count":  len(tags),
				"tagIds": tagIDs,
			})
			return
		}
	}

	workflow.Tags = append(workflow.Tags, tagID)
	if err := s.storage.UpdateWorkflow(workflowID, workflow); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update workflow", err)
		return
	}

	tags, tagIDs := s.resolveWorkflowTags(workflow)
	s.sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":   tags,
		"count":  len(tags),
		"tagIds": tagIDs,
	})
}

// RemoveWorkflowTag detaches a single tag from a workflow.
//
// DELETE /api/v1/workflows/{id}/tags/{tagId} → 204 (no body)
func (s *APIServer) RemoveWorkflowTag(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.sendError(w, http.StatusServiceUnavailable, "Storage not configured", nil)
		return
	}

	vars := mux.Vars(r)
	workflowID := vars["id"]
	tagID := vars["tagId"]

	workflow, err := s.storage.GetWorkflow(workflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, "Workflow not found", err)
		return
	}

	remaining := make([]string, 0, len(workflow.Tags))
	for _, existing := range workflow.Tags {
		if existing != tagID {
			remaining = append(remaining, existing)
		}
	}

	if len(remaining) != len(workflow.Tags) {
		workflow.Tags = remaining
		if err := s.storage.UpdateWorkflow(workflowID, workflow); err != nil {
			s.sendError(w, http.StatusInternalServerError, "Failed to update workflow", err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveWorkflowTags turns the tag IDs stored on the workflow into
// full tag objects. The second return value is the resolved ID list —
// unknown IDs are dropped from it.
func (s *APIServer) resolveWorkflowTags(workflow *model.Workflow) ([]*storage.Tag, []string) {
	tags := make([]*storage.Tag, 0, len(workflow.Tags))
	tagIDs := make([]string, 0, len(workflow.Tags))

	for _, tagID := range workflow.Tags {
		tag, err := s.storage.GetTag(tagID)
		if err != nil || tag == nil {
			continue
		}
		tags = append(tags, tag)
		tagIDs = append(tagIDs, tagID)
	}

	return tags, tagIDs
}

// validateTagIDs checks that every requested tag exists and returns
// the de-duplicated list.
func (s *APIServer) validateTagIDs(requested []string) ([]string, error) {
	seen := make(map[string]bool, len(requested))
	tagIDs := make([]string, 0, len(requested))

	for _, raw := range requested {
		tagID := strings.TrimSpace(raw)
		if tagID == "" {
			continue
		}
		if seen[tagID] {
			continue
		}
		if _, err := s.storage.GetTag(tagID); err != nil {
			return nil, &tagError{message: "tag not found: " + tagID}
		}
		seen[tagID] = true
		tagIDs = append(tagIDs, tagID)
	}

	return tagIDs, nil
}

// tagError is a validation error with a client-facing message.
type tagError struct {
	message string
}

func (e *tagError) Error() string {
	return e.message
}

// findTagByName returns the tag with the given name (case-insensitive)
// or nil when there is none.
func (s *APIServer) findTagByName(name string) (*storage.Tag, error) {
	tags, err := s.storage.ListTags()
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		if strings.EqualFold(tag.Name, name) {
			return tag, nil
		}
	}
	return nil, nil
}

// isTagNotFound reports whether a storage error means "no such tag".
// The SQL backends return a plain fmt.Errorf, so the message is the
// only signal available.
func isTagNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// sortTags orders tags by name (case-insensitive), then by id, so the
// list is deterministic across storage backends.
func sortTags(tags []*storage.Tag) {
	sort.SliceStable(tags, func(i, j int) bool {
		left := strings.ToLower(tags[i].Name)
		right := strings.ToLower(tags[j].Name)
		if left == right {
			return tags[i].ID < tags[j].ID
		}
		return left < right
	})
}

// paginateTags applies the offset/limit window to a tag slice.
func paginateTags(tags []*storage.Tag, offset, limit int) []*storage.Tag {
	if offset > 0 {
		if offset >= len(tags) {
			return []*storage.Tag{}
		}
		tags = tags[offset:]
	}
	if limit > 0 && limit < len(tags) {
		tags = tags[:limit]
	}
	return tags
}

// maxPaginationLimit returns the configured page-size cap, falling back
// to the default when the server has no config (unit tests).
func (s *APIServer) maxPaginationLimit() int {
	if s.config == nil || s.config.MaxPaginationLimit <= 0 {
		return 100
	}
	return s.config.MaxPaginationLimit
}
