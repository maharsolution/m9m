package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/storage"
)

// newTagsFilterTestServer wires a router with an in-memory store — no
// database needed.
func newTagsFilterTestServer(t *testing.T) (*mux.Router, *storage.MemoryStorage) {
	t.Helper()

	store := storage.NewMemoryStorage()
	server := NewAPIServer(engine.NewWorkflowEngine(), nil, store)
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router, store
}

// tagsFilterEnvelope is the JSON body of GET /api/v1/workflows.
type tagsFilterEnvelope struct {
	Data      []model.Workflow `json:"data"`
	Workflows []model.Workflow `json:"workflows"`
	Total     int              `json:"total"`
	Offset    int              `json:"offset"`
	Limit     int              `json:"limit"`
}

// getWorkflowsFiltered performs GET /api/v1/workflows with the given
// query string and returns the status code plus the raw JSON body.
func getWorkflowsFiltered(t *testing.T, router http.Handler, query string) (int, []byte) {
	t.Helper()

	req := httptest.NewRequest("GET", "/api/v1/workflows"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w.Code, w.Body.Bytes()
}

// decodeTagsFilterEnvelope decodes the workflow list envelope.
func decodeTagsFilterEnvelope(t *testing.T, body []byte) tagsFilterEnvelope {
	t.Helper()

	var envelope tagsFilterEnvelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope
}

// seedTagsFilterFixtures stores two tags and three workflows: one
// carrying the first tag, one carrying the second tag and one without
// any tag.
func seedTagsFilterFixtures(t *testing.T, store *storage.MemoryStorage) {
	t.Helper()

	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production", Color: "#FF5733"}))
	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-b", Name: "Staging", Color: "#00FF00"}))
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-production", Name: "Production flow", Tags: []string{"tag-a"},
	}))
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-staging", Name: "Staging flow", Tags: []string{"tag-b"},
	}))
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-untagged", Name: "Untagged flow",
	}))
}

// TestListWorkflowsWithoutTagFilterIsUnchanged asserts that a request
// without the `tags` parameter keeps returning every workflow.
func TestListWorkflowsWithoutTagFilterIsUnchanged(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 3, envelope.Total)
	assert.Len(t, envelope.Data, 3)
	assert.Equal(t, 0, envelope.Offset)
	assert.Equal(t, 20, envelope.Limit)
}

// TestListWorkflowsEmptyTagFilterIsUnchanged asserts that an empty
// `tags` parameter behaves exactly like a missing one.
func TestListWorkflowsEmptyTagFilterIsUnchanged(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "?tags=")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 3, envelope.Total)
	assert.Len(t, envelope.Data, 3)
	assert.Equal(t, 0, envelope.Offset)
	assert.Equal(t, 20, envelope.Limit)
}

// TestListWorkflowsFiltersBySingleTagID asserts that a tag ID restricts
// the result to the workflows carrying that tag.
func TestListWorkflowsFiltersBySingleTagID(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "?tags=tag-a")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 1, envelope.Total)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "wf-production", envelope.Data[0].ID)
}

// TestListWorkflowsFiltersBySingleTagName asserts that a tag name
// resolves to the workflows carrying the matching tag ID.
func TestListWorkflowsFiltersBySingleTagName(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "?tags=Production")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 1, envelope.Total)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "wf-production", envelope.Data[0].ID)
}

// TestListWorkflowsFiltersByTagNameCaseInsensitive asserts that the tag
// name lookup ignores the casing.
func TestListWorkflowsFiltersByTagNameCaseInsensitive(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	for _, query := range []string{"?tags=production", "?tags=PRODUCTION", "?tags=PrOdUcTiOn"} {
		code, body := getWorkflowsFiltered(t, router, query)
		require.Equal(t, http.StatusOK, code, "query: %s", query)

		envelope := decodeTagsFilterEnvelope(t, body)
		assert.Equal(t, 1, envelope.Total, "query: %s", query)
		require.Len(t, envelope.Data, 1, "query: %s", query)
		assert.Equal(t, "wf-production", envelope.Data[0].ID, "query: %s", query)
	}
}

// TestListWorkflowsTagFilterUsesAndSemantics asserts that a workflow is
// only returned when it carries ALL of the requested tags.
func TestListWorkflowsTagFilterUsesAndSemantics(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-both", Name: "Both tags", Tags: []string{"tag-a", "tag-b"},
	}))

	// Two IDs.
	code, body := getWorkflowsFiltered(t, router, "?tags=tag-a,tag-b")
	require.Equal(t, http.StatusOK, code, "body: %s", body)
	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 1, envelope.Total)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "wf-both", envelope.Data[0].ID)

	// A name and an ID.
	code, body = getWorkflowsFiltered(t, router, "?tags=Production,tag-b")
	require.Equal(t, http.StatusOK, code, "body: %s", body)
	envelope = decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 1, envelope.Total)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "wf-both", envelope.Data[0].ID)
}

// TestListWorkflowsUnknownTagNameReturnsEmptyResult asserts that an
// unknown tag name yields an empty list and not an error.
func TestListWorkflowsUnknownTagNameReturnsEmptyResult(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "?tags=nosuchtag")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 0, envelope.Total)
	assert.Empty(t, envelope.Data)
	assert.Empty(t, envelope.Workflows)
}

// TestListWorkflowsUnknownTagIDReturnsEmptyResult asserts that an
// unknown tag ID yields an empty list and not an error.
func TestListWorkflowsUnknownTagIDReturnsEmptyResult(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "?tags=tag-missing")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 0, envelope.Total)
	assert.Empty(t, envelope.Data)
}

// TestListWorkflowsTagFilterPagination asserts that the tag filter is
// applied before the pagination: `total` reports the filtered count and
// the page holds the matching workflows only.
func TestListWorkflowsTagFilterPagination(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	require.NoError(t, store.SaveTag(&storage.Tag{ID: "tag-a", Name: "Production"}))

	for _, id := range []string{"wf-1", "wf-2", "wf-3", "wf-4", "wf-5"} {
		require.NoError(t, store.SaveWorkflow(&model.Workflow{
			ID: id, Name: id, Tags: []string{"tag-a"},
		}))
	}
	// Two workflows without the tag — they must not show up and must not
	// be counted.
	require.NoError(t, store.SaveWorkflow(&model.Workflow{ID: "wf-x", Name: "wf-x"}))
	require.NoError(t, store.SaveWorkflow(&model.Workflow{
		ID: "wf-y", Name: "wf-y", Tags: []string{"tag-b"},
	}))

	// First page.
	code, body := getWorkflowsFiltered(t, router, "?tags=tag-a&limit=2&offset=0")
	require.Equal(t, http.StatusOK, code, "body: %s", body)
	envelope := decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 5, envelope.Total)
	assert.Len(t, envelope.Data, 2)
	assert.Equal(t, 0, envelope.Offset)
	assert.Equal(t, 2, envelope.Limit)
	for _, workflow := range envelope.Data {
		assert.Contains(t, workflow.Tags, "tag-a")
	}

	// Last page.
	code, body = getWorkflowsFiltered(t, router, "?tags=tag-a&limit=2&offset=4")
	require.Equal(t, http.StatusOK, code, "body: %s", body)
	envelope = decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 5, envelope.Total)
	assert.Len(t, envelope.Data, 1)
	assert.Equal(t, 4, envelope.Offset)
	for _, workflow := range envelope.Data {
		assert.Contains(t, workflow.Tags, "tag-a")
	}

	// Behind the end.
	code, body = getWorkflowsFiltered(t, router, "?tags=tag-a&limit=2&offset=6")
	require.Equal(t, http.StatusOK, code, "body: %s", body)
	envelope = decodeTagsFilterEnvelope(t, body)
	assert.Equal(t, 5, envelope.Total)
	assert.Empty(t, envelope.Data)
}

// TestListWorkflowsResponseEnvelopeUnchanged asserts that the list
// endpoint still answers with the documented envelope.
func TestListWorkflowsResponseEnvelopeUnchanged(t *testing.T) {
	router, store := newTagsFilterTestServer(t)
	seedTagsFilterFixtures(t, store)

	code, body := getWorkflowsFiltered(t, router, "")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))

	for _, key := range []string{"data", "workflows", "total", "offset", "limit"} {
		assert.Contains(t, raw, key)
	}
	assert.Len(t, raw, 5, "the envelope must not grow or lose keys: %s", body)

	// `data` and `workflows` carry the same list.
	var data, workflows []model.Workflow
	require.NoError(t, json.Unmarshal(raw["data"], &data))
	require.NoError(t, json.Unmarshal(raw["workflows"], &workflows))
	assert.Equal(t, data, workflows)

	// The same envelope is used when a tag filter is applied.
	code, body = getWorkflowsFiltered(t, router, "?tags=tag-a")
	require.Equal(t, http.StatusOK, code, "body: %s", body)

	raw = nil
	require.NoError(t, json.Unmarshal(body, &raw))
	for _, key := range []string{"data", "workflows", "total", "offset", "limit"} {
		assert.Contains(t, raw, key)
	}
	assert.Len(t, raw, 5, "the envelope must not grow or lose keys: %s", body)
}
