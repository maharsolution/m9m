---
title: "Architecture Overview"
description: "Understanding m9m's architecture and design."
keywords: "m9m architecture, workflow engine, job queue, Go runtime"
---

# Architecture Overview

Understanding m9m's architecture and design.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Clients                             │
│  (CLI, REST API, Webhooks, Schedules, MCP, n8n sync bridge) │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                      REST API Layer                         │
│  ┌─────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐  │
│  │Auth     │  │Workflows │  │Executions│  │Webhooks     │  │
│  │Middleware│ │Handlers  │  │Handlers  │  │Handlers     │  │
│  └─────────┘  └──────────┘  └──────────┘  └─────────────┘  │
│  ┌─────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐  │
│  │AI       │  │DLQ       │  │Telemetry │  │NodeTypes    │  │
│  │Assistant│  │Inspect   │  │Config    │  │Introspection│  │
│  └─────────┘  └──────────┘  └──────────┘  └─────────────┘  │
└─────────────────────┬───────────────────────────────────────┘
                      │
        ┌─────────────┼─────────────────────┐
        ▼             ▼                     ▼
┌────────────────┐ ┌─────────────────────────┐ ┌──────────────┐
│  Workflow      │ │  Cross-cutting services │ │  MCP server  │
│  Engine        │ │  ┌──────────────────┐  │ │  (AI agents) │
│  (orchestrator,│ │  │ AI Runtime       │  │ │  37 tools    │
│   expression,  │ │  │ (OpenAI/         │  │ └──────────────┘
│   credentials, │ │  │  Anthropic/      │  │
│   node         │ │  │  MiniMax/        │  │
│   registry,    │ │  │  Ollama)         │  │
│   data flow)   │ │  └──────────────────┘  │
│                │ │  ┌──────────────────┐  │
│                │ │  │ OpenTelemetry    │  │
│                │ │  │ pipeline         │  │
│                │ │  └──────────────────┘  │
│                │ │  ┌──────────────────┐  │
│                │ │  │ DLQ              │  │
│                │ │  │ (terminal-fail   │  │
│                │ │  │  holding pen)    │  │
│                │ │  └──────────────────┘  │
└───────┬────────┘ └─────────────────────────┘
        │
        ▼
┌─────────────────┐     ┌─────────────────┐
│   Job Queue     │     │   Storage       │
│  ┌───────────┐  │     │  ┌───────────┐  │
│  │  Memory   │  │     │  │  SQLite   │  │
│  │  SQLite   │  │     │  │  Postgres │  │
│  │  Redis    │  │     │  │  Memory   │  │
│  │  RabbitMQ │  │     │  └───────────┘  │
│  └───────────┘  │     └─────────────────┘
└─────────────────┘

  Sidecar: sync-service/sync.py (n8n ↔ m9m bridge)
    - polls N8N_BASE_URL on POLL_INTERVAL_SECONDS
    - decrypts credentials from N8N_PG_*
    - pushes workflows + credentials into M9M_BASE_URL
```

## Core Components

### REST API Layer

Handles all incoming HTTP requests:

- **Authentication middleware** - JWT/API key validation
- **Rate limiting** - Prevents abuse
- **Request routing** - Maps endpoints to handlers
- **Response formatting** - Consistent JSON responses

### Workflow Engine

The heart of m9m:

| Component | Responsibility |
|-----------|----------------|
| Orchestrator | Manages workflow execution flow |
| Expression Evaluator | Processes `{{ }}` expressions |
| Node Registry | Stores available node types |
| Data Transformer | Handles data flow between nodes |
| Credential Manager | Securely provides credentials |
| Error Handler | Manages failures and retries |

### Node System

Nodes are pluggable execution units:

```go
type NodeExecutor interface {
    Execute(inputData []DataItem, params map[string]interface{}) ([]DataItem, error)
    Description() NodeDescription
    ValidateParameters(params map[string]interface{}) error
}
```

### Job Queue

Manages asynchronous workflow execution:

- **Memory** - In-process queue for single instance
- **SQLite** - Persistent queue for single instance
- **Redis** - Distributed queue for multiple instances

### Storage Layer

Persists workflows and executions:

- **SQLite** - Default, file-based
- **PostgreSQL** - Production, scalable

## Data Flow

### Workflow Execution Flow

```
1. Trigger (Manual/Webhook/Cron)
       │
       ▼
2. Load Workflow Definition
       │
       ▼
3. Create Execution Context
       │
       ▼
4. Queue Job (if async)
       │
       ▼
5. Execute Nodes (topological order)
       │
       ▼
6. For each node:
   a. Resolve expressions
   b. Get credentials
   c. Execute node logic
   d. Transform output
   e. Pass to next nodes
       │
       ▼
7. Store Execution Result
       │
       ▼
8. Return Response
```

### Data Item Structure

```json
{
  "json": {
    "field1": "value1",
    "field2": 123
  },
  "binary": {
    "file": {
      "data": "base64...",
      "mimeType": "application/pdf",
      "fileName": "document.pdf"
    }
  }
}
```

## Design Principles

### Performance First

- **Compiled language** - Go provides native performance
- **Minimal allocations** - Object pooling, efficient data structures
- **Concurrent execution** - Goroutines for parallel processing
- **Connection pooling** - Efficient database/HTTP connections

### Cloud Native

- **Stateless** - Horizontal scaling
- **12-factor app** - Environment-based configuration
- **Container ready** - Docker/Kubernetes native
- **Observable** - Prometheus metrics, structured logging

### n8n Compatible

- **Workflow format** - Same JSON structure
- **Expression syntax** - Same `{{ }}` expressions
- **Node types** - Compatible node identifiers
- **Seamless migration** - Import n8n workflows directly

## Module Structure

```
m9m/
├── cmd/
│   └── m9m/           # Application entry point
├── internal/
│   ├── api/           # REST API handlers (workflows, executions, AI, DLQ, telemetry, …)
│   ├── engine/        # Workflow execution engine (orchestrator, EdgesTaken, recursion)
│   ├── nodes/         # Node implementations
│   │   ├── base/      # Base interfaces
│   │   ├── transform/ # Data transformation
│   │   ├── http/      # HTTP requests
│   │   ├── database/  # Postgres, MySQL, SQLite, Mongo, Redis, ES
│   │   ├── messaging/ # Slack, Discord, Twilio, Teams
│   │   ├── ai/        # OpenAI, Anthropic workflow nodes
│   │   ├── trigger/   # Webhook, cron, error, respondToWebhook
│   │   ├── core/      # Start, noOp, wait, executeWorkflow
│   │   ├── code/      # Python sandbox
│   │   ├── cli/       # Execute Command (bubblewrap)
│   │   ├── cloud/     # AWS, Azure, GCP
│   │   ├── email/     # SMTP, SendGrid
│   │   ├── file/      # Read/write binary
│   │   ├── productivity/  # Google Sheets, Notion, Stripe
│   │   ├── timer/     # Cron
│   │   └── vcs/       # GitHub, GitLab
│   ├── ai/            # In-app AI assistant runtime (OpenAI/Anthropic/MiniMax/Ollama)
│   ├── otel/          # OpenTelemetry pipeline (env-default + DB-override + UI)
│   ├── dlq/           # Dead-letter queue (terminal-failure holding pen)
│   ├── mcp/           # MCP server for AI agents (37 tools)
│   ├── queue/         # Job queue implementations (memory/sqlite/redis/rabbitmq)
│   ├── storage/       # Data persistence (memory/sqlite/postgres)
│   ├── credentials/   # Credential management (AES-256-GCM at rest)
│   ├── expressions/   # Expression evaluation (Goja)
│   ├── monitoring/    # Prometheus metrics
│   ├── scheduler/     # Distributed cron-like scheduler
│   ├── audit/         # Audit log
│   ├── reliability/   # Circuit breakers, retry policies
│   ├── sandbox/       # bubblewrap wrappers for code/CLI/AI agents
│   ├── variables/     # Workflow variables
│   ├── versioning/    # Git-based workflow versioning
│   ├── workspaces/    # Multi-tenant workspace isolation
│   ├── plugins/       # Out-of-tree plugin loader
│   ├── consensus/     # Distributed leader election (Raft-backed)
│   ├── connections/   # Connection multiplexing
│   ├── service/       # Long-running service supervisor
│   ├── tags/          # Workflow tags
│   ├── templates/     # Workflow templates
│   ├── tenancy/       # Multi-tenancy primitives
│   ├── web/           # Web UI handler
│   ├── webhooks/      # Webhook ingress
│   ├── worker/        # Worker pool coordinator
│   └── showtime/      # Reference data for the UI
├── sync-service/      # n8n ↔ m9m bridge (FastAPI/uvicorn)
└── docs/              # Documentation
```

## Scalability

### Horizontal Scaling

```
┌────────────────────────────────────────┐
│            Load Balancer               │
└──────┬─────────────┬─────────────┬─────┘
       │             │             │
       ▼             ▼             ▼
   ┌───────┐     ┌───────┐     ┌───────┐
   │ m9m-1 │     │ m9m-2 │     │ m9m-3 │
   └───┬───┘     └───┬───┘     └───┬───┘
       │             │             │
       └─────────────┼─────────────┘
                     │
                     ▼
              ┌─────────────┐
              │    Redis    │ (Job Queue)
              └─────────────┘
                     │
                     ▼
              ┌─────────────┐
              │  PostgreSQL │ (Storage)
              └─────────────┘
```

### Worker Scaling

Configure worker count based on workload:

```yaml
queue:
  workers: 10        # Number of concurrent workers
  maxRetries: 3      # Retry failed jobs
  retryDelay: 5s     # Delay between retries
```

## Security Architecture

### Authentication Flow

```
Client → JWT/API Key → Auth Middleware → Protected Resource
```

### Credential Security

```
Plaintext → AES-256 Encryption → Storage
Storage → Decryption → Node Execution → Clear from Memory
```

### Network Security

- TLS for all external connections
- Private network for internal services
- Rate limiting on API endpoints

## Next Steps

- [Workflow Engine](engine.md) - Deep dive into execution
- [Job Queue](queue.md) - Queue system details
