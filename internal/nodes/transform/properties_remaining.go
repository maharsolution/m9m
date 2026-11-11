package transform

// BUG-UI-2026-09-09-001 — final Properties migration. The 5
// remaining Data-Transformation nodes (splitInBatches, function,
// itemLists, json, xml) now populate NodeDescription.Properties
// from the canonical n8n parameter surface so the NDV Settings
// tab renders the structured form instead of falling back to the
// raw-JSON dump.
//
// Every property name below is the exact key the node's Execute
// implementation reads — no dotted n8n-style aliases, because the
// runtime stores parameters in a flat map.

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// loopProperties returns the Loop node's parameter descriptors.
// Matches LoopNode.Execute: mode is each / times / while, with
// times, condition, and maxIterations.
func loopProperties() []base.NodeProperty {
	modes := []base.Option{
		{Name: "Each Item", Value: "each"},
		{Name: "Times", Value: "times"},
		{Name: "While", Value: "while"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Mode", "mode", "each", "How to iterate: each input item, a fixed count, or while a condition is true.", modes, true),
		nodeprops.Num("Times", "times", 1, "Number of iterations when mode is times. Capped at 1000.", false).
			WithDisplayOptions(nodeprops.Show("mode", "times")),
		nodeprops.Str("Condition", "condition", "", "JavaScript expression evaluated each iteration when mode is while. $index and $item are in scope.", "$index < 3", false).
			WithDisplayOptions(nodeprops.Show("mode", "while")),
		nodeprops.Num("Max Iterations", "maxIterations", float64(defaultMaxIterations), "Safety cap for while mode.", false).
			WithDisplayOptions(nodeprops.Show("mode", "while")),
	}
	return append(props, nodeprops.Settings()...)
}

// splitInBatchesProperties returns the SplitInBatches node's
// parameter descriptors. Mirrors n8n's
// `n8n-nodes-base.splitInBatches` properties.
//
// The engine reads `batchSize` (default 10) and the `options` map
// (which carries `reset`), so the reset toggle is surfaced as an
// `options` object rather than a flat flag.
func splitInBatchesProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Num("Batch Size", "batchSize", 10, "Number of items per iteration. Must be greater than zero.", true),
		nodeprops.JSON("Options", "options", "{}", "Optional options object. Supported key: `reset` (boolean) — restart the batch from the first item on each workflow run."),
	}
	return append(props, nodeprops.Settings()...)
}

// functionProperties returns the Function node's parameter
// descriptors. Mirrors n8n's `n8n-nodes-base.function` properties.
//
// The runtime reads the `jsCode` key (the legacy n8n Function node
// parameter) — not `functionCode`, which the n8n v2 node uses.
func functionProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Text("Function Code", "jsCode", "", "JavaScript function body. Receives `items` array; must return an array of items.", "// Each item has a .json property.\nreturn items;", 12),
	}
	return append(props, nodeprops.Settings()...)
}

// itemListsProperties returns the Item Lists node's parameter
// descriptors.
//
// Divergence from n8n: n8n's Item Lists node exposes an `operation`
// picker with values such as `splitOut` / `combine`. m9m's runtime
// reads a `mode` parameter whose only accepted values are `combine`
// and `split`, so the metadata mirrors the runtime.
func itemListsProperties() []base.NodeProperty {
	modes := []base.Option{
		{Name: "Combine", Value: "combine"},
		{Name: "Split", Value: "split"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Mode", "mode", "combine", "Whether to combine the incoming items into one list or split them.", modes, true),
	}
	return append(props, nodeprops.Settings()...)
}

// jsonProperties returns the JSON node's parameter descriptors.
//
// The runtime supports four operations: parse, stringify, extract and
// merge. Each operation reads its own parameter set, so the
// operation-specific fields are shown conditionally.
func jsonProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Parse", Value: "parse"},
		{Name: "Stringify", Value: "stringify"},
		{Name: "Extract", Value: "extract"},
		{Name: "Merge", Value: "merge"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "parse", "What to do with the JSON.", ops, true),
		nodeprops.Str("JSON Path", "jsonPath", "$json", "Expression resolving to the JSON string to parse (parse operation).", "$json", false).
			WithDisplayOptions(nodeprops.Show("operation", "parse")),
		nodeprops.Bool("Include Item Index", "includeItemIndex", false, "When the parsed value is an array, emit one item per element with its index (parse operation).").
			WithDisplayOptions(nodeprops.Show("operation", "parse")),
		nodeprops.Str("Data Path", "dataPath", "$json", "Expression resolving to the value to stringify (stringify operation).", "$json", false).
			WithDisplayOptions(nodeprops.Show("operation", "stringify")),
		nodeprops.Bool("Indent", "indent", false, "Pretty-print the JSON output with two-space indentation (stringify operation).").
			WithDisplayOptions(nodeprops.Show("operation", "stringify")),
		nodeprops.JSON("Extraction Fields", "extractionFields", "[]", "Array of {name, path} objects — each path resolves one output field (extract operation).").
			WithDisplayOptions(nodeprops.Show("operation", "extract")),
		nodeprops.JSON("Merge Sources", "mergeSources", "[]", "Array of expressions resolving to JSON objects that are merged into the item (merge operation).").
			WithDisplayOptions(nodeprops.Show("operation", "merge")),
	}
	return append(props, nodeprops.Settings()...)
}

// xmlProperties returns the XML node's parameter descriptors.
//
// The runtime reads `mode` plus the `options` map (attributePrefix /
// textKey). The two option keys are also accepted as flat parameters
// so the parameter panel can drive them individually.
func xmlProperties() []base.NodeProperty {
	modes := []base.Option{
		{Name: "XML → JSON", Value: "xmlToJson"},
		{Name: "JSON → XML", Value: "jsonToXml"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Mode", "mode", "xmlToJson", "Conversion direction.", modes, true),
		nodeprops.Str("Attribute Prefix", "attributePrefix", "@", "Prefix used for XML attributes in the JSON output (xmlToJson mode).", "@", false).
			WithDisplayOptions(nodeprops.Show("mode", "xmlToJson")),
		nodeprops.Str("Text Key", "textKey", "_text", "Wrapper key for mixed-content text nodes (xmlToJson mode).", "_text", false).
			WithDisplayOptions(nodeprops.Show("mode", "xmlToJson")),
		nodeprops.JSON("Options", "options", "{}", "Optional options object. Supported keys: `attributePrefix`, `textKey` — they override the flat fields above."),
	}
	return append(props, nodeprops.Settings()...)
}
