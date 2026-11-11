package base

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neul-labs/m9m/internal/model"
)

func TestBaseNodeCreation(t *testing.T) {
	description := NodeDescription{
		Name:        "Test Node",
		Description: "A test node",
		Category:    "Test",
	}

	node := NewBaseNode(description)
	if node == nil {
		t.Fatal("Expected node to be created, got nil")
	}

	desc := node.Description()
	if desc.Name != description.Name {
		t.Errorf("Expected name %s, got %s", description.Name, desc.Name)
	}
}

func TestBaseNodeParameterHelpers(t *testing.T) {
	description := NodeDescription{
		Name:        "Test Node",
		Description: "A test node",
		Category:    "Test",
	}

	node := NewBaseNode(description)

	params := map[string]interface{}{
		"stringParam": "testValue",
		"intParam":    42,
		"boolParam":   true,
		"floatParam":  3.14,
	}

	// Test GetStringParameter
	strValue := node.GetStringParameter(params, "stringParam", "default")
	if strValue != "testValue" {
		t.Errorf("Expected 'testValue', got '%s'", strValue)
	}

	strDefault := node.GetStringParameter(params, "missingParam", "default")
	if strDefault != "default" {
		t.Errorf("Expected 'default', got '%s'", strDefault)
	}

	// Test GetIntParameter
	intValue := node.GetIntParameter(params, "intParam", 0)
	if intValue != 42 {
		t.Errorf("Expected 42, got %d", intValue)
	}

	intDefault := node.GetIntParameter(params, "missingParam", 100)
	if intDefault != 100 {
		t.Errorf("Expected 100, got %d", intDefault)
	}

	// Test GetIntParameter with float64 (common in JSON)
	intFromFloat := node.GetIntParameter(params, "floatParam", 0)
	if intFromFloat != 3 {
		t.Errorf("Expected 3, got %d", intFromFloat)
	}

	// Test GetBoolParameter
	boolValue := node.GetBoolParameter(params, "boolParam", false)
	if !boolValue {
		t.Errorf("Expected true, got %v", boolValue)
	}

	boolDefault := node.GetBoolParameter(params, "missingParam", true)
	if !boolDefault {
		t.Errorf("Expected true, got %v", boolDefault)
	}

	// Test with nil params
	nilStr := node.GetStringParameter(nil, "anyParam", "nilDefault")
	if nilStr != "nilDefault" {
		t.Errorf("Expected 'nilDefault', got '%s'", nilStr)
	}
}

func TestBaseNodeValidateParameters(t *testing.T) {
	description := NodeDescription{
		Name:        "Test Node",
		Description: "A test node",
		Category:    "Test",
	}

	node := NewBaseNode(description)

	// Test with nil params
	err := node.ValidateParameters(nil)
	if err != nil {
		t.Errorf("Expected no error with nil params, got %v", err)
	}

	// Test with empty params
	err = node.ValidateParameters(make(map[string]interface{}))
	if err != nil {
		t.Errorf("Expected no error with empty params, got %v", err)
	}

	// Test with valid params
	params := map[string]interface{}{
		"param1": "value1",
		"param2": 42,
	}

	err = node.ValidateParameters(params)
	if err != nil {
		t.Errorf("Expected no error with valid params, got %v", err)
	}
}

func TestBaseNodeExecute(t *testing.T) {
	description := NodeDescription{
		Name:        "Test Node",
		Description: "A test node",
		Category:    "Test",
	}

	node := NewBaseNode(description)

	inputData := []model.DataItem{
		{
			JSON: map[string]interface{}{
				"test": "data",
			},
		},
	}

	nodeParams := map[string]interface{}{
		"nodeParam": "value",
	}

	outputData, err := node.Execute(inputData, nodeParams)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(outputData) != len(inputData) {
		t.Errorf("Expected %d data items, got %d", len(inputData), len(outputData))
	}
}

func TestBaseNodeCreateError(t *testing.T) {
	description := NodeDescription{
		Name:        "Test Node",
		Description: "A test node",
		Category:    "Test",
	}

	node := NewBaseNode(description)

	err := node.CreateError("test error", nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "node Test Node error: test error" {
		t.Errorf("Expected 'node Test Node error: test error', got '%s'", err.Error())
	}
}

// TestNodeDescriptionMetadataFields asserts the n8n-style metadata
// contract a metadata-driven parameter panel depends on: group,
// credentials, usableAsTool, icon, iconColor and the node's own
// version all survive JSON serialization.
func TestNodeDescriptionMetadataFields(t *testing.T) {
	description := NodeDescription{
		Name:         "HTTP Request",
		Description:  "Makes an HTTP request",
		Category:     "core",
		Version:      3,
		Group:        []string{"transform"},
		UsableAsTool: true,
		Icon:         "fa:globe",
		IconColor:    "#ff6d5a",
		Credentials: []NodeCredentialSpec{
			{Type: "httpBasicAuth", DisplayName: "Basic Auth", Required: true},
			{Type: "httpHeaderAuth", DisplayName: "Header Auth", Required: false},
		},
	}

	data, err := json.Marshal(description)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "HTTP Request", decoded["name"])
	assert.Equal(t, "Makes an HTTP request", decoded["description"])
	assert.Equal(t, "core", decoded["category"])
	assert.Equal(t, float64(3), decoded["version"])
	assert.Equal(t, []interface{}{"transform"}, decoded["group"])
	assert.Equal(t, true, decoded["usableAsTool"])
	assert.Equal(t, "fa:globe", decoded["icon"])
	assert.Equal(t, "#ff6d5a", decoded["iconColor"])

	credentials, ok := decoded["credentials"].([]interface{})
	require.True(t, ok)
	require.Len(t, credentials, 2)

	first, ok := credentials[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "httpBasicAuth", first["type"])
	assert.Equal(t, "Basic Auth", first["displayName"])
	assert.Equal(t, true, first["required"])

	second, ok := credentials[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "httpHeaderAuth", second["type"])
	assert.Equal(t, "Header Auth", second["displayName"])
	assert.Equal(t, false, second["required"])
}

// TestNodeDescriptionOmitsUnsetMetadata keeps the wire format
// backward compatible: nodes that never declare the new metadata must
// not grow empty keys in the JSON payload.
func TestNodeDescriptionOmitsUnsetMetadata(t *testing.T) {
	description := NodeDescription{
		Name:        "Plain Node",
		Description: "No metadata",
		Category:    "utility",
	}

	data, err := json.Marshal(description)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	for _, key := range []string{"group", "credentials", "usableAsTool", "icon", "iconColor", "version"} {
		assert.NotContains(t, decoded, key)
	}
}

// TestOptionCarriesActionAndDescription covers the n8n loadOptions
// semantics: an option may name a backend action the frontend calls
// to fetch dynamic choices, plus a per-option description.
func TestOptionCarriesActionAndDescription(t *testing.T) {
	option := Option{
		Name:        "Project",
		Value:       "project",
		Action:      "loadProjects",
		Description: "Pick a project",
	}

	data, err := json.Marshal(option)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "Project", decoded["name"])
	assert.Equal(t, "project", decoded["value"])
	assert.Equal(t, "loadProjects", decoded["action"])
	assert.Equal(t, "Pick a project", decoded["description"])
}

// TestOptionOmitsUnsetActionAndDescription keeps the legacy option
// payload byte-identical for nodes that declare no dynamic action.
func TestOptionOmitsUnsetActionAndDescription(t *testing.T) {
	data, err := json.Marshal(Option{Name: "GET", Value: "GET"})
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "GET", decoded["name"])
	assert.Equal(t, "GET", decoded["value"])
	assert.NotContains(t, decoded, "action")
	assert.NotContains(t, decoded, "description")
}
