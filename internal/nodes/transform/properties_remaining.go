package transform

// BUG-UI-2026-09-09-001 — final Properties migration. The 5
// remaining Data-Transformation nodes (splitInBatches, function,
// itemLists, json, xml) now populate NodeDescription.Properties
// from the canonical n8n parameter surface so the NDV Settings
// tab renders the structured form instead of falling back to the
// raw-JSON dump.

import "github.com/neul-labs/m9m/internal/nodes/base"

// splitInBatchesProperties returns the SplitInBatches node's
// parameter descriptors. Mirrors n8n's
// `n8n-nodes-base.splitInBatches` properties.
func splitInBatchesProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.NumberProp("Batch Size", "batchSize", 1, "Number of items per iteration.", true),
		base.StringOpt("Reset", "reset", "false", "When true, restart the batch from the first item on each workflow run.", []base.Option{
			{Name: "False", Value: "false"},
			{Name: "True", Value: "true"},
		}, false),
		base.StringOpt("Options", "options", "", "Optional JSON-string of advanced options (e.g. `{\"split\": \"every\"}`).", []base.Option{
			{Name: "Default", Value: ""},
			{Name: "Split into batches (default)", Value: "split"},
			{Name: "Aggregate items", Value: "aggregate"},
		}, false),
	}
	return append(props, base.CommonSettings()...)
}

// functionProperties returns the Function node's parameter
// descriptors. Mirrors n8n's `n8n-nodes-base.function` properties.
func functionProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.TextProp("Function Code", "functionCode", "", "JavaScript function body. Receives `items` array; must return an array of items.", "// Each item has a .json property.\nreturn items;", 12),
	}
	return append(props, base.CommonSettings()...)
}

// itemListsProperties returns the Item Lists node's parameter
// descriptors. Mirrors n8n's `n8n-nodes-base.itemLists` properties.
func itemListsProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Concatenate Items", Value: "concatenateItems"},
		{Name: "Limit", Value: "limit"},
		{Name: "Remove Duplicates", Value: "removeDuplicates"},
		{Name: "Sort", Value: "sort"},
		{Name: "Split Out Items", Value: "splitOutItems"},
		{Name: "Summarize", Value: "summarize"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "concatenateItems", "What to do with the items list.", ops, true),
		base.StringProp("Field To Sort By", "sortBy", "", "Field name to sort by (sort operation).", "", false),
		base.StringOpt("Sort Direction", "sortDirection", "ascending", "Sort direction.", []base.Option{
			{Name: "Ascending", Value: "ascending"},
			{Name: "Descending", Value: "descending"},
		}, false),
		base.StringProp("Field To Split Out", "fieldToSplitOut", "", "Field whose value becomes the new items list (split operation).", "", false),
		base.NumberProp("Limit", "limit", 1, "Max items to keep (limit operation).", false),
		base.StringOpt("Keep", "keep", "firstItems", "Whether to keep the first N or last N items.", []base.Option{
			{Name: "First items", Value: "firstItems"},
			{Name: "Last items", Value: "lastItems"},
		}, false),
		base.StringOpt("Duplicate Field", "duplicateField", "", "Field whose value drives the dedup (removeDuplicates).", []base.Option{
			{Name: "All fields (deep equal)", Value: ""},
			{Name: "Single field by name", Value: "byName"},
		}, false),
		base.StringOpt("Aggregate By", "aggregateBy", "aggregateAllItemData", "What to compute per group (summarize).", []base.Option{
			{Name: "All item data", Value: "aggregateAllItemData"},
			{Name: "Specific fields", Value: "aggregateFields"},
			{Name: "Count", Value: "count"},
		}, false),
		base.JsonProp("Fields to Summarize", "fieldsToAggregate", "[]", "Array of {field, operation} objects (summarize)."),
	}
	return append(props, base.CommonSettings()...)
}

// jsonProperties returns the JSON node's parameter descriptors.
// Mirrors n8n's `n8n-nodes-base.json` properties.
func jsonProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Convert to JSON", Value: "toJson"},
		{Name: "Convert from JSON", Value: "fromJson"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Operation", "operation", "toJson", "What to do with the JSON.", ops, true),
		base.StringOpt("Mode", "mode", "eachItem", "When to run the operation.", []base.Option{
			{Name: "Once per item", Value: "eachItem"},
			{Name: "Once for all items", Value: "onceForAllItems"},
		}, true),
		base.StringProp("Property Name", "propertyName", "", "Property to read JSON from (toJson) or write to (fromJson).", "", false),
		base.StringOpt("JSON Parse Mode", "jsonParseMode", "strict", "How lenient to be when parsing JSON (fromJson).", []base.Option{
			{Name: "Strict (RFC 8259)", Value: "strict"},
			{Name: "Lenient (comments, trailing commas)", Value: "lenient"},
		}, false),
		base.BoolProp("Always Output Data", "alwaysOutputData", true, "Emit an item even when input is empty."),
		base.NumberProp("Max Depth", "maxDepth", 100, "Max nesting depth for toJson; -1 for unlimited.", false),
	}
	return append(props, base.CommonSettings()...)
}

// xmlProperties returns the XML node's parameter descriptors.
// Mirrors n8n's `n8n-nodes-base.xml` properties.
func xmlProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Mode", "mode", "jsonToXml", "Conversion direction.", []base.Option{
			{Name: "JSON → XML", Value: "jsonToXml"},
			{Name: "XML → JSON", Value: "xmlToJson"},
		}, true),
		base.StringProp("Property Name", "propertyName", "", "Property to read the input from / write the output to.", "data", false),
		base.StringProp("Attribute Key Name", "attributeKey", "$", "Wrapper key for XML attributes in the JSON output (xmlToJson).", "$", false),
		base.StringProp("Text Key Name", "textKey", "_", "Wrapper key for mixed-content text nodes (xmlToJson).", "_", false),
		base.BoolProp("Add CDATA", "addCdata", true, "Wrap text content in CDATA (jsonToXml)."),
		base.StringProp("Root Element", "rootElement", "root", "Root element name (jsonToXml).", "root", false),
		base.StringProp("Encoding", "encoding", "UTF-8", "XML encoding (jsonToXml).", "UTF-8", false),
	}
	return append(props, base.CommonSettings()...)
}
