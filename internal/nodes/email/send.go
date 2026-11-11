/*
Package email provides email-related node implementations for m9m.
*/
package email

import (
	"fmt"
	"net/smtp"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// SendEmailNode implements the Send Email node functionality
type SendEmailNode struct {
	*base.BaseNode
}

// NewSendEmailNode creates a new Send Email node
func NewSendEmailNode() *SendEmailNode {
	description := base.NodeDescription{
		Name:        "Send Email",
		Description: "Sends an email through an SMTP server.",
		Category:    "Email",
		Group:       []string{"communication"},
		Properties:  sendEmailProperties(),
		Inputs:      []string{"main"},
		Outputs:     []string{"main"},
	}

	return &SendEmailNode{
		BaseNode: base.NewBaseNode(description),
	}
}

// sendEmailProperties describes the Send Email node.
//
// email.SendEmailNode reads `smtpHost`, `smtpPort`, `username`,
// `password`, `fromEmail`, `toEmail`, `subject` and `body`.
func sendEmailProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("SMTP host", "smtpHost", "",
			"Host of the SMTP server used to deliver the mail.",
			"smtp.example.com", true),
		nodeprops.Num("SMTP port", "smtpPort", 587,
			"Port of the SMTP server.", true),
		nodeprops.Str("Username", "username", "",
			"SMTP user name (leave empty for unauthenticated relays).",
			"mailer@example.com", false),
		nodeprops.Secret("Password", "password", "",
			"SMTP password.",
			"", false),
		nodeprops.Str("From email", "fromEmail", "",
			"Sender address.",
			"noreply@example.com", true),
		nodeprops.Str("To email", "toEmail", "",
			"Recipient address.",
			"user@example.com", true),
		nodeprops.Str("Subject", "subject", "",
			"Subject line of the mail.",
			"", true),
		nodeprops.Required(nodeprops.Text("Body", "body", "",
			"Mail body. Falls back to the inbound item's `body` field when empty.",
			"Hello from m9m!", 6)),
	}
	return append(props, nodeprops.Settings()...)
}

// Description returns the node description
func (s *SendEmailNode) Description() base.NodeDescription {
	return s.BaseNode.Description()
}

// ValidateParameters validates Send Email node parameters
func (s *SendEmailNode) ValidateParameters(params map[string]interface{}) error {
	if params == nil {
		return s.CreateError("parameters cannot be nil", nil)
	}

	// Check required SMTP parameters
	smtpHost := s.GetStringParameter(params, "smtpHost", "")
	if smtpHost == "" {
		return s.CreateError("smtpHost is required", nil)
	}

	smtpPort := s.GetIntParameter(params, "smtpPort", 0)
	if smtpPort <= 0 {
		return s.CreateError("smtpPort is required", nil)
	}

	// Check required email parameters
	fromEmail := s.GetStringParameter(params, "fromEmail", "")
	if fromEmail == "" {
		return s.CreateError("fromEmail is required", nil)
	}

	toEmail := s.GetStringParameter(params, "toEmail", "")
	if toEmail == "" {
		return s.CreateError("toEmail is required", nil)
	}

	subject := s.GetStringParameter(params, "subject", "")
	if subject == "" {
		return s.CreateError("subject is required", nil)
	}

	return nil
}

// Execute processes the Send Email node operation
func (s *SendEmailNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	if len(inputData) == 0 {
		return []model.DataItem{}, nil
	}

	// Get SMTP parameters
	smtpHost := s.GetStringParameter(nodeParams, "smtpHost", "")
	smtpPort := s.GetIntParameter(nodeParams, "smtpPort", 587)
	username := s.GetStringParameter(nodeParams, "username", "")
	password := s.GetStringParameter(nodeParams, "password", "")

	// Get email parameters
	fromEmail := s.GetStringParameter(nodeParams, "fromEmail", "")
	toEmail := s.GetStringParameter(nodeParams, "toEmail", "")
	subject := s.GetStringParameter(nodeParams, "subject", "")

	// Process each input data item
	result := make([]model.DataItem, len(inputData))

	for i, item := range inputData {
		// Get email body from input data or parameters
		var body string
		if bodyParam, ok := item.JSON["body"].(string); ok {
			body = bodyParam
		} else {
			body = s.GetStringParameter(nodeParams, "body", "")
		}

		// Send the email
		err := s.sendEmail(smtpHost, smtpPort, username, password, fromEmail, toEmail, subject, body)
		if err != nil {
			return nil, s.CreateError(fmt.Sprintf("failed to send email: %v", err), nil)
		}

		// Create a new item with success information
		newItem := model.DataItem{
			JSON: make(map[string]interface{}),
		}

		// Copy existing JSON data
		for k, v := range item.JSON {
			newItem.JSON[k] = v
		}

		// Add success information
		newItem.JSON["success"] = true
		newItem.JSON["message"] = "Email sent successfully"

		result[i] = newItem
	}

	return result, nil
}

// sendEmail sends an email via SMTP
func (s *SendEmailNode) sendEmail(smtpHost string, smtpPort int, username, password, fromEmail, toEmail, subject, body string) error {
	// Create the email message
	message := fmt.Sprintf(
		"To: %s\r\n"+
			"From: %s\r\n"+
			"Subject: %s\r\n"+
			"\r\n"+
			"%s",
		toEmail, fromEmail, subject, body)

	// Connect to the SMTP server
	auth := smtp.PlainAuth("", username, password, smtpHost)
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	err := smtp.SendMail(addr, auth, fromEmail, []string{toEmail}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	return nil
}
