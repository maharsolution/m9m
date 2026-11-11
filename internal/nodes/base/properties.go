package base

// Property descriptors for the n8n core nodes. These mirror the
// shape n8n ships in `INodeTypeDescription.properties` so the
// m9m UI can render the same Parameters tab the n8n Node Details
// View (NDV) renders.
//
// Each builder returns a slice of NodeProperty, ready to be
// assigned to NodeDescription.Properties. The helpers below are
// deliberately short and named after the parameter they describe
// (e.g. URL(), Method(), JsonBody()) so adding new properties to a
// node reads as English:
//
//   description := base.NodeDescription{
//     Name:       "HTTP Request",
//     Properties: base.StdRequestProperties(),  // method/url/etc
//   }
//
// A handful of cross-cutting property types repeat across many
// nodes (retry on fail, continue on fail, etc). Those live in
// CommonSettings() so adding the n8n "Settings" tab to a node is a
// one-liner.

// StringOpt builds a string property with a fixed set of options.
// Used heavily by HTTP method, IF operator, Set assignment type, etc.
func StringOpt(displayName, name, defaultVal, desc string, options []Option, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "options",
		Default:     defaultVal,
		Description: desc,
		Required:    required,
		Options:     options,
	}
}

// StringProp builds a free-form string property.
func StringProp(displayName, name, defaultVal, desc, placeholder string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "string",
		Default:     defaultVal,
		Description: desc,
		Required:    required,
		Options: []Option{
			{Name: placeholder, Value: ""},
		},
	}
}

// TextProp builds a multi-line string property.
func TextProp(displayName, name, defaultVal, desc, placeholder string, rows int) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "string",
		Default:     defaultVal,
		Description: desc,
		TypeOptions: map[string]interface{}{
			"rows": rows,
		},
		Options: []Option{
			{Name: placeholder, Value: ""},
		},
	}
}

// NumberProp builds an integer property.
func NumberProp(displayName, name string, defaultVal float64, desc string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "number",
		Default:     defaultVal,
		Description: desc,
		Required:    required,
		TypeOptions: map[string]interface{}{
			"minValue": 0,
		},
	}
}

// BoolProp builds a boolean toggle.
func BoolProp(displayName, name string, defaultVal bool, desc string) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "boolean",
		Default:     defaultVal,
		Description: desc,
	}
}

// JsonProp builds a JSON property. The UI renders this as a JSON
// editor with syntax highlighting.
func JsonProp(displayName, name, defaultVal, desc string) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "json",
		Default:     defaultVal,
		Description: desc,
	}
}

// CollectionProp builds a collection property (repeating rows of
// sub-properties). Used by IF conditions, headerParameters, query
// parameters, etc.
func CollectionProp(displayName, name, desc string, options []Option) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "collection",
		Description: desc,
		Options:     options,
		TypeOptions: map[string]interface{}{
			"multipleValues": true,
		},
	}
}

// FixedCollectionProp builds a fixedCollection property (named
// buckets of sub-properties). Used by HTTP Request's "Send Query
// Parameters" / "Send Headers" / "Send Body" groups.
func FixedCollectionProp(displayName, name, desc string) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "fixedCollection",
		Description: desc,
	}
}

// CommonSettings returns the property descriptors for the n8n
// "Settings" tab. Adding this slice to a node's Properties
// exposes: notes, retryOnFail, maxTries, waitBetweenTries,
// alwaysOutputData, continueOnFail, onError.
//
// Every core node in n8n supports these settings; m9m's engine
// already honours retryOnFail (ReliableWorkflowEngine), so this
// is the surface area the UI needs to start populating it.
func CommonSettings() []NodeProperty {
	return []NodeProperty{
		TextProp("Notes", "notes", "", "Optional notes shown next to the node on the canvas.", "Add notes…", 4),
		BoolProp("Display note in flow?", "notesInFlow", false, "If enabled, the node's notes render on the canvas directly below the node."),
		BoolProp("Retry on fail", "retryOnFail", false, "If enabled, the node retries automatically when it fails."),
		NumberProp("Max tries", "maxTries", 3, "Maximum number of attempts when retryOnFail is enabled.", false),
		NumberProp("Wait between tries (ms)", "waitBetweenTries", 1000, "Delay between retries in milliseconds.", false),
		BoolProp("Always output data", "alwaysOutputData", false, "If enabled, the node emits an empty item even when its input was empty."),
		BoolProp("Continue on fail", "continueOnFail", false, "If enabled, the workflow continues even when this node fails."),
		StringOpt(
			"On error", "onError", "stopWorkflow",
			"What the workflow does when this node errors.",
			[]Option{
				{Name: "Stop workflow", Value: "stopWorkflow"},
				{Name: "Continue (regular output)", Value: "continueRegularOutput"},
				{Name: "Continue (error output)", Value: "continueErrorOutput"},
			},
			false,
		),
	}
}

// ResourceMapperModes returns the mapping modes a `resourceMapper`
// property accepts.
//
// n8n calls this list `ResourceMapperMode`: "defineBelow" asks the
// user to wire the fields up by hand, "autoMapInputData" derives the
// mapping from the incoming items.
func ResourceMapperModes() []OptionValue {
	return []OptionValue{
		{Name: "Define Below", Value: "defineBelow", Description: "Map fields manually."},
		{Name: "Auto Map Input Data", Value: "autoMapInputData", Description: "Map fields automatically from the input data."},
	}
}

// FilterOperators returns the comparison operators a `filter`
// property accepts.
//
// Every id is the n8n operator key `"<type>:<operation>"` — the
// frontend splits it into the `type` / `operation` halves of the
// `operator` object n8n stores. Every operation listed here is
// implemented by m9m's condition evaluator
// (internal/nodes/transform/conditions.go): the numeric operators
// use n8n's short names (`gt`, `gte`, `lt`, `lte`), which the
// evaluator normalises to `greaterThan` / `greaterThanOrEqual` /
// `lessThan` / `lessThanOrEqual`.
//
// The dateTime operators are deliberately absent — m9m's evaluator
// does not implement "after" / "before".
func FilterOperators() []OptionValue {
	return []OptionValue{
		// string
		{Name: "Equals", Value: "string:equals"},
		{Name: "Does not equal", Value: "string:notEquals"},
		{Name: "Does not contain", Value: "string:notContains"},
		{Name: "Contains", Value: "string:contains"},
		{Name: "Starts with", Value: "string:startsWith"},
		{Name: "Ends with", Value: "string:endsWith"},
		{Name: "Matches regex", Value: "string:regex"},
		{Name: "Is empty", Value: "string:empty"},
		{Name: "Is not empty", Value: "string:notEmpty"},
		{Name: "Exists", Value: "string:exists"},
		{Name: "Does not exist", Value: "string:notExists"},
		// number
		{Name: "Equals", Value: "number:equals"},
		{Name: "Does not equal", Value: "number:notEquals"},
		{Name: "Greater than", Value: "number:gt"},
		{Name: "Greater than or equal", Value: "number:gte"},
		{Name: "Less than", Value: "number:lt"},
		{Name: "Less than or equal", Value: "number:lte"},
		// boolean
		{Name: "Equals", Value: "boolean:equals"},
		{Name: "Does not equal", Value: "boolean:notEquals"},
		{Name: "Is true", Value: "boolean:true"},
		{Name: "Is false", Value: "boolean:false"},
	}
}

// FileModes returns the modes a `file` property accepts.
//
// "binaryPropertyName" points at the binary property that carries the
// payload, "fileName" names the file explicitly.
func FileModes() []OptionValue {
	return []OptionValue{
		{Name: "Binary property name", Value: "binaryPropertyName"},
		{Name: "File name", Value: "fileName"},
	}
}

// IconValues returns the curated icon set an `icon` property offers.
//
// The names are lucide-style identifiers ("document-text", "cog-6-tooth")
// which the web UI maps onto Heroicons. The name equals the value so
// the list can be rendered straight from the metadata.
func IconValues() []OptionValue {
	names := []string{
		"user",
		"folder",
		"document-text",
		"calendar",
		"clock",
		"tag",
		"bolt",
		"cloud",
		"code-bracket",
		"globe-alt",
		"heart",
		"star",
		"trash",
		"cog-6-tooth",
		"link",
		"photo",
		"shield-check",
		"bell",
		"circle-stack",
		"table-cells",
	}
	values := make([]OptionValue, 0, len(names))
	for _, name := range names {
		values = append(values, OptionValue{Name: name, Value: name})
	}
	return values
}

// ResourceMapperProp builds a `resourceMapper` property — n8n's
// field-to-field mapping widget.
//
// The stored value is n8n's `ResourceMapperValue`:
//
//	{
//	  "mappingMode": "defineBelow",
//	  "value": { "<destination>": "<expression>" },
//	  "matchingColumns": [],
//	  "schema": [ { "id": ..., "displayName": ..., "defaultMatch": false,
//	                "required": false, "display": true } ],
//	  "attemptToConvertTypes": false,
//	  "convertFieldsToString": false
//	}
func ResourceMapperProp(displayName, name, desc string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "resourceMapper",
		Default: map[string]interface{}{
			"mappingMode":           "defineBelow",
			"value":                 map[string]interface{}{},
			"matchingColumns":       []interface{}{},
			"schema":                []interface{}{},
			"attemptToConvertTypes": false,
			"convertFieldsToString": false,
		},
		Description: desc,
		Required:    required,
		Values:      ResourceMapperModes(),
		TypeOptions: map[string]interface{}{
			"resourceMapper": map[string]interface{}{
				"mode": "map",
			},
		},
	}
}

// FilterProp builds a `filter` property — n8n's conditional filter
// widget.
//
// The stored value is n8n's `FilterValue`: an `options` block, the
// list of `conditions` and the `combinator` that joins them.
func FilterProp(displayName, name, desc string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "filter",
		Default: map[string]interface{}{
			"options": map[string]interface{}{
				"caseSensitive":  true,
				"leftValue":      "",
				"typeValidation": "strict",
				"version":        2,
			},
			"conditions": []interface{}{},
			"combinator": "and",
		},
		Description: desc,
		Required:    required,
		Values:      FilterOperators(),
		TypeOptions: map[string]interface{}{
			"filter": map[string]interface{}{
				"version":            2,
				"caseSensitive":      true,
				"leftValue":          "",
				"typeValidation":     "strict",
				"allowedCombinators": []string{"and", "or"},
			},
		},
	}
}

// FilterConditionProp builds the `collection` of condition rows a
// `filter` property is made of.
//
// The row fields (leftValue / operator / rightValue) are declared as
// plain options *without* an option-level `values` list on purpose:
// the collection renderer treats the first option that carries a
// `values` array as the child-field bucket, so hanging the operator
// list off it would break the row layout. The operator list lives on
// the property itself (NodeProperty.Values) instead.
func FilterConditionProp(displayName, name, desc string) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "collection",
		Default:     []interface{}{},
		Description: desc,
		Values:      FilterOperators(),
		TypeOptions: map[string]interface{}{
			"multipleValues": true,
		},
		Options: []Option{
			{Name: "Left Value", Value: "leftValue"},
			{Name: "Operator", Value: "operator"},
			{Name: "Right Value", Value: "rightValue"},
		},
	}
}

// FileProp builds a `file` property — n8n's binary-payload picker.
//
// The stored value is `{ "mode": <mode>, "value": <name> }`, where
// the mode is one of FileModes().
func FileProp(displayName, name, desc string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "file",
		Default: map[string]interface{}{
			"mode":  "binaryPropertyName",
			"value": "",
		},
		Description: desc,
		Required:    required,
		Values:      FileModes(),
		TypeOptions: map[string]interface{}{
			"binaryDataProperty": true,
		},
	}
}

// IconProp builds an `icon` property — n8n's icon picker.
//
// The stored value is n8n's `IconOrEmoji`: `{ "type": "icon",
// "value": "<icon name>" }`.
func IconProp(displayName, name, desc string, required bool) NodeProperty {
	return NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "icon",
		Default: map[string]interface{}{
			"type":  "icon",
			"value": "",
		},
		Description: desc,
		Required:    required,
		Values:      IconValues(),
	}
}
