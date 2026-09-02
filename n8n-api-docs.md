# n8n Public API & Webhook Reference

Captured 2026-09-02 from `https://docs.n8n.io/connect/n8n-api/workflow`, `https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/`, and live verification against `http://187.77.113.218:5678/`.

The `/api/v1/workflows` doc page itself is a 404; the canonical API reference now lives at `https://docs.n8n.io/connect/n8n-api/workflow/` (returns OpenAPI 3.0.1 spec, `n8n Public API` v1.1.1).

---

## Authentication

```http
X-N8N-API-KEY: <key>
```

- API key is created in the n8n UI (Settings → n8n API) and is the recommended auth scheme.
- Bearer JWT (the same JWT issued at login) and the `n8n-auth` cookie are also accepted by the same endpoints, but `X-N8N-API-KEY` is what to expose externally.
- Header is mandatory for every `/api/v1/*` request below; missing it returns `401 Unauthorized`.

---

## Workflow CRUD endpoints

Base path: `/api/v1`. All paths are relative to the n8n base URL (e.g. `http://localhost:5678/api/v1`).

### GET `/workflows` — Retrieve all workflows
- Query params: `offset` (default 0), `limit` (default 100, max 250), `cursor` (pagination cursor returned as `nextCursor`), `active` (`true|false`), `tags`, `name`, `projectId`, `excludePinnedData` (default `false`).
- Response 200: `{ "data": [Workflow…], "nextCursor": string|null }`.

### GET `/workflows/{workflowId}` — Retrieve a workflow
- Query: `excludePinnedData` (default `false`).
- Response 200: full `Workflow` object.

### GET `/workflows/{workflowId}/{versionId}` — Retrieve a specific workflow version
- Response 200: `WorkflowVersion` (nodes + connections + authors + timestamps for that snapshot).

### GET `/workflows/{workflowId}/history` — Retrieve workflow version history
- Query: `limit`, `cursor`.
- Response 200: `{ "data": [VersionMetadata…], "nextCursor": string|null }`.

### GET `/workflows/{workflowId}/tags` — Get workflow tags

### POST `/workflows` — Create a workflow
- Body: `{ name, nodes, connections, settings, … }`. `name`, `nodes`, `connections`, `settings` are required. `id`, `active`, `createdAt`, `updatedAt`, `isArchived`, `versionId`, `triggerCount`, `tags`, `shared`, `activeVersion`, `meta` are read-only on input.
- Optional: `projectId`, `parentFolderId`, `staticData`, `pinData`, `nodeGroups`, `description`.
- Response 200: the created `Workflow` (with generated `id`, `versionId`, `versionCounter`, …).
- Errors: 400, 401, 403, 404, 415, 422.

### PUT `/workflows/{workflowId}` — Update a workflow
- Query: `publishIfActive` (`true|false`, default `true`). When the workflow is currently published, the update is auto-published unless `publishIfActive=false` (saves as a draft on the existing published version).
- Body: same shape as POST (without read-only fields).
- Response 200: updated `Workflow`.
- 409 on webhook path conflicts or open workflow review (`{ message, reason: "review_pending"|"changes_requested", workflowReviewRequestId }`).

### DELETE `/workflows/{workflowId}` — Delete a workflow
- Hard delete. Response 200: deleted `Workflow` object.
- 404 if not found; 409 on conflict.

### POST `/workflows/{workflowId}/archive` — Archive (soft-delete)
- Idempotent — archiving an already-archived workflow returns 200 with the current workflow.
- Response 200: `Workflow`.

### POST `/workflows/{workflowId}/unarchive` — Unarchive
- Response 200: `Workflow`.

### PUT `/workflows/{workflowId}/transfer` — Transfer to another project
- Body: `{ "destinationProjectId": "<projectId>" }`.
- Response 204.

### POST `/workflows/{workflowId}/publish` — Publish (activate) a workflow
- Optional body: `{ versionId?, name?, description? }`. If `versionId` is omitted, the latest version is published.
- Response 200: `WorkflowPublishPublicDto` (includes `workflowPublishHistory`).
- 409 on review pending / path conflict.

### POST `/workflows/{workflowId}/unpublish` — Unpublish (deactivate)

### ⚠️ Deprecated
- `POST /workflows/{workflowId}/activate` — use `/publish` instead.
- `POST /workflows/{workflowId}/deactivate` — use `/unpublish` instead.

### PUT `/workflows/{workflowId}/tags` — Update workflow tags
- Body: array of `{ id }`.

---

## Workflow execution endpoints

### ❌ There is no `POST /workflows/{id}/execute` endpoint

Modern n8n (v1.x) intentionally removed the public "execute by API call" endpoint. Webhook triggers are the supported way to invoke a workflow over HTTP. Confirmed against live `187.77.113.218:5678`:

```http
POST /api/v1/workflows/7jvsinmQA292FaD7/execute  →  HTTP 405 Method Not Allowed
```

### Execution history API (separate sub-resource)

The execution-listing endpoints live under `/api/v1/executions` (confirmed 200 on the live instance) — they are documented as a sibling tag "Execution" in the same OpenAPI spec but not in the workflow page itself. Typical operations:

- `GET /api/v1/executions` — list executions (`includeData`, `status`, `workflowId`, `projectId`, `limit`, `cursor`).
- `GET /api/v1/executions/{id}` — get one execution with full node run data.
- `DELETE /api/v1/executions/{id}` — delete an execution.

Webhook deliveries are themselves executions; their `data` payload is the Webhook node's `json` body plus the last node's items when `respond: "lastNode"`.

### Triggering execution today (supported paths)

1. **Production webhook**: `POST {baseUrl}/webhook/{path}` (when workflow is published). This is the official "API execution" surface.
2. **Test webhook**: `POST {baseUrl}/webhook-test/{path}` (when "Listen for test event" is active in the editor).
3. **MCP** (when `settings.availableInMCP=true` and the workflow is published): the workflow becomes an MCP tool on the built-in n8n MCP server.
4. **Internal Execute Workflow node**: `Execute Workflow` sub-node parameter `callerPolicy` controls which workflows may invoke another.

> **Implication for m9m:** to stay drop-in compatible, m9m must NOT advertise a `POST /workflows/{id}/execute` endpoint. Document and implement webhook execution instead. If we want to expose a programmatic "run" API, model it after n8n's webhook shape — invoke the trigger node by its webhook URL, not a dedicated /execute path.

---

## Webhook HTTP response shape (critical)

Source: `https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/` plus a live POST to `POST /webhook/bocahtuanakal` on `187.77.113.218:5678`.

The Webhook node has two URLs:

- **Test**: registered when the editor is in "Listen for Test Event" mode. Data is shown in the editor.
- **Production**: registered when the workflow is published (`active=true`). Data is not shown in the editor but is visible on the Executions tab.

### Default HTTP methods supported

`DELETE`, `GET`, `HEAD`, `PATCH`, `POST`, `PUT`. Default method is `POST`.

### Path

By default a random path; can be set to literal or templated patterns:

- `/:variable`
- `/path/:variable`
- `/:variable/path`
- `/:variable1/path/:variable2`
- `/:variable1/:variable2`

### Respond modes (drives response shape)

The `Respond` parameter on the Webhook node controls what the caller receives:

| Respond mode                              | HTTP status         | Body                                                                                |
|-------------------------------------------|---------------------|-------------------------------------------------------------------------------------|
| `Immediately`                             | `Response Code` (default 200) | `{"message":"Workflow got started"}`                                            |
| `When Last Node Finishes`                 | `Response Code`     | Determined by `Response Data` (see below)                                            |
| `Using 'Respond to Webhook' Node`         | As configured       | As configured                                                                        |
| `Streaming response`                      | Chunked             | Streams node output as the workflow runs (requires streaming-capable nodes)          |

### Response Data (only with `Respond = When Last Node Finishes`)

- **All Entries** — `[{...}, {...}]` JSON array of every entry from the last node.
- **First Entry JSON** — `{...}` JSON object (the first entry only).
- **First Entry Binary** — the binary file from the first entry (no JSON wrapper).
- **No Response Body** — status code only.

### Verified sync example (live `187.77.113.218:5678`)

Request:

```http
POST /webhook/bocahtuanakal HTTP/1.1
Content-Type: application/json

{"varA":1,"varB":2}
```

Response (the test workflow sums two numbers):

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Content-Length: 13

[{"total":3}]
```

This matches `responseMode: "lastNode"` + `responseData: "allEntries"` and confirms the body is a **JSON array** of the last node's data items.

### Async / queue response

When Respond is `Immediately`, the body is `{"message":"Workflow got started"}` with the configured status code (typically 200). The workflow continues running in the background; the caller does not get its output. Confirmed by docs (this is the documented message string). If m9m needs to queue executions, this is the body to emit.

### Other Webhook options worth knowing

- `Allowed Origins (CORS)` — defaults to `*`.
- `Binary Property` — required if the request body is binary and `httpMethod` is `POST|PATCH|PUT`.
- `Ignore Bots` — drop link previewers/crawlers.
- `IP(s) Allowlist` — comma-separated; non-matching IPs get 403.
- `No Response Body` — works with `Respond: Immediately`.
- `Only Run If` — expression evaluated against `{ body, headers, params, query }`. Non-matching requests get `200` with no execution created.
- `Raw Body` — receive raw JSON/XML.
- `Response Content-Type` — JSON / string / etc. Applies to `When Last Node Finishes` + `First Entry JSON`.
- `Response Data` — custom response for `Respond: Immediately`.
- `Response Headers` — extra response headers.
- `Property Name` — return a specific JSON key from the last node.
- Max payload: 16 MB (`N8N_PAYLOAD_SIZE_MAX` env).
- HTML responses are auto-wrapped in `<iframe sandbox>` starting n8n 1.103.0 (sandboxed JS, no top-level storage access, no auth headers in iframe — relative URLs break).

### Authentication for webhook URLs (separate from API key)

- None / Basic auth / Header auth / JWT auth.
- These are configured in Webhook credentials, NOT via `X-N8N-API-KEY`.

---

## Live API key verification

```http
GET /api/v1/workflows HTTP/1.1
Host: 187.77.113.218:5678
X-N8N-API-KEY: eyJhbGc…
Accept: application/json
```

Returned `200 OK` with `{"data":[…6 workflows…],"nextCursor":null}`. The first workflow in the response was:

```json
{
  "id":"7jvsinmQA292FaD7",
  "name":"Simple Webhook - Set Condition",
  "active":true,
  "activeVersionId":"96d9c841-0cfc-4d0c-89d4-579208a47190",
  "createdAt":"2026-09-02T11:53:59.098Z",
  "updatedAt":"2026-09-02T11:57:58.047Z",
  "isArchived":false,
  "versionId":"96d9c841-0cfc-4d0c-89d4-579208a47190",
  "triggerCount":1,
  "nodes":[…],
  "connections":{…},
  "nodeGroups":[],
  "settings":{"executionOrder":"v1","binaryMode":"separate"},
  "staticData":null,
  "pinData":{},
  "meta":null,
  "tags":[],
  "shared":[…],
  "activeVersion":{…}
}
```

Six workflows are visible on the test instance (`Simple Webhook - Set Condition`, `Simple Webhook`, `Simple Webhook - Set`, `Simple Workflow - Set Calculation`, `Simple CronWorkflow`, `Webhook Processing`). Three of them use `responseMode: "lastNode"` with `responseData: "allEntries"` (which is what produced the array response above).

---

## Discrepancies vs. user's framing

1. **The task description references `POST /workflows/{id}/execute` — that endpoint does NOT exist in current n8n.** Live test: `405 Method Not Allowed`. The docs page the user pointed at (`connect/n8n-api/workflow`) is a modern reference with no execute endpoint at all. The traditional `POST /workflows/{id}/execute` was removed from n8n in v1.x; "execution via API" now means invoking the Webhook URL, not calling a dedicated execute endpoint.
2. **`/api/v1/workflows` and `/api/v1/workflow-execution` are 404** — the doc has been consolidated to `/connect/n8n-api/workflow`. The OpenAPI spec dumped there covers only Workflow CRUD plus a separate Execution tag (which is `GET/DELETE /executions`, not "execute a workflow").
3. **Webhook async message is `"Workflow got started"`** (not `"Workflow was started"` as in the user's brief). The user-quoted variant may come from older n8n versions or from translations; the current docs and source string are `Workflow got started`.
4. **Live sync webhook body is a JSON array `[{"total":3}]`** when `responseMode=lastNode` + `responseData=allEntries` (single-entry workflows still wrap the single entry in an array). With `responseData=firstEntryJSON` the body would be a bare object.

---

## Local infrastructure (test rig)

- Real n8n instance: `http://187.77.113.218:5678/` — base URL for cross-comparison testing.
- m9m clone under test: `http://187.77.113.218:8080/` — should answer the same `/api/v1/*` and `/webhook/*` shapes.
- API key (kept out of source code; only in this doc and the memories file): see `C:\Users\HYPE R Series\OneDrive\workspace\m9m\.claude\memories\n8n-test-instance.md`.
