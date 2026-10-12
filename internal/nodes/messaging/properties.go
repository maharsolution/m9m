package messaging

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the messaging nodes that were previously only reachable via
// raw parameters JSON. Each helper mirrors n8n's
// `INodeTypeDescription.properties` for the corresponding
// `n8n-nodes-base.<node>` type.

// twilioProperties returns the Twilio node's parameter
// descriptors.
func twilioProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "SMS", Value: "sms"},
		{Name: "Call", Value: "call"},
		{Name: "WhatsApp", Value: "whatsapp"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "sms", "Twilio resource to use.", resources, true),
		base.StringOpt("Operation", "operation", "send", "What to do with the resource.", []base.Option{
			{Name: "Send", Value: "send"},
			{Name: "List", Value: "list"},
		}, true),
		base.StringProp("From", "from", "", "Sender phone number (E.164, e.g. +14155551234).", "", true),
		base.StringProp("To", "to", "", "Recipient phone number (E.164).", "", true),
		base.TextProp("Body", "body", "", "Message body (up to 1600 characters for SMS).", "Hello from m9m!", 4),
		base.StringProp("Media URL", "mediaUrl", "", "Optional MMS media URL.", "", false),
		base.StringProp("Status Callback", "statusCallback", "", "Webhook URL to receive delivery status updates.", "", false),
	}
	return append(props, base.CommonSettings()...)
}

// teamsProperties returns the Microsoft Teams node's parameter
// descriptors.
func teamsProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "message", "Teams resource to use.", []base.Option{
			{Name: "Message", Value: "message"},
			{Name: "Channel", Value: "channel"},
		}, true),
		base.StringOpt("Operation", "operation", "send", "What to do with the resource.", []base.Option{
			{Name: "Send", Value: "send"},
			{Name: "Create", Value: "create"},
			{Name: "List", Value: "list"},
		}, true),
		base.StringProp("Team ID", "teamId", "", "Microsoft Teams team ID (GUID).", "", false),
		base.StringProp("Channel ID", "channelId", "", "Microsoft Teams channel ID (GUID).", "", false),
		base.TextProp("Message", "message", "", "Message body (supports simple markdown).", "Hello from m9m!", 4),
		base.StringProp("Title", "title", "", "Optional title for adaptive cards.", "", false),
		base.JsonProp("Card", "card", "{}", "Adaptive card JSON."),
	}
	return append(props, base.CommonSettings()...)
}
