---
title: "Executions API"
description: "API endpoints for managing workflow executions."
keywords: "m9m API, REST API, workflow API, n8n API compatibility"
---

# Executions API

API endpoints for managing workflow executions.

## List Executions

Retrieve execution history with optional filtering.

```http
GET /api/v1/executions
```

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `offset` | integer | 0 | Pagination offset |
| `limit` | integer | 20 | Items per page (max 100) |
| `workflowId` | string | - | Filter by workflow |
| `status` | string | - | Filter by status |
| `mode` | string | - | Filter by execution mode |
| `since` | datetime | - | Executions since timestamp |
| `until` | datetime | - | Executions until timestamp |

### Status Values

| Status | Description |
|--------|-------------|
| `pending` | Queued, waiting to run |
| `running` | Currently executing |
| `completed` | Completed successfully |
| `failed` | Completed with errors |
| `cancelled` | Manually cancelled |

### Mode Values

| Mode | Description |
|------|-------------|
| `manual` | Started via API/CLI |
| `scheduled` | Triggered by schedule |
| `webhook` | Triggered by webhook |

### Example Request

```bash
curl "http://localhost:8080/api/v1/executions?workflowId=wf-123&status=failed&limit=10" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "data": [
    {
      "id": "exec-123456",
      "workflowId": "550e8400-e29b-41d4-a716-446655440000",
      "workflowName": "Daily Report",
      "status": "completed",
      "mode": "manual",
      "startedAt": "2024-01-26T10:00:00Z",
      "finishedAt": "2024-01-26T10:00:01Z",
      "duration": 1234
    }
  ],
  "executions": [
    {
      "id": "exec-123456",
      "status": "completed"
    }
  ],
  "total": 150,
  "offset": 0,
  "limit": 20
}
```

---

## Get Execution

Retrieve detailed execution information.

```http
GET /api/v1/executions/{id}
```

### Example Request

```bash
curl http://localhost:8080/api/v1/executions/exec-123456 \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "id": "exec-123456",
  "workflowId": "550e8400-e29b-41d4-a716-446655440000",
  "workflowName": "Daily Report",
  "status": "completed",
  "mode": "manual",
  "startedAt": "2024-01-26T10:00:00Z",
  "finishedAt": "2024-01-26T10:00:01Z",
  "duration": 1234,
  "inputData": [
    {
      "json": {
        "key": "value"
      }
    }
  ],
  "data": [
    {
      "json": {
        "result": "processed"
      }
    }
  ],
  "nodeExecutions": [
    {
      "nodeId": "start-1",
      "nodeName": "Start",
      "status": "completed",
      "startedAt": "2024-01-26T10:00:00.000Z",
      "finishedAt": "2024-01-26T10:00:00.001Z",
      "duration": 1,
      "outputData": [{"json": {}}]
    },
    {
      "nodeId": "http-1",
      "nodeName": "Fetch Data",
      "status": "completed",
      "startedAt": "2024-01-26T10:00:00.001Z",
      "finishedAt": "2024-01-26T10:00:00.824Z",
      "duration": 823,
      "outputData": [{"json": {"response": "..."}}]
    }
  ]
}
```

### Failed Execution Response

```json
{
  "id": "exec-789",
  "status": "failed",
  "error": {
    "message": "HTTP request failed: connection timeout",
    "node": "Fetch Data",
    "nodeId": "http-1"
  },
  "nodeExecutions": [
    {
      "nodeId": "start-1",
      "status": "success"
    },
    {
      "nodeId": "http-1",
      "status": "failed",
      "error": "connection timeout"
    }
  ]
}
```

---

## Delete Execution

Delete an execution record.

```http
DELETE /api/v1/executions/{id}
```

### Example Request

```bash
curl -X DELETE http://localhost:8080/api/v1/executions/exec-123456 \
  -H "Authorization: Bearer <token>"
```

### Response

```
204 No Content
```

---

## Retry Execution

Retry a failed execution with the same input data.

```http
POST /api/v1/executions/{id}/retry
```

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/executions/exec-789/retry \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "id": "exec-790",
  "workflowId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "mode": "retry",
  "startedAt": "2024-01-26T16:00:00Z",
  "finishedAt": "2024-01-26T16:00:01Z"
}
```

---

## Cancel Execution

Cancel a running execution.

```http
POST /api/v1/executions/{id}/cancel
```

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/executions/exec-running/cancel \
  -H "Authorization: Bearer <token>"
```

### Response (cancellation requested)

```json
{
  "message": "Cancellation requested",
  "executionId": "exec-running",
  "status": "cancel_requested"
}
```

Status code: `202 Accepted`

### Response (runtime cannot cancel this execution)

```json
{
  "error": true,
  "message": "Execution is running but cancellation is not supported by this runtime",
  "executionId": "exec-running",
  "status": "running"
}
```

Status code: `409 Conflict`

---

## Bulk Delete

Delete multiple executions.

```http
POST /api/v1/executions/delete
```

### Request Body

```json
{
  "ids": ["exec-1", "exec-2", "exec-3"]
}
```

Or filter-based deletion:

```json
{
  "workflowId": "wf-123",
  "status": "success",
  "olderThan": "2024-01-01T00:00:00Z"
}
```

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/executions/delete \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "workflowId": "wf-123",
    "status": "success",
    "olderThan": "2024-01-01T00:00:00Z"
  }'
```

### Response

```json
{
  "deleted": 150,
  "message": "150 executions deleted"
}
```

---

## Execution Statistics

Get execution statistics.

```http
GET /api/v1/executions/stats
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `workflowId` | string | Filter by workflow |
| `since` | datetime | Stats since timestamp |

### Example Request

```bash
curl "http://localhost:8080/api/v1/executions/stats?since=2024-01-01" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "total": 1500,
  "success": 1400,
  "failed": 80,
  "cancelled": 20,
  "averageDuration": 1234,
  "byWorkflow": [
    {
      "workflowId": "wf-123",
      "workflowName": "Daily Report",
      "total": 365,
      "success": 360,
      "failed": 5
    }
  ],
  "byDay": [
    {
      "date": "2024-01-26",
      "total": 50,
      "success": 48,
      "failed": 2
    }
  ]
}
```

---

## Error Responses

### 404 Not Found

```json
{
  "error": "Execution not found",
  "code": "NOT_FOUND"
}
```

### 409 Conflict

```json
{
  "error": true,
  "message": "Execution is running but cancellation is not supported by this runtime",
  "executionId": "exec-running",
  "status": "running"
}
```

### 400 Bad Request

```json
{
  "error": true,
  "message": "Execution is not running",
  "code": 400
}
```

---

## Per-Node Retry

Re-run a single node from a previous execution, then continue the rest of the workflow from that point. The original execution is left intact; a new execution is created with `mode: "retry-node"`.

```http
POST /api/v1/executions/{id}/retry-node
```

### Request Body

| Field | Type | Required | Description |
|---|---|---|---|
| `nodeId` | string | Yes | The node id (from the original execution's `nodeExecutions`) to re-run |
| `data` | array | No | Override input data for the retried node. Defaults to the captured input from the original execution. |

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/executions/exec-789/retry-node \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "nodeId": "http-1",
    "data": [{"json": {"userId": 42}}]
  }'
```

### Response

```json
{
  "id": "exec-791",
  "workflowId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "running",
  "mode": "retry-node",
  "startedAt": "2024-01-26T16:00:00Z",
  "retriedFrom": {
    "executionId": "exec-789",
    "nodeId": "http-1"
  }
}
```

### Use case

Workflow failed at the `SendGrid` node. Fix the upstream data, hit `/retry-node` on the SendGrid node, and the rest of the workflow runs with the new output.

---

## Recent Executions

Lightweight "what just ran" feed for the dashboard. Returns the last 20 executions ordered by `startedAt` descending. Lighter than `GET /executions` because it skips `nodeExecutions`.

```http
GET /api/v1/executions/recent
```

### Query Parameters

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | `20` | Max items (max 100) |
| `workflowId` | string | — | Filter by workflow |

### Example Request

```bash
curl "http://localhost:8080/api/v1/executions/recent?limit=5" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "data": [
    {
      "id": "exec-790",
      "workflowId": "wf-123",
      "workflowName": "Daily Report",
      "status": "completed",
      "mode": "manual",
      "startedAt": "2024-01-26T16:00:00Z",
      "duration": 1234
    }
  ]
}
```

---

## Count Executions

Total count, optionally filtered. Cheaper than `GET /executions` because no documents are read.

```http
GET /api/v1/executions/count
```

### Query Parameters

Same as [`GET /executions`](#list-executions).

### Example Request

```bash
curl "http://localhost:8080/api/v1/executions/count?status=failed" \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{ "count": 42 }
```

---

## Debug Mode

Each workflow has a `debug` boolean (see `GET /workflows/{id}`). When `debug: true`:

- Every node's full input and output data is captured and surfaced in `nodeExecutions[*].inputData` / `outputData`.
- The NDV Input/Output/Settings tabs show the captured payload.

When `debug: false` (default):

- Only the **most recent executed node** retains its full payload.
- Earlier nodes retain only their summary (status, duration, error message). NDV falls back to the most recent node's output.
- Sticky-note nodes (`type: n8n-nodes-base.stickyNote`) are skipped entirely from `nodeExecutions` since they don't actually run.

Toggle via `PATCH /api/v1/workflows/{id}` with `"debug": true`.

See [workflows/debug-and-retry](../workflows/debug-and-retry.md) for the user-facing guide.

---

## Edges Taken

Each execution record carries an `edgesTaken` array — the list of connection ids the engine actually traversed. The UI uses this to colour edges per state (taken / skipped / errored). Sub-workflow invocations preserve the parent's `edgesTaken` accumulator.

See [workflows/execution-edges](../workflows/execution-edges.md).

```json
{
  "id": "exec-790",
  "edgesTaken": [
    "Start → Fetch Data",
    "Fetch Data → SendGrid"
  ]
}
```
