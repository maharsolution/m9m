package messaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/nodes/base"
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
			Properties:  discordProperties(),
		}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Execute sends a message to Discord
func (n *DiscordNode) Execute(inputData []model.DataItem, nodeParams map[string]interface{}) ([]model.DataItem, error) {
	// Get parameters
	webhookURL := n.GetStringParameter(nodeParams, "webhookUrl", "")
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

		// Send to Discord
		resp, err := n.httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %v", err)
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

// discordProperties returns the Discord node's parameter
// descriptors. BUG-UI-2026-09-09-001 — populates Properties so the
// Settings tab renders the structured form.
func discordProperties() []base.NodeProperty {
	authModes := []base.Option{
		{Name: "Webhook URL", Value: "webhook"},
		{Name: "Bot Token", Value: "bot"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Authentication", "authentication", "webhook", "Send via a Discord webhook URL or via a bot token.", authModes, true),
		base.StringProp("Webhook URL", "webhookUrl", "", "Discord webhook URL.", "", false),
		base.StringProp("Bot Token", "botToken", "", "Discord bot token (from the developer portal).", "", false),
		base.StringProp("Channel ID", "channelId", "", "Target channel ID (when using Bot Token auth).", "", false),
		base.TextProp("Content", "content", "", "Message body (up to 2000 characters).", "Hello from m9m!", 4),
		base.StringProp("Username", "username", "", "Override the bot's display name.", "m9m Bot", false),
		base.StringProp("Avatar URL", "avatarUrl", "", "Override the bot's avatar (https URL).", "", false),
		base.BoolProp("TTS", "tts", false, "Send the message with text-to-speech enabled."),
		base.JsonProp("Embeds", "embeds", "[]", "Optional embeds array (max 10)."),
	}
	return append(props, base.CommonSettings()...)
}
