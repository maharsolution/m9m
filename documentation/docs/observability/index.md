---
title: "Observability"
description: "Prometheus metrics, OpenTelemetry tracing, structured logs, and health checks."
keywords: "m9m observability, OpenTelemetry, OTLP, Prometheus, tracing, sampling"
---

# Observability

m9m ships with first-class observability: Prometheus metrics, OpenTelemetry tracing, structured JSON logs, and detailed health checks. Nothing is gated behind an enterprise tier — it's all in the open-source build.

This page covers tracing. For metrics see `/metrics`, for logs see the structured JSON format, for health see `/healthz` and `/health/detailed`.

---

## Quick start

```bash
# 1. Point at any OTLP-compatible backend
export M9M_OTEL_ENABLED=true
export M9M_OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export M9M_OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# 2. Start the server
m9m serve

# 3. Confirm a span was emitted
curl -X POST http://localhost:8080/api/v1/otel/test \
  -H "Content-Type: application/json" \
  -d '{"message": "hello"}'
```

The backend should receive a `m9m.otel.test` span with attributes `service.name=m9m`, `service.version=<git-sha>`.

---

## OpenTelemetry tracing

### Exporters

Two protocols, both OTLP:

| Protocol | Accepted values | Default endpoint | Notes |
|---|---|---|---|
| OTLP gRPC | `grpc` | `http://localhost:4317` | Lower overhead, recommended for production |
| OTLP HTTP/protobuf | `http`, `http/protobuf` | `http://localhost:4318` | Easier to debug; slightly higher overhead |

The OTel SDK default is `http/protobuf`. m9m's `ApplyDefaults()` mirrors that
unless the operator set `Protocol=grpc`. Protocol strings are case-insensitive
(the loader lower-cases them before validating).

Jaeger is **no longer** a first-class exporter — use an OTLP collector that forwards to Jaeger (or use Tempo, Honeycomb, Datadog, etc., all of which speak OTLP).

### Configuration layers

There are **five layers**, applied in this order, **last wins**:

1. **Built-in defaults** — `Enabled=false`, `Protocol="http/protobuf"`, defaults shown in the table below.
2. **Standard OpenTelemetry SDK vars** — `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER`, etc. Honoured by the upstream OTel SDK; loaded as a fallback.
3. **`M9M_OTEL_*` env vars** — m9m's convenience aliases (names prefixed with `M9M_` to avoid clashing with n8n on a shared host). Win over the bare `OTEL_*` names when both are set.
4. **DB-stored override** — what you save via `PUT /api/v1/otel`. Persists across restarts. Wins over env.
5. **Live UI toggle** in **Settings → Telemetry** — overrides everything for the on/off bit only. Other fields keep their env / DB value.

### Environment variables

The m9m-side names are listed first. When only a standard OTel-SDK name is
shown, m9m reads the standard SDK var as a fallback (the m9m name still wins
when both are set).

#### Core tracing

| Variable | Type | Default | Description |
|---|---|---|---|
| `M9M_OTEL_ENABLED` | boolean | `false` | Master on/off |
| `M9M_OTEL_EXPORTER_OTLP_PROTOCOL` (or `OTEL_EXPORTER_OTLP_PROTOCOL`) | string | `http/protobuf` | `grpc`, `http`, or `http/protobuf` |
| `M9M_OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_ENDPOINT`) | string | `http://localhost:4317` (gRPC) or `http://localhost:4318` (HTTP) | OTLP collector base URL; the exporter appends the protocol path |
| `M9M_OTEL_EXPORTER_OTLP_HEADERS` (or `OTEL_EXPORTER_OTLP_HEADERS`) | string | — | Comma-separated `key=value` pairs (e.g. `x-api-key=abc,foo=bar`) |
| `M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE` (or `OTEL_EXPORTER_OTLP_HEADERS_FILE`) | string | — | Path to a file containing the same `key=value` pairs. When set, takes precedence over `HEADERS` |
| `M9M_OTEL_TRACES_SAMPLE_RATE` (or `OTEL_TRACES_SAMPLER_ARG`) | number | `1.0` (when enabled) | 0.0–1.0 fraction of traces to keep. Combined with `OTEL_TRACES_SAMPLER` (`always_on`, `always_off`, `traceidratio`, `parentbased_*)` |
| `M9M_OTEL_TRACES_PRODUCTION_ONLY` | boolean | `true` | When `true`, executions whose mode is NOT `webhook`/`trigger`/`manual`/`retry` are sampled with `NeverSample`. Matches n8n's n8n-OTEL behaviour |
| `M9M_OTEL_TRACES_INCLUDE_NODE_SPANS` | boolean | `true` | When `false`, the engine skips `node.execute` spans and only emits `workflow.execute` |
| `M9M_OTEL_TRACES_INJECT_OUTBOUND` | boolean | `true` | Whether the HTTP Request node injects a `traceparent` header on outbound calls |

#### Resource attributes

| Variable | Type | Default | Description |
|---|---|---|---|
| `M9M_OTEL_SERVICE_NAME` (or `OTEL_SERVICE_NAME`) | string | `m9m` | `service.name` on the OTel resource |
| `M9M_OTEL_SERVICE_VERSION` (or `OTEL_SERVICE_VERSION`) | string | `<git-sha>` (or `dev`) | `service.version` on the OTel resource |
| `M9M_OTEL_INSTANCE_ID` | string | `<hostname>:<pid>` | `service.instance.id`. Auto-generated when unset |
| `M9M_ENV` | string | `production` / `development` | `deployment.environment` on the OTel resource |

#### Agents (GenAI) tracing

| Variable | Type | Default | Description |
|---|---|---|---|
| `M9M_AGENTS_TRACING_ENABLED` | boolean | `false` | Toggles GenAI agent + tool-call spans |
| `M9M_AGENTS_TRACING_RECORD_INPUTS` | boolean | `true` | Whether prompts / tool args are recorded as `gen_ai.prompt` / `gen_ai.tool.call.arguments` |
| `M9M_AGENTS_TRACING_RECORD_OUTPUTS` | boolean | `true` | Whether completions / tool results are recorded as `gen_ai.completion` / `gen_ai.tool.call.result` |

#### Standard OTel-SDK fallbacks

Read as fallbacks (m9m-prefixed names win on conflict):

- `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`,
  `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_HEADERS_FILE`
- `OTEL_SERVICE_NAME`, `OTEL_SERVICE_VERSION`
- `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`

### Sampling

| Strategy | When |
|---|---|
| Always-on | `M9M_OTEL_TRACES_SAMPLE_RATE=1` |
| Always-off | `M9M_OTEL_ENABLED=false` or `M9M_OTEL_TRACES_SAMPLE_RATE=0` |
| Ratio | `M9M_OTEL_TRACES_SAMPLE_RATE=0.1` keeps ~10% |
| Parent-based (default when unset) | `OTEL_TRACES_SAMPLER=parentbased_*` propagates the parent's decision |

For multi-service tracing, set `sampleRatio` consistently or rely on parent-based; otherwise you'll see partial traces.

### Span tree

Every workflow execution produces a span tree like:

```
workflow.execute                          (root)
├── node.execute: Start
├── node.execute: Fetch Data
│   └── http.client                        (the inner HTTP call)
├── node.execute: IF (amount > 100)
├── node.execute: SendGrid
│   └── http.client
└── ai.generate                            (only if the AI assistant was invoked)
```

Each `node.execute` span carries attributes:

| Attribute | Description |
|---|---|
| `m9m.workflow.id` | Stable workflow id |
| `m9m.workflow.name` | Workflow display name |
| `m9m.execution.id` | Execution id |
| `m9m.node.id` | Node id |
| `m9m.node.name` | Node display name |
| `m9m.node.type` | `n8n-nodes-base.*` |
| `m9m.node.attempt` | Attempt number (1 for first try, 2+ for retries) |
| `m9m.node.duration_ms` | Wall-clock duration |
| `m9m.node.status` | `ok` / `error` / `skipped` |

The root `workflow.execute` span carries:

| Attribute | Description |
|---|---|
| `m9m.workflow.id` | Workflow id |
| `m9m.execution.id` | Execution id |
| `m9m.execution.mode` | `manual` / `webhook` / `scheduled` / `retry` / `retry-node` |
| `m9m.execution.status` | `running` / `completed` / `failed` / `cancelled` |
| `m9m.execution.started_at` | ISO 8601 |
| `m9m.execution.finished_at` | ISO 8601 (when done) |

### Resource attributes

Standard OTel resource attributes are set on the `Resource` and apply to every span:

| Attribute | Value |
|---|---|
| `service.name` | `m9m` (configurable via `OTEL_SERVICE_NAME`) |
| `service.version` | `<git-sha>` or `dev` (when run from source) |
| `service.instance.id` | Hostname + PID |
| `deployment.environment` | `M9M_ENV` or `production` / `development` |
| `host.name` | OS hostname |

### Health checks

`GET /healthz` includes the OTel pipeline status:

```json
{
  "status": "ok",
  "checks": {
    "otel_pipeline": {
      "status": "ok",
      "last_export": "2026-09-11T08:30:00Z",
      "errors_last_hour": 0,
      "exporter": "otlp/grpc"
    }
  }
}
```

`GET /health/detailed` adds per-check diagnostics (memory, queue depth, scheduler state, …).

### Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/otel` | Read effective config |
| `PUT` | `/api/v1/otel` | Write DB override |
| `PATCH` | `/api/v1/otel` | Partial update (same body) |
| `DELETE` | `/api/v1/otel` | Clear DB override |
| `POST` | `/api/v1/otel/test` | Emit one test span |

Details in [Telemetry API](../api/telemetry.md).

---

## Prometheus metrics

`GET /metrics` (default port 9090 when `M9M_METRICS_PORT` is set, otherwise the main port `/metrics`) returns the standard Prometheus exposition format.

Top-level metric groups:

| Group | Prefix | What it tracks |
|---|---|---|
| Workflow | `m9m_workflow_*` | Total, by status, duration histogram |
| Execution | `m9m_execution_*` | Total, by status, queue depth, per-node duration |
| Node | `m9m_node_*` | Per-node-type count and duration |
| Queue | `m9m_queue_*` | Pending, in-flight, DLQ size |
| HTTP | `m9m_http_*` | Per-endpoint request count and duration |
| AI | `m9m_ai_*` | Calls by provider, tokens, errors |
| Storage | `m9m_storage_*` | Per-backend latency |
| Scheduler | `m9m_scheduler_*` | Tick duration, missed ticks |

### Common dashboard queries

```promql
# 5xx rate over 5m
sum(rate(m9m_http_requests_total{status=~"5.."}[5m]))

# p95 workflow execution time
histogram_quantile(0.95,
  sum(rate(m9m_workflow_duration_seconds_bucket[5m])) by (le, workflow_name))

# DLQ growth
increase(m9m_queue_dlq_size[1h])
```

---

## Structured logs

Every log line is JSON on stdout:

```json
{
  "ts": "2026-09-11T08:30:00.123Z",
  "level": "info",
  "msg": "workflow executed",
  "workflow_id": "wf-123",
  "execution_id": "exec-790",
  "status": "completed",
  "duration_ms": 1234,
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
}
```

When an OpenTelemetry trace is active, `trace_id` and `span_id` are injected automatically — pipe stdout into any log aggregator that correlates by trace id.

Set `M9M_LOG_LEVEL=debug` for verbose output (per-node payload captures, expression resolution, etc.). Note that debug logs include workflow data — don't enable in production unless you have a reason.

---

## Health endpoints

| Path | Purpose |
|---|---|
| `GET /health` | Liveness — process up |
| `GET /healthz` | Readiness — pipeline, deps, OTel |
| `GET /ready` | Alias for `/healthz` |
| `GET /health/detailed` | Per-component diagnostics |
| `GET /api/v1/performance` | Throughput, latency, queue stats |

---

## Examples

### Self-hosted Grafana Tempo

```bash
# Run Tempo locally
docker run --rm -p 4317:4317 -p 4318:4318 \
  -v /tmp/tempo.yaml:/etc/tempo.yaml \
  grafana/tempo:latest \
  -config.file=/etc/tempo.yaml

# Configure m9m
export M9M_OTEL_ENABLED=true
export M9M_OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export M9M_OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
m9m serve
```

### Honeycomb

```bash
export M9M_OTEL_ENABLED=true
export M9M_OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export M9M_OTEL_EXPORTER_OTLP_ENDPOINT=api.honeycomb.io:443
export M9M_OTEL_EXPORTER_OTLP_HEADERS="x-honeycomb-team=YOUR_API_KEY"
m9m serve
```

### Datadog (via the Datadog Agent)

```bash
export M9M_OTEL_ENABLED=true
export M9M_OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export M9M_OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
m9m serve
```

The Datadog Agent (>=7.34) speaks OTLP natively; no extra config on the m9m side.

### See also

- [Telemetry API](../api/telemetry.md) — programmatic config
- [Configuration → Environment](../configuration/environment.md#telemetry) — full env-var matrix
