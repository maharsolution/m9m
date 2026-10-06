package plugins

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
)

// Wrapper adapts a JS plugin to m9m's `base.NodeExecutor`
// interface. It serialises `execute` calls because Goja's runtime
// is not safe for concurrent use — the engine can invoke the same
// node from multiple workflows (or from multiple items in the
// same workflow) in parallel, but a single VM must execute one
// function at a time.
type Wrapper struct {
	plugin *Plugin
	mu     sync.Mutex // serialises `Execute` and `ValidateParameters`
}

// Execute passes `inputData` and `nodeParams` to the plugin's
// `execute` JS function and converts the return value back into
// `[]model.DataItem`. The plugin may return either an array of
// items (`[{json: {...}}, ...]`) or a single object (which we
// wrap in a one-element slice).
//
// Conversion rules mirror the n8n `INodeExecutionData` shape:
//
//   - `[object, ...]`  → `[]DataItem{ {JSON: object}, ... }`
//   - `{json: ...}`    → unwrap to `DataItem{JSON: ...}` (one item)
//   - `[{json, binary, pairedItem}, ...]`  → preserve as-is
//   - `string|number|bool|null`            → wrap as `{"value": x}`
//   - `null`/`undefined`                   → empty slice
func (w *Wrapper) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.plugin == nil || w.plugin.VM == nil {
		return nil, fmt.Errorf("plugin: not loaded")
	}

	vm := w.plugin.VM

	// Marshal inputs to JS-friendly shapes. The expression
	// runtime already does this for workflow data, but plugins
	// are invoked with raw `DataItem` values and need explicit
	// conversion.
	jsInput := itemsToJS(vm, inputData)
	jsParams := mapToJS(vm, nodeParams)

	result, err := w.plugin.ExecuteFn(goja.Undefined(), jsInput, jsParams)
	if err != nil {
		// Surface the underlying JS exception as a Go error
		// without re-running it through the VM (which would
		// throw the same panic again on a subsequent call).
		return nil, fmt.Errorf("plugin %s execute: %v", w.plugin.NodeType, err)
	}
	if goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil
	}
	return jsToItems(result.Export())
}

// ValidateParameters delegates to the plugin's optional
// `validateParameters` function. When the plugin does not export
// one, we accept the call and return nil (matching the
// `BaseNode` default behaviour).
func (w *Wrapper) ValidateParameters(params map[string]interface{}) error {
	if w.plugin == nil || w.plugin.ValidateFn == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	vm := w.plugin.VM
	jsParams := mapToJS(vm, params)
	result, err := w.plugin.ValidateFn(goja.Undefined(), jsParams)
	if err != nil {
		return fmt.Errorf("plugin %s validate: %v", w.plugin.NodeType, err)
	}
	if goja.IsUndefined(result) || goja.IsNull(result) {
		return nil
	}
	if b, ok := result.Export().(bool); ok {
		if !b {
			return fmt.Errorf("plugin %s rejected parameters", w.plugin.NodeType)
		}
	}
	return nil
}

// Description returns the plugin's resolved `NodeDescription`.
// This is what `GET /api/v1/node-types` surfaces to the UI.
func (w *Wrapper) Description() base.NodeDescription {
	if w.plugin == nil {
		return base.NodeDescription{}
	}
	return w.plugin.Description
}

// itemsToJS converts a slice of `DataItem` to a Goja array of
// plain objects. Each item's `json` becomes a `json` field on the
// JS object; `binary` and `pairedItem` round-trip as-is when
// present.
func itemsToJS(vm *goja.Runtime, items []model.DataItem) goja.Value {
	arr := vm.NewArray()
	for i, item := range items {
		obj := vm.NewObject()
		_ = obj.Set("json", mapToJS(vm, item.JSON))
		if len(item.Binary) > 0 {
			_ = obj.Set("binary", mapToJS(vm, binaryToMap(item.Binary)))
		}
		if item.PairedItem != nil {
			_ = obj.Set("pairedItem", mapToJS(vm, pairedToMap(item.PairedItem)))
		}
		_ = arr.SetIdx(int64(i), obj)
	}
	return arr
}

// mapToJS recursively mirrors a Go map into a Goja object. Slices
// become Goja arrays, maps become objects, everything else is
// passed through `vm.ToValue`.
func mapToJS(vm *goja.Runtime, v interface{}) goja.Value {
	switch t := v.(type) {
	case map[string]interface{}:
		obj := vm.NewObject()
		for k, vv := range t {
			_ = obj.Set(k, mapToJS(vm, vv))
		}
		return obj
	case []interface{}:
		arr := vm.NewArray()
		for i, vv := range t {
			_ = arr.SetIdx(int64(i), mapToJS(vm, vv))
		}
		return arr
	case nil:
		return goja.Null()
	default:
		return vm.ToValue(t)
	}
}

// jsToItems converts the JS return value to `[]DataItem`.
// Accepts an array of item-shaped objects, a single object, or a
// primitive. See `Wrapper.Execute` for the full rules.
func jsToItems(v interface{}) ([]model.DataItem, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case []interface{}:
		out := make([]model.DataItem, 0, len(t))
		for _, el := range t {
			out = append(out, itemFromJS(el))
		}
		return out, nil
	case map[string]interface{}:
		return []model.DataItem{itemFromJS(t)}, nil
	default:
		return []model.DataItem{{
			JSON: map[string]interface{}{"value": t},
		}}, nil
	}
}

func itemFromJS(v interface{}) model.DataItem {
	m, ok := v.(map[string]interface{})
	if !ok {
		return model.DataItem{JSON: map[string]interface{}{"value": v}}
	}
	// Unwrap the n8n `INodeExecutionData` envelope: if the
	// object has only a `json` field, return that field's
	// contents directly. Otherwise preserve the full shape so
	// downstream code can read `binary` / `pairedItem`.
	if len(m) == 1 {
		if j, ok := m["json"].(map[string]interface{}); ok {
			return model.DataItem{JSON: j}
		}
	}
	jsonMap, _ := m["json"].(map[string]interface{})
	if jsonMap == nil {
		jsonMap = map[string]interface{}{}
	}
	item := model.DataItem{JSON: jsonMap}
	if b, ok := m["binary"].(map[string]interface{}); ok {
		item.Binary = mapFromBinary(b)
	}
	if p, ok := m["pairedItem"].(map[string]interface{}); ok {
		item.PairedItem = pairedFromMap(p)
	}
	return item
}

// binaryToMap flattens a `map[string]model.BinaryData` for
// transport across the JS boundary. We only carry the public
// metadata (filename, mime, size); the actual bytes stay on the Go
// side and are referenced by id.
func binaryToMap(b map[string]model.BinaryData) map[string]interface{} {
	out := make(map[string]interface{}, len(b))
	for k, v := range b {
		out[k] = map[string]interface{}{
			"fileName": v.FileName,
			"mimeType": v.MimeType,
			"id":       v.ID,
			"size":     len(v.Data),
		}
	}
	return out
}

func mapFromBinary(m map[string]interface{}) map[string]model.BinaryData {
	out := make(map[string]model.BinaryData, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			bd := model.BinaryData{}
			if s, ok := vm["fileName"].(string); ok {
				bd.FileName = s
			}
			if s, ok := vm["mimeType"].(string); ok {
				bd.MimeType = s
			}
			if s, ok := vm["id"].(string); ok {
				bd.ID = s
			}
			out[k] = bd
		}
	}
	return out
}

func pairedToMap(p *model.PairedItem) map[string]interface{} {
	if p == nil {
		return nil
	}
	return map[string]interface{}{
		"item":  p.Item,
		"input": p.Input,
	}
}

func pairedFromMap(m map[string]interface{}) *model.PairedItem {
	if m == nil {
		return nil
	}
	pi := &model.PairedItem{}
	if v, ok := m["item"].(int); ok {
		pi.Item = v
	}
	if v, ok := m["input"].(int); ok {
		pi.Input = v
	}
	return pi
}
