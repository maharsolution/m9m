package code

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// pythonCodeProperties describes the Python Code node.
//
// The executor (code.PythonCodeNode) reads exactly two parameters:
// `pythonCode` and `mode`. The mode values it accepts are "runOnce"
// and "runForEach" — the previously advertised
// "runOnceForAllItems" / "runOnceForEachItem" values were rejected by
// ValidateParameters, and `pythonVersion` / `inputItems` were never
// read at all.
func pythonCodeProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Opt("Mode", "mode", "runOnce",
			"When the Python code runs: once for every input item, or once for all items.",
			[]base.Option{
				{Name: "Run Once for All Items", Value: "runOnce"},
				{Name: "Run Once for Each Item", Value: "runForEach"},
			}, true),
		nodeprops.Required(nodeprops.Text("Python code", "pythonCode", "",
			"Python source to execute. `items` holds all input items; `_item` holds the current item in per-item mode.",
			"return [{'json': {'hello': 'world'}}]", 12)),
	}
	return append(props, nodeprops.Settings()...)
}
