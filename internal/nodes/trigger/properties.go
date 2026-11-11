package trigger

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the trigger nodes that previously fell back to raw JSON.
//
// Every parameter name below is the exact key the implementation
// reads; the conditional fields carry n8n-style `displayOptions` so
// the parameter panel only shows the inputs that apply to the
// selected mode.

func errorTriggerProperties() []base.NodeProperty {
	// Error Trigger has no node-specific parameters; only the
	// engine settings (notes / retry / on-error).
	return nodeprops.Settings()
}

func executeWorkflowTriggerProperties() []base.NodeProperty {
	// The sub-workflow entry point only validates the mapping mode
	// n8n writes into `workflowInputs`; the parent Execute Workflow
	// node carries the actual values.
	props := []base.NodeProperty{
		nodeprops.Opt("Mapping Mode", "workflowInputs.mappingMode", "defineBelow",
			"How the parent workflow passes its inputs into this workflow.",
			[]base.Option{
				{Name: "Define Below", Value: "defineBelow"},
				{Name: "Using Fields Below (auto)", Value: "auto"},
			}, false),
	}
	return append(props, nodeprops.Settings()...)
}

func respondToWebhookProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Opt("Respond With", "respondWith", "allIncomingItems",
			"What to send back to the webhook caller.",
			[]base.Option{
				{Name: "All Incoming Items", Value: "allIncomingItems"},
				{Name: "First Incoming Item", Value: "firstIncomingItem"},
				{Name: "JSON", Value: "json"},
				{Name: "Text", Value: "text"},
				{Name: "No Response Body", Value: "noData"},
			}, true),
		nodeprops.JSON("Response Body", "responseBody", "{}",
			"Body sent back to the caller. Used when Respond With is JSON or Text.").
			WithDisplayOptions(nodeprops.Show("respondWith", "json", "text")),
		nodeprops.Collection("Response Headers", "options.responseHeaders",
			"Extra HTTP headers to send with the response. Each entry needs a `name` and a `value`; values may be expressions.",
			nil),
	}
	return append(props, nodeprops.Settings()...)
}
