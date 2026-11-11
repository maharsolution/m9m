package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// metadataNode is a minimal node executor that only carries a
// NodeDescription — enough to assert the engine forwards the node's
// declared metadata to API consumers.
type metadataNode struct {
	description base.NodeDescription
}

func (n *metadataNode) Description() base.NodeDescription { return n.description }

func (n *metadataNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	return inputData, nil
}

func (n *metadataNode) ValidateParameters(params map[string]interface{}) error { return nil }

// TestGetRegisteredNodeTypesCarriesNodeMetadata asserts the engine
// threads every new metadata field (and the node's real version)
// through to NodeTypeInfo.
func TestGetRegisteredNodeTypesCarriesNodeMetadata(t *testing.T) {
	eng := NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.httpRequest", &metadataNode{
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
					Name:        "url",
					DisplayName: "URL",
					Type:        "string",
					Description: "The URL to call",
					Required:    true,
					TypeOptions: map[string]interface{}{"rows": 2},
				},
			},
		},
	})

	registered := eng.GetRegisteredNodeTypes()
	require.Len(t, registered, 1)

	info := registered[0]
	assert.Equal(t, "n8n-nodes-base.httpRequest", info.TypeID)
	assert.Equal(t, "HTTP Request", info.DisplayName)
	assert.Equal(t, "core", info.Category)
	assert.Equal(t, 3, info.Version)
	assert.Equal(t, []string{"transform"}, info.Group)
	assert.True(t, info.UsableAsTool)
	assert.Equal(t, "fa:globe", info.Icon)
	assert.Equal(t, "#ff6d5a", info.IconColor)

	require.Len(t, info.Credentials, 1)
	assert.Equal(t, "httpBasicAuth", info.Credentials[0].Type)
	assert.Equal(t, "Basic Auth", info.Credentials[0].DisplayName)
	assert.True(t, info.Credentials[0].Required)

	require.Len(t, info.Properties, 1)
	assert.Equal(t, "url", info.Properties[0].Name)
	assert.Equal(t, map[string]interface{}{"rows": 2}, info.Properties[0].TypeOptions)
}

// TestGetRegisteredNodeTypesDefaultsVersionToOne keeps the previous
// behaviour for nodes that never declare a version — the API used to
// hard-code 1, so consumers must not see a change for those nodes.
func TestGetRegisteredNodeTypesDefaultsVersionToOne(t *testing.T) {
	eng := NewWorkflowEngine()
	eng.RegisterNodeExecutor("n8n-nodes-base.noOp", &metadataNode{
		description: base.NodeDescription{
			Name:     "No Op",
			Category: "utility",
		},
	})

	registered := eng.GetRegisteredNodeTypes()
	require.Len(t, registered, 1)

	assert.Equal(t, 1, registered[0].Version)
	assert.Empty(t, registered[0].Group)
	assert.Empty(t, registered[0].Credentials)
	assert.False(t, registered[0].UsableAsTool)
	assert.Empty(t, registered[0].Icon)
	assert.Empty(t, registered[0].IconColor)
}
