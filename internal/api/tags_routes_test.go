package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// newTagRoutesTestServer wires a router with the tag routes registered
// against an in-memory store — no database needed.
func newTagRoutesTestServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()

	store := storage.NewMemoryStorage()
	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router, store
}

// tagListEnvelope is the JSON body of GET /api/v1/tags.
type tagListEnvelope struct {
	Data   []storage.Tag `json:"data"`
	Tags   []storage.Tag `json:"tags"`
	Total  int           `json:"total"`
	Count  int           `json:"count"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// TestTagRoutesRegistered asserts the tag routes exist — the live
// server answered 404 for /api/v1/tags because they were never wired.
func TestTagRoutesRegistered(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/tags", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"data\"")
}

// TestTagCollectionRouteBeforeTagIDRoute asserts the route ordering:
// the "/tags" collection is registered before the "/tags/{id}" item
// route, so the collection is never read as a tag whose id is "tags".
func TestTagCollectionRouteBeforeTagIDRoute(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	collectionIndex := routeIndex(router, "/tags")
	itemIndex := routeIndex(router, "/tags/{id}")

	require.NotEqual(t, -1, collectionIndex, "tag collection route must be registered")
	require.NotEqual(t, -1, itemIndex, "tag item route must be registered")
	assert.Less(t, collectionIndex, itemIndex,
		"the tag collection route has to be registered before the tag id wildcard")
}

// TestListTags asserts the list endpoint returns the stored tags in the
// documented envelope.
func TestListTags(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-b", Name: "Beta", Color: "#111111"}))
	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Alpha"}))

	req := httptest.NewRequest("GET", "/api/v1/tags", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope tagListEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 2)
	// Sorted by name.
	assert.Equal(t, "Alpha", envelope.Data[0].Name)
	assert.Equal(t, "Beta", envelope.Data[1].Name)
	assert.Len(t, envelope.Tags, 2)
	assert.Equal(t, 2, envelope.Total)
	assert.Equal(t, 2, envelope.Count)
	assert.Equal(t, 50, envelope.Limit)
	assert.Equal(t, 0, envelope.Offset)
}

// TestListTagsSearch asserts the ?search= filter.
func TestListTagsSearch(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production"}))
	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-b", Name: "Staging"}))

	req := httptest.NewRequest("GET", "/api/v1/tags?search=prod", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope tagListEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "Production", envelope.Data[0].Name)
	assert.Equal(t, 1, envelope.Total)
}

// TestCreateTag asserts the create endpoint stores the tag, assigns an
// id and answers 201 with the created tag.
func TestCreateTag(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	body := strings.NewReader(`{"name":"Production","color":"#FF5733"}`)
	req := httptest.NewRequest("POST", "/api/v1/tags", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var created storage.Tag
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Production", created.Name)
	assert.Equal(t, "#FF5733", created.Color)
	assert.False(t, created.CreatedAt.IsZero())

	// The tag really landed in the store.
	stored, err := store.GetTag(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Production", stored.Name)
}

// TestCreateTagMissingName asserts a 400 for a tag without a name.
func TestCreateTagMissingName(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	body := strings.NewReader(`{"color":"#FF5733"}`)
	req := httptest.NewRequest("POST", "/api/v1/tags", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Contains(t, envelope["message"], "name")
}

// TestCreateTagDuplicateName asserts a 409 for a duplicate name (the
// `tags` table has a UNIQUE constraint on `name`).
func TestCreateTagDuplicateName(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production"}))

	body := strings.NewReader(`{"name":"production"}`)
	req := httptest.NewRequest("POST", "/api/v1/tags", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
}

// TestGetTag asserts the single-tag endpoint.
func TestGetTag(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production", Color: "#FF5733"}))

	req := httptest.NewRequest("GET", "/api/v1/tags/tag-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var tag storage.Tag
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tag))
	assert.Equal(t, "tag-a", tag.ID)
	assert.Equal(t, "Production", tag.Name)
}

// TestGetTagNotFound asserts a 404 for an unknown tag id.
func TestGetTagNotFound(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/tags/tag-missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusNotFound), envelope["code"])
}

// TestUpdateTag asserts the update endpoint renames the tag and keeps
// its creation timestamp.
func TestUpdateTag(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production", Color: "#FF5733"}))
	existing, err := store.GetTag("tag-a")
	require.NoError(t, err)

	body := strings.NewReader(`{"name":"Staging","color":"#00FF00"}`)
	req := httptest.NewRequest("PATCH", "/api/v1/tags/tag-a", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var updated storage.Tag
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "tag-a", updated.ID)
	assert.Equal(t, "Staging", updated.Name)
	assert.Equal(t, "#00FF00", updated.Color)
	assert.Equal(t, existing.CreatedAt.UTC(), updated.CreatedAt.UTC())

	stored, err := store.GetTag("tag-a")
	require.NoError(t, err)
	assert.Equal(t, "Staging", stored.Name)
}

// TestUpdateTagNotFound asserts a 404 for an unknown tag id.
func TestUpdateTagNotFound(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	body := strings.NewReader(`{"name":"Staging"}`)
	req := httptest.NewRequest("PUT", "/api/v1/tags/tag-missing", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestDeleteTag asserts the delete endpoint removes the tag and
// answers 204.
func TestDeleteTag(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production"}))

	req := httptest.NewRequest("DELETE", "/api/v1/tags/tag-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	_, err := store.GetTag("tag-a")
	require.Error(t, err)
}

// TestDeleteTagNotFound asserts a 404 for an unknown tag id.
func TestDeleteTagNotFound(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/v1/tags/tag-missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestWorkflowTagRoutes asserts the workflow ↔ tag assignment endpoints:
// a tag created through the API can be attached to a workflow and read
// back.
func TestWorkflowTagRoutes(t *testing.T) {
	router, store := newTagRoutesTestServer(t)

	require.NoError(t, store.SaveWorkflow(&model.Workflow{ID: "wf-1", Name: "Tagged"}))
	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production"}))

	// Attach.
	body := strings.NewReader(`{"tagIds":["tag-a"]}`)
	req := httptest.NewRequest("POST", "/api/v1/workflows/wf-1/tags", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var attach struct {
		Data   []storage.Tag `json:"data"`
		Count  int           `json:"count"`
		TagIDs []string      `json:"tagIds"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &attach))
	require.Len(t, attach.Data, 1)
	assert.Equal(t, "Production", attach.Data[0].Name)
	assert.Equal(t, []string{"tag-a"}, attach.TagIDs)

	// Read back.
	req = httptest.NewRequest("GET", "/api/v1/workflows/wf-1/tags", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &attach))
	require.Len(t, attach.Data, 1)
	assert.Equal(t, "tag-a", attach.Data[0].ID)

	// The workflow itself now carries the tag id.
	workflow, err := store.GetWorkflow("wf-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"tag-a"}, workflow.Tags)

	// Detach.
	req = httptest.NewRequest("DELETE", "/api/v1/workflows/wf-1/tags/tag-a", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	workflow, err = store.GetWorkflow("wf-1")
	require.NoError(t, err)
	assert.Empty(t, workflow.Tags)
}

// TestWorkflowTagRoutesUnknownWorkflow asserts a 404 for a workflow
// that does not exist.
func TestWorkflowTagRoutesUnknownWorkflow(t *testing.T) {
	router, _ := newTagRoutesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/workflows/wf-missing/tags", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}
