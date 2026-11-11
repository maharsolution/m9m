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
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/storage"
)

// metadataCatalogNode is a node executor that only carries metadata.
type metadataCatalogNode struct {
	description base.NodeDescription
}

func (n *metadataCatalogNode) Description() base.NodeDescription { return n.description }

func (n *metadataCatalogNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return inputData, nil
}

func (n *metadataCatalogNode) ValidateParameters(params map[string]interface{}) error { return nil }

// newNodeTypesTestServer wires a real engine with two registered node
// types: one that declares the full n8n-style metadata, one that
// declares none.
func newNodeTypesTestServer(t *testing.T) *mux.Router {
	t.Helper()

	eng := engine.NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.httpRequest", &metadataCatalogNode{
		description: base.NodeDescription{
			Name:         "HTTP Request",
			Description:  "Makes an HTTP request",
			Category:     "core",
			Version:      3,
			Group:        []string{"transform"},
			UsableAsTool: true,
			Icon:         "fa:globe",
			IconColor:    "#ff6d5a",
			Credentials: []base.NodeCredentialSpec{
				{Type: "httpBasicAuth", DisplayName: "Basic Auth", Required: true},
			},
			Properties: []base.NodeProperty{
				{
					Name:        "body",
					DisplayName: "Body",
					Type:        "string",
					Description: "Request body",
					Required:    false,
					TypeOptions: map[string]interface{}{"rows": 4},
					DisplayOptions: map[string]interface{}{
						"show": map[string]interface{}{"method": []string{"POST"}},
					},
					Options: []base.Option{
						{Name: "Raw", Value: "raw", Action: "loadContentTypes", Description: "Send a raw body"},
					},
				},
			},
		},
	})
	eng.RegisterNodeExecutor("n8n-nodes-base.noOp", &metadataCatalogNode{
		description: base.NodeDescription{
			Name:     "No Op",
			Category: "utility",
		},
	})

	server := NewAPIServer(eng, nil, storage.NewMemoryStorage())
	router := mux.NewRouter()
	server.RegisterRoutes(router)

	return router
}

// TestListNodeTypesIncludesExtendedMetadata asserts the
// /api/v1/node-types payload carries group, credentials, the node's
// real version and the icon metadata a parameter panel needs.
func TestListNodeTypesIncludesExtendedMetadata(t *testing.T) {
	router := newNodeTypesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 2)

	entries := map[string]map[string]interface{}{}
	for _, entry := range response {
		entries[entry["name"].(string)] = entry
	}

	httpRequest := entries["n8n-nodes-base.httpRequest"]
	require.NotNil(t, httpRequest)

	// Pre-existing keys keep their meaning.
	assert.Equal(t, "HTTP Request", httpRequest["displayName"])
	assert.Equal(t, "Makes an HTTP request", httpRequest["description"])
	assert.Equal(t, "core", httpRequest["category"])
	assert.Equal(t, []interface{}{"main"}, httpRequest["inputs"])
	assert.Equal(t, []interface{}{"main"}, httpRequest["outputs"])
	assert.Equal(t, map[string]interface{}{"name": "HTTP Request"}, httpRequest["defaults"])

	// New keys.
	assert.Equal(t, []interface{}{"transform"}, httpRequest["group"])
	assert.Equal(t, float64(3), httpRequest["version"])
	assert.Equal(t, "fa:globe", httpRequest["icon"])
	assert.Equal(t, "#ff6d5a", httpRequest["iconColor"])
	assert.Equal(t, true, httpRequest["usableAsTool"])

	credentials, ok := httpRequest["credentials"].([]interface{})
	require.True(t, ok)
	require.Len(t, credentials, 1)

	credential, ok := credentials[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "httpBasicAuth", credential["type"])
	assert.Equal(t, "Basic Auth", credential["displayName"])
	assert.Equal(t, true, credential["required"])

	// A node without declared metadata still reports empty arrays, not null.
	noOp := entries["n8n-nodes-base.noOp"]
	require.NotNil(t, noOp)
	assert.Equal(t, []interface{}{}, noOp["group"])
	assert.Equal(t, []interface{}{}, noOp["credentials"])
	assert.Equal(t, float64(1), noOp["version"])
}

// TestListNodeTypesPreservesPropertyMetadata asserts the property
// payload keeps typeOptions, displayOptions and the option action —
// the fields the parameter panel renders from.
func TestListNodeTypesPreservesPropertyMetadata(t *testing.T) {
	router := newNodeTypesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	var httpRequest map[string]interface{}
	for _, entry := range response {
		if entry["name"] == "n8n-nodes-base.httpRequest" {
			httpRequest = entry
		}
	}
	require.NotNil(t, httpRequest)

	properties, ok := httpRequest["properties"].([]interface{})
	require.True(t, ok)
	require.Len(t, properties, 1)

	property, ok := properties[0].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "body", property["name"])
	assert.Equal(t, "Body", property["displayName"])
	assert.Equal(t, "Request body", property["description"])
	assert.Equal(t, "string", property["type"])
	assert.Equal(t, false, property["required"])
	assert.Contains(t, property, "default")

	// typeOptions must survive serialization — the panel uses it to
	// render textareas (rows) and number bounds.
	typeOptions, ok := property["typeOptions"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(4), typeOptions["rows"])

	displayOptions, ok := property["displayOptions"].(map[string]interface{})
	require.True(t, ok)
	show, ok := displayOptions["show"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"POST"}, show["method"])

	options, ok := property["options"].([]interface{})
	require.True(t, ok)
	require.Len(t, options, 1)

	option, ok := options[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "loadContentTypes", option["action"])
	assert.Equal(t, "Send a raw body", option["description"])
}

// TestGetNodeTypeIncludesExtendedMetadata asserts the single node-type
// lookup exposes the same metadata as the list endpoint.
func TestGetNodeTypeIncludesExtendedMetadata(t *testing.T) {
	router := newNodeTypesTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/node-types/n8n-nodes-base.httpRequest", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "n8n-nodes-base.httpRequest", response["name"])
	assert.Equal(t, []interface{}{"transform"}, response["group"])
	assert.Equal(t, float64(3), response["version"])
	assert.Equal(t, "fa:globe", response["icon"])
	assert.Equal(t, "#ff6d5a", response["iconColor"])
	assert.Equal(t, true, response["usableAsTool"])

	credentials, ok := response["credentials"].([]interface{})
	require.True(t, ok)
	require.Len(t, credentials, 1)

	credential, ok := credentials[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "httpBasicAuth", credential["type"])
	assert.Equal(t, "Basic Auth", credential["displayName"])
	assert.Equal(t, true, credential["required"])

	properties, ok := response["properties"].([]interface{})
	require.True(t, ok)
	require.Len(t, properties, 1)

	property, ok := properties[0].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, property, "typeOptions")
}
