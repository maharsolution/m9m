package core

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the core nodes (wait, noop, loop, executeWorkflow). Each
// helper mirrors n8n's `INodeTypeDescription.properties` for
// the corresponding `n8n-nodes-base.<node>` type.

func waitProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Resume", "resume", "afterTimeInterval", "When to resume the workflow.", []base.Option{
			{Name: "After Time Interval", Value: "afterTimeInterval"},
			{Name: "At Specified Time", Value: "atSpecifiedTime"},
			{Name: "On Webhook Call", Value: "onWebhookCall"},
		}, true),
		base.NumberProp("Amount", "amount", 1, "Number of units to wait (depends on `unit`).", false),
		base.StringOpt("Unit", "unit", "seconds", "Unit of the wait amount.", []base.Option{
			{Name: "Seconds", Value: "seconds"},
			{Name: "Minutes", Value: "minutes"},
			{Name: "Hours", Value: "hours"},
			{Name: "Days", Value: "days"},
		}, false),
		base.StringProp("Date Time", "dateTime", "", "ISO-8601 date-time (when resume=atSpecifiedTime).", "", false),
		base.BoolProp("Busy", "busy", false, "Mark the workflow as busy while waiting (so the editor treats it as running)."),
	}
	return append(props, base.CommonSettings()...)
}

func noopProperties() []base.NodeProperty {
	// No-op node has no parameters; only engine settings.
	return base.CommonSettings()
}

func loopProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Loop Over", "loopOver", "items", "What to iterate over.", []base.Option{
			{Name: "Items", Value: "items"},
			{Name: "Number", Value: "number"},
		}, true),
		base.NumberProp("Batch Size", "batchSize", 1, "How many items per loop iteration.", false),
		base.StringProp("Done Path", "donePath", "", "Optional JSONPath to mark loop done early (e.g. $.done).", "", false),
	}
	return append(props, base.CommonSettings()...)
}

func executeWorkflowProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Source", "source", "database", "Where the child workflow lives.", []base.Option{
			{Name: "Database", Value: "database"},
			{Name: "Parameter", Value: "parameter"},
			{Name: "URL", Value: "url"},
		}, true),
		base.StringProp("Workflow ID", "workflowId", "", "ID of the child workflow (source=database).", "", false),
		base.JsonProp("Workflow JSON", "workflowJson", "{}", "Inline child workflow JSON (source=parameter)."),
		base.StringProp("Workflow URL", "workflowUrl", "", "URL to fetch the child workflow JSON (source=url).", "", false),
		base.StringOpt("Mode", "mode", "each", "Run mode.", []base.Option{
			{Name: "Once (single input)", Value: "once"},
			{Name: "Each Item", Value: "each"},
		}, true),
		base.BoolProp("Wait for Sub-workflow", "waitForSubWorkflow", true, "Block until the sub-workflow finishes."),
	}
	return append(props, base.CommonSettings()...)
}
