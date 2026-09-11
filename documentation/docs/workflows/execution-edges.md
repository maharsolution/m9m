---
title: "Execution edges and EdgesTaken"
description: "How m9m records which branches were taken during a workflow execution, and how the UI colours edges per state."
keywords: "m9m, execution, edges, edgesTaken, branch, debugging, visualization"
---

# Execution edges and `EdgesTaken`

Every workflow execution carries an `edgesTaken` array — the list of connection ids the engine actually traversed. The execution-graph UI uses this to colour edges per state (taken / skipped / errored) and to render the path a particular execution took through a workflow.

This page explains what gets recorded, why it matters, and how it interacts with sub-workflows and parallel branches.

---

## What is an "edge"?

A workflow is a graph. **Nodes** are the work units; **edges** are the directed connections between them. In n8n's JSON shape, edges live under `workflow.connections`:

```json
{
  "connections": {
    "If amount > 100": {
      "main": [
        [{ "node": "Send Email",     "type": "main", "index": 0 }],
        [{ "node": "Log Reject",     "type": "main", "index": 0 }]
      ]
    }
  }
}
```

`main[0]` is the "true" branch, `main[1]` is the "false" branch. The actual id m9m uses internally is `"If amount > 100 → Send Email"` (source display name, arrow, target display name), but the engine doesn't care about the format — only that the edge is uniquely identifiable.

## What is `EdgesTaken`?

For every execution, m9m records which edges were traversed, in order:

```json
{
  "id": "exec-790",
  "edgesTaken": [
    "Start → Fetch Data",
    "Fetch Data → If amount > 100",
    "If amount > 100 → Send Email"
  ]
}
```

This is the same shape surfaced via:

- `GET /api/v1/executions/{id}` → `edgesTaken`
- The execution graph in the web UI (drives edge colour)
- The OTel `workflow.execute` span (under attribute `m9m.execution.edges_taken`)

## Edge colours

The UI colours each edge according to its state at the moment the execution finished:

| Edge state | Colour | Meaning |
|---|---|---|
| Taken | Solid green | The edge was traversed by the engine. The downstream node ran (or was skipped due to empty input — see below). |
| Skipped | Solid grey | The edge exists but was not traversed. Typical of the `false` branch on an `IF` that evaluated true, or a downstream node whose input was empty. |
| Errored | Solid red | The upstream node failed; the engine did not propagate items to this edge. |
| Not-yet-run | Dashed | The execution was cancelled or is still running and has not yet reached this edge. |

This is the same classification that drives the [NDV Input/Output/Settings tabs](debug-and-retry.md#ndv-tabs): a "skipped" edge implies the downstream node was either skipped or had no input.

## Skipped nodes and empty input

When a node receives **zero input items**, m9m skips it — matching n8n's behaviour. The edge into that node is recorded as `taken` (the engine did call into it), but the downstream edge is recorded as `skipped` (no items propagated). The node's `status` in `nodeExecutions` is `skipped`.

This is the same fix referenced in commit `fcb3e56` ("skip nodes with empty input") from the September 2026 cycle — earlier m9m builds would throw on empty input, leaving the rest of the execution in an indeterminate state.

## Sub-workflow edge preservation

When a parent workflow invokes a sub-workflow via `executeWorkflow`, m9m accumulates the parent's `edgesTaken` across the recursive invocation. The sub-workflow's own internal edges are recorded separately, so a single execution graph on the parent shows the parent's path (with a marker node for the sub-workflow call), and drilling into the sub-workflow shows the child's path.

This is the fix referenced in `[0.2.0-parity]` (`a2fc20d fix(engine): preserve parent EdgesTaken across sub-workflow recursion`). Earlier builds would lose the parent's accumulator on return from the sub-workflow.

```json
{
  "id": "exec-790",
  "edgesTaken": [
    "Start → Fetch User",
    "Fetch User → Run Reports (sub-workflow)",
    "Run Reports (sub-workflow) → Email User"
  ],
  "subExecutions": [
    {
      "id":        "exec-791",
      "workflowId": "wf-sub-reports",
      "edgesTaken": [
        "Called From Parent → Set Output",
        "Set Output → Email Output"
      ]
    }
  ]
}
```

For the parent's `edgesTaken`, the sub-workflow call appears as a single edge `Run Reports (sub-workflow) → next parent node`. To see the internal edges, drill into the `subExecutions[*]` entry.

## Parallel branches

For a `Switch` or `IF` node that fans out, every emitted branch records its own edge. For a `Merge` node that joins branches, the merge edge records `mergedFrom` (the branches that fed it).

```json
{
  "edgesTaken": [
    "Start → Fork",
    "Fork → Branch A",
    "Fork → Branch B",
    "Branch A → Merge",
    "Branch B → Merge",
    "Merge → Send"
  ]
}
```

In the UI, the `Merge` node's incoming edges are coloured green (taken); the merge node itself shows the merged output payload.

## Edge state when an upstream node errors

If `Fetch Data` errors, every edge out of `Fetch Data` is `errored`, every edge into nodes downstream of `Fetch Data` is `skipped` (no items propagated), and the edge between downstream siblings (if any) is `errored` (the engine didn't get to them). The UI colours the whole post-error subgraph red-and-grey.

The execution's overall `status` is `failed`. The execution record still has `edgesTaken` — the partial path is preserved so you can see how far the execution got.

## Curl — get an execution with its edges

```bash
curl http://localhost:8080/api/v1/executions/exec-790
```

```json
{
  "id":        "exec-790",
  "status":    "completed",
  "edgesTaken": [
    "Start → Fetch Data",
    "Fetch Data → SendGrid"
  ],
  "nodeExecutions": [ /* per-node payload */ ]
}
```

## See also

- [Per-node retry](debug-and-retry.md)
- [Executions API](../api/executions.md#edges-taken)
- [Observability — span attributes](../observability/index.md#span-tree)
