# m9m n8n Sync Bridge (`sync-service/sync.py`)

Polls n8n's REST API for workflows and pushes them into m9m's REST API, and
doubles as a local-file importer that mirrors the **real n8n CLI import
semantics**.

Two modes, one file:

```bash
# 1. Service mode (default) — FastAPI + uvicorn, background poller.
python sync-service/sync.py

# 2. Import mode — one-shot import of an n8n export file.
python sync-service/sync.py --import workflows.json [--dry-run] [--active-state=from-json]
```

Without `--import` the module boots uvicorn exactly as before (supervisord
invokes `uvicorn sync:app` in production). With `--import` it reads a local
file, normalizes every workflow to n8n shape, validates it against m9m's
contract, and either prints the result (`--dry-run`) or pushes it into m9m.

---

## Import CLI

```
--import FILE            workflow JSON file: one workflow object OR an array
--dry-run                validate + normalize only; prints the normalized JSON
                         and makes ZERO HTTP requests
--active-state VALUE     "false" (default) | "from-json"
                         alias: --activeState=fromJson
```

Output is one verdict line per workflow plus a summary:

```
[import] ok       Old credential format (WfOldFormat)
{ ...normalized n8n-shaped JSON... }
[import] skipped  Missing connections: missing 'connections' property
[import] 1 ok, 1 skipped, 0 failed
```

| Verdict | Meaning |
| --- | --- |
| `ok` | Normalized and valid (pushed to m9m unless `--dry-run`) |
| `skipped` | Entry is invalid — the reason is printed, the batch continues |
| `failed` | m9m rejected the push (HTTP status + body in the reason) |

Exit code: `0` success, `1` at least one workflow failed to push, `2` the file
is missing / not valid JSON / not a workflow payload.

### Push target

The valid entries of a batch are pushed **as one array** to m9m's n8n import
endpoint:

```
POST /api/v1/workflows/import[?activeState=fromJson]
Content-Type: application/json

[ { ...workflow... }, { ...workflow... } ]
```

That endpoint (`internal/api/workflow_import_export.go`) mirrors n8n's batch
import:

- every entry is validated on its own — one invalid entry never aborts the
  batch, the response carries a per-item `workflows[]` / `errors[]` summary;
- a fresh `versionId` is assigned per workflow;
- `active` is forced to `false` unless the request asks for
  `activeState=fromJson` (`--active-state=from-json`);
- credentials are resolved by **name + type** against m9m's credential store;
  an unresolvable one keeps its name and is reported as a warning.

m9m builds that predate that endpoint answer `404`; the bridge then falls back
to `POST /api/v1/workflows` per workflow (no server-side credential
resolution, but the import still succeeds).


---

## Normalization rules (mirrored from n8n)

Canonical reference: **`docs/N8N_WORKFLOW_PARITY_REFERENCE.md`** — distilled from
the real n8n sources in this repo (`n8n/` upstream checkout, commit
`04fb7076`, plus `n8n/graphify-out/graph.json`). This section mirrors it.

Sources in this repo:

- `n8n/packages/cli/src/commands/import/workflow.ts`
- `n8n/packages/cli/src/services/import.service.ts`
- `n8n/packages/cli/src/commands/export/workflow.ts`
- `n8n/packages/cli/src/workflow-helpers.ts`
- `n8n/packages/frontend/editor-ui/src/app/composables/useCanvasOperations.ts`
- `n8n/packages/frontend/editor-ui/src/app/utils/workflowUtils.ts`

| Rule | Behaviour in the bridge |
| --- | --- |
| Input may be a single workflow object **or** an array | Accepted; an array is processed entry by entry. n8n's `export:workflow` always writes an array, so arrays are the common case. |
| Every entry needs **both** `nodes` and `connections` | Missing either → entry is `skipped` with the reason `missing 'nodes' property` / `missing 'connections' property`. The batch continues (n8n's `--separate` behaviour). |
| Workflow `id` is a string | Kept as a string; a non-string id is stringified, a missing id is generated (`uuid4`). |
| Fresh `versionId` on every import | Always overwritten with a new `uuid4` (import path). The live polling path keeps n8n's `versionId` so m9m tracks n8n's version. |
| Nodes without an `id` | Get a generated `uuid4`. |
| Credentials — OLD shape `{"httpBasicAuth": "My Cred"}` | Converted to the NEW shape `{"httpBasicAuth": {"id": null, "name": "My Cred"}}`. |
| Credentials — NEW shape `{"httpBasicAuth": {"id": "abc123", "name": "My Cred"}}` | Preserved, including `id: null`. |
| Credentials on **disabled** nodes | **Skipped** — the payload is passed through untouched, n8n does not repair credentials of disabled nodes (`workflow-helpers.ts` L286-288). |
| `__aiGatewayManaged` credentials | **Skipped** — no record exists behind them (`workflow-helpers.ts` L301-302). They survive normalization but are dropped from the m9m payload. |
| Unresolved credential id | Kept as `{id: null, name}` — never fatal for the batch. |
| `tags` are objects `{id, name}` | Normalized to `[{id, name}]`. Plain strings are accepted and given a generated id. |
| `typeVersion` is numeric | Coerced to `int` (float `2.1` → `2`, string `"3"` → `3`). A non-numeric value falls back to `1` and is reported as a warning. |
| `active` | Forced to `false` by default; `--active-state=from-json` honours the JSON field. |
| Node type support | Checked against m9m's registered node types. Unknown types are a **warning**, not an error — the workflow still imports, matching n8n's lenient import. |

### Editor-side import rules (reference §3)

The n8n **editor** import (`importWorkflowData()` in
`useCanvasOperations.ts` L3230-3345) applies rules the CLI import does not.
The ones that make sense for a headless bridge are mirrored in
`normalize_n8n_workflow()`. Each one is a **warning**, never a hard failure:

| Rule | n8n editor | Bridge |
| --- | --- | --- |
| Nodes without a `type` | Filtered out, count reported to the user | Dropped, count reported as a warning |
| `position` | `ensureNodePosition()` → `[0, 0]` when missing/invalid | Same, normalised to `[x, y]` integers (m9m's model types `position` as `[]int`) |
| Connections pointing at unknown node names | `sanitizeConnections()` drops them | Same — the source key and each edge target are checked against the node names |
| Malformed connection entries | Dropped | Dropped (node/type must be strings, index must be a number) |
| Unnamed nodes | Get a unique name (display name or type) | Same — unique name from `sync.NODE_DISPLAY_NAMES`, falling back to the node type |
| `webhookId` regeneration | Only for `UPDATE_WEBHOOK_ID_NODE_TYPES` (the form trigger) | Not applied by the bridge — but m9m's import endpoint regenerates it server-side and keeps `parameters.path` in sync; the live polling path keeps n8n's id so an existing webhook route stays stable |
| Node id regeneration | The editor regenerates ids on paste (`regenerateIds = true`) | Not applied — the import path keeps existing ids and only fills in missing ones, mirroring the CLI import |

### Validation verdicts

`validate_n8n_workflow(workflow, known_types=None)` returns:

```python
{"valid": bool, "errors": [...], "warnings": [...], "unsupportedTypes": [...]}
```

- **Errors** (entry is skipped): missing/empty `nodes`, missing workflow name,
  node without `name` or `type`, missing `nodes`/`connections` property.
- **Warnings** (entry still imports): node types m9m does not implement, plus
  every normalization note (dropped typeless nodes, sanitized connections,
  skipped credentials, defaulted positions, assigned node names).
- `known_types` is the optional live list from `GET /api/v1/node-types` on
  m9m; it is merged with the static map in `sync.KNOWN_NODE_TYPES`.

### m9m wire adaptation (`to_m9m_payload`)

The normalized workflow stays n8n-shaped. `to_m9m_payload()` produces the
payload that is actually POSTed to m9m, closing two contract gaps:

| Field | n8n | m9m (`internal/model/workflow.go`) | Adaptation |
| --- | --- | --- | --- |
| `tags` | `[{id, name}]` | `[]string` | Tag **names** are sent. |
| node `credentials` | `{type: {id, name}}` | `map[string]Credential` = `{id, name, type}` | The credential type is copied from the map key. |

Everything else (`nodes`, `connections`, `versionId`, `active`, `settings`, …)
is already n8n-compatible and passes through untouched.

The resulting payload is accepted by **both** m9m entry points:

| Field | Value the bridge sends | `POST /api/v1/workflows/import` (Go) | `POST /api/v1/workflows` (Go model) |
| --- | --- | --- | --- |
| `id` | string | `N8nWorkflowData.ID interface{}` → string | `model.Workflow.ID string` |
| `tags` | `["production"]` | `convertTags()` accepts strings **and** `{id, name}` objects, stores names | `model.Workflow.Tags []string` |
| node `credentials` | `{"httpBasicAuth": {"id": "", "name": "My Cred", "type": "httpBasicAuth"}}` | `convertCredentials()` reads `id`/`name` (the extra `type` is ignored) and resolves the id by name + type | `model.Credential{ID, Name, Type}` |
| `connections` | `{"Webhook": {"main": [[{"node": "Set", "type": "main", "index": 0}]]}}` | `map[string]N8nConnectionGroup` (source-node map) | `map[string]model.Connections` |
| `nodes[].typeVersion` | `int` | `N8nTypeVersion` (float) accepts an int | `model.Node.TypeVersion int` |
| `nodes[].position` | `[x, y]` ints | `N8nPosition` (float) accepts ints | `model.Node.Position []int` |
| `nodes` + `connections` keys | both always present | `ValidateN8nWorkflow()` requires both | not required |

Verified end to end: a payload produced by `normalize_n8n_workflow()` +
`to_m9m_payload()` passes `ValidateN8nWorkflow()`, is imported by
`N8nWorkflowImporter.ImportWorkflowWithOptions()` (id as string, fresh
`versionId`, `active=false`, tags as names, credential resolved by name +
type, connections in the source-node map shape) and decodes into
`model.Workflow` without a JSON error.


---

## n8n parity

| Rule | m9m sync-service behaviour | n8n behaviour | Match? |
| --- | --- | --- | --- |
| Input: single object or array | Both accepted; array processed per entry | Both accepted (`jsonParse` + `Array.isArray` check) | Yes |
| Required properties `nodes` + `connections` | Entry missing either → `skipped`, batch continues | Non-`--separate`: aborts with `File does not seem to contain valid workflows.`; `--separate`: skips the file with a warning | Yes (`--separate` semantics) |
| Workflow `id` is a string | Kept/stringified; generated when missing | String; generated when missing | Yes |
| Fresh `versionId` per import | New `uuid4` on every import | `workflow.versionId = uuid()` on every import | Yes |
| Node `id` generation | `uuid4` when missing | `uuid()` when missing | Yes |
| Old credential shape → new shape | `{"type": "name"}` → `{"type": {"id": null, "name": "name"}}` | `toNewCredentialFormat()` converts the same way | Yes |
| Credential `id` resolution | Not resolved locally — `id` stays `null`, the name is preserved; the batch import goes to m9m's import endpoint, which resolves it by **name + type** exactly like n8n | Resolved by matching credential **name + type** against the credential store; exactly one match wins | Yes (resolution happens server-side in m9m) |
| Unresolvable credential | Non-fatal, workflow still imports | Repaired/stripped, batch continues | Yes |
| Credentials on disabled nodes | Skipped — payload passed through untouched | Skipped — `replaceInvalidCredentials` skips `node.disabled` | Yes |
| `__aiGatewayManaged` credentials | Skipped — dropped from the m9m payload | Skipped — no record behind them | Yes |
| `active` on import | Forced `false`; `--active-state=from-json` honours the JSON field | `--activeState=false` (default) / `fromJson` | Yes |
| `tags` are objects | Normalized to `[{id, name}]` | Stored/exported as `{id, name}` objects | Yes |
| `typeVersion` type | Coerced to `int` (float `2.1` → `2`) | Kept as written (n8n writes minor versions as floats) | **No** — m9m's Go model types it `int` |
| Missing `typeVersion` | Defaults to `1` | Left as-is (the property is required in n8n's model) | **No** — bridge default |
| Unknown node types | Warning; the workflow is still imported | Imported as-is | **No** — m9m cannot execute the node |
| Nodes without a `type` | Dropped, count reported as a warning | Editor import filters them out and reports the count | Yes (editor rule) |
| `position` normalised | Missing/invalid → `[0, 0]` (ints) | `ensureNodePosition()` → `[0, 0]` | Yes |
| Connections to unknown nodes | Dropped (source key and edge target checked) | `sanitizeConnections()` drops them | Yes (editor rule) |
| Unnamed nodes | Get a unique name (display name or type) | Editor import assigns a unique name | Yes (editor rule) |
| Empty `nodes` array | Error → entry skipped | Accepted by the CLI assert | **No** — m9m would store a workflow that can never run |
| Content-policy violations | Not implemented | Violating workflow is skipped, rest of the batch imports | **No** — no policy engine in the bridge |
| Export shape | Accepts an array (n8n's `export:workflow` always emits one) | Always an array; `--separate` writes `<workflowId>.json` per workflow | Yes |
| Batch import target | `POST /api/v1/workflows/import` with a JSON array (fallback: `POST /api/v1/workflows` per workflow) | `n8n import:workflow` imports the whole file in one batch | Yes (m9m's endpoint mirrors the batch semantics) |
| One bad entry in a batch | Rejected on its own, the rest of the batch is imported | Skipped with a warning, the rest of the batch is imported | Yes |
| Workflow id already taken | m9m mints a fresh id instead of overwriting the existing workflow | n8n upserts under the same id | **No** — m9m never overwrites an unrelated workflow |
| Webhook activation | Import deactivates by default, so **imported workflows must be activated in m9m** to serve webhooks | n8n activates on import only with `--activeState=fromJson` | Yes (both need explicit activation) |

### Exact divergence points

Items **1–5** are verbatim the canonical list in
`docs/N8N_WORKFLOW_PARITY_REFERENCE.md` §5 — keep them in sync so the two
documents cannot drift. Items **6–10** are bridge-specific additions.

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
6. **`versionId` in the live path** (bridge-specific) — the background poller keeps
   n8n's `versionId` (so m9m tracks n8n's version), while the import path always
   assigns a new one (n8n parity).
7. **Live sync keeps `active`** (bridge-specific) — the poller normalizes with
   `active_state=from-json` because m9m only registers webhooks for active
   workflows, so deactivating there would silently break every webhook. The
   import path uses the n8n default (`false`), i.e. reference §2.4.
8. **Import target** (bridge-specific) — the import CLI pushes the whole batch to
   m9m's n8n import endpoint (`POST /api/v1/workflows/import`); m9m builds
   without that endpoint fall back to `POST /api/v1/workflows` per workflow,
   which cannot resolve credentials server-side.
9. **Workflow id collision** (bridge-specific) — m9m's import endpoint mints a
   fresh workflow id when the imported id is already taken (an unrelated
   workflow is never overwritten); n8n upserts under the same id.
10. **Webhook id on import** (bridge-specific) — the bridge does not regenerate
    webhook ids; m9m's import endpoint does (n8n editor parity) and keeps
    `parameters.path` in sync, while the live polling path keeps n8n's id so
    an existing webhook route stays stable.

---

## HTTP API (service mode)

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Service banner + phase list |
| `GET /health` | Liveness probe |
| `GET /status` | Last cycle summary |
| `GET /debug/credentials` | Decrypted credential envelopes read from n8n Postgres |
| `POST /sync` | Run one cycle now (`?force=true` clears the change cache) |
| `POST /sync/data-tables` | Run only the data-tables phase |

Cycle order: **data tables → credentials → workflows**.

Environment: `N8N_BASE_URL`, `N8N_API_KEY`, `M9M_BASE_URL`, `POLL_INTERVAL_SECONDS`,
`SYNC_ONLY_ACTIVE`, `SYNC_PORT`, `N8N_ENCRYPTION_KEY` / `GEN_ENCRYPTION_KEY`,
`N8N_PG_*`.

---

## Tests

```bash
python sync-service/test_sync.py     # plain runner
pytest sync-service/test_sync.py     # pytest
```

The suite covers the credential-decrypt vectors plus n8n parity cases: old
credential format, missing node id, tags as strings, `active: true` with
default deactivation, an entry missing `connections` (skipped, batch
continues), array input, `typeVersion` as float/string, credentials on
disabled nodes, `__aiGatewayManaged` credentials, nodes without a `type`,
position normalisation, connection sanitization, unnamed nodes, and the CLI
dry run making zero HTTP requests.
