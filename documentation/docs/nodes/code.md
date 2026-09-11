---
title: "Code Nodes"
description: "Code nodes execute scripts inside sandboxed runtimes."
keywords: "m9m nodes, n8n nodes, workflow nodes, Python, sandbox"
---

# Code Nodes

Code nodes execute scripts inside sandboxed runtimes. Unlike the [Function](../transform.md#function-node) node (which runs JavaScript), the Code category covers languages that need their own runtime.

---

## PythonCode Node

Execute Python inside a bubblewrap-isolated environment. Use this for data wrangling, statistics, ML inference, or any task where JavaScript is a poor fit.

### Type

```
n8n-nodes-base.pythonCode
```

### Description

- Code runs in a subprocess inside bubblewrap (`bwrap`) with no network and a writable `/tmp` only.
- The Python interpreter is bundled with the binary (no host Python required).
- Receives a `data` argument: a list of items (each `{ json: {...}, binary?: {...} }`).
- Must `return` a list of items. Returning an empty list halts the workflow downstream.

### Parameters

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | Yes | — | Python source |
| `pythonVersion` | string | No | `3.11` | `3.10`, `3.11`, `3.12` |
| `timeout` | integer | No | `30` | Max execution time, seconds |
| `memoryLimitMb` | integer | No | `256` | Memory cap |

### Example

```json
{
  "type": "n8n-nodes-base.pythonCode",
  "parameters": {
    "code": "import statistics\namounts = [item['json']['amount'] for item in data]\nreturn [{ 'json': {\n    'count': len(amounts),\n    'mean': statistics.mean(amounts),\n    'stdev': statistics.stdev(amounts) if len(amounts) > 1 else 0\n} }]",
    "pythonVersion": "3.11",
    "timeout": 10
  }
}
```

### Stdlib and packages

- The full Python standard library is available.
- A curated set of common third-party packages is pre-installed (numpy, pandas, requests, etc.).
- For new packages, file an issue; pre-baked wheels reduce cold-start vs pip-install-at-runtime.

### Security model

- Process runs with the m9m binary's UID/GID; bubblewrap restricts filesystem visibility to a temporary scratch directory.
- Network is disabled by default. Set `network: true` if the script needs to call out (rare; prefer an HTTP Request node).
- Resource limits (memory, CPU time, wall-clock) are enforced by cgroups; over-quota scripts are killed and reported as errors.

### Related

- [Function node](../transform.md#function-node) — JavaScript in-process, faster startup
- [CLI / Execute node](cli.md) — run arbitrary external commands (including AI agents) inside bubblewrap
