package messaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// SlackNode sends messages to Slack
type SlackNode struct {
	*base.BaseNode
	httpClient *http.Client
}

// NewSlackNode creates a new Slack node
func NewSlackNode() *SlackNode {
	return &SlackNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Slack",
			Description: "Send messages and interact with Slack",
			Category:    "communication",
			Group:       []string{"communication"},
			Properties:  slackProperties(),
			Credentials: []base.NodeCredentialSpec{
				{Type: "slack", DisplayName: "Slack", Required: false},
			},
		}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Execute sends a message to Slack
func (n *SlackNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Get parameters
	webhookURL, token := resolveSlackAuth(nodeParams)
	text := n.GetStringParameter(nodeParams, "text", "")
	channel := n.GetStringParameter(nodeParams, "channel", "")
	username := n.GetStringParameter(nodeParams, "username", "m9m Bot")

	if webhookURL == "" && token == "" {
		return nil, fmt.Errorf("either webhookUrl or token is required")
	}

	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	var results []model.DataItem

	for range inputData {
		var result model.DataItem

		if webhookURL != "" {
			// Use webhook URL method
			err := n.sendViaWebhook(webhookURL, text, username)
			if err != nil {
				return nil, fmt.Errorf("failed to send message: %v", err)
			}

			result = model.DataItem{
				JSON: map[string]interface{}{
					"success": true,
					"text":    text,
					"method":  "webhook",
				},
			}
		} else {
			// Use API token method
			if channel == "" {
				return nil, fmt.Errorf("channel is required when using token authentication")
			}

			err := n.sendViaAPI(token, channel, text)
			if err != nil {
				return nil, fmt.Errorf("failed to send message: %v", err)
			}

			result = model.DataItem{
				JSON: map[string]interface{}{
					"success": true,
					"text":    text,
					"channel": channel,
					"method":  "api",
				},
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// sendViaWebhook sends a message using Slack webhook
func (n *SlackNode) sendViaWebhook(webhookURL, text, username string) error {
	payload := map[string]interface{}{
		"text":     text,
		"username": username,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	resp, err := n.httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// sendViaAPI sends a message using Slack API
func (n *SlackNode) sendViaAPI(token, channel, text string) error {
	payload := map[string]interface{}{
		"channel": channel,
		"text":    text,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		errMsg := "unknown error"
		if msg, exists := result["error"].(string); exists {
			errMsg = msg
		}
		return fmt.Errorf("slack API error: %s", errMsg)
	}

	return nil
}

// resolveSlackAuth reads the Slack authentication material out of the
// node parameters.
//
// The credential manager flattens the `slack` credential into the node
// parameters under keys shaped `<credentialType>_<field>`, i.e.
// `slack_accessToken`. The legacy plain parameter names (`webhookUrl`,
// `token`) are kept as fallbacks so workflows that inline the secret
// directly into `parameters` keep working.
//
// The values are never logged and never appear in an error message.
func resolveSlackAuth(params map[string]interface{}) (webhookURL string, token string) {
	webhookURL = base.CredentialString(params, "slack_webhookUrl", "webhookUrl")
	token = base.CredentialString(params, "slack_accessToken", "token")
	return webhookURL, token
}

// slackProperties returns the Slack node's parameter descriptors.
//
// messaging.SlackNode reads `webhookUrl`, `token`, `channel`, `text`
// and `username`. The old panel advertised n8n's authentication
// switch, channel type and formatting flags — none of those keys are
// read by the executor, so they were dropped rather than kept as
// misleading fields.
//
// The `webhookUrl` / `token` properties are the legacy inline form;
// when a `slack` credential is attached the same values arrive as
// `slack_webhookUrl` / `slack_accessToken` (see resolveSlackAuth).
func slackProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("Webhook URL", "webhookUrl", "",
			"Slack incoming webhook URL. Used when no bot token is set.",
			"https://hooks.slack.com/services/…", false),
		nodeprops.Secret("Bot token", "token", "",
			"Slack bot / OAuth token (xoxb-…). Used when no webhook URL is set.",
			"xoxb-…", false),
		nodeprops.Str("Channel", "channel", "",
			"Target channel (#general), user (@alice) or channel ID (C0123). Required when a bot token is used.",
			"#general", false),
		nodeprops.Required(nodeprops.Text("Message", "text", "",
			"Message body.",
			"Hello from m9m!", 4)),
		nodeprops.Str("Username", "username", "m9m Bot",
			"Display name the message is posted under (webhook mode).",
			"m9m Bot", false),
	}
	return append(props, nodeprops.Settings()...)
}
