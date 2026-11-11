package messaging

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// twilioProperties describes the Twilio node.
//
// messaging.TwilioNode reads `operation`, `accountSid`, `authToken`,
// `baseUrl` and — for the sendSms operation — `from`, `to` and
// `body`. The old panel advertised a n8n-style resource switch
// (sms / call / whatsapp) and send/list operations; the executor
// only knows sendSms and getMessages.
func twilioProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Send SMS", Value: "sendSms"},
		{Name: "List messages", Value: "getMessages"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "sendSms",
			"Twilio operation to run.",
			ops, true),
		nodeprops.Str("Account SID", "accountSid", "",
			"Twilio account SID.",
			"ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", true),
		nodeprops.Secret("Auth token", "authToken", "",
			"Twilio auth token.",
			"", true),
		nodeprops.Str("Base URL", "baseUrl", "",
			"Override the Twilio API root. Empty = https://api.twilio.com/2010-04-01/Accounts/<accountSid>.",
			"https://api.twilio.com/2010-04-01", false),
		nodeprops.Str("From", "from", "",
			"Sender phone number in E.164 format.",
			"+14155551234", true).
			WithDisplayOptions(nodeprops.Show("operation", "sendSms")),
		nodeprops.Str("To", "to", "",
			"Recipient phone number in E.164 format.",
			"+14155557654", true).
			WithDisplayOptions(nodeprops.Show("operation", "sendSms")),
		nodeprops.Required(nodeprops.Text("Message", "body", "",
			"Text of the SMS (up to 1600 characters).",
			"Hello from m9m!", 4)).
			WithDisplayOptions(nodeprops.Show("operation", "sendSms")),
	}
	return append(props, nodeprops.Settings()...)
}

// teamsProperties describes the Microsoft Teams node.
//
// messaging.TeamsNode reads `operation`, `accessToken`, `baseUrl`,
// `teamId`, `channelId` and `text`. The channel and the message only
// apply when a message is sent, so they are shown for that operation
// alone.
func teamsProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Send message", Value: "sendMessage"},
		{Name: "List channels", Value: "listChannels"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "sendMessage",
			"Microsoft Teams operation to run.",
			ops, true),
		nodeprops.Secret("Access token", "accessToken", "",
			"Microsoft Graph access token.",
			"", true),
		nodeprops.Str("Base URL", "baseUrl", "https://graph.microsoft.com/v1.0",
			"Microsoft Graph base URL.",
			"https://graph.microsoft.com/v1.0", false),
		nodeprops.Str("Team ID", "teamId", "",
			"Microsoft Teams team ID.",
			"19:team-id@thread.tacv2", true),
		nodeprops.Str("Channel ID", "channelId", "",
			"Microsoft Teams channel ID.",
			"19:channel-id@thread.tacv2", true).
			WithDisplayOptions(nodeprops.Show("operation", "sendMessage")),
		nodeprops.Required(nodeprops.Text("Message", "text", "",
			"Message body.",
			"Hello from m9m!", 4)).
			WithDisplayOptions(nodeprops.Show("operation", "sendMessage")),
	}
	return append(props, nodeprops.Settings()...)
}
