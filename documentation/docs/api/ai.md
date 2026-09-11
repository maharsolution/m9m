---
title: "AI Assistant API"
description: "Endpoints that power the in-app AI assistant (renamed from Copilot in v0.2.0)."
keywords: "m9m API, AI assistant, Copilot, OpenAI, Anthropic, MiniMax, Ollama"
---

# AI Assistant API

The AI assistant powers "Ask AI to fix this", "Suggest next node", and the chat panel in the workflow editor. It was renamed from "Copilot" to "AI" in `0.2.0-parity` (2026-09-10); the `/api/v1/copilot/*` routes now redirect to the `/api/v1/ai/*` equivalents.

For the user-facing guide to provider setup, see [nodes/ai.md → In-app AI assistant](../nodes/ai.md#in-app-ai-assistant-settings--ai).

---

## Provider config

### Get the effective provider config

```http
GET /api/v1/ai
```

### Example

```bash
curl http://localhost:8080/api/v1/ai \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "provider": "anthropic",
  "model":    "claude-3-5-sonnet-latest",
  "baseUrl":  "https://api.anthropic.com",
  "apiKey":   "***",
  "enabled":  true,
  "source":   "db_override"
}
```

`source` is one of `defaults`, `env`, `db_override`. `apiKey` is always masked on read; pass it back to keep.

### Update provider config

```http
PUT /api/v1/ai
```

```bash
curl -X PUT http://localhost:8080/api/v1/ai \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "minimax",
    "model":    "MiniMax-Plus",
    "baseUrl":  "https://api.minimax.chat/v1",
    "apiKey":   "sk-...",
    "enabled":  true
  }'
```

### Clear DB override

```http
DELETE /api/v1/ai
```

Reverts to env-merged defaults.

---

## Provider catalog

The available providers are hardcoded in the binary (see [`internal/ai/providers.go`](https://github.com/maharsolution/m9m)). Adding a new provider is an engineering change, not a config change.

| Provider | Default `baseUrl` | Default models |
|---|---|---|
| `openai` | `https://api.openai.com/v1` | `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo` |
| `anthropic` | `https://api.anthropic.com` | `claude-3-5-sonnet-latest`, `claude-3-5-haiku-latest`, `claude-3-opus-20240229` |
| `minimax` | `https://api.minimax.chat/v1` | `MiniMax-Plus`, `MiniMax-Pro`, `MiniMax` |
| `ollama` | `http://localhost:11434/v1` | Whatever your local Ollama has pulled |

Override `baseUrl` for OpenAI-compatible gateways, regional Anthropic endpoints, self-hosted proxies, etc.

### MiniMax

MiniMax exposes an OpenAI-compatible chat-completions endpoint at `https://api.minimax.chat/v1`. The `internal/ai` package detects the MiniMax base URL by lowercased substring and routes accordingly. No code change needed to add MiniMax — just configure `provider=minimax` and a MiniMax API key.

---

## Generation endpoints

### Generate a workflow from a prompt

```http
POST /api/v1/ai/generate
```

### Request body

| Field | Type | Required | Description |
|---|---|---|---|
| `prompt` | string | Yes | Natural-language description of the workflow |
| `context` | object | No | Optional context (existing nodes, credentials, constraints) |

### Response

```json
{
  "workflow": {
    "name": "Daily GitHub digest",
    "nodes": [
      { "id": "cron",  "type": "n8n-nodes-base.cron", "position": [250, 300] },
      { "id": "gh",    "type": "n8n-nodes-base.github", "position": [450, 300] },
      { "id": "email", "type": "n8n-nodes-base.sendGrid", "position": [650, 300] }
    ],
    "connections": { "cron": { "main": [[{ "node": "gh", "type": "main", "index": 0 }]] } }
  },
  "explanation": "Cron triggers daily; GitHub node lists PRs; SendGrid emails the digest."
}
```

### Suggest the next node

```http
POST /api/v1/ai/suggest
```

```json
{
  "existingNodes": [
    { "id": "cron",  "type": "n8n-nodes-base.cron" },
    { "id": "gh",    "type": "n8n-nodes-base.github" }
  ]
}
```

### Response

```json
{
  "suggestions": [
    { "type": "n8n-nodes-base.sendGrid", "reason": "Send the digest by email" },
    { "type": "n8n-nodes-base.slack",    "reason": "Post to Slack instead" },
    { "type": "n8n-nodes-base.httpRequest", "reason": "POST to a custom webhook" }
  ]
}
```

### Explain a failed execution

```http
POST /api/v1/ai/explain
```

```json
{
  "executionId": "exec-789",
  "nodeId": "sendgrid-1"
}
```

### Response

```json
{
  "summary": "SendGrid returned 401 Unauthorized — the API key is invalid or revoked.",
  "nextSteps": [
    "Open Settings → Credentials and re-issue the SendGrid key",
    "Or rotate the secret in your secret manager and re-import",
    "Then POST /api/v1/executions/exec-789/retry-node with nodeId=sendgrid-1"
  ]
}
```

### Suggest a fix

```http
POST /api/v1/ai/fix
```

Same request body as `/explain`. Returns a candidate patch (parameter changes or workflow-graph diff) the user can apply.

### Open-ended chat

```http
POST /api/v1/ai/chat
```

```json
{ "message": "How do I add a retry policy to my Postgres node?" }
```

### Response

```json
{ "reply": "Set the `retryOnFail` and `maxTries` parameters on the node. …" }
```

---

## Provider health

```http
GET /api/v1/ai/health
```

```bash
curl http://localhost:8080/api/v1/ai/health
```

### Response

```json
{
  "enabled": true,
  "provider": "anthropic",
  "model": "claude-3-5-sonnet-latest",
  "status": "ok",
  "lastCheck": "2026-09-11T08:30:00Z",
  "latencyMs": 412
}
```

`status` is `ok` if a recent test chat succeeded, `degraded` if it's been failing for >5 minutes, `disabled` if `enabled=false`.

---

## Test the configured provider

Emits a one-shot chat call to confirm the config works end-to-end without going through the UI.

```http
POST /api/v1/ai/test
```

```json
{ "message": "Reply with the word OK." }
```

```json
{ "ok": true, "reply": "OK", "provider": "minimax", "model": "MiniMax-Plus" }
```

---

## Legacy aliases

| Old route | Current route |
|---|---|
| `POST /api/v1/copilot/generate` | `POST /api/v1/ai/generate` |
| `POST /api/v1/copilot/suggest`  | `POST /api/v1/ai/suggest`  |
| `POST /api/v1/copilot/explain`  | `POST /api/v1/ai/explain`  |
| `POST /api/v1/copilot/fix`      | `POST /api/v1/ai/fix`      |
| `POST /api/v1/copilot/chat`     | `POST /api/v1/ai/chat`     |
| `GET  /api/v1/copilot/health`   | `GET  /api/v1/ai/health`   |

The old routes return `301` to the new path with `Location` header set. The internal package is `internal/ai` (was `internal/copilot`); legacy imports are still compiled but emit a deprecation log on first use.

Legacy env vars `M9M_COPILOT_PROVIDER`, `M9M_COPILOT_API_KEY`, `M9M_COPILOT_MODEL`, `M9M_COPILOT_BASE_URL`, `M9M_COPILOT_ENABLED` are honoured as aliases of `M9M_AI_*`.

---

## See also

- [User guide: AI assistant](../nodes/ai.md#in-app-ai-assistant-settings--ai)
- [Environment variables](../configuration/environment.md#ai-assistant)
