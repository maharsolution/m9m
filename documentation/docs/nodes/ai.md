---
title: "AI & LLM Nodes"
description: "AI nodes integrate with large language models for text generation and analysis."
keywords: "m9m nodes, n8n nodes, workflow nodes, HTTP, database, AI, messaging, integrations"
---

# AI & LLM Nodes

AI nodes integrate with large language models for text generation and analysis.

## OpenAI Node

Interact with OpenAI's GPT models.

### Type

```
n8n-nodes-base.openAi
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `apiKey` | string | Yes | - | OpenAI API key |
| `model` | string | No | `gpt-3.5-turbo` | Model name |
| `prompt` | string | Yes | - | Input prompt |
| `maxTokens` | number | No | 1000 | Max response tokens |
| `temperature` | number | No | 0.7 | Randomness (0-2) |

### Models

| Model | Description |
|-------|-------------|
| `gpt-4` | Most capable, slower |
| `gpt-4-turbo` | Fast GPT-4 |
| `gpt-3.5-turbo` | Fast, cost-effective |

### Examples

#### Basic Completion

```json
{
  "id": "openai-1",
  "name": "Generate Text",
  "type": "n8n-nodes-base.openAi",
  "position": [450, 300],
  "parameters": {
    "apiKey": "={{ $credentials.openai.apiKey }}",
    "model": "gpt-3.5-turbo",
    "prompt": "Summarize this article: {{ $json.content }}",
    "maxTokens": 500
  }
}
```

#### Dynamic Prompt

```json
{
  "type": "n8n-nodes-base.openAi",
  "parameters": {
    "apiKey": "={{ $credentials.openai.apiKey }}",
    "model": "gpt-4",
    "prompt": "Analyze the following customer feedback and extract:\n1. Sentiment (positive/negative/neutral)\n2. Key topics\n3. Action items\n\nFeedback: {{ $json.feedback }}",
    "maxTokens": 1000,
    "temperature": 0.3
  }
}
```

#### Code Generation

```json
{
  "type": "n8n-nodes-base.openAi",
  "parameters": {
    "apiKey": "={{ $credentials.openai.apiKey }}",
    "model": "gpt-4",
    "prompt": "Write a {{ $json.language }} function that {{ $json.description }}",
    "maxTokens": 2000,
    "temperature": 0.2
  }
}
```

### Output

```json
{
  "json": {
    "response": "Generated text content...",
    "usage": {
      "prompt_tokens": 50,
      "completion_tokens": 150,
      "total_tokens": 200
    },
    "finish_reason": "stop"
  }
}
```

| Field | Description |
|-------|-------------|
| `response` | Generated text |
| `usage` | Token usage statistics |
| `finish_reason` | Why generation stopped |

### Temperature Guide

| Temperature | Use Case |
|-------------|----------|
| 0.0 - 0.3 | Factual, deterministic |
| 0.4 - 0.7 | Balanced creativity |
| 0.8 - 1.2 | Creative writing |
| 1.3 - 2.0 | Highly random |

---

## Anthropic Node

Interact with Anthropic's Claude models.

### Type

```
n8n-nodes-base.anthropic
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `apiKey` | string | Yes | - | Anthropic API key |
| `model` | string | No | `claude-3-5-sonnet-20241022` | Model name |
| `prompt` | string | Yes | - | Input prompt |
| `maxTokens` | number | No | 1024 | Max response tokens |
| `temperature` | number | No | 1.0 | Randomness (0-1) |

### Models

| Model | Description |
|-------|-------------|
| `claude-3-5-sonnet-20241022` | Latest, balanced |
| `claude-3-opus-20240229` | Most capable |
| `claude-3-sonnet-20240229` | Fast, capable |
| `claude-3-haiku-20240307` | Fastest, economical |

### Examples

#### Basic Completion

```json
{
  "id": "anthropic-1",
  "name": "Claude Response",
  "type": "n8n-nodes-base.anthropic",
  "position": [450, 300],
  "parameters": {
    "apiKey": "={{ $credentials.anthropic.apiKey }}",
    "model": "claude-3-5-sonnet-20241022",
    "prompt": "Explain {{ $json.topic }} in simple terms.",
    "maxTokens": 500
  }
}
```

#### Analysis Task

```json
{
  "type": "n8n-nodes-base.anthropic",
  "parameters": {
    "apiKey": "={{ $credentials.anthropic.apiKey }}",
    "model": "claude-3-opus-20240229",
    "prompt": "Analyze this document and provide:\n1. Summary (2-3 sentences)\n2. Key points\n3. Recommendations\n\nDocument:\n{{ $json.document }}",
    "maxTokens": 2000,
    "temperature": 0.5
  }
}
```

#### Data Extraction

```json
{
  "type": "n8n-nodes-base.anthropic",
  "parameters": {
    "apiKey": "={{ $credentials.anthropic.apiKey }}",
    "model": "claude-3-sonnet-20240229",
    "prompt": "Extract the following information from this email as JSON:\n- sender_name\n- subject\n- action_items (array)\n- urgency (low/medium/high)\n\nEmail:\n{{ $json.emailBody }}\n\nRespond only with valid JSON.",
    "maxTokens": 500,
    "temperature": 0.1
  }
}
```

### Output

```json
{
  "json": {
    "response": "Generated response text...",
    "usage": {
      "input_tokens": 100,
      "output_tokens": 200
    },
    "stop_reason": "end_turn"
  }
}
```

---

## Common Patterns

### Prompt Templates

Store prompts in variables:

```json
{
  "type": "n8n-nodes-base.set",
  "parameters": {
    "assignments": [
      {
        "name": "prompt",
        "value": "You are a helpful assistant. User query: {{ $json.userInput }}"
      }
    ]
  }
}
```

### Error Handling

Check for API errors:

```json
{
  "type": "n8n-nodes-base.filter",
  "parameters": {
    "conditions": [
      {
        "leftValue": "={{ $json.error }}",
        "operator": "notExists"
      }
    ]
  }
}
```

### Response Parsing

Parse JSON from AI response:

```json
{
  "type": "n8n-nodes-base.code",
  "parameters": {
    "language": "javascript",
    "code": "const response = items[0].json.response;\ntry {\n  return [{json: JSON.parse(response)}];\n} catch(e) {\n  return [{json: {raw: response, parseError: true}}];\n}"
  }
}
```

---

## Quick Reference

| Node | Type | Models |
|------|------|--------|
| OpenAI | `n8n-nodes-base.openAi` | GPT-4, GPT-3.5-turbo |
| Anthropic | `n8n-nodes-base.anthropic` | Claude 3.5, Claude 3 |

### Use Cases

| Use Case | Recommended | Model |
|----------|-------------|-------|
| Summarization | Either | Sonnet/GPT-4 |
| Code generation | OpenAI | GPT-4 |
| Analysis | Anthropic | Opus |
| Quick responses | Either | Haiku/GPT-3.5 |
| Data extraction | Either | Sonnet/GPT-4 |

---

## In-app AI assistant (Settings → AI)

The AI assistant is a separate, in-app surface from the OpenAI / Anthropic *workflow* nodes. It powers "Ask AI to fix this", "Suggest next node", and the chat panel inside the workflow editor.

The assistant's provider, model, API key, and base URL are all configurable from **Settings → AI** in the web UI. The configuration is layered:

1. **Env defaults** — `M9M_AI_*` (or legacy `M9M_COPILOT_*`) read at boot.
2. **DB-stored override** — what you save in Settings; persists across restarts.
3. **Live UI toggle** — the on/off switch in Settings.

> The Settings value wins over env. Env wins over zero. See [`api/ai.md`](../api/ai.md) for the underlying `/api/v1/ai` endpoints.

### Supported providers

| Provider | Default `baseURL` | Default models |
|---|---|---|
| OpenAI | `https://api.openai.com/v1` | `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo` |
| Anthropic | `https://api.anthropic.com` | `claude-3-5-sonnet-latest`, `claude-3-5-haiku-latest`, `claude-3-opus-20240229` |
| MiniMax | `https://api.minimax.chat/v1` | `MiniMax-Plus`, `MiniMax-Pro`, `MiniMax` |
| Ollama (local) | `http://localhost:11434/v1` | Anything your local Ollama has pulled |

Override `baseURL` if you run an OpenAI-compatible gateway, a self-hosted Anthropic proxy, or a regional Anthropic endpoint.

### MiniMax

MiniMax exposes an OpenAI-compatible chat-completions endpoint. m9m detects a MiniMax base URL by lowercased substring (`minimax.chat` or `minimax.`) and routes the call accordingly, so the assistant and the Anthropic workflow node can talk to MiniMax without code changes.

Configuration:

| Env var | Purpose |
|---|---|
| `M9M_AI_PROVIDER=minimax` | Selects the MiniMax provider preset |
| `M9M_AI_BASE_URL=https://api.minimax.chat/v1` | Override the default |
| `M9M_AI_API_KEY=...` | MiniMax API key |
| `M9M_AI_MODEL=MiniMax-Plus` | Model name from MiniMax's catalog |

Or in Settings → AI: choose the **MiniMax** provider preset, paste the key, pick a model.

### Curl — read effective config

```bash
curl http://localhost:8080/api/v1/ai
```

### Curl — update config

```bash
curl -X PUT http://localhost:8080/api/v1/ai \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "minimax",
    "apiKey":  "sk-...",
    "model":   "MiniMax-Plus",
    "baseUrl": "https://api.minimax.chat/v1",
    "enabled": true
  }'
```

### Curl — test chat

```bash
curl -X POST http://localhost:8080/api/v1/ai/test \
  -H "Content-Type: application/json" \
  -d '{"message": "Reply with the word OK."}'
```

> Legacy `M9M_COPILOT_*` env vars (`M9M_COPILOT_PROVIDER`, `M9M_COPILOT_API_KEY`, etc.) remain honoured as aliases of `M9M_AI_*` so existing deployments continue to work. They will be removed in a future major release.

### Related

- [API reference for `/api/v1/ai/*`](../api/ai.md)
- [Environment variables](../configuration/environment.md#ai-assistant)
