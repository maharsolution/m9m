---
title: "Core Nodes"
description: "Core nodes provide fundamental workflow control functionality."
keywords: "m9m nodes, n8n nodes, workflow nodes, HTTP, database, AI, messaging, integrations"
---

# Core Nodes

Core nodes provide fundamental workflow control functionality.

## Start Node

The Start node is the entry point for workflows that don't use trigger nodes.

### Type

```
n8n-nodes-base.start
```

### Description

The Start node initiates workflow execution. It:

- Passes through any input data provided
- Creates an empty item if no input is given
- Must be the first node in non-triggered workflows

### Parameters

The Start node has no configurable parameters.

### Example

```json
{
  "id": "start-1",
  "name": "Start",
  "type": "n8n-nodes-base.start",
  "position": [250, 300],
  "parameters": {}
}
```

### Input

- **None required** - Creates empty item `[{json: {}}]`
- **Optional** - Passes through provided input data

### Output

```json
[
  {
    "json": {}
  }
]
```

Or with input data:

```json
[
  {
    "json": {
      "inputField": "inputValue"
    }
  }
]
```

### Usage

#### Basic Workflow Entry

```json
{
  "nodes": [
    {
      "id": "start",
      "name": "Start",
      "type": "n8n-nodes-base.start",
      "position": [250, 300],
      "parameters": {}
    },
    {
      "id": "next",
      "name": "Next Step",
      "type": "n8n-nodes-base.set",
      "position": [450, 300],
      "parameters": {
        "assignments": [
          {"name": "message", "value": "Workflow started!"}
        ]
      }
    }
  ],
  "connections": {
    "Start": {
      "main": [[{"node": "Next Step", "type": "main", "index": 0}]]
    }
  }
}
```

#### With Input Data (CLI)

```bash
m9m run workflow.json --input '{"name": "John"}'
```

The Start node passes through `{"name": "John"}` to connected nodes.

#### With Input Data (API)

```bash
curl -X POST http://localhost:8080/api/v1/workflows/{id}/execute \
  -H "Content-Type: application/json" \
  -d '{"inputData": [{"json": {"name": "John"}}]}'
```

### When to Use

| Scenario | Use Start Node? |
|----------|-----------------|
| Manual execution | Yes |
| Webhook trigger | No - use Webhook node |
| Scheduled execution | No - use Cron node |
| Subworkflow | Yes |

### Best Practices

1. **One Start node per workflow** - Only one entry point needed
2. **Position first** - Place at the left of your workflow
3. **Clear naming** - Keep the default "Start" name for clarity

### Related Nodes

- [Webhook](triggers.md#webhook-node) - HTTP trigger
- [Cron](triggers.md#cron-node) - Scheduled trigger

---

## NoOp Node

The NoOp node passes its input through unchanged. It's useful as a placeholder, anchor point for connections, or visual divider while authoring a workflow.

### Type

```
n8n-nodes-base.noOp
```

### Description

- Receives items from connected upstream nodes and emits them untouched.
- Has no parameters, no credentials, and no side effects.
- Common use: stub a node you intend to wire up later; mark a checkpoint; debug-flow anchor.

### Parameters

None.

### Example

```json
{
  "id": "noop-1",
  "name": "Placeholder",
  "type": "n8n-nodes-base.noOp",
  "position": [450, 300],
  "parameters": {}
}
```

### When to Use

| Scenario | Use NoOp? |
|---|---|
| Temporarily disable a branch | Yes |
| Anchor a sticky note visually | Yes |
| Stub a node you're about to wire up | Yes |
| Modify data | No — use [Set](transform.md#set-node) or [Code](code.md) |

---

## Wait Node

Pauses workflow execution for a fixed duration or until a specified date/time.

### Type

```
n8n-nodes-base.wait
```

### Parameters

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `resume` | string | Yes | `time` | `time` (wait a duration) or `specificTime` (wait until ISO 8601) |
| `amount` | integer | When `resume=time` | `1` | Amount to wait |
| `unit` | string | When `resume=time` | `seconds` | `seconds`, `minutes`, `hours`, `days` |
| `dateTime` | string | When `resume=specificTime` | — | ISO 8601 timestamp |

### Example

Wait 30 seconds:

```json
{
  "type": "n8n-nodes-base.wait",
  "parameters": {
    "resume": "time",
    "amount": 30,
    "unit": "seconds"
  }
}
```

Wait until a specific datetime:

```json
{
  "type": "n8n-nodes-base.wait",
  "parameters": {
    "resume": "specificTime",
    "dateTime": "2026-12-31T23:59:59Z"
  }
}
```

---

## ExecuteWorkflow Node

Calls another workflow by its id and feeds its output back into the current workflow.

### Type

```
n8n-nodes-base.executeWorkflow
```

### Description

- Resolves the target workflow by **stable id**, not by display name, matching n8n's reference behaviour.
- Accepts an optional `workflowInputs` mapping that injects JSON into the sub-workflow's `$json`.
- Preserves the parent's `EdgesTaken` accumulator across the recursive invocation (see [execution-edges](../workflows/execution-edges.md)).

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `workflowId` | string | Yes | Id of the workflow to invoke |
| `workflowInputs` | object | No | Map of field name → value injected into the sub-workflow's `$json` |

### Example

```json
{
  "type": "n8n-nodes-base.executeWorkflow",
  "parameters": {
    "workflowId": "wf-abc-123",
    "workflowInputs": {
      "userId": "={{ $json.userId }}",
      "feature": "report"
    }
  }
}
```

### Sub-workflow requirements

The target workflow must either:

- Start with an `executeWorkflowTrigger` (recommended), or
- Have a `Start` node that reads `$json` for the injected inputs.

---

## IF Node

Branches execution into two paths (`true` / `false`) based on a boolean comparison of two values.

### Type

```
n8n-nodes-base.if
```

> Full reference lives in [transform.md](transform.md#if-node) alongside the rest of the conditional nodes.

---

## Loop Node

Iterates over an input array, emitting one item per iteration.

### Type

```
n8n-nodes-base.loop
```

> Full reference lives in [transform.md](transform.md#loop-node) alongside the rest of the iteration nodes.

---

## ErrorTrigger Node

Triggers a workflow when a different workflow fails. Pair with a workflow's error workflow setting to receive failed-execution payloads.

### Type

```
n8n-nodes-base.errorTrigger
```

> Full reference lives in [triggers.md](triggers.md#errortrigger-node).
