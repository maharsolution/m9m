---
title: "Debug mode and per-node retry"
description: "Capture full per-node payload data when you need it, and re-run a single node without restarting the workflow."
keywords: "m9m, debug, retry, NDV, execution, per-node"
---

# Debug mode and per-node retry

Two complementary features for diagnosing a failing workflow:

1. **Per-workflow debug mode** — capture full input/output for every node on every run.
2. **Per-node retry** — re-run a single node from an existing execution and continue from there.

Together they let you iterate on a single bad step without paying for an entire re-execution or losing context about what the upstream nodes produced.

---

## Debug mode

Each workflow carries a `debug` boolean (visible on `GET /api/v1/workflows/{id}` and toggleable from the toolbar in the web UI).

| `debug` | What gets captured | When to use |
|---|---|---|
| `false` (default) | Only the **most recent executed node** retains its full input/output payload. Earlier nodes keep their summary (status, duration, error) but their payload is freed. | Routine execution — keeps the executions table cheap. |
| `true` | **Every** node's full input and output payload is captured and surfaced in `GET /executions/{id}.nodeExecutions[*].inputData` / `.outputData`. Multiplies storage and CPU cost roughly by the number of nodes per execution. | Diagnosing a workflow that intermittently fails or that you are actively developing. |

> Debug mode is **per-workflow**, not global. Turn it on for the one workflow you are debugging; leave the others on default.

### Toggle in the UI

Open the workflow, click the toolbar's **Debug** toggle, save. The next execution captures every node's payload.

### Toggle via the API

```bash
curl -X PATCH http://localhost:8080/api/v1/workflows/{id} \
  -H "Content-Type: application/json" \
  -d '{"debug": true}'
```

### NDV tabs

The Node Detail View (the panel that opens when you click a node on an execution graph) has three tabs:

| Tab | What it shows |
|---|---|
| **Input** | The captured input items for this node from the execution you clicked. |
| **Output** | The captured output items. |
| **Settings** | Per-node controls: `Retry from here` button (see below), node-level retry policy (max tries, wait between tries), and the option to skip this node on the next run. |

If you opened the NDV on the most recent executed node in a `debug: false` execution, NDV falls back to that node's payload (which is always retained). For earlier nodes in a `debug: false` execution, the Input/Output tabs say "No data captured — turn on debug mode for this workflow to inspect intermediate nodes."

### Sticky notes

Nodes with `type: n8n-nodes-base.stickyNote` (visual annotations) are skipped entirely from `nodeExecutions` since they don't run. Their position is still rendered on the execution graph but no execution record is created.

---

## Per-node retry

When a workflow fails mid-way, you usually know which node blew up. Per-node retry lets you:

1. Inspect the captured input and error for the failed node.
2. Fix the upstream cause (rotate a credential, repair a payload, whatever).
3. Hit **Retry from here** — only that node re-runs, and the rest of the workflow continues from its output.

### UI flow

1. Open an execution.
2. Click the failed node → NDV opens with Input / Output / Settings tabs.
3. Switch to **Settings**.
4. Optionally edit the input data (the JSON the node will receive on the retry).
5. Click **Retry from here**.
6. A new execution appears in the executions list with `mode: "retry-node"` and a `retriedFrom` pointer back to the original.

### API flow

```bash
curl -X POST http://localhost:8080/api/v1/executions/exec-789/retry-node \
  -H "Content-Type: application/json" \
  -d '{
    "nodeId": "sendgrid-1",
    "data":   [{"json": {"to": "new@recipient.com"}}]
  }'
```

The `nodeId` must match a node from the original execution's `nodeExecutions`. The `data` field is optional — omit it to retry with the originally captured input. Provide a different `data` field to override what the retried node sees.

### What is and isn't retried

| Behaviour | Detail |
|---|---|
| New execution record | Yes — the original is left intact for the audit log. |
| Mode | `"retry-node"` on the new execution. |
| `retriedFrom` | `{ executionId, nodeId }` linking back to the original. |
| Subsequent nodes | Re-run from the retried node's output onward, including any branches. |
| Concurrent retries | One at a time per source execution. The second call returns 409. |
| Debug mode | The retry inherits the source workflow's `debug` flag. |
| Sub-workflow invocations | Preserved — the retried subtree runs the same way it did originally. |
| OTel trace | The retry is a new trace; the source's `trace_id` is recorded in the new root span's attributes for correlation. |

### When NOT to use retry-node

- **The workflow shape changed.** If you've edited the workflow since the original execution, the connections may no longer match. Re-run the whole workflow instead.
- **The original workflow is gone.** Retry-node resolves the original execution's `nodeExecutions`; if the workflow has been deleted, the lookup fails.
- **The data is irrelevant.** If the failed node produced no useful data and the next node doesn't need it, `POST /executions/{id}/retry` (whole-workflow retry) is simpler.

---

## See also

- [Executions API — per-node retry](../api/executions.md#per-node-retry)
- [Executions API — debug flag](../api/executions.md#debug-mode)
- [Workflows API — debug flag](../api/workflows.md#debug-flag)
- [DLQ API](../api/dlq.md) — when retries are exhausted
