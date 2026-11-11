package email

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// smtpProperties describes the SMTP node.
//
// email.SMTPNode reads `host`, `port`, `username`, `password`,
// `auth`, `tls`, `to`, `cc`, `bcc`, `from`, `replyTo`, `subject`,
// `body`, `isHTML`, `headers`, `attachmentName` and
// `keepOriginalData`. The old panel advertised fromEmail / toEmail /
// htmlBody / textBody / ignoreSSL / timeout keys that this executor
// never reads.
func smtpProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("SMTP host", "host", "",
			"Host of the SMTP server.",
			"smtp.example.com", true),
		nodeprops.Num("SMTP port", "port", 587,
			"Port of the SMTP server (465 for implicit TLS).", false),
		nodeprops.Str("Username", "username", "",
			"SMTP user name.",
			"mailer@example.com", false),
		nodeprops.Secret("Password", "password", "",
			"SMTP password.",
			"", false),
		nodeprops.Opt("Authentication", "auth", "plain",
			"SMTP authentication mechanism.",
			[]base.Option{
				{Name: "Plain", Value: "plain"},
				{Name: "Login", Value: "login"},
				{Name: "CRAM-MD5", Value: "crammd5"},
				{Name: "None", Value: "none"},
			}, false),
		nodeprops.Bool("TLS", "tls", false,
			"Use STARTTLS for the connection."),
		nodeprops.Required(nodeprops.Text("To", "to", "",
			"Recipient address (comma-separated for several).",
			"user@example.com", 2)),
		nodeprops.Str("CC", "cc", "",
			"CC recipients (comma-separated).",
			"", false),
		nodeprops.Str("BCC", "bcc", "",
			"BCC recipients (comma-separated).",
			"", false),
		nodeprops.Str("From", "from", "",
			"Sender address.",
			"noreply@example.com", true),
		nodeprops.Str("Reply to", "replyTo", "",
			"Reply-To address.",
			"", false),
		nodeprops.Str("Subject", "subject", "",
			"Subject line of the mail.",
			"", true),
		nodeprops.Required(nodeprops.Text("Body", "body", "",
			"Mail body.",
			"Hello from m9m!", 6)),
		nodeprops.Bool("Body is HTML", "isHTML", false,
			"Treat the body as HTML instead of plain text."),
		nodeprops.JSON("Headers", "headers", "{}",
			"Extra mail headers (name → value)."),
		nodeprops.Str("Attachment name", "attachmentName", "",
			"Name the binary attachment is stored under.",
			"attachment", false),
		nodeprops.Bool("Keep original data", "keepOriginalData", false,
			"Keep the input item's fields on the output item."),
	}
	return append(props, nodeprops.Settings()...)
}
