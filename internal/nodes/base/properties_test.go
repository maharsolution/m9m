package base

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// valuesOf maps a value list to its ids so the assertions read like
// the metadata they describe.
func valuesOf(values []OptionValue) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.Value)
	}
	return ids
}

func TestResourceMapperModes(t *testing.T) {
	modes := ResourceMapperModes()

	require.Len(t, modes, 2)
	assert.Equal(t, "Define Below", modes[0].Name)
	assert.Equal(t, "defineBelow", modes[0].Value)
	assert.Equal(t, "Map fields manually.", modes[0].Description)
	assert.Equal(t, "Auto Map Input Data", modes[1].Name)
	assert.Equal(t, "autoMapInputData", modes[1].Value)
	assert.Equal(t, "Map fields automatically from the input data.", modes[1].Description)
}

func TestFilterOperators(t *testing.T) {
	operators := FilterOperators()
	require.NotEmpty(t, operators)

	ids := valuesOf(operators)

	// Every operator id is the n8n "<type>:<operation>" key.
	for _, operator := range operators {
		assert.Contains(t, operator.Value, ":", "operator %q must be typed", operator.Value)
		parts := strings.SplitN(operator.Value, ":", 2)
		require.Len(t, parts, 2)
		assert.NotEmpty(t, parts[0], "operator %q must declare a type", operator.Value)
		assert.NotEmpty(t, parts[1], "operator %q must declare an operation", operator.Value)
	}

	// The operators m9m's condition evaluator implements.
	assert.Contains(t, ids, "string:equals")
	assert.Contains(t, ids, "string:contains")
	assert.Contains(t, ids, "string:empty")
	assert.Contains(t, ids, "number:gt")
	assert.Contains(t, ids, "boolean:true")

	// n8n's real ids are "empty" / "notEmpty" — the invented
	// "isEmpty" / "isNotEmpty" names must never show up.
	for _, id := range ids {
		assert.NotContains(t, id, "isEmpty")
		assert.NotContains(t, id, "isNotEmpty")
	}

	// No dateTime operators — m9m's evaluator has no after/before.
	for _, id := range ids {
		assert.False(t, strings.HasPrefix(id, "dateTime:"), "unexpected dateTime operator %q", id)
	}
}

func TestFileModes(t *testing.T) {
	modes := FileModes()

	require.Len(t, modes, 2)
	assert.Equal(t, "Binary property name", modes[0].Name)
	assert.Equal(t, "binaryPropertyName", modes[0].Value)
	assert.Equal(t, "File name", modes[1].Name)
	assert.Equal(t, "fileName", modes[1].Value)
}

func TestIconValues(t *testing.T) {
	icons := IconValues()

	require.NotEmpty(t, icons)
	for _, icon := range icons {
		assert.Equal(t, icon.Value, icon.Name, "icon name should equal its value")
	}

	ids := valuesOf(icons)
	for _, expected := range []string{
		"user", "folder", "document-text", "calendar", "clock", "tag",
		"bolt", "cloud", "code-bracket", "globe-alt", "heart", "star",
		"trash", "cog-6-tooth", "link", "photo", "shield-check", "bell",
		"circle-stack", "table-cells",
	} {
		assert.Contains(t, ids, expected)
	}
}

func TestResourceMapperProp(t *testing.T) {
	prop := ResourceMapperProp("Field Mapping", "fieldMapping", "Map the input fields.", true)

	assert.Equal(t, "resourceMapper", prop.Type)
	assert.Equal(t, "Field Mapping", prop.DisplayName)
	assert.Equal(t, "fieldMapping", prop.Name)
	assert.Equal(t, "Map the input fields.", prop.Description)
	assert.True(t, prop.Required)

	// Property-level values carry the mapping modes.
	assert.Equal(t, []string{"defineBelow", "autoMapInputData"}, valuesOf(prop.Values))

	// Type options mirror n8n's `resourceMapper` type options.
	typeOptions, ok := prop.TypeOptions["resourceMapper"].(map[string]interface{})
	require.True(t, ok, "expected a resourceMapper type option")
	assert.Equal(t, "map", typeOptions["mode"])

	// Default is n8n's `ResourceMapperValue` shape.
	def, ok := prop.Default.(map[string]interface{})
	require.True(t, ok, "expected a map default")
	assert.Equal(t, "defineBelow", def["mappingMode"])
	assert.Equal(t, map[string]interface{}{}, def["value"])
	assert.Equal(t, []interface{}{}, def["matchingColumns"])
	assert.Equal(t, []interface{}{}, def["schema"])
	assert.Equal(t, false, def["attemptToConvertTypes"])
	assert.Equal(t, false, def["convertFieldsToString"])
}

func TestFilterProp(t *testing.T) {
	prop := FilterProp("Conditions", "conditions", "Only keep matching items.", true)

	assert.Equal(t, "filter", prop.Type)
	assert.Equal(t, "Conditions", prop.DisplayName)
	assert.Equal(t, "conditions", prop.Name)
	assert.Equal(t, "Only keep matching items.", prop.Description)
	assert.True(t, prop.Required)

	// Property-level values carry the operator list.
	assert.NotEmpty(t, prop.Values)
	for _, value := range prop.Values {
		assert.Contains(t, value.Value, ":")
	}

	typeOptions, ok := prop.TypeOptions["filter"].(map[string]interface{})
	require.True(t, ok, "expected a filter type option")
	assert.Equal(t, 2, typeOptions["version"])
	assert.Equal(t, true, typeOptions["caseSensitive"])
	assert.Equal(t, "", typeOptions["leftValue"])
	assert.Equal(t, "strict", typeOptions["typeValidation"])
	assert.Equal(t, []string{"and", "or"}, typeOptions["allowedCombinators"])

	def, ok := prop.Default.(map[string]interface{})
	require.True(t, ok, "expected a map default")
	assert.Equal(t, "and", def["combinator"])
	assert.Equal(t, []interface{}{}, def["conditions"])

	options, ok := def["options"].(map[string]interface{})
	require.True(t, ok, "expected an options block")
	assert.Equal(t, true, options["caseSensitive"])
	assert.Equal(t, "", options["leftValue"])
	assert.Equal(t, "strict", options["typeValidation"])
	assert.Equal(t, 2, options["version"])
}

func TestFilterConditionProp(t *testing.T) {
	prop := FilterConditionProp("Conditions", "conditions", "The filter conditions.")

	assert.Equal(t, "collection", prop.Type)
	assert.Equal(t, "Conditions", prop.DisplayName)
	assert.Equal(t, "conditions", prop.Name)
	assert.Equal(t, "The filter conditions.", prop.Description)
	assert.False(t, prop.Required)
	assert.Equal(t, []interface{}{}, prop.Default)
	assert.Equal(t, true, prop.TypeOptions["multipleValues"])

	// The operator list lives on the property itself.
	assert.NotEmpty(t, prop.Values)
	assert.Contains(t, valuesOf(prop.Values), "string:equals")

	// Regression guard: the collection renderer treats the first
	// option that carries a `values` array as the child-field
	// bucket, so no option may declare one.
	require.Len(t, prop.Options, 3)
	for _, option := range prop.Options {
		assert.Empty(t, option.Values, "option %q must not declare values", option.Value)
	}
	assert.Equal(t, "leftValue", prop.Options[0].Value)
	assert.Equal(t, "operator", prop.Options[1].Value)
	assert.Equal(t, "rightValue", prop.Options[2].Value)
}

func TestFileProp(t *testing.T) {
	prop := FileProp("Input File", "inputFile", "The file to read.", false)

	assert.Equal(t, "file", prop.Type)
	assert.Equal(t, "Input File", prop.DisplayName)
	assert.Equal(t, "inputFile", prop.Name)
	assert.Equal(t, "The file to read.", prop.Description)
	assert.False(t, prop.Required)
	assert.Equal(t, []string{"binaryPropertyName", "fileName"}, valuesOf(prop.Values))
	assert.Equal(t, true, prop.TypeOptions["binaryDataProperty"])

	def, ok := prop.Default.(map[string]interface{})
	require.True(t, ok, "expected a map default")
	assert.Equal(t, "binaryPropertyName", def["mode"])
	assert.Equal(t, "", def["value"])
}

func TestIconProp(t *testing.T) {
	prop := IconProp("Icon", "icon", "The icon to show.", false)

	assert.Equal(t, "icon", prop.Type)
	assert.Equal(t, "Icon", prop.DisplayName)
	assert.Equal(t, "icon", prop.Name)
	assert.Equal(t, "The icon to show.", prop.Description)
	assert.False(t, prop.Required)
	assert.NotEmpty(t, prop.Values)

	def, ok := prop.Default.(map[string]interface{})
	require.True(t, ok, "expected a map default")
	assert.Equal(t, "icon", def["type"])
	assert.Equal(t, "", def["value"])
}

func TestNodePropertyJSONRoundTrip(t *testing.T) {
	prop := FilterProp("Conditions", "conditions", "Only keep matching items.", true)
	prop.Options = []Option{
		{Name: "Left Value", Value: "leftValue"},
		{
			Name:  "Operator",
			Value: "operator",
			Values: []OptionValue{
				{Name: "Equals", Value: "string:equals"},
			},
		},
	}

	encoded, err := json.Marshal(prop)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &raw))

	values, ok := raw["values"].([]interface{})
	require.True(t, ok, "the property-level values must survive the round trip")
	assert.NotEmpty(t, values)

	options, ok := raw["options"].([]interface{})
	require.True(t, ok, "the options must survive the round trip")
	require.Len(t, options, 2)

	option, ok := options[1].(map[string]interface{})
	require.True(t, ok)
	optionValues, ok := option["values"].([]interface{})
	require.True(t, ok, "an option-level values list must survive the round trip")
	require.Len(t, optionValues, 1)
	entry, ok := optionValues[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string:equals", entry["value"])
	assert.Equal(t, "Equals", entry["name"])

	// Decoding back into the Go type keeps everything in place.
	var decoded NodeProperty
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, prop.Type, decoded.Type)
	assert.Equal(t, valuesOf(prop.Values), valuesOf(decoded.Values))
	require.Len(t, decoded.Options, 2)
	assert.Equal(t, []OptionValue{{Name: "Equals", Value: "string:equals"}}, decoded.Options[1].Values)
}
