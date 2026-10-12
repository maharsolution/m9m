# m9m — Product Requirements Document (PRD)

| Field | Value |
| --- | --- |
| Document | `m9m.MD` — canonical PRD |
| Version | `0.4.0-draft` |
| Status | **Living document** (updated each release cycle) |
| Last updated | 2026-10-12 (0.4.0 cycle: Kafka SASL, 5×P5 hygiene, JWT fuzz 21-case, 31-node Properties migration, UI retry wiring, OAuth2 RFC 6749 client, OIDC SSO provider, RBAC enforcement middleware + persistence, 12 P6 items implemented: edges-taken renderer, credential form, execution timeline, XML↔JSON helper, queue-soak, scaling, leader election, sub-workflow palette, static data, blobstore, streaming) |
| Owner | Mahar Solution (current maintainer) |
| Original author | Dipankar Sarkar (Neul Labs) |
| Repository | <https://github.com/maharsolution/m9m> |
| License | MIT |
| Source artefacts | `README.md`, `CLAUDE.md`, `CHANGELOG.md`, `docs/roadmap.md`, `docs/technical-spec.md`, `docs/N8N_FEATURE_COMPARISON.md`, `workflow-test.MD`, `parity/parity_gaps.json`, `credentials_n8n_api.MD`, `n8n-nodes.MD`, `gap-current-vs-doc.MD` |

---

## 1. One-line summary

> **m9m is the n8n alternative without the bugs — faster, more reliable workflow automation, written in Go, ships as a single 30 MB static binary with zero runtime dependencies.**

### Tagline (verbatim, from `README.md`)

> *The n8n alternative without the bugs — faster, more reliable workflow automation.*

### Pillars

1. **Faster** — 5–10× faster workflow execution, ~6× faster cold start, 70% lower memory.
2. **More reliable** — single static Go binary, deterministic execution, no npm transitive-dep CVEs, no JS heap leaks.
3. **Drop-in n8n compatible** — runs n8n workflow JSON, expression syntax, credentials, and webhook wire shape unchanged.
4. **Agent-native** — built-in MCP server (37 tools) for Claude Code / Cursor / other AI coding agents.

---

## 2. Mission & vision

### Mission

Deliver a workflow automation platform that is wire-compatible with n8n at the protocol level but is operationally superior — cheaper to run, easier to deploy, harder to break — so existing n8n users can migrate without rewriting workflows and new users get a system that is "boring" in the best sense of the word.

### Vision (12-month horizon)

m9m becomes the **default self-hosted workflow engine** for teams that have outgrown Zapier/Make pricing, find n8n too fragile in production, and are unwilling to give up the n8n ecosystem (workflows, expressions, credentials, UI). Concretely:

- 100% webhook parity with n8n for the 19 canonical cases (already at 19/19 as of 2026-09-10).
- A first-class credential bridge (`sync-service/`) so a n8n design instance and an m9m execution instance run side-by-side in lock-step.
- A self-hosted, AI-assisted workflow editor that uses the same in-app AI runtime already wired through `internal/ai/`.
- Observable in production by default — Prometheus metrics, OpenTelemetry tracing, structured logs, health/readiness probes, all without paid add-ons.

### Why we say "without the bugs"

Three concrete properties that fall out of the Go runtime choice (per `README.md`):

1. **No JavaScript-runtime memory leaks.** Go's GC and value semantics eliminate the long-tail heap growth that requires regular n8n process restarts.
2. **Deterministic execution.** Same input → same output, every time. No event-loop ordering surprises.
3. **Smaller attack surface.** Single static binary; CVEs in n8n's ~1,000-package npm tree do not apply to the runtime path.

---

## 3. Target users & personas

| Persona | Job to be done | Why m9m |
| --- | --- | --- |
| **Platform engineer** running a self-hosted n8n today | Tame n8n's memory leaks, restart cycles, and event-loop stalls. | Single static binary, 5–10× faster, no Node.js to babysit. |
| **Backend engineer** integrating SaaS → DB pipelines | Build durable integrations without hand-rolled cron + queue + retry code. | n8n workflow JSON, durable MySQL/Postgres storage, DLQ, retry-node. |
| **AI engineer** orchestrating LLM agents | Chain Claude/OpenAI calls, scrape, summarise, route by sentiment — and inspect the trace. | Built-in MCP server, in-app AI assistant, OpenTelemetry, AI cost metrics. |
| **Solutions architect** migrating off Zapier/Make | Move from per-task SaaS pricing to a single self-hosted engine. | MIT-licensed, no fair-use clauses, can run inside a customer's VPC. |
| **AI coding agent** (Claude Code, Cursor) | Discover, author, run, and debug workflows without leaving the editor. | MCP server with 37 tools, native `m9m exec` for headless runs. |

### Anti-personas (we explicitly do not optimise for)

- Users who need the full n8n Cloud feature set (managed multi-tenant SaaS, AI credits marketplace, SSO via n8n's IdP).
- Users who depend on community `n8n-nodes-*` npm packages — not supported in m9m.
- Users who need the n8n visual editor 1:1 — m9m ships its own Vue editor, with a growing subset of n8n UX (Schema/Table/JSON, branch-aware NDV, per-node I/O capture).

---

## 4. Use cases

| Use case | Notes |
| --- | --- |
| **AI agent orchestration** | Chain Claude / OpenAI calls, run sandboxed `cli.executeCommand` for agentic tools, route by sentiment. |
| **Data operations** | Scheduled DB sync, conflict resolution, multi-source report merge, pre-write validation. |
| **Business automation** | New-signup routing, lead scoring, invoice approval routing. |
| **Scheduled jobs** | Nightly S3 backups, weekly report emails, API health-checks → Slack alerts. |
| **Webhook fan-out** | Public HTTP entry points (basic auth, header auth, JWT) that fan out to multiple downstream systems. |
| **Hybrid n8n ↔ m9m** | Design in n8n (rich editor), execute on m9m (cheap, fast) via `sync-service/`. |

---

## 5. Goals & non-goals

### 5.1 Goals (in priority order)

| # | Goal | Measure of success |
| --- | --- | --- |
| G1 | **Wire-compat with n8n** for the 19 canonical webhook cases. | 19/19 PASS on every parity cycle (`workflow-test.MD`). |
| G2 | **Drop-in credential sync** so a n8n design instance and m9m execution instance stay in lock-step. | `sync-service/` keeps `node.credentials` envelopes identical post-cycle; positive/3 and positive/8 PASS via real auth. |
| G3 | **Production-grade observability** without paid tiers. | Prometheus metrics, OTel tracing, structured logs, `/healthz` + `/readyz` all green out-of-the-box. |
| G4 | **Performance dominance vs n8n.** | 5–10× execution, 70% lower memory, 6× faster cold start, on the same hardware. |
| G5 | **Single-binary deployment.** | One statically-linked Go binary, ~30 MB, zero npm, no Node.js. |
| G6 | **Agent-native operation.** | Built-in MCP server (≥37 tools), in-app AI assistant, `m9m exec` for headless orchestration. |
| G7 | **Reliable engine semantics.** | Deterministic execution, `RunAwareNodeExecutor` for `$(NodeName)`, IF v2 boolean coercion, `splitInBatches` anchor + done-branch, sticky-note skipping. |
| G8 | **DX parity for debugging.** | n8n-style NDV (Input/Output/Settings tabs), Schema/Table/JSON views, per-node retry, branch-coloured execution edges. |

### 5.2 Non-goals (explicit)

| # | Non-goal | Why |
| --- | --- | --- |
| N1 | n8n Cloud feature parity (managed billing, AI marketplace, n8n's own SSO). | We are self-hosted OSS, MIT. |
| N2 | Running `n8n-nodes-*` community npm packages. | Incompatible with "single static binary" goal. Plugin system (`internal/plugins/`) is the supported extension path. |
| N3 | Sharing the live DB with a running n8n instance. | Schema divergence. `sync-service/` is the bridge. |
| N4 | 1:1 n8n UI clone. | We have our own Vue editor with a focused subset; full n8n UI is out of scope. |
| N5 | Browser-based "click to design, click to deploy" SaaS. | We are an engine + a self-hosted editor, not a hosted service. |

---

## 6. Core capabilities (current state, 2026-10-12)

### 6.1 Runtime

- Go 1.21+, single static binary, ~30 MB.
- Modular, interface-driven design (see `internal/` packages: `engine`, `nodes`, `api`, `queue`, `monitoring`, `credentials`, `expressions`, `runtime`, `webhooks`, `scheduler`, `consensus`, `tenancy`, `versioning`, `audit`, `reliability`, `storage`).
- Queue backends: in-memory, Redis, RabbitMQ.
- Storage backends: in-memory, MySQL (default), Postgres, SQLite.
- Deterministic execution semantics: `RunAwareNodeExecutor`, `$(NodeName)` reads most recent `runIndex`, IF v2 boolean coercion, `splitInBatches` anchor + done-branch.

### 6.2 Node catalogue — 42 registered executors (`cmd/m9m/commands/exec.go::RegisterAllNodes`)

| Category | Count | Nodes |
| --- | --- | --- |
| Core | 7 | `start`, `set`, `if`, `switch`, `filter`, `merge`, `noOp`, `wait`, `itemLists`, `executeWorkflow`, `code`, `function`, `json` |
| Transform | + | `if`, `switch`, `loop`, `splitInBatches`, `itemLists`, `xml`, `code`, `function` |
| Trigger | 8 | `webhook`, `cron`, `errorTrigger`, `executeWorkflowTrigger`, `respondToWebhook`, `kafkaTrigger`, `kafka`, `respond_to_webhook` |
| HTTP | 1 | `httpRequest` |
| Database | 8 | `postgres`, `mySql`, `sqlite`, `redis`, `mongoDb`, `elasticsearch`, plus helpers |
| Messaging | 5 | `slack`, `discord`, `twilio`, `microsoftTeams`, `kafka`, `kafka_native` |
| Email | 3 | `sendEmail`, `sendGrid`, `smtp` |
| AI / LLM | 2 | `@n8n/n8n-nodes-langchain.openAi`, `@n8n/n8n-nodes-langchain.anthropic` |
| Cloud | 4 | `awsLambda`, `s3`, `azureBlobStorage`, `gcpCloudStorage` |
| VCS | 2 | `github`, `gitlab` |
| Productivity | 3 | `googleSheets`, `notion`, `stripe` |
| Code | 1 | `pythonCode` |
| File | 2 | `readBinaryFile`, `writeBinaryFile` |
| CLI | 1 | `executeCommand` |

(`m9m node list` is the authoritative count.)

### 6.3 HTTP / API surface (n8n-compatible)

- **Workflows** — CRUD + activate/deactivate + duplicate + history + tags + transfer.
- **Executions** — list / get / cancel / retry / retry-node / count / recent.
- **Schedules** — CRUD + enable/disable + history.
- **Credentials** — list / get / create / update / delete / test / schema (per `credentials_n8n_api.MD`).
- **Node types** — list + get-one, returning `INodeTypeDescription`-equivalent `properties` arrays.
- **AI assistant** — `generate`, `suggest`, `explain`, `fix`, `chat`, plus provider config GET/PUT/DELETE/test.
- **OTel telemetry** — `GET/PUT/DELETE /api/v1/otel` for live config; standard env-var + DB-override stack.
- **DLQ** — list / get / retry / discard / stats.
- **Settings** — global, license, LDAP.
- **Performance / push** — `GET /api/v1/performance`, `GET /api/v1/push` (WebSocket).
- **Health** — `/healthz`, `/readyz`, `/health/detailed`, `/metrics`.

### 6.4 Integrations (current)

- **AI**: OpenAI, Anthropic, MiniMax (Anthropic-compatible wire shape), Ollama (local).
- **DB**: PostgreSQL (sslMode=disable default to match n8n), MySQL, SQLite, MongoDB, Redis, Elasticsearch.
- **Messaging**: Slack, Discord, Microsoft Teams, Twilio, Kafka.
- **Email**: SMTP (`sendEmail`), SendGrid.
- **Cloud**: AWS S3, AWS Lambda, Azure Blob, GCP Cloud Storage.
- **VCS**: GitHub, GitLab.
- **Productivity**: Notion, Stripe, Google Sheets.
- **CLI agents**: Claude Code, Codex, Aider (sandboxed via `executeCommand`).
- **Data formats**: JSON, XML (`jsonToxml` / `xmlTojson`).
- **Files**: Read/Write Binary File.

### 6.5 Tooling

- **CLI** — `m9m serve`, `m9m exec`, `m9m node list`, `m9m demo`, `m9m benchmark`, `m9m config validate`, `m9m workflow ...`.
- **MCP server** — 37 tools for Claude Code / Cursor; resources for live node-type catalogue.
- **SDKs** — Go (`pkg.go.dev/github.com/maharsolution/m9m`), Node.js (`m9m-cli` on npm), Python (`m9m-cli` on PyPI).
- **Docker** — `ghcr.io/maharsolution/m9m` and `maharsolution/m9m` (Docker Hub). Compose stack at `docker-compose.yml` for m9m + n8n + sync bridge.
- **Helm** — `deploy/helm/`.

### 6.6 Observability

- **Metrics** — Prometheus text exposition on `:9090/metrics` (workflow, node, queue, scheduler, webhook, AI token counters).
- **Traces** — OpenTelemetry OTLP gRPC + HTTP/protobuf. Three-layer config (UI toggle → DB-stored override → env). Span tree: `workflow.execute` → `node.execute` → (`agent.generate` | `db.query` | `http.request`).
- **Logs** — structured (JSON in prod, text in dev). Correlation IDs.
- **Health** — `/healthz` (liveness), `/readyz` (readiness), `/health/detailed`, `/metrics`.

### 6.7 Sync service (n8n ↔ m9m bridge)

`sync-service/sync.py` is a Python FastAPI bridge that lives inside the `m9m-backend` container (managed by `supervisord.conf`). It:

- Polls n8n every `POLL_INTERVAL_SECONDS` (default 30).
- On each cycle, GETs active workflows, GETs/POSTs/PUTs to m9m via `X-N8N-API-KEY`.
- Decrypts n8n's AES-256-CBC + EVP_BytesToKey credential envelopes using `N8N_ENCRYPTION_KEY` and re-encrypts under m9m's envelope.
- Exposes `POST /sync` (manual cycle), `GET /status`, `GET /health` on port 8001.
- Idempotent — POST first, PUT-on-409 fallback.
- Per-workflow failures do not stop the cycle (logged and surfaced in the JSON response).

---

## 7. Functional requirements

### FR-1. Workflow execution
- m9m MUST accept n8n workflow JSON without conversion (`m9m exec my-n8n-workflow.json`).
- m9m MUST evaluate n8n expression syntax `{{ $json.field }}`, `{{ $node["name"].item.json }}`, and built-in functions.
- m9m MUST honour `RunAwareNodeExecutor` semantics for `$(NodeName)`.
- m9m MUST route IF / Switch outputs by the rules n8n produces, including IF v2 boolean coercion.
- m9m MUST skip nodes whose input is empty (n8n's "0 items → don't run" semantics).
- m9m MUST skip decorative nodes (Sticky Notes, comments).

### FR-2. Webhook surface
- m9m MUST register webhook paths exactly as n8n does (`/{path}` or `/webhook/{path}`, plus `/webhook-test/{path}` for editor).
- m9m MUST support HTTP methods `GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS`.
- m9m MUST support `none | basicAuth | headerAuth | JWT` webhook authentication.
- m9m MUST emit Content-Type per the priority: per-node `options.responseContentType` → inbound `Content-Type` → `application/json`.
- m9m MUST honour `Respond: Immediately` (return `{"message":"Workflow got started"}`), `lastNode` (return last node's data), and `responseNode` (use Respond to Webhook).
- m9m MUST clamp payload size to `N8N_PAYLOAD_SIZE_MAX` (16 MB) by default.

### FR-3. Credentials
- m9m MUST expose n8n-compatible credential REST endpoints (`/api/v1/credentials[/{id}|/test|/schema/{type}]`).
- m9m MUST resolve `node.credentials[typeName]` at execution time and pass the resolved secret set to the node's `Execute`.
- m9m MUST fail loudly with an n8n-style error if a credential is missing (e.g. `No credentials found for type 'httpBasicAuth'`) — never fall back to anonymous.
- m9m MUST decrypt n8n Postgres `credentials_entity.data` blobs using `N8N_ENCRYPTION_KEY` during sync and re-encrypt at rest under m9m's envelope.

### FR-4. Sync bridge
- m9m MUST expose a sync service on `:8001` with `POST /sync`, `GET /status`, `GET /health`.
- The sync bridge MUST upsert credentials BEFORE workflows that reference them.
- The sync bridge MUST surface per-workflow failures without aborting the cycle.

### FR-5. API surface
- m9m MUST mirror n8n's public REST surface: workflows, executions, credentials, schedules, tags, users, etc.
- m9m MUST accept `X-N8N-API-KEY` (or Bearer JWT) on `/api/v1/*`.
- m9m MUST emit the same error envelope shape n8n uses for 4xx/5xx.

### FR-6. Editor & UX
- m9m MUST ship its own Vue editor with: visual canvas, drag-to-connect, parameter panels, credential picker, run button.
- m9m MUST render execution detail (NDV) with **Input / Output / Settings** tabs.
- m9m MUST render Input/Output in **Schema / Table / JSON** modes.
- m9m MUST colour execution edges per state (taken, skipped, errored) using `EdgesTaken`.
- m9m MUST support per-workflow debug mode (capture all node I/O when ON, last-node-only when OFF).
- m9m MUST support per-node retry (`POST /executions/{id}/retry-node` + "Retry from here" button).

### FR-7. AI assistant
- m9m MUST expose an in-app AI assistant (Settings → AI card) with provider/model/base-URL/API-key/max-tokens/temperature.
- m9m MUST support OpenAI, Anthropic, MiniMax (Anthropic-compatible), Ollama.
- m9m MUST support `generate`, `suggest`, `explain`, `fix`, `chat` endpoints.
- m9m MUST hot-reload AI config without restart (mutex-guarded).

### FR-8. Observability
- m9m MUST emit Prometheus metrics on `:9090/metrics` when started with `--metrics-port`.
- m9m MUST emit OpenTelemetry traces when enabled, defaulting to a workflow → node → AI-agent span tree.
- m9m MUST honour three-layer OTel config (UI toggle → DB override → env).
- m9m MUST answer `/healthz` and `/readyz` correctly (liveness vs readiness).
- m9m MUST include OTel pipeline health in `/readyz`.

### FR-9. Multi-tenancy & workspaces
- m9m MUST isolate workflows/credentials/executions by workspace.
- m9m MUST support per-workspace API key scoping.

### FR-10. Plugin extension
- m9m MUST support three plugin transports: JavaScript (Goja), gRPC, REST.
- m9m MUST auto-discover plugins from a directory at startup.
- m9m MUST register each plugin under `n8n-nodes-base.<name>` so existing n8n workflows that reference the same name can use it.

---

## 8. Non-functional requirements

### 8.1 Performance (target, vs n8n)

| Metric | Target | How measured |
| --- | --- | --- |
| Cold start | < 1 s | `time m9m serve` to `/healthz` 200 |
| Memory (idle) | < 200 MB | RSS after 5 min idle |
| Workflow execution (small) | 15K workflows/sec | `m9m benchmark` |
| Workflow execution (medium) | 9K workflows/sec | `m9m benchmark` |
| Workflow execution (complex) | 5K workflows/sec | `m9m benchmark` |
| Expression evaluation | 170K ops/sec | `m9m benchmark` expression suite |
| Container size | < 350 MB | `docker images m9m` |
| Binary size | < 35 MB | `ls -la m9m.exe` |
| Concurrent workflows | ≥ 500 | load test harness in `test-workflows/` |

### 8.2 Reliability

- Deterministic execution — same input always produces same output bytes.
- No JS heap growth — long-running m9m process must not need periodic restarts.
- Engine MUST survive a webhook crash (panic-recovery middleware, structured error response).
- DLQ MUST capture terminal workflow failures with full input context for replay.
- `/readyz` MUST return non-200 when any dependency is down (storage, queue, OTel pipeline).

### 8.3 Security

- Credentials encrypted at rest (AES-256-GCM, see `internal/credentials/n8n_cipher.go`).
- Webhook auth via basic / header / JWT, secrets never logged.
- No secrets in trace attributes; trace `http.request` span must redact `Authorization` header.
- Sandbox JavaScript (Goja) and Python (sandboxed) — no `eval`, no `Function`, no unrestricted FS.
- CodeQL / gosec / govulncheck gates in CI.
- Coordinated disclosure per `SECURITY.md` (`me@dipankar.name`, 48h ack SLA).

### 8.4 Operability

- One static binary, one config file, one env file.
- Health/readiness probes out of the box.
- Structured JSON logs in prod, text in dev.
- Per-workflow, per-node, per-AI-token metrics and traces.
- Upgrade path: replace binary, restart — no data migration in the happy path.

### 8.5 Portability

- Target platforms: macOS (Intel + Apple Silicon), Linux (AMD64 + ARM64), Windows (AMD64).
- Storage: MySQL 8, PostgreSQL 13+, SQLite 3.
- Queue: in-memory (dev), Redis, RabbitMQ.
- Container base: distroless or alpine; no Node.js in the runtime image.

### 8.6 Compatibility contract

The contract m9m targets is "behaves like n8n for the documented inputs":

- Workflow JSON (structure + keys).
- Expression syntax + built-in function set.
- Credential envelope shape (minus `data` for parity testing).
- REST API surface (paths, methods, status codes, error shapes).
- Webhook wire shape (paths, methods, headers, Content-Type priority, response modes).
- Auth (X-N8N-API-KEY, JWT, basic, header).
- Node `type` and `typeVersion` integer per `INodeTypeDescription`.

If n8n changes any of the above in a way that breaks parity, m9m opens a parity-gap ticket (`parity/parity_gaps.json`) and a fix lands before the next release.

---

## 9. Architecture overview

```
                    ┌───────────────────────────────────────────┐
                    │  n8n Editor (:5678)                       │
                    │  — design workflows                       │
                    │  — has its own Postgres                   │
                    └─────────────┬─────────────────────────────┘
                                  │ REST + X-N8N-API-KEY
                                  ▼
┌──────────────────────────────────────────────────────────────┐
│  m9m-backend container (single image, supervisord-managed)  │
│  ┌────────────────────────┐  ┌────────────────────────────┐  │
│  │  m9m serve (:8080)     │  │  sync-service/sync.py      │  │
│  │  — engine              │  │  (:8001)                   │  │
│  │  — 42 node executors   │◄─┤  — poll n8n every 30s      │  │
│  │  — REST API            │  │  — decrypt n8n creds       │  │
│  │  — webhooks            │  │  — POST/PUT to m9m         │  │
│  │  — schedules           │  └────────────────────────────┘  │
│  │  — OTel / Prometheus   │                                  │
│  │  — AI runtime          │                                  │
│  │  — DLQ                 │                                  │
│  │  — MCP server (:??? )  │                                  │
│  └─────┬──────────────────┘                                  │
└────────│─────────────────────────────────────────────────────┘
         │
   ┌─────┼──────────┐
   ▼     ▼          ▼
┌──────┐ ┌──────┐ ┌────────────────┐
│MySQL │ │Redis │ │Object storage  │
│:3306 │ │:6379 │ │(S3 / OSS / GCS)│
└──────┘ └──────┘ └────────────────┘
```

Subsystems (Go packages, see `internal/`):

- `engine` — workflow execution, node orchestration, edge routing, retry.
- `nodes/*` — 42 executors across 13 categories.
- `api` — REST handlers, request/response shapes, auth, pagination.
- `webhooks` — webhook manager, content-type priority table, header passthrough.
- `expressions` — Goja-based expression engine, n8n function library.
- `runtime` — sandboxed JS (Goja) and Python.
- `credentials` — envelope store, n8n AES-256-CBC + EVP_BytesToKey decoder, m9m AES-256-GCM encoder.
- `queue` — in-memory / Redis / RabbitMQ drivers.
- `scheduler` — cron + interval, schedule history.
- `storage` — MySQL / Postgres / SQLite / in-memory backends.
- `monitoring` + `otel` — Prometheus metrics, OpenTelemetry tracing, health probes.
- `ai` — in-app AI runtime, provider catalog, mutex-guarded hot reload.
- `mcp` — Model Context Protocol server, 37 tools.
- `plugins` — JS / gRPC / REST plugin transports.
- `reliability` — DLQ, retry policies, circuit breakers.
- `tenancy` + `workspace` — multi-tenant isolation.
- `versioning` — workflow version history.
- `audit` — audit log.
- `consensus` — leader election for queue mode.
- `sandbox` — execution sandbox policies.

---

## 10. Compatibility contract (formal)

m9m targets the following n8n surfaces. Any drift is a bug; see `bug-fix.MD`.

| Surface | Reference | Status |
| --- | --- | --- |
| Workflow JSON | `workflow_n8n_api.MD` | ✅ |
| Credential envelope | `credentials_n8n_api.MD` | ⚠️ envelope shape matched; `data` stripped for parity |
| REST API | `n8n-api-docs.md` | ✅ paths, methods, status codes |
| Expression syntax | `docs/expressions/` | ✅ |
| 19 webhook cases | `workflow-test.MD` Section B/C | ✅ 19/19 |
| Node types (42 of n8n's ~250) | `docs/N8N_FEATURE_COMPARISON.md` | ⚠️ 42 registered, more in backlog |
| `INodeTypeDescription.properties` | `n8n-nodes.MD` §5.2 | ✅ top-10 core nodes + Postgres / MySQL / SQLite / Redis / Mongo / Kafka; legacy fallback for the rest |
| Content-Type priority | `workflow-test.MD` GAP-2026-09-09-1 | ✅ (commit `f92b76b`) |

---

## 11. Ecosystem & integrations

### 11.1 Distribution

- **GitHub Releases** — prebuilt binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.
- **Homebrew** — `brew tap maharsolution/tap && brew install m9m`.
- **Go install** — `go install github.com/maharsolution/m9m/cmd/m9m@latest`.
- **npm / PyPI** — `m9m-cli` SDK packages.
- **Docker** — `ghcr.io/maharsolution/m9m` and `maharsolution/m9m` (Docker Hub); image also pushed to Harbor by `.github/workflows/harbor-image.yml`.
- **Helm** — `deploy/helm/`.

### 11.2 Distribution channels

- Internal Harbor at `harbor.solutiontech.id/m9m/m9m-core` (per `docker-compose.yml` `REGISTRY_IMAGE_URL`).

### 11.3 Adjacent projects (same maintainer)

| Project | Role |
| --- | --- |
| [brat](https://github.com/neul-labs/brat) | Multi-agent harness for AI coding tools. |
| [ringlet](https://github.com/neul-labs/ringlet) | One CLI to rule all your coding agents. |
| [fastworker](https://github.com/neul-labs/fastworker) | Background tasks in Python, no Redis/RabbitMQ. |
| [conductor](https://github.com/neul-labs/conductor) | Multi-agent CLI orchestrator. |

---

## 12. Milestones & release plan

Release cadence: minor versions every 4–6 weeks; patch versions as needed. We follow semver:

- **MAJOR** — breaking change to workflow JSON, REST API, or credential envelope shape.
- **MINOR** — new node, new endpoint (additive), new feature, parity cycle.
- **PATCH** — bug fix, perf improvement, doc fix, dependency bump.

### Roadmap (forward-looking)

| Version | Theme | Anchor |
| --- | --- | --- |
| **0.2.0-parity** ✅ | 19/19 webhook parity | Released 2026-09-10. |
| **0.2.x** | Engine semantics (`EdgesTaken`, debug mode, retry-node, IF v2 coercion, `sslMode=disable`) | Released 2026-09. |
| **0.3.0** ✅ | AI assistant rename, in-app AI Settings card, OTel 3-layer config, sync bridge GA | 2026-09 → 2026-10. |
| **0.4.0** (this PRD) | **Credential sync GA**, DLQ hardening, full execution detail (NDV) rewrite, `INodeTypeDescription` parity for top-25 nodes, 30+ missing n8n nodes, Helm chart GA, sub-workflow UI, edges-taken renderer, execution timeline composable, XML↔JSON helper, queue-soak harness, task-runner interface + in-process bus, leader-election wrapper, static-data KV, blobstore (local + S3), streaming responses. | 2026 Q4. |
| **0.5.0** | Queue mode (Redis/RabbitMQ) GA via the `internal/scaling/` interface, task-runner production wiring (replace `inProcessBus` with Redis / RabbitMQ), horizontal scaling rollout (multi-instance + leader election). | 2027 Q1. |
| **0.6.0** | OAuth2 + JWT + SSO, role-based access, LDAP/AD, audit log. | 2027 Q1–Q2. |
| **0.7.0** | Workflow templates marketplace, community-contributed node packs via plugin transport. | 2027 Q2. |
| **0.8.0** | Production rollout of `internal/blobstore/` (`S3Store` via MinIO / S3), `StreamResponse` UI integration (NDV side panel that streams), image/audio node support. | 2027 Q3. |
| **1.0.0** | Production hardening freeze. SLAs, on-call runbooks, cluster deployment guide. | 2027 Q4. |

### 0.4.0 exit criteria (this PRD's named release)

- [x] `parity_gaps.json GAP-2026-09-09-2` resolved end-to-end (credentials sync + `/api/v1/credentials` + BaseNode resolver). **Resolved 2026-10-12** (BUG-2026-09-09-002, see `bug-fix.MD` §9.1).
- [ ] `internal/ai` legacy `M9M_COPILOT_*` aliases removed in code; CHANGELOG entry.
- [x] `INodeTypeDescription`-equivalent `properties` populated for top-25 core nodes (HTTP, Set, IF, Switch, Code, Function, Webhook, Cron, Filter, Merge, Wait, Loop, SplitInBatches, Postgres, MySQL, SQLite, Mongo, Redis, Kafka, Slack, Discord, Teams, Twilio, SendGrid, RespondToWebhook). **~25 done 2026-10-12** (BUG-UI-2026-09-09-001 PARTIAL, see `bug-fix.MD` §9.12); remaining ~10 long-tail nodes use the raw-JSON fallback in the Settings tab.
- [x] `documentation/docs/` parity claim updated from "95% backend feature parity" to "19/19 webhook parity (2026-09-10 cycle)".
- [ ] DLQ retry path integration-tested with one authed workflow (positive/8).
- [ ] `n8n-nodes.MD` §5.3 (NDV rewrite) marked complete end-to-end (Input/Output/Settings tabs + Schema/Table/JSON + multi-branch + binary tab). **PARTIAL** — Settings tab now renders for every node via raw-JSON fallback (BUG-UI-2026-09-09-002 PARTIAL).
- [ ] Helm chart reaches `v1` (install/upgrade/rollback smoke-tested on k8s 1.28+).
- [x] `,env` accidental filename and `Unconfirmed 467040.crdownload` 112 MB stray file removed from repo (see `bug-fix.MD` §9.11). **Resolved 2026-10-12.**
- [x] Kafka SASL credential injection (BUG-2026-10-12-002, see `bug-fix.MD` §9.10). **Resolved 2026-10-12.**
- [x] MongoDB native driver + per-op displayOptions + credential schema (BUG-MONGO-2026-10-12-001, see `bug-fix.MD` §9.3). **Resolved 2026-10-12.**
- [x] `n8n-nodes-base.dataTable` executor + REST + sync-service (BUG-DATATABLE-2026-10-12-001, see `bug-fix.MD` §9.2). **Resolved 2026-10-12.**
- [x] JWT webhook auth fuzzing harness (BUG-SEC-2026-10-12-001 PARTIAL, see `bug-fix.MD` §9.13). **Resolved 2026-10-12.**

---

## 13. Success metrics

| Metric | Q4 2026 | Q1 2027 | Q2 2027 |
| --- | --- | --- | --- |
| Webhook parity (canonical 19) | 19/19 | 19/19 | 19/19 |
| `m9m node list` count | 50 | 80 | 120 |
| `INodeTypeDescription` coverage (top-25) | 25/25 | 25/25 | 25/25 |
| n8n node coverage (of ~250) | 50/250 | 100/250 | 150/250 |
| Docker image size | < 350 MB | < 320 MB | < 300 MB |
| Cold start | < 1 s | < 0.8 s | < 0.6 s |
| Memory (idle) | < 200 MB | < 180 MB | < 160 MB |
| Concurrent workflows (single instance) | 500 | 750 | 1000 |
| Open P0/P1 bugs (see `bug-fix.MD`) | < 5 | < 3 | 0 |
| Mean time to parity-gap close | < 7 days | < 3 days | < 24h |
| CI pipeline runtime | < 12 min | < 10 min | < 8 min |
| Test coverage (Go) | ≥ 75% | ≥ 80% | ≥ 85% |
| Doc coverage (`documentation/docs/` vs `internal/`) | ≥ 90% | ≥ 95% | ≥ 98% |

---

## 14. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| n8n changes a wire shape (REST path, JSON key, status code) without notice. | High | High | Parity test suite runs in CI on every PR; `parity/parity_gaps.json` is the live backlog; `workflow-test.MD` Section H is the published scoreboard. |
| n8n deprecates `X-N8N-API-KEY` in favour of something else. | Low | High | Watch the upstream OpenAPI spec; m9m accepts `X-N8N-API-KEY`, `Authorization: Bearer`, and `n8n-auth` cookie today. |
| Credential envelope decryption mismatch (n8n rotates its cipher). | Medium | High | `internal/credentials/n8n_cipher.go` is isolated; a unit test vectors file is checked in. New ciphers are additive. |
| Go runtime memory leak in a third-party library (e.g. a MongoDB driver under sustained load). | Medium | High | Add `go test -race` + `pprof` snapshots to the smoke-test pipeline; weekly memory-leak soak test (`test-workflows/soak/`). |
| AI provider rate-limit / outage. | High | Medium | In-app AI is mutex-guarded; circuit breaker in `internal/reliability/`. |
| Sub-workflow recursion blows the stack. | Low | Medium | Engine has a recursion-depth cap (`EngineOptions.MaxSubWorkflowDepth`, default 8). |
| User loses `N8N_ENCRYPTION_KEY` → all synced credentials unreadable. | Medium | Critical | Document the key in `SECURITY.md`; recommend a backup strategy; sync service aborts on missing key. |
| Docs drift (already 12 months behind per `gap-current-vs-doc.MD`). | High | Medium | Doc-only PRs are exempt from CI; one named owner per doc; `mkdocs build` regenerates `documentation/site/` after every docs change. |
| Repo URL drift (`neul-labs/m9m` vs `maharsolution/m9m`). | Medium | Low | Bulk-rewrite in docs already done (Phase 0 of `gap-current-vs-doc.MD`); CI grep check for stale URLs. |
| m9m vs n8n license confusion (Sustainable Use vs MIT). | Low | High | README is explicit (MIT). `LICENSE` file is canonical. |

---

## 15. Out of scope (definitively)

1. **n8n Cloud features** (managed billing, AI credits marketplace, n8n's SSO).
2. **Custom `n8n-nodes-*` npm packages** — use the plugin system instead.
3. **1:1 n8n UI clone** — we have our own editor with a focused subset.
4. **Live DB sharing with a running n8n instance** — use `sync-service/`.
5. **Browser-only "design in the cloud" SaaS** — m9m is engine + self-hosted editor.
6. **Mobile app** — the Vue editor is desktop-first; mobile is web-responsive.

---

## 16. Open questions

| # | Question | Owner | Target date |
| --- | --- | --- | --- |
| OQ-1 | Do we add a `M9M_EXECUTION_MODE=queue` flag for the queue backend, or auto-detect from env? | TBD | 0.5.0 |
| OQ-2 | How do we version credential envelopes — per-write key rotation, or per-instance? | TBD | 0.4.0 (with credential sync GA) |
| OQ-3 | Should the in-app AI assistant route to the user's own API key (BYOK), or to a Mahar Solution proxy? | TBD | 0.4.0 |
| OQ-4 | Do we expose `m9m exec --replay <execution-id>` for one-click replay from the UI? | TBD | 0.4.0 |
| OQ-5 | Helm chart values: how do we let ops pin `M9M_MYSQL_DSN` without templating the entire `.env`? | TBD | 0.4.0 |
| OQ-6 | Should the plugin transport support WASM (in addition to JS/gRPC/REST)? | TBD | 0.6.0 |
| OQ-7 | Multi-tenant isolation — row-level security in MySQL, or schema-per-tenant? | TBD | 0.5.0 |
| OQ-8 | Do we ship a "lite" build (no OTel, no AI, no MCP) for embedded use? | TBD | 0.6.0 |

---

## 17. References

- README — <https://github.com/maharsolution/m9m/blob/main/README.md>
- `CLAUDE.md` — agent working notes for the codebase.
- `CHANGELOG.md` — release notes.
- `docs/roadmap.md` — original 16-week roadmap (now superseded by §12 above).
- `docs/technical-spec.md` — original technical spec (now superseded by §6–§9 above).
- `docs/N8N_FEATURE_COMPARISON.md` — feature gap analysis, last updated 2026-10-07.
- `docs/REFRACTOR_PLAN.md` — module decomposition plan.
- `docs/PLUGIN_SYSTEM.md` — plugin architecture.
- `docs/PLUGIN_ARCHITECTURE.md` — plugin design.
- `n8n-api-docs.md` — captured n8n public API + webhook reference.
- `workflow_n8n_api.MD` — n8n workflow OpenAPI 3.0.1 dump.
- `credentials_n8n_api.MD` — n8n credentials API reference.
- `n8n-nodes.MD` — n8n core nodes reference for m9m.
- `workflow-test.MD` — live n8n ↔ m9n parity test report.
- `parity/parity_gaps.json` — live parity-gap backlog.
- `gap-current-vs-doc.MD` — docs vs code drift analysis.
- `SECURITY.md` — coordinated disclosure.
- `CONTRIBUTING.md` — contribution guide.

---

## 18. Sign-off

This PRD is a **living document**. It is updated:

- At the start of each release cycle.
- When a new node or endpoint lands.
- When a parity gap is opened or closed.
- When a milestone slips.

Owner of the PRD: the maintainer (`Mahar Solution`).
Review cadence: every 4 weeks or before each minor release.
