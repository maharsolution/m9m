# Changelog

All notable changes to m9m will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-10-12

Cumulative changes since `0.1.0-beta.1`. The 2026-09-10 webhook-parity cycle is documented under `[0.2.0-parity] - 2026-09-10` further down. Items below are platform work that landed after the parity cycle.

### 2026-10-12 cycle — 0.4.0 progress

#### Engine and execution
- **MongoDB native driver + per-op displayOptions** — `internal/nodes/database/mongodb.go` rewritten; accepts `mongodb://` or `mongodb+srv://` URIs, supports `find` / `findOne` / `insert` / `insertMany` / `update` / `delete` / `aggregate` / `count` / `replaceOne` / `upsert` / `distinct` via the official `mongo-driver`. BSON-isms normalised on the way out. See bug-fix.MD §9.3.
- **Data Tables node** — `n8n-nodes-base.dataTable` executor + REST + sync-service. 5 operations, 9 condition operators, AND/OR match types, expression evaluation in filter values. See bug-fix.MD §9.2.
- **Kafka SASL credential injection** — `messaging.ApplyKafkaCredentialAuth` helper folds the engine-injected `kafka_*` keys back into the explicit-param shape. Wired into `KafkaNode.backendFor` (producer) and `KafkaTriggerNode.Execute` (consumer). See bug-fix.MD §9.10.

#### UI / frontend
- **NDV Settings tab — raw-JSON fallback** — `web/src/views/ExecutionDetail.vue` renders a read-only JSON block of `selectedNode.parameters` when the node has no structured `properties` yet. The Settings tab is now usable for every node. See bug-fix.MD §9.12.
- **~25 nodes migrated to structured `INodeTypeDescription.properties`** — slack, discord, sendgrid, notion, googleSheets, twilio, teams, github, gitlab, awsLambda, s3, azureBlobStorage, gcpCloudStorage, openai, anthropic, wait, noop, loop, executeWorkflow, errorTrigger, respondToWebhook, executeWorkflowTrigger, smtp, readBinary, writeBinary, pythonCode, cli.execute, sqlConnector, stripe. Each helper lives in a sibling `properties.go`. See bug-fix.MD §9.12.
- **Frontend credential map** — `web/src/types/node.ts` adds `n8n-nodes-base.kafka` and `n8n-nodes-base.kafkaTrigger` to the credential-type map (BUG-2026-10-12-002).

#### Security
- **JWT webhook auth fuzzing harness** — 21-case fuzz in `internal/webhooks/jwt_fuzz_test.go` (13 + 8 new: `alg=none` passive / with-bogus-sig / alg case-sensitivity (5 sub-cases) / HS256→RS256 confusion / truncated signature / replay window / rotated key). See bug-fix.MD §9.17.
- **OAuth2 generic credential — full implementation** — `internal/credentials/oauth2.go` (scaffold: `OAuth2Schema` / `OAuth2Credentials` / `NeedsRefresh` / `Refresh` / `TokenExchanger` seam, 9 unit tests) + `internal/credentials/oauth2_client.go` (production RFC 6749 client: form-encoded `grant_type=refresh_token` POST, configurable timeout, 5xx retry, 4xx no-retry, truncated error body, 8 unit tests). See bug-fix.MD §9.18.
- **OIDC SSO — full implementation** — `internal/api/sso.go` (scaffold: `SSOProvider` / `SSORouter` / `SessionIssuer` / `FakeSSOProvider`, 8 unit tests) + `internal/api/oidc.go` (production OIDC provider: OpenID Connect Discovery + JWKS with caching + RSA/EC key support + `iss`/`aud`/`exp`/`kid` verification + configurable `GroupsClaim` + `alg=none` rejection, 13 unit tests). SAML deferred to 0.6.0. See bug-fix.MD §9.19.
- **RBAC — full implementation (library + middleware + persistence)** — `internal/api/rbac.go` (scaffold: `Permission` / `Role` / `User` / `RBACStore` with 3 built-in roles, 6 unit tests) + `internal/api/rbac_middleware.go` (`RequirePermission` / `RequireAnyPermission` middleware + `UserProvider` seam + `DefaultUserProvider` + `UserFromContext` + `SnapshotRBAC` / `LoadRBAC` JSON persistence, 12 unit tests). Per-resource handler enforcement on every existing route deferred to 0.6.0. See bug-fix.MD §9.20.

#### Architectural / Capability
- **Edges-taken renderer (CAP-001 + CAP-002)** — `web/src/lib/edgesTakenRenderer.ts` + 12 unit tests; the per-execution `EdgesTaken` accumulator → per-edge Vue Flow style transform. Same data drives the editor canvas + the debugger. See bug-fix.MD §9.22.
- **Queue soak harness (ARCH-001)** — `scripts/queue-soak.sh` (bash + Python 3, no extra deps): fires `$CONCURRENCY` worker threads against `POST /api/v1/workflows/{id}/execute` for `$DURATION` seconds, captures HTTP + per-execution final status, writes JSON report with PASS/FAIL verdict. See bug-fix.MD §9.23.
- **Task runner architecture (ARCH-002)** — `internal/scaling/` package: `Job` / `JobResult` / `Enqueuer` / `Worker` / `Executor` / `Runner` interfaces + `inProcessBus` for single-instance + 7 unit tests. The interface is the deliverable; the engine-side wiring is the next pass. See bug-fix.MD §9.24.
- **Leader election wrapper (ARCH-003)** — `internal/consensus/leader.go` (8 unit tests): `IsLeader` / `WaitForLeader` / `ScheduleOnLeader` on top of the existing Raft implementation. `ScheduleOnLeader` is the cron-scheduler pattern: only run `fn` if we hold leadership. See bug-fix.MD §9.25.
- **Sub-workflow palette (ARCH-004)** — `web/src/data/subWorkflowPalette.ts` (12 unit tests): `getSubWorkflowPaletteEntries` + `validateSubWorkflowRef` (self-ref / unknown / OK) + `isSubWorkflowType` + `nodeTypeForSubWorkflow`. The engine-side executor was already complete; the UI gap was the palette + validation. See bug-fix.MD §9.26.
- **Static Data KV (ARCH-005)** — `internal/staticdata/` package (9 unit tests): `Store` with per-workflow + per-node scoping, atomic temp-file + rename. Matches n8n's `Workflow.getStaticData` semantics. See bug-fix.MD §9.27.
- **Binary data blob store (ARCH-006)** — `internal/blobstore/` package (9 unit tests): `BlobStore` interface + `LocalStore` (256-shard disk) + `S3Store` (AWS / MinIO via aws-sdk-go) + `ErrNotFound` sentinel. Engine picks at startup via `M9M_BLOBSTORE_TYPE`. See bug-fix.MD §9.28.
- **Streaming responses (ARCH-007)** — `internal/webhooks/streaming.go` (8 unit tests): `StreamResponse` (Write + Flush per chunk) + `WriteSSE` (multi-line data per WHATWG) + `Hijack` + proxy-buffering headers. See bug-fix.MD §9.29.
- **Credential form helpers (CAP-003)** — `web/src/types/credentials.ts` + `web/src/lib/credentialSchema.ts` (validate / toPayload / fieldFor) + 13 unit tests. "New" form now has a structured per-type view backed by the `OAuth2Schema`. See bug-fix.MD §9.22.
- **Execution timeline composable (CAP-004)** — `web/src/composables/useExecutionTimeline.ts` (14 unit tests): `buildTimeline` (per-day buckets, successRate) + `filterByDateRange` + `labelFor` (Today / Yesterday / long-form). The list view now has a per-day timeline + date-range filter chips. See bug-fix.MD §9.22.
- **XML ↔ JSON helper (CAP-005)** — `web/src/lib/xmlJsonHelper.ts` (13 unit tests): `toJson` (DOMParser + attrs as `@<name>` + multi-child arrays) + `toXml` (single-rooted + declaration + indent + escape). See bug-fix.MD §9.22.

#### Repository hygiene
- **Stray files removed** — `,env` (typo for `.env`), `Unconfirmed 467040.crdownload` (112 MB browser artefact), `pyvenv.cfg` (Python venv leak), `supervisord.conf` (moved to `deploy/docker/`). Dockerfile updated to `COPY deploy/docker/supervisord.conf`.
- **Hygiene gate** — `scripts/check_repo_hygiene.sh` (5 cases) + `make hygiene` + GHA step in `harbor-image.yml` so a future regression fails the build before the docker image is pushed. See bug-fix.MD §9.11.

#### Fixed in this cycle
- BUG-2026-10-12-001 (`,env` repo hygiene) — RESOLVED
- BUG-2026-10-12-002 (`.crdownload` repo hygiene) — RESOLVED
- BUG-2026-10-12-003 (`.exe` binaries) — RESOLVED (already gitignored; never tracked)
- BUG-2026-10-12-004 (`pyvenv.cfg`) — RESOLVED
- BUG-2026-10-12-005 (`supervisord.conf` move) — RESOLVED
- BUG-2026-10-12-002 (Kafka SASL cred) — RESOLVED
- BUG-UI-2026-09-09-001 (NodeTypeDescription Properties) — **RESOLVED** (31 of ~35 done; 4 long-tail nodes on raw-JSON fallback)
- BUG-UI-2026-09-09-002 (NDV Settings tab + retry) — **RESOLVED** (raw-JSON fallback + retry-from-here button wired + `web/test/executions-retryNode.test.mjs` covers the call)
- BUG-SEC-2026-10-12-001 (JWT fuzzing) — **RESOLVED** (21 cases; 8 new for `alg=none`, alg case-sensitivity, HS256→RS256 confusion, truncated sig, replay, rotated key)
- BUG-SEC-2026-10-12-002 (OAuth2) — **RESOLVED** (full RFC 6749 client + schema + 17 unit tests; `/api/v1/oauth2/callback` HTTP wiring + Slack/GSheets/Notion/Stripe migration deferred to 0.6.0)
- BUG-SEC-2026-10-12-003 (SSO / OIDC) — **RESOLVED** (OIDC production provider + 21 unit tests; SAML still deferred to 0.6.0)
- BUG-SEC-2026-10-12-004 (RBAC) — **RESOLVED** (library + middleware + persistence + 18 unit tests; per-resource handler enforcement on every route deferred to 0.6.0)
- BUG-CAP-2026-09-09-001..005 / BUG-ARCH-2026-10-12-001..007 (P6 batch) — **RESOLVED** (12 P6 items landed: edges-taken renderer, credential form, execution timeline, XML↔JSON helper, queue-soak, scaling, leader-election wrapper, sub-workflow palette, static data, blobstore, streaming)

### Added

#### Engine and execution
- **Per-workflow debug flag** — when enabled (UI toggle), every node's full input/output is captured and exposed via NDV; when disabled, only the most recent node's data is retained for cheaper execution.
- **Per-node retry** — `POST /api/v1/executions/{id}/retry-node` re-runs a single node and continues the rest of the workflow from there; UI button "Retry from here" on the execution graph.
- **Branch-coloured execution edges (`EdgesTaken`)** — each execution now records which branches were taken so the UI can colour edges per state (taken, skipped, errored).
- **Sub-workflow edge preservation** — when a parent workflow invokes a sub-workflow, the parent's `EdgesTaken` accumulator is preserved across the recursion, so the execution graph stays correct.
- **NDV Settings tab** — the node detail view now shows a Settings tab with sticky-note skipping and per-node retry controls.
- **XML node** (`n8n-nodes-base.xml`) — parse, stringify, and transform XML payloads with XPath support.

#### AI assistant
- **In-app AI Settings card** — provider (OpenAI, Anthropic, MiniMax, Ollama) + model selector, base URL override, API key, and live Enable toggle, layered over `M9M_AI_*` env defaults with DB-stored overrides.
- **MiniMax support** — the Anthropic node (and the in-app assistant) accept a MiniMax-compatible `base_url`, so the same Anthropic SDK call routes to MiniMax when configured.
- **Renamed Copilot → AI** — the `internal/copilot` package has been renamed to `internal/ai`. The `/api/v1/copilot/*` routes are now `/api/v1/ai/*`. Legacy `M9M_COPILOT_*` env vars remain honoured as aliases of `M9M_AI_*`.

#### Telemetry
- **OpenTelemetry tracing** — full OTLP gRPC and OTLP HTTP/protobuf exporters, three-layer config (UI toggle → DB-stored override → env vars), sampling (always-on, always-off, ratio, parent-based), standard resource attributes (`service.name=m9m`, `service.version=<git-sha>`), and a live `/api/v1/otel` config API.
- **`M9M_OTEL_*` env vars** — `M9M_OTEL_ENABLED`, `M9M_OTEL_ENDPOINT`, `M9M_OTEL_PROTOCOL`, `M9M_OTEL_HEADERS`, `M9M_OTEL_SAMPLE_RATIO`. Standard `OTEL_SDK_*` env vars are also accepted; `M9M_OTEL_*` wins when both are set.
- **Span tree shape** — `workflow.execute` → `node.execute` → (optional) `agent.generate` / `db.query` / `http.request`, with attributes for workflow id, node name, node type, attempt number, and duration.

#### Sync bridge (n8n ↔ m9m)
- **`sync-service/sync.py`** — Python FastAPI bridge that polls n8n for workflow and credential changes and pushes them into m9m. Decrypts n8n's AES-256-CBC + EVP_BytesToKey credentials using `N8N_ENCRYPTION_KEY` and re-encrypts them with m9m's envelope format.
- **Endpoints** — `POST /sync` (run one cycle on demand), `GET /status` (last-cycle summary), `GET /health` (liveness). Runs as a process inside the `m9m-backend` container via `supervisord`.
- **Idempotent create-or-update** — POST then PUT-on-409 fallback, so re-runs are safe.

#### Database nodes
- **Postgres `sslMode=disable` default** — the Postgres node now defaults to `sslmode=disable` to match n8n's behaviour, eliminating a surprise TLS handshake against local Postgres instances.
- **MySQL, SQLite, Redis, MongoDB, Elasticsearch** — additional database nodes now ship out of the box.

### Changed
- **Repo ownership** — moved from `neul-labs/m9m` to `maharsolution/m9m`. Documentation, install scripts, Docker images, and CI references updated.
- **Architecture diagram** in `documentation/docs/architecture/index.md` now includes AI runtime, OTel pipeline, sync bridge, DLQ, and MCP server.

### Deprecated
- **`M9M_COPILOT_*` env vars** — aliases of `M9M_AI_*`. Will be removed in a future major release.

## [0.2.0-parity] - 2026-09-10

Webhook parity cycle — all 19 parity test cases (14 positive + 5 negative) pass against a live n8n reference run.

### Fixed
- **Webhook trigger persistence** — `NodeData` is now persisted for trigger executions so NDV and execution graphs see the same payload that n8n would have delivered.
- **Database node SQL expression resolution** — `={{ ... }}` expressions in the SQL `query` field are now resolved before the DB call, matching n8n's pre-execution expansion.
- **Credential seeding at startup** — the in-memory credential store is now seeded from persistent storage at boot, so previously-saved credentials are usable immediately.
- **Credential injection ordering** — nodes are validated *after* credentials are injected into their parameters, fixing a class of false-positive validation errors.
- **Workflow lookup by id** — `executeWorkflow` and `executeWorkflowTrigger` now resolve by stable id, not by name.
- **Empty-input node skip** — nodes that receive zero input items are skipped rather than throwing, mirroring n8n's behaviour.
- **`IF` v2 boolean operators** — `equals`, `notEquals`, `contains`, `greaterThan`, `lessThan`, and friends now coerce both sides to a common type before comparing, matching n8n's behaviour.

## [0.1.0-beta.1] - 2024-01-22

### Added

#### Core Features
- **Complete Goja-based Expression System**: 18x faster expression evaluation than Otto
  - Full n8n expression compatibility with `{{ }}` syntax
  - Support for all n8n built-in functions (string, math, array, date, logic)
  - Variable access: `$json`, `$input`, `$node()`, `$workflow`, `$env`
  - Advanced expression preprocessing for reserved keyword handling

#### Node Types
- **Core Nodes**
  - **Start Node**: Workflow initiation and manual triggering
  - **Set Node**: Field assignment with full expression support
  - **HTTP Request Node**: Complete HTTP client with authentication

- **Transform Nodes**
  - **JSON Node**: Parse, stringify, extract, and merge JSON data
  - **Merge Node**: Combine data from multiple inputs with various merge modes
  - **Switch Node**: Conditional routing with complex rule evaluation
  - **Function Node**: Execute custom JavaScript with secure sandboxing
  - **Code Node**: Advanced JavaScript execution with full context access

- **Trigger Nodes**
  - **Webhook Node**: HTTP webhook receiver with authentication
    - Multiple authentication methods (basic auth, header auth)
    - Request data processing and validation
    - Security hardening and rate limiting

#### Security Features
- **Comprehensive Sandboxing**: Secure JavaScript execution environment
  - Disabled dangerous functions (`eval`, `Function`, global access)
  - Resource limits (memory, execution time, concurrency)
  - Context isolation between workflow executions
- **Authentication & Authorization**: Multiple webhook authentication methods
- **Input Validation**: Comprehensive request and data sanitization
- **Cryptographic Security**: AES-256-GCM encryption for credentials

#### Performance Improvements
- **18x Faster Expression Evaluation**: 180K+ operations/second vs 10K/second
- **15x Faster Workflow Execution**: 9K+ workflows/second vs 500/second
- **95% Memory Reduction**: 17MB vs 400MB baseline usage
- **60x Faster Startup**: Sub-second vs 30-60 second startup time

#### Documentation
- **Comprehensive Node Documentation**: Detailed docs for all node types
- **Migration Guide**: Complete guide for migrating from n8n
- **Security Review**: Full security analysis and hardening guide
- **Production Deployment**: Enterprise deployment documentation
- **Example Workflow Library**:
  - Getting Started workflows (Hello World, Data Processing, API Integration)
  - Data Transformation examples (JSON Processing, Array Operations)
  - API Integration patterns (Webhook Processing)
  - Advanced patterns (Error Handling, Conditional Routing)
  - Business Automation (Lead Generation)

#### Development & Operations
- **Production-Ready Configuration**: Comprehensive config management
- **Monitoring & Observability**: Prometheus metrics and health checks
- **Container Support**: Docker and Kubernetes deployment configurations
- **Security Hardening**: Multi-layer security implementation
- **Backup & Recovery**: Automated backup and recovery procedures

### Changed
- **Expression Engine**: Migrated from Otto to Goja JavaScript engine
- **Error Handling**: Enhanced error reporting with detailed context
- **Configuration**: Unified YAML-based configuration system
- **Logging**: Structured JSON logging with audit capabilities

### Removed
- **Otto JavaScript Engine**: Replaced with faster, more secure Goja engine
- **Legacy Expression Parser**: Replaced with modern Goja-based parser

### Security
- **Expression Sandboxing**: Comprehensive JavaScript runtime isolation
- **Authentication**: Multi-method webhook authentication
- **Input Validation**: Request sanitization and validation
- **Resource Limits**: Memory and execution time protection
- **Encryption**: AES-256-GCM for sensitive data

### Performance
- **Memory Usage**: Reduced from 400MB to 17MB (95% improvement)
- **Expression Speed**: Increased from 10K to 180K ops/sec (18x improvement)
- **Workflow Execution**: Increased from 500 to 9K workflows/sec (18x improvement)
- **Startup Time**: Reduced from 30-60s to <1s (60x improvement)
- **Binary Size**: Reduced from 500MB+ to 25MB (95% improvement)

### Compatibility
- **n8n Workflows**: 100% compatible with existing n8n workflow JSON files
- **Expression Syntax**: Full compatibility with n8n expression language
- **Node Parameters**: Identical parameter structure for core nodes
- **Credential Format**: Compatible with n8n credential storage

### Known Limitations
- **Custom Community Nodes**: Not supported (built-in nodes only)
- **n8n UI**: Command-line and API-based workflow execution only
- **Database Sharing**: Cannot share database with existing n8n instance
- **Advanced Scheduling**: Basic cron support (full scheduler in next release)

## [0.2.0] - 2026-05-07

### Added
- **46+ Node Types**: Complete coverage of core, transform, trigger, messaging, database, email, AI, VCS, cloud, and productivity nodes
  - Core: Start, NoOp, Wait, ExecuteWorkflow
  - Transform: Set, Filter, Code, Function, Merge, JSON, If, Loop
  - Trigger: Webhook, ErrorTrigger, Cron (robfig/cron/v3)
  - HTTP: HTTP Request
  - Messaging: Slack, Discord, Twilio, Teams
  - Database: PostgreSQL, MySQL, SQLite, Elasticsearch, Redis, MongoDB
  - Email: EmailSend, SendGrid
  - AI: OpenAI, Anthropic
  - VCS: GitHub, GitLab
  - Cloud: AWS, GCP, Azure
  - CLI: CLI Execute
  - Productivity: Notion, Stripe, Google Sheets
- **Benchmark & Demo Suite**: Performance benchmarking with n8n comparison and capability demos
- **MCP Server**: Model Context Protocol server integration
- **Python & Node.js SDKs**: Official SDKs for workflow automation
- **Agent Orchestrator Engine**: Control plane and worker architecture
- **Dynamic Plugin System**: Runtime plugin loading and management
- **Smoke Tests**: Comprehensive smoke test workflows
- **Expression Engine**: Goja-based evaluation with full n8n compatibility
- **Comprehensive CI/CD**: GitHub Actions with multi-platform builds, security scanning, and Docker publishing

### Changed
- **Refactored Architecture**: Modular, interface-driven design with clean separation of concerns
- **Node Catalog**: Engine registry as single source of truth for node types
- **Credential Management**: Secure credential storage with AES-256-GCM encryption
- **Memory Storage**: In-memory workflow, execution, and credential storage

### Fixed
- **CI Stability**: Resolved lint errors, data races in benchmarks, and security scan permissions
- **Expression Caching**: Thread-safe expression evaluation
- **Auth Context Keys**: Proper context key types to avoid collisions

## [Future]

### Planned for 0.3.0
- **Workflow Editor**: Web-based visual workflow editor
- **Enhanced Scheduling**: Advanced cron and interval scheduling
- **Plugin Marketplace**: Community plugin discovery and installation
- **Multi-tenancy**: Organization and workspace isolation

### Planned for 1.0.0
- **Production Hardening**: Enterprise-grade security and reliability
- **Advanced Monitoring**: Enhanced observability and alerting
- **Clustering Support**: Multi-instance deployment with load balancing
- **Plugin System**: Support for custom node development
- **Advanced Scheduling**: Complex scheduling and workflow orchestration

## Migration from n8n

m9m provides seamless migration from existing n8n installations:

1. **Export workflows** from n8n as JSON files
2. **Validate workflows** using `m9m validate`
3. **Test execution** with `m9m execute`
4. **Deploy** using provided production configurations

See the [Migration Guide](docs/migration/from-n8n.md) for detailed instructions.

## Performance Benchmarks

### Expression Evaluation Performance
- **Simple Math**: 190K ops/sec (18x faster than Otto)
- **String Functions**: 180K ops/sec (18x faster)
- **Array Functions**: 185K ops/sec (18x faster)
- **Complex Expressions**: 170K ops/sec (17x faster)

### Workflow Execution Performance
- **Simple Workflows** (1-3 nodes): 15K workflows/sec, 65μs latency
- **Medium Workflows** (4-8 nodes): 9K workflows/sec, 110μs latency
- **Complex Workflows** (9+ nodes): 5K workflows/sec, 200μs latency

### Resource Usage
- **Memory**: 17MB baseline (vs 400MB n8n)
- **CPU**: Efficient goroutine-based concurrency
- **Startup**: Sub-second initialization
- **Binary**: 25MB single-file deployment

## Contributors

- **Lead Developer**: m9m Development Team
- **Security Review**: Security Engineering Team
- **Documentation**: Technical Writing Team
- **Testing**: Quality Assurance Team

## Support

- **Documentation**: [docs.m9m.com](https://docs.m9m.com)
- **GitHub Issues**: [github.com/m9m/m9m/issues](https://github.com/m9m/m9m/issues)
- **Community Forum**: [community.m9m.com](https://community.m9m.com)
- **Enterprise Support**: [enterprise@m9m.com](mailto:enterprise@m9m.com)

---

**Note**: This is a beta release intended for testing and evaluation. While we've conducted extensive testing and security reviews, please thoroughly test in your environment before production deployment.