---
title: "Telemetry API"
description: "Inspect and override the OpenTelemetry configuration that the running m9m process uses."
keywords: "m9m API, REST API, OpenTelemetry, tracing, OTLP, observability"
---

# Telemetry API

Read and update the OpenTelemetry configuration that the running m9m process uses for distributed tracing. This endpoint is the live config surface; the underlying defaults come from env vars, and a DB-stored override layer sits between env and runtime.

For the user-facing guide (exporters, sampling, span tree, env vars), see [Observability](../observability/index.md).

---

## Get the Effective Telemetry Config

Returns the **effective** config — what the runtime is actually using right now, after merging env defaults with any DB-stored override.

```http
GET /api/v1/otel
```

### Example Request

```bash
curl http://localhost:8080/api/v1/otel \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "enabled": true,
  "protocol": "grpc",
  "endpoint": "otel-collector.observability.svc.cluster.local:4317",
  "headers": {
    "x-api-key": "***"
  },
  "sampleRatio": 1.0,
  "serviceName": "m9m",
  "serviceVersion": "0.2.0-parity-2026-09-10-abcdef0",
  "source": "db_override"
}
```

`source` is one of:

| Value | Meaning |
|---|---|
| `defaults` | No env or DB override; running on built-in defaults (off) |
| `env` | Built-in defaults overridden by env vars |
| `db_override` | Effective config came from the DB-stored override |

`headers` are masked; pass the request back through `PUT /api/v1/otel` to keep them, or omit the field to clear them.

---

## Update the Telemetry Config

Writes the supplied fields to the DB-stored override. On the next read, the runtime merges this with env defaults and returns the effective config.

```http
PUT /api/v1/otel
```

Or `PATCH /api/v1/otel` for partial updates (same semantics; body is a partial).

### Request Body

| Field | Type | Required | Description |
|---|---|---|---|
| `enabled` | boolean | No | Live on/off toggle |
| `protocol` | string | No | `grpc` or `http` |
| `endpoint` | string | No | OTLP endpoint (host:port for `grpc`, full URL for `http`) |
| `headers` | object | No | Headers for the OTLP exporter, e.g. `{"x-api-key": "..."}` |
| `sampleRatio` | number | No | 0.0–1.0, or omit to use parent-based sampling |

### Example Request — enable OTLP gRPC

```bash
curl -X PUT http://localhost:8080/api/v1/otel \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled":      true,
    "protocol":     "grpc",
    "endpoint":     "otel-collector:4317",
    "headers":      {"x-api-key": "abc123"},
    "sampleRatio":  1.0
  }'
```

### Response

The same shape as `GET /api/v1/otel`, reflecting the new effective config.

---

## Clear the DB Override

Reverts to the env-merged defaults. Useful when a bad config has been saved.

```http
DELETE /api/v1/otel
```

### Example Request

```bash
curl -X DELETE http://localhost:8080/api/v1/otel \
  -H "Authorization: Bearer <token>"
```

### Response

```json
{
  "enabled": false,
  "protocol": "grpc",
  "endpoint": "",
  "headers": {},
  "sampleRatio": 0.0,
  "serviceName": "m9m",
  "serviceVersion": "0.2.0-parity-2026-09-10-abcdef0",
  "source": "defaults"
}
```

---

## Emit a Test Span

Forces one synthetic span through the configured exporter so you can confirm wiring without running a real workflow.

```http
POST /api/v1/otel/test
```

### Request Body

| Field | Type | Required | Description |
|---|---|---|---|
| `message` | string | No | Body of the test span (default `m9m otel test`) |

### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/otel/test \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"message": "hello from manual test"}'
```

### Response

```json
{
  "emitted": true,
  "traceId": "4bf92f3577b34da6a3ce929d0e0e4736",
  "spanId":  "00f067aa0ba902b7",
  "endpoint": "otel-collector:4317"
}
```

### 409 — telemetry is disabled

```json
{ "error": true, "message": "Telemetry is disabled. Enable it via PUT /api/v1/otel first.", "code": 409 }
```

---

## Layered configuration (priority order)

When the runtime reads the config, the layers merge in this order, **last wins**:

1. **Built-in defaults** — telemetry is off (`Enabled=false`, `Protocol=http/protobuf`).
2. **Standard `OTEL_*` SDK env vars** — `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER`, etc. Honoured as a fallback.
3. **`M9M_OTEL_*` env vars** — `M9M_OTEL_ENABLED`, `M9M_OTEL_EXPORTER_OTLP_ENDPOINT`, `M9M_OTEL_EXPORTER_OTLP_PROTOCOL`, `M9M_OTEL_EXPORTER_OTLP_HEADERS`, `M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE`, `M9M_OTEL_TRACES_SAMPLE_RATE`, `M9M_OTEL_TRACES_PRODUCTION_ONLY`, `M9M_OTEL_TRACES_INCLUDE_NODE_SPANS`, `M9M_OTEL_TRACES_INJECT_OUTBOUND`, `M9M_OTEL_SERVICE_NAME`, `M9M_OTEL_SERVICE_VERSION`, `M9M_OTEL_INSTANCE_ID`. The `M9M_AGENTS_TRACING_*` family controls GenAI spans. `M9M_OTEL_*` wins over the bare `OTEL_*` names when both are set.
4. **DB-stored override** — what you save via `PUT /api/v1/otel`. Persists across restarts.
4. **Live UI toggle** — the master on/off switch in Settings.

The `/healthz` endpoint reports whether the OTel pipeline is healthy (exporter connected, no recent errors).

### Related

- [Observability guide](../observability/index.md)
- [Environment variables](../configuration/environment.md#telemetry)
