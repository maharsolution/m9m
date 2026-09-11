# m9m

> **The n8n alternative without the bugs — faster, more reliable workflow automation.**

m9m is an open-source workflow automation platform written in Go. It runs n8n workflow JSON unchanged, executes 5–10× faster, uses 70% less memory, and ships as a single 30 MB binary with zero runtime dependencies. No Node.js, no npm tree, no event-loop stalls.

**Live webhook parity with n8n: 19/19 cases passing** (14 positive + 5 negative) as of the 2026-09-10 cycle.

[![Build Status](https://img.shields.io/github/actions/workflow/status/neul-labs/m9m/ci.yml?branch=main&style=flat-square&logo=github)](https://github.com/neul-labs/m9m/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/neul-labs/m9m?style=flat-square)](https://goreportcard.com/report/github.com/neul-labs/m9m)
[![Coverage](https://img.shields.io/codecov/c/github/neul-labs/m9m?style=flat-square&logo=codecov)](https://codecov.io/gh/neul-labs/m9m)
[![Go Reference](https://img.shields.io/badge/go.dev-reference-007d9c?style=flat-square&logo=go)](https://pkg.go.dev/github.com/neul-labs/m9m)
[![Release](https://img.shields.io/github/v/release/neul-labs/m9m?style=flat-square&logo=github)](https://github.com/neul-labs/m9m/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](https://opensource.org/licenses/MIT)

```bash
curl -fsSL https://raw.githubusercontent.com/neul-labs/m9m/main/install.sh | bash
m9m demo
```

---

## Table of contents

- [What is m9m?](#what-is-m9m)
- [Why m9m? (vs n8n)](#why-m9m-vs-n8n)
- [30-second quickstart](#30-second-quickstart)
- [Install](#install)
- [Use cases](#use-cases)
- [Drop-in n8n compatibility](#drop-in-n8n-compatibility)
- [Built-in integrations](#built-in-integrations)
- [AI assistant (in-app)](#ai-assistant-in-app)
- [Telemetry & observability](#telemetry--observability)
- [Ecosystem](#ecosystem)
- [FAQ](#faq)
- [Recent highlights (2026-09 cycle)](#recent-highlights-2026-09-cycle)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

---

## What is m9m?

m9m is a cloud-native workflow automation engine — a faster, more reliable drop-in alternative to [n8n](https://n8n.io). You give it a workflow (a directed graph of nodes that fetch data, transform it, call APIs, run AI agents, or wait on schedules), and it executes that workflow with the same JSON format, expression syntax, and credentials that n8n uses.

The difference is the runtime. n8n is a Node.js application; m9m is a single statically-linked Go binary. That swap eliminates an entire class of production issues — JavaScript heap leaks, npm supply-chain risk, event-loop pauses under load, slow cold starts — while delivering 5–10× faster workflow execution on the same hardware.

m9m is open source under the MIT license, ships for macOS, Linux, and Windows on both AMD64 and ARM64, and integrates natively with Claude Code and other AI coding agents via the Model Context Protocol (MCP).

---

## Why m9m? (vs n8n)

| | m9m | n8n |
|---|---|---|
| **Cold start** | ~500 ms | ~3 s |
| **Memory (idle)** | ~150 MB | ~512 MB |
| **Container size** | ~300 MB | ~1.2 GB |
| **Workflow execution** | Baseline | 5–10× slower |
| **Concurrent workflows** | 500 | 50 |
| **Runtime** | Single static Go binary | Node.js + ~1,000 npm packages |
| **Deterministic execution** | Yes | No (event-loop ordering) |
| **n8n workflow JSON** | Runs unchanged | Native |
| **n8n webhook parity** | **19/19 cases** (14 pos + 5 neg) | Native |
| **MCP server for Claude Code** | Built in (37 tools) | Not available |
| **License** | MIT | Sustainable Use License |

### Why we say *"without the bugs"*

Not a marketing line — three specific reliability properties that fall out of the runtime choice:

1. **No JavaScript-runtime memory leaks.** Go's garbage collector and value semantics eliminate the long-tail heap growth that requires regular n8n process restarts in production.
2. **Deterministic execution.** Same input, same output, every time — no event-loop ordering surprises or async timing edge cases. Workflows behave the same in test, staging, and prod.
3. **Smaller attack surface.** Single static binary with no transitive npm dependencies in the runtime path. CVEs in n8n's ~1,000-package dependency tree don't apply.

Performance numbers are reproducible on your hardware with `m9m benchmark`. The full methodology is in [docs/performance-report.md](docs/performance-report.md).

---

## 30-second quickstart

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/neul-labs/m9m/main/install.sh | bash

# 2. Run the bundled demo (six workflows execute end-to-end)
m9m demo

# 3. Run your own workflow
m9m exec workflow.json
m9m exec workflow.json --input '{"customer_id": 42}'

# 4. List all built-in nodes
m9m node list
```

Output from `m9m demo`:

```
m9m Capability Demo
===================

Demo 1: E-Commerce Order Processing
  Pipeline:  Start -> Set -> Set -> Filter
  Status:    PASS
  Duration:  8ms

Demo 2: Customer Segmentation
  Pipeline:  Start -> Set -> Filter -> Set
  Status:    PASS
  Duration:  12ms

...

Results: 6/6 demos passed
```

---

## Install

| Surface | Command |
|---|---|
| **Homebrew (macOS / Linux)** | `brew tap neul-labs/tap && brew install m9m` |
| **Installer script (any platform)** | `curl -fsSL https://raw.githubusercontent.com/neul-labs/m9m/main/install.sh \| bash` |
| **Go** | `go install github.com/neul-labs/m9m/cmd/m9m@latest` |
| **Docker (GHCR)** | `docker run -p 8080:8080 ghcr.io/neul-labs/m9m:latest` |
| **Docker (Docker Hub)** | `docker run -p 8080:8080 neul-labs/m9m:latest` |
| **Python SDK** | `pip install m9m-cli` |
| **Node.js SDK** | `npm install m9m-cli` |
| **GitHub Releases** | [Download prebuilt binaries](https://github.com/neul-labs/m9m/releases) |

Supported platforms: macOS (Intel + Apple Silicon), Linux (AMD64 + ARM64), Windows (AMD64).

---

## Use cases

### AI agent orchestration
Run Claude Code, Codex, or Aider in sandboxed environments with resource limits and namespace isolation. Chain multiple AI models, add human-review steps, route by sentiment. See [docs/nodes/cli.md](docs/nodes/cli.md).

### Data operations
Sync databases on a schedule with conflict resolution. Merge reports from multiple SaaS sources into a single dashboard. Clean and validate incoming data before it hits production tables.

### Business automation
Process new signups and route to the right onboarding flow. Score leads from activity across platforms. Route invoices to approvers by amount and department.

### Scheduled jobs
Back up critical data to S3 nightly. Generate and email weekly reports. Monitor APIs and alert on Slack when something breaks.

Ready-to-run examples live in [`examples/`](examples/).

---

## Drop-in n8n compatibility

Already running n8n? Point m9m at your existing workflow JSON. No conversion step.

```bash
m9m exec my-n8n-workflow.json
```

**What runs unchanged:**

- Workflow JSON format and node connections
- Expression syntax (`{{ $json.field }}`, `{{ $node["name"].data }}`)
- 40+ node types covering the most-used n8n functionality
- Trigger types (webhooks, cron schedules)
- Credential formats and environment variables
- REST API endpoints (n8n-compatible surface)
- Webhook wire shape (headers, status codes, response bodies, Content-Type priority)
- JWT auth (RS256/HS256), basic auth, header auth — `X-N8N-API-KEY` compatible
- Execution semantics: `$(NodeName)` resolution, `runIndex`, `RunAwareNodeExecutor`
- IF v2 boolean operators, Switch rule routing, splitInBatches loop semantics

**What doesn't (yet):**

- Custom community nodes from `n8n-nodes-*` npm packages
- n8n Cloud–specific features
- Live database sharing with an existing n8n instance
- The n8n web UI (m9m ships its own)

### n8n webhook parity — 19/19 (2026-09-10)

The live parity suite runs 19 webhook cases (14 positive + 5 negative) against a production n8n instance and an m9m instance side-by-side, verifying identical HTTP responses, headers, status codes, and execution semantics. The 2026-09-10 cycle resolved the last two long-standing gaps (IF/Switch routing leaks + Postgres `sslMode`) — the full per-case report and the cycle-by-cycle diff are kept inside the repository.

| Suite | Cases | Status |
|---|---|---|
| Positive webhook tests | 14 | ✅ PASS |
| Negative webhook tests | 5 | ✅ PASS |
| **Total** | **19** | **✅ 19/19** |

Coverage includes: Webhook → Code, Webhook → Switch (route), Webhook → HTTP basic auth, Webhook → Set (assign + formula), Webhook → XML (`jsonToxml` / `xmlTojson`), Webhook → Code → Loop → Response, Webhook → Code → IF (Pay validation), Webhook → IF (mysql/postgres switch), plus negative cases for malformed JSON, auth failures, and missing routes.

Full migration guide: [docs/migration/from-n8n.md](docs/migration/from-n8n.md).

---

## Built-in integrations

| Category | Nodes |
|---|---|
| **Databases** | PostgreSQL, MySQL, SQLite, MongoDB, Redis, Elasticsearch |
| **Cloud** | AWS S3, AWS Lambda, Azure Blob Storage, GCP Cloud Storage |
| **AI / LLM** | OpenAI (GPT-4), Anthropic Claude |
| **Messaging** | Slack, Discord, Twilio, Microsoft Teams |
| **Email** | SMTP (`sendEmail`), SendGrid |
| **Version control** | GitHub, GitLab |
| **Productivity** | Notion, Stripe, Google Sheets |
| **CLI agents** | Claude Code, Codex, Aider (sandboxed), `executeCommand` |
| **HTTP / Webhooks** | HTTP Request, Webhook trigger, Respond to Webhook |
| **Data formats** | JSON, XML (jsonToxml / xmlTojson) |
| **Files** | Read Binary File, Write Binary File |
| **Code execution** | Code (JS via Goja), Function, Python Code |
| **Logic & flow** | Set, Filter, Merge, Item Lists, If, Switch, Loop, Cron, splitInBatches, Wait, NoOp, Execute Workflow |

Run `m9m node list` for the full catalog. 42 executors registered in `cmd/m9m/commands/exec.go::RegisterAllNodes`. Custom logic in JavaScript (Code / Function) or Python (Python Code) when you need it.

---

## AI assistant (in-app)

The in-app **AI assistant** powers workflow generation, node suggestions, error fixes, and chat. It is configured live from **Settings → AI** — provider, base URL, API key, model, max tokens, and temperature override env defaults without a restart. Supports **OpenAI** (GPT-4o, o1, o3, …), **Anthropic** (Claude Opus 4.1, Sonnet 4.5, Haiku 4.5), **MiniMax-M3** (Anthropic-compatible wire shape), and **Ollama** (local, no key). The same Settings card drives the in-app agent and the workflow `openAi` / `anthropic` nodes — both share the resolved runtime, so a change in Settings is picked up by the next workflow execution as well.

---

## Telemetry & observability

m9m ships production-grade observability out of the box — no extra agents, no paid tiers. Every workflow execution, node call, AI agent invocation, database query, and webhook is instrumented by default and exported through three complementary surfaces.

### What's emitted

| Signal | Source | Format | Default endpoint |
|---|---|---|---|
| **Metrics** | Engine, queue, scheduler, node registry | Prometheus text | `:9090/metrics` (when started with `--metrics-port`) |
| **Traces** | Workflow span → per-node span → AI agent span | OpenTelemetry (OTLP gRPC + HTTP) | Disabled by default — flip on via env, DB, or UI |
| **Logs** | Engine + nodes, with correlation IDs | Structured (JSON in prod, text in dev) | stderr |
| **Health** | Storage, queue, OTEL pipeline, dependency health | JSON | `GET /healthz`, `GET /readyz` |

### OpenTelemetry tracing — what your workflows get

Every workflow execution produces a span tree:

```
workflow.execute                          (workflow_id, mode, status)
├── node.execute  (n8n-nodes-base.httpRequest)
├── node.execute  (n8n-nodes-base.set)
└── node.execute  (n8n-nodes-base.code)
    └── agent.generate  (openAi / anthropic, model, tokens)
```

Spans carry `workflow.id`, `workflow.name`, `node.type`, `node.name`, `execution.id`, `execution.mode`, plus standard `service.name=m9m`, `service.version=<git-sha>` resource attributes.

### Configuration

Three layers, resolved in this priority order (highest wins):

1. **Live UI toggle** — flip tracing on/off from **Settings → Telemetry** without a restart. The UI writes to the DB; the engine reloads on next execution.
2. **DB override** — `m9m.otel_config` row in the active storage backend. Survives restarts, overrides env.
3. **Environment variables** — standard OpenTelemetry vars take effect at boot:
   - `OTEL_SDK_DISABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL` (grpc | http/protobuf | http/json)
   - `OTEL_SERVICE_NAME` (defaults to `m9m`), `OTEL_RESOURCE_ATTRIBUTES`
   - m9m-specific: `M9M_OTEL_ENABLED`, `M9M_OTEL_ENDPOINT`, `M9M_OTEL_PROTOCOL`, `M9M_OTEL_HEADERS`, `M9M_OTEL_SAMPLE_RATIO` — these win over the standard `OTEL_*` vars

A sanitised `.env.example` is checked in; the typo'd local `,env` is git-ignored.

### Prometheus metrics — what's exposed

Representative metrics (full list at `GET /metrics`):

```
# Workflow
m9m_workflow_executions_total{workflow_id, workflow_name, status, mode}
m9m_workflow_duration_seconds{workflow_id, workflow_name}     histogram
m9m_workflow_errors_total{workflow_id, error_type}
m9m_workflow_concurrent_executions                            gauge

# Nodes
m9m_node_executions_total{node_type, status}
m9m_node_duration_seconds{node_type}                          histogram

# Queue + scheduler
m9m_queue_jobs_total{queue, status}
m9m_scheduler_next_run{workflow_id}                           gauge

# HTTP / webhooks
m9m_webhook_requests_total{path, status}
m9m_webhook_duration_seconds{path}                            histogram

# AI
m9m_ai_tokens_total{provider, model, direction}               (input/output)
```

Scrape config (Prometheus):

```yaml
scrape_configs:
  - job_name: m9m
    static_configs:
      - targets: ['m9m-host:9090']
```

### Health endpoints

| Endpoint | Purpose | Used by |
|---|---|---|
| `GET /healthz` | Liveness — process is up | K8s livenessProbe |
| `GET /readyz` | Readiness — storage, queue, OTEL pipeline reachable | K8s readinessProbe / load balancers |
| `GET /metrics` | Prometheus exposition | Prometheus / VictoriaMetrics |

### Debug mode

Per-workflow debug flag in the UI: when ON, every node's full input and output is persisted with the execution (NDV-style Input / Output / Settings tabs). When OFF, only the last-actually-executed node's data is captured — keeps the executions table small for production.

Full guide: [docs/monitoring/README.md](docs/monitoring/README.md).

---

## Ecosystem

- **CLI** — `m9m` binary, the primary surface. Serve, exec, schedule, MCP.
- **Go SDK** — embed the engine directly in a Go application. [Reference](https://pkg.go.dev/github.com/neul-labs/m9m).
- **Node.js SDK** — [`m9m-cli`](https://www.npmjs.com/package/m9m-cli) on npm. TypeScript-first.
- **Python SDK** — [`m9m-cli`](https://pypi.org/project/m9m-cli/) on PyPI. Full type hints, context-manager API.
- **MCP server** — 37 tools for Claude Code / Cursor / other MCP clients. [docs/mcp/README.md](docs/mcp/README.md).
- **Docker** — `ghcr.io/neul-labs/m9m` (releases) and `neul-labs/m9m` (Docker Hub).
- **Kubernetes** — Helm chart and example manifests in [deploy/](deploy/).

---

## FAQ

### Is m9m a drop-in replacement for n8n?
For workflow execution: yes. m9m runs n8n workflow JSON, expression syntax, and credentials unchanged across 40+ built-in node types. Community nodes published as `n8n-nodes-*` npm packages and n8n Cloud–specific features are not yet supported. See the [migration guide](docs/migration/from-n8n.md).

### How much faster is m9m than n8n?
On the same hardware: 5–10× faster workflow execution, ~6× faster cold start (~500 ms vs ~3 s), 70% lower memory (~150 MB vs ~512 MB), 75% smaller container (~300 MB vs ~1.2 GB). Reproducible with `m9m benchmark`. Full numbers and methodology in [docs/performance-report.md](docs/performance-report.md).

### Why is m9m more reliable than n8n?
m9m is a single statically-linked Go binary. Three concrete consequences: no long-running Node.js heap growth (no scheduled restarts), deterministic execution (no event-loop ordering surprises), and no npm transitive-dependency CVEs in the runtime path. The trade-off is that custom logic is sandboxed JavaScript (via Goja) or Python rather than arbitrary npm imports.

### Can I import existing n8n workflows?
Yes. Export the workflow JSON from n8n and run `m9m exec workflow.json`. No conversion step. Credentials use the same format.

### Does m9m work with Claude Code, Cursor, and other AI coding agents?
Yes, natively. m9m ships a built-in MCP (Model Context Protocol) server with 37 tools for workflow orchestration. Agents can list, create, execute, and inspect workflows directly. See [docs/mcp/README.md](docs/mcp/README.md).

### Does m9m run on Windows?
Yes. Prebuilt Windows AMD64 binaries are on every GitHub release. macOS (Intel + Apple Silicon) and Linux (AMD64 + ARM64) are also first-class.

### Is m9m production-ready?
Yes. Single static binary, Prometheus metrics, OpenTelemetry tracing (env-default + DB-override, toggleable live from the UI), `/healthz` + `/readyz` probes, Git-based workflow versioning, audit logs, multi-workspace support, per-workflow debug flag, and per-node retry are all built in — not gated behind a paid tier. Production deployment guide: [docs/deployment/](docs/deployment/).

### Does m9m support OpenTelemetry tracing?
Yes — full OTLP export (gRPC and HTTP/protobuf) with a three-layer config stack: standard `OTEL_SDK_*` env vars at boot, `M9M_OTEL_*` overrides, DB-stored config that survives restarts, and a Settings → Telemetry toggle that flips tracing on/off live without a restart. Every workflow produces a span tree (`workflow.execute` → `node.execute` → `agent.generate`) with `workflow.id`, `node.type`, `execution.id`, and `service.version=<git-sha>` resource attributes. See [Telemetry & observability](#telemetry--observability) above for the full config matrix and metrics list.

### Has m9m actually been tested against a live n8n instance?
Yes. A parity suite runs 19 webhook cases (14 positive + 5 negative) against a production n8n instance and an m9m instance side-by-side, and reports 19/19 PASS as of the 2026-09-10 cycle. The full per-case report with request/response diffs lives in the repository. Every n8n wire-shape fix from the last cycle (Content-Type priority, JWT auth, header auth, basic auth, malformed-JSON 422, response-body trim, IF v2 boolean operators, splitInBatches loop semantics, Postgres `sslMode`, respond-to-webhook unwrapping) is verified live, not just unit-tested.

### Is m9m free and open source?
Yes. MIT-licensed. No "fair-use" clauses, no source-available restrictions, no commercial-use carveouts.

### How does m9m compare to Zapier or Make?
Zapier and Make are hosted SaaS platforms; m9m is self-hosted open-source software you run on your own infrastructure. The right comparison is to n8n, Airflow, or Temporal — and m9m is faster than all three for the integration-automation use case.

---

## Recent highlights (2026-09 cycle)

The September 2026 cycle focused on closing the remaining n8n wire-shape and execution-semantics gaps so m9m can serve as a true drop-in replacement behind the same webhook URLs:

- **19/19 webhook parity with n8n** — all positive and negative cases PASS on the live side-by-side parity suite.
- **n8n-style execution detail UI** — Input / Output / Settings tabs, Schema / Table / JSON views, sticky-note skipping, visible workflow edges with state-coloured execution edges.
- **Per-workflow debug flag + per-node retry** — capture all node I/O only when debug is ON; `POST /executions/:id/retry-node` + "Retry from here" button.
- **OpenTelemetry tracing (3-layer config)** — standard `OTEL_SDK_*` env vars accepted at boot, `M9M_OTEL_*` wins, DB-stored override survives restarts, **Settings → Telemetry** live toggle without restart. Workflow → node → AI-agent span tree with `service.version=<git-sha>` resource attribute.
- **AI assistant with live Settings wiring** — full **Settings → AI** card (OpenAI / Anthropic Claude / MiniMax / Ollama, with per-provider model dropdown) using the same env-default + DB-override pattern as Telemetry; runtime swap is mutex-guarded so in-flight chat requests finish on the previous *AI. The in-app `Agent Copilot` is renamed to `AI` to reflect that it handles all providers, not just GitHub Copilot.
- **Credential bridge** — sync from n8n Postgres (REST API strips the `data` field), seeded into m9m's in-memory store at startup, idempotent POST.
- **XML node** — `n8n-nodes-base.xml` executor (`jsonToxml` / `xmlTojson`, case-insensitive mode spellings) + parity test script.
- **Engine semantics** — `$(NodeName)` reads the most recent `runIndex`, `RunAwareNodeExecutor` for `$(NodeName).item.json` in Code nodes, splitInBatches anchor + done-branch scheduling stabilised, decorative nodes (sticky notes / comments) skipped.
- **Database** — Postgres `sslMode=disable` default to match n8n, automatic `node_data` column migration for existing MySQL/Postgres tables.
- **Build pipeline** — Vue frontend built inside Docker so `//go:embed dist/*` is non-empty; `web/dist` untracked and wiped in web-builder stage.

---

## Documentation

| Resource | Description |
|---|---|
| [Documentation site](https://docs.neullabs.com/m9m) | Full docs — installation, nodes, API, deployment |
| [Getting Started](docs/README.md) | Quick-start guide |
| [Why m9m?](docs/N8N_FEATURE_COMPARISON.md) | Feature comparison and gap analysis |
| [Performance report](docs/performance-report.md) | Benchmarks and methodology |
| [Migrate from n8n](docs/migration/from-n8n.md) | Step-by-step migration |
| [Architecture](docs/architecture/README.md) | System design |
| [API reference](docs/api/API_COMPATIBILITY.md) | REST API |
| [Node development](docs/nodes/README.md) | Build custom nodes |
| [Deployment](docs/deployment/DEPLOYMENT_GUIDE.md) | Production deployment |
| [MCP integration](docs/mcp/README.md) | Claude Code & MCP clients |

---

## Contributing

```bash
git clone https://github.com/neul-labs/m9m.git
cd m9m
make deps && make test && make build
```

See the [Contributing Guide](docs/CONTRIBUTING.md). Issues and discussions live on [GitHub](https://github.com/neul-labs/m9m/issues).

## Community

- [GitHub Discussions](https://github.com/neul-labs/m9m/discussions) — questions and design proposals
- [GitHub Issues](https://github.com/neul-labs/m9m/issues) — bugs and feature requests
- [Release Notes](https://github.com/neul-labs/m9m/releases) — changelog

## License

MIT License. See [LICENSE](LICENSE).

## Part of the Neul Labs toolchain

m9m is part of the Neul Labs orchestration toolchain:

| Project | Description |
|---------|-------------|
| [brat](https://github.com/neul-labs/brat) | Multi-agent harness for AI coding tools — crash-safe state, parallel execution. |
| [ringlet](https://github.com/neul-labs/ringlet) | One CLI to rule all your coding agents. |
| [fastworker](https://github.com/neul-labs/fastworker) | Background tasks in Python with zero infrastructure — no Redis, no RabbitMQ. |
| [conductor](https://github.com/neul-labs/conductor) | Multi-agent CLI orchestrator for AI coding agents. |

Learn more at [neullabs.com](https://www.neullabs.com).

---

[Documentation](https://docs.neullabs.com/m9m) · [GitHub](https://github.com/neul-labs/m9m) · [npm](https://www.npmjs.com/package/m9m-cli) · [PyPI](https://pypi.org/project/m9m-cli/) · [Docker](https://github.com/neul-labs/m9m/pkgs/container/m9m)
