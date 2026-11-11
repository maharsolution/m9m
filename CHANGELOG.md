# Changelog

All notable changes to m9m will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-11-11

Cumulative changes since `0.1.0-beta.1`. The 2026-09-10 webhook-parity cycle is documented under `[0.2.0-parity] - 2026-09-10` further down. Items below are platform work that landed after the parity cycle.

### 2026-11-11 cycle — credentials hardening + workflow parity fixes

33 commits landed on `main` between 2026-11-10 and 2026-11-11: credential verification against live services, credential-editor UX fixes, n8n workflow-parity fixes in the engine, the n8n workflow import/export feature, and the switch of the deploy pipeline to the Harbor image + the `mhr` SSH account. The `sync-service/` hardening documented at the end of this block is still uncommitted.

#### Credentials — live connection verification
- **Real connection test replaces static field checks** — `internal/api/credential_conn.go` (new) adds `testCredentialConnection(credType, data)`, dispatching per credential type under a `credentialConnectTimeout = 10 * time.Second` bound; the second return value reports whether a live connection was attempted, so types that cannot be verified over the wire keep static validation only. `validateCredentialData` in `internal/api/credentials.go` calls it after the required-field and format checks. (b55318e, 04aeb5e)
- **PostgreSQL / MySQL ping** — `internal/api/credential_conn.go` uses `database/sql` with `lib/pq` (`sql.Open("postgres", url)` + `PingContext`, `sslmode` honoured) and `go-sql-driver/mysql` (`sql.Open("mysql", dsn)` + `PingContext`). (b55318e)
- **MongoDB ping** — `internal/api/credential_conn.go` uses the official `mongo-driver` (`mongo.Connect` + `client.Ping`), accepting a `mongodb://` or `mongodb+srv://` URI, or a host/port/user/password form. (b55318e)
- **Redis RESP auth** — `internal/api/credential_conn.go` speaks RESP over raw TCP, sending `AUTH [user] password` and then `PING`. (b55318e)
- **Kafka SASL handshake** — `internal/api/credential_conn.go` uses `segmentio/kafka-go` with TLS + SASL, trying `plain.Mechanism`, then `scram.SHA256`, then `scram.SHA512`; success requires a real `Metadata` request answered by the broker (`kafkaMetadata`, `kafkaBrokerList`, `kafkaTLSConfig`). (4465c67)
- **RabbitMQ AMQP handshake** — `internal/api/credential_conn.go` dials with `amqp.DialConfig` using `amqp.PlainAuth` and checks the vhost (`testRabbitMQConnection`, `isAMQPAuthError`); `go.mod` and `go.sum` gain `github.com/rabbitmq/amqp091-go v1.16.0`. (4465c67)
- **OpenAI / Anthropic / Slack probes** — `internal/api/credential_conn.go` performs an authenticated HTTP probe (`/v1/models` for OpenAI and Anthropic, `auth.test` for Slack) and reports the status code with a truncated body. (b55318e)
- **Secrets never reach the browser** — `scrubSecret(msg, secret)` in `internal/api/credential_conn.go` replaces the password with `***` in every returned error; `truncate` shortens API error bodies before they are shown. (b55318e, 4465c67)
- **Flattened PEM repair** — `normalizePEM(raw)` in `internal/api/credential_conn.go` rebuilds a PEM block from n8n-synced certificates that were flattened onto one line or contain literal `\n` sequences, re-wrapping the base64 body at 64 columns so `x509.CertPool.AppendCertsFromPEM` can parse it (fixes "Kafka CA certificate could not be parsed"). (e8e3fd2)
- **Aliased credential fields** — `credentialFieldAliases` in `internal/api/credential_conn.go` maps `host` ← `hostname`/`server`/`serverName`, `user` ← `username`/`login`/`loginName`, `database` ← `db`/`dbName`/`databaseName`, `password` ← `pass`/`pwd`; `credentialFieldValue`, `dataString` and `dataInt` read through the aliases, so synced n8n credentials validate and connect. (5794ae9)
- **New credential schemas** — `internal/api/credentials.go` adds the `mySql`, `openAiApi`, `rabbitmq` and `redis` schemas (`mySql` / `openAiApi` are aliases of `mysql` / `openai`), and `web/src/views/Credentials.vue` renders a generic field list for types that have no schema. (13307ae)
- **Credential API routes** — `internal/api/routes.go` registers `POST /api/v1/credentials/test` (unsaved payload), `POST /api/v1/credentials/{id}/test`, `GET /api/v1/credentials/schema/{type}` and `GET /api/v1/credentials/types`; the static paths are registered before `/credentials/{id}` because gorilla/mux matches in registration order. (04aeb5e, 13307ae)
- **Single-credential GET returns the payload** — `internal/api/credentials.go` adds `toCredentialResponseWithData`, so `GET /api/v1/credentials/{id}` returns the stored `data` while the list endpoint still strips it. (04aeb5e)
- **PATCH merges instead of replacing** — `internal/api/credentials.go` `mergeCredentialData` overlays only the fields present in the request body, so a partial update no longer wipes untouched secrets. (04aeb5e)
- **Kafka credential: one username, one password** — `credentialSchemas["kafka"]` in `internal/api/credentials.go` now declares `username` / `password` (n8n parity — `n8n/packages/nodes-base/credentials/Kafka.credentials.ts` stores the Kafka login as `username` / `password`) and no longer declares the second `saslUsername` / `saslPassword` pair; `saslMechanism`, `authentication` and `apiKey` stay as they are.
- **Kafka connection test uses the canonical login** — `internal/api/credential_conn.go` adds `kafkaCredentialUserPass`, `kafkaSASLMechanism`, `kafkaSASLMechanisms`, `kafkaRESTAuthMode`, `testKafkaRESTConnection`, `kafkaExactString` and `scrubKafkaSecrets`; the legacy `saslUsername` / `saslPassword` spellings stay valid as aliases, the canonical key always wins over a stale alias, the SASL mechanism selected by the operator is used exclusively (PLAIN → SCRAM-SHA-256 → SCRAM-SHA-512 is only the fallback for an empty mechanism), a missing username is reported explicitly instead of falling back to an unauthenticated metadata request, and the REST-proxy mode is probed with the same single username / password pair.
- **Wrong Kafka username is now an authentication failure** — the tester read the login from `dataString(data, "user")`, which never saw the Kafka credential's own `saslUsername` / `saslPassword` keys and could be shadowed by a stale alias, so a wrong username silently produced an unauthenticated metadata probe and reported `OK` while a wrong password correctly failed; `internal/api/credentials_kafka_test.go` (new) locks the behaviour in with a stubbed broker probe.

#### Credentials — UI
- **Edit modal prefills the stored values** — `web/src/views/Credentials.vue` loads the credential through `fetchCredential` (`web/src/stores/credentials.ts`, `web/src/api/credentials.ts`) and fills every schema field with the stored value. (04aeb5e)
- **Test button honours unsaved edits** — the UI posts `{ type, data }` to `POST /api/v1/credentials/test` via `testCredentialPayload` / `testPayload`, so the result reflects the current form and not the stored row. (04aeb5e, b55318e)
- **Credential type chooser is backend-driven** — `GET /api/v1/credentials/types` feeds `supportedCredentialTypes` in `web/src/stores/credentials.ts`, so the "Add Credential" list can no longer drift from the backend schema registry. (04aeb5e, 13307ae)
- **Password / secret reveal toggle** — `isSecretField`, `revealedFields` and `toggleReveal` in `web/src/views/Credentials.vue` mask and reveal secret-looking fields in both the schema-driven form and the fallback field list. (b55318e)
- **Aliased stored keys prefill the schema fields** — `applyFieldAliases`, `fieldAliases` and `typeFieldAliases` in `web/src/views/Credentials.vue` copy synced n8n keys (`hostname` → `host`, `username` → `saslUsername` for Kafka) into the schema fields; the original keys are kept so nothing is lost on save. (aee1097, e8cbb2e)
- **Select fields keep their raw value** — `optionValues` in `web/src/views/Credentials.vue` adds the stored value as an extra option when it is not part of the schema options, so a select never renders blank. (e8cbb2e)
- **Extra stored fields stay editable** — `extraDataFields` in `web/src/views/Credentials.vue` lists stored keys the schema does not describe (e.g. `hostname` / `username`). (b55318e)
- **Draft persistence across reloads** — the edit form is persisted in `sessionStorage` under `m9m:credential-edit-draft` (`writeDraft`, `readDraft`, `clearDraft`), so a discarded or reloaded browser tab does not blank the form. (e9547d1)
- **Scroll position restored after re-render** — the modal panel uses `overscroll-contain` and restores `scrollTop` (`restorePanelScroll`, `panelScrollTop`) after the schema or test banners re-render. (e9547d1)
- **Kafka form shows a single username / password** — `typeFieldAliases` in `web/src/views/Credentials.vue` folds the legacy `saslUsername` / `saslPassword` keys into the canonical `username` / `password` fields and then drops them, the generic `user` ← `username` alias is skipped for Kafka (`typeAliasBlocklist`), and `extraDataFields` no longer lists an alias key — the Kafka credential used to render the username three times (`username`, `saslUsername`, `user`).
- **Kafka save keeps the login in sync** — `normalizeCredentialData` in `web/src/views/Credentials.vue` writes the canonical `username` / `password` into the legacy `saslUsername` / `saslPassword` keys on save and on test, so the node's credential bridge (`ApplyKafkaCredentialAuth`) and the connection test always use the login the operator sees.

#### Engine and execution
- **Per-execution `runExecutionData` via a ctx frame** — `internal/engine/engine.go` keeps the run state in an `execFrame` attached to the context (`withExecFrame`, `execFrameFrom`, `activeRunData`), so concurrent webhook and Kafka runs on one engine cannot clobber each other's node data. (1132766)
- **IF/retry loop driven from the back-edge target** — `internal/engine/engine.go` starts the If-retry poll from the body entry when that entry already holds items (`BackEdgeTarget`), seeds the back-edge target from the loop input, and keeps polling when the body returns no items. (8f6012d)
- **BFS body ordering** — `internal/connections/if_retry_loop.go` walks forward from the back-edge target in BFS order, so the retry body runs `Find Respond → Value → Check Key Exists1` instead of canvas order. (e2c1f9e)
- **Find seed preserved across empty polls** — `internal/engine/engine.go` keeps the previous seed when the body entry misses, and `internal/nodes/core/execute_workflow.go` `ensureParentFields` copies parent fields (e.g. `key`) into mapped sub-workflow inputs. (04bae90, 3342dca)
- **Run-aware IF guard** — `internal/nodes/transform/if_node.go` adds `ExecuteWithRun` and `internal/nodes/transform/conditions.go` adds `EvaluateConditionsWithRun`, so `$('Set Counter').item.json.attempt` resolves against the live run; `condToFloat64` additionally accepts `int64`, `uint`, `uint32`, `uint64`, `float32` and `json.Number`. (04bae90)
- **Merge `chooseBranch` emits only the selected input** — `internal/engine/engine.go` buckets routed data per target input index (`incomingInputIndex`, `chooseBranchInput`) and replaces the flattened slice with the selected branch; `internal/nodes/transform/merge.go` accepts `chooseBranch` without `branchCondition`. (258c962, f28fcc7)
- **n8n short operators in conditions** — `internal/nodes/transform/conditions.go` normalises `lt`, `lte`, `lteq`, `gt`, `gte`, `gteq`, `eq`, `neq` and `ne` to their long operator names, so validation and evaluation share one vocabulary. (7d598f8)
- **Respond to Webhook honours `respondWith=text`** — `internal/nodes/trigger/respond_to_webhook.go` returns the raw string under `data` for `respondWith: "text"`; `internal/engine/engine.go` keeps the IF routing tags on the terminal body output so the guard can still distinguish exit from retry. (509d42a)
- **Webhook mode seeds only webhook nodes** — `internal/engine/engine.go` `findWebhookStarts` restricts a `webhook`-mode run to webhook trigger nodes, so sibling Kafka / Execute-Workflow-Trigger nodes are not seeded with the HTTP payload. (8f6012d)
- **Sub-workflows start at `executeWorkflowTrigger`** — `internal/engine/engine.go` and `internal/engine/tracing_ctx.go` add `WithSubWorkflowStart`, `IsSubWorkflowStart` and `SubWorkflowDepth`; a child run seeds only `n8n-nodes-base.executeWorkflowTrigger` nodes. (c5e0a78)
- **Sub-workflow ctx forwarding** — `internal/nodes/core/execute_workflow.go` adds `EngineAdapter.ExecuteCtxFn`, and `cmd/m9m/commands/exec.go` switches to `EngineAdapterFromEngine`, so the sub-workflow start flag and the recursion depth survive into the child run. (dfa0391)
- **HTTP request body honours `jsonBody` / `specifyBody`** — `internal/nodes/http/request.go` `resolveRequestBody` posts `jsonBody` when `specifyBody` is `json` (or when `jsonBody` is set and `body` is empty), falls back to the `body` string/map, and honours `rawContentType` / `contentType`. (b220301)
- **HTTP response flattened into `$json`** — `internal/nodes/http/request.go` `shapeHTTPResponse` makes the parsed JSON body the top-level `$json` keys; `headers` and `statusCode` are only added when `response.includeResponseHeadersAndStatus` (or `options.response.fullResponse`) is set. (ac62c66)
- **`$input.item` exposed as a property** — `internal/expressions/data_proxy.go` binds `$input.item` to the n8n item wrapper `{json, binary, pairedItem}` as a property instead of a callable, so snippets like `$input.item.json.attempt` resolve. (4028e41)
- **MongoDB `findOneAndReplace`** — `internal/nodes/database/mongodb.go` implements the `findOneAndReplace` operation (`updateKey`, `fields`, `upsert`) and stops storing the run context on the node singleton, so concurrent Kafka and webhook pollers cannot clobber each other's `$('…')` lookups. (c0510f7, a97567a)
- **MongoDB query/filter evaluated with the run context** — `internal/nodes/database/mongodb.go` `resolveQueryExpressions` evaluates `query` / `filter` expressions against the live run, accepts the n8n `query` alias for `filter`, and reads the credential-injected `mongoDb_*` keys (`resolveConnParams`). (6446115, a932c01)
- **MongoDB match-all rejection and key recovery** — `internal/nodes/database/mongodb.go` rejects an empty filter when a query/filter was configured (`hasNonEmptyQueryParam`) and recovers a `"key": "undefined"` template from the live run (`lookupRunDataKey`). (c545189, 3342dca)
- **MongoDB miss logging** — `internal/nodes/database/mongodb.go` logs the resolved filter and collection to stderr on an empty Find, so a miss is diagnosable without a debugger. (fc29436, 1ab92b6)
- **Set node uses the live `$execution.id`** — `internal/nodes/transform/set.go` adds `ExecuteWithRun` and resolves `={{ $execution.id }}` from `RunExecutionData.ExecutionID` instead of the hardcoded `set-processing` stub; `internal/expressions/data_proxy.go` exposes it on the `$execution` proxy. (9494eae)
- **Respond-to-Webhook header expressions** — `internal/nodes/trigger/respond_to_webhook.go` evaluates `options.responseHeaders` against the input item and stamps the result under `ResponseHeadersMetaKey`; `internal/webhooks/manager.go` reads and strips that key before writing the response body. (a97567a)

#### Deployment
- **Harbor image pull replaces the remote source build** — `docker-compose.yml` resolves the backend image through `REGISTRY_IMAGE_URL` (documented default `https://harbor.solutiontech.id/m9m/m9m-core:latest`) and `Dockerfile` records that the `COMMIT` / `BUILD_DATE` build args come from `/home/mhr/bin/build` or the `harbor-image.yml` workflow. (4b32572)
- **`mhr` SSH account on port 2212** — `scripts/deploy_m9m.ps1` now defaults to `ServerUser = "mhr"` and `ComposeDir = "/home/mhr/gabungan"`, and passes `-i "$HOME\.ssh\id_rsa"` with `BatchMode=yes` so a wrong key fails fast instead of hanging. (4b32572)
- **Parity orchestrator deploy path** — `scripts/parity_orchestrator.py` deploys via `mhr@187.77.113.218` with `-o BatchMode=yes` and `-i ~/.ssh/id_rsa`, running `docker compose pull m9m-backend && docker compose up -d m9m-backend` in `/home/mhr/gabungan`. (4b32572)
- **Deployment docs realigned** — `documentation/docs/deployment/docker.md` replaces every `/root/bin/*` reference with `/home/mhr/bin/*` and the `root@` SSH user with `mhr@`. (4b32572)

#### Workflow import/export (n8n JSON)
- **Export endpoints** — `internal/api/workflow_import_export.go` (new) adds `GET /api/v1/workflows/{id}/export`, which answers with **one** n8n workflow object (n8n editor "Download" parity), and `GET|POST /api/v1/workflows/export`, which always answers with an **array** of workflow objects, also for a single id (n8n CLI `export:workflow` parity); both send `Content-Type: application/json` and a `Content-Disposition: attachment` file name (`<workflow-slug>.json`, `workflows-export.json` for a bulk export). (849d59b)
- **Import endpoint** — `POST /api/v1/workflows/import` accepts a multipart upload (a `.json` file in the `file` field), a single JSON workflow object, a JSON array of workflow objects, or the envelope `{"workflows": [...], "activeState": "fromJson"}`; the payload is capped at 32 MiB (`maxImportPayloadBytes`). (849d59b)
- **Static routes registered first** — `internal/api/routes.go` registers `/workflows/export`, `/workflows/import` and `/workflows/{id}/export` before `/workflows/{id}`, because gorilla/mux matches in registration order and the static paths would otherwise be swallowed by the `{id}` route. (849d59b)
- **Per-item import verdicts** — the import response is a `WorkflowImportSummary` (`imported`, `failed`, `activeState`, `workflows[]`, `errors[]`); a rejected entry never aborts the batch, and a run in which every entry fails answers `400` with the same summary under a `summary` key. (849d59b)
- **Structure validation** — `ValidateN8nWorkflow` in `internal/compatibility/workflow_importer.go` requires every entry to carry **both** `nodes` and `connections` and rejects it with n8n's wording (`File does not seem to contain valid workflows: missing "nodes" property`). (849d59b)
- **Identity rules** — the source workflow id is kept as a **string** (a missing one is generated, an id that already exists in m9m is replaced by a fresh one so an unrelated workflow is never overwritten), every import receives a fresh `versionId`, and a node without an id gets one. (849d59b)
- **Credential shape normalisation** — `convertCredentials` accepts the legacy n8n map (`{"httpBasicAuth": "My Cred"}`) and the modern one (`{"httpBasicAuth": {"id": "…", "name": "…"}}`) and normalises both to `{id, name, type}`; an id that cannot be resolved against m9m's credential store is cleared, the name is kept, and the item is reported with a warning instead of failing. (849d59b)
- **`active` handling** — imported workflows are stored **inactive**; the JSON `active` flag is honoured only when the request asks for it (`?active=fromJson`, `?activeState=fromJson`, or the envelope's `activeState`) — n8n's `activeState: fromJson` semantics. (849d59b)
- **Tags** — the importer accepts both `{id, name}` objects and plain strings and stores the tag **names** (m9m's `model.Workflow.Tags` is `[]string`); a tag id cannot be preserved and is reported as a warning, while the export writes `{id, name}` objects and resolves the id through `resolveTagID`. (849d59b)
- **Node and connection sanitizing** — nodes without a `type` are dropped, nodes without a `name` get a unique name derived from the node type display name, a malformed `position` falls back to `[0, 0]`, the connection map is sanitised against the real node names, and only `main` connections are stored (every other connection type is reported as a warning). (849d59b)
- **Webhook id regeneration** — the importer regenerates the `webhookId` of webhook node types and rewrites `parameters.path` / `parameters.options.path` when they referenced the old id. (849d59b)
- **Node fields kept** — `internal/model/workflow.go` adds `color`, `onError`, `retryOnFail`, `maxTries`, `waitBetweenTries`, `continueOnFail`, `alwaysOutputData`, `executeOnce` and `root` to `model.Node`, so an n8n workflow survives an import → export round-trip without losing settings. (849d59b)
- **Workflow list UI** — `web/src/views/WorkflowList.vue` adds a selection checkbox per card, an `Export selected (n)` button, an `Import` button that opens a hidden `.json` file input, a per-row `Export` entry in the `⋮` menu, and a success/error banner that lists the per-item warnings and failures; the list refreshes after an import. (849d59b)
- **Workflow editor UI** — `web/src/components/workflow/WorkflowEditorToolbar.vue` adds an `Export` action (hidden for an unsaved draft, which has no id yet) and `web/src/views/WorkflowEditor.vue` downloads the open workflow and reports the result in a banner. (849d59b)
- **UI client and types** — `web/src/api/workflows.ts` adds `exportWorkflow`, `exportWorkflows`, `importWorkflowFile` and `importWorkflowDocument` with a `WorkflowImportActiveState` of `false` / `fromJson`; `web/src/types/api.ts` adds `WorkflowImportItem`, `WorkflowImportError`, `WorkflowImportSummary` and `WorkflowExportFile`, and `web/src/types/workflow.ts` adds the n8n wire types (`N8nWorkflowDocument`, `N8nWorkflowNode`, `N8nWorkflowTag`, `N8nWorkflowCredential`). (849d59b)
- **API tests** — `internal/api/workflows_import_export_test.go` (new) covers the single and bulk export, the multipart / object / array import, the `nodes` / `connections` rejection, the batch continuation, the credential normalisation, the `active` opt-in, the id collision, the round-trip and the editor-side rules (23 tests). (849d59b)
- **Feature documentation** — `docs/WORKFLOW_IMPORT_EXPORT.md` (new) documents the endpoints, the accepted payloads, the import rules and the n8n parity table for the feature. (849d59b)
- **Parity reference** — `docs/N8N_WORKFLOW_PARITY_REFERENCE.md` (new) distils the rules from the `n8n/` checkout (upstream commit `04fb7076`) and `n8n/graphify-out/graph.json` into export semantics, import semantics, the editor-side import behaviour, the m9m mapping table, the deliberate divergences and a parity checklist; it is the canonical source for both the server importer and the `sync-service/` bridge. (docs/N8N_WORKFLOW_PARITY_REFERENCE.md, uncommitted)

#### n8n sync-service hardening
- **Import normaliser** — `normalize_n8n_workflow` in `sync-service/sync.py` returns an n8n-shaped, import-ready copy of a workflow: it requires **both** `nodes` and `connections` (otherwise `InvalidWorkflowError` is raised), keeps the workflow `id` a string (generated when missing), assigns a fresh `versionId`, normalises `tags` to `{id, name}` objects, coerces `typeVersion` to an int (float `2.1` → `2`, a non-numeric value defaults to `1` with a warning) and forces `active` to `false` unless `active_state` asks for `from-json`. (sync-service, uncommitted)
- **Editor-side import rules** — the same normaliser drops nodes without a `type`, normalises a missing or malformed `position` to `[0, 0]`, gives unnamed nodes a unique name and drops connections that reference unknown node names; every one of these is a warning, never a hard failure. (sync-service, uncommitted)
- **Credential conversion** — `normalize_credentials` rewrites the OLD n8n shape (`{"httpBasicAuth": "My Cred"}`) into the NEW one (`{"httpBasicAuth": {"id": null, "name": "My Cred"}}`), keeps a null id for an unresolved credential, skips the credentials of a disabled node and drops `__aiGatewayManaged` entries. (sync-service, uncommitted)
- **Validation verdicts** — `validate_n8n_workflow` returns `valid`, `errors`, `warnings` and `unsupportedTypes`: structural problems (a missing name, an empty `nodes` list, a node without a type) are **errors**, while a node type m9m does not implement is a **warning** — the workflow is still imported and the gap is surfaced instead of dropping the file. (sync-service, uncommitted)
- **m9m wire adaptation** — `to_m9m_payload` converts n8n tag objects into m9m's `[]string` tag names and reshapes node credentials into `{id, name, type}`, dropping `__aiGatewayManaged` entries. (sync-service, uncommitted)
- **Latent poll-path 400 fixed** — the live poll path (`run_cycle`) now runs the same normaliser and pushes the adapted payload; before, the raw n8n document went straight to m9m, so n8n's tag objects hit m9m's `Tags []string` model and the push was rejected with HTTP 400. The poller normalises with `active_state=from-json` and carries n8n's `versionId` over, so m9m keeps tracking the n8n version and the webhooks of active workflows stay registered. (sync-service, uncommitted)
- **Import CLI** — `python sync.py --import FILE [--dry-run] [--active-state=from-json]` imports a local n8n export (a single object or an array); `import_workflows_from_file` returns one verdict per entry (`ok` / `skipped` / `failed` with a reason and the collected warnings), a bad entry never kills the batch, and `--dry-run` makes **zero** HTTP calls (the m9m node-type probe is skipped) while printing the normalised JSON. (sync-service, uncommitted)
- **Service mode unchanged** — without `--import` the module still boots the FastAPI / uvicorn service with the background poller; `sanitize_type_versions` is kept as part of the module's public surface but is superseded by the normaliser. (sync-service, uncommitted)
- **Test suite** — `sync-service/test_sync.py` grows from 4 to 51 tests (all passing) covering the credential-decrypt vectors, the credential shapes, the id and `versionId` generation, the tag normalisation, the `typeVersion` coercion, the `active` opt-in, the `nodes` / `connections` requirement, the typeless-node drop, the position normalisation, the connection sanitising, the unnamed-node naming, the validation verdicts and the zero-HTTP dry run. (sync-service, uncommitted)
- **Bridge documentation** — `sync-service/README.md` (new) documents the service mode, the import CLI, the normalisation rules, the validation verdicts, the m9m wire adaptation, the HTTP API and a parity table that lists the n8n rules with a match / divergence verdict plus the exact divergence points. (sync-service, uncommitted)

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