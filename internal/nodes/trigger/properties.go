package trigger

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the trigger nodes that previously fell back to raw JSON.

func errorTriggerProperties() []base.NodeProperty {
	// Error Trigger has no parameters; only engine settings.
	return base.CommonSettings()
}

func executeWorkflowTriggerProperties() []base.NodeProperty {
	// Execute Workflow Trigger has no parameters; only engine settings.
	return base.CommonSettings()
}

func respondToWebhookProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Respond With", "respondWith", "json", "What to send back to the webhook caller.", []base.Option{
			{Name: "JSON", Value: "json"},
			{Name: "Text", Value: "text"},
			{Name: "Binary", Value: "binary"},
			{Name: "No Response (continue)", Value: "none"},
			{Name: "First Incoming Item", Value: "firstIncomingItem"},
		}, true),
		base.JsonProp("Response Body", "responseBody", "{}", "Body to send back. Ignored when respondWith=none or firstIncomingItem."),
		base.StringProp("Response Content-Type", "responseContentType", "", "Override the response Content-Type (defaults to the inbound Content-Type).", "", false),
		base.StringProp("Response Headers", "responseHeaders", "", "Optional response headers (JSON string).", "", false),
		base.StringProp("Status Code", "responseCode", "200", "HTTP status code (200 / 201 / 204 / 4xx).", "200", false),
	}
	return append(props, base.CommonSettings()...)
}
