# n8n Workflow Import/Export Parity Reference

Source of truth for the m9m workflow import/export feature and for the
`n8n` → `m9m` sync bridge.

- n8n checkout: `n8n/` (upstream commit `04fb7076ca0f88051dcdb8e38aa107fc589ce6fe`)
- n8n knowledge graph: `n8n/graphify-out/graph.json` (built at the same commit)
- m9m side: `internal/compatibility/workflow_importer.go`, `internal/api/workflows.go`,
  `internal/api/workflow_import_export.go`, `sync-service/sync.py`

Every rule below was read from the n8n sources; the file:line anchors are given
so the rule can be re-verified.

---

## 1. Export — `n8n/packages/cli/src/commands/export/workflow.ts`

| Rule | Detail | Anchor |
| --- | --- | --- |
| Output is always an ARRAY | `JSON.stringify(workflowsToExport)` — even for a single `--id` | L161-166 |
| `--separate` | One file per workflow, named `<workflowId>.json` | L150-159 |
| `--backup` | Shorthand for `--all --pretty --separate` | L63-67 |
| Selectors | Exactly one of `--all`, `--id`, `--projectId` | L79-89 |
| Version selection | `--version <id>` or `--published` (uses `activeVersionId`); mutually exclusive | L68-77, L236-243 |
| Exported object | Full workflow entity spread plus, for a selected version, `nodes`, `connections`, `nodeGroups`, `versionId`, `versionMetadata` | L248-280 |
| Relations loaded | `tags`, `shared`, `shared.project` | L124-126 |

n8n editor "Download" produces a single workflow object; the CLI produces an
array. m9m mirrors both: single-workflow export → object, bulk export → array.

## 2. Import — `n8n/packages/cli/src/commands/import/workflow.ts` + `n8n/packages/cli/src/services/import.service.ts`

### 2.1 Input shape and validation

| Rule | Detail | Anchor |
| --- | --- | --- |
| Object or array | `Array.isArray(workflows) ? workflows : [workflows]` | import/workflow.ts L286-290 |
| Required keys | Every entry MUST have BOTH `nodes` and `connections`, else `UserError('File does not seem to contain valid workflows.')` | import/workflow.ts L28-37 |
| `--separate` | Globs `*.json`; an invalid file is skipped with a warning, the batch continues | import/workflow.ts L292-315 |
| Structure validation | `validateWorkflowStructure(workflow)` — zod-backed; failures become `WorkflowStructureBadRequestError` with `path (code): message` details | import.service.ts L169; workflow-helpers.ts L198-217 |

### 2.2 Identity and versioning

| Rule | Detail | Anchor |
| --- | --- | --- |
| Workflow id is a STRING | nanoid/uuid style; `id: created.id || generateNanoId()` | import/workflow.ts L50-56 |
| Node id | `if (!node.id) node.id = uuid()` | import.service.ts L160-163 |
| Fresh versionId | `workflow.versionId = uuid()` on every import — keeps history ordering correct | import.service.ts L234-235 |
| History row | One `WorkflowHistory` row per imported workflow (`nodes`, `connections`, `nodeGroups`, `authors: 'import'`) | import.service.ts L292-304 |

### 2.3 Credentials

| Rule | Detail | Anchor |
| --- | --- | --- |
| Two shapes | OLD `{"httpBasicAuth": "My Cred"}` / NEW `{"httpBasicAuth": {"id": "abc"\|null, "name": "My Cred"}}` | import.service.ts L808-824 |
| Conversion | `toNewCredentialFormat` converts OLD → NEW and resolves `id` by matching credential NAME + TYPE | import.service.ts L810-823 |
| Unresolved | `replaceInvalidCredentials` resolves by name + type; exactly one match wins, otherwise `{id: null, name}` is kept. Disabled nodes and `__aiGatewayManaged` entries are skipped | workflow-helpers.ts L274-335 |
| Non-fatal | A credential that cannot be repaired never fails the batch | import.service.ts L165-168 |

### 2.4 Activation, tags, policy

| Rule | Detail | Anchor |
| --- | --- | --- |
| `active` forced false | Upsert always stores `active = false`, `activeVersionId = null` | import.service.ts L244-252 |
| Activation opt-in | Only with `activeState: 'fromJson'` AND `workflow.active === true`; happens after the transaction | import.service.ts L238-242, L307-318 |
| Activation order | Sub-workflow dependencies are activated first (Kahn topological sort, cycle-safe) | import.service.ts L322-405 |
| Tags are objects | `[{id, name}]`, upserted into the tag mapping table | import.service.ts L278-288 |
| Policy violations | A blocked workflow is SKIPPED; the rest of the batch still imports | import.service.ts L194-219 |

## 3. Editor UI import — `n8n/packages/frontend/editor-ui/src/app/composables/useCanvasOperations.ts`

`importWorkflowData()` (L3230) is what the n8n UI does with a dropped JSON file:

| Rule | Detail | Anchor |
| --- | --- | --- |
| Structure gate | Missing `nodes` or `connections` → error toast, import aborted | L3253-3261 |
| Nodes without `type` | Filtered out, count reported to the user | L3265-3280 |
| Position | `ensureNodePosition(node.position)` normalises a missing/invalid position | L3282-3287 |
| Connections | `sanitizeConnections(connections, validNodeNames)` drops edges pointing at unknown node names | L3289-3292 |
| Node names | Unnamed nodes get a unique name (node display name or type) | L3299-3313 |
| Webhook ids | `webhookId` regenerated for webhook node types; `parameters.path` kept in sync when it matched the old id | L3315-3328 |
| Node ids | Regenerated by default (`regenerateIds = true`); `nodeGroups[].nodeIds` remapped | L3330-3345 |
| Tags | Imported optionally (`importTags = true`) | L3232-3250 |

## 4. m9m mapping

| n8n rule | m9m implementation | File |
| --- | --- | --- |
| Import conversion + issues | `N8nWorkflowImporter.ImportWorkflow` → `ImportResult` / `ConversionIssue` | `internal/compatibility/workflow_importer.go` |
| Import with n8n CLI semantics | `N8nWorkflowImporter.ImportWorkflowWithOptions` (inactive by default, credential resolution, id collision handling, fresh version id) | `internal/compatibility/workflow_importer.go` |
| Export to n8n shape | `N8nWorkflowImporter.ExportWorkflow` / `ExportWorkflows` (always an array) | `internal/compatibility/workflow_importer.go` |
| Structure validation | `N8nWorkflowImporter.ValidateN8nWorkflow` | `internal/compatibility/workflow_importer.go` |
| HTTP import/export | `POST /api/v1/workflows/import`, `GET /api/v1/workflows/{id}/export`, bulk export | `internal/api/workflow_import_export.go`, `internal/api/routes.go` |
| Bridge normalization | `normalize_n8n_workflow`, `validate_n8n_workflow`, `to_m9m_payload`, `--import` CLI | `sync-service/sync.py` |
| Bridge batch push | `push_batch_to_m9m` → `POST /api/v1/workflows/import` (fallback `POST /api/v1/workflows`) | `sync-service/sync.py` |
| Bridge documentation | Normalization rules, verdicts, parity table, divergences | `sync-service/README.md` |

### 4.1 The bridge → m9m wire contract

The bridge normalizes an n8n payload and adapts it with `to_m9m_payload()`.
The result is accepted by both m9m entry points:

| Field | Value the bridge sends | `POST /api/v1/workflows/import` | `POST /api/v1/workflows` (`model.Workflow`) |
| --- | --- | --- | --- |
| `id` | string | `N8nWorkflowData.ID interface{}` → string | `ID string` |
| `tags` | `["production"]` (names) | `convertTags()` accepts strings **and** `{id, name}` objects, stores names | `Tags []string` |
| node `credentials` | `{"httpBasicAuth": {"id": "", "name": "My Cred", "type": "httpBasicAuth"}}` | `convertCredentials()` reads `id`/`name`, ignores the extra `type`, resolves by name + type | `Credential{ID, Name, Type}` |
| `connections` | `{"Webhook": {"main": [[{"node": "Set", "type": "main", "index": 0}]]}}` | `map[string]N8nConnectionGroup` (n8n source-node map) | `map[string]Connections` |
| `nodes[].typeVersion` | `int` | `N8nTypeVersion` (float) accepts an int | `TypeVersion int` |
| `nodes[].position` | `[x, y]` ints | `N8nPosition` (float) accepts ints | `Position []int` |
| `nodes` + `connections` keys | always both present | `ValidateN8nWorkflow()` requires both | not required |

Verified: a payload built by `normalize_n8n_workflow()` + `to_m9m_payload()`
passes `ValidateN8nWorkflow()`, is imported by
`ImportWorkflowWithOptions()` (string id, fresh `versionId`, `active=false`,
tags as names, credential resolved by name + type, connections in the
source-node map shape) and decodes into `model.Workflow` without a JSON error.

## 5. Known deliberate divergences

1. **Credential id resolution happens server-side** — the Python bridge has no
   credential store, so it keeps `{id: null, name}`; the batch import goes to
   m9m's import endpoint (`POST /api/v1/workflows/import`), which resolves the
   id by **name + type** exactly like n8n does. The fallback path
   (`POST /api/v1/workflows`) cannot resolve it.
2. **`typeVersion` coercion** — m9m's Go model types it as `int`, so `2.1` becomes `2`.
   Lossy by design; documented in `sync-service/README.md`.
3. **Unknown node types** — n8n imports them; m9m stores them but cannot execute
   them, so the bridge reports a warning instead of failing the file.
4. **Empty `nodes`** — rejected by the bridge, accepted by n8n's CLI assert.
5. **Content-policy engine** — n8n skips policy-violating workflows; m9m has no
   equivalent policy layer yet.

### 5.1 Bridge-specific divergences (mirrored in `sync-service/README.md`)

6. **`versionId` in the live path** — the background poller keeps n8n's
   `versionId` (so m9m tracks n8n's version), while the import path always
   assigns a new one (n8n parity).
7. **Live sync keeps `active`** — the poller normalizes with
   `active_state=from-json` because m9m only registers webhooks for active
   workflows, so deactivating there would silently break every webhook. The
   import path uses the n8n default (`false`), i.e. reference §2.4.
8. **Import target** — the import CLI pushes the whole batch to m9m's n8n
   import endpoint (`POST /api/v1/workflows/import`); m9m builds without that
   endpoint fall back to `POST /api/v1/workflows` per workflow, which cannot
   resolve credentials server-side.
9. **Workflow id collision** — m9m's import endpoint mints a fresh workflow id
   when the imported id is already taken (an unrelated workflow is never
   overwritten); n8n upserts under the same id.
10. **Webhook id on import** — the bridge does not regenerate webhook ids;
    m9m's import endpoint does (n8n editor parity) and keeps
    `parameters.path` in sync, while the live polling path keeps n8n's id so
    an existing webhook route stays stable.

## 6. Parity checklist

- [ ] Import accepts an object and an array.
- [ ] An entry without `nodes` or `connections` is rejected without aborting the batch.
- [ ] Missing node ids are generated; missing workflow ids are generated.
- [ ] Credential OLD shape is converted to the NEW shape; unresolved ids stay `null`.
- [ ] `tags` are objects `{id, name}`.
- [ ] `active` is false after import unless explicitly requested.
- [ ] Export of one workflow returns an object; export of many returns an array.
- [ ] Export round-trip: import → export keeps name, node count, node types, connections.
- [ ] Connections referencing unknown nodes are dropped, not imported as-is.
- [ ] The bridge payload is accepted by `POST /api/v1/workflows/import` (string id, tags as
      names, credentials in the new shape, connections in the source-node map shape).
- [ ] The bridge pushes a batch as an array; one rejected entry never aborts the rest.
