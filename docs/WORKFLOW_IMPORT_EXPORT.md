# Workflow import / export

m9m can export workflows as n8n-compatible JSON and import n8n workflow
documents back into the editor. The wire format follows n8n's own
`export:workflow` / `import:workflow` commands, so a file exported from
n8n opens in m9m and a file exported from m9m opens in n8n.

- API: `internal/api/workflow_import_export.go`, `internal/api/routes.go`
- Core: `internal/compatibility/workflow_importer.go` (`N8nWorkflowImporter`)
- Model: `internal/model/workflow.go`
- Tests: `internal/api/workflows_import_export_test.go`,
  `internal/compatibility/workflow_importer_test.go`
- UI: `web/src/views/WorkflowList.vue`, `web/src/views/WorkflowEditor.vue`,
  `web/src/components/workflow/WorkflowEditorToolbar.vue`
- API client: `web/src/api/workflows.ts`
- Types: `web/src/types/api.ts`, `web/src/types/workflow.ts`

---

## Endpoints

| Method | Path | Result |
| --- | --- | --- |
| `GET` | `/api/v1/workflows/{id}/export` | **One** workflow object (n8n editor "Download" parity) |
| `GET` | `/api/v1/workflows/export?ids=id1,id2` | **Array** of workflow objects |
| `POST` | `/api/v1/workflows/export` `{"ids":["id1","id2"]}` | **Array** of workflow objects |
| `POST` | `/api/v1/workflows/import` | Import summary (JSON, multipart or envelope) |

`/workflows/export` and `/workflows/import` are registered **before**
`/workflows/{id}` in `internal/api/routes.go` — gorilla/mux matches
routes in registration order, so the static paths would otherwise be
swallowed by the `{id}` route.

Every export answers with:

```
Content-Type: application/json
Content-Disposition: attachment; filename="<slug>.json"
```

The file name of a **single** export is the slug of the workflow name
(`Daily Report` → `daily-report.json`). A name without usable
characters falls back to the workflow id and finally to `workflow.json`.
The **bulk** export always uses `workflows-export.json`, also for a
single id, because its body is always an array.

---

## Export

### Single workflow

```bash
curl -OJ http://localhost:8080/api/v1/workflows/wf-1/export
```

The body is one workflow object:

```json
{
  "id": "wf-1",
  "name": "Daily report",
  "active": false,
  "nodes": [
    {
      "id": "a1b2",
      "name": "Cron",
      "type": "n8n-nodes-base.scheduleTrigger",
      "typeVersion": 1,
      "position": [0, 0],
      "parameters": { "rule": { "interval": [ { "field": "hours", "hoursInterval": 24 } ] } },
      "credentials": { "httpBasicAuth": { "id": "cred-1", "name": "My Cred" } },
      "disabled": false,
      "onError": "stopWorkflow",
      "maxTries": 3,
      "waitBetweenTries": 1000
    }
  ],
  "connections": {
    "Cron": { "main": [ [ { "node": "HTTP Request", "type": "main", "index": 0 } ] ] }
  },
  "settings": { "executionOrder": "v1" },
  "staticData": {},
  "pinData": {},
  "meta": {},
  "tags": [ { "id": "tag-1", "name": "ops" } ],
  "versionId": "0d0f8a2e-..."
}
```

Node credentials are always written in the **modern** n8n shape
`{type: {id, name}}`. A credential that m9m could not resolve is written
with `"id": null` and keeps its name. Every node carries a
`credentials` object, an empty one when the node has no credential.
Tags are written as `{id, name}` objects; the id is resolved from m9m's
tag table when a matching tag exists, otherwise it is an empty string.
A workflow that never went through an import has no version id of its
own, but the export always writes a `versionId` - n8n writes one too.

Optional node fields (`notes`, `color`, `onError`, `retryOnFail`,
`maxTries`, `waitBetweenTries`, `continueOnFail`, `alwaysOutputData`,
`executeOnce`, `webhookId`, `root`) are omitted when empty, and
`typeVersion` is written as an integer — m9m stores the integer part of
the n8n version (`2.1` becomes `2`).

### Bulk export

```bash
curl "http://localhost:8080/api/v1/workflows/export?ids=wf-1,wf-2" -o workflows.json
curl -X POST http://localhost:8080/api/v1/workflows/export \
     -H 'Content-Type: application/json' \
     -d '{"ids":["wf-1","wf-2"]}' -o workflows.json
```

The body is **always an array**, also for a single id — that mirrors
n8n's CLI, which writes `JSON.stringify(workflowsToExport)`. Without an
id list every workflow is exported (capped at 1000). An unknown id
answers `404`.

---

## Import

### Accepted payloads

`POST /api/v1/workflows/import` accepts all of:

1. **multipart/form-data** with a `.json` file (field `file`, or any
   uploaded file):

   ```bash
   curl -X POST http://localhost:8080/api/v1/workflows/import \
        -F "file=@workflows.json"
   ```

2. **A JSON workflow object**:

   ```bash
   curl -X POST http://localhost:8080/api/v1/workflows/import \
        -H 'Content-Type: application/json' \
        --data-binary @workflow.json
   ```

3. **A JSON array of workflow objects** — the shape n8n's CLI writes.

4. **An envelope** that carries the active-state flag:

   ```json
   { "workflows": [ { "nodes": [], "connections": {} } ], "activeState": "fromJson" }
   ```

The payload is capped at 32 MiB. A body that exceeds the cap is
refused with `413` and the message
`the import payload is too large: the limit is 32 MiB` - the upload is
never truncated into invalid JSON.

### Validation

Every entry must carry **both** a `nodes` and a `connections` property.
An entry that misses one is rejected with the n8n wording:

```
File does not seem to contain valid workflows: missing "nodes" property
```

A rejected entry **never aborts the batch**: the remaining entries are
still imported, and the response reports the failure per item.

### Response

```json
{
  "imported": 2,
  "failed": 1,
  "activeState": "false",
  "workflows": [
    {
      "index": 0,
      "id": "workflow_2f3c…",
      "name": "Daily report",
      "sourceId": "wf-1",
      "versionId": "8b7a…",
      "active": false,
      "warnings": [
        "node a1b2: credential \"My Cred\" (httpBasicAuth) could not be resolved in m9m; the name was kept and the id left empty"
      ]
    }
  ],
  "errors": [
    { "index": 2, "name": "Broken", "message": "File does not seem to contain valid workflows: missing \"nodes\" property" }
  ]
}
```

When **every** entry fails, the endpoint answers `400`. The summary
fields are repeated at the top level of the body (`imported`, `failed`,
`activeState`, `workflows`, `errors`) and are also available under the
`summary` key, so a client can show the reasons without digging into a
nested object:

```json
{
  "error": true,
  "message": "No workflow was imported: File does not seem to contain valid workflows: missing \"connections\" property",
  "code": 400,
  "imported": 0,
  "failed": 1,
  "activeState": "false",
  "workflows": [],
  "errors": [
    { "index": 0, "name": "Broken", "message": "File does not seem to contain valid workflows: missing \"connections\" property" }
  ],
  "summary": { "...": "the same summary" }
}
```

### Import rules

| Rule | Behaviour |
| --- | --- |
| Workflow id | The source n8n id is kept as a **string**. A missing id is generated. An id that already exists in m9m is replaced by a fresh one — an unrelated workflow is never overwritten. |
| Version id | A **fresh** `versionId` is assigned on every import (n8n does the same with `uuid()`). |
| Node id | A node without an id gets one. |
| Node without a `type` | **Dropped**, like n8n's editor does. The number of dropped nodes is reported as a warning. |
| Node without a `name` | A **unique** name is generated from the node type display name (`Webhook`, `Webhook1`, …), like n8n's editor. The generated name is reported as a warning. |
| Node `position` | Normalised like n8n's `ensureNodePosition`: a position that is not two finite numbers falls back to `[0, 0]`. |
| Node fields | `id`, `name`, `type`, `typeVersion`, `position`, `parameters`, `credentials`, `disabled`, `notes`, `color`, `onError`, `retryOnFail`, `maxTries`, `waitBetweenTries`, `continueOnFail`, `alwaysOutputData`, `executeOnce`, `webhookId`, `root` are all kept. |
| `typeVersion` | Accepts `2`, `2.1` and `"2.1"`; m9m stores the integer part. |
| Credentials | Legacy `{"httpBasicAuth": "My Cred"}` and modern `{"httpBasicAuth": {"id": "…", "name": "…"}}` are both accepted and normalised to the modern shape. A reference that carries an id is resolved by **id + type**, a reference without one by **name + type** - only a unique match wins (n8n's `replaceInvalidCredentials`). The resolved credential contributes the name m9m stores. An id that cannot be resolved is cleared, the **name is kept**, and a warning is reported - the import still succeeds. AI Gateway managed credentials (`__aiGatewayManaged`) are skipped, like n8n does. |
| Tags | Both `{id, name}` objects and plain strings are accepted. m9m stores tag **names**; a tag id cannot be preserved and is reported as a warning. |
| `active` | Imported workflows are **inactive** by default. The JSON `active` flag is honoured only when the request explicitly asks for it (`?active=fromJson`, `?activeState=fromJson`, or the envelope's `activeState` field). |
| Connections | n8n keys the connection map by node **name**. The map is sanitised against the real node names: a group of an unknown source node is dropped and a connection to an unknown target node is skipped (both are reported as warnings). Only `main` connections are stored; other connection types are reported as a warning. |
| `webhookId` | **Regenerated** for webhook node types (`n8n-nodes-base.webhook`, `n8n-nodes-base.formTrigger`), like n8n's editor does. When `parameters.path` or `parameters.options.path` referenced the old id, it follows the new id. |
| Expressions | Kept verbatim — m9m's expression runtime exposes the same variables as n8n. |

---

## UI

### Workflow list (`web/src/views/WorkflowList.vue`)

- Every card carries a **checkbox** for selection.
- **Export selected** downloads the selected workflows as one array
  file (the button shows the selection count).
- The per-card **⋮ menu** has an **Export** entry that downloads that
  single workflow.
- **Import** opens a file picker restricted to `.json`. After the
  upload the list refreshes and a banner reports how many workflows
  were imported and which entries failed. A rejected batch (the server
  answers `400`) lists the rejected entries in the same banner.

### Workflow editor (`web/src/views/WorkflowEditor.vue`)

- The toolbar has an **Export** button that downloads the open workflow
  as a single n8n-compatible JSON file. It is hidden for a draft
  workflow that has not been saved yet.

---

## n8n parity

| n8n rule | m9m | Notes |
| --- | --- | --- |
| `export:workflow` always writes an **array** | **Match (bulk)** | `GET`/`POST /workflows/export` always answer with an array, also for one id. |
| Editor "Download" writes a **single object** | **Match (single)** | `GET /workflows/{id}/export` answers with one object. |
| `--separate` names files `<workflowId>.json` | **Differs** | m9m names the file `<workflow-slug>.json` (or `workflows-export.json` for a bulk export). The file name is advisory; the payload is identical. |
| Import accepts an object **or** an array | **Match** | Plus an envelope form (`{"workflows": [...], "activeState": …}`) for the active-state flag. |
| An entry must have **both** `nodes` and `connections` | **Match** | Rejected per entry with n8n's wording; the batch continues. |
| A bad file in `--separate` mode is **skipped** with a warning | **Match** | A bad entry is reported in `errors[]`; the other entries are imported. |
| Workflow ids are **strings**; a missing id is generated | **Match** | m9m's `model.Workflow.ID` is a string. |
| Every import gets a **new `versionId`** | **Match** | A fresh UUID is assigned per import. |
| Every node without an id gets one | **Match** | |
| Editor: nodes **without a `type`** are dropped | **Match** | The count is reported as a per-item warning. |
| Editor: nodes **without a `name`** get a unique name | **Match** | Derived from the node type display name (`Webhook`, `Webhook1`, …). |
| Editor: `position` is normalised (`ensureNodePosition`) | **Match** | A malformed position falls back to `[0, 0]`. |
| Editor: connections are **sanitised** against the real node names | **Match** | Unknown source groups and unknown targets are dropped and reported. |
| Editor: the `webhookId` is **regenerated** for webhook nodes | **Match (broadened)** | n8n regenerates it for `n8n-nodes-base.formTrigger` only; m9m regenerates it for every webhook trigger type (`n8n-nodes-base.webhook`, `n8n-nodes-base.formTrigger`). `parameters.path` / `parameters.options.path` follow the new id when they referenced the old one. |
| Editor: node ids are regenerated on paste (`regenerateIds: true`) | **Deliberate difference** | The server-side import keeps the source node ids (like n8n's CLI import) so a workflow keeps its identity across systems. Only a **missing** id is generated. |
| Credentials: old → new shape (`toNewCredentialFormat`) | **Match** | Legacy `{"type": "Name"}` becomes `{"type": {"id": null, "name": "Name"}}`; the modern shape is kept as-is. |
| A credential id is resolved by **id + type** | **Match** | A credential id that belongs to another credential type is not reused. Without an id the lookup runs by name + type and only a unique match wins. |
| AI Gateway managed credentials are skipped | **Match** | `__aiGatewayManaged` entries are never resolved and never reported. |
| Unresolvable credential id (`replaceInvalidCredentials`) | **Deliberate difference** | n8n keeps a stale id when the id+type pair no longer exists, and it skips disabled nodes completely. m9m clears the id, keeps the **name** and reports a per-item warning — the same shape n8n writes for the legacy-string path. m9m normalises disabled nodes the same way, so a dangling reference never survives an import. A credential never fails the import. |
| `active` is forced to `false` unless `activeState: fromJson` | **Match** | Default `false`; `fromJson` is opt-in per request. |
| Activation happens **after** the transaction, in dependency order | **Partially** | m9m activates the imported workflow directly when `fromJson` is requested. m9m has no workflow-to-workflow activation graph yet (`executeWorkflow` ordering is not modelled), so the Kahn topological sort is not implemented. |
| Tags are `{id, name}` **objects** | **Deliberate difference** | m9m stores workflow tags by **name** (`model.Workflow.Tags []string`). The export writes `{id, name}` and resolves the id from the tag table when it exists; the import accepts both shapes. |
| A workflow-history row is written per import | **Differs** | m9m stores the current workflow version only; it has no `WorkflowHistory` table. The fresh `versionId` still marks the import. |
| Content-policy violations skip one workflow, the rest import | **Match (shape)** | m9m has no content-import policy; any entry that fails validation is skipped and reported in `errors[]`. |
| `--published` / `--version` export a specific history version | **Not supported** | m9m has no workflow history to select from; the export always serves the current version. |
| `--projectId` / `--userId` re-ownership | **Not supported** | m9m has a single project/workspace model. |

---

## Relation to the sync bridge and the parity reference

Two other parts of the repository build on this feature.

### `sync-service/sync.py` (the n8n → m9m bridge)

The bridge normalises an n8n payload (`normalize_n8n_workflow`),
adapts it to m9m's wire format (`to_m9m_payload`) and pushes the batch
to **`POST /api/v1/workflows/import`** - the endpoint documented
above. The payload it builds is accepted by the importer:

| Field | The bridge sends | The import endpoint expects |
| --- | --- | --- |
| `id` | string | string (a number is coerced) |
| `tags` | `["production"]` (names) | names **and** `{id, name}` objects |
| node `credentials` | `{"httpBasicAuth": {"id": "", "name": "My Cred", "type": "httpBasicAuth"}}` | the modern shape, the extra `type` is ignored |
| `connections` | `{"Webhook": {"main": [[{"node": "Set", "type": "main", "index": 0}]]}}` | the n8n source-node map |
| `nodes` + `connections` | always both present | both are required per entry |

The bridge has no credential store, so it keeps `{id: null, name}` and
lets the **server** resolve the id by name + type - that is the one
place where the credential id is repaired. When m9m runs without this
endpoint, the bridge falls back to `POST /api/v1/workflows`, which
cannot resolve credentials.

The two bridge paths differ in their active-state handling: the
`--import` CLI uses the n8n default (every imported workflow is
inactive), while the background poller keeps `activeState=from-json`
so the webhooks of the synced workflows stay registered.

### `docs/N8N_WORKFLOW_PARITY_REFERENCE.md` (the source of truth)

The parity reference distils the n8n rules (with anchors into the
`n8n/` checkout) into a checklist. This document describes how m9m
implements them:

| Reference | This document |
| --- | --- |
| §2.1 export rules | [Export](#export) |
| §2.2 import rules | [Import](#import) and [Import rules](#import-rules) |
| §2.3 credentials | [Import rules](#import-rules) and the parity table |
| §2.4 activation, tags, policy | [Import rules](#import-rules) |
| §3 editor UI import | [UI](#ui) |
| §5 known deliberate divergences | the [n8n parity](#n8n-parity) table |

The divergences listed in §5 of the reference are intentional and must
not be "fixed": credential ids are resolved server-side, `typeVersion`
is coerced to an integer, unknown node types are imported with a
warning, an empty `nodes` list is accepted by m9m and there is no
content-policy layer.

---

## n8n source references

The behaviour above was read from the n8n checkout in this repository
(`n8n/`, commit `04fb7076ca0f88051dcdb8e38aa107fc589ce6fe`):

| File | What it defines |
| --- | --- |
| `n8n/packages/cli/src/commands/export/workflow.ts` | `export:workflow` always writes an array; `--separate`, `--backup`, `--version`, `--published` |
| `n8n/packages/cli/src/commands/import/workflow.ts` | `import:workflow`: object-or-array input, the `nodes` + `connections` check, `activeState` |
| `n8n/packages/cli/src/services/import.service.ts` | `ImportService.importWorkflows`, `toNewCredentialFormat`, the forced `active=false` upsert, the fresh `versionId` |
| `n8n/packages/cli/src/workflow-helpers.ts` | `validateWorkflowStructure`, `replaceInvalidCredentials` |
| `n8n/packages/frontend/editor-ui/src/app/composables/useCanvasOperations.ts` | `importWorkflowData` (drop typeless nodes, `ensureNodePosition`, `sanitizeConnections`, unique node names, webhook-id regeneration) |
| `n8n/packages/frontend/editor-ui/src/app/utils/workflowUtils.ts` | `ensureNodePosition`, `sanitizeConnections` |
| `n8n/packages/frontend/editor-ui/src/app/constants/nodeTypes.ts` | `UPDATE_WEBHOOK_ID_NODE_TYPES` |

---

## Round-trip

```
n8n  --export-->  workflows.json  --import-->  m9m
m9m  --export-->  workflows.json  --import-->  n8n
```

The round-trip is covered by `TestImportExport_RoundTrip` in
`internal/api/workflows_import_export_test.go` (name, node count, node
types, connections, the n8n credential shape and the version id).
The credential rules are covered by
`internal/compatibility/workflow_importer_test.go`.
