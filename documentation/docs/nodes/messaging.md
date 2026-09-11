---
title: "Messaging Nodes"
description: "Messaging nodes send notifications to chat platforms."
keywords: "m9m nodes, n8n nodes, workflow nodes, HTTP, database, AI, messaging, integrations"
---

# Messaging Nodes

Messaging nodes send notifications to chat platforms.

## Slack Node

Send messages to Slack channels and users.

### Type

```
n8n-nodes-base.slack
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `webhookUrl` | string | No* | Slack webhook URL |
| `token` | string | No* | Slack API token |
| `text` | string | Yes | Message content |
| `channel` | string | Depends | Channel (required with token) |
| `username` | string | No | Bot display name |

*Either `webhookUrl` OR `token` required.

### Using Webhook URL

The simplest method - create an [Incoming Webhook](https://api.slack.com/messaging/webhooks) in Slack:

```json
{
  "id": "slack-1",
  "name": "Send to Slack",
  "type": "n8n-nodes-base.slack",
  "position": [450, 300],
  "parameters": {
    "webhookUrl": "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXX",
    "text": "New order received: {{ $json.orderId }}"
  }
}
```

### Using API Token

For more control, use a [Slack App](https://api.slack.com/apps) with Bot Token:

```json
{
  "type": "n8n-nodes-base.slack",
  "parameters": {
    "token": "={{ $credentials.slackApi.token }}",
    "channel": "#notifications",
    "text": "Alert: {{ $json.message }}",
    "username": "m9m Bot"
  }
}
```

### Output

```json
{
  "json": {
    "success": true,
    "method": "webhook"
  }
}
```

### Examples

#### Simple Notification

```json
{
  "type": "n8n-nodes-base.slack",
  "parameters": {
    "webhookUrl": "https://hooks.slack.com/services/...",
    "text": "Workflow completed successfully!"
  }
}
```

#### Dynamic Message

```json
{
  "type": "n8n-nodes-base.slack",
  "parameters": {
    "webhookUrl": "https://hooks.slack.com/services/...",
    "text": "New user signup:\n- Name: {{ $json.name }}\n- Email: {{ $json.email }}\n- Plan: {{ $json.plan }}"
  }
}
```

#### With Custom Bot Name

```json
{
  "type": "n8n-nodes-base.slack",
  "parameters": {
    "token": "xoxb-...",
    "channel": "#alerts",
    "text": "Server alert: High CPU usage detected",
    "username": "Server Monitor"
  }
}
```

### Use Cases

| Use Case | Example Message |
|----------|-----------------|
| Error alerts | `Error in workflow: {{ $json.error }}` |
| New signups | `New user: {{ $json.email }}` |
| Order notifications | `Order #{{ $json.id }} received` |
| Deployment notices | `Deployed {{ $json.version }} to production` |

---

## Discord Node

Send messages to Discord channels via webhooks.

### Type

```
n8n-nodes-base.discord
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `webhookUrl` | string | Yes | - | Discord webhook URL |
| `content` | string | Yes | - | Message content |
| `username` | string | No | `m9m Bot` | Bot display name |
| `avatarUrl` | string | No | - | Bot avatar URL |

### Creating a Discord Webhook

1. Go to Server Settings → Integrations → Webhooks
2. Click "New Webhook"
3. Copy the webhook URL

### Examples

#### Simple Message

```json
{
  "id": "discord-1",
  "name": "Send to Discord",
  "type": "n8n-nodes-base.discord",
  "position": [450, 300],
  "parameters": {
    "webhookUrl": "https://discord.com/api/webhooks/...",
    "content": "Hello from m9m!"
  }
}
```

#### Dynamic Alert

```json
{
  "type": "n8n-nodes-base.discord",
  "parameters": {
    "webhookUrl": "https://discord.com/api/webhooks/...",
    "content": "**Alert**: {{ $json.alertType }}\n\nDetails: {{ $json.details }}",
    "username": "Alert Bot"
  }
}
```

#### With Custom Avatar

```json
{
  "type": "n8n-nodes-base.discord",
  "parameters": {
    "webhookUrl": "https://discord.com/api/webhooks/...",
    "content": "Build {{ $json.buildNumber }} completed",
    "username": "CI/CD Bot",
    "avatarUrl": "https://example.com/bot-avatar.png"
  }
}
```

### Output

```json
{
  "json": {
    "success": true,
    "statusCode": 204
  }
}
```

### Formatting

Discord supports Markdown:

```json
{
  "content": "**Bold** and *italic* text\n\n```json\n{\"code\": \"block\"}\n```\n\n> Quote"
}
```

---

## Quick Reference

| Node | Type | Auth Method |
|------|------|-------------|
| Slack | `n8n-nodes-base.slack` | Webhook URL or API Token |
| Discord | `n8n-nodes-base.discord` | Webhook URL |
| Twilio | `n8n-nodes-base.twilio` | Account SID + Auth Token |
| Microsoft Teams | `n8n-nodes-base.microsoftTeams` | Incoming Webhook URL or OAuth |

### When to Use Each

| Scenario | Recommended |
|----------|-------------|
| Simple notifications | Webhook URL |
| Channel selection | API Token (Slack |
| Rich formatting | All support Markdown |
| File attachments | API Token (requires additional setup) |
| SMS / voice alerts | Twilio |
| Office 365 alerts | Microsoft Teams |

---

## Twilio Node

Send SMS messages or trigger Twilio Programmable Voice calls.

### Type

```
n8n-nodes-base.twilio
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `accountSid` | string | Yes | Twilio account SID |
| `authToken` | string | Yes | Twilio auth token |
| `from` | string | Yes | Sender phone number (E.164 format) |
| `to` | string | Yes | Destination phone number |
| `body` | string | Yes | SMS message body |
| `operation` | string | No | `sms` (default) or `call` |

### Example

```json
{
  "type": "n8n-nodes-base.twilio",
  "parameters": {
    "accountSid": "={{ $credentials.twilio.accountSid }}",
    "authToken": "={{ $credentials.twilio.authToken }}",
    "from": "+15551234567",
    "to": "={{ $json.recipientPhone }}",
    "body": "Alert: {{ $json.message }}"
  }
}
```

---

## MicrosoftTeams Node

Post adaptive cards or text messages to a Teams channel.

### Type

```
n8n-nodes-base.microsoftTeams
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `webhookUrl` | string | Yes* | Incoming webhook URL for the channel |
| `messageType` | string | Yes | `text` or `adaptiveCard` |
| `message` | string | When `messageType=text` | Plain-text body |
| `card` | object | When `messageType=adaptiveCard` | Adaptive card payload |

### Example — text

```json
{
  "type": "n8n-nodes-base.microsoftTeams",
  "parameters": {
    "webhookUrl": "={{ $credentials.teamsIncomingWebhook.url }}",
    "messageType": "text",
    "message": "Build finished: {{ $json.status }}"
  }
}
```

### Example — adaptive card

```json
{
  "type": "n8n-nodes-base.microsoftTeams",
  "parameters": {
    "webhookUrl": "={{ $credentials.teamsIncomingWebhook.url }}",
    "messageType": "adaptiveCard",
    "card": "={{ { type: 'AdaptiveCard', version: '1.4', body: [{ type: 'TextBlock', text: $json.title, weight: 'Bolder' }] } }}"
  }
}
```
