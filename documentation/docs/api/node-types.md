---
title: "Node Types API"
description: "List and inspect the node executors that the running m9m process has registered."
keywords: "m9m API, REST API, node types, executors"
---

# Node Types API

Inspect what node executors the running m9m process has loaded. The list reflects the union of all `RegisterNodeExecutor` calls in the binary.

---

## List Node Types

```http
GET /api/v1/node-types
```

### Query Parameters

| Parameter | Type | Description |
|---|---|---|
| `category` | string | Filter by category (e.g. `transform`, `database`) |
| `search` | string | Case-insensitive substring match against `name` or `type` |

### Example Request

```bash
curl http://localhost:8080/api/v1/node-types \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "data": [
    {
      "name": "Start",
      "type": "n8n-nodes-base.start",
      "displayName": "Start",
      "description": "Workflow initiation node",
      "category": "core",
      "version": 1,
      "inputs": ["main"],
      "outputs": ["main"],
      "parameters": [],
      "credentials": []
    },
    {
      "name": "HTTP Request",
      "type": "n8n-nodes-base.httpRequest",
      "displayName": "HTTP Request",
      "description": "Make HTTP requests",
      "category": "http",
      "version": 1,
      "inputs": ["main"],
      "outputs": ["main", "main_error"],
      "parameters": [
        { "name": "method", "type": "options", "default": "GET" },
        { "name": "url",   "type": "string",  "required": true }
      ],
      "credentials": ["httpBasicAuth", "httpHeaderAuth", "oAuth2Api"]
    }
  ],
  "total": 42
}
```

### Filtered example — only AI nodes

```bash
curl "http://localhost:8080/api/v1/node-types?category=ai" \
  -H "Authorization: Bearer <token>"
```

---

## Get a Single Node Type

```http
GET /api/v1/node-types/{type}
```

Where `{type}` is the n8n type identifier, URL-encoded. For example, `n8n-nodes-base.openAi` → `n8n-nodes-base.openAi` (no encoding needed; the only reserved char in this name is the `.`, which is allowed in path segments).

### Example Request

```bash
curl http://localhost:8080/api/v1/node-types/n8n-nodes-base.httpRequest \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "name": "HTTP Request",
  "type": "n8n-nodes-base.httpRequest",
  "displayName": "HTTP Request",
  "description": "Make HTTP requests",
  "category": "http",
  "version": 1,
  "inputs": ["main"],
  "outputs": ["main", "main_error"],
  "parameters": [
    {
      "name": "method",
      "type": "options",
      "default": "GET",
      "options": [
        { "value": "GET",    "description": "GET" },
        { "value": "POST",   "description": "POST" },
        { "value": "PUT",    "description": "PUT" },
        { "value": "PATCH",  "description": "PATCH" },
        { "value": "DELETE", "description": "DELETE" }
      ]
    },
    { "name": "url", "type": "string", "required": true },
    { "name": "sendHeaders", "type": "boolean", "default": false },
    { "name": "headers", "type": "fixedCollection", "displayName": "Headers" },
    { "name": "sendBody", "type": "boolean", "default": false },
    { "name": "body", "type": "string", "displayName": "Body" },
    { "name": "options", "type": "fixedCollection", "displayName": "Options" }
  ],
  "credentials": [
    { "name": "httpBasicAuth",   "required": false },
    { "name": "httpHeaderAuth",  "required": false },
    { "name": "httpQueryAuth",   "required": false },
    { "name": "oAuth2Api",       "required": false }
  ]
}
```

### 404 — unknown type

```json
{ "error": true, "message": "Node type not found", "code": 404 }
```

---

## Notes

- The response shape is the **static** part of the descriptor (parameters, credentials, IO). The runtime-computed metadata (in-flight stats, span info) is not included.
- The `category` field is sourced from the node's registration block in `cmd/m9m/commands/exec.go`.
- Versioning: `version` is the n8n-style semver the node reports; m9m currently exposes v1 of every node, but the field exists for forward compatibility.

### Use cases

- Drive a UI schema-from-API renderer (the web editor does this).
- Validate a workflow's `type` references against the running binary before saving.
- Build tooling that lists "all HTTP-capable nodes" or "all AI nodes" for governance reports.
