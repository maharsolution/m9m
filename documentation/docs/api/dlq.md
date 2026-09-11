---
title: "Dead-Letter Queue (DLQ) API"
description: "Inspect, retry, and discard executions that exhausted all retries."
keywords: "m9m API, REST API, dead letter queue, DLQ, retries"
---

# Dead-Letter Queue (DLQ) API

When a workflow execution exhausts its retry policy (or is permanently failing for a reason that won't fix itself), it ends up in the **dead-letter queue**. The DLQ is a holding pen: it does not run, retry, or delete automatically. Operators inspect it, decide whether the failure is recoverable, and either **retry** (re-queue for execution) or **discard** (drop permanently).

---

## List DLQ Items

```http
GET /api/v1/dlq
```

### Query Parameters

| Parameter | Type | Default | Description |
|---|---|---|---|
| `offset` | integer | `0` | Pagination offset |
| `limit` | integer | `50` | Items per page (max 200) |
| `workflowId` | string | — | Filter by workflow |
| `nodeId` | string | — | Filter by node that failed |
| `since` | datetime | — | Only items enqueued after this timestamp |
| `until` | datetime | — | Only items enqueued before this timestamp |

### Example Request

```bash
curl "http://localhost:8080/api/v1/dlq?limit=20&workflowId=wf-123" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "data": [
    {
      "id": "dlq-001",
      "executionId": "exec-789",
      "workflowId": "wf-123",
      "workflowName": "Daily Report",
      "nodeId": "sendgrid-1",
      "nodeName": "SendGrid",
      "nodeType": "n8n-nodes-base.sendGrid",
      "attempts": 5,
      "lastError": {
        "message": "401 Unauthorized",
        "stack": "..."
      },
      "enqueuedAt": "2024-01-26T16:00:00Z",
      "inputSnapshot": [{ "json": { "to": "x@y.com", "subject": "Daily" } }]
    }
  ],
  "total": 14,
  "offset": 0,
  "limit": 20
}
```

---

## Get a Single DLQ Item

```http
GET /api/v1/dlq/{id}
```

Returns the same shape as one element of the list, with an extra `payload` field containing the full execution record (input, output, all node executions).

```bash
curl http://localhost:8080/api/v1/dlq/dlq-001 \
  -H "Authorization: Bearer <token>"
```

---

## Retry a DLQ Item

Re-queue the execution for another run. Useful after you've fixed the underlying cause (e.g. rotated a credential, fixed the upstream API).

```http
POST /api/v1/dlq/{id}/retry
```

### Request Body

| Field | Type | Required | Description |
|---|---|---|---|
| `resetAttempts` | boolean | No | Default `true`. If `true`, the attempt counter resets to 1; if `false`, the existing count is preserved. |

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/dlq/dlq-001/retry \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"resetAttempts": true}'
```

### Response

```json
{
  "executionId": "exec-790",
  "previousExecutionId": "exec-789",
  "status": "running"
}
```

The original DLQ item is marked `retried` (and removed from the active list); a new execution is created.

---

## Discard a DLQ Item

Permanently drop the execution. Cannot be undone.

```http
POST /api/v1/dlq/{id}/discard
```

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/dlq/dlq-001/discard \
  -H "Authorization: Bearer <token>"
```

### Response

```
204 No Content
```

---

## DLQ Stats

Aggregate counts for the dashboard.

```http
GET /api/v1/dlq/stats
```

### Query Parameters

| Parameter | Type | Description |
|---|---|---|
| `since` | datetime | Window start |
| `until` | datetime | Window end |
| `workflowId` | string | Filter to one workflow |

### Example Request

```bash
curl "http://localhost:8080/api/v1/dlq/stats?since=2024-01-01" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "total": 42,
  "byWorkflow": [
    { "workflowId": "wf-123", "workflowName": "Daily Report", "count": 14 },
    { "workflowId": "wf-456", "workflowName": "Hourly Sync",  "count": 28 }
  ],
  "byNodeType": [
    { "nodeType": "n8n-nodes-base.sendGrid", "count": 30 },
    { "nodeType": "n8n-nodes-base.httpRequest", "count": 12 }
  ],
  "oldest": "2023-12-01T08:00:00Z",
  "newest": "2024-01-26T16:00:00Z"
}
```

---

## How items land in the DLQ

- A node's retry policy exhausts (e.g. `maxTries: 5` → 5 failures).
- The execution is permanently cancelled and recorded in the DLQ along with a snapshot of the input that triggered it.
- Existing successful or in-flight executions are unaffected.

The DLQ is **append-only until acted on**. It does not auto-retry, auto-discard, or page anyone out of the box. Wire up an [`errorTrigger` workflow](../nodes/triggers.md#errortrigger-node) on the DLQ if you want notifications.

### Related

- [Retry-node endpoint](executions.md#per-node-retry) — retry a single node inside an existing execution
- [Error Trigger node](../nodes/triggers.md#errortrigger-node) — capture failures from another workflow
