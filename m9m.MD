# m9m — PRD (Product Requirements Document)

**Versi:** 1.2 — 2026-10-06 — implementasi P0 + P1 selesai (credential live, redis native, kafka, plugin JS, NodeData, Properties)
**Source:** `README.md`, `docs/*`, `n8n-nodes.MD`, `documentation/docs/integrations/n8n-sync.md`, `cmd/m9m/commands/exec.go` (45 executors), `internal/webhooks/*`, `internal/plugins/*`, `internal/nodes/messaging/kafka.go`, `internal/nodes/trigger/kafka.go`
**Tagline:** The n8n alternative without the bugs — faster, more reliable workflow automation.

---

## 1. Capability — Inventory Lengkap

### 1.1 Core Engine
- Workflow JSON n8n unchanged. DAG, `connections.main`, parallel, conditional branch.
- Engine: `internal/engine` — `WorkflowEngine`, `ExecuteWorkflowWithContext`, `RunAwareNodeExecutor`, `EdgesTaken`, `NodeOutputs`.
- Expression: `{{ $json }}`, `{{ $node["name"].data }}`, `$(NodeName)`, `runIndex`, 60+ funcs, Goja sandbox.
- Triggers: webhook prod/test, cron/schedule, errorTrigger, executeWorkflowTrigger, start, wait.
- Mode: `manual`, `trigger`, `test`. `WithExecutionMode`, `WithExecutionID`.

### 1.2 Node Catalog — 42 Executors (`RegisterAllNodes`)

| Kategori | Type ID | Status |
|---|---|---|
| Transform | `set`, `filter`, `code`, `merge`, `splitInBatches`, `itemLists`, `switch`, `function`, `json`, `xml`, `if`, `loop` | Done |
| HTTP | `httpRequest` | Done |
| Trigger | `webhook`, `respondToWebhook`, `errorTrigger`, `executeWorkflowTrigger` | Done |
| Timer | `cron` | Done |
| Code | `executeCommand`, `pythonCode` | Done |
| File | `readBinaryFile`, `writeBinaryFile` | Done |
| Database | `postgres`, `mySql`, `sqlite`, `elasticsearch`, `redis`, `mongoDb` | Done (redis HTTP proxy — §3) |
| Messaging | `slack`, `discord`, `twilio`, `microsoftTeams` | Done |
| AI | `@n8n/n8n-nodes-langchain.openAi`, `anthropic` | Done |
| Core | `start`, `noOp`, `wait`, `executeWorkflow` | Done |
| Cloud | `awsLambda`, `s3`, `azureBlobStorage`, `gcpCloudStorage` | Done S3/Lambda/Blob/GCS only |
| VCS | `github`, `gitlab` | Done |
| Productivity | `googleSheets`, `notion`, `stripe` | Done |
| Email | `sendEmail`, `sendGrid` | Done |

Gap builtin vs n8n 400+ nodes: Salesforce, HubSpot, Shopify, Jira, Supabase, Airtable, WhatsApp, Dropbox/Drive, Kafka, Redis native, dll — belum. Community `n8n-nodes-*` (npm) belum — lihat §1.9 + §4.5.

### 1.3 Webhook Surface — Parity 19/19 (2026-09-10)
- Routes `/webhook/{path:.*}`, `/webhook-test/{path:.*}`, `/api/v1/webhooks*`.
- Auth `none`/`basic`/`apiKey`/`header`/`jwt` (RS256/HS256, 403 `jwt expired`/`jwt not yet valid`).
- ResponseMode `onReceived` async `{"message":"Workflow was started"}`, `lastNode`, `responseNode` BFS `respondWith`.
- ResponseData `firstEntryJson`/`allEntries`/`noData`. Content-Type `options.responseHeaders.entries` > inbound XML/text mirror > `application/json`.
- Error wire `{code,message,hint}` 404/401/422. OTel `otelExtract` traceparent.

### 1.4 API & Storage
- REST n8n-compat: `/api/v1/workflows`, `/credentials`, `/executions`, `/node-types`, `/webhooks`, `/schedules`.
- Storage MySQL/Postgres/SQLite/memory. `node_data`, `edges_taken`. Debug flag per-workflow, retry `POST /executions/:id/retry-node`.

### 1.5 Sync Bridge (n8n↔m9m) — `documentation/docs/integrations/n8n-sync.md`
- FastAPI `:8001` di `m9m-backend` via `supervisord`, poll `GET /api/v1/workflows` (`X-N8N-API-KEY`) tiap `POLL_INTERVAL_SECONDS=30`, hash dedupe, `typeVersion` float→int, active-filter.
- Credential decrypt direct Postgres `credentials_entity.data` AES-256-CBC `Salted__`+EVP_BytesToKey, `N8N_ENCRYPTION_KEY`/`GEN_ENCRYPTION_KEY`, re-encrypt via `POST/PUT /api/v1/credentials`.
- Endpoints: `GET /` banner, `GET /health` `{status:ok}`, `GET /status` `{last_cycle_at, source, count, is_running}`, `POST /sync` cycle sync `{results[], started_at, duration_ms}` (502 jika ada failed).
- Env: `N8N_BASE_URL`, `N8N_API_KEY`, `M9M_BASE_URL`, `N8N_ENCRYPTION_KEY`, `N8N_PG_*`, `SYNC_ONLY_ACTIVE=true`, `SYNC_PORT=8001`.
- Disable: comment `supervisord` block atau `POLL_INTERVAL_SECONDS=0`. Workflow community node tanpa executor → sync OK tapi exec fail graceful (butuh §4.5).

### 1.6 AI Assistant (in-app)
- Settings→AI: OpenAI/Anthropic/MiniMax/Ollama, baseURL, key, model, maxTokens, temp. Mutex-guarded, shared workflow nodes.

### 1.7 Telemetry
- Prometheus `:9090/metrics` — `m9m_workflow_*`, `m9m_node_*`, `m9m_queue_*`, `m9m_webhook_*`, `m9m_ai_tokens_total`.
- Tracing OTLP gRPC+HTTP 3-layer: UI toggle > DB `otel_config` > env `OTEL_*`/`M9M_OTEL_*`. Span `workflow.execute`→`node.execute`→`agent.generate`.
- Health `/healthz`, `/readyz`.

### 1.8 Ecosystem
- CLI `m9m exec|serve|node list|benchmark|demo`, MCP 37 tools, Go/Node/Python SDK, Docker `harbor.solutiontech.id/m9m/m9m-core:latest`, K8s Helm.

### 1.9 Community Nodes `n8n-nodes-*` — Status & Gap (`n8n-nodes.MD` + `docs/PLUGIN_ARCHITECTURE.md`)
- **Hari ini:** 0 community nodes. `RegisterAllNodes` static compile — tambah node butuh recompile+redeploy. `n8n-nodes-*` npm tidak jalan.
- **Gap NDV/execution:** `base.NodeDescription` hanya `Name/DisplayName/Description/Category` — tanpa `Properties []NodeProperty` (`INodeTypeDescription.properties`). Akibat: `GET /api/v1/node-types` tanpa schema, Settings panel dump raw JSON, tanpa Schema/Table/JSON switcher, tanpa multi-branch tab (IF/Switch/Merge), tanpa Binary tab, `execution.NodeData` tidak populate per-node (hanya workflow-level `data`).
- **Infra siap:** Goja JS runtime (`internal/runtime/javascript_runtime.go`), Python runtime, expression evaluator 60+ funcs — bisa jadi plugin runtime tanpa dependensi baru.
- **Opsi plugin (evaluated):** 1) JS Goja ⭐ recommended LOW 1-2d, 2) Go `.so` plugin (fragile, no Windows), 3) gRPC external (HIGH), 4) WASM (HIGH cutting-edge), 5) REST HTTP node (LOW), 6) Hybrid JS+Go+REST ⭐ best balance. Target Phase 1 = JS Goja.

---

## 2. What Has Been Done

- **2026-09 cycle:** 19/19 webhook parity, engine semantics `$(NodeName)`/`RunAwareNodeExecutor`/splitInBatches/Binary, webhook manager `IsAsyncResponseMode`/`ExecuteWebhookAsync`/`recordWorkflowExecution` NDV Input/Output/Settings+`EdgesTaken`.
- Credential bridge sync Postgres, telemetry 3-layer live toggle, AI Settings wiring, per-workflow debug+retry, Vue `dist` embed, Harbor pipeline.
- **Docs ready:** `n8n-nodes.MD` canonical per-node-type parameter surfaces (HTTP Request/Set/IF/Switch/Code/Webhook/Postgres/Cron/Filter/Merge/Wait/Loop) + gap matrix + phases 7a-7g. `PLUGIN_ARCHITECTURE.md` 6 opsi plugin + loader/registry/wrapper spec + hot-reload + sandbox limits. `n8n-sync.md` bridge spec lengkap.
- **2026-10-06 P0+P1 implementasi:**
  - **B-01+B-02 (Credential live fix + bypass)**: `Webhook.CredentialID` field + `liveAuthData()` re-read `workflowStorage.GetCredential` per request. `handler.go:authenticateRequest` hard-fail 401 jika `CredentialID` set tapi live nil (anti-bypass). Legacy `CredentialID==""` masih preserve fallback.
  - **B-03 (Redis native)**: `internal/nodes/database/redis.go` refactor dual-mode `redisBackend` — `nativeBackend` (TCP/RESP, AUTH+SELECT, reconnect, RESP3 simple/bulk/int/array/error/nil) + `httpBackend` (REST gateway). 12 ops (get/set/delete/keys/hget/hset/lpush/rpush/lrange/incr/expire/publish). Default `mode=native` no-proxy required.
  - **B-04 (Kafka)**: `internal/nodes/messaging/kafka.go` producer + `internal/nodes/trigger/kafka.go` consumer via Confluent REST Proxy (`POST /topics/{topic}/produce`, `/consumers/{group}`). Auth basic/apiKey/none. Properties `restProxyUrl/topic/key/value/headers/auth`. `materialiseValue` evaluate `={{ $json.x }}`.
  - **B-08 (NodeData capture)**: `internal/engine/execution_node_data.go` `BuildExecutionNodeData(workflow, result)` → start+last (Debug=OFF) atau all (Debug=ON) → `model.WorkflowExecution.NodeData`. `result.NodeOutputs` populated per node di engine loop. `recordWorkflowExecution` callers wired.
  - **B-09 (Properties top-10)**: `internal/nodes/base/{node,properties}.go` `Properties []NodeProperty` (`displayName/name/type/default/options/required/typeOptions/displayOptions/placeholder`). Populated di HTTP/Set/IF/Switch/Code/Webhook/Cron/Filter/Merge/Wait/Postgres/MySQL/SQLite/Redis. `GET /api/v1/node-types` return schema → NDV render ready.
  - **B-10 (JS plugin loader)**: `internal/plugins/loader.go` (Goja VM, eval/Function disabled, 30s interrupt, `module.exports` shim), `registry.go` (LoadDir, RegisterAll, List, Reload, multi-error), `wrapper.go` (NodeExecutor adapter, Goja VM serialized via mutex, JS→DataItem conversion incl binary/pairedItem). Flag `--plugin-dir` di `cmd/m9m/commands/serve.go` + `M9M_PLUGIN_DIR` env. Default nodeType `n8n-nodes-base.<basename>`.
  - **45 executors** (was 42): +`kafka` +`kafkaTrigger` +plugin community.

---

## 3. Temuan Saat Ini

| # | Temuan | Severity | Dampak |
|---|---|---|---|
| T-01 | Credential per-node stale snapshot — `webhook.AuthData` snapshot → edit Credentials tidak honoured. Fix WIP `Webhook.CredentialID`+`liveAuthData()` | P0 Bug | Rotate auth gagal |
| T-02 | Redis bukan native driver — `RedisHTTPClient` HTTP gateway, bukan `go-redis` | P1 Gap | Tanpa proxy fail |
| T-03 | Kafka node missing total — `kafka`+`kafkaTrigger` tidak ada | P1 Gap | Event streaming blocked |
| T-04 | Auth fallback bypass — credential miss → `well-formed header` PASS | P1 Security | Bypass |
| T-05 | `apiKey` map `httpHeaderAuth` hack | P2 | Drift |
| T-06 | `readRespondToWebhookHeaders` lower-case lossy | P3 | Minor |
| T-07 | `N8N_FEATURE_COMPARISON.md` stale 2024-09-22 | P2 | Mislead |
| **T-08** | **`NodeDescription` tanpa `Properties`** — tanpa `INodeTypeDescription` parity, NDV tidak bisa render per-node form, Schema/Table/JSON switcher, branch picker. `ExecutionDetail.vue` hanya raw JSON dump | **P0 Gap** | Parity UX n8n gagal, `n8n-nodes.MD` §5.2 blocking |
| **T-09** | **`execution.NodeData` tidak populate** — engine tidak isi per-node I/O, NDV Input/Output kosong walau `model.Workflow.NodeData` ada | **P0 Bug** | Debug unusable `n8n-nodes.MD` §5.1 blocking |
| **T-10** | **Community `n8n-nodes-*` tidak support** — workflow import dengan `n8n-nodes-base.*` langka OK, tapi `n8n-nodes-xxx.*` community fail `failed to get executor` | P1 Gap | Migrasi enterprise blocked |

Detail T-01/T-04 → `bug-fix.MD`. T-08/T-09/T-10 → §4.5-4.6.

---

## 4. Kebutuhan Capability Baru

### 4.1 Node Kafka (Baru)
- Executor `n8n-nodes-base.kafka`+`n8n-nodes-base.kafkaTrigger`. Ops `send`/`sendAndWait`/`consume`. Params `topic`/`key`/`value`/`headers`/`acks`/`partition`/`clientId`/`groupId`/`fromBeginning`/`sasl`/`ssl`. Lib `github.com/segmentio/kafka-go` (pure Go). Credential `kafka`. Trigger emit `{topic,partition,offset,key,value,headers,timestamp}`. Register + compose `kafka/kraft` dev. Acceptance `m9m exec` publish→`kcat` verify, `m9m node list` tampil.

### 4.2 Node Redis — Upgrade Native Driver
- Refactor `redis.go`: `go-redis/v9` + keep `RedisHTTPClient` fallback jika `baseUrl` http. Params `host`/`port`/`database`/`password` via cred `redis`/`tls`. 11 ops existing + `publish` optional → map `go-redis`. Backward compat http. Test `miniredis` 11 ops PASS, compose `redis:6379` roundtrip.

### 4.3 Credentials Bisa Save di Node (Fix T-01)
- Persist `PUT /api/v1/workflows/{id}` `credentials:{type:{id,name}}` sudah. Wiring `CredentialID`+`liveAuthData()` re-read `GetCredential` tiap request (WIP merge). Lazy invalidate cukup (eager iterate `activeHooks` opsional). Migrasi backfill `CredentialID` ListWebhooks. Test rotation old 401 new 200 tanpa re-save.

### 4.4 Non-Functional Kafka/Redis
- Metrics `m9m_node_duration_seconds`, `CreateError` wrap, TLS/SASL, constant-time compare.

### 4.5 Support `n8n-nodes-*` Community Nodes (Baru — `n8n-nodes.MD` §5-§8 + `PLUGIN_ARCHITECTURE.md`)

**Goal:** Workflow yang pakai `n8n-nodes-*` community (400+ npm) bisa import+exec tanpa recompile m9m binary. Parity NDV `n8n-nodes.MD` §2-§4.

**Scope V1 — JS Goja Plugin (90% use-case, LOW 1-2 minggu):**

**4.5.1 Plugin Loader (`internal/plugins/loader.go`)**
- `LoadJavaScriptPlugin(filePath)` → `goja.New()` run `module.exports = {description, execute, validateParameters, permissions}`. Disable `eval`/`Function`, timeout `vm.Interrupt` 30s.
- Params `description` → `base.NodeDescription` + `Properties []NodeProperty` (mirror `INodeProperties`: `displayName,name,type,default,options,displayOptions,required,typeOptions`).
- `permissions: {network, filesystem, env}` gate.

**4.5.2 Registry (`internal/plugins/registry.go`)**
- `LoadPluginsFromDirectory(dir)` `filepath.Glob("*.js")`, `RegisterWithEngine(engine)` wrapper. Flag `--plugin-dir ./plugins` di `main.go`.
- Hot reload `fsnotify` watch write → `ReloadPlugin`. Hitung `Loaded N plugins` log. Backoff jika fail (jangan crash engine).

**4.5.3 Wrapper (`internal/plugins/wrapper.go`)**
- `JavaScriptNodeWrapper.Execute(inputData, params)` call JS `execute`, convert `[]DataItem`. `Description()` return plugin description.

**4.5.4 NodeDescription Parity (Blocking `n8n-nodes.MD` §5.2)**
- Extend `internal/nodes/base/node.go`: `Properties []NodeProperty` + `Inputs/Outputs` meta. Populate di `NewXxxNode()` untuk top-10 dulu: HTTP, Set, IF, Switch, Code, Webhook, Cron, Filter, Merge, Wait, Postgres/MySQL/SQLite. API `GET /api/v1/node-types` + `GET /api/v1/node-types/{name}` return `properties`. Plugin JS `description.properties` langsung jadi sumber — tidak perlu Go rebuild.

**4.5.5 Per-Node I/O Capture (Blocking §5.1)**
- Engine after each node `execution.NodeData[node.Name]=output` (sukses+gagal). Storage persist `NodeData` sama seperti webhook `recordWorkflowExecution` (sudah ada `BuildExecutionNodeData` tapi belum wired di engine utama — wire sekarang). Sync bridge workflow yang sync dari n8n tetap dapat NodeData setelah exec pertama di m9m.

**4.5.6 ExecutionDetail NDV Rebuild (§5.3-5.5) — Frontend `web/src/views/ExecutionDetail.vue`**
- Ganti collapsible Output/Parameters → 3 tabs `Input | Output | Settings(Parameters)` default Output. Sub-toolbar `Schema · Table · JSON` persist `localStorage`. Composables `useSchemaView`/`useTableView`/`useJsonView` (`vue-virtual-scroller` >100 rows). Render params via `properties` schema (known label, unknown fallback Raw JSON). Multi-branch picker (IF 2, Switch N, Merge 2) pull `nodeData[branch]`. Binary 4th tab jika `item.binary`. Edge `sourceHandle/targetHandle` `output-N`/`input-N` match handle id (sudah fix di `WorkflowCanvas.vue` — verifikasi live).

**4.5.7 Sync Bridge untuk Community Nodes**
- Workflow dengan `type: n8n-nodes-xxx.foo` sync OK via `sync.py` (hash+typeVersion). Jika executor belum install, `Engine.ExecuteWorkflow` return error `unknown node type: n8n-nodes-xxx.foo — install plugin ke --plugin-dir` (jangan panic). `POST /sync` result `failed` dengan hint `place n8n-nodes-xxx-foo.js in plugins/`. Tidak perlu decrypt tambahan.

**4.5.8 Opsi Lanjut (Phase 2-3, `PLUGIN_ARCHITECTURE.md`)**
- Go `.so` plugin (MEDIUM, Linux/macOS only, ABI fragile) — hanya untuk CPU-intensive (image/crypto).
- gRPC external (HIGH, language-agnostic, isolasi) — enterprise.
- WASM (HIGH cutting-edge).
- REST API node (LOW) — microservice existing.
- Hybrid final: JS default + Go native + REST.

**Acceptance V1:**
- `cp plugins/my-custom-node.js plugins/` + `m9m serve --plugin-dir ./plugins` → `m9m node list` tampil custom node, `m9m exec workflow-custom.json` PASS tanpa recompile.
- HTTP/Set/IF/Switch/Code/Webhook top-10 `GET /api/v1/node-types/{name}` return `properties[]` non-empty.
- `exec_xxx` `nodeData` terisi per-node, NDV tabs Input/Output/Settings + Schema/Table/JSON + branch picker work live (verified screenshot).
- Community workflow `n8n-nodes-telegram` port sebagai JS plugin → exec PASS.
- Overhead JS <2× native, load <100ms, hot reload <1s (`PLUGIN_ARCHITECTURE.md` success metrics).

### 4.6 NDV Polish & Community Marketplace (Phase 2 — Out of V1)
- Pinned/mocked data, Expression CodeMirror, marketplace `m9m plugin install n8n-nodes-xxx`, `registry` search. Effort 1-2 minggu setelah V1.

---

## 5. Roadmap Singkat

| Phase | Item | ETA | Ref |
|---|---|---|---|
| P0 Week 1 | Merge T-01 credential live fix + T-04 bypass fix + rotation test | 2d | `bug-fix.MD` B-01/B-02 |
| P0 Week 1 | Redis native `go-redis/v9` dual-mode + `miniredis` test | 3d | §4.2 |
| **P0 Week 1-2** | **Engine `NodeData` capture + `NodeDescription.Properties` extend + top-10 populate** (blocking NDV) | **3d** | `n8n-nodes.MD` §5.1-5.2 |
| P1 Week 2 | Kafka send+trigger `kafka-go` + compose | 5d | §4.1 |
| **P1 Week 2** | **JS plugin loader/registry/wrapper + `--plugin-dir` + hot reload + sandbox** | **4d** | §4.5.1-4.5.3 |
| P1 Week 3 | ExecutionDetail NDV rebuild tabs Schema/Table/JSON + branch picker + Binary | 5d | §4.5.6 |
| P1 Week 3 | Compose dev kafka+redis + parity scripts | 1d | — |
| P2 Week 4 | Docs sync `N8N_FEATURE_COMPARISON.md`/`technical-spec.md` + marketplace POC + community node port 2 contoh (REST API + data aggregator) | 5d | §4.6 |

---

## 6. Out of Scope (Update)
- n8n Cloud-specific features, m9m Web UI drag-drop sudah ada (bukan n8n UI), SSO/SAML, `n8n-nodes-*` **Go binary rebuild path** untuk 95% cases (JS plugin cukup — Go `.so` hanya perf-critical opt-in). gRPC/WASM external nodes Phase 2 enterprise.

## 7. Referensi
- `README.md` parity 19/19, `docs/roadmap.md`, `docs/technical-spec.md`, `docs/PLUGIN_ARCHITECTURE.md` (opsi plugin + wrapper code), `n8n-nodes.MD` (§1-§9 core nodes + NDV + gap matrix + 2026-09-09 live verification), `documentation/docs/integrations/n8n-sync.md` (bridge endpoints+decrypt+env), `internal/webhooks/*.go` diff 2026-10-06, `m9m node list` 42 executors.
