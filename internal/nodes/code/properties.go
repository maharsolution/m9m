package code

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the PythonCode node.

func pythonCodeProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Mode", "mode", "runOnceForAllItems", "When the code runs.", []base.Option{
			{Name: "Run Once for All Items", Value: "runOnceForAllItems"},
			{Name: "Run Once for Each Item", Value: "runOnceForEachItem"},
		}, true),
		base.TextProp("Code", "pythonCode", "", "Python code body. Exposes `items` and `_item` globals.", "return [{'json': {'hello': 'world'}}]", 12),
		base.StringProp("Python Version", "pythonVersion", "3.11", "Python interpreter to use.", "3.11", false),
		base.JsonProp("Input Items", "inputItems", "[]", "Optional static input items (used when no upstream)."),
	}
	return append(props, base.CommonSettings()...)
}
