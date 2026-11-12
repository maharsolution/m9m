# m9m ↔ n8n UI Parity — Feasibility, Gap Analysis & Implementation Plan

Status: historical research baseline; see Testing & Parity Harness below for current evidence. Original capability matrix and live probes describe the original inspection, not today's deployed state.
Date: 2026-11-11.
Sources inspected: m9m repo root (`c:\Users\mahar\OneDrive\workspace\m9m`) and the local read-only
n8n upstream checkout at `n8n/` (monorepo version **2.47.0**, `n8n/package.json:3`).
Orientation was done graphify-first (m9m `graphify-out/graph.json`, 10 937 nodes; n8n
`n8n/graphify-out/graph.json`, 191 177 nodes) before any file reads.

---

## Testing & Parity Harness — 2026-11-12 evidence refresh

The original research below is a historical baseline, not a current live capability inventory. Routes and editor components subsequently implemented must not be relabeled missing solely from the original matrix. The gap plan records local implementation progress; deployed behavior requires separate evidence.

Canonical saved report: `tmp/parity-results/fase-d-2026-11-12/parity-results.json`, generated `2026-11-11T22:50:30.399439+00:00`. It is selected automatically as the newest saved report that contains both the positive and the negative suite and whose totals match its own case verdicts; every earlier report under `tmp/parity-results/` is listed as superseded and is never merged into the totals.

- Total: **25/55 PASS; 30 FAIL or BLOCKED**. Positive **9/14**, negative **4/5**, focus **4/4**, credentials **2/2**, content type **5/5**, roundtrip **0/1**, extended contracts **1/24** (5 FAIL, 18 BLOCKED).
- `post_balance` is **green again**: f01/f02/f04 are XML round-trip equivalent (n8n 200, m9m 200) and f03 malformed JSON answers 422 on both hosts. The m9m client timeouts seen in the superseded `extended-all` run do not reproduce; that timeout observation is closed.
- The authentication rejection wire format now matches n8n: HTTP **401** when no credentials were presented (no header, wrong basic-auth credentials) or when the bearer token is missing, HTTP **403** when credentials were presented and are not acceptable (invalid bearer token, wrong API key). The body is **plain text**, the `Content-Type` header is **not sent**, and every rejection carries `WWW-Authenticate: Basic realm="Webhook"`. The negative suite moved from 1/5 to 4/5 because of this fix; the one remaining negative failure is a case whose expected status contradicts n8n's own behaviour (`n04` expects 401, n8n answers 403), and the case definition belongs to the parity worker.
- One residual authentication difference: for a malformed JWT n8n answers `jwt malformed` and for a disallowed algorithm `invalid algorithm`, while the deployed m9m build answers `invalid token`. The current working-tree source implements the full n8n error taxonomy but is not deployed (`GAP-2026-11-12-JWT-ERROR-BODY`).
- SQL nodes emit **one item per row**, not a `{"rows":[...]}` wrapper (`internal/nodes/database/rows.go`). Both database branches now answer HTTP 200 with 10 array items on each host and identical item key sets; the remaining difference is the row payload itself, not the shape.
- The Google Sheets resource locator is served by a `loadOptions` provider (`cmd/m9m/commands/load_options.go`, `internal/api/node_type_options.go`, `internal/nodes/productivity/google_sheets_options.go`): the access token is resolved **server-side** from the credential store through `AccessTokenForType("googleSheetsOAuth2Api")`, so the browser never receives a token.
- Signup returns 500 on both hosts while the case expects 200, and the two 500 bodies differ (n8n: "No item to return was found"; m9m: "Validation Switch: rules parameter is required"). The stored case path `/webhook/user-signup` is joined by `scripts/parity/runner.py` as base + `/webhook/` + path, so the harness probes `/webhook/webhook/user-signup`; the case definition and the signup fixture must be fixed before it can prove anything.
- Database cases return 200 with different row payloads on both branches. Matching failures are baseline blockers, not passes.
- The n8n REST API answered **HTTP 200** this run, so credential inventory, roundtrip, telemetry, sync and cron contracts compared and reported differences ("credential names/types differ", "nodes/parameters/credentials/connections differ"). Expression, async subworkflow, fault injection, load, UI journey, Go regression, drift-guard contracts and the missing n8n C6 sync fixtures are still BLOCKED pending fixtures/access.

Harness entry points: `scripts/parity/runner.py`, `compare.py`, `report.py`; case definitions under `scripts/parity/cases/`. Run `python scripts/parity/runner.py --help` for supported live options. Regenerate canonical webhook documentation with `python scripts/update_parity_sheets.py --update-docs`; use `--results <path>` for explicit evidence selection. The updater validates report totals against case verdicts and defaults to the newest report (by `generated_at`) that contains both the positive and negative suites, so a partial single-suite report can never silently become the canonical source. Reports are written continuously by the parity workers, so always check which file the updater selected.

`python scripts/update_parity_sheets.py --apply` signs its own RS256 JWT from `secrets/sheets-sa.json` (no `google-auth` dependency) and matches an existing sheet row only when the stored request is exactly the request of a canonical case — path, content type, payload and auth headers. Only result cells (D, F, G, H) of attributable rows are written, with RAW input; request columns, unrelated rows, formulas, tabs and row order are never touched, no row is cleared and no row is appended. Response bodies that carry credential material are withheld, not published. Every written range is read back and compared cell by cell. A dry run prints the plan plus a redacted `DIFF current -> planned` line per cell.

Sheet sync performed for this refresh: **30 cell updates over 9 rows** in the `Webhook Controlling` tab (D/F/G/H of rows 6, 9, 10, 11, 12, 13, 14, 15, 16), readback verified. Every written cell already held the planned value, so the sheet status columns were already in agreement with the canonical report and the write re-asserted them. Left unchanged with a recorded reason: `Webhook Controlling` rows 7, 8, 13 (n8n side), 14/15 (response bodies withheld — credential material), 17, 18; `Webhook Auth` rows 6, 7, 8 (the stored requests carry different credential headers and payloads than the harness fixtures); `Capabilty Gap` rows 5, 6 (the capability corrections are already present); the whole `Webhook Controlling Negative Test` tab (its header row places the request in column D while its data rows keep responses in columns E-H, so no cell can be attributed). The `Capabilty Gap` tab still carries its original wording plus the correction that an XML/JSON transform node exists and that `GET /api/v1/executions` answers HTTP 200.

A read-only drift check (`python scripts/parity/report.py --results tmp/parity-results/fase-d-2026-11-12/parity-results.json --drift-check`, public API key) reports **no drift** between the sheet, `workflow-test.MD` and the live report.

`workflow-test.MD` is rendered from one saved report, with every non-passing case listed explicitly, and **every status-bearing line uses one Markdown table shape** — `| Suite | Case | Path | n8n | m9m | Verdict | Evidence |` — so `report.py::document_drift` parses the document with the same rule it uses for the harness report; the shape is documented in Section F of that document. Section J labels the old September 19/19 claim historical and superseded and now states the same canonical numbers as Section B. `parity/parity_gaps.json` retains historical evidence plus explicit current observations (`GAP-2026-11-11-AUTH-WIRE` and `GAP-2026-11-11-APIKEY-TRANSPORT` are now RESOLVED; `GAP-2026-09-10-1`, `GAP-2026-09-10-2`, `G1`, `G2`, `G3` are recorded with their fix commits). `credentials_n8n_api.MD` distinguishes current m9m routes/envelopes from n8n public API behavior, including secret-bearing detail reads, the rejection wire format, the row-per-item SQL output and the server-side `loadOptions` credential resolution. No runtime code, plans, harness files or n8n workflows were edited for this refresh; no commit, push or deploy was performed.

## 1. Verdict

**Yes — n8n-UI parity is feasible in m9m, and m9m is already roughly half-way there.**
m9m already ships a Vue 3 + Vue Flow canvas editor (`web/src/components/workflow/WorkflowCanvas.vue`),
a searchable node palette (`NodePalette.vue`), a metadata-driven node parameter panel (`NodePanel.vue`),
an n8n-shaped execution inspector (`web/src/views/ExecutionDetail.vue`), and n8n-compatible
import/export. The n8n editor itself is built on the *same* canvas library family m9m already uses
(`@vue-flow/core`: m9m `^1.38.0` in `web/package.json:18`; n8n `1.48.0` in
`n8n/packages/frontend/editor-ui/package.json`). So the gap is **not** architectural — it is
(a) the depth of the parameter form system, (b) missing canvas affordances (sticky notes, node
groups, multi-select, copy/paste, undo/redo, auto-layout), and (c) several **backend endpoints that
exist in code but are not routed** (tags, workflow versions) or are missing entirely (node-type
credential metadata, pin-data, workflow settings fields).

Three levels of parity:

| Level | Definition | Feasible? | Notes |
|---|---|---|---|
| **(a) UX parity by reimplementation** | Same interaction model and feature set, own code, own visual design; n8n JSON stays the wire format | **Yes — recommended.** ~6–10 engineer-weeks spread over P0–P3 | No n8n source copied. Legal: n8n is Sustainable Use License (fair-code), see `n8n/LICENSE.md:16`; copy/paste of n8n frontend code is off-limits. n8n's own design system (`n8n/packages/frontend/@n8n/design-system/src/components/**`, ~120 components) is n8n code — reimplement, don't vendor. |
| **(b) Approximate parity** | Same feature set, n8n-*like* visuals (Tailwind-based, similar layout, similar icons) | **Yes** | m9m already uses Tailwind (`web/package.json:35`) and Heroicons; a visually close but non-identical editor is achievable. |
| **(c) Pixel-perfect clone** | Identical pixels, identical design-system components | **No.** | Requires copying n8n's design-system + canvas components (SUL-licensed, enterprise `.ee` files separately licensed — `n8n/LICENSE.md:6-9`), plus maintaining a pixel-diff harness against a moving upstream. Not legally safe, not economically sane. |

---

## 2. Capability matrix

Legend — m9m status: **exists** / **partial** / **missing**. Effort: S ≤ 3 d, M 4–10 d, L > 10 d.

| Area | n8n feature (source file) | m9m status | m9m file | Effort |
|---|---|---|---|---|
| Canvas engine | Vue Flow `<VueFlow>` + custom canvas wrapper — `n8n/packages/frontend/editor-ui/src/features/workflows/canvas/components/Canvas.vue:69,2063,2197` | **exists** (same lib, older minor) | `web/src/components/workflow/WorkflowCanvas.vue:3-6,173-206` | S (upgrade `@vue-flow/*` to 1.48/1.5) |
| Zoom / pan / fit-view / snap | `Canvas.vue:437,1580`; `:selection-key-code` at `:334` | **exists** (min-zoom 0.2, max 2, snap 20px, fit-on-init) | `WorkflowCanvas.vue:183-190` | S |
| Minimap | `Canvas.vue:70,2197` (`@vue-flow/minimap`) | **exists** | `WorkflowCanvas.vue:199-203` | S |
| Zoom controls | `Canvas.vue:2214` (`@vue-flow/controls`) | **exists** | `WorkflowCanvas.vue:196-198` | S |
| Node drag / connect / delete | `Canvas.vue` (`onConnect`), `CanvasNode.vue`, `CanvasNodeToolbar.vue` | **exists** (drag, connect, delete, edge removal) | `WorkflowCanvas.vue:26-92`; `web/src/components/nodes/BaseNode.vue:83-88` | S |
| Multi-select | `Canvas.vue:1193` (cmd/ctrl multi-select), `useCanvasNodeGroupSelection.ts` | **partial** — store supports arrays (`web/src/stores/workflowEditor.ts:17,157-172`) but canvas select handler selects one only | `WorkflowCanvas.vue:36-42` | S |
| Copy / paste / duplicate nodes | `n8n/.../src/app/views/NodeView.vue:201,265,622,680`; `useCanvasOperations.ts` | **missing** (shortcut presets exist but are never registered) | `web/src/composables/useKeyboardShortcuts.ts:283-300` (dead code) | M |
| Undo / redo | `n8n/.../src/app/models/history.ts`, `src/app/stores/history.store.ts`, `src/app/composables/useHistoryHelper.ts` | **missing** | — (no history in `web/src/stores/workflowEditor.ts`) | M |
| Keyboard shortcuts (editor) | `n8n/.../src/app/components/KeyboardShortcutTooltip.vue`; `NodeView.vue`, `Canvas.vue` | **missing in practice** — composable + presets written, never mounted | `web/src/composables/useKeyboardShortcuts.ts:309-425` | S |
| Sticky notes | `n8n/.../canvas/components/elements/nodes/render-types/CanvasNodeStickyNote.vue`, `.../nodes/toolbar/CanvasNodeStickyColorSelector.vue` | **missing** | — | M |
| Node groups (collapse/expand) | `n8n/.../canvas/nodeGroups.utils.ts`, `canvas/composables/useCanvasNodeGroupActions.ts`, `.../elements/groups/CanvasNodeGroupTitleBar.vue` | **missing** | — | L |
| Auto-arrange / layout | `n8n/.../canvas/composables/useCanvasLayout.ts:1` (`@dagrejs/dagre`) | **missing** | — (no dagre dep) | S–M |
| Node palette (search + categories) | `n8n/.../features/shared/nodeCreator/views/NodeCreation.vue`, `components/Panel/SearchBar.vue`, `components/Panel/NodesListPanel.vue`, `components/Renderers/CategorizedItemsRenderer.vue` | **partial** — search + category accordion + drag-to-canvas; no resource/action tabs, no community nodes, no favorites/recent | `web/src/components/workflow/NodePalette.vue:11-58,133-157` | M |
| "Get started" / onboarding panel | n8n node-creator view stack (`features/shared/nodeCreator/views/viewsData.ts`, `composables/useViewStacks.ts`) | **missing** | — | S |
| Dynamic parameter form (core of "detail edit workflow") | `n8n/.../features/ndv/settings/components/NodeSettings.vue`, `features/ndv/settings/composables/useNodeSettingsParameters.ts:41-54,208-257`, `features/ndv/parameters/components/ParameterInput.vue` | **partial** — renders string/number/boolean/options/json + `displayOptions` show/hide; **no** collection/fixedCollection/multiOptions/resourceLocator/resourceMapper/filterConditions; `typeOptions` not consumed | `web/src/components/workflow/NodePanel.vue:53-91,300-388` | L |
| Conditional fields / `displayOptions` | `useNodeSettingsParameters.ts:41-54,208-257` | **partial** — show/hide only; only **4 of 52** live node types carry `displayOptions` | `NodePanel.vue:53-91`; `internal/nodes/base/node.go:255-262` | M |
| `typeOptions` (rows, password, min/max, multipleValues) | n8n `INodeProperties.typeOptions` (`n8n/packages/workflow/src/Interfaces.ts:2264`) | **missing in UI** (field exists in Go DTO, never read by the panel) | `internal/nodes/base/node.go:248-251`; `web/src/types/node.ts:58-68` | S |
| Dynamic option loading (`loadOptions`, resource locator) | `n8n/.../features/ndv/parameters/composables/useResourceLocatorModes.ts`, `.../components/ResourceLocator/ResourceLocator.vue`, `useWorkflowResourcesLocator.ts` | **missing** — Go `Option` has no `action` field, so no async option loading is even representable | `internal/nodes/base/node.go:264-267`; `web/src/types/node.ts:43-47` | L |
| Credential picker in node panel | `n8n/.../features/credentials/components/CredentialPicker/CredentialPicker.vue`, `CredentialsDropdown.vue`, `NodeCredentials.vue` | **partial** — per-type `<select>` + "Create new" link; supported types come from a **hard-coded frontend map**, not from node metadata | `NodePanel.vue:139-201`; `web/src/types/node.ts:154-203` | M |
| Credential create/edit modal (schema-driven) | n8n credential editor + schema service | **partial** — frontend keeps its own static schema table; backend already serves `/credentials/schema/{type}` and `/credentials/types` | `web/src/lib/credentialSchema.ts:20-78`; `internal/api/routes.go:74-75` | M |
| Workflow name edit | `n8n/.../src/app/components/MainHeader/**` | **exists** | `web/src/components/workflow/WorkflowEditorToolbar.vue:103-128`; `web/src/stores/workflowEditor.ts:177-185` | S |
| Active / deactivate toggle | n8n main header + `ActivationModal.vue` | **exists** (no conflict modal for webhook collisions) | `WorkflowEditorToolbar.vue:162-177`; `internal/api/routes.go:34-35` | S |
| Debug toggle | n8n dev-mode toggle | **exists** (m9m-specific `debug` flag) | `WorkflowEditorToolbar.vue:179-198`; `internal/model/workflow.go:44` | S |
| Workflow settings modal (execution order, error workflow, timeout, save manual executions, caller policy) | `n8n/.../features/workflows/components/WorkflowSettings/WorkflowSettings.vue:191-215,223,239,471-502` | **missing in UI** — backend `WorkflowSettings` only has `executionOrder, timezone, saveDataError, saveDataSuccess, saveManualExecutions` | `internal/model/workflow.go:119-124`; `web/src/types/workflow.ts:57-63` | M (UI) + M (Go fields) |
| Tags (add/remove tag modal, filter) | n8n workflow tags + `WorkflowTags` UI | **missing in UI** — tag CRUD handlers exist but are **not routed** (live `/api/v1/tags` → 404) | `internal/api/tags.go:11-63`; `internal/tags/tag_handler.go:24-29` | S |
| Workflow tag persistence | n8n `tags` on workflow | **exists in data model + storage** (`tags JSONB`) but no UI/API route | `internal/model/workflow.go:25`; `internal/storage/postgres.go:60,177,190,227` | S |
| Version history / diff / restore | `n8n/.../features/workflows/workflowHistory/views/WorkflowHistory.vue`, `WorkflowHistoryDiff.vue`, `components/WorkflowHistoryList.vue` | **partial/dead** — frontend components (`web/src/components/version/VersionHistory.vue`, `VersionCompare.vue`, `stores/versions.ts`, `api/versions.ts`) + Go handler (`internal/versions/version_handler.go:25-35`) exist but are **not registered** (live `/api/v1/workflows/{id}/versions` → 404) and not routed in `web/src/router/index.ts` | see files | M (mostly wiring) |
| Import / export (n8n JSON) | n8n editor import/export + CLI parity | **exists** | `internal/api/routes.go:27-30`; `internal/api/workflow_import_export.go`; `web/src/api/workflows.ts:101-166`; `docs/WORKFLOW_IMPORT_EXPORT.md`; `docs/N8N_WORKFLOW_PARITY_REFERENCE.md` | S |
| Duplicate workflow | `n8n/.../features/workflows/components/DuplicateWorkflowDialog.vue` | **exists** (client-side clone, no modal) | `web/src/api/workflows.ts:56-63`; `internal/api/routes.go:38` | S |
| Move to folder / project | `n8n/.../src/features/core/folders/**`; projects are cloud-only | **missing** — `WorkspaceID`/`ProjectID` exist in the Go model but are unused by API/UI | `internal/model/workflow.go:51,60` | L |
| Share (team, EE) | `n8n/.../features/workflows/components/WorkflowShareModal.ee.vue` (enterprise-licensed) | **missing** (out of scope for SUL-licensed reimplementation anyway) | — | L |
| Manual "Execute workflow" | n8n canvas run button | **exists** but result is only `console.log`ed — no in-editor execution panel | `web/src/views/WorkflowEditor.vue:118-132` | S |
| Execution data panel (table / tree / JSON) | `n8n/.../features/ndv/runData/components/RunDataTable.vue`, `RunDataJson.vue`, `RunDataSearch.vue`, `VirtualSchema.vue` | **partial/exists** — separate route, not docked in the editor; schema/table/json tabs present | `web/src/views/ExecutionDetail.vue:1-35`; `web/src/components/execution/ExecutionDataView.vue` | M |
| Pin data (per-node) | `n8n/.../features/ndv/runData/components/RunDataPinButton.vue` | **missing** — `pinData` is imported/exported and stored, but no endpoint to set it and no UI | `internal/model/workflow.go:24`; `internal/compatibility/workflow_importer.go:370-371` | M |
| Log streaming (live run) | n8n push channel / logs panel (`features/execution/logs/components/LogsPanel.vue`) | **partial** — websocket exists in backend; editor run is request/response only | `internal/api/routes.go:118` (`/push`) | M |
| Execution list + retry / retry-node | n8n executions view | **exists** | `internal/api/routes.go:49-57`; `web/src/api/executions.ts` | S |
| Node-type metadata feed | n8n `src/app/stores/nodeTypes.store.ts` + `packages/workflow/src/Interfaces.ts:2673` (`INodeType`) | **partial** — `/node-types` returns name/displayName/description/category/version/defaults/inputs/outputs/properties. **Missing**: `credentials`, `group`, `usableAsTool`, `icon`, `codex`; `version` hard-coded to `1` | `internal/api/health.go:161-189`; `internal/engine/engine.go:97-106,229-239` | M |

### 2.1 Live-verified facts (read-only probes against the deployed server `187.77.113.218:8080`)

- `GET /api/v1/node-types` → **52** node types; **50** have non-empty `properties`; only **4** carry `displayOptions`.
- Top-level keys returned: `description, inputs, category, version, outputs, defaults, name, properties, displayName` — **no** `credentials`, `group`, `usableAsTool`, `icon`.
- Property-level keys returned: `options, description, type, default, displayOptions, required, name, typeOptions, displayName`.
- `GET /api/v1/tags` → **404** (handler exists, route missing).
- `GET /api/v1/workflows/{id}/versions` → **404** (handler exists, route missing).

---

## 3. Backend gaps (Go)

### 3.1 Register the handlers that already exist (cheapest wins)

| Change | File | Detail |
|---|---|---|
| Add tag routes | `internal/api/routes.go` (near `:68-82`) | `api.HandleFunc("/tags", s.ListTags).Methods("GET","OPTIONS")`, `POST`; `"/tags/{id}"` `PUT/PATCH/DELETE` — handlers already exist in `internal/api/tags.go:11-63` |
| Add version routes | `internal/api/routes.go` | `"/workflows/{id}/versions"` GET/POST, `"/workflows/{id}/versions/{versionId}"` GET/DELETE, `.../restore` POST, `.../compare` GET — handlers exist in `internal/versions/version_handler.go:25-35` but need a `VersionManager` instance wired in `cmd/m9m/commands/serve.go` (currently only `apiServer`/`webhookHandler`/`webHandler` are registered — `serve.go:343,356,368`) |
| Alternative (cleaner) | `internal/tags/tag_handler.go:24-29` | `TagHandler` already registers `/tags` on the `/api/v1` subrouter — instantiate in `serve.go` instead of duplicating routes in `routes.go` |

### 3.2 Extend node-type metadata (`/node-types`)

File: `internal/api/health.go:161-224` (handlers) + `internal/engine/engine.go:97-106` (`NodeTypeInfo`) +
`internal/nodes/base/node.go:70-77` (`NodeDescription`).

Add to the payload (n8n `INodeType` parity):

```go
type NodeTypeInfo struct {
    TypeID      string              `json:"name"`
    DisplayName string              `json:"displayName"`
    Description string              `json:"description"`
    Category    string              `json:"category"`
    Version     int                 `json:"version"`
    Inputs      []string            `json:"inputs"`
    Outputs     []string            `json:"outputs"`
    Properties  []base.NodeProperty `json:"properties,omitempty"`
    // NEW:
    Group        []string             `json:"group,omitempty"`        // n8n: ["trigger"], ["transform"], ...
    Credentials  []NodeCredentialSpec `json:"credentials,omitempty"`  // {name, required}
    UsableAsTool bool                `json:"usableAsTool,omitempty"`
    Icon         string              `json:"icon,omitempty"`
    IconColor    string              `json:"iconColor,omitempty"`
    Docs         string              `json:"docs,omitempty"`
}
```

Also required:

- `Version` is currently hard-coded to `1` (`internal/engine/engine.go:234`). Add a `Version int` field to
  `base.NodeDescription` (`internal/nodes/base/node.go:70-77`) and thread it through
  `GetRegisteredNodeTypes()`; otherwise every imported n8n workflow's `typeVersion` is meaningless in the UI.
- `base.Option` (`internal/nodes/base/node.go:264-267`) has only `Name`/`Value`. n8n's `INodePropertyOptions`
  also carries `action` (async `loadOptions`) and `description`. Add `Action string \`json:"action,omitempty"\``
  + `Description string` so the frontend can support dynamic dropdowns later.
- `base.NodeProperty` already carries `TypeOptions` and `DisplayOptions`
  (`internal/nodes/base/node.go:248-259`) — good. But `DisplayOptions` is `map[string]interface{}`; the
  frontend must be able to read `{show:{param:[values]}}` / `{hide:{param:[values]}}` (it already does:
  `web/src/components/workflow/NodePanel.vue:53-91`).

### 3.3 New / changed endpoints for the editor

| Purpose | Proposed route | Handler file | DTO |
|---|---|---|---|
| Node-type metadata (existing, extend) | `GET /api/v1/node-types` | `internal/api/health.go:161` | `NodeTypeInfo` (§3.2) |
| Credential metadata per node type | `GET /api/v1/node-types/{name}/credentials` (optional) | new `internal/api/node_type_credentials.go` | `[]{name, required}` — or fold into §3.2 (recommended) |
| Workflow settings read/write | `GET/PATCH /api/v1/workflows/{id}/settings` | new `internal/api/workflow_settings.go` | `model.WorkflowSettings` — extend `internal/model/workflow.go:119-124` with `ErrorWorkflow`, `Timeout` (seconds), `CallerPolicy`, `AvailableInMCP`, `SaveDataErrorExecution`, `SaveDataSuccessExecution` |
| Tags CRUD | `GET/POST /api/v1/tags`, `PUT/DELETE /api/v1/tags/{id}`, `POST /api/v1/workflows/{id}/tags` | `internal/api/tags.go:11` (exists) + new workflow-tag glue | `storage.Tag` |
| Versions | `GET/POST /api/v1/workflows/{id}/versions`, `.../{versionId}`, `.../restore`, `.../compare` | `internal/versions/version_handler.go:25` (exists, unwired) | `versions.WorkflowVersion` |
| Pin data | `PUT /api/v1/workflows/{id}/pin-data` | new `internal/api/pin_data.go` | `map[string][]model.DataItem` (model field already exists: `internal/model/workflow.go:24`) |
| Execution pin / step run | `POST /api/v1/executions/{id}/pin` | new | `{nodeName, items}` |
| Node run data for the editor dock | `GET /api/v1/executions/{id}/nodes/{nodeName}` | new (`internal/api/executions.go`) | `model.DataItem[]` (backend already stores `nodeData`: `web/src/types/workflow.ts:102`) |
| Credential schema (exists) | `GET /api/v1/credentials/schema/{type}`, `GET /api/v1/credentials/types` | `internal/api/routes.go:74-75` | `CredentialSchema` — frontend should consume instead of its static table (`web/src/lib/credentialSchema.ts:20`) |
| Credential validation on save | `POST /api/v1/credentials/test` | `internal/api/routes.go:76` | exists |

Do **not** invent endpoints that already exist — verified live list in `internal/api/routes.go:13-152`.

### 3.4 Packaging / serving (no change needed, but relevant)

`Dockerfile:8-20` builds `web/dist` in a Node stage; `Dockerfile:48` copies it to
`internal/web/dist`; `internal/web/handler.go:22-23` embeds it via `//go:embed all:dist/*`;
`internal/web/handler.go:58` registers the SPA routes. Consequence: **every frontend change ships
inside the Go binary** → bundle size directly affects the 30 MB single-binary claim in `CLAUDE.md`,
and the Harbor pipeline (`.github/workflows/harbor-image.yml`) rebuilds both stages. Any new npm
dependency (e.g. `@dagrejs/dagre`, `@vue-flow/node-resizer`) increases that artifact.

---

## 4. Frontend gaps (concrete file list under `web/src/`)

### 4.1 Files to create

| New file | Purpose |
|---|---|
| `web/src/components/workflow/NodePalette/Panel.vue` | n8n-style node creator: search, "Get started" view stack, category tree, favorites/recent, resource/action tabs |
| `web/src/components/workflow/NodePalette/SearchBar.vue` | debounced fuzzy search over `nodeTypes` |
| `web/src/components/workflow/parameters/ParameterField.vue` | one renderer per `NodeProperty.type`: `string, number, boolean, options, multiOptions, collection, fixedCollection, json, dateTime, color, resourceLocator, resourceMapper` |
| `web/src/components/workflow/parameters/ResourceLocatorField.vue` | async option loading (`action: loadOptions`) + cached mode list |
| `web/src/components/workflow/parameters/CollectionField.vue` / `FixedCollectionField.vue` | repeatable item groups (n8n `collection` / `fixedCollection`) |
| `web/src/components/workflow/parameters/ExpressionInput.vue` | `{{ $json.foo }}` expression editor with autocomplete against upstream node outputs |
| `web/src/components/workflow/parameters/DisplayOptionsGuard.vue` | wraps fields, evaluates `show`/`hide` against current parameters (moves the logic out of `NodePanel.vue`) |
| `web/src/components/workflow/StickyNote.vue` | sticky-note node type for Vue Flow (model field `notes` exists: `internal/model/workflow.go:80`) |
| `web/src/components/workflow/NodeGroup.vue` | group frame + title bar + collapse/expand |
| `web/src/components/workflow/WorkflowSettingsModal.vue` | execution order, error workflow, timeout, save manual executions, caller policy |
| `web/src/components/workflow/TagsModal.vue` | add/remove workflow tags |
| `web/src/components/workflow/VersionHistoryDrawer.vue` | wraps the existing `components/version/VersionHistory.vue` |
| `web/src/components/workflow/ExecutionDock.vue` | docks run data (table/tree/JSON) beside the canvas, like n8n's bottom panel |
| `web/src/stores/history.ts` | undo/redo stack (snapshot-based, mirroring n8n's `src/app/models/history.ts`) |
| `web/src/stores/tags.ts` | tag list + CRUD (currently no tag client exists — `web/src/api/` has no `tags.ts`) |
| `web/src/api/tags.ts` | client for `/tags` + workflow-tag assignment |
| `web/src/api/pinData.ts` | client for the pin-data endpoint |

### 4.2 Files to modify

| File | Change |
|---|---|
| `web/src/components/workflow/WorkflowCanvas.vue` | multi-select, box-select, copy/paste, sticky notes, groups, auto-arrange button, node resize, edge-label + edge-toolbar, execution overlay (`CanvasRunWorkflowButton` analogue) |
| `web/src/components/workflow/NodePanel.vue` | replace inline field rendering with `ParameterField`; consume `typeOptions`, `multiOptions`, `collection`, `fixedCollection`, `resourceLocator`; credential types from node metadata (not the hard-coded map at `web/src/types/node.ts:154-203`) |
| `web/src/components/workflow/NodePalette.vue` | delegate to the new panel; keep drag-and-drop contract (`application/vueflow` MIME, `WorkflowCanvas.vue:97`) |
| `web/src/components/workflow/WorkflowEditorToolbar.vue` | add settings, tags, versions, "auto-arrange", undo/redo buttons; keep existing emits stable |
| `web/src/views/WorkflowEditor.vue` | wire keyboard shortcuts, execution dock, version drawer, settings modal; replace the `console.log` in `executeWorkflow` (`:118-132`) with the dock |
| `web/src/stores/workflowEditor.ts` | add history stack hooks, multi-node selection ops, sticky/group node kinds, `duplicateNodes`, `pasteNodes` |
| `web/src/stores/nodes.ts` | cache node-type metadata, expose `getCredentialsFor(nodeType)`, category grouping driven by the new `group` field |
| `web/src/types/node.ts` | extend `NodeType` with `group`, `usableAsTool`, `icon`, `iconColor`; extend `NodeProperty` with `action`, `collection`/`fixedCollection` variants |
| `web/src/types/workflow.ts` | extend `WorkflowSettings` with the n8n fields |
| `web/src/lib/workflowGraph.ts` | map sticky/group nodes to Vue Flow node kinds; keep `buildFlowNodes`/`buildFlowEdges`/`parseEdgeId` contracts (used by `ExecutionDetail.vue` too) |
| `web/src/composables/useKeyboardShortcuts.ts` | actually mount `useWorkflowShortcuts()` in the editor; add canvas-specific keys (`Delete`, `Ctrl+D`, `Ctrl+C/V`, `Ctrl+Z/Shift+Z`, `Ctrl+= / - / 0`) |
| `web/src/router/index.ts` | add `workflows/:id/versions` route (or render the drawer inside the editor) |
| `web/src/views/WorkflowList.vue` | tag filter chips, folders column, share/move actions |
| `web/src/components/nodes/BaseNode.vue` | add per-node status icons (executed/error), notes/sticky variant, tooltips, icon from node metadata |

### 4.3 npm dependencies to add

| Package | Why | Size impact |
|---|---|---|
| `@dagrejs/dagre` (^1.1.4, same as n8n) | auto-arrange / tidy-up | small |
| `@vue-flow/node-resizer` (^1.5.0, same as n8n) | resizable nodes / groups | small |
| `@vue-flow/*` upgrade to n8n's pins (`core` 1.48.0, `minimap` 1.5.4, `controls` 1.1.3, `background` 1.3.2) | parity with n8n canvas behaviour | small |
| `fuse.js` (optional) | fuzzy node-palette search like n8n | ~25 KB |
| `dagre` alternatives: none needed | — | — |

No new UI-kit dependency is required: Tailwind (`web/package.json:35`) + Headless UI (`:14`) already cover
an n8n-like look. Adopting n8n's `@n8n/design-system` is **not** an option (SUL-licensed source).

---

## 5. Phased implementation plan

Effort is in person-days for one engineer familiar with the repo.

### P0 — Make the existing editor feel like n8n (≈ 8–12 d)

**Goal:** close the "obvious gaps" users notice first: multi-select, copy/paste, delete, undo/redo,
keyboard shortcuts, auto-arrange, sticky notes, and accurate node metadata.

- Frontend:
  - `web/src/components/workflow/WorkflowCanvas.vue` — multi-select (`selection-key-code`), box select,
    copy/paste/duplicate, node resize, `VueFlow` instance handle for zoom/fit API.
  - `web/src/stores/history.ts` (new) + `web/src/stores/workflowEditor.ts` — snapshot-based undo/redo
    (debounced, 50-step cap, mirroring n8n's `src/app/models/history.ts`).
  - `web/src/composables/useKeyboardShortcuts.ts` — mount in `WorkflowEditor.vue`; wire the presets
    that already exist.
  - `web/src/components/workflow/StickyNote.vue` (new) + sticky node kind in `workflowGraph.ts`.
  - `web/src/components/workflow/NodePalette.vue` — multi-select-aware drop, "Add nodes" empty-state.
  - `web/src/components/workflow/NodeGroup.vue` (new) — basic group frame (collapse/expand deferred to P2).
  - Auto-arrange: `web/src/lib/autoLayout.ts` (new, dagre) + toolbar button.
- Backend:
  - `internal/engine/engine.go:97-106,229-239` — add `Group`, `Credentials`, `Icon`, `IconColor`,
    `UsableAsTool`; stop hard-coding `Version: 1`.
  - `internal/nodes/base/node.go:70-77` — add `Version`, `Group`, `Credentials` to `NodeDescription`.
  - `internal/api/health.go:161-224` — emit the extended payload.
  - `internal/nodes/base/node.go:264-267` — add `Option.Action`, `Option.Description`.
- Acceptance:
  - Ctrl/Cmd+C, Ctrl+V, Ctrl+D, Delete, Ctrl+Z/Ctrl+Shift+Z all work on the canvas.
  - Sticky note can be added, edited, moved, deleted; survives save/reload/export.
  - "Tidy up" produces a readable left-to-right layout for a 20-node workflow.
  - `/api/v1/node-types` returns `group`, `credentials`, `version` matching each node's real
    `typeVersion` (spot-check 10 nodes against `internal/nodes/**`).
  - Round-trip: export → import → export yields byte-identical JSON (existing test harness in
    `internal/api/workflows_import_export_test.go`).
- Effort: frontend 5–7 d, backend 2–3 d, tests 1–2 d. **≈ 10 d.**

### P1 — Metadata-driven parameter panel (the "detail edit workflow" core) (≈ 12–18 d)

**Goal:** n8n-grade per-node forms driven by node metadata, not hard-coded field lists.

- Frontend:
  - `web/src/components/workflow/parameters/**` — `ParameterField.vue` + per-type renderers
    (options, multiOptions, collection, fixedCollection, json, number, boolean, resourceLocator,
    resourceMapper, filterConditions).
  - `DisplayOptionsGuard.vue` — declarative `show`/`hide` evaluation (move `NodePanel.vue:53-91` here).
  - `typeOptions` support (rows, password, min/max, multipleValues, numberPrecision).
  - `ResourceLocatorField.vue` — async option loading against a new backend endpoint.
  - Credential picker sourced from node metadata (`credentials[]`), not the static map.
- Backend:
  - `internal/api/node_type_options.go` (new) — `POST /api/v1/node-types/{name}/options`
    body `{property, params}` → `[{name, value}]`; executes the node's `loadOptions` hook.
  - Extend `base.NodeProperty` to support `collection`/`fixedCollection` shapes
    (`internal/nodes/base/node.go:239-262`).
  - Audit + fill metadata for all 52 node types (target: ≥90% of properties with correct
    `displayName`, `type`, `default`, `description`, `required`, `options`).
- Acceptance:
  - For every registered node type, opening the panel renders every declared property with the right
    widget, and toggling a controlling parameter immediately shows/hides dependent fields.
  - Unknown property types degrade gracefully to the existing JSON editor (never crash).
  - No hard-coded per-node field lists remain in the frontend.
- Effort: frontend 8–11 d, backend 4–6 d. **≈ 15 d.**

### P2 — Workflow-level features (≈ 8–12 d)

**Goal:** settings, tags, versions, folders, execution dock.

- Backend:
  - Register tag + version routes (`internal/api/routes.go`, `cmd/m9m/commands/serve.go:343,356,368`).
  - Extend `model.WorkflowSettings` (`internal/model/workflow.go:119-124`) with `ErrorWorkflow`,
    `Timeout`, `CallerPolicy`, `AvailableInMCP`; expose `GET/PATCH /api/v1/workflows/{id}/settings`.
  - Pin data: `PUT /api/v1/workflows/{id}/pin-data`.
- Frontend:
  - `WorkflowSettingsModal.vue`, `TagsModal.vue`, `VersionHistoryDrawer.vue` (reuse the existing
    `components/version/VersionHistory.vue` + `VersionCompare.vue` + `stores/versions.ts`).
  - `ExecutionDock.vue` — dock the existing `components/execution/ExecutionDataView.vue` next to the canvas
    during a manual run; wire `web/src/views/WorkflowEditor.vue:118-132` to it.
  - `web/src/api/tags.ts`, `web/src/stores/tags.ts`.
- Acceptance:
  - Tag add/remove persists and is visible in the workflow list filter.
  - Version snapshot/restore/diff works end-to-end from the editor.
  - A manual run shows per-node input/output in the editor without leaving the page.
- Effort: **≈ 10 d.**

### P3 — Long-tail parity (≈ 15–25 d, optional)

- Node groups with collapse/expand + group-level execution status.
- Expression editor with autocomplete against upstream node outputs.
- Resource mapper / filter-conditions UI.
- Folders & projects organisation (`WorkspaceID`/`ProjectID` already in `internal/model/workflow.go:51,60`).
- Share/permissions (note: n8n's share UI is enterprise-licensed — treat as a differentiator to design
  from scratch, not to copy).
- "Get started" onboarding panel, AI-assisted node configuration (m9m already has `components/ai/AgentAI.vue`).
- Live log streaming over the existing `/push` websocket (`internal/api/routes.go:118`).

---

## 6. Risks & constraints

1. **Licensing (highest risk).** n8n is under the **Sustainable Use License (fair-code)**, not MIT/Apache —
   `n8n/LICENSE.md:16`; `.ee`-suffixed files are separately enterprise-licensed (`n8n/LICENSE.md:6-9`).
   Consequence: **do not copy n8n frontend source** (canvas components, design system, node creator).
   Reimplement the UX; use n8n only as a behavioural reference and for JSON schema parity. m9m's own
   positioning (CLAUDE.md: "n8n alternative without the bugs") is compatible with this as long as the code
   is original.
2. **Metadata completeness is the real blocker for P1.** Only **4 of 52** live node types expose
   `displayOptions`, and `Option` has no `action`, so no dynamic option loading is representable.
   A "n8n-identical parameter panel" is impossible until the Go node metadata is filled in — that is a
   **node-by-node content project**, not a UI project.
3. **Bundle size vs the single-binary claim.** The frontend is embedded into the Go binary
   (`internal/web/handler.go:22-23`) and shipped via Harbor (`Dockerfile:8-20,48`). Every dependency
   (dagre, node-resizer, fuse.js) and every large editor view lands in the artifact that CLAUDE.md
   describes as ~30 MB. Budget deps, lazy-load the execution dock and version diff views.
4. **Canvas performance.** n8n's own canvas needed viewport auto-adjust (`useViewportAutoAdjust.ts`),
   edge-render optimisations (`elements/edges/utils/getEdgeRenderData.ts`) and background-tab fit-view
   handling (`Canvas.vue:1933-1941`) to stay smooth. m9m's canvas currently re-derives
   `flowNodes`/`flowEdges` on every workflow mutation (`WorkflowCanvas.vue:23-24` computed from the
   store) — that will degrade with hundreds of nodes; plan node-level memoisation + `v-memo`.
5. **Canvas-library version drift.** m9m is on `@vue-flow/core` 1.38.0, n8n on 1.48.0. Behavioural
   differences in selection/keyboard handling are expected; upgrading is cheap but must be regression-tested
   against the existing drag/connect/delete flows.
6. **JSON round-trip compatibility.** The editor must keep emitting the n8n wire format
   (`internal/compatibility/workflow_importer.go:1104`, export at `:1169-1179`). Any new editor concept
   (sticky notes, groups, pin data) must be representable in that format or be explicitly flagged as
   m9m-only — sticky notes already are (n8n uses `type: "n8n-nodes-base.stickyNote"`; confirm the exact
   upstream shape before emitting). Guard with the existing
   `internal/api/workflows_import_export_test.go` assertions (`"staticData","pinData","meta","tags","versionId"` at `:183`).
7. **Dead code trap.** `web/src/composables/useKeyboardShortcuts.ts`, `web/src/components/version/**`,
   `web/src/stores/versions.ts`, `internal/api/tags.go`, `internal/versions/version_handler.go` all look
   implemented but are unreachable (no route / no mount). Any plan that assumes they work will be wrong;
   the first task in P0/P2 is to wire them and prove them with an integration test.
8. **Two tag implementations.** `internal/api/tags.go` (storage-backed, `storage.Tag`) and
   `internal/tags/*` (manager + own storage interface) coexist. Pick one before exposing the API, or the
   UI will see two divergent tag stores.
9. **Vue 3 + embed coupling.** `web/` is built by Vite and embedded at compile time; no CDN, no runtime
   plugin loading. Any n8n-style plugin/marketplace behaviour must be implemented in Go (m9m already has
   `internal/plugins/plugin_handler.go`).
10. **Local `n8n/` checkout is gitignored** (`.gitignore:140`) — it is a read-only reference, never stage it;
    `n8n/graphify-out/` must not be committed either.

---

## 7. Recommendation

**Cheapest high-value path:** P0 first (≈10 days) — the canvas already exists and already uses Vue Flow,
so multi-select + copy/paste + undo/redo + sticky notes + tidy-up + correct node metadata
(`group`, `credentials`, real `version`) deliver the "feels like n8n" jump for the least money.
Then P1 (≈15 days), the metadata-driven parameter panel, which is where n8n users spend most of their
time ("detail edit workflow"). P2 next (≈10 days) for settings/tags/versions/execution dock, which is
mostly *wiring existing code* rather than new code. P3 only if budget allows.

**Defer:** node groups with collapse/expand, resource mapper, expression autocomplete, folders/projects,
share/permissions, live log streaming, "Get started" onboarding.

**Why this order:** the two highest-risk assumptions (n8n-equivalent parameter forms, and "the editor
features exist already") are both cheap to falsify early — P0's backend work (extend `NodeTypeInfo`,
stop hard-coding `version`) is 2–3 days and unblocks everything downstream, while P1's true cost driver
is **metadata authoring for 52 node types in Go**, which should be scheduled as content work in parallel
with the frontend, not discovered late.

**Explicitly out of scope / never:** copying n8n source or its design system; pixel-for-pixel cloning;
anything from n8n's `.ee` tree.

---

## Appendix — verification log

- graphify orientation: `graphify query` on m9m (10 937 nodes) and n8n (191 177 nodes); `graphify explain
  "WorkflowCanvas"` on m9m (22 connections: store, `workflowGraph.ts`, `BaseNode.vue`).
- m9m files read: `web/package.json`, `web/src/views/WorkflowEditor.vue`, `web/src/views/WorkflowList.vue`,
  `web/src/views/ExecutionDetail.vue`, `web/src/components/workflow/{WorkflowCanvas,NodePanel,NodePalette,WorkflowEditorToolbar}.vue`,
  `web/src/components/nodes/BaseNode.vue`, `web/src/components/execution/ExecutionDataView.vue`,
  `web/src/components/version/VersionHistory.vue`, `web/src/stores/{workflowEditor,nodes,versions}.ts`,
  `web/src/lib/{workflowGraph,credentialSchema}.ts`, `web/src/api/{workflows,executions,nodes,versions,credentials}.ts`,
  `web/src/composables/useKeyboardShortcuts.ts`, `web/src/types/{node,workflow}.ts`, `web/src/router/index.ts`,
  `internal/api/{routes.go,health.go,workflows.go,tags.go,credentials.go}`, `internal/engine/engine.go`,
  `internal/nodes/base/node.go`, `internal/model/workflow.go`, `internal/storage/postgres.go`,
  `internal/versions/version_handler.go`, `internal/tags/tag_handler.go`, `internal/compatibility/workflow_importer.go`,
  `internal/web/handler.go`, `Dockerfile`, `cmd/m9m/commands/serve.go`, `docs/{N8N_WORKFLOW_PARITY_REFERENCE,WORKFLOW_IMPORT_EXPORT,CREDENTIALS}.md`.
- n8n files read: `n8n/package.json`, `n8n/LICENSE.md`,
  `n8n/packages/frontend/editor-ui/package.json`,
  `n8n/packages/frontend/editor-ui/src/features/workflows/canvas/**` (Canvas.vue, WorkflowCanvas.vue,
  useCanvasLayout.ts, nodeGroups.utils.ts, node render-types incl. CanvasNodeStickyNote.vue,
  groups/CanvasNodeGroupTitleBar.vue, handles, edges),
  `n8n/packages/frontend/editor-ui/src/features/ndv/**` (NodeDetailsView.vue, NodeSettings.vue,
  useNodeSettingsParameters.ts, ParameterInput*.vue, FixedCollection/**, ResourceLocator/**,
  ResourceMapper/**, runData/**),
  `n8n/packages/frontend/editor-ui/src/features/shared/nodeCreator/**`,
  `n8n/packages/frontend/editor-ui/src/features/workflows/components/WorkflowSettings/WorkflowSettings.vue`,
  `n8n/packages/frontend/editor-ui/src/features/workflows/workflowHistory/**`,
  `n8n/packages/frontend/editor-ui/src/features/credentials/components/**`,
  `n8n/packages/frontend/editor-ui/src/features/execution/**`,
  `n8n/packages/frontend/editor-ui/src/app/{views/NodeView.vue,stores/{nodeTypes,history}.store.ts,models/history.ts,composables/{useHistoryHelper,useCanvasOperations}.ts}`,
  `n8n/packages/frontend/@n8n/design-system/src/components/**`, `n8n/packages/workflow/src/Interfaces.ts`.
- Live read-only probes: `GET /api/v1/node-types` (52 types), `GET /api/v1/tags` (404),
  `GET /api/v1/workflows/test/versions` (404) on `http://187.77.113.218:8080`.
