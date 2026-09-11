# n8n Core Nodes — Reference for m9m Implementation

> **Why this file exists**
>
> On 2026-09-09 the user reported that the m9m Execution Detail page was not
> matching n8n's debug UX:
>
> 1. The workflow canvas nodes in the execution view were not connected.
> 2. The right-side per-node panel did not match n8n's Node Details View
>    (NDV) — it dumped raw JSON instead of:
>    - Showing **parameters and settings specific to the node type** (e.g.
>      HTTP Request → URL/method/headers/auth, Set → assignments, IF →
>      left/right/operator).
>    - Letting the user switch between **Schema / Table / JSON** views.
>    - **Following the node's input/output type** instead of always rendering
>      everything as a flat JSON blob.
>
> This document is the canonical reference for what n8n's core nodes look
> like, how n8n's UI surfaces them, and what m9m needs to add to reach
> parity. It is sourced from the n8n docs at
> <https://docs.n8n.io/integrations/builtin/core-nodes>, the n8n source on
> GitHub (`packages/frontend/editor-ui/src/features/ndv/...`), and DeepWiki
> walk-throughs of the editor.

---

## 1. The n8n model in 30 seconds

Every n8n workflow is a DAG of **nodes**. Each node has:

| Concept              | Meaning                                                                  |
| -------------------- | ------------------------------------------------------------------------ |
| **type**             | `n8n-nodes-base.<service>` (e.g. `n8n-nodes-base.httpRequest`).         |
| **typeVersion**      | Integer schema version. UI changes shape per version.                    |
| **parameters**       | User-supplied config (URL, method, headers, etc.).                      |
| **credentials**      | Map of credential-type → credential id used by this node.               |
| **inputs/outputs**   | Connection slots — usually `["main"]`, IF can have two, Switch has N.   |
| **position**         | Canvas x/y.                                                              |
| **disabled / notes** | Misc.                                                                    |

A node's **runtime behaviour** is described declaratively by
`INodeTypeDescription` (server-side metadata shipped with the node).
m9m's equivalent today is `base.NodeDescription`
(`internal/nodes/base/node.go`), which exposes only:

```go
type NodeDescription struct {
    Name        string
    DisplayName string
    Description string
    Category    string
}
```

That is a strict subset of what n8n exposes. The gaps are listed in §6.

---

## 2. n8n's Node Details View (NDV)

When you click a node on the n8n canvas, a **right-side panel** slides in
with two top-level areas:

### 2.1 Settings tab (left)

- **Parameters** — the form rendered from `INodeTypeDescription.properties`.
- **Settings** — node-instance flags (e.g. "Always Output Data", "Execute
  Once", "Retry on Fail", "On Error").
- **Credential picker** — single-line row with a Private badge, Not
  Connected / Connected indicator, and a Connect dropdown offering Modify /
  Disconnect.

### 2.2 Output tabs (right)

Three sub-tabs, always present:

| Tab           | What it shows                                                                                                    |
| ------------- | ---------------------------------------------------------------------------------------------------------------- |
| **Input**     | The `INodeExecutionData[]` that the previous node(s) passed into this one.                                       |
| **Output**    | The `INodeExecutionData[]` this node produced.                                                                   |
| **Parameters**| The evaluated `parameters` object that the node ran with (after expression substitution).                        |

Each of Input and Output supports three display modes:

| Mode      | Layout                                                                                                                  |
| --------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Table** | Rows = items; columns = JSON keys. Nested objects are flattened or shown as JSON cells. Resizable, virtualised.         |
| **JSON**  | Raw `INodeExecutionData[]` with syntax highlighting, copy button, lazy expansion.                                       |
| **Schema**| A virtualised tree of field names and inferred types (`string`, `number`, `boolean`, `array`, `object`, `date`, `binary`). Type comes from a JSON sample. |

A fourth **Binary** tab appears when an item has a `binary` property —
shows file previews / downloads.

### 2.3 Why this matters for debugging

n8n's debugging workflow is:

1. Open the failing execution.
2. Click the failing node. The panel shows **Parameters** — what was it
   told to do?
3. Switch to **Input** in JSON mode to see what it received.
4. Switch to **Table** mode to scan a 1000-row dataset.
5. Switch to **Schema** mode to understand the data shape.

m9m's current implementation (`web/src/views/ExecutionDetail.vue`) only
shows the raw JSON dump. That is enough for tiny flows but unusable for
real workloads.

---

## 3. Per-node-type parameter surfaces (the part the user wants)

Each n8n node type ships its own `properties` array. The UI renders
different field widgets for each: dropdowns, multi-selects, code
editors, collection editors, resource locators, etc.

Below is the canonical n8n parameter surface for the core nodes m9m
already ships. This is what the Settings panel of the NDV (and by
extension the Parameters tab in the execution view) must surface.

### 3.1 HTTP Request (`n8n-nodes-base.httpRequest`)

Source: <https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.httprequest/>

**Parameters (main tab)**

- `method` — `string` enum: `GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS`
- `url` — `string`, supports expressions
- `authentication` — `options`: `none | predefinedCredentialType | genericCredentialType`
- `nodeCredentialType` — when `predefinedCredentialType`, picks one of the
  known credentials registered for this node
- `genericAuthType` — when `genericCredentialType`: `basicAuth | headerAuth |
  queryAuth | digestAuth | oauth1Api | oauth2Api | customAuth | simplifiedCustomAuth`

**Parameters (optional, behind "Add Option")**

- `sendQuery` → `queryParameters` (`collection` of `name, value` OR `json`)
- `sendHeaders` → `headerParameters` (same shape)
- `sendBody` → `body` (`form-urlencoded | multipart-form-data | json | n8nBinary | raw`)
- `arrayFormatInQueryParameters` — `No Brackets | Brackets Only | Brackets with Indices`
- `batching` → `itemsPerBatch`, `batchInterval`
- `ignoreSSLIssues` — `boolean`
- `redirects` → `redirectOptions.maxRedirects`
- `response` → `response.responseFormat` (`autodetect | file | json | text`),
  `response.includeResponseHeadersAndStatus`, `response.neverError`
- `pagination` → `pagination.paginationMode` (`off | updateAParameterInEachRequest |
  responseContainsNextURL`), `pagination.nextURLExpression`
- `proxy` — `string` (overrides HTTP_PROXY env)
- `timeout` — `number` ms

**Settings tab**

- `notes`, `notesInFlow`, `retryOnFail`, `maxTries`, `waitBetweenTries`,
  `alwaysOutputData`, `continueOnFail`, `onError` (`stopWorkflow | continueRegularOutput | continueErrorOutput`)

### 3.2 Set (`n8n-nodes-base.set`)

- `mode` — `manual | raw`
- `duplicateItem` — `boolean`
- `assignments.assignments` — `collection` of `name, type, value`
  - `type` is one of `string, number, boolean, array, object, dateTime`
  - `value` is an expression editor
- `options` — `stripBinary`, `dotNotation` (legacy)

### 3.3 IF (`n8n-nodes-base.if`)

- `conditions.combinator` — `and | or`
- `conditions.conditions` — `collection` of:
  - `leftValue`, `rightValue` (expressions)
  - `operator` — 40+ enum (`equals, notEquals, contains, greaterThan, ...`)
- `options` — `caseSensitive`, `typeValidation`

IF has **two output branches** (`main: [[trueItems], [falseItems]]`). The
execution UI must render both branches in the graph.

### 3.4 Switch (`n8n-nodes-base.switch`)

- `rules.values` — `collection` of `conditions` (same shape as IF) plus
  `renameOutput`, `outputKey`
- `options` — `fallbackOutput` (single output index when no rule matches)

Switch has **N output branches**, named by `outputKey`.

### 3.5 Code (`n8n-nodes-base.code`)

- `language` — `javaScript | python` (Python only in beta at time of
  writing)
- `mode` — `runOnceForEachItem | runOnceForAllItems`
- `sourceCode` — large multi-line string, editor with autocomplete

### 3.6 Webhook (`n8n-nodes-base.webhook`)

- `httpMethod` — `GET, POST, PUT, PATCH, DELETE, HEAD`
- `path` — `string` (URL path)
- `authentication` — same options as HTTP Request
- `responseMode` — `onReceived | immediately | lastNode | responseNode`
- `responseData` — when `responseMode=onReceived`: `allEntries | firstEntry | noData`
- `options` — `binaryData`, `rawBody`, `ipWhitelist`, `noResponseBody`

### 3.7 Postgres / MySQL / SQLite (`n8n-nodes-base.postgres|mySql|sqlite`)

- `operation` — `executeQuery | insert | update | delete | select`
- `query` / `queryParameters` / `additionalFields`
- `options` — `connectionTimeout`, `queryTimeout`

### 3.8 Cron / Schedule (`n8n-nodes-base.cron` / `scheduleTrigger`)

- `rule` — `cron` (cron expression) **or** `interval` (`mode: minutes|hours|days|weeks|months|custom`,
  list of field values)

### 3.9 Filter (`n8n-nodes-base.filter`)

- `conditions` — same as IF

### 3.10 Merge (`n8n-nodes-base.merge`)

- `mode` — `append | combine | chooseBranch | multiply`
- `combinationMode` — `mergeByPosition | mergeByFields`, `joinMode`,
  `preferLeft`

### 3.11 Wait (`n8n-nodes-base.wait`)

- `resume` — `afterTime | afterTimeFromLastItem | afterWebhook | atTime`
- `amount`, `unit` for time waits; `webhookSuffix` for webhook waits

### 3.12 Loop / Split In Batches (`splitInBatches`)

- `batchSize` — `number`
- `options` — `reset`

---

## 4. Per-node-type Input/Output rendering

The user asked for "input and output follow the node type". This means
different node types surface their I/O differently in the NDV. The
following table is what n8n actually does:

| Node type            | Input view                                | Output view                             |
| -------------------- | ----------------------------------------- | --------------------------------------- |
| HTTP Request         | (same)                                    | **flatten `json` field** (raw response body). If `responseFormat=json`, parse and show as table. If `file`, show **Binary** tab. If `includeResponseHeadersAndStatus=true`, show `headers` / `statusCode` as separate top-level keys. |
| Set                  | (same)                                    | Diff vs input — fields added/removed/changed are highlighted. |
| IF                   | (same)                                    | Two streams: **true** (main[0]) and **false** (main[1]). NDV shows a tab strip at the top to switch. |
| Switch               | (same)                                    | N streams; same tab strip pattern. |
| Code                 | (same)                                    | New items the code returned. If `runOnceForEachItem`, every row is independently produced. |
| Postgres / MySQL    | (same)                                    | Query result rows → table. For non-SELECT ops, single row with `affectedRows` or empty. |
| Cron                 | **No input.** The NDV shows "This node has no input". | (one item, `{"timestamp": "...", "cron": "..."}`) |
| Webhook              | (one item, the request body / headers)    | The configured `responseMode`'s output. |
| Filter               | (same)                                    | Filtered subset of input items.          |
| Merge                | **Two inputs.** NDV shows a tab strip.    | Merged stream per `mode`.                |

The pattern is:

1. **Inspect the node's `INodeTypeDescription`** to learn:
   - how many input slots it has (`inputs: [{type:"main", displayName:"..."}]`)
   - whether the output is multi-branch (IF/Switch/Merge)
   - whether it has any **special output rendering** (e.g. HTTP Request's
     `responseFormat`).
2. **Render the appropriate view per branch** in the right panel.
3. **Annotate the items** with type-aware extras (status codes for HTTP,
   query plan for SQL, etc.).

---

## 5. How m9m needs to change

These are the concrete implementation gaps. None of them is a
research question; each maps to a file change.

### 5.1 Persist per-node I/O during execution — **BLOCKING**

`internal/model/workflow.go:188` declares
`NodeData map[string][]DataItem`. The struct is there, but **the engine
never populates it** while running a workflow. Today an execution stores
only the workflow-level `data`.

**Fix**: in `internal/engine/...` (the workflow runner), after each node
finishes, write
`execution.NodeData[node.Name] = output` before moving on. This must be
done for **both** successful and failed nodes, capturing the failure
under `data[i].json.error` if the engine currently throws.

**Effort**: ~1 day. Touch engine + storage layer.

### 5.2 Expose `INodeTypeDescription`-equivalent properties — **BLOCKING for parity**

`internal/nodes/base/node.go`'s `NodeDescription` has only
`Name, DisplayName, Description, Category`. The
`GET /api/v1/node-types` endpoint therefore returns no `properties`
array, so the Settings panel in the NDV has no schema to render.

**Fix**: extend `base.NodeDescription` with a `Properties []NodeProperty`
field (mirroring n8n's `INodeProperties`:
`displayName, name, type, default, options, displayOptions, required, typeOptions`)
and populate it in each `NewXxxNode()` factory. Then surface it from the
node-types API and from a new `GET /api/v1/node-types/{name}` endpoint.

**Effort**: ~1 week (100+ nodes need their `properties` filled in). The
most-used nodes (HTTP, Set, IF, Switch, Code, Webhook, Cron) should be
done first.

### 5.3 Rebuild `ExecutionDetail.vue` to match n8n's NDV

The current implementation (`web/src/views/ExecutionDetail.vue`) has:

- ✅ Per-node state colouring on the canvas
- ✅ Right panel with state badge, error, output, parameters, credentials
- ❌ No tabbed input/output/parameters layout
- ❌ No Schema/Table/JSON mode switcher
- ❌ Per-node-type parameter rendering (just dumps raw `parameters`)
- ❌ Multi-branch output rendering (IF/Switch/Merge show no tab strip)
- ❌ The "execution step nodes are not connected" reported by the user is
  actually a **layout bug** in the right-pane header, not the canvas (the
  edges between canvas nodes ARE drawn — see `ExecutionDetail.vue:340-350`
  `<VueFlow :edges="flowEdges">`). Confirm with screenshot in §7.

**Fix plan**:

1. Replace the current `Output`/`Parameters` collapsible sections with
   three tabs at the top of the right panel: **Input**, **Output**,
   **Settings (Parameters)**. Default to **Output**.
2. Inside Input and Output tabs, add a sub-toolbar with three buttons:
   **Schema · Table · JSON**. Persist the chosen mode per tab in
   `localStorage`.
3. Implement the three renderers (described in §6 below).
4. Render parameters using the schema from §5.2: each known property
   renders as a labelled row (`HTTP Request → method = POST`). Unknown
   properties fall back to a "Raw" section showing the original JSON.
5. For nodes with multiple outputs (IF / Switch / Merge), add a
   `<select>` that picks the branch (true/false, branch-1/2/3, in1/in2).
   The Table/Schema/JSON views must respect the selected branch — pull
   from `nodeData[nodeName]` whose key matches the branch.
6. For nodes with binary output (HTTP Request `responseFormat=file`),
   show a fourth **Binary** tab listing each `data[i].binary.<key>`
   with name / mime / size / download button.

### 5.4 Render Schema / Table / JSON views

Three pure-functional Vue composables:

- `useSchemaView(items: DataItem[])` — builds the nested type tree by
  walking one item. Use the same algorithm as n8n's `VirtualSchema.vue`
  (described in <https://deepwiki.com/n8n-io/n8n/6.3-parameter-input-and-expression-editor>):
  recursively classify each leaf as `string | number | boolean | array | object | date | binary`.
  Render as a tree with expand/collapse. Use `vue-virtual-scroller` for
  large trees.
- `useTableView(items: DataItem[])` — builds the column set by unioning
  the top-level keys of every item, then renders rows × columns. Nested
  objects render as compact JSON strings in the cell. Use
  `vue-virtual-scroller` for >100 rows.
- `useJsonView(items: DataItem[])` — pretty-prints with
  `JSON.stringify(items, null, 2)` and a Copy button. Existing
  `prettyJson()` already does this; just needs to be wrapped in a tab.

### 5.5 Fix the canvas state colouring (it already works)

The user reported "execution step nodes also not connected". The
**edges are drawn** (verified live on 2026-09-09 against the deployed
build at `http://187.77.113.218:8080`) — the issue is that
`buildFlowEdges` produces edges with `sourceHandle`/`targetHandle` keys
of `"output-N"` / `"input-N"`, but the Vue Flow nodes are created with
`Handle id="output-0"` / `id="input-0"`. If the edge id mismatches, the
edge is drawn but its anchor points are at the node centre instead of
the handle, which looks like "no connection".

**Fix** (the same shape as in `WorkflowCanvas.vue`'s `onConnect`):
match each edge's `sourceHandle` / `targetHandle` against the node's
handle id, and fall back to `0` if absent. Verify in the browser.

### 5.6 Confirm the canvas-not-connected problem in the editor

The user also reported "the nodes are still not connected" in the
editor. The current `WorkflowCanvas.vue` changes:

- `:connection-radius="120"` — generous drop radius
- 28×28 px visual handles with a 72×72 px invisible hit-area
- `:fit-view-on-init-options="{ minZoom: 0.6, maxZoom: 1 }"` — keeps
  handles at usable size
- Connection preview line in indigo (`#6366f1`)

This was **verified live on 2026-09-09**: the editor at
`http://187.77.113.218:8080/workflows/workflow_1788515398385712912_4`
shows two nodes with a single connecting edge, and handles are 26 px
on screen at 0.93× zoom. If the user is still seeing a problem, it's
either:

- A stale frontend bundle in their browser (hard reload with Ctrl+Shift+R)
- A new build that was made before this fix landed

---

## 6. Gap matrix vs n8n (today vs target)

| Feature                                        | n8n         | m9m today                              | Target m9m                                              |
| ---------------------------------------------- | ----------- | -------------------------------------- | ------------------------------------------------------- |
| `INodeTypeDescription.properties` per node     | ✅ 100%     | ❌ absent                              | ✅ all ~100 core nodes populated                         |
| Per-node Input/Output/Settings tabs            | ✅          | ❌ collapsible sections                | ✅ three top-level tabs                                  |
| Schema / Table / JSON switcher                 | ✅          | ❌ JSON only                           | ✅ all three modes                                       |
| Multi-branch output (IF / Switch / Merge)      | ✅ tab strip| ❌ hidden                              | ✅ branch picker + per-branch render                     |
| Binary data preview                            | ✅ tab      | ❌ hidden                              | ✅ Binary tab when `item.binary` is set                  |
| Pinned / mocked data                           | ✅          | ❌ not implemented                     | phase 2 (out of scope for current ask)                  |
| Expression-aware parameter rendering           | ✅ CodeMirror | plain text inputs                   | ⚠️ partial — Settings panel only                       |
| Per-node credentials section                   | ✅          | ✅ already in right panel              | ✅ keep, but render next to NDV credential row          |
| Per-node input captured at runtime             | ✅          | ❌ engine doesn't capture              | ✅ populate `execution.NodeData`                         |
| Per-node output captured at runtime            | ✅          | ⚠️ partial — only `data` (workflow-level) | ✅ per-node snapshot under `nodeData[nodeName]`       |
| Cascade-skipped nodes                          | ✅          | ✅ already implemented                 | ✅ keep                                                  |

---

## 7. Live verification (2026-09-09)

Confirmed against `http://187.77.113.218:8080`:

- The latest build ships **new asset hashes** (WorkflowEditor `jwZM75eE`,
  ExecutionDetail `sX8J2Xrs`, PerformanceView `DpkTbDOJ`, `_plugin-vue_export-helper`
  files all present).
- Editor canvas (`/workflows/workflow_1788515398385712912_4`) shows
  2 nodes connected by 1 edge, handles at 26 px on screen.
- Execution canvas (`/executions/exec_1788963329048735538`) shows
  5 nodes connected by 4 edges (WebHook → Set → postgres → telegram → telegram2).

Screenshot evidence stored in:
`tmp/validation-2026-09-09/editor-connected.png`
`tmp/validation-2026-09-09/execution-connected.png`

---

## 8. Implementation phases (proposed)

| Phase | Title                                          | Outcome                                                                 |
| ----- | ---------------------------------------------- | ----------------------------------------------------------------------- |
| 7a    | Capture per-node I/O in the engine             | `execution.NodeData` populated for every node; survives retries         |
| 7b    | Extend `base.NodeDescription` + API            | `/api/v1/node-types` and `/api/v1/node-types/{name}` return `properties`|
| 7c    | Populate `properties` for top-10 core nodes    | HTTP, Set, IF, Switch, Code, Webhook, Cron, Filter, Merge, Wait, Postgres, MySQL, SQLite |
| 7d    | Rebuild `ExecutionDetail.vue` with tabs        | Input / Output / Settings tabs + Schema / Table / JSON modes            |
| 7e    | Multi-branch renderers (IF/Switch/Merge)       | Branch picker, per-branch render                                        |
| 7f    | Binary tab                                     | HTTP Request `file` mode, etc.                                          |
| 7g    | Pin data                                       | Local dev tool — out of scope unless explicitly asked                  |

Each phase is independently shippable and worth its own PR.

---

## 9. Source references

- n8n Core Nodes index — <https://docs.n8n.io/integrations/builtin/core-nodes/>
- HTTP Request — <https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.httprequest/>
- DeepWiki: Execution Data and History — <https://deepwiki.com/n8n-io/n8n-docs/9.3-execution-data-and-history>
- DeepWiki: Parameter Input and Expression Editor (NDV internals) — <https://deepwiki.com/n8n-io/n8n/6.3-parameter-input-and-expression-editor>
- DeepWiki: Workflow Editor UI — <https://deepwiki.com/gwolf999/n8n/3.1-workflow-editor-ui>
- n8n Debugging guide — <https://n8nautomation.cloud/blog/n8n-workflow-debugging-complete-guide-2026>
- n8n source: `RunData.vue` — <https://github.com/n8n-io/n8n/blob/3f7258b1/packages/frontend/editor-ui/src/features/ndv/runData/components/RunData.vue>
