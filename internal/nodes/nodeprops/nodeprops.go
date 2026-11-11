/*
Package nodeprops provides constructors for node parameter metadata
(base.NodeProperty).

Every node in m9m ships a NodeDescription.Properties slice that the
API exposes via GET /api/v1/node-types and that the web parameter panel
renders. The helpers in this package exist so those slices are:

  - truthful — the parameter name is exactly the key the node's
    Execute implementation reads;
  - complete — every property carries a DisplayName, a Description, a
    non-nil Default and a Required flag;
  - renderable — free-form fields do NOT carry a synthetic
    placeholder entry in Options (a placeholder in the option list
    makes the frontend render a bogus first select entry), and the
    Placeholder field is used for the input hint instead;
  - conditional — the Show / Hide / ShowBool helpers build the n8n
    `{show: {param: [values]}}` / `{hide: {param: [values]}}` maps the
    frontend evaluates when it filters the visible properties.

The builders deliberately do not live in internal/nodes/base because
that package is owned by another worker and its builders cannot be
changed from here.
*/
package nodeprops

import "github.com/neul-labs/m9m/internal/nodes/base"

// Opt builds a property with a fixed choice set (rendered as a select).
func Opt(displayName, name, defaultValue, description string, options []base.Option, required bool) base.NodeProperty {
	if options == nil {
		options = []base.Option{}
	}
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "options",
		Default:     defaultValue,
		Description: description,
		Required:    required,
		Options:     options,
	}
}

// Str builds a free-form string property. No Options are attached —
// the value is typed by the user, not picked from a list.
func Str(displayName, name, defaultValue, description, placeholder string, required bool) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "string",
		Default:     defaultValue,
		Description: description,
		Required:    required,
		Placeholder: placeholder,
	}
}

// Secret builds a free-form string property whose value is masked in
// the UI. Use it for passwords, tokens and API keys.
func Secret(displayName, name, defaultValue, description, placeholder string, required bool) base.NodeProperty {
	p := Str(displayName, name, defaultValue, description, placeholder, required)
	p.TypeOptions = map[string]interface{}{
		"password": true,
	}
	return p
}

// Text builds a multi-line string property.
func Text(displayName, name, defaultValue, description, placeholder string, rows int) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "string",
		Default:     defaultValue,
		Description: description,
		Placeholder: placeholder,
		TypeOptions: map[string]interface{}{
			"rows": rows,
		},
	}
}

// Num builds a numeric property.
func Num(displayName, name string, defaultValue float64, description string, required bool) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "number",
		Default:     defaultValue,
		Description: description,
		Required:    required,
		TypeOptions: map[string]interface{}{
			"minValue": 0,
		},
	}
}

// NumRange builds a numeric property with an explicit value range.
// Use it when the default sits outside the normal 0…∞ window (e.g.
// an inclusive list index whose default is -1).
func NumRange(displayName, name string, defaultValue, minValue, maxValue float64, description string, required bool) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "number",
		Default:     defaultValue,
		Description: description,
		Required:    required,
		TypeOptions: map[string]interface{}{
			"minValue": minValue,
			"maxValue": maxValue,
		},
	}
}

// Bool builds a boolean toggle.
func Bool(displayName, name string, defaultValue bool, description string) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "boolean",
		Default:     defaultValue,
		Description: description,
	}
}

// JSON builds a JSON property (rendered as a JSON editor).
func JSON(displayName, name, defaultValue, description string) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "json",
		Default:     defaultValue,
		Description: description,
	}
}

// Collection builds a repeatable collection property. `options` lists
// the value types a row may hold (mirrors n8n's per-row valueOptions).
func Collection(displayName, name, description string, options []base.Option) base.NodeProperty {
	if options == nil {
		options = []base.Option{}
	}
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "collection",
		Default:     []interface{}{},
		Description: description,
		Options:     options,
		TypeOptions: map[string]interface{}{
			"multipleValues": true,
		},
	}
}

// FixedCollection builds a fixedCollection property — named buckets of
// sub-properties (n8n's "Send Query Parameters" / "Send Body" groups).
func FixedCollection(displayName, name, description string, buckets ...base.Option) base.NodeProperty {
	return base.NodeProperty{
		DisplayName: displayName,
		Name:        name,
		Type:        "fixedCollection",
		Default:     map[string]interface{}{},
		Description: description,
		Options:     buckets,
	}
}

// Required marks a property as required. The builders that mirror a
// free-form field (Text, Str, JSON) have no required argument because
// n8n treats those as "not required by default" — wrapping them in
// Required keeps the call sites readable:
//
//	nodeprops.Required(nodeprops.Text("Code", "code", "", "…", "", 10))
func Required(p base.NodeProperty) base.NodeProperty {
	p.Required = true
	return p
}

// Show builds the n8n `{show: {param: [values]}}` display rule: the
// property is visible only while the sibling parameter `param` holds
// one of `values`.
func Show(param string, values ...string) map[string]interface{} {
	if values == nil {
		values = []string{}
	}
	return map[string]interface{}{
		"show": map[string]interface{}{
			param: values,
		},
	}
}

// Hide builds the n8n `{hide: {param: [values]}}` display rule: the
// property is hidden while the sibling parameter `param` holds one of
// `values`.
func Hide(param string, values ...string) map[string]interface{} {
	if values == nil {
		values = []string{}
	}
	return map[string]interface{}{
		"hide": map[string]interface{}{
			param: values,
		},
	}
}

// ShowBool builds a display rule driven by a boolean sibling
// parameter. The frontend compares the parameter value with strict
// equality, so the allowed value must be a real boolean and not the
// string "true".
func ShowBool(param string, value bool) map[string]interface{} {
	return map[string]interface{}{
		"show": map[string]interface{}{
			param: []bool{value},
		},
	}
}

// Settings returns the property descriptors for the n8n "Settings"
// tab (notes, retryOnFail, maxTries, waitBetweenTries,
// alwaysOutputData, continueOnFail, onError).
//
// It replaces base.CommonSettings() inside the node packages because
// the base builder attaches a placeholder entry to the free-form
// "notes" field (which makes the frontend render it as a select) and
// because the retry knobs are only meaningful while retryOnFail is
// enabled — that conditional is expressed here as a display rule.
func Settings() []base.NodeProperty {
	return []base.NodeProperty{
		Text("Notes", "notes", "", "Optional notes shown next to the node on the canvas.", "Add notes…", 4),
		Bool("Display note in flow?", "notesInFlow", false, "If enabled, the node's notes render on the canvas directly below the node."),
		Bool("Retry on fail", "retryOnFail", false, "If enabled, the node retries automatically when it fails."),
		Num("Max tries", "maxTries", 3, "Maximum number of attempts when retryOnFail is enabled.", false).
			WithDisplayOptions(ShowBool("retryOnFail", true)),
		Num("Wait between tries (ms)", "waitBetweenTries", 1000, "Delay between retries in milliseconds.", false).
			WithDisplayOptions(ShowBool("retryOnFail", true)),
		Bool("Always output data", "alwaysOutputData", false, "If enabled, the node emits an empty item even when its input was empty."),
		Bool("Continue on fail", "continueOnFail", false, "If enabled, the workflow continues even when this node fails."),
		Opt("On error", "onError", "stopWorkflow", "What the workflow does when this node errors.", []base.Option{
			{Name: "Stop workflow", Value: "stopWorkflow"},
			{Name: "Continue (regular output)", Value: "continueRegularOutput"},
			{Name: "Continue (error output)", Value: "continueErrorOutput"},
		}, false),
	}
}
