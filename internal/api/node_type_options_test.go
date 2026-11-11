package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/engine"
	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/storage"
)

// optionsTestNodeType is the node id used by the option endpoint tests.
const optionsTestNodeType = "n8n-nodes-base.optionsTest"

// optionsTestAction is the `options[].action` declared by the test node.
const optionsTestAction = "loadResourceLists"

// optionsTestNode is a metadata-only node executor: it declares a
// property whose option carries a backend action.
type optionsTestNode struct {
	description base.NodeDescription
}

func (n *optionsTestNode) Description() base.NodeDescription { return n.description }

func (n *optionsTestNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return inputData, nil
}

func (n *optionsTestNode) ValidateParameters(params map[string]interface{}) error { return nil }

// newNodeTypeOptionsTestServer wires a router with the option endpoint
// registered, one node type that declares a dynamic action and one
// node type whose property has only static options.
func newNodeTypeOptionsTestServer(t *testing.T) *mux.Router {
	t.Helper()

	eng := engine.NewWorkflowEngine()
	eng.RegisterNodeExecutor(optionsTestNodeType, &optionsTestNode{
		description: base.NodeDescription{
			Name:     "Options Test",
			Category: "utility",
			Properties: []base.NodeProperty{
				{
					Name:        "resource",
					DisplayName: "Resource",
					Type:        "resourceLocator",
					Options: []base.Option{
						{Name: "List", Value: "list", Action: optionsTestAction, Description: "Pick a list"},
						{Name: "Static", Value: "static"},
					},
				},
				{
					Name:        "staticOnly",
					DisplayName: "Static Only",
					Type:        "string",
					Options: []base.Option{
						{Name: "A", Value: "a"},
					},
				},
			},
		},
	})

	server := NewAPIServer(eng, nil, storage.NewMemoryStorage())
	router := mux.NewRouter()
	server.RegisterRoutes(router)
	return router
}

// decodeNodeTypeOptionsResponse decodes the `{data, message}` envelope.
func decodeNodeTypeOptionsResponse(t *testing.T, body []byte) nodeTypeOptionResponse {
	t.Helper()
	var response nodeTypeOptionResponse
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

// TestGetNodeTypeOptionsUnknownNodeType asserts a 404 with the
// standard error envelope for an unregistered node type.
func TestGetNodeTypeOptionsUnknownNodeType(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/n8n-nodes-base.nope/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusNotFound), envelope["code"])
	assert.Contains(t, envelope["message"], "node type not found")
}

// TestGetNodeTypeOptionsUnknownProperty asserts a 400 when the
// requested property does not exist on the node type.
func TestGetNodeTypeOptionsUnknownProperty(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusBadRequest), envelope["code"])
	assert.Contains(t, envelope["message"], "missing")
}

// TestGetNodeTypeOptionsMissingProperty asserts a 400 when the
// property is not supplied at all.
func TestGetNodeTypeOptionsMissingProperty(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetNodeTypeOptionsNoProviderRegistered asserts the graceful
// degradation: 200, an empty option list and an explanatory message —
// never a 5xx.
func TestGetNodeTypeOptionsNoProviderRegistered(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	require.NotNil(t, response.Data)
	assert.Empty(t, response.Data)
	assert.Contains(t, response.Message, optionsTestAction)
}

// TestGetNodeTypeOptionsPropertyWithoutAction asserts a property that
// declares no action also answers 200 with an empty list and a message.
func TestGetNodeTypeOptionsPropertyWithoutAction(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=staticOnly", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	assert.Empty(t, response.Data)
	assert.NotEmpty(t, response.Message)
}

// TestGetNodeTypeOptionsRegisteredProvider asserts a registered
// provider is called and its options are returned.
func TestGetNodeTypeOptionsRegisteredProvider(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	var captured NodeTypeOptionRequest
	RegisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction,
		func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error) {
			captured = req
			return []NodeTypeOption{
				{Name: "First", Value: "first", Description: "the first one"},
				{Name: "Second", Value: "second"},
			}, nil
		})
	t.Cleanup(func() {
		UnregisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction)
	})

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	require.Len(t, response.Data, 2)
	assert.Equal(t, "First", response.Data[0].Name)
	assert.Equal(t, "first", response.Data[0].Value)
	assert.Equal(t, "the first one", response.Data[0].Description)
	assert.Equal(t, "Second", response.Data[1].Name)

	// The provider receives the resolved action and the node/property.
	assert.Equal(t, optionsTestNodeType, captured.NodeType)
	assert.Equal(t, "resource", captured.Property)
	assert.Equal(t, optionsTestAction, captured.Action)
}

// TestGetNodeTypeOptionsProviderHonoursContext asserts the provider
// runs under a bounded context (deadline) so it cannot hang the API.
func TestGetNodeTypeOptionsProviderHonoursContext(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	var hasDeadline bool
	RegisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction,
		func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error) {
			_, hasDeadline = ctx.Deadline()
			return []NodeTypeOption{}, nil
		})
	t.Cleanup(func() {
		UnregisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction)
	})

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, hasDeadline, "provider context must carry a deadline")
}

// TestGetNodeTypeOptionsProviderFailureDegrades asserts a failing
// provider never turns into a 5xx: the panel gets an empty list and an
// explanatory message.
func TestGetNodeTypeOptionsProviderFailureDegrades(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	RegisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction,
		func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error) {
			return nil, errors.New("upstream exploded")
		})
	t.Cleanup(func() {
		UnregisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction)
	})

	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	assert.Empty(t, response.Data)
	assert.Contains(t, response.Message, "upstream exploded")
}

// TestGetNodeTypeOptionsPostForm asserts the POST variant works and
// forwards the current parameter values to the provider.
func TestGetNodeTypeOptionsPostForm(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	var capturedParams map[string]interface{}
	RegisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction,
		func(ctx context.Context, req NodeTypeOptionRequest) ([]NodeTypeOption, error) {
			capturedParams = req.Params
			return []NodeTypeOption{{Name: "One", Value: "one"}}, nil
		})
	t.Cleanup(func() {
		UnregisterOptionProvider(optionsTestNodeType, "resource", optionsTestAction)
	})

	body := strings.NewReader(`{"property":"resource","params":{"mode":"list"}}`)
	req := httptest.NewRequest("POST", "/api/v1/node-types/"+optionsTestNodeType+"/options", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	require.Len(t, response.Data, 1)
	assert.Equal(t, "One", response.Data[0].Name)
	require.NotNil(t, capturedParams)
	assert.Equal(t, "list", capturedParams["mode"])
}

// TestGetNodeTypeOptionsMalformedBody asserts a 400 for a malformed
// POST body.
func TestGetNodeTypeOptionsMalformedBody(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("POST", "/api/v1/node-types/"+optionsTestNodeType+"/options", strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["error"])
	assert.Equal(t, float64(http.StatusBadRequest), envelope["code"])
}

// TestGetNodeTypeOptionsEmptyBody asserts a 400 for an empty POST body.
func TestGetNodeTypeOptionsEmptyBody(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	req := httptest.NewRequest("POST", "/api/v1/node-types/"+optionsTestNodeType+"/options", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestNodeTypeOptionsRouteNotShadowedByNodeTypeRoute asserts the
// route ordering: "/node-types/{name}/options" is served by the options
// handler and "/node-types/{name}" still serves the node-type lookup.
func TestNodeTypeOptionsRouteNotShadowedByNodeTypeRoute(t *testing.T) {
	router := newNodeTypeOptionsTestServer(t)

	// The options route must not be swallowed by "/node-types/{name}".
	req := httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType+"/options?property=resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	response := decodeNodeTypeOptionsResponse(t, w.Body.Bytes())
	require.NotNil(t, response.Data)
	assert.Empty(t, response.Data)
	assert.NotEmpty(t, response.Message)

	// The node-type lookup keeps working.
	req = httptest.NewRequest("GET", "/api/v1/node-types/"+optionsTestNodeType, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	assert.Equal(t, optionsTestNodeType, entry["name"])
	assert.Contains(t, entry, "properties")
}
