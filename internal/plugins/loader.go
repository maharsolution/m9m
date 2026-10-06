// Package plugins implements dynamic node loading for m9m.
//
// A plugin is a JavaScript file that exports a `module.exports`
// object with at minimum `description` and `execute`. Plugins are
// executed inside a Goja runtime, so they can use any standard JS
// (no `require()` for npm modules — the plugin file is the unit of
// code). The contract mirrors n8n's `INodeType` shape so a node
// authored against n8n's docs can be ported by changing only the
// imports and the `nodeType` field.
//
// File layout — `plugins/my-node.js`:
//
//	module.exports = {
//	  description: {
//	    name: "My Custom Node",
//	    description: "Does something useful.",
//	    category: "custom",
//	    properties: [
//	      {displayName: "URL", name: "url", type: "string", default: "", required: true},
//	    ],
//	    inputs:  ["main"],
//	    outputs: ["main"],
//	  },
//	  execute: function(inputData, nodeParams) {
//	    return inputData.map(function(item) {
//	      return { json: { ...item.json, processed: nodeParams.url } };
//	    });
//	  },
//	};
//
// The plugin is registered with the engine under a `nodeType` of
// the file's basename (without the `.js` extension), e.g.:
// `m9m node list` will show `n8n-nodes-base.my-node` (or whatever
// the loader is configured to map to). Use the optional
// `description.nodeType` field to override the default.
package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/neul-labs/m9m/internal/nodes/base"
)

// Plugin is the resolved shape of a loaded plugin file. The
// `description` mirrors n8n's `INodeTypeDescription` and is what
// gets surfaced through `GET /api/v1/node-types` so the UI can
// render the Parameters tab without any Go-side changes.
type Plugin struct {
	FilePath     string
	NodeType     string
	Description  base.NodeDescription
	Source       string
	LoadedAt     time.Time
	VM           *goja.Runtime
	ExecuteFn    goja.Callable
	ValidateFn   goja.Callable
	rawExports   map[string]interface{}
}

// Load reads `path`, runs the script inside a Goja runtime, and
// returns the resolved plugin. Returns an error when the file is
// missing, the script throws, or the required exports are absent.
//
// The returned Plugin is safe to use from multiple goroutines for
// reading the description; the VM itself is single-threaded (Goja
// is not re-entrant), so the wrapper serialises `execute` calls.
func Load(path string) (*Plugin, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: resolve %s: %w", path, err)
	}
	code, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", abs, err)
	}

	vm := goja.New()
	// Sandbox: disable `eval` and the `Function` constructor so a
	// malicious plugin can't escape the runtime by constructing
	// closures from source strings.
	_ = vm.Set("eval", goja.Undefined())
	_ = vm.Set("Function", goja.Undefined())

	// Provide a minimal `module` object so the n8n-style export
	// shape works. Plugins typically write
	// `module.exports = { ... }`; honour that.
	moduleObj := vm.NewObject()
	_ = moduleObj.Set("exports", vm.NewObject())
	_ = vm.Set("module", moduleObj)

	// Run the script with a hard timeout. The Interrupt channel
	// flips the VM out of execution after 30s — long enough for
	// any reasonable plugin init, short enough that a runaway
	// loop won't lock the engine on startup.
	timeout := 30 * time.Second
	interrupt := make(chan func(), 1)
	vm.Interrupt = interrupt
	go func() {
		time.Sleep(timeout)
		interrupt <- func() { panic("plugin: load timeout") }
	}()
	if _, err := vm.RunString(string(code)); err != nil {
		close(interrupt)
		return nil, fmt.Errorf("plugin: run %s: %w", abs, err)
	}
	close(interrupt)

	exports := moduleObj.Get("exports")
	if goja.IsUndefined(exports) || goja.IsNull(exports) {
		return nil, fmt.Errorf("plugin: %s did not assign module.exports", abs)
	}
	expObj := exports.ToObject(vm)
	rawExports := map[string]interface{}{}
	for _, k := range expObj.Keys() {
		rawExports[k] = expObj.Get(k).Export()
	}

	// Resolve the description block. Accept either
	// `module.exports = { description: {...} }` or
	// `module.exports = function() { this.description = ... }`.
	descMap, err := readDescriptionMap(rawExports, vm)
	if err != nil {
		return nil, fmt.Errorf("plugin: %s: %w", abs, err)
	}
	description, err := parseDescription(descMap)
	if err != nil {
		return nil, fmt.Errorf("plugin: %s description: %w", abs, err)
	}

	// `nodeType` defaults to the file basename without `.js`. The
	// plugin can override it via `description.nodeType`.
	nodeType, _ := descMap["nodeType"].(string)
	if nodeType == "" {
		base := filepath.Base(abs)
		nodeType = strings.TrimSuffix(base, filepath.Ext(base))
		// The n8n-style `n8n-nodes-base.<service>` identifier is
		// the convention the engine expects; prefix bare names so
		// community nodes look like the rest of the catalog.
		if !strings.Contains(nodeType, ".") {
			nodeType = "n8n-nodes-base." + nodeType
		}
	}
	description.Name = nodeType

	// `execute` is required; the registry refuses to register a
	// plugin without one. `validateParameters` is optional.
	execVal := expObj.Get("execute")
	if goja.IsUndefined(execVal) || goja.IsNull(execVal) {
		return nil, fmt.Errorf("plugin: %s: missing `execute` function", abs)
	}
	execCallable, ok := goja.AssertFunction(execVal)
	if !ok {
		return nil, fmt.Errorf("plugin: %s: `execute` must be a function", abs)
	}
	var validateCallable goja.Callable
	if v := expObj.Get("validateParameters"); !goja.IsUndefined(v) && !goja.IsNull(v) {
		if c, ok := goja.AssertFunction(v); ok {
			validateCallable = c
		}
	}

	return &Plugin{
		FilePath:   abs,
		NodeType:   nodeType,
		Description: description,
		Source:     string(code),
		LoadedAt:   time.Now(),
		VM:         vm,
		ExecuteFn:  execCallable,
		ValidateFn: validateCallable,
		rawExports: rawExports,
	}, nil
}

// readDescriptionMap extracts the description from either an object
// exports or a constructor-style exports. The latter is a minor
// concession to n8n's `class MyNode { description = {...} }` shape.
func readDescriptionMap(rawExports map[string]interface{}, vm *goja.Runtime) (map[string]interface{}, error) {
	if d, ok := rawExports["description"].(map[string]interface{}); ok {
		return d, nil
	}
	// Fallback: maybe description is on a `default` export.
	if d, ok := rawExports["default"].(map[string]interface{}); ok {
		if nested, ok := d["description"].(map[string]interface{}); ok {
			return nested, nil
		}
	}
	return nil, fmt.Errorf("missing `description` object")
}

// parseDescription normalises the JS description shape into the
// m9m `base.NodeDescription` struct. Unknown keys are ignored; a
// missing `name` is fine because the loader fills it from the
// file's basename via `description.Name = nodeType`.
func parseDescription(raw map[string]interface{}) (base.NodeDescription, error) {
	out := base.NodeDescription{
		Name:        stringOf(raw["name"]),
		Description: stringOf(raw["description"]),
		Category:    stringOf(raw["category"]),
	}
	if out.Category == "" {
		out.Category = "Community"
	}
	if v, ok := raw["properties"].([]interface{}); ok {
		for _, p := range v {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			np := base.NodeProperty{
				DisplayName: stringOf(pm["displayName"]),
				Name:        stringOf(pm["name"]),
				Type:        stringOf(pm["type"]),
				Default:     pm["default"],
				Description: stringOf(pm["description"]),
				Required:    boolOf(pm["required"]),
				Placeholder: stringOf(pm["placeholder"]),
			}
			if v, ok := pm["options"].([]interface{}); ok {
				for _, o := range v {
					om, ok := o.(map[string]interface{})
					if !ok {
						continue
					}
					np.Options = append(np.Options, base.Option{
						Name:  stringOf(om["name"]),
						Value: stringOf(om["value"]),
					})
				}
			}
			out.Properties = append(out.Properties, np)
		}
	}
	if v, ok := raw["inputs"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				out.Inputs = append(out.Inputs, str)
			}
		}
	}
	if out.Inputs == nil {
		out.Inputs = []string{"main"}
	}
	if v, ok := raw["outputs"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				out.Outputs = append(out.Outputs, str)
			}
		}
	}
	if out.Outputs == nil {
		out.Outputs = []string{"main"}
	}
	return out, nil
}

func stringOf(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolOf(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
