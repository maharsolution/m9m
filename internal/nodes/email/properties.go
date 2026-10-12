package email

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the SMTP node.

func smtpProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringProp("From Email", "fromEmail", "", "Sender email (SMTP `MAIL FROM`).", "noreply@example.com", true),
		base.StringProp("To Email", "toEmail", "", "Recipient email (comma-separated for multiple).", "user@example.com", true),
		base.StringProp("CC", "ccEmail", "", "CC recipients (comma-separated).", "", false),
		base.StringProp("BCC", "bccEmail", "", "BCC recipients (comma-separated).", "", false),
		base.StringProp("Subject", "subject", "", "Email subject line.", "", true),
		base.TextProp("Body (HTML)", "htmlBody", "", "HTML body. Empty = plain text only.", "", 6),
		base.TextProp("Body (Text)", "textBody", "", "Plain-text body. Used as fallback if HTML is empty.", "Hello from m9m!", 4),
		base.StringProp("Reply To", "replyTo", "", "Reply-To email address.", "", false),
		base.BoolProp("Ignore SSL", "ignoreSSL", false, "Skip TLS verification (dev only)."),
		base.NumberProp("Timeout (s)", "timeout", 30, "SMTP timeout in seconds.", false),
	}
	return append(props, base.CommonSettings()...)
}
