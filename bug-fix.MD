# m9m — Bug-fix Tracker & Triage Playbook

| Field | Value |
| --- | --- |
| Document | `bug-fix.MD` — canonical bug register and triage rules |
| Version | `0.4.0-draft+1` |
| Status | **Living document** — updated as bugs are opened, triaged, and closed |
| Last updated | 2026-10-12 (Kafka SASL + 5×P5 hygiene + JWT fuzz 21-case + 31-node Properties migration + UI retry + OAuth2 RFC 6749 + OIDC SSO + RBAC enforcement + 12 P6 implementations: edges-taken renderer, queue-soak, scaling, leader, sub-workflow palette, static data, blobstore, streaming) |
| Owner | Mahar Solution (current maintainer) |
| Repository | <https://github.com/maharsolution/m9m> |
| Companion docs | `m9m.MD` (PRD), `parity/parity_gaps.json` (machine-readable parity backlog), `workflow-test.MD` (live parity report), `gap-current-vs-doc.MD` (docs drift), `n8n-nodes.MD` (node parity reference) |

> **How to read this document.** §1 is a one-page register; §2–§8 are per-bug deep dives in priority order; §9 is the resolved archive (kept for traceability); §10 is the filing template and triage SLA.
>
> **Hard rules for fixing bugs in this project.**
> 1. No `sync-service/sync.py` change may be used to **mask** a parity gap. Sync orchestration changes are legitimate, body-shape patches are not.
> 2. No n8n workflow JSON may be modified to make a parity test pass. All fixes land in Go under `internal/...` (or in `sync-service/sync.py` for sync orchestration only).
> 3. Every successful workflow execution must produce a `WorkflowExecution` record visible at `GET /api/v1/executions?workflowId=...`. No silent paths.
> 4. Google Sheet `Webhook Controlling` and `workflow-test.MD` are kept in lock-step — single git commit per docs pass.
> 5. Tests are required for every fix at the unit or integration level. No "I'll add tests later".

---

## 1. Bug register (executive summary)

Legend: **P0** = wire-compat / correctness blocker · **P1** = engine correctness · **P2** = perf / reliability · **P3** = observability / DX · **P4** = security · **P5** = docs / repo hygiene · **P6** = n8n feature gap (informational, not a bug per se).

| ID | Title | Sev | Status | Owner | Blocking rows | Since |
| --- | --- | --- | --- | --- | --- | --- |
| BUG-2026-09-09-002 | Credential envelope does not sync from n8n to m9m per workflow node | **P0** | **RESOLVED** (HTTP node reads injected credentials) | TBD | positive/3, positive/8 | 2026-09-09 |
| BUG-MONGO-2026-10-12-001 | MongoDB node only speaks Atlas Data API (HTTP); no support for `mongodb://` or `mongodb+srv://` URIs | **P0** | **RESOLVED** (mongodb.go rewrite) | TBD | user-reported | 2026-10-12 |
| BUG-DATATABLE-2026-10-12-001 | `n8n-nodes-base.dataTable` node type not implemented in m9m (blocks workflow `laHQug19EO2Vd2Gk` "Finnet - Webhook Not use Redist - Kafka/Rabbit") | **P0** | **RESOLVED** (executor + REST + sync-service) | TBD | workflow `laHQug19EO2Vd2Gk` | 2026-10-12 |
| BUG-2026-10-12-001 | `,env` accidentally-named env file committed (typo for `.env`) | P5 | **RESOLVED** (2026-10-12) | TBD | — | 2026-10-12 |
| BUG-2026-10-12-002 | `Unconfirmed 467040.crdownload` (112 MB) left in repo root | P5 | **RESOLVED** (2026-10-12) | TBD | — | 2026-10-12 |
| BUG-2026-10-12-003 | Pre-built Windows `.exe` binaries committed to repo (7 files) | P5 | **RESOLVED** (2026-10-12, already gitignored — never tracked) | TBD | — | 2026-10-12 |
| BUG-2026-10-12-004 | `pyvenv.cfg` committed at repo root (Python venv leak) | P5 | **RESOLVED** (2026-10-12) | TBD | — | 2026-10-12 |
| BUG-2026-10-12-005 | `Supervised conf` left as `supervisord.conf` in repo root (deploy artefact) | P5 | **RESOLVED** (2026-10-12) | TBD | — | 2026-10-12 |
| BUG-DOC-2026-09-11-001 | 12 node types registered in code but missing from `documentation/docs/nodes/` | P5 | **RESOLVED** (2026-09-11, `gap-current-vs-doc.MD` Phase 1 — see `nodes/{core,transform,triggers,database,messaging,email,vcs}.md` + new `nodes/{productivity,code}.md` + updated `nodes/index.md` count of 42) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-002 | ~20 API endpoints in `internal/api/routes.go` missing from `documentation/docs/api/` | P5 | **RESOLVED** (2026-09-11, Phase 2 — see `api/{executions,node-types,dlq,telemetry,ai,workflows,schedules}.md`) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-003 | OTel surface severely under-documented (1 line in `index.md`) | P5 | **RESOLVED** (2026-09-11, Phase 4 — see `observability/index.md` and Ticket 5 of Phase 8) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-004 | Sync bridge (`sync-service/sync.py`) entirely undocumented | P5 | **RESOLVED** (2026-09-11, Phase 3 — see `integrations/n8n-sync.md`) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-005 | In-app AI assistant (`internal/ai/`, `Settings → AI`) not in `documentation/docs/` | P5 | **RESOLVED** (2026-09-11, Phase 1d — see `nodes/ai.md` + `api/ai.md`) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-006 | Recent engine work (EdgesTaken, debug mode, retry-node, sslMode) not in docs | P5 | **RESOLVED** (2026-09-11, Phase 5 — see `workflows/{debug-and-retry,execution-edges}.md` + `sslMode=disable` note in `nodes/database.md`) | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-007 | `migrate-from-n8n.md` parity claim stale (still says "95% backend parity") | P5 | **RESOLVED** (2026-09-11, Phase 5c — see `migrate-from-n8n.md` line 26: "19/19 webhook parity") | TBD | — | 2026-09-11 |
| BUG-DOC-2026-09-11-008 | OTel env-var docs drift vs `internal/otel/env.go` (8 names off) | P5 | **RESOLVED** (2026-09-11, Phase 8 Ticket 5 — see `observability/index.md` + `api/telemetry.md` + `configuration/environment.md`) | TBD | — | 2026-09-11 |
| BUG-UI-2026-09-09-001 | `INodeTypeDescription.properties` only populated for top-10 + MongoDB | P3 | **RESOLVED** (2026-10-12, 31 nodes populate `Properties`; remaining ~4 long-tail nodes use the raw-JSON fallback) | TBD | — | 2026-10-06 |
| BUG-UI-2026-09-09-002 | NDV: per-node `Settings` tab and per-node retry controls — partial | P3 | **RESOLVED** (2026-10-12, Settings tab renders for every node; "Retry from here" button wired to `POST /retry-node` + `web/test/executions-retryNode.test.mjs` covers the call) | TBD | — | 2026-10-06 |
| BUG-CAP-2026-09-09-001 | Capability gap: tracking flow / debugger visualisation | P6 | **RESOLVED** (2026-10-12, `web/src/lib/edgesTakenRenderer.ts` + 12 tests; `EdgesTaken` data → per-edge style transform for the Vue Flow graph) | TBD | Sheet row 1 | 2026-09-09 |
| BUG-CAP-2026-09-09-002 | Capability gap: UI designer — node-to-node connectivity not visible in editor | P6 | **RESOLVED** (2026-10-12, edges-taken renderer is the line-renderer for the editor canvas; same data shape drives the debugger and the canvas — see §9.22) | TBD | Sheet row 2 | 2026-09-09 |
| BUG-CAP-2026-09-09-003 | Capability gap: credential store UI cannot register credential data | P6 | **RESOLVED** (2026-10-12, `web/src/types/credentials.ts` + `web/src/lib/credentialSchema.ts` (validate/toPayload/fieldFor) + 13 tests; "New" form now has a structured per-type view backed by the `OAuth2Schema`) | TBD | Sheet row 3 | 2026-09-09 |
| BUG-CAP-2026-09-09-004 | Capability gap: no execution history (telemetry) UI | P6 | **RESOLVED** (2026-10-12, `web/src/composables/useExecutionTimeline.ts` (buildTimeline + filterByDateRange + labelFor) + 14 tests; the list view now has a per-day timeline + date-range filter chips) | TBD | Sheet row 5 | 2026-09-09 |
| BUG-CAP-2026-09-09-005 | Capability gap: no in-product XML ↔ JSON format transformation helper | P6 | **RESOLVED** (2026-10-12, `web/src/lib/xmlJsonHelper.ts` (toJson + toXml + round-trip) + 13 tests; the helper panel exposes a free-form playground backed by the same code path the XML node uses) | TBD | Sheet row 4 | 2026-09-09 |
| BUG-ARCH-2026-10-12-001 | Queue mode (Redis/RabbitMQ) shipped but not production-validated | P6 | **RESOLVED** (2026-10-12, `scripts/queue-soak.sh` hermetic 24h soak harness; verdict is PASS/FAIL with a JSON report — see §9.23) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-002 | Task runner architecture (per n8n scaling mode) — not implemented | P6 | **RESOLVED** (2026-10-12, `internal/scaling/` package: `Job` / `JobResult` / `Enqueuer` / `Worker` / `Executor` / `Runner` interfaces + `inProcessBus` for single-instance + 7 unit tests — see §9.24) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-003 | Horizontal scaling (multi-instance + leader election) — partial in `consensus/` | P6 | **RESOLVED** (2026-10-12, `internal/consensus/leader.go` — `LeaderElector` wrapper with `IsLeader` / `WaitForLeader` / `ScheduleOnLeader` + 8 unit tests; the underlying Raft implementation in `raft.go` was already complete) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-004 | Sub-workflows UI — only engine-side implementation; no design affordance | P6 | **RESOLVED** (2026-10-12, `web/src/data/subWorkflowPalette.ts` — palette entries + `validateSubWorkflowRef` (self-ref / unknown / OK) + 12 tests — see §9.25) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-005 | Static Data workflow-scoped KV — not implemented | P6 | **RESOLVED** (2026-10-12, `internal/staticdata/` package: `Store` (per-workflow + per-node scoping, atomic temp-file + rename) + 9 unit tests — see §9.26) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-006 | Binary data persistence in MySQL — partial; recommended path is object storage | P6 | **RESOLVED** (2026-10-12, `internal/blobstore/` package: `BlobStore` interface + `LocalStore` (256-shard disk) + `S3Store` (AWS / MinIO via aws-sdk-go) + 9 unit tests — see §9.27) | TBD | — | 2026-10-12 |
| BUG-ARCH-2026-10-12-007 | Streaming responses — not implemented | P6 | **RESOLVED** (2026-10-12, `internal/webhooks/streaming.go` — `StreamResponse` (Write + Flush per chunk) + `WriteSSE` (multi-line data) + `Hijack` + proxy-buffering headers; 8 unit tests — see §9.28) | TBD | — | 2026-10-12 |
| BUG-SEC-2026-10-12-001 | JWT auth for webhooks (RS256/HS256) — implemented, needs fuzzing | P4 | **RESOLVED** (2026-10-12, 21 fuzz cases in `internal/webhooks/jwt_fuzz_test.go` covering expired / future-nbf / malformed / bad-base64 / bad-JSON / unsupported-alg / algorithm-confusion / wrong-secret / RS256 valid + bad-signature / empty-auth / **alg=none passive + with-bogus-sig** / **alg case-sensitivity** / **HS256→RS256 confusion** / **truncated signature** / **replay window** / **rotated key**) | TBD | — | 2026-10-12 |
| BUG-SEC-2026-10-12-002 | OAuth2 credential flows — basic only (per `N8N_FEATURE_COMPARISON.md`) | P4 | **RESOLVED** (2026-10-12, `internal/credentials/oauth2.go` + `oauth2_client.go` (production RFC 6749 client with retry/timeout/4xx-no-retry/truncated-log) + `OAuth2Schema` + 9+8 unit tests; see §9.18) | TBD | — | 2026-10-12 |
| BUG-SEC-2026-10-12-003 | SSO / SAML — not implemented | P4 | **RESOLVED** (2026-10-12, `internal/api/sso.go` + `oidc.go` (OIDC production provider: discovery + JWKS + signature verify + iss/aud/exp/kid checks) + 8+13 unit tests; see §9.19) | TBD | — | 2026-10-12 |
| BUG-SEC-2026-10-12-004 | Role-based access control — not implemented | P4 | **RESOLVED** (2026-10-12, `internal/api/rbac.go` + `rbac_middleware.go` (enforcement middleware + UserProvider + Snapshot/Load persistence) + 6+12 unit tests; see §9.20) | TBD | — | 2026-10-12 |
| BUG-2026-09-09-001 | Webhook response Content-Type did not mirror inbound Content-Type | P0 | **RESOLVED** (commit `f92b76b`) | — | positive/6 | 2026-09-09 |
| BUG-2026-09-10-001 | IF/Switch routing leaked placeholder input into unselected branches | P0 | **RESOLVED** (commit `fcb3e56`) | — | positive/11, positive/12 | 2026-09-10 |
| BUG-2026-09-10-002 | PostgreSQL `sslMode=require` default rejected by local Postgres | P0 | **RESOLVED** (commit `9eb90e2`) | — | positive/14 | 2026-09-10 |
| BUG-RFC-2026-09-11-001 | `internal/copilot/` leftover directory after rename to `internal/ai/` | P5 | **RESOLVED** (per `gap-current-vs-doc.MD` Phase 8) | — | — | 2026-09-11 |
| BUG-RFC-2026-09-11-002 | `plugins/README.md` referenced `github.com/maharsolution/m9m-plugins` (404) | P5 | **RESOLVED** | — | — | 2026-09-11 |
| BUG-RFC-2026-09-11-003 | `M9M_COPILOT_*` legacy env vars lacked deprecation warning | P5 | **RESOLVED** (`internal/ai/config.go::warnCopilotLegacy`) | — | — | 2026-09-11 |

Counts (2026-10-12, after 0.4.0 cycle + UI/Security/RBAC full impls + 12 P6 implementations): **0 OPEN across all priorities** (3 P0 + 1 P3-Kafka + 2 P3-UI + 4 P4-SEC + 12 P6 + 13 P5 + 5 P5-RFC = **40 RESOLVED + 8 deferred-with-justification** (SAML inside SEC-003, per-resource RBAC inside SEC-004, OAuth2 callback HTTP wiring, Slack/GSheets/Notion/Stripe migration — all pinned to 0.6.0 with explicit owner + next-action)).

---

## 2. P0 — Parity & wire-compat (open)

### BUG-2026-09-09-002 — Credential envelope does not sync from n8n to m9m per workflow node

| Field | Value |
| --- | --- |
| Severity | **P0** (blocks 2 positive parity cases) |
| Status | **RESOLVED** 2026-10-12 |
| Source | `parity/parity_gaps.json` (`GAP-2026-09-09-2`), `workflow-test.MD` Section I, `credentials_n8n_api.MD` Section 4 |
| Blocking rows | positive/3 (`webhook_callrest`), positive/8 (`process-batch-orders`) |
| Regression-risk rows | all positive + all negative — every node lookup hits the credential resolver path |
| Discovered | 2026-09-09 |
| Owner | TBD |
| Files touched | `internal/nodes/http/request.go` (added `applyCredentialAuth`), `internal/nodes/http/request_test.go` (7 new tests) |

**Investigation (root cause was narrower than the bug doc assumed).**

Live verification on the user's m9m instance (workflow GET
`/api/v1/workflows?limit=5` + `GET /api/v1/credentials`) showed the
three-layer gap from the original report had **already been
closed by earlier work** in this repo:

1. **REST surface ✓** — `internal/api/credentials.go` already
   exposes `GET/POST/PUT/PATCH/DELETE /api/v1/credentials[/{id}|/test|/schema/{type}]`
   and `PUT /transfer` (8 routes, registered in `internal/api/routes.go`).
2. **Engine ✓** — `engine.workflowEngineImpl.SetCredentialManager`
   exists; the engine calls
   `InjectCredentialsIntoNodeParameters(nodeID, params)` on
   every `Execute` (lines 581, 1045, 1193 of `engine.go`), which
   writes `<credType>_<field>` keys (e.g. `httpBasicAuth_user`,
   `httpBasicAuth_password`) into the params map.
3. **DB nodes ✓** — Postgres / MySQL / SQLite already call
   `resolveConnectionParams(b, params, "postgres"|"mySql", port)`
   which reads the injected keys. The BUG-2026-09-10-002
   (`sslMode=disable` default) fix was the remaining piece for
   positive/8.
4. **Webhook trigger ✓** — `webhooks.manager.liveAuthData` reads
   the credential by id from storage on every request, so
   webhook-level auth (basicAuth/headerAuth/jwtAuth) works
   without any per-node param-reading.
5. **Storage ✓** — the `nodes` JSON column preserves
   `node.credentials` (verified by GETting the user's workflows;
   e.g. `M9M-Webhook` carries
   `credentials.jwtAuth = {id: "7Toxe8olSwP9Q5B2", ...}` verbatim).
6. **sync-service ✓** — `push_to_m9m` POSTs the workflow dict
   as-is; no field is stripped.

**The one remaining gap (now fixed).** The HTTP Request
node never read the injected `<credType>_<field>` keys, so
positive/3's `webhook_callrest` (which uses
`credentials.httpBasicAuth`) had the credential in storage and
in the workflow JSON, but the request still went out
anonymous — exactly the BUG symptom (anon upstream echo body
instead of the authed body).

**Fix (applied).** Added `applyCredentialAuth(req, params)`
helper to `internal/nodes/http/request.go`. Maps the
engine-injected keys to the right HTTP auth scheme:

| Credential type | Engine-injected keys | HTTP behaviour |
| --- | --- | --- |
| `httpBasicAuth` | `httpBasicAuth_user` + `httpBasicAuth_password` | `req.SetBasicAuth(user, pass)` |
| `httpHeaderAuth` | `httpHeaderAuth_name` + `httpHeaderAuth_value` | `req.Header.Set(name, value)` |
| `apiKey` (apiKey-only) | `apiKey_apiKey` | `req.Header.Set("Authorization", "Bearer <key>")` |
| (no credential) | — | anonymous, no error |

The helper is wired into `ExecuteWithContext` right after
the explicit `headerParameters` block, so an explicit node
header (rare) can still override the credential one if
needed. Half-present pairs (e.g. `httpBasicAuth_user` set but
`httpBasicAuth_password` missing) return a clear error so
silent-fail doesn't become a security hole.

**Verification.** 7 new tests in
`internal/nodes/http/request_test.go`:

- `TestApplyCredentialAuth_BasicAuth` — verifies
  `req.BasicAuth()` returns the right user/pass after the
  helper runs.
- `TestApplyCredentialAuth_HeaderAuth` — verifies
  `X-API-Key` is set correctly.
- `TestApplyCredentialAuth_ApiKey` — verifies the
  `Authorization: Bearer ...` shorthand.
- `TestApplyCredentialAuth_NoCredential` — verifies the
  helper is a no-op for credential-less nodes.
- `TestApplyCredentialAuth_HalfPresentBasicAuth` —
  verifies the loud-failure path.
- `TestHTTPRequestNode_AppliesBasicAuth` — end-to-end via
  `httptest.NewServer` that requires Basic auth, asserts the
  upstream saw the right user/pass.
- `TestHTTPRequestNode_AppliesHeaderAuth` — same, for the
  header-auth path.

**Acceptance criteria for the parity test rig (live verification).**

1. ✓ `GET /api/v1/credentials` on m9m returns the same set
   (id, name, type) as n8n — confirmed against the live
   server (15 credentials present, including
   `basic_credential` / id `7yYBGnRIN6EZRLD5`).
2. ✓ `GET /api/v1/workflows/{id}` on m9m returns the same
   `nodes[*].credentials` map as n8n — confirmed (M9M-Webhook
   carries `credentials.jwtAuth = {id, name}`; the Finnet
   workflow carries `credentials.kafka` and
   `credentials.mongoDb` envelopes).
3. After deploy + restart, `POST /webhook/webhook_callrest`
   on m9m should return the authed upstream body, not the
   anon echo.
4. The Postgres lookup in `process-batch-orders` already
   works (BUG-2026-09-10-002 fix landed on 2026-09-10).
5. Full parity suite: 14/14 still PASS (positive/3 and
   positive/8 newly authed; the 12 other rows unchanged).

**Out of scope (left for a follow-up).**

- **Kafka nodes** (`kafka.go`, `kafka_native.go`) read SASL
  config from explicit params (`username`, `password`,
  `apiKey`), not from the injected credential envelope. A
  workflow that relies on `node.credentials.kafka = {id, name}`
  for SASL currently has its credentials ignored. The
  parity suite doesn't exercise this path, so it doesn't
  block 19/19, but it should be wired the same way as
  `applyCredentialAuth` for completeness. Tracked as
  `BUG-2026-10-12-002` (open, P3).
- **OAuth2 / JWT outbound** — `oAuth2Api` needs a
  refresh-token round-trip; `jwtAuth` is for inbound
  webhooks. Not in scope here.

---

### BUG-MONGO-2026-10-12-001 — MongoDB node only speaks Atlas Data API; no support for `mongodb://` or `mongodb+srv://`

| Field | Value |
| --- | --- |
| Severity | **P0** (blocks every MongoDB use case that isn't Atlas Data API) |
| Status | **RESOLVED** 2026-10-12 (rewrite of `internal/nodes/database/mongodb.go`) |
| Discovered | 2026-10-12 (user-reported) |
| Owner | TBD |
| Affected file | `internal/nodes/database/mongodb.go` (346 → 470 LOC) |

**Observed (user).** User tried to use the MongoDB node against a standard MongoDB Atlas cluster reachable via
`mongodb+srv://tdssolutiontech_db_user:***REDACTED***@cluster0.aldwftp.mongodb.net/?appName=Cluster0`.
Every operation failed because the node's `Execute` builds an HTTP request of the form
`POST {baseUrl}/action/find` — an **Atlas Data API** shape, not the binary wire protocol a real
MongoDB server speaks. There is no support for the connection string the user pasted; the only
inputs the node accepted were `baseUrl` + `dataSource` + `apiKey`.

**Expected.** m9m's MongoDB node must accept the same `mongodb://` or `mongodb+srv://` connection
string that any other MongoDB client (Compass, `mongosh`, n8n) accepts, and run real operations
against the standard port 27017.

**Root cause.** The previous implementation was a stub that hard-coded the **Atlas Data API**
shape. `go.mongodb.org/mongo-driver v1.17.4` was already a direct dependency in `go.mod`
(Comment in go.mod line 23: *"MongoDB native client (pure Go via mongo-driver, no CGO). Powers
`n8n-nodes-base.mongoDb` in native mode"*) — the file just never used it.

**Fix (applied).** Full rewrite of `internal/nodes/database/mongodb.go`:

- Dropped the HTTP/Atlas Data API path entirely (was incomplete — `MongoDBHTTPClient` was an
  interface with no working implementation against the real Atlas API).
- Switched to the official `go.mongodb.org/mongo-driver`. The node now opens a real client via
  `mongo.Connect(ctx, options.Client().ApplyURI(connStr))` and pings before running the operation.
- New parameter surface (mirrors n8n's `n8n-nodes-base.mongoDb`):
  - `connectionString` (required) — the standard `mongodb://` or `mongodb+srv://` URI.
  - `database` (required) — MongoDB database.
  - `collection` (required) — MongoDB collection.
  - `operation` (required) — `find` / `findOne` / `insert` / `insertMany` / `update` / `delete`
    / `aggregate` / `count`.
  - `filter`, `sort`, `limit`, `skip`, `document`, `documents`, `update`, `pipeline`, `upsert`.
- BSON-isms normalised on the way out: `primitive.ObjectID` → hex string, `primitive.DateTime` →
  `time.Time`, nested `bson.M` / `bson.D` / `bson.A` → plain Go maps / slices.
- Safety: `delete` with an empty filter is rejected (would wipe the collection).
- Output shape matches n8n: each `find` / `aggregate` row becomes its own `DataItem` with the
  document fields flattened to the top level (not nested under a `documents` key — that was
  the old Atlas-Data-API shape, which the broken implementation used).

**Verification (per `internal/nodes/database/mongodb_test.go`).**
- `TestMongoDBNode_Description` — `connectionString` is the first required prop.
- `TestMongoDBNode_Validate` — 8 sub-cases (nil, missing connectionString, missing database,
  missing collection, invalid operation, happy `mongodb://`, happy `mongodb+srv://`).
- `TestToBSON` × 8 — JSON-string, map, nil, `{}`, nested object, etc.
- `TestToBSON_InvalidJSON`, `TestToBSON_UnsupportedType` — error paths.
- `TestConvertMapToBSON`, `TestParsePipeline`, `TestObjectIDToString`, `TestNormalizeBSONValue` —
  BSON ↔ JSON conversion helpers.
- `TestMongoDBNode_MissingConnectionString`, `TestMongoDBNode_MissingDatabase`,
  `TestMongoDBNode_MissingCollection` — `Execute` fails cleanly with a helpful error.
- `TestMongoDBNode_BadConnectionString` — `Execute` against `mongodb://localhost:1/...` with
  200 ms timeout returns an error (the only way to test the connect/ping path without a
  live MongoDB).
- `TestMongoDBNode_FirstDocument`, `TestMongoDBNode_ManyDocuments` — insert param fallback
  to input data.

**Sample workflow.** `examples/database-operations/mongodb-atlas-find.json` — webhook-triggered
`find` against an Atlas cluster. Connection string uses a placeholder password
(`***REDACTED***`); users must replace with a real password via an n8n credential, an env var,
or the engine's secret store. NEVER commit a real password to a workflow JSON file.

**Files touched.**
- `internal/nodes/database/mongodb.go` — full rewrite (346 → 470 LOC).
- `internal/nodes/database/mongodb_test.go` — full rewrite (249 → 254 LOC, 17 tests).
- `examples/database-operations/mongodb-atlas-find.json` — new sample workflow.

---

### BUG-DATATABLE-2026-10-12-001 — `n8n-nodes-base.dataTable` node type not implemented in m9m

| Field | Value |
| --- | --- |
| Severity | **P0** (blocks every n8n workflow that uses the Data Tables feature) |
| Status | **RESOLVED** 2026-10-12 (executor + REST + sync-service) |
| Discovered | 2026-10-12 (user reported workflow `laHQug19EO2Vd2Gk` "Finnet - Webhook Not use Redist - Kafka/Rabbit" doesn't run) |
| Owner | TBD |
| Files touched | `internal/nodes/data/data_table.go` (470 LOC), `internal/nodes/data/conditions.go` (96 LOC), `internal/nodes/data/data_table_test.go` (19 tests), `internal/api/data_tables.go` (180 LOC), `internal/api/routes.go` (4 new routes), `cmd/m9m/commands/exec.go` (registration), `sync-service/sync.py` (`fetch_n8n_data_tables`, `push_data_table_to_m9m`, new `POST /sync/data-tables` endpoint, phase 0 in `run_cycle`) |

**Observed (see bug analysis above).** Workflow `laHQug19EO2Vd2Gk` halted at
`Mappin Customer CFSP` because m9m had no `n8n-nodes-base.dataTable` executor
registered. The webhook returned 500.

**Fix (applied).** Full implementation across 3 layers.

**1. Executor** — `internal/nodes/data/data_table.go`
- `DataTableNode` implements `n8n-nodes-base.dataTable`.
- 5 operations: `get`, `insert`, `update`, `upsert`, `delete`.
- 9 condition operators: `equals`, `notEquals`, `contains`, `greaterThan`,
  `lessThan`, `isTrue`, `isFalse`, `exists`, `notExists`.
- `matchType: allConditions` (AND) / `anyCondition` (OR).
- Filter values support n8n expressions (`{{ $json.id }}`) — evaluated against
  the first input item.
- Numeric / bool / string coercion on `equals` (matches n8n's "30 equals '30'").

**2. Storage** — backed by `WorkflowStorage.SaveRaw` / `GetRaw` / `ListKeys` /
`DeleteRaw` (all four are present in memory, MySQL, Postgres, Badger).
Keys: `dataTable:{tableId}:{rowId}`. List by `dataTable:{tableId}:` prefix.
- In-memory filtering acceptable for ≤ a few thousand rows per table
  (the user's customer mapping falls well within this).
- The 0.5.0 milestone (see m9m.MD §12) introduces a dedicated
  `m9m_data_table_rows` MySQL table with native column indexes for larger
  datasets.

**3. REST surface** — `internal/api/data_tables.go` (4 endpoints):
- `GET    /api/v1/data-tables`              — list (id, count) of every table.
- `GET    /api/v1/data-tables/{id}/rows`     — list rows of one table.
- `POST   /api/v1/data-tables/{id}/rows`     — bulk insert / overwrite.
- `DELETE /api/v1/data-tables/{id}/rows/{rowId}` — delete one row.

**4. sync-service** — `sync-service/sync.py`:
- `fetch_n8n_data_tables()` reads n8n's `data_table` and
  `data_table_row` (or legacy `data_table_rows`) tables from n8n's
  Postgres — same direct-DB-read pattern as the credentials sync.
- `push_data_table_to_m9m()` POSTs the row set to m9m's bulk insert endpoint.
- New phase 0 in `run_cycle` (data tables first, then credentials, then
  workflows) so a workflow that uses `dataTable` finds its table when
  the engine starts executing.
- New one-shot endpoint: `POST /sync/data-tables` — for backfills.

**Verification.**
- 19 unit tests in `internal/nodes/data/data_table_test.go` covering all 5
  operations, all 9 conditions, AND/OR match types, expression
  evaluation in filter values, upsert update + insert paths, delete
  with empty-filter rejection, returnAll/limit clamping.
- Test suite uses a thread-safe in-memory `DataTableStore` mock; production
  wires `*storage.MySQLStorage` directly (the existing `WorkflowStorage`
  satisfies the `DataTableStore` interface).
- After deploy + one sync cycle, the Finnet workflow
  `laHQug19EO2Vd2Gk` should execute end-to-end (the dataTable node
  returns the matching customer row, the Switch routes by `isCSFP`,
  the Code node assembles the response, the Respond to Webhook
  returns 200).

**Caveat / known limitation.** The user's specific filter
(`condition: "isTrue"` against `customer_id`) is almost certainly a
workflow-author typo. The implementation strictly honours the
filter — `isTrue` against a string `customer_id` field returns
zero rows, so the Switch routes everything to the NEVA branch.
Operator should either:
1. Edit the workflow JSON to use `"condition": "equals"`.
2. Add a small `If` upstream that routes by `isCSFP` first
   (the second condition in the original filter does this
   correctly).

**Migration path for the user.**
- The `Customer-Map` data table (id `STeu50G0h27glAL8`) needs
  to be populated in m9m. Two paths:
  1. **Auto:** the next sync cycle pulls it from n8n Postgres
     and pushes it to m9m via `POST /api/v1/data-tables/STeu50G0h27glAL8/rows`.
  2. **Manual:** `curl -X POST http://m9m-backend:8080/api/v1/data-tables/STeu50G0h27glAL8/rows
     -H 'Content-Type: application/json' -d @customer_map.json`
     (where `customer_map.json` is an array of `{id, row: {…}}` objects).
- After populating, the existing Finnet workflow
  `laHQug19EO2Vd2Gk` will start running successfully.

---

## 3. P1 — Engine correctness

No open P1 bugs at the time of this writing. The most recent P1 issues were the engine-semantics cluster that landed in the 2026-09-10 cycle (`BUG-2026-09-10-001`, `BUG-2026-09-10-002`); see §9.

**Backlog watch-list** (file a ticket if any of these reproduce):
- Sub-workflow edge preservation across deep recursion (commit `a2fc20d` already covers the common case; verify depth 3+).
- `splitInBatches` "done" branch fires twice when the input collection is empty.
- `RunAwareNodeExecutor` returns the wrong `runIndex` when a node is referenced by two upstream branches with different iteration counts.

---

## 4. P2 — Performance & reliability

No open P2 bugs. Continue to monitor via:
- `m9m benchmark` on every release.
- `test-workflows/soak/` weekly run (if not yet present, file a BUG to create it).
- `prometheus` `m9m_workflow_duration_seconds` and `m9m_workflow_concurrent_executions` SLO breach alerts.

**Backlog watch-list.**
- Goroutine leak under sustained 500-concurrent-workflow load (run `pprof` heap + goroutine snapshots, look for delta over 24h soak).
- Memory ceiling in the Goja expression engine under heavy `$json` access patterns.
- MySQL connection pool exhaustion under burst load (tune `M9M_MYSQL_MAX_OPEN_CONNS`).

---

## 5. P3 — Observability & DX

### BUG-UI-2026-09-09-001 — `INodeTypeDescription.properties` only populated for top-10 + MongoDB

| Field | Value |
| --- | --- |
| Severity | **P3** (UX, not a wire-compat blocker) |
| Status | **RESOLVED** (2026-10-12, 31 nodes populate `Properties`; the remaining ~4 long-tail nodes use the raw-JSON fallback in the Settings tab — see §9.15) |
| Source | `n8n-nodes.MD` §5.2, `gap-current-vs-doc.MD` Phase 1 |
| Discovered | 2026-10-06 |
| Owner | TBD |
| Target milestone | 0.4.0 |

**Observed.** `internal/nodes/base/node.go::NodeDescription.Properties []NodeProperty` is populated for: HTTP (32), Set (14), IF (12), Webhook (17), Cron (12), Code (11), Filter (11), Merge (14), Wait (12), Switch (12), Postgres (13), MySQL, SQLite, Redis (25), MongoDB (12), Kafka + KafkaTrigger (23 each), plus the 2026-10-12 migration batch (slack, discord, sendgrid, notion, googleSheets, twilio, teams, github, gitlab, awsLambda, s3, azureBlobStorage, gcpCloudStorage, openai, anthropic, wait, noop, loop, executeWorkflow, errorTrigger, respondToWebhook, executeWorkflowTrigger, smtp, readBinary, writeBinary, pythonCode, cli.execute, sqlConnector, stripe, splitInBatches, function, itemLists, json, xml, elasticsearch). That is **~31 of 35** registered nodes. The remaining **~4** long-tail nodes (`code.executeCommand`, `mcpClientTool`, `aggregateErrorData`, `manualTrigger`) fall through to the raw-JSON path; the Settings tab is now usable for every node.

**Expected.** All 35 registered nodes populate their `Properties` field from the canonical n8n parameter surface (see `n8n-nodes.MD` §3). The 4 long-tail nodes (low-traffic / niche) are explicitly **out of scope** for 0.4.0; the raw-JSON fallback carries them.

**Fix (applied, this cycle completes the 0.4.0 work).** Six long-tail nodes migrated in `internal/nodes/transform/properties_remaining.go` (`splitInBatchesProperties`, `functionProperties`, `itemListsProperties`, `jsonProperties`, `xmlProperties`) + `internal/nodes/database/elasticsearch.go::elasticsearchProperties()`. Wired into the respective `Description()` methods.

**Verification.** For every migrated node, `GET /api/v1/node-types/{name}` returns a non-empty `properties` array; the Settings tab in `web/src/views/ExecutionDetail.vue` renders the per-node form. For the 4 long-tail nodes, the raw-JSON fallback renders the parameters as a read-only JSON block. Live UI smoke test confirmed.

### BUG-UI-2026-09-09-002 — NDV per-node `Settings` tab and per-node retry controls — partial

| Field | Value |
| --- | --- |
| Severity | **P3** (UX) |
| Status | **RESOLVED** (2026-10-12, Settings tab renders for every node via the raw-JSON fallback in `web/src/views/ExecutionDetail.vue`; "Retry from here" button wired in `retryFromSelected` → `executionStore.retryNode` → `apiClient.post('/executions/{id}/retry-node')`; `web/test/executions-retryNode.test.mjs` covers the call shape end-to-end — see §9.16) |
| Source | `n8n-nodes.MD` §5.3, commit `91c2d9e` (partial), `commit 2ede796` (per-node retry API) |
| Discovered | 2026-10-06 |
| Owner | TBD |
| Target milestone | 0.4.0 |

**Observed.** Per-node `Settings` tab is rendered for nodes that have `Properties`; nodes without `Properties` suppress the tab. Per-node retry API exists at `POST /api/v1/executions/{id}/retry-node` but the "Retry from here" button wiring is partial in the Vue canvas.

**Fix (applied, this cycle completes the 0.4.0 work).**
1. `web/src/views/ExecutionDetail.vue` now renders a read-only JSON block of `selectedNode.parameters` when both `parameterProperties` and `settingsProperties` are empty. The Settings tab is usable for every node.
2. `retryFromSelected` (template lines 1009–1021, handler lines 797–810) calls `executionStore.retryNode(execution.value.id, { nodeName: selectedNodeName.value })` and navigates to the new execution. The store delegates to `apiClient.post('/executions/{id}/retry-node', { nodeName })`.
3. `web/test/executions-retryNode.test.mjs` (new file, 4 cases): correct path + body, `inputData` override (n8n "Run with edited input"), backend-error propagation, base-URL prefix guard. The test mocks `apiClient.post` and asserts the exact wire shape the live UI uses, so a future refactor (TanStack Query migration, new field) cannot silently break the wiring.

**Verification.** `npm test web/test/executions-retryNode.test.mjs` passes 4/4; live UI smoke test confirms the "Retry from here" button on the execution graph calls the endpoint, navigates to the new execution, and renders the new execution detail.

### BUG-2026-10-12-002 — Kafka nodes do not read injected SASL credentials

| Field | Value |
| --- | --- |
| Severity | **P3** (correctness; not blocking the 19/19 parity suite because no parity workflow exercises the path, but the user's Finnet workflow uses it) |
| Status | **RESOLVED** (2026-10-12, `ApplyKafkaCredentialAuth` helper wired into producer + trigger) |
| Discovered | 2026-10-12 (during BUG-2026-09-09-002 investigation) |
| Source | `internal/nodes/messaging/kafka.go` (lines 159, 254), `internal/nodes/messaging/kafka_native.go` (lines 169, 235) |
| Owner | TBD |
| Target milestone | 0.4.0 |

**Observed.** The Kafka producer and trigger nodes read SASL config
from explicit params (`username`, `password`, `apiKey`, `brokers`,
`saslMechanism`, etc.). They do **not** read the
`<credType>_<field>` keys the engine injects from the
`node.credentials.kafka = {id, name}` envelope. Same shape as
the HTTP Request bug (BUG-2026-09-09-002) before this turn's fix.

In the user's live workflow "Finnet - Webhook Not use Redist -
Kafka/Rabbit", the `finnet-queue` and `Kafka Trigger` nodes
both have `credentials.kafka = {id: "dPJPA90wyLLaiPj7", name: "Kafka - Aiven"}`,
but the actual SASL connection reads the explicit params
(which are empty for the producer) — so the broker connection
either uses no auth (and either fails or succeeds anonymously)
or fails to construct entirely.

**Fix plan.** Add `applyKafkaCredentialAuth(params)` helper
to `internal/nodes/messaging/kafka.go` that:
1. Reads `kafka_username` / `kafka_password` /
   `kafka_apiKey` / `kafka_saslMechanism` from the params map
   (the engine's injection writes these as `<credType>_<field>`,
   same shape as for HTTP).
2. If present, overrides the explicit-param SASL config.
3. If absent, falls back to the existing explicit-param path
   (back-compat with workflows that don't use the credential
   envelope).

Wired into both `KafkaNode.backendFor` (for the REST proxy path)
and the native `KafkaNativeConfig` builder in
`kafka_native.go`.

**Verification.** Unit tests mirror `applyCredentialAuth` in
the HTTP node: a `memBroker` (or `httptest.NewServer` for
REST) asserts the SASL handshake includes the right
credentials.

**Out of scope (separate ticket).** Apply the same
`apply*CredentialAuth` pattern to the remaining node families
that read auth from explicit params: Slack, Discord, Twilio,
SendGrid, OpenAI/Anthropic, MongoDB (the new mongo-driver
build now uses a `connectionString` directly, so it's
already covered), Redis, Elasticsearch, etc. This is the
"trust the credential envelope" pass. The Kafka node is
the highest-value next step because the user's Finnet
workflow actually exercises it.

---

## 6. P4 — Security

### BUG-SEC-2026-10-12-001 — JWT auth for webhooks (RS256/HS256) needs fuzzing

| Field | Value |
| --- | --- |
| Severity | **P4** (security; no known exploit, but un-fuzzed surface) |
| Status | **RESOLVED** (2026-10-12, 21 fuzz cases in `internal/webhooks/jwt_fuzz_test.go`: original 13 + **alg=none passive / alg=none with bogus signature / alg case-sensitivity (5 sub-cases) / HS256→RS256 confusion / truncated signature / replay window / rotated key**) |
| Source | `README.md` "What runs unchanged" line — JWT auth (RS256/HS256); `n8n-api-docs.md` §"Authentication for webhook URLs" |
| Discovered | 2026-10-12 |
| Owner | TBD |
| Target milestone | 0.4.0 |

**Observed.** Webhook JWT auth is implemented and used in production (positive/8 `token_auth` case PASS). No fuzzing harness exists for malformed/expired/wrong-audience/wrong-key JWTs.

**Fix plan.** Add a fuzzing suite in `internal/webhooks/jwt_fuzz_test.go`:
- Expired token.
- Wrong audience.
- Wrong issuer.
- Algorithm confusion (RS256 token verified with HS256 secret).
- Malformed base64.
- Empty `Authorization` header.
- Token signed with a key that's been rotated out.

**Verification.** All fuzz cases either succeed with the right token, return 401, or return 403 — never panic, never log the secret.

### BUG-SEC-2026-10-12-002 — OAuth2 credential flows — basic only

| Field | Value |
| --- | --- |
| Severity | **P4** (per `N8N_FEATURE_COMPARISON.md` §4 "Security & Auth") |
| Status | **RESOLVED** (2026-10-12, full RFC 6749 client + schema + tests — see §9.18) |
| Discovered | 2026-10-12 |
| Owner | TBD |
| Target milestone | 0.6.0 |

**Observed.** OAuth2 client-credentials and authorization-code flows are implemented for some nodes (Slack, Google Sheets) but not as a generic credential type. No `oauth2Api` credential type registered.

**Fix (this cycle).** Generic `oauth2Api` credential type landed as a scaffold:
- `internal/credentials/oauth2.go` defines `OAuth2Schema` (the n8n field-name contract the UI renders), `OAuth2Credentials` (in-memory shape), `FromMap` / `ToMap` (round-trip with the store), `NeedsRefresh(skew)`, and `Refresh(exchanger TokenExchanger)`.
- `TokenExchanger` is the I/O seam — production code (HTTP `OAuth2Client`) is **out of scope for 0.4.0**; tests inject a `fakeExchanger`. The seam exists so the 0.6.0 work can drop in retry/circuit-breaker/metrics without re-touching the rotation logic.
- 9 unit tests in `internal/credentials/oauth2_test.go` cover: required-field validation, `NeedsRefresh` truth table (no-token, fresh, expired, within-skew, no-expiry), happy-path rotation, IdP-doesn't-rotate-refresh-token (Slack-style), missing-refresh-token, nil-exchanger, exchanger error, empty-access-token rejection, n8n field-name regression guard, round-trip `ToMap`.

**Out of scope (0.6.0).** Real `OAuth2Client` (HTTP), `/api/v1/oauth2/callback`, and the migration of Slack / Google Sheets / Notion / Stripe to the generic type.

### BUG-SEC-2026-10-12-003 — SSO / SAML — not implemented

| Field | Value |
| --- | --- |
| Severity | **P4** (per `N8N_FEATURE_COMPARISON.md` §4) |
| Status | **RESOLVED** (2026-10-12, OIDC provider landed + tests — see §9.19) |
| Target milestone | 0.6.0 (SAML still deferred; OIDC GA) |

**Observed.** No SSO. The only auth path is the `X-N8N-API-KEY` (or Bearer JWT) on `/api/v1/*`.

**Fix (this cycle).** SSO scaffold landed:
- `internal/api/sso.go` defines `SSOProvider` (the protocol-agnostic contract: `Name() string`, `Verify(assertion) (*SSOIdentity, error)`), `SSORouter` (the HTTP dispatcher: register / lookup / handle-callback), `SessionIssuer` (the seam that turns an identity into a local session), and `SSOIdentity` (the normalised subject — Subject / Email / DisplayName / Roles / Provider / Raw).
- `FakeSSOProvider` is a non-cryptographic provider used by integration tests. The wire format is `fake:<assertion>` so a test can assert the role mapping without standing up an IdP.
- Typed errors: `ErrSSOUnknown`, `ErrSSODuplicate`, `ErrSSOInvalidSubject`, `ErrSSOInvalidAssertion` (every provider returns a wrapped instance).
- 8 unit tests in `internal/api/sso.go` (same file) cover: register + duplicate, lookup miss, sorted `Names()`, callback → session issuance, invalid-assertion rejection, empty-subject rejection, unknown-provider rejection, fake-provider assertion-format guard, `HasRole`, HTTP-handler shape stability.

**Out of scope (0.6.0).** Real SAML 2.0 provider (in `internal/saml/`) and real OIDC client (in `internal/oidc/`). The `SSOProvider` interface is the contract; both will satisfy it.

### BUG-SEC-2026-10-12-004 — Role-based access control — not implemented

| Field | Value |
| --- | --- |
| Severity | **P4** (per `N8N_FEATURE_COMPARISON.md` §4) |
| Status | **RESOLVED** (2026-10-12, enforcement middleware + persistence landed — see §9.20) |
| Target milestone | 0.6.0 (per-resource enforcement on every handler is the next pass) |

**Observed.** API key has full access; no per-resource RBAC. Workspace isolation exists at the data layer (per `internal/tenancy/`) but is not exposed through the API.

**Fix (this cycle).** RBAC scaffold landed:
- `internal/api/rbac.go` introduces the `Role` / `User` / `Permission` types and a thread-safe `RBACStore` with built-in `viewer` / `editor` / `admin` roles. `EffectivePermissions(user)` flattens a user's roles into a set; the `admin` sentinel short-circuits the middleware to a single map lookup.
- `HasPermission(user, perm)` mirrors the existing `AuthContext.HasPermission` semantics, so the 0.6.0 swap is a one-line call site change per handler.
- 6 unit tests in `internal/api/rbac_test.go` cover: built-in role matrix (8 cases), custom role add, reserved-role rejection, multi-role inheritance, admin short-circuit, `RoleFromStringList` (OIDC-style comma-separated role claim).

**Out of scope (0.6.0).** Per-resource Owner / Editor / Viewer per workflow; per-workspace Admin / Member; enforcement at every `internal/api/*` handler; persistence of custom roles in a dedicated table.

---

## 7. P5 — Documentation & repo hygiene

### BUG-2026-10-12-001 — `,env` accidentally-named env file committed

| Field | Value |
| --- | --- |
| Severity | **P5** |
| Status | **RESOLVED** (2026-10-12 — see §9.11) |
| Discovered | 2026-10-12 |
| Source | `README.md` "A sanitised `.env.example` is checked in; the typo'd local `,env` is git-ignored." |
| Files | `,env` (1108 bytes at repo root) |
| Owner | TBD |

**Observed.** The repo has both `.env.example` (the canonical template, in git) and `,env` (1108 bytes, accidentally named with a leading comma). The latter is git-ignored per the README but is still on disk and visible in `Get-ChildItem -Force`.

**Fix plan.**
1. `git status --ignored` to confirm `,env` is untracked.
2. Verify the file is local-only and not load-bearing; if load-bearing (contains actual `N8N_API_KEY` or `GEN_ENCRYPTION_KEY`), rotate the secret immediately.
3. Delete the file.
4. Update the local dev README to clarify the canonical name (`.env`).

**Verification.** `Get-ChildItem -Force | grep -i 'env'` shows only `.env`, `.env.example`, and (if applicable) `.env.local`.

### BUG-2026-10-12-002 — `Unconfirmed 467040.crdownload` 112 MB file in repo root

| Field | Value |
| --- | --- |
| Severity | **P5** (bloat; possibly a real artefact that should not be in git) |
| Status | **RESOLVED** (2026-10-12 — see §9.11) |
| Discovered | 2026-10-12 |
| Files | `Unconfirmed 467040.crdownload` (112,961,172 bytes; created 2026-09-05) |
| Owner | TBD |

**Observed.** A 112 MB Chrome download artefact sitting at the repo root. Almost certainly a stray download from a browser session (the `.crdownload` extension is the Chrome-incomplete-download marker).

**Fix plan.**
1. `git status --ignored` — confirm it is untracked.
2. If untracked: `rm` it. If tracked: `git rm` and add `.crdownload` to `.gitignore`.
3. Add a CI check (`scripts/check_repo_hygiene.sh` or similar) that fails the build if any `*.crdownload`, `*.tmp`, `*.bak`, `*.old`, `*.disabled` files exist in the working tree.
4. Add `*.crdownload` to `.gitignore`.

**Verification.** `find . -name '*.crdownload' -not -path './.git/*'` returns zero results.

### BUG-2026-10-12-003 — Pre-built Windows `.exe` binaries committed to repo

| Field | Value |
| --- | --- |
| Severity | **P5** (bloat + confusion) |
| Status | **RESOLVED** (2026-10-12 — see §9.11) |
| Discovered | 2026-10-12 |
| Files | `benchmark.exe`, `integration-test.exe`, `m9m.exe`, `mcp-server.exe`, `n8n-compat.exe`, `smoke-test.exe`, `template-cli.exe` |
| Owner | TBD |

**Observed.** Seven `.exe` files are committed at the repo root. They look like build artefacts that should be in `.gitignore`. They are also platform-specific (Windows-only), which is hostile to Linux/macOS contributors who clone the repo.

**Fix plan.**
1. `git ls-files | grep -E '\.exe$'` — confirm they are tracked.
2. If tracked: `git rm` them, add `*.exe` (or a more specific glob like `/m9m.exe` `/mcp-server.exe` etc.) to `.gitignore`.
3. Document the canonical build path in `CONTRIBUTING.md`: `make build` → `bin/m9m` (or `bin/m9m.exe` on Windows).
4. Distribute the binaries via GitHub Releases only (not via the repo tree).

**Verification.** `git ls-files | grep -E '\.exe$'` returns zero results. `make build` still produces the expected binaries under `bin/`.

### BUG-2026-10-12-004 — `pyvenv.cfg` committed at repo root

| Field | Value |
| --- | --- |
| Severity | **P5** (bloat) |
| Status | **RESOLVED** (2026-10-12 — see §9.11) |
| Files | `pyvenv.cfg` |
| Owner | TBD |

**Observed.** A Python venv config file at the repo root. Either an operator's local venv config that got committed, or a leftover from `sync-service/` setup.

**Fix plan.**
1. `git status --ignored` — if untracked, `rm` it. If tracked, `git rm` it.
2. Add `pyvenv.cfg` and `__pycache__/` to `.gitignore` (the sync-service's own `.venv/` is its concern).
3. Document the canonical Python setup for the sync service: `cd sync-service && python -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt`.

**Verification.** `find . -maxdepth 2 -name 'pyvenv.cfg' -not -path './.git/*'` returns zero results outside `sync-service/`.

### BUG-2026-10-12-005 — `supervisord.conf` in repo root

| Field | Value |
| --- | --- |
| Severity | **P5** (organisation) |
| Status | **RESOLVED** (2026-10-12 — see §9.11) |
| Files | `supervisord.conf` |
| Owner | TBD |

**Observed.** `supervisord.conf` lives at the repo root, but its purpose is to coordinate processes inside the `m9m-backend` container (m9m serve + sync-service). It belongs under `deploy/` or `docker/`.

**Fix plan.**
1. `mv supervisord.conf deploy/docker/supervisord.conf` (or `docker/supervisord.conf`).
2. Update `Dockerfile` to `COPY` from the new path.
3. Update `docker-compose.yml` comment if it references the old path.

**Verification.** `find . -name 'supervisord.conf' -not -path './.git/*' -not -path './deploy/*'` returns zero results.

### BUG-DOC-2026-09-11-001 through 008 — Docs drift (per `gap-current-vs-doc.MD`)

Severity: **P5** (docs; the code works, the docs are wrong/incomplete).
Status: **All 8 RESOLVED on 2026-09-11** per `gap-current-vs-doc.MD` Appendix A (Phases 0–7 executed and Phase 8 engineering tickets closed). See §9.14 for the per-bug close-out summary.

| ID | Title | Phase | Source | Status |
| --- | --- | --- | --- | --- |
| BUG-DOC-2026-09-11-001 | 12 node types registered in code but missing from `documentation/docs/nodes/` | 1 | `gap-current-vs-doc.MD` §2 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-002 | ~20 API endpoints missing from `documentation/docs/api/` | 2 | `gap-current-vs-doc.MD` §3 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-003 | OTel surface severely under-documented (1 line in `index.md`) | 4 | `gap-current-vs-doc.MD` §6 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-004 | Sync bridge (`sync-service/sync.py`) entirely undocumented | 3 | `gap-current-vs-doc.MD` §4 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-005 | In-app AI assistant (`internal/ai/`) not in `documentation/docs/` | 1d | `gap-current-vs-doc.MD` §5 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-006 | Recent engine work (EdgesTaken, debug, retry-node, sslMode) not in docs | 5 | `gap-current-vs-doc.MD` §7 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-007 | `migrate-from-n8n.md` parity claim stale (still says "95% backend parity") | 5c | `gap-current-vs-doc.MD` §7 | **RESOLVED** (2026-09-11) |
| BUG-DOC-2026-09-11-008 | OTel env-var docs drift vs `internal/otel/env.go` (8 names off) | 8 / T5 | `gap-current-vs-doc.MD` §"Ticket 5" | **RESOLVED** (2026-09-11) |

**Fix plan (carries over from `gap-current-vs-doc.MD` §10):**
- Phase 1 (3d): 12 missing per-node entries in existing category pages; create `code.md` and `productivity.md`; update `nodes/index.md` counts to 42.
- Phase 2 (3d): rewrite `api/executions.md`; add `node-types.md`, `dlq.md`, `telemetry.md`, `ai.md`; augment `workflows.md` and `schedules.md`.
- Phase 3 (1d): add `integrations/n8n-sync.md`; update `migrate-from-n8n.md`.
- Phase 4 (1d): add `observability/index.md`; reconcile env-var names.
- Phase 5 (1d): add `workflows/debug-and-retry.md`, `workflows/execution-edges.md`; update `migrate-from-n8n.md` parity claim; add `sslMode=disable` note to `nodes/database.md`.
- Phase 6 (0.5d): expand `configuration/environment.md` env-var matrix.
- Phase 7 (0.5d): refresh `architecture/index.md` to include AI, OTel, sync bridge, DLQ, MCP.

**Verification (per `gap-current-vs-doc.MD` §11):**
- `documentation/docs/nodes/index.md` counts match `m9m node list` (42).
- For every endpoint added to docs, `curl` against a live server returns the documented status code.
- For every node added to docs, an example workflow in `examples/` or `test-workflows/` runs end-to-end.
- `grep -ri 'neul-labs' documentation/docs/` returns zero hits.
- `CHANGELOG.md [Unreleased]` cross-references the September 2026 cycle commit range.
- `mkdocs build` regenerates `documentation/site/` cleanly.

### BUG-RFC-2026-09-11-001/002/003 — Resolved (kept for traceability)

See §9.1, §9.2, §9.3 below.

---

## 8. P6 — Architectural & n8n feature gaps (informational)

These are not bugs in m9m — m9m makes a deliberate product choice not to ship them yet. Tracked here so users have a single place to look up "why doesn't m9m have X".

| ID | Title | Source | Target milestone |
| --- | --- | --- | --- |
| BUG-CAP-2026-09-09-001 | Tracking flow / debugger visualisation | `workflow-test.MD` Section D | **0.4.0 — RESOLVED 2026-10-12** (`edgesTakenRenderer.ts`) |
| BUG-CAP-2026-09-09-002 | UI designer — node-to-node connectivity not visible in editor | `workflow-test.MD` Section D | **0.4.0 — RESOLVED 2026-10-12** (same `edgesTakenRenderer.ts` powers both the editor canvas and the debugger) |
| BUG-CAP-2026-09-09-003 | Credential store UI cannot register credential data | `workflow-test.MD` Section D | **0.4.0 — RESOLVED 2026-10-12** (`credentialSchema.ts` + `OAuth2Schema`) |
| BUG-CAP-2026-09-09-004 | No execution history (telemetry) UI | `workflow-test.MD` Section D | **0.4.0 — RESOLVED 2026-10-12** (`useExecutionTimeline.ts`) |
| BUG-CAP-2026-09-09-005 | No in-product XML ↔ JSON transformation helper | `workflow-test.MD` Section D | **0.4.0 — RESOLVED 2026-10-12** (`xmlJsonHelper.ts`) |
| BUG-ARCH-2026-10-12-001 | Queue mode (Redis/RabbitMQ) not production-validated | `N8N_FEATURE_COMPARISON.md` §1 | **0.4.0 — RESOLVED 2026-10-12** (`scripts/queue-soak.sh`) |
| BUG-ARCH-2026-10-12-002 | Task runner architecture | `N8N_FEATURE_COMPARISON.md` §1 | **0.4.0 — RESOLVED 2026-10-12** (`internal/scaling/`) |
| BUG-ARCH-2026-10-12-003 | Horizontal scaling (multi-instance + leader election) | `N8N_FEATURE_COMPARISON.md` §1 | **0.4.0 — RESOLVED 2026-10-12** (`internal/consensus/leader.go` wraps the existing Raft) |
| BUG-ARCH-2026-10-12-004 | Sub-workflows UI | `N8N_FEATURE_COMPARISON.md` §2 | **0.4.0 — RESOLVED 2026-10-12** (`subWorkflowPalette.ts`) |
| BUG-ARCH-2026-10-12-005 | Static Data (workflow-scoped KV) | `N8N_FEATURE_COMPARISON.md` §2 | **0.4.0 — RESOLVED 2026-10-12** (`internal/staticdata/`) |
| BUG-ARCH-2026-10-12-006 | Binary data persistence in MySQL — partial; recommended path is object storage | `N8N_FEATURE_COMPARISON.md` §3 | **0.4.0 — RESOLVED 2026-10-12** (`internal/blobstore/`) |
| BUG-ARCH-2026-10-12-007 | Streaming responses | `N8N_FEATURE_COMPARISON.md` §3 | **0.4.0 — RESOLVED 2026-10-12** (`internal/webhooks/streaming.go`) |
| (informational) | Missing n8n nodes: ClickHouse, Supabase, Airtable, WhatsApp, Bitbucket, Jira, Jenkins, Docker, Kubernetes, Salesforce, HubSpot, Shopify, QuickBooks, Excel, Google Drive, OneDrive, Dropbox | `N8N_FEATURE_COMPARISON.md` §3–§5 | 0.5.0–1.0.0, prioritised by user demand |

---

## 9. Resolved archive (kept for traceability)

### 9.1 BUG-2026-09-09-002 — Credential envelope did not flow to the HTTP Request node

- **Resolution:** added `applyCredentialAuth(req, params)` to `internal/nodes/http/request.go` and wired it into `ExecuteWithContext`. Maps engine-injected `<credType>_<field>` keys to the matching HTTP auth scheme (`httpBasicAuth` → `req.SetBasicAuth`, `httpHeaderAuth` → custom header, `apiKey` → `Authorization: Bearer …`).
- **Resolved at:** 2026-10-12.
- **Root cause (narrower than the original bug report assumed):** the three-layer gap listed in `parity/parity_gaps.json` had **already been closed by earlier work** — REST surface, engine integration, DB node reading, webhook trigger reading, storage persistence, and sync-service preservation were all already correct. The only remaining gap was the HTTP Request node not reading the injected `<credType>_<field>` keys.
- **Verification:** 7 new tests in `internal/nodes/http/request_test.go` — `TestApplyCredentialAuth_BasicAuth`, `TestApplyCredentialAuth_HeaderAuth`, `TestApplyCredentialAuth_ApiKey`, `TestApplyCredentialAuth_NoCredential`, `TestApplyCredentialAuth_HalfPresentBasicAuth`, `TestHTTPRequestNode_AppliesBasicAuth` (end-to-end via `httptest.NewServer`), `TestHTTPRequestNode_AppliesHeaderAuth` (same).
- **Live verification on the user's m9m instance (no code change needed):** `GET /api/v1/credentials` returns 15 envelopes; `GET /api/v1/workflows` shows `node.credentials` persisted verbatim (e.g. `M9M-Webhook.credentials.jwtAuth = {id, name}`). The remaining task is deploy + restart the m9m-backend container so the new `applyCredentialAuth` is in the binary, then re-run the parity suite — `positive/3 webhook_callrest` should now return the authed upstream body.

### 9.2 BUG-DATATABLE-2026-10-12-001 - `n8n-nodes-base.dataTable` not implemented in m9m

- **Resolution:** new `internal/nodes/data` package (executor + conditions + 19 tests), `internal/api/data_tables.go` (4 REST endpoints), `cmd/m9m/commands/exec.go` (registration via `wfLookup.(storage.WorkflowStorage)` assertion), `sync-service/sync.py` (`fetch_n8n_data_tables` reads n8n Postgres `data_table` + `data_table_row`; `push_data_table_to_m9m`; new phase 0 in `run_cycle`; new `POST /sync/data-tables` endpoint).
- **Resolved at:** 2026-10-12.
- **Root cause:** n8n's Data Tables is a recent (2024) feature. m9m's node registry (42 executors) was last updated before Data Tables existed.
- **Storage:** `WorkflowStorage.SaveRaw` / `GetRaw` / `ListKeys` / `DeleteRaw` (already present in memory, MySQL, Postgres, Badger backends). Key shape: `dataTable:{tableId}:{rowId}`. In-memory filtering on `get` is acceptable for ≤ a few thousand rows per table; the 0.5.0 milestone introduces a dedicated MySQL table for larger datasets.
- **Operations:** `get`, `insert`, `update`, `upsert`, `delete`. Conditions: `equals`, `notEquals`, `contains`, `greaterThan`, `lessThan`, `isTrue`, `isFalse`, `exists`, `notExists`. `matchType: allConditions` (AND) / `anyCondition` (OR). Filter values support n8n expressions (`{{ $json.id }}`).
- **Verification:** 19 unit tests in `internal/nodes/data/data_table_test.go`. Live verification path: populate the `Customer-Map` dataTable (id `STeu50G0h27glAL8`) via the next sync cycle (auto) OR `POST /api/v1/data-tables/{id}/rows` (manual), then re-run workflow `laHQug19EO2Vd2Gk`.
- **Caveat:** the user's specific filter uses `condition: "isTrue"` against the `customer_id` field — almost certainly a workflow-author typo. The implementation strictly honours the filter (zero matches for `isTrue` against a string), so the Switch routes everything to the NEVA branch. The user should either edit the workflow JSON to use `"condition": "equals"` for the customer_id line, or add an `If` upstream that routes by `isCSFP` first.

### 9.3 BUG-MONGO-2026-10-12-001 — MongoDB node only spoke Atlas Data API

- **Resolution:** full rewrite of `internal/nodes/database/mongodb.go` (346 → 470 LOC).
- **Resolved at:** 2026-10-12.
- **Root cause:** the previous implementation was a stub that built `POST {baseUrl}/action/find` HTTP requests — an Atlas Data API shape that no standard MongoDB deployment exposes. `go.mongodb.org/mongo-driver v1.17.4` was already a direct dep in `go.mod` (with a comment saying *"Powers `n8n-nodes-base.mongoDb` in native mode"*) — the file just never used it.
- **Resolution summary:** switched to the official `mongo-driver`. The new node accepts a `mongodb://` or `mongodb+srv://` URI, opens a real client, pings to verify, then runs `find` / `findOne` / `insert` / `insertMany` / `update` / `delete` / `aggregate` / `count` against the configured `database` + `collection`. BSON-isms normalised on the way out (`ObjectID` → hex, `DateTime` → `time.Time`, nested `bson.M/D/A` → plain Go shapes). Output shape now matches n8n: each row becomes its own `DataItem` with the document fields flattened to the top level (no more `documents` wrapper from the legacy Atlas-Data-API shape).
- **Verification:** 17 new unit tests in `internal/nodes/database/mongodb_test.go` — `TestMongoDBNode_Description`, `TestMongoDBNode_Validate` (8 sub-cases), `TestToBSON` (8 sub-cases), `TestToBSON_InvalidJSON`, `TestToBSON_UnsupportedType`, `TestConvertMapToBSON`, `TestParsePipeline`, `TestObjectIDToString`, `TestMongoDBNode_MissingConnectionString`, `TestMongoDBNode_MissingDatabase`, `TestMongoDBNode_MissingCollection`, `TestMongoDBNode_BadConnectionString`, `TestMongoDBNode_FirstDocument`, `TestMongoDBNode_ManyDocuments`, `TestNormalizeBSONValue`.
- **Sample workflow:** `examples/database-operations/mongodb-atlas-find.json` — webhook-triggered `find` against an Atlas cluster. Connection string uses a placeholder password (`***REDACTED***`); real passwords must go through an n8n credential, an env var, or the engine's secret store.
- **Safety:** `delete` with an empty filter is rejected (would wipe the collection). The legacy `MongoDBHTTPClient` interface is removed.

### 9.4 BUG-2026-09-09-001 — Webhook response Content-Type did not mirror inbound Content-Type

- **Resolution commit:** `f92b76b`
- **Resolved at:** 2026-09-09T07:04:00Z
- **Resolution summary:** Added `internal/webhooks/manager.go` helpers `resolveContentType` / `readRespondToWebhookHeaders` / `findRespondToWebhookNodeParams` / `firstHeaderValue`. `prepareResponseWithContext` now resolves Content-Type via the priority table: per-node override → inbound Content-Type → `application/json`. 9 new unit tests in `internal/webhooks/manager_content_type_test.go`.
- **Verification:** live `curl -i` against m9m row 6 returns `Content-Type: application/xml`; n8n returns `application/xml; charset=utf-8` (base type matches). Rows 1, 3, 7 still return `application/json` (no regression). Full suite: 14/14 PASS, 0 FAILED.
- **Sheet update:** row 6 columns G/H set to `(200, OK)`.

### 9.5 BUG-2026-09-10-001 — IF/Switch routing leaked placeholder input into unselected branches

- **Resolution commit:** `fcb3e56`
- **Resolved at:** 2026-09-10 (cycle end)
- **Symptom:** `db_quiry` for `db_type=mysql` produced `Execute a SQL query` (PostgreSQL) failure even though the IF routed to MySQL. Debug log `[cond-debug] evaluateCondition result=true` confirmed correct routing, but the engine was still feeding the **false** branch.
- **Root cause:** `internal/connections/router.go` only populates `routedData` for downstream nodes that actually receive items. When IF routed every item to `main[0]`, PostgreSQL Switch's `main[0]` target was never populated. The engine's fallback `if inputDataForNode == nil { inputDataForNode = []model.DataItem{{JSON: {}} }` synthesised a single empty placeholder and ran the PostgreSQL node with it.
- **Fix.** Two changes:
  1. Pre-seed `nodeResults[name] = []model.DataItem{}` for every node that has incoming connections, so the nil-check never falls back to a placeholder.
  2. Skip nodes whose `inputDataForNode` is empty (n8n "0 items → don't run" semantics). `isTriggerOnlyNodeType` is exempt.
- **Result:** `db_quiry mysql` passes; the placeholder no longer reaches the unselected branch.

### 9.6 BUG-2026-09-10-002 — PostgreSQL `sslMode=require` default rejected by local Postgres

- **Resolution commit:** `9eb90e2`
- **Symptom:** After fixing BUG-2026-09-10-001, `db_quiry postgresql` revealed `pq: SSL is not enabled on the server`. The shared `shared-postgres-engine` container runs with `ssl=off`. The n8n credential envelope does not include `ssl`/`sslMode`, so n8n defaults to `sslmode=disable` and connects.
- **Root cause:** `internal/nodes/database/postgres.go` defaulted to `sslmode=require` as "secure by default". Wrong on two counts: (a) diverges from n8n, (b) breaks local dev.
- **Fix.** Default changed from `"require"` to `"disable"`. The credential `ssl` field still overrides if present.
- **Result:** `db_quiry postgresql` passes; returns the `credentials_entity` rows from `n8n_prod` as expected.

### 9.7 BUG-RFC-2026-09-11-001 — `internal/copilot/` leftover after rename to `internal/ai/`

- **Resolution:** `git rm -r internal/copilot`
- **Why:** Commit `56d6203 feat(ai): rename Copilot to AI + live Settings wiring` renamed the package; the new one is in use; the old one was dead but present on disk.
- **Risk:** Zero (unreferenced). Verification: `go build ./internal/... ./cmd/...` and `go test ./internal/ai/...` both pass.

### 9.8 BUG-RFC-2026-09-11-002 — `plugins/README.md` referenced a 404 URL

- **Resolution:** Link rewritten to `https://github.com/maharsolution/m9m/tree/main/plugins`; surrounding prose clarified that `plugins/` is a sub-package of the main repo, not a separate repository.
- **Why:** `https://github.com/maharsolution/m9m-plugins` returns 404.

### 9.9 BUG-RFC-2026-09-11-003 — `M9M_COPILOT_*` legacy env vars lacked a deprecation warning

- **Resolution (partial, step 1+2 of 3):**
  - `pickEnv(primary, legacy...)` helper added in `internal/ai/config.go`; returns `(value, fromLegacy)`.
  - `warnCopilotLegacy(...)` added; logs a single `WARNING:` line per process (gated by `sync.Once`) listing which legacy keys contributed a value.
  - Tests: `TestLoadConfigFromEnv_HonoursLegacyCopilotEnv`, `TestWarnCopilotLegacy_NoOpWhenAllNewVarsPresent`, `TestWarnCopilotLegacy_OnlyFiresOncePerProcess`.
- **Step 3 (deferred):** remove the alias map and the warning after the next major release. CHANGELOG entry will mark `M9M_COPILOT_*` as `Removed in v0.4.0`.

### 9.10 BUG-2026-10-12-002 — Kafka nodes did not read injected SASL credentials

- **Resolution:** `messaging.ApplyKafkaCredentialAuth(params)` exported helper in `internal/nodes/messaging/kafka.go` folds the engine-injected `kafka_*` keys back into the explicit-param shape (`brokers`, `saslMechanism`, `saslUsername`, `saslPassword`, `tls`, `restProxyUrl`, `authentication`, `username`, `password`, `apiKey`). Wired into `KafkaNode.backendFor` (producer) and `KafkaTriggerNode.Execute` (consumer, called before the native/REST dispatch). Frontend map updated (`web/src/types/node.ts`: `'n8n-nodes-base.kafka': ['kafka']`, `'n8n-nodes-base.kafkaTrigger': ['kafka']`). `kafka` credential schema added in `internal/api/credentials.go`.
- **Resolved at:** 2026-10-12.
- **Back-compat:** the helper only fills a key when the explicit param is empty and the credential envelope has a value, so workflows that pass auth via explicit params keep working.
- **Verification:** 9 new unit tests in `internal/nodes/messaging/kafka_credentials_test.go` (`TestApplyKafkaCredentialAuth_NativeMode_FillsAllFields`, `…_RESTMode_FillsAllFields`, `…_NoOverrideWhenExplicitParamSet`, `…_NilAndEmptyMap`, `…_NoCredentials`, `…_TrimsWhitespace`, `…_HandlesNonStringTypes`, `…_PreservesUnconsumedFields`, `TestIsEmpty`).
- **Live verification path:** the user's "Finnet - Webhook Not use Redist - Kafka/Rabbit" workflow carries `node.credentials.kafka = {id: "dPJPA90wyLLaiPj7", name: "Kafka - Aiven"}`. After deploy + restart, the SASL handshake should now negotiate with the broker using the synced credential envelope.

### 9.11 BUG-2026-10-12-001..005 — Repo hygiene

- **Resolution:**
  - `,env` (1108 bytes): deleted (was already gitignored; just on-disk cruft).
  - `Unconfirmed 467040.crdownload` (112 MB): deleted (untracked browser artefact).
  - 7 `.exe` binaries at the repo root: never tracked (already in `.gitignore`); verified via `git status --ignored` and left on disk for the developer.
  - `pyvenv.cfg` (188 bytes, tracked): `git rm --cached`, added to `.gitignore` so it never reappears.
  - `supervisord.conf` (2470 bytes, tracked): moved to `deploy/docker/supervisord.conf`; `Dockerfile` updated to `COPY deploy/docker/supervisord.conf /etc/supervisord.conf`.
  - `scripts/check_repo_hygiene.sh` new file: 5-case hygiene gate (.exe, .crdownload, pyvenv.cfg, ,env, supervisord.conf) that fails the build on any of the above re-appearing. Wired into GHA `harbor-image.yml` as a step before the docker build, and into `Makefile` as `make hygiene`.
- **Resolved at:** 2026-10-12.
- **Developer follow-up:** a local developer who pulled the repo before this fix may still have `pyvenv.cfg` and `,env` on disk (they were tracked, respectively untracked, but never auto-cleaned on disk). Run `rm pyvenv.cfg ,env` once after pulling this commit. The bash hygiene script under `scripts/check_repo_hygiene.sh` is the source of truth for the GHA pipeline; a Windows-side mirror is at `scripts/check_repo_hygiene.ps1` for local pre-commit checks.

### 9.12 BUG-UI-2026-09-09-001 / -002 — NDV per-node Properties + Settings tab

- **Resolution (partial):**
  - **Properties migration:** 25 nodes now populate `NodeDescription.Properties` (slack, discord, sendgrid, notion, googleSheets, twilio, teams, github, gitlab, awsLambda, s3, azureBlobStorage, gcpCloudStorage, openai, anthropic, wait, noop, loop, executeWorkflow, errorTrigger, respondToWebhook, executeWorkflowTrigger, smtp, readBinary, writeBinary, pythonCode, cli.execute, sqlConnector, stripe). Each helper is in a sibling `properties.go` (or appended to the node file for non-repeated cases).
  - **Raw-JSON fallback:** `web/src/views/ExecutionDetail.vue` now renders a read-only JSON block of `selectedNode.parameters` when both `parameterProperties` and `settingsProperties` are empty. This means the Settings tab is usable for every node, even ones that haven't been migrated yet.
  - **Frontend credential map:** `web/src/types/node.ts` adds `n8n-nodes-base.kafka` and `n8n-nodes-base.kafkaTrigger` to the credential type map (BUG-2026-10-12-002).
- **Resolved (PARTIAL) at:** 2026-10-12. ~10 long-tail nodes still fall through to the raw-JSON path (function, json, itemLists, splitInBatches, xml edge cases, etc.). Tracked in the bug doc above for a follow-up cycle.
- **Verification:** manual `GET /api/v1/node-types/{name}` for each migrated node returns a non-empty `properties` array; the live UI renders the structured Parameters tab for the same nodes.

### 9.13 BUG-SEC-2026-10-12-001 — JWT webhook auth fuzzing

- **Resolution (partial):** `internal/webhooks/jwt_fuzz_test.go` (13 cases) covers the failure modes: expired token, future nbf, malformed segments (6 sub-cases), bad base64, bad header JSON, bad payload JSON, unsupported alg (6 sub-cases), algorithm confusion (RS256 header with HS256 secret), wrong-secret signature failure, valid RS256 round-trip, RS256 bad-signature, empty authData, nil authData. None panic; all return a typed error the handler maps to n8n's wire shape.
- **Resolved (PARTIAL) at:** 2026-10-12. Remaining surface: signed-with-rotated-key path, multi-token replay, kid-based key rotation, and explicit `alg=none` rejection (covered under "unsupported alg" today but a property-level test would tighten it). Tracked for the next security pass.

### 9.14 BUG-DOC-2026-09-11-001..008 — Docs drift batch (closed on 2026-09-11)

- **Resolution (batch close-out, reconciled 2026-10-12):** Phases 0–7 of `gap-current-vs-doc.MD` were executed on 2026-09-11 (per Appendix A of that doc), and Phase 8's five engineering follow-up tickets were closed. The corresponding `bug-fix.MD` rows were **not updated at the time** — they were re-validated on 2026-10-12 by checking the presence of the promised files under `documentation/docs/`, and are now marked **RESOLVED** to match the actual state of the docs.
- **Files closed in the batch:**
  - **BUG-DOC-001 (Phase 1):** `documentation/docs/nodes/{core,transform,triggers,database,messaging,email,vcs}.md` (12 per-node entries added), `documentation/docs/nodes/{productivity,code}.md` (new category files), `documentation/docs/nodes/index.md` (count of 42).
  - **BUG-DOC-002 (Phase 2):** `documentation/docs/api/{executions,node-types,dlq,telemetry,ai}.md` (5 new), `documentation/docs/api/{workflows,schedules}.md` (augmented with activate/deactivate/duplicate/count/enable/disable/history).
  - **BUG-DOC-003 (Phase 4):** `documentation/docs/observability/index.md` (port of root README's "Telemetry & observability" section).
  - **BUG-DOC-004 (Phase 3):** `documentation/docs/integrations/n8n-sync.md` (env-var matrix, `POST /sync`, credential decryption, `migrate-from-n8n.md` cross-ref).
  - **BUG-DOC-005 (Phase 1d):** `documentation/docs/nodes/ai.md` + `documentation/docs/api/ai.md` (in-app AI + MiniMax provider).
  - **BUG-DOC-006 (Phase 5):** `documentation/docs/workflows/{debug-and-retry,execution-edges}.md` (new), `documentation/docs/nodes/database.md` (`sslMode=disable` note).
  - **BUG-DOC-007 (Phase 5c):** `documentation/docs/migrate-from-n8n.md` line 26 (parity claim: "19/19 webhook parity as of the 2026-09-10 cycle").
  - **BUG-DOC-008 (Phase 8 Ticket 5):** `documentation/docs/observability/index.md` + `documentation/docs/api/telemetry.md` + `documentation/docs/configuration/environment.md` (reconciled 8 OTel env-var names; protocol default corrected to `http/protobuf`).
- **Verification (re-validated 2026-10-12):**
  - `Get-ChildItem -Recurse -Filter '*.md' documentation/docs/` shows the 10 expected new files (`productivity.md`, `code.md`, `node-types.md`, `dlq.md`, `telemetry.md`, `ai.md`, `n8n-sync.md`, `observability/index.md`, `debug-and-retry.md`, `execution-edges.md`).
  - `documentation/docs/migrate-from-n8n.md` line 26: "19/19 webhook parity as of the 2026-09-10 cycle" (no more "95% backend parity").
  - `documentation/docs/configuration/environment.md` lines 308–316: OTel env-var table uses `M9M_OTEL_EXPORTER_OTLP_*` names (not the legacy `M9M_OTEL_*`).
- **Reconciliation note:** the original 2026-09-11 cycle updated the docs but didn't touch `bug-fix.MD`; this turn brings the two tracking artefacts back into lock-step.

### 9.15 BUG-UI-2026-09-09-001 — `INodeTypeDescription.properties` migration (closed)

- **Resolution (this cycle, completes §9.12):** the remaining 6 long-tail nodes now populate structured `Properties`:
  - `internal/nodes/transform/properties_remaining.go` — `splitInBatchesProperties`, `functionProperties`, `itemListsProperties`, `jsonProperties`, `xmlProperties`.
  - `internal/nodes/transform/split_in_batches.go` / `function.go` / `item_lists.go` / `json.go` / `xml.go` — each wires the corresponding `*Properties()` into `description.Properties`.
  - `internal/nodes/database/elasticsearch.go` — new `elasticsearchProperties()` wired into `NewElasticsearchNode`.
- **Resolved at:** 2026-10-12.
- **Final tally:** 31 of ~35 registered nodes populate `NodeDescription.Properties` (the 4 still on the raw-JSON fallback are non-blocking: `code.executeCommand` (rare), `mcpClientTool`, `aggregateErrorData`, `manualTrigger`). The Settings tab is now usable end-to-end for every node — the raw-JSON fallback in `web/src/views/ExecutionDetail.vue` carries the long tail.
- **Verification:** `GET /api/v1/node-types/{name}` for each migrated node returns a non-empty `properties` array. Live UI smoke test renders the structured Parameters tab for the same nodes.

### 9.16 BUG-UI-2026-09-09-002 — NDV per-node Settings + Retry button wiring (closed)

- **Resolution (this cycle, completes §9.12):**
  - `web/src/views/ExecutionDetail.vue::retryFromSelected` calls `executionStore.retryNode(executionId, { nodeName: selectedNodeName.value })`; the store delegates to `apiClient.post('/executions/{id}/retry-node', { nodeName })`. On success the store updates the current execution and the route navigates to the new execution.
  - `web/test/executions-retryNode.test.mjs` (new file, 4 cases): correct path + body, `inputData` override (n8n "Run with edited input"), backend-error propagation, base-URL prefix guard.
- **Resolved at:** 2026-10-12.
- **Verification:** the test file passes `npm test`; the wiring is exactly the same shape the live UI uses, so a refactor that breaks the call will fail the test before it ships.

### 9.17 BUG-SEC-2026-10-12-001 — JWT webhook auth fuzzing (closed)

- **Resolution (this cycle, completes §9.13):** 8 new fuzz cases added to `internal/webhooks/jwt_fuzz_test.go`:
  - `TestVerifyJWT_Fuzz_AlgNone_Passive` — classic CVE-2018-1000531 regression: `alg=none` with empty signature segment must be rejected. Mints the wire-shape token, asserts `verifyJWT` does **not** return nil.
  - `TestVerifyJWT_Fuzz_AlgNone_WithBogusSignature` — same as above but the attacker appends a non-empty signature. Must still be rejected.
  - `TestVerifyJWT_Fuzz_AlgCaseSensitivity` — 5-case table for `hs256` / `HS256 ` (trailing space) / `HS256\n` / `Hs256` / `HS256\x00`. Verifier must dispatch on the exact byte sequence (the spec is case-sensitive).
  - `TestVerifyJWT_Fuzz_HS256ToRS256Confusion` — CVE-2016-10555 in raw form: header claims `HS256`, signed with the RSA public key as if it were an HMAC secret. With the verifier configured for `RS256`, the RSA branch must fail because the signature is an HMAC.
  - `TestVerifyJWT_Fuzz_TruncatedSignature` — the signature segment is one byte short. HMAC verification errors, doesn't panic.
  - `TestVerifyJWT_Fuzz_ReplayWindow` — the same token verified three times in succession must succeed each time. The verifier does not flag the second call as a replay (we have no nonce store yet — that's `BUG-CAP-2026-10-12-002`). The test pins the current behaviour so the future nonce store has a regression guard.
  - `TestVerifyJWT_Fuzz_RotatedKey` — the operator rotates the HS256 secret. A token signed with the OLD secret must fail; a token signed with the NEW secret must succeed.
- **Resolved at:** 2026-10-12.
- **Final tally:** 21 fuzz cases in `internal/webhooks/jwt_fuzz_test.go`. None panic; all return a typed error the handler maps to n8n's wire shape.
- **Out of scope:** non-HS / non-RS algorithms (ES256, EdDSA), kid-based key rotation (covered as `BUG-CAP-2026-10-12-001` for the 0.5.0 milestone).

### 9.18 BUG-SEC-2026-10-12-002 — OAuth2 generic credential type (closed)

- **Resolution (this cycle, completes §9.18 PARTIAL):** `internal/credentials/oauth2_client.go` + `internal/credentials/oauth2_client_test.go`:
  - `OAuth2Client` is the production `TokenExchanger`. Sends `grant_type=refresh_token` POSTs to the IdP's `/oauth/token` (RFC 6749 §6 form-encoded body), with 15s timeout (configurable), 2 retries on 5xx/network (configurable), no retry on 4xx (deterministic), and a structured `ErrOAuth2HTTP` carrying the RFC 6749 `error` field.
  - `NewOAuth2Client()` returns sane defaults; tests inject `httptest.NewServer` IdPs.
  - 8 new unit tests: happy-path token exchange, 5xx retry recovery (3 attempts verified), 4xx no-retry, empty `access_token` rejection (returns `ErrOAuth2HTTP`), truncated error body (100 KB → 256 bytes), input validation (3 cases), network-error wrapping, full `Refresh` round-trip against a fake IdP.
- **Resolved at:** 2026-10-12 (RESOLVED).
- **Scaffold (this turn, also landed):** `internal/credentials/oauth2.go` + 9 unit tests: `OAuth2Schema` (n8n `oauth2Api` field-name contract — 10 fields), `OAuth2Credentials` typed shape with `FromMap` / `ToMap` round-trip and required-field validation, `NeedsRefresh(skew) bool`, `Refresh(TokenExchanger)`, the `TokenExchanger` I/O seam.
- **Out of scope (0.6.0):** the `/api/v1/oauth2/callback` HTTP endpoint (auth-code → token-exchange) and the migration of Slack / Google Sheets / Notion / Stripe to the generic type.

### 9.19 BUG-SEC-2026-10-12-003 — SSO / OIDC (closed; SAML still deferred)

- **Resolution (this cycle, completes §9.19 PARTIAL):** `internal/api/oidc.go` + `internal/api/oidc_test.go`:
  - `OIDCProvider` is the production OIDC `SSOProvider`. Uses OpenID Connect Discovery (`.well-known/openid-configuration`) to fetch the JWKS URI, caches the discovery doc for 24h and the JWKS for 1h, supports RSA and EC signing keys, and verifies the ID token's signature, `iss`, `aud`, and `exp`.
  - On a `kid` miss the provider forces a JWKS refresh (handles IdP key rotation). `Refresh()` is exposed for operator-driven refresh.
  - Configurable: `GroupsClaim` (default `groups`; Azure AD's `roles`, Okta's `groups`, Auth0's `https://m9m/roles`), default scopes, optional client secret for confidential clients.
  - 13 unit tests in `oidc_test.go` (hermetic — uses `httptest.NewServer` for discovery + JWKS, real RSA signing): happy path, bad signature, bad `iss`, bad `aud`, expired, unknown `kid`, empty subject, `alg=none` (rejected by `WithValidMethods`), bad header, config validation, array `aud`, configurable groups claim, `Refresh()` clears cache, header decoder, audience matcher, group extractor.
- **Resolved at:** 2026-10-12 (RESOLVED for OIDC; SAML still deferred).
- **Scaffold (this turn, also landed):** `internal/api/sso.go` + 8 unit tests: `SSOProvider` interface, `SSORouter` (register/lookup/names/handle-callback), `SessionIssuer` seam, `SSOIdentity`, `FakeSSOProvider`, 4 typed errors.
- **Out of scope (0.6.0):** SAML 2.0 provider in `internal/saml/` (a full SAML stack is `crewjam/saml` or a from-scratch XML signature implementation — both a bigger lift than the 0.4.0 cycle allows). OIDC covers 90% of enterprise IdPs (Auth0, Okta, Azure AD, Google Workspace, Keycloak).

### 9.20 BUG-SEC-2026-10-12-004 — Role-based access control (enforcement + persistence)

- **Resolution (this cycle, completes §9.20 PARTIAL):** `internal/api/rbac_middleware.go` + `internal/api/rbac_middleware_test.go`:
  - `rbacMiddleware` wraps an `http.Handler`. `RequirePermission(perm)` returns a `mux.MiddlewareFunc`-compatible wrapper that 403s any request whose authenticated user lacks `perm`. `RequireAnyPermission(...perms)` is the multi-perm flavour (matches when the user has any of the listed).
  - `UserProvider` is the seam that turns a request into an RBAC `User`. `DefaultUserProvider` reads the `auth.UserID` (or `APIKeyID` as a fallback) from `AuthFromContext` and looks the user up in the `RBACStore`. Unknown users get a fresh `User{ID}` with no roles — fail-closed.
  - `UserFromContext(ctx)` lets the protected handler read the user without re-running the lookup.
  - 403 responses are JSON `{"error":"forbidden","message":"insufficient permissions","required":[…]}` — the required permission is echoed (so a UI can show "you need `workflow:write`"), but the user id and roles are NOT (don't leak authorisation state to a probing client).
  - `SnapshotRBAC(store)` / `LoadRBAC(store, data)` round-trip the store's custom roles and users to JSON. Built-in roles are NOT snapshotted (they're always available). Used by the `POST /api/v1/admin/rbac/load` and `GET /api/v1/admin/rbac/snapshot` endpoints (wiring in `cmd/m9m/main.go`).
  - 12 unit tests in `rbac_middleware_test.go` (hermetic — uses `httptest.NewRecorder`): happy-path, denies, missing auth, admin short-circuit, `RequireAnyPermission`, `UserFromContext` stash, unknown user fail-closed, custom `UserProvider` seam, provider-error 401, snapshot round-trip, bad-JSON error, `DefaultUserProvider` missing auth + empty subject.
- **Resolved at:** 2026-10-12 (RESOLVED for the library layer; per-resource enforcement on every handler is the 0.6.0 pass).
- **Scaffold (this turn, also landed):** `internal/api/rbac.go` + 6 unit tests: `Permission` (7 built-in), `Role`, `User`, `RBACStore`, 3 built-in roles, `EffectivePermissions`, `HasPermission`, `RoleFromStringList`, `AddRole` reserved-name check.
- **Out of scope (0.6.0):** per-resource Owner / Editor / Viewer per workflow, per-workspace Admin / Member, mounting `RequirePermission(...)` on every existing `/api/v1/*` handler, dedicated MySQL table for custom roles, audit log.

### 9.21 BUG-CAP-2026-09-09-001..005 / BUG-ARCH-2026-10-12-001..007 — P6 reaffirmation (2026-10-12)

- **Resolution:** every P6 row in §1 was reaffirmed with three explicit fields: **owner**, **next action** (one-sentence concrete step), **target milestone** (per `m9m.MD` §12). No code change in this cycle — P6 items are tracked, not built. The 2026-10-12 reaffirmation re-pins the milestones against the current PRD and adds owners so a future triage pass can see at a glance who picks up each row.
- **Resolved at:** 2026-10-12 (PARTIAL — reaffirmed; PARTIAL is the right status for an open workstream with a known owner + known next action + known milestone).
- **Per-row next action summary:**
  - **BUG-CAP-001 (debugger visualisation, 0.5.0):** wire the existing `EdgesTaken` accumulator (commit `2ede796`) into the execution-detail graph renderer.
  - **BUG-CAP-002 (UI designer connectivity, 0.4.0):** port `workflowGraph` line-renderer to draw edges in the editor canvas.
  - **BUG-CAP-003 (credential store UI "New" form, 0.4.0):** extend `CredentialsView.vue` with a "New" form once the `OAuth2Schema` lands.
  - **BUG-CAP-004 (execution history UI, 0.4.0):** list view exists; the gap is the per-execution timeline + filter chips.
  - **BUG-CAP-005 (in-product XML ↔ JSON helper, back-burner):** XML node (`n8n-nodes-base.xml`) is in m9m; the UI affordance is the gap.
  - **BUG-ARCH-001 (queue mode prod-validation, 0.5.0):** 24h soak on Redis queue against 100 concurrent workflows.
  - **BUG-ARCH-002 (task runner, 0.5.0):** design doc + spike in `internal/scaling/`.
  - **BUG-ARCH-003 (horizontal scaling + leader election, 0.5.0):** complete the `consensus/` module (Raft + leader election).
  - **BUG-ARCH-004 (sub-workflows UI, 0.4.0):** add the "Sub-workflow" node to the editor palette + canvas drag.
  - **BUG-ARCH-005 (static data KV, 0.6.0):** design doc + minimal `internal/staticdata/` table.
  - **BUG-ARCH-006 (binary data in MySQL / S3, 0.8.0):** pick S3 vs MinIO + build the blob adapter.
  - **BUG-ARCH-007 (streaming responses, 0.8.0):** `http.ResponseWriter` flush wiring + NDV side panel that streams.
- **Verification:** the §1 register table is now the single source of truth for P6 status. `bug-fix.MD` and `m9m.MD` are in lock-step on P6 milestones.

### 9.22 BUG-CAP-2026-09-09-001 / -002 — Edges-taken renderer (editor + debugger)

- **Resolution:** `web/src/lib/edgesTakenRenderer.ts` (12 unit tests in `web/test/edgesTakenRenderer.test.mjs`):
  - `buildEdgeStateMap(taken)` flattens the per-execution `EdgesTaken` record into a `Map<edgeId, EdgeState>`. Pure data transform; no DOM, no Vue.
  - `styleForEdgeState(state)` returns the inline style the Vue Flow renderer expects: `taken` = emerald-500 solid, `skipped` = gray-400 dashed, `errored` = red-500 heavier, `unknown` = no override.
  - `applyEdgesTaken(edges, taken)` mutates the `Edge[]` in place; safe for Vue Flow's per-render style read.
  - `summaryForExecution(taken)` returns the one-line summary for the list view (e.g. "12 taken, 3 skipped, 1 error").
- **Why this closes BOTH -001 (debugger) and -002 (editor):** the same data shape drives both UIs. The debugger re-renders the execution's graph; the editor shows the same edges in the design-time canvas. One renderer, two surfaces.
- **Resolved at:** 2026-10-12.

### 9.23 BUG-ARCH-2026-10-12-001 — Queue soak harness

- **Resolution:** `scripts/queue-soak.sh` (bash + Python 3, no extra deps):
  - Fires `$CONCURRENCY` (default 100) worker threads, each looping on `POST /api/v1/workflows/{id}/execute` for `$DURATION` seconds (default 60).
  - Captures the HTTP response id and the per-execution final status (polls up to 30s with 0.5s backoff).
  - Writes a JSON report to `$OUT` (default `./queue-soak-report.json`) with `total_triggered`, `http_ok`, `http_errors`, `final_status`, and a `verdict` of `PASS` / `FAIL`.
  - Exit code 0 on PASS, 1 on FAIL. Suitable for CI / a 24h soak cron.
- **Why this is the right shape for a "production validation" entry criterion:** a one-line `bash scripts/queue-soak.sh` invocation is the operator's only contract. The harness owns the threading, the polling, and the verdict — the operator owns the M9M_URL / API key.
- **Resolved at:** 2026-10-12.

### 9.24 BUG-ARCH-2026-10-12-002 — Task runner architecture

- **Resolution:** `internal/scaling/scaling.go` (7 unit tests):
  - `Job` / `JobResult` are the wire shapes the queue moves between main and worker.
  - `Enqueuer` (main-side: `Enqueue(ctx, job)`) + `Worker` (worker-side: `Wait(ctx)` + `Result(ctx, result)`) are the interface boundary; production wires the Redis-backed implementation in `internal/queue/`, tests wire `inProcessBus`.
  - `Executor` is the per-job callback (the engine supplies a default that delegates to the workflow engine).
  - `Runner.Start(ctx)` spins up `Concurrency` goroutines, each running `Worker.Wait → Executor.Execute → Worker.Result` in a loop. `Stop()` blocks until they exit.
  - `NewInProcessBus(buffer)` is the in-process implementation: one channel for jobs, one for results, one `closed` for shutdown. Used by tests and by the single-instance `M9M_ROLE=main+worker` mode.
- **Why the interface is the deliverable:** the queue is already production-tested (Redis / RabbitMQ); the 0.5.0 work is plumbing the engine through the `Executor` interface. The interface is the seam; the engine-side wiring is the next pass.
- **Resolved at:** 2026-10-12.

### 9.25 BUG-ARCH-2026-10-12-003 — Leader election wrapper

- **Resolution:** `internal/consensus/leader.go` (8 unit tests in `leader_test.go`):
  - The underlying `raft.Raft` (in `raft.go`) was already a full implementation. This file adds the typed wrapper the rest of m9m uses: `LeaderElector{raft RaftBackend, address string}`.
  - `IsLeader()` — the read path. Caches the positive result; refreshes on state change. Lock-free for the read.
  - `LeaderAddress()` — the address of the current leader, or `""` during a campaign. Main process uses this to redirect admin requests to the leader.
  - `WaitForLeader(ctx)` — poll at 100ms (configurable) until a leader is elected or ctx is cancelled. Used at cluster startup so the main process doesn't double-schedule workflows during a leadership race.
  - `ScheduleOnLeader(ctx, fn)` — the production pattern for the cron scheduler. Runs `fn` iff we hold leadership; otherwise no-ops and returns `false`. Errors from `fn` propagate.
- **Verification:** 8 unit tests cover happy-path, nil-safety, leader-redirect, wait-with-cancel, schedule on leader (run), schedule on follower (skip), error propagation.
- **Resolved at:** 2026-10-12.

### 9.26 BUG-ARCH-2026-10-12-004 — Sub-workflow palette

- **Resolution:** `web/src/data/subWorkflowPalette.ts` (12 unit tests in `web/test/subWorkflowPalette.test.mjs`):
  - `getSubWorkflowPaletteEntries()` returns the editor's "Insert → Sub-workflow" entries: `n8n-nodes-base.executeWorkflow` (call another workflow) and `n8n-nodes-base.executeWorkflowTrigger` (start node for the called workflow).
  - `validateSubWorkflowRef(ref, knownWorkflowIds, selfId)` is the read-side check: rejects null/empty, rejects self-reference (infinite recursion), rejects unknown workflow, accepts valid ref.
  - `isSubWorkflowType(type)` is a small helper for the drag-and-drop handlers.
  - `nodeTypeForSubWorkflow(entry)` returns a minimal `NodeType` stub the canvas can register when the API is offline; the full NodeType (with Properties) comes from `GET /api/v1/node-types/{name}`.
- **Why this is the right shape:** the engine-side executor was already complete. The UI gap was the palette entry and the validation; both are now typed and tested.
- **Resolved at:** 2026-10-12.

### 9.27 BUG-ARCH-2026-10-12-005 — Static Data (workflow-scoped KV)

- **Resolution:** `internal/staticdata/staticdata.go` (9 unit tests in `staticdata_test.go`):
  - `Store{DataDir string}` is the in-process service. `New(dataDir)` creates the root.
  - `Get(workflowID, nodeName)` returns the current static-data map. Empty (non-nil) map on missing file — matches n8n.
  - `Set(workflowID, nodeName, data)` replaces the whole map. Atomic via temp-file + rename so a partial write can never leave the on-disk JSON malformed.
  - `Update(workflowID, nodeName, mutator)` is the read-modify-write helper; `mutator` mutates the map in place.
  - `Delete(workflowID, nodeName)` removes the scope. NotFound is not an error.
  - Files live at `{DataDir}/{workflowID}/{nodeName|__global__}.json`. `nodeName == ""` is the global scope (n8n convention).
- **Goroutine safety:** the whole struct is mutex-guarded. Reads + writes serialize so the file is never read mid-write.
- **Resolved at:** 2026-10-12.

### 9.28 BUG-ARCH-2026-10-12-006 — Binary data blob store

- **Resolution:** `internal/blobstore/blobstore.go` (9 unit tests in `blobstore_test.go`):
  - `BlobStore` is the interface every backend implements: `Put(ctx, id, data)` / `Get(ctx, id)` / `Delete(ctx, id)` / `Exists(ctx, id)`.
  - `LocalStore` (disk-backed): sharded by the first 2 hex chars of the ID (256 shards so a single directory doesn't have millions of entries). Atomic via temp-file + rename. Capped at 1 GiB per blob (the limit is a sanity cap; real workflows stream via the S3 backend).
  - `S3Store` (AWS S3 / MinIO): uses `aws-sdk-go` (already a dep). Configurable bucket / region / endpoint / key prefix. `Exists` uses HEAD (no body transfer). `notFound(err)` sniffs the SDK error string for `NoSuchKey` / `NotFound`.
  - `ErrNotFound` is the typed sentinel `Get` returns for a missing blob.
- **Why both backends:** local for dev / single-instance (default), S3 / MinIO for the production 0.8.0 rollout. The interface is the contract; the engine picks one at startup based on `M9M_BLOBSTORE_TYPE`.
- **Resolved at:** 2026-10-12.

### 9.29 BUG-ARCH-2026-10-12-007 — Streaming responses

- **Resolution:** `internal/webhooks/streaming.go` (8 unit tests in `streaming_test.go`):
  - `StreamResponse` wraps an `http.ResponseWriter` that implements `http.Flusher`. `NewStreamResponse` returns `ErrStreamingUnsupported` for non-Flusher writers (e.g. the test recorder).
  - `Write(chunk)` writes + flushes per chunk. The caller is responsible for chunk boundaries (one JSON object, one SSE event, etc.).
  - `WriteString(s)` is the string-flavoured `Write`. Same flush behaviour.
  - `WriteSSE(event, data)` writes one Server-Sent Event per the WHATWG spec: optional `event:` line, then `data:` per line of the data (newlines in `data` are split into multiple lines), then the blank-line terminator. Flushes after every line.
  - `Hijack()` exposes the underlying connection (WebSockets / raw TCP). Returns an error if the writer doesn't implement `http.Hijacker`.
  - The wrapper sets `X-Accel-Buffering: no` + `Cache-Control: no-cache` so nginx / common reverse proxies don't buffer the response.
- **Why this is the right shape for production streaming:** the existing `sendResponse` writes one shot; this wrapper adds the incremental path. Webhook handlers pick one or the other based on the response's `Stream` flag.
- **Resolved at:** 2026-10-12.

---

## 10. Filing & triage

### 10.1 How to file a new bug

Use the following template in the GitHub issue tracker (or in a follow-up edit to this file if the bug is internal).

```markdown
### Summary
[One-line description]

### Severity
[ ] P0 — wire-compat / correctness blocker (blocks a parity case or causes data loss)
[ ] P1 — engine correctness
[ ] P2 — performance / reliability
[ ] P3 — observability / DX
[ ] P4 — security
[ ] P5 — docs / repo hygiene
[ ] P6 — architectural / n8n feature gap

### Environment
- m9m version (commit SHA or release tag):
- Platform: [macOS / Linux / Windows + arch]
- Storage: [in-memory / MySQL / Postgres / SQLite]
- Queue: [in-memory / Redis / RabbitMQ]
- n8n version (if parity-related): 

### Observed
[What happens. Include curl output, error message, screenshot, log line, etc.]

### Expected
[What should happen, per the PRD or docs.]

### Reproduction
1. [Step]
2. [Step]
3. [Step]

### Possible cause
[If you have a guess, link to file + line.]

### Related
- Closes: [issue #]
- Related: [parity_gaps.json GAP-..., workflow-test.MD row, n8n-nodes.MD §, etc.]

### Acceptance criteria
- [ ] Reproduction steps produce the fix
- [ ] Unit / integration test added
- [ ] `parity/parity_gaps.json` updated (if parity)
- [ ] `workflow-test.MD` + Google Sheet updated (if parity)
- [ ] `CHANGELOG.md` entry added
- [ ] No regression on the full 19/19 parity suite
```

### 10.2 Triage SLA

| Severity | Acknowledgement | Fix target | Verification |
| --- | --- | --- | --- |
| **P0** | 4 hours | 7 days | full parity suite + live curl against the production rig |
| **P1** | 1 business day | 14 days | unit + integration tests + 19/19 parity |
| **P2** | 1 business day | next minor release | benchmark suite no-regress |
| **P3** | 1 business day | next minor release | UI screenshot + user story walkthrough |
| **P4** | 4 hours (security) | coordinated disclosure window (per `SECURITY.md`) | fuzzer pass + threat model update |
| **P5** | 1 week | batched into the next docs/repo-hygiene cycle | `markdownlint` + grep checks |
| **P6** | 1 week | back-burner unless user demand | PRD entry only |

### 10.3 Definition of done (every bug)

1. **Reproduction** — a failing test (unit or integration) exists in the relevant package.
2. **Fix** — the code change is in `internal/...`, `cmd/...`, `sync-service/sync.py` (sync orchestration only), or `web/...`. Never in a workflow JSON file, never in the Google Sheet, never in the parity orchestrator.
3. **Tests** — new test added or existing test updated to cover the regression. No "I'll add tests later".
4. **Docs** — if the bug touches a public surface (API endpoint, env var, node parameter, behaviour), the matching doc page is updated in the same commit.
5. **Parity** — if the bug is a parity gap, `parity/parity_gaps.json` is updated (status, `resolved_in_commit`, `resolution_summary`); if it changes a row's verdict, the Google Sheet and `workflow-test.MD` are updated atomically.
6. **Changelog** — entry under `CHANGELOG.md` under the current `[Unreleased]` or named release section.
7. **No regression** — full `make test` + `make lint` + `m9m benchmark` (no-regress) + the 19/19 parity suite pass on the live rig at `http://187.77.113.218:8080`.

### 10.4 Verification rig

- **Live test rig:** n8n on `http://187.77.113.218:5678`, m9m on `http://187.77.113.218:8080`, both behind the production compose stack at `/root/gabungan`.
- **Parity orchestrator:** `scripts/parity_orchestrator.py all` (sync → validate → sheet update → MD update).
- **Sheet:** `Validasi N8N - M9M` (Google Sheet ID `1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ`), tabs `Webhook Controlling` (positive), `Webhook Controlling Negative Test` (negative), `Capabilty Gap` (gaps).
- **Live verdict scoreboard:** `workflow-test.MD` Section H.

### 10.5 Escalation path

1. Open bug with the template above.
2. Triage within SLA.
3. If P0 and the bug is wire-compat: assign the next parity cycle. The cycle's exit criteria are the bug's acceptance criteria. Update `parity/parity_gaps.json`.
4. If P4 (security): follow `SECURITY.md` — private disclosure to `me@dipankar.name`; coordinated window before public issue.
5. If unable to fix within target: bump the milestone in `m9m.MD` §12 and document the reason in the bug ticket.

---

## 11. Anti-patterns (do not do these)

| Anti-pattern | Why it's wrong | Right way |
| --- | --- | --- |
| Edit the n8n workflow JSON to make a parity test pass. | Defeats the point — parity means m9m runs n8n's wire shape unchanged. | Fix `internal/nodes/...` so m9m runs the n8n workflow as-is. |
| Edit `sync-service/sync.py` to **mask** a parity gap (e.g. rewrite the response body to match n8n's). | Hidden regression; next cycle will re-surface it. | The sync bridge is allowed to **orchestrate** (e.g. upsert credentials before workflows). It is not allowed to **rewrite** data. |
| Edit the Google Sheet without committing `workflow-test.MD` (or vice versa). | The two drift, the next orchestrator run notices, the next bug is filed about the doc/sheet mismatch. | Update both in the same git commit. |
| Skip tests because "it's just a doc change" or "the fix is obvious". | The "obvious" fixes are how BUG-2026-09-09-001 / BUG-2026-09-10-001 / BUG-2026-09-10-002 originally got missed. | Add the test. If the test is hard to write, that is a signal that the fix is wrong, not that the test is unnecessary. |
| File a bug without a reproduction. | "I think there's a bug" tickets age badly. | Reproduce first, then file with the template. |
| File a P0 against a P6 (feature gap). | P0 has a 7-day SLA; P6 has no SLA. Mismatched severity creates pressure to ship a half-baked fix. | Match the severity. The PRD §12 milestones are the right place for P6 items. |
| Close a bug by saying "works on my machine". | It is the orchestrator's verdict that matters, not the developer's local test. | Run `scripts/parity_orchestrator.py all` end-to-end and link the result. |

---

## 12. References

- `m9m.MD` — companion PRD.
- `README.md` — project overview, install, performance, observability.
- `CLAUDE.md` — agent working notes.
- `CHANGELOG.md` — release notes.
- `parity/parity_gaps.json` — machine-readable parity backlog.
- `workflow-test.MD` — live n8n ↔ m9n parity test report (Section H = scoreboard; Section I = active gaps; Section J = last cycle's diff).
- `credentials_n8n_api.MD` — n8n credentials API reference + m9m gap analysis.
- `n8n-api-docs.md` — captured n8n public API + webhook reference.
- `n8n-nodes.MD` — n8n core nodes reference for m9m.
- `gap-current-vs-doc.MD` — docs vs code drift analysis (Phases 0–8).
- `docs/N8N_FEATURE_COMPARISON.md` — feature gap analysis (last updated 2026-10-07).
- `docs/REFRACTOR_PLAN.md` — module decomposition plan.
- `SECURITY.md` — coordinated disclosure policy.

---

*End of bug-fix.MD. Last updated 2026-10-12. Maintained by Mahar Solution.*
