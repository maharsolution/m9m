---
title: "Environment Variables"
description: "All configuration options can be set via environment variables."
keywords: "m9m configuration, environment variables, server config, database config"
---

# Environment Variables

All configuration options can be set via environment variables.

## Naming Convention

Environment variables use the `M9M_` prefix with underscores:

```
config.yaml path → Environment variable
server.port     → M9M_SERVER_PORT
database.type   → M9M_DATABASE_TYPE
```

## Core Variables

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_HOST` | `0.0.0.0` | Server bind address |
| `M9M_PORT` | `8080` | Server port |
| `M9M_DEV_MODE` | `false` | Enable development mode |

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_DB_TYPE` | `sqlite` | Database type |
| `M9M_DB_PATH` | `~/.m9m/data/m9m.db` | SQLite path |
| `M9M_POSTGRES_URL` | - | PostgreSQL connection URL |
| `M9M_POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `M9M_POSTGRES_PORT` | `5432` | PostgreSQL port |
| `M9M_POSTGRES_DATABASE` | `m9m` | PostgreSQL database |
| `M9M_POSTGRES_USER` | `m9m` | PostgreSQL user |
| `M9M_POSTGRES_PASSWORD` | - | PostgreSQL password |

### Queue

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_QUEUE_TYPE` | `sqlite` | Queue type |
| `M9M_QUEUE_PATH` | `~/.m9m/data/queue.db` | Queue SQLite path |
| `M9M_MAX_WORKERS` | `4` | Worker threads |

### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_JWT_SECRET` | - | JWT signing secret |
| `M9M_JWT_EXPIRATION` | `24h` | JWT expiration |
| `M9M_ENCRYPTION_KEY` | - | Credential encryption key |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_LOG_LEVEL` | `info` | Log level |
| `M9M_LOG_FORMAT` | `json` | Log format (json, text) |

### Monitoring

| Variable | Default | Description |
|----------|---------|-------------|
| `M9M_METRICS_PORT` | `0` | Metrics port (0=disabled) |

## n8n Compatibility Variables

For compatibility with existing n8n configurations:

| Variable | Maps To |
|----------|---------|
| `N8N_ENCRYPTION_KEY` | `M9M_ENCRYPTION_KEY` |
| `N8N_PORT` | `M9M_PORT` |
| `N8N_HOST` | `M9M_HOST` |

## Usage Examples

### Basic Setup

```bash
export M9M_PORT=3000
export M9M_LOG_LEVEL=debug

m9m serve
```

### Production Setup

```bash
export M9M_PORT=8080
export M9M_DB_TYPE=postgres
export M9M_POSTGRES_URL="postgres://user:pass@db:5432/m9m?sslmode=require"
export M9M_QUEUE_TYPE=sqlite
export M9M_MAX_WORKERS=8
export M9M_JWT_SECRET="$(openssl rand -base64 32)"
export M9M_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export M9M_LOG_LEVEL=info

m9m serve
```

### Docker

```dockerfile
# Dockerfile
ENV M9M_PORT=8080
ENV M9M_DB_TYPE=sqlite
ENV M9M_DB_PATH=/data/m9m.db
```

```yaml
# docker-compose.yml
services:
  m9m:
    image: ghcr.io/maharsolution/m9m:latest
    environment:
      M9M_PORT: 8080
      M9M_DB_TYPE: postgres
      M9M_POSTGRES_URL: postgres://user:pass@db:5432/m9m
      M9M_JWT_SECRET: ${JWT_SECRET}
      M9M_ENCRYPTION_KEY: ${ENCRYPTION_KEY}
```

### Kubernetes

```yaml
# ConfigMap for non-sensitive values
apiVersion: v1
kind: ConfigMap
metadata:
  name: m9m-config
data:
  M9M_PORT: "8080"
  M9M_DB_TYPE: "postgres"
  M9M_LOG_LEVEL: "info"
  M9M_MAX_WORKERS: "8"
```

```yaml
# Secret for sensitive values
apiVersion: v1
kind: Secret
metadata:
  name: m9m-secrets
type: Opaque
stringData:
  M9M_JWT_SECRET: "your-jwt-secret"
  M9M_ENCRYPTION_KEY: "your-encryption-key"
  M9M_POSTGRES_URL: "postgres://..."
```

```yaml
# Deployment
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: m9m
          envFrom:
            - configMapRef:
                name: m9m-config
            - secretRef:
                name: m9m-secrets
```

## .env File

Create a `.env` file in your project:

```bash
# .env
M9M_PORT=8080
M9M_DB_TYPE=sqlite
M9M_LOG_LEVEL=debug
M9M_JWT_SECRET=development-secret-key-32-chars-min
M9M_ENCRYPTION_KEY=development-encryption-key-32chars
```

Load with:

```bash
source .env
m9m serve

# Or with direnv
# .envrc
dotenv
```

## Priority Order

Configuration is applied in this order (later overrides earlier):

1. Default values
2. Configuration file (`config.yaml`)
3. Environment variables
4. Command-line flags

Example:

```bash
# config.yaml has port: 8080
# Environment has M9M_PORT=3000
# CLI has --port 4000

# Result: port 4000 is used
```

## Viewing Configuration

See effective configuration:

```bash
m9m config show
```

Check a specific variable:

```bash
m9m config get database.type
```

## Troubleshooting

### Variable Not Applied

- Check spelling and prefix (`M9M_`)
- Verify export: `echo $M9M_PORT`
- Check override order (CLI > env > file)

### Secret in Logs

- Use `M9M_LOG_LEVEL=info` (not debug)
- Sensitive values are redacted in logs

---

## AI assistant

The in-app AI assistant (formerly "Copilot") reads `M9M_AI_*` env vars at boot. Any value set here can be overridden at runtime through **Settings → AI** or `PUT /api/v1/ai`.

| Variable | Default | Description |
|---|---|---|
| `M9M_AI_ENABLED` | `false` | Master on/off for the AI assistant |
| `M9M_AI_PROVIDER` | `openai` | `openai`, `anthropic`, `minimax`, `ollama` |
| `M9M_AI_MODEL` | provider-specific | Model name (e.g. `gpt-4o`, `claude-3-5-sonnet-latest`, `MiniMax-Plus`) |
| `M9M_AI_API_KEY` | — | Provider API key |
| `M9M_AI_BASE_URL` | provider-specific | Override the default API endpoint |

Example — Anthropic:

```bash
export M9M_AI_ENABLED=true
export M9M_AI_PROVIDER=anthropic
export M9M_AI_MODEL=claude-3-5-sonnet-latest
export M9M_AI_API_KEY=sk-ant-...
m9m serve
```

Example — MiniMax (OpenAI-compatible):

```bash
export M9M_AI_ENABLED=true
export M9M_AI_PROVIDER=minimax
export M9M_AI_MODEL=MiniMax-Plus
export M9M_AI_API_KEY=...
export M9M_AI_BASE_URL=https://api.minimax.chat/v1
m9m serve
```

### Legacy aliases (deprecated)

The legacy `M9M_COPILOT_*` env vars remain honoured as aliases of `M9M_AI_*`:

| Legacy | Current |
|---|---|
| `M9M_COPILOT_ENABLED` | `M9M_AI_ENABLED` |
| `M9M_COPILOT_PROVIDER` | `M9M_AI_PROVIDER` |
| `M9M_COPILOT_MODEL` | `M9M_AI_MODEL` |
| `M9M_COPILOT_API_KEY` | `M9M_AI_API_KEY` |
| `M9M_COPILOT_BASE_URL` | `M9M_AI_BASE_URL` |

These will be removed in a future major release. Migrate to the `M9M_AI_*` names.

---

## Telemetry

OpenTelemetry tracing configuration. Mirrors the upstream SDK env vars with m9m-specific overrides.

The m9m-side name is listed first; the standard `OTEL_*` SDK name is read as
a fallback when the m9m name is unset (the m9m name still wins when both
are set).

### Core tracing

| Variable | Default | Description |
|---|---|---|
| `M9M_OTEL_ENABLED` | `false` | Master on/off |
| `M9M_OTEL_EXPORTER_OTLP_PROTOCOL` (or `OTEL_EXPORTER_OTLP_PROTOCOL`) | `http/protobuf` | `grpc`, `http`, or `http/protobuf` |
| `M9M_OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_ENDPOINT`) | `http://localhost:4317` (gRPC) / `http://localhost:4318` (HTTP) | OTLP collector base URL; the exporter appends the protocol path |
| `M9M_OTEL_EXPORTER_OTLP_HEADERS` (or `OTEL_EXPORTER_OTLP_HEADERS`) | — | Comma-separated `key=value` pairs (e.g. `x-api-key=abc,foo=bar`) |
| `M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE` (or `OTEL_EXPORTER_OTLP_HEADERS_FILE`) | — | File with same `key=value` pairs; takes precedence over `HEADERS` when set |
| `M9M_OTEL_TRACES_SAMPLE_RATE` (or `OTEL_TRACES_SAMPLER_ARG`) | `1.0` (when enabled) | 0.0–1.0 trace ratio |
| `M9M_OTEL_TRACES_PRODUCTION_ONLY` | `true` | When `true`, non-production execution modes are sampled with `NeverSample` |
| `M9M_OTEL_TRACES_INCLUDE_NODE_SPANS` | `true` | When `false`, only `workflow.execute` spans are emitted (no per-node span) |
| `M9M_OTEL_TRACES_INJECT_OUTBOUND` | `true` | Whether the HTTP Request node injects a `traceparent` header on outbound calls |

### Resource attributes

| Variable | Default | Description |
|---|---|---|
| `M9M_OTEL_SERVICE_NAME` (or `OTEL_SERVICE_NAME`) | `m9m` | `service.name` on the OTel resource |
| `M9M_OTEL_SERVICE_VERSION` (or `OTEL_SERVICE_VERSION`) | `<git-sha>` (or `dev`) | `service.version` on the OTel resource |
| `M9M_OTEL_INSTANCE_ID` | `<hostname>:<pid>` | `service.instance.id` |
| `M9M_ENV` | `production` / `development` | `deployment.environment` on the OTel resource |

### Agents (GenAI) tracing

| Variable | Default | Description |
|---|---|---|
| `M9M_AGENTS_TRACING_ENABLED` | `false` | Toggle GenAI agent + tool-call spans |
| `M9M_AGENTS_TRACING_RECORD_INPUTS` | `true` | Whether prompts / tool args are recorded as `gen_ai.prompt` / `gen_ai.tool.call.arguments` |
| `M9M_AGENTS_TRACING_RECORD_OUTPUTS` | `true` | Whether completions / tool results are recorded as `gen_ai.completion` / `gen_ai.tool.call.result` |

A DB-stored override layer (set via `PUT /api/v1/otel`) wins over env. See [Observability](../observability/index.md) and [Telemetry API](../api/telemetry.md).

---

## Sync service (n8n ↔ m9m)

The bundled `sync-service/sync.py` reads these env vars. They are usually set in `docker-compose.yml`, not the host shell.

| Variable | Default | Description |
|---|---|---|
| `N8N_BASE_URL` | `http://n8n-designer-frontend:5678` | n8n REST API base URL |
| `N8N_API_KEY` | — | n8n API key |
| `M9M_BASE_URL` | `http://m9m-backend:8080` | m9m REST API base URL |
| `POLL_INTERVAL_SECONDS` | `30` | Background poll cadence |
| `SYNC_ONLY_ACTIVE` | `true` | Only push `active: true` workflows |
| `SYNC_PORT` | `8001` | Bridge's HTTP API listen port |
| `N8N_ENCRYPTION_KEY` | — | n8n's encryption key (also reads `GEN_ENCRYPTION_KEY`) |
| `N8N_PG_HOST` | `postgres-db` | n8n Postgres host |
| `N8N_PG_PORT` | `5432` | n8n Postgres port |
| `N8N_PG_USER` | `database_admin` | n8n Postgres user |
| `N8N_PG_PASSWORD` | `Secure_Db_Pass_2026` | n8n Postgres password (change in production) |
| `N8N_PG_DATABASE` | `n8n_prod` | n8n Postgres database |

Full reference: [n8n ↔ m9m sync bridge](../integrations/n8n-sync.md).

---

## See also

- [Configuration → Database](database.md)
- [Configuration → Queue](queue.md)
- [Configuration → Server](server.md)
- [Configuration → Security](security.md)
- [Observability](../observability/index.md)
- [AI Assistant](../nodes/ai.md#in-app-ai-assistant-settings--ai)
