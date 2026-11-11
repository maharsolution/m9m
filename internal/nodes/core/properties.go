package core

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// noopProperties describes the No Op node. It takes no parameters —
// only the shared Settings tab (which carries the n8n retry
// conditionals).
func noopProperties() []base.NodeProperty {
	return nodeprops.Settings()
}

// loopProperties describes the (core) Loop node. The registered
// "n8n-nodes-base.loop" node lives in the transform package; this
// descriptor is kept for the core package's own loop node.
func loopProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Opt("Loop over", "loopOver", "items",
			"What the loop iterates over.",
			[]base.Option{
				{Name: "Items", Value: "items"},
				{Name: "Number", Value: "number"},
			}, true),
		nodeprops.Num("Batch size", "batchSize", 1,
			"How many items are handed to the loop body per iteration.", false),
		nodeprops.Str("Done path", "donePath", "",
			"JSONPath that marks the loop as finished early (e.g. $.done).",
			"$.done", false),
	}
	return append(props, nodeprops.Settings()...)
}

// executeWorkflowProperties describes the Execute Workflow node.
//
// core.ExecuteWorkflowNode reads `workflowId` (string or n8n
// resource-locator object), the legacy `workflowPath`, `source`
// (only "localFile" is accepted besides the empty default),
// `maxDepth` and the `workflowInputs` block (mappingMode + value).
//
// The previously advertised "database | parameter | url" source
// values made the node fail validation, and workflowJson / mode /
// waitForSubWorkflow were never read.
func executeWorkflowProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("Workflow ID", "workflowId", "",
			"ID of the child workflow to run.",
			"STeu50G0h27glAL8", false).
			WithDisplayOptions(nodeprops.Hide("source", "localFile")),
		nodeprops.Str("Workflow path", "workflowPath", "",
			"Path of a child workflow JSON file on disk (legacy local-file source).",
			"/data/workflows/child.json", false).
			WithDisplayOptions(nodeprops.Show("source", "localFile")),
		nodeprops.Opt("Source", "source", "",
			"Where the child workflow comes from. Leave on the database to resolve the workflow by ID.",
			[]base.Option{
				{Name: "Database (workflow ID)", Value: ""},
				{Name: "Local file", Value: "localFile"},
			}, false),
		nodeprops.Num("Maximum depth", "maxDepth", 10,
			"How many nested sub-workflow levels may run before the node stops.", false),
		nodeprops.Opt("Input mapping mode", "workflowInputs.mappingMode", "defineBelow",
			"How the parent's data is handed to the child workflow.",
			[]base.Option{
				{Name: "Pass all input items", Value: "auto"},
				{Name: "Define below", Value: "defineBelow"},
			}, false),
		nodeprops.JSON("Input mappings", "workflowInputs.value", "{}",
			"Mapping of child input names to expressions (only used when the mapping mode is \"Define below\").").
			WithDisplayOptions(nodeprops.Show("workflowInputs.mappingMode", "defineBelow")),
	}
	return append(props, nodeprops.Settings()...)
}
