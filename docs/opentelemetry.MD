# OpenTelemetry Integration — `m9m`

m9m emits OTel traces that mirror the n8n telemetry semantics:
`workflow.execute`, `node.execute`, `<agent>.generate` and `execute_tool <name>` spans,
plus the standard OpenTelemetry GenAI semantic conventions. Everything
is OTLP — there is no vendor SDK in the binary, so any OTel-compatible
collector works (Jaeger, SigNoz, Tempo, Honeycomb, Datadog, New Relic,
Grafana Cloud, etc.).

This document covers:

1. Configuration: environment variables and DB-backed overrides
2. Span model: what each span represents and which attributes it carries
3. Propagation: W3C trace context (inbound webhooks + outbound HTTP)
4. Custom attributes: per-project / per-workflow / per-node metadata
5. Verification: a backend-agnostic OTLP smoke test
6. Troubleshooting

---

## 1. Configuration

m9m reads the OTEL configuration in two layers:

| Layer | Where stored | Precedence |
| --- | --- | --- |
| Environment variables | process | defaults |
| Database override | `otel.config` row in the workflow storage (raw KV) | wins on non-nil fields |

The override is read on every tracer reload — reloading happens
when the operator edits the settings via the UI (`PUT /api/v1/otel`) or
when the API server starts.

### 1.1 Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `M9M_OTEL_ENABLED` | `false` | Master switch. When `false`, every helper on the OTel Manager returns a no-op span, so downstream code stays branch-free. |
| `M9M_OTEL_PROTOCOL` | `grpc` | `grpc` (OTLP gRPC) or `http/protobuf` (OTLP/HTTP). |
| `M9M_OTEL_ENDPOINT` | `localhost:4317` | OTLP collector URL. For HTTP use `http://host:4318/v1/traces`. For gRPC, `host:4317`. |
| `M9M_OTEL_HEADERS` | (unset) | Comma-separated `key=value` pairs added to every export request (e.g. `Authorization=Bearer <token>`). |
| `M9M_OTEL_HEADERS_FILE` | (unset) | Path to a JSON file with header dict (e.g. `{"x-honeycomb-team":"..."}`). Preferred for secret values. |
| `M9M_OTEL_SAMPLE_RATE` | `1.0` | `TraceIDRatioBased` ratio, `0.0 .. 1.0`. |
| `M9M_OTEL_PRODUCTION_ONLY` | `false` | When `true`, dev/test execution modes are dropped at sampling time even if their trace IDs are rolled in. |
| `M9M_OTEL_INCLUDE_NODE_SPANS` | `true` | Emit per-node `node.execute` spans. Disable to reduce span volume. |
| `M9M_OTEL_INJECT_OUTBOUND` | `true` | Inject the active traceparent into outbound HTTP requests emitted by the HTTP Request node. |
| `M9M_OTEL_SERVICE_NAME` | `m9m` | Replaces `service.name` in the resource. |
| `M9M_OTEL_SERVICE_VERSION` | (unset) | `service.version` attribute. |

### 1.2 Agent-specific settings

| Variable | Default | Description |
| --- | --- | --- |
| `M9M_AGENTS_TRACING_ENABLED` | `false` | Master switch for AI agent spans. Independent of `M9M_OTEL_ENABLED` so you can disable the AI traces without losing the workflow telemetry. |
| `M9M_AGENTS_TRACING_RECORD_INPUTS` | `false` | Allow `gen_ai.prompt` (prompt text) on `<agent>.generate` spans. |
| `M9M_AGENTS_TRACING_RECORD_OUTPUTS` | `false` | Allow `gen_ai.response` (response text) on `<agent>.generate` spans. |

> Privacy: leave the record-inputs / record-outputs flags off by
> default. PII flowing through AI workflows must never leave the
> operator's possession unchecked. The flags exist so GDPR-bound
> operators can opt in intentionally.

### 1.3 UI / DB override

The REST API exposes the same knobs the env vars expose, plus a
delete endpoint that restores env defaults.

```
GET    /api/v1/otel            # returns {env, override, effective}
PUT    /api/v1/otel            # body is the override; persists and reloads
DELETE /api/v1/otel            # clears the override
POST   /api/v1/otel/test       # emits a no-op `m9m.otel.test` span
```

The PUT body matches `ConfigOverride` (snake/camelCase tolerant):

```json
{
  "enabled": true,
  "protocol": "http/protobuf",
  "endpoint": "http://collector:4318/v1/traces",
  "headers": "Authorization=Bearer xyz",
  "sampleRate": 0.1,
  "includeNodeSpans": true,
  "injectOutbound": true,
  "productionOnly": false,
  "agentsEnabled": true,
  "agentsRecordInputs": false,
  "agentsRecordOutputs": false
}
```

A field that is omitted (or null) is interpreted as "use the env
default" — matching n8n's documented behaviour. Setting
`endpoint: ""` does NOT clear the field; it strips whitespace. Use
`DELETE /api/v1/otel` if you want a full reset.

---

## 2. Span model

### 2.1 Workflow spans

`workflow.execute` (kind: `server`)

| Attribute | Example | Source |
| --- | --- | --- |
| `m9m.workflow.id` | `wf_42` | `model.Workflow.ID` |
| `m9m.workflow.name` | `Order fulfillment` | `model.Workflow.Name` |
| `m9m.workflow.version.id` | `v123` | `model.Workflow.VersionID` |
| `m9m.workflow.node_count` | `7` | `len(workflow.Nodes)` |
| `m9m.project.id` | `marketing` | `model.Workflow.ProjectID` |
| `m9m.execution.id` | `exec_1736501234_42` | runtime execution id |
| `m9m.execution.mode` | `webhook` / `manual` / `trigger` / `retry` / `cli` / `schedule` | set by the caller |
| `m9m.execution.is_retry` | `true` / `false` | set by retry endpoint |
| `m9m.execution.retry_of` | `exec_...` | previous execution id |
| `m9m.workflow.custom.*` | `m9m.workflow.custom.team = data-eng` | from `model.Workflow.CustomSpanAttributes` |
| `service.name`, `service.version`, `m9m.instance.id`, `m9m.instance.role` | — | resource attributes |
| `status` | `success` / `error` / `cancelled` | set by `EndWorkflowSpan` |

### 2.2 Node spans

`node.execute` (kind: `internal`). Emitted only when
`includeNodeSpans` is true.

| Attribute | Source |
| --- | --- |
| `m9m.node.id`, `m9m.node.name`, `m9m.node.type`, `m9m.node.type_version` | `model.Node` |
| `m9m.node.items.input`, `m9m.node.items.output` | engine counts |
| `m9m.node.termination.reason` | set by the engine on failure / cancellation |
| `m9m.node.custom.*` | from `model.Node.CustomSpanAttributes` + programmatic `SpanMetadata()` |

### 2.3 Agent spans

`<agent-name>.generate` for synchronous calls, `<agent-name>.stream`
for streamed calls (kind: `internal`). Emitted only when
`M9M_AGENTS_TRACING_ENABLED` is true.

Standard GenAI semantic-convention attributes:

| Attribute | Example |
| --- | --- |
| `gen_ai.operation.name` | `invoke_agent` |
| `gen_ai.agent.name` | `openai`, `anthropic` |
| `gen_ai.request.model` | `openai/gpt-4o-mini`, `anthropic/claude-3.5-sonnet` |
| `gen_ai.conversation.id` | (n8n thread id when the workflow uses one) |
| `gen_ai.prompt` | prompt text (only when `recordInputs` is true) |
| `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` | from the API's `usage` block |

`execute_tool <tool-name>` (kind: `internal`). Emitted whenever the
agent makes a tool call (function calling). Attributes include
`tool.name`, `tool.call.id`, `tool.argument` (JSON), `tool.result`
(JSON), with the same record-inputs / record-outputs gating.

### 2.4 Links

| Link type | When emitted | `link.attributes` |
| --- | --- | --- |
| After Wait (resumed workflow) | `engine.WithContinuationSpan(ctx, prevSC)` before invoking the engine | `reason = "after_wait"` |
| Sub-workflow | The inner `workflow.execute` span automatically chains onto the outer one — no explicit link needed | — |

---

## 3. Trace context propagation

m9m uses the W3C trace-context (`traceparent`, `tracestate`) and
baggage headers via OTel's `propagation.TraceContext`. The global
propagator is set even when tracing is disabled, so the OTLP pipeline
can be turned on mid-run without reissuing outbound requests.

* **Inbound**: the webhook handler extracts `traceparent` from the
  inbound request and stamps it on the `context.Context` that is
  passed to the engine. The engine's `workflow.execute` span chains
  onto the caller's trace.
* **Outbound**: the HTTP Request node injects the active traceparent
  into the outbound `http.Request` so downstream services can
  continue the trace. Gated by `M9M_OTEL_INJECT_OUTBOUND`.
* **Sub-workflows**: invoked in-process via
  `engine.ExecuteWorkflowWithContext`. The inner `workflow.execute`
  span is a child of the outer one because the OTel SDK chains onto
  the SpanContext on ctx.
* **Async webhook runs**: detached from the request context (so the
  caller disconnect does not kill the run), but the trace context
  survives because we re-extract it from the stored request headers
  into the new background ctx.

---

## 4. Custom attributes

### 4.1 Static (per-workflow / per-node)

`model.Workflow` carries:

```go
type Workflow struct {
    ...
    ProjectID            string            `json:"projectId,omitempty"`
    CustomSpanAttributes map[string]string `json:"customSpanAttributes,omitempty"`
}
```

`model.Node` carries:

```go
type Node struct {
    ...
    CustomSpanAttributes map[string]interface{} `json:"customSpanAttributes,omitempty"`
}
```

These attributes show up under `m9m.workflow.custom.*` and
`m9m.node.custom.*`. Non-string keys are coerced to strings / bools /
ints at attachment time — see `internal/otel/attributes.go` for the
exact conversion rules.

### 4.2 Programmatic

Nodes that need to add attributes per-execution (rather than per
configuration) implement `base.MetadataAwareNodeExecutor`:

```go
type MetadataAwareNodeExecutor interface {
    NodeExecutor
    SpanMetadata() map[string]interface{}
}
```

The engine calls this right before `EndNodeSpan` and merges the
result with the static config — programmatic values win on key
collisions, mirroring n8n's documented behaviour.

---

## 5. OTLP smoke-test (backend-agnostic)

This test confirms the pipeline end-to-end without locking you into
any vendor's UI.

**Pre-flight checklist**

- [ ] m9m is built and on the `$PATH` of the host that will run it.
- [ ] A TCP-reachable OTLP collector is running on `<HOST>`:
      `<HOST>:4317` for gRPC, `<HOST>:4318` for HTTP.
- [ ] The collector accepts unauthenticated submissions (or you have
      an auth header you know about).

**Generate an env file** (e.g. `signoz.env`) — the exact pair
depends on your collector URL, not the vendor:

```bash
# OTLP over gRPC (default for most collectors)
export M9M_OTEL_ENABLED=true
export M9M_OTEL_PROTOCOL=grpc
export M9M_OTEL_ENDPOINT=<HOST>:4317
# If your collector requires auth:
# export M9M_OTEL_HEADERS="Authorization=Bearer <TOKEN>"

# OTLP over HTTP/protobuf (alternative)
# export M9M_OTEL_PROTOCOL=http/protobuf
# export M9M_OTEL_ENDPOINT=http://<HOST>:4318

# Reduce volume initially while sanity-checking
export M9M_OTEL_SAMPLE_RATE=1.0
export M9M_OTEL_INCLUDE_NODE_SPANS=true
export M9M_OTEL_PRODUCTION_ONLY=false

# AI agent tracing (optional)
export M9M_AGENTS_TRACING_ENABLED=true
export M9M_AGENTS_TRACING_RECORD_INPUTS=false
export M9M_AGENTS_TRACING_RECORD_OUTPUTS=false
```

**Boot m9m**

```bash
source signoz.env
./m9m serve --port 8080
```

Look for the boot line:

```
OpenTelemetry initialised: enabled=true protocol=grpc endpoint=<HOST>:4317
```

**Send a webhook that triggers a workflow**

The presence of the spans in your backend is the test result. Use
whatever your collector UI is — Jaeger's search box, SigNoz' "Services"
dropdown, Tempo's traceQL, etc. — and look for `service.name =
m9m` (or whatever you set `M9M_OTEL_SERVICE_NAME` to).

```bash
curl -X POST http://localhost:8080/webhook/test \
    -H 'content-type: application/json' \
    -H 'traceparent: 00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01' \
    -d '{"hello":"world"}'
```

Expected trace tree under `service.name = m9m`:

```
workflow.execute          (root or child of the inbound traceparent)
├── node.execute          (one per node)
├── node.execute
└── ...
```

**Disabled-tracing sanity check**

Override to disable via the API:

```bash
curl -X PUT http://localhost:8080/api/v1/otel \
    -H 'content-type: application/json' \
    -d '{"enabled": false}'
```

The next `workflow.execute` should NOT show up in the collector. The
relentless hot-reload is verified by then re-enabling with
`{"enabled": true}`.

---

## 6. Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| Boot log says "failed to initialise OTEL pipeline" | Wrong endpoint, unreachable host, or the OTel collector is on a different port than configured | Verify `nc -vz <HOST> 4317` (gRPC) or `nc -vz <HOST> 4318` (HTTP). Set `M9M_OTEL_ENDPOINT` accordingly. |
| Spans arrive but no `workflow.execute` root | Sampling dropped the root | Check `M9M_OTEL_SAMPLE_RATE`. A root drop also drops all children. |
| Spans arrive but no GenAI attributes on `<agent>.generate` | `M9M_AGENTS_TRACING_ENABLED` is `false` (default) | Set it to `true`. Restart the binary if you can't override via the UI. |
| Spans arrive but no input/output text on AI spans | `M9M_AGENTS_TRACING_RECORD_INPUTS` / `RECORD_OUTPUTS` are false (default) | Opt in for sampling under GDPR controls. |
| Outbound HTTP calls have no `traceparent` header | `M9M_OTEL_INJECT_OUTBOUND` is `false`, OR you used a node that doesn't propagate (e.g. raw `http_request` outside the `nodes/http/request.go` path) | Set the flag. Wrap outbound calls in `HTTPRequestNode` / `WebhookNode`. |
| `Enabled` field shows `false` after PUT | JSON omitted the field (`null` is fine, missing is a problem) | Re-PUT with `"enabled": true` in the body. |
| Old API clients break after upgrade | The legacy `internal/monitoring/tracing.go` was rewritten as a shim over `internal/otel` | Migrate to `internal/otel` directly. The shim is back-compat but limited — the new helpers (Reload, ConfigOverride, ConfigStore, Manager methods) only exist on the new package. |

---

## 7. Reference: the implementation

| File | Purpose |
| --- | --- |
| `internal/otel/otel.go` | `Manager` lifecycle (NewManager, Shutdown, Reload) |
| `internal/otel/env.go` | env → `Config` translator (`LoadConfigFromEnv`) |
| `internal/otel/config.go` | `ConfigOverride`, `EffectiveConfig(env, override)`, `ConfigView` |
| `internal/otel/store.go` | `ConfigStore` (raw KV persistence) |
| `internal/otel/api.go` | REST handler (`/api/v1/otel` GET/PUT/DELETE/TEST) |
| `internal/otel/exporter.go` | OTLP gRPC + http/protobuf exporter builders |
| `internal/otel/sampling.go` | `TraceIDRatioBased` wrapped with `productionOnlySampler` |
| `internal/otel/propagation.go` | W3C extractor / injector, `HeaderCarrier` |
| `internal/otel/resource.go` | `service.name`, `service.version`, `m9m.instance.id` |
| `internal/otel/attributes.go` | `WorkflowAttrs`, `NodeAttrs`, attribute key constants |
| `internal/otel/spans.go` | `StartWorkflowSpan`, `StartNodeSpan`, `StartAgentSpan`, `StartToolSpan`, link helpers |
| `internal/otel/tls.go` | `*tls.Config` → `credentials.TransportCredentials` for gRPC mTLS |
| `internal/engine/tracing_ctx.go` | ctx helpers (`WithExecutionMode`, `WithExecutionID`, `WithContinuationSpan`) |
| `internal/nodes/base/node.go` | `MetadataAwareNodeExecutor` interface |
| `internal/monitoring/tracing.go` | back-compat shim over `internal/otel` |
| `internal/api/server.go` + `routes.go` | `/api/v1/otel` registration + `SetOTelManager` |
| `cmd/m9m/commands/serve.go` | bootstrap (env → DB → Manager → engine → AI nodes → API) |
