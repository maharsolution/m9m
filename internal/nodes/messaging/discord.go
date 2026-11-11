package messaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// DiscordNode sends messages to Discord
type DiscordNode struct {
	*base.BaseNode
	httpClient *http.Client
}

// NewDiscordNode creates a new Discord node
func NewDiscordNode() *DiscordNode {
	return &DiscordNode{
		BaseNode: base.NewBaseNode(base.NodeDescription{
			Name:        "Discord",
			Description: "Send messages and interact with Discord",
			Category:    "communication",
			Group:       []string{"communication"},
			Properties:  discordProperties(),
			Credentials: []base.NodeCredentialSpec{
				{Type: "discord", DisplayName: "Discord", Required: true},
			},
		}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Execute sends a message to Discord
func (n *DiscordNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Get parameters
	webhookURL, botToken := resolveDiscordAuth(nodeParams)
	content := n.GetStringParameter(nodeParams, "content", "")
	username := n.GetStringParameter(nodeParams, "username", "m9m Bot")
	avatarURL := n.GetStringParameter(nodeParams, "avatarUrl", "")

	if webhookURL == "" {
		return nil, fmt.Errorf("webhookUrl is required")
	}

	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	var results []model.DataItem

	for range inputData {
		// Prepare Discord webhook payload
		payload := map[string]interface{}{
			"content":  content,
			"username": username,
		}

		if avatarURL != "" {
			payload["avatar_url"] = avatarURL
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %v", err)
		}

		// Send to Discord. When the `discord` credential carries a
		// bot token we authenticate the call with it; the token is
		// never logged and never appears in an error message.
		req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %v", stripURLFromError(err))
		}
		req.Header.Set("Content-Type", "application/json")
		if botToken != "" {
			req.Header.Set("Authorization", "Bot "+botToken)
		}

		resp, err := n.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %v", stripURLFromError(err))
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("discord returned status %d", resp.StatusCode)
		}

		result := model.DataItem{
			JSON: map[string]interface{}{
				"success":  true,
				"content":  content,
				"username": username,
				"status":   resp.StatusCode,
			},
		}

		results = append(results, result)
	}

	return results, nil
}

// resolveDiscordAuth reads the Discord authentication material out of
// the node parameters.
//
// The credential manager flattens the `discord` credential into the
// node parameters under keys shaped `<credentialType>_<field>`, i.e.
// `discord_webhookUrl` and `discord_botToken`. The legacy plain
// parameter names (`webhookUrl`, `botToken`) are kept as fallbacks so
// workflows that inline the secret directly into `parameters` keep
// working.
//
// The values are never logged and never appear in an error message.
func resolveDiscordAuth(params map[string]interface{}) (webhookURL string, botToken string) {
	webhookURL = base.CredentialString(params, "discord_webhookUrl", "webhookUrl")
	botToken = base.CredentialString(params, "discord_botToken", "botToken")
	return webhookURL, botToken
}

// stripURLFromError removes the request URL from a transport / URL
// parse error.
//
// A Discord webhook URL embeds the webhook token, so echoing the URL
// back in an error message would leak the credential into the
// execution record. The underlying cause (DNS failure, refused
// connection, malformed URL) is kept.
func stripURLFromError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// discordProperties returns the Discord node's parameter
// descriptors.
//
// messaging.DiscordNode reads `webhookUrl`, `content`, `username`
// and `avatarUrl`; both the webhook URL and the content are
// mandatory. The old panel advertised a bot-token mode and embed /
// TTS fields the executor never reads.
//
// The `webhookUrl` property is the legacy inline form; when a
// `discord` credential is attached the same values arrive as
// `discord_webhookUrl` / `discord_botToken` (see resolveDiscordAuth).
func discordProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("Webhook URL", "webhookUrl", "",
			"Discord webhook URL.",
			"https://discord.com/api/webhooks/…", true),
		nodeprops.Required(nodeprops.Text("Content", "content", "",
			"Message body (up to 2000 characters).",
			"Hello from m9m!", 4)),
		nodeprops.Str("Username", "username", "m9m Bot",
			"Display name the message is posted under.",
			"m9m Bot", false),
		nodeprops.Str("Avatar URL", "avatarUrl", "",
			"Avatar shown next to the message (https URL).",
			"https://example.com/avatar.png", false),
	}
	return append(props, nodeprops.Settings()...)
}
