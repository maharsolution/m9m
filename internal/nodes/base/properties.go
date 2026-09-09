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
