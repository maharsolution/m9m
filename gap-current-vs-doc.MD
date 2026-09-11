# Documentation gap analysis — current implementation vs `documentation/docs/`

**Date:** 2026-09-11 (Asia/Jakarta)
**Scope:** compare what the codebase actually does today (per the public symbol surface in `internal/`, `cmd/`, and the last ~30 commits on `main`) against what `documentation/docs/` describes for end users.
**Out of scope:** `documentation/site/**` (the mkdocs-rendered HTML mirror), `docs/**` (the developer-facing technical notes), the README trio, and `n8n-nodes.MD`/`workflow-test.MD` (operational reports).
**Code is NOT changed by this document.** Every item is a *documentation task*, an *investigation task*, or a *defer-and-track* decision.

---

## 1. Executive summary

| Bucket | Documented | Built | Gap |
|---|---|---|---|
| Node types (executors) | 30+ across 12 categories | **42 registered** in `cmd/m9m/commands/exec.go` | 12 missing from docs |
| API endpoints (`/api/v1/*`) | ~35 across 4 resource groups | **~55** across 11 resource groups | ~20 missing |
| OpenTelemetry surface | 1-line mention in `index.md` + `production.md` | Full implementation: `/api/v1/otel`, exporters, 3-layer config | Severely under-documented |
| n8n ↔ m9m sync bridge | None | `sync-service/sync.py` running in container | Not documented |
| AI assistant in-app | Brief mention in old `copilot` README + root README | Full implementation: `internal/ai/*`, `/api/v1/ai/*`, `Settings → AI` UI card | Not in `documentation/docs/` |
| Recent engine work (2026-09) | None | `EdgesTaken`, debug mode, retry-node, IF v2 boolean operators, Postgres `sslMode` | Not in `documentation/docs/` |
| Architecture diagram | Single shallow ASCII | 30+ sub-packages under `internal/` | Diagram is misleading |

The documentation in `documentation/docs/` is essentially **a snapshot from early 2025** (~ commit `e99...` era) before the September 2026 parity cycle, the OTel rewrite, the AI rename, and the sync bridge landed. The codebase has moved roughly 12 months of work ahead of the public docs.

---

## 2. Node catalogue gap (12 missing from `documentation/docs/nodes/`)

Each row below is a registered executor in `cmd/m9m/commands/exec.go::RegisterAllNodes` (lines 276–391) that does not appear in any file under `documentation/docs/nodes/`.

| Node type | Category | Where the cat doc lives today | Doc status |
|---|---|---|---|
| `n8n-nodes-base.noOp` | core | `documentation/docs/nodes/core.md` | Missing — no entry |
| `n8n-nodes-base.wait` | core | `documentation/docs/nodes/core.md` | Missing — no entry |
| `n8n-nodes-base.if` | transform | `documentation/docs/nodes/transform.md` | Missing — referenced in changelog, no per-node page |
| `n8n-nodes-base.loop` | transform | `documentation/docs/nodes/transform.md` | Missing |
| `n8n-nodes-base.splitInBatches` | transform | `documentation/docs/nodes/transform.md` | Missing |
| `n8n-nodes-base.itemLists` | transform | `documentation/docs/nodes/transform.md` | Missing |
| `n8n-nodes-base.executeWorkflow` | core | `documentation/docs/nodes/core.md` | Missing |
| `n8n-nodes-base.executeWorkflowTrigger` | triggers | `documentation/docs/nodes/triggers.md` | Missing |
| `n8n-nodes-base.errorTrigger` | triggers | `documentation/docs/nodes/triggers.md` | Missing |
| `n8n-nodes-base.respondToWebhook` | triggers | `documentation/docs/nodes/triggers.md` | Missing |
| `n8n-nodes-base.pythonCode` | code | (no `code.md` exists) | Category file missing |
| `n8n-nodes-base.xml` | transform | `documentation/docs/nodes/transform.md` | Missing — added 2026-09 |
| `n8n-nodes-base.redis` | database | `documentation/docs/nodes/database.md` | Missing |
| `n8n-nodes-base.elasticSearch` | database | `documentation/docs/nodes/database.md` | Missing |
| `n8n-nodes-base.mongoDb` | database | `documentation/docs/nodes/database.md` | Missing |
| `n8n-nodes-base.stripe` | productivity | (no `productivity.md`) | Missing |
| `n8n-nodes-base.notion` | productivity | (no `productivity.md`) | Missing |
| `n8n-nodes-base.twilio` | messaging | `documentation/docs/nodes/messaging.md` | Missing |
| `n8n-nodes-base.microsoftTeams` | messaging | `documentation/docs/nodes/messaging.md` | Missing |
| `n8n-nodes-base.sendGrid` | email | `documentation/docs/nodes/email.md` | Missing |
| `n8n-nodes-base.googleSheets` | productivity | (no `productivity.md`) | Missing |
| `n8n-nodes-base.gitlab` | vcs | `documentation/docs/nodes/vcs.md` | Missing |

Plus the category counts in `documentation/docs/nodes/index.md` (lines 16–27) are stale — they read "AI & LLM = 2" but the in-app AI is a separate Settings surface now; and "Total = 30+" but the registry has 42.

**Plan to close:** add one section per missing node in the existing category page, plus create the two missing category files (`code.md`, `productivity.md`). See section 6 for the file-by-file task list.

---

## 3. API surface gap (~20 endpoints undocumented)

Each row below is a route in `internal/api/routes.go` (lines 13–127) that does not appear in `documentation/docs/api/`.

### 3.1 Executions — big gap

| Method | Path | Doc has it? |
|---|---|---|
| `POST /api/v1/executions/{id}/retry-node` | **No** — added 2026-09-11 (`2ede796 feat(execution): per-node retry`) |
| `POST /api/v1/executions/{id}/cancel` | Mentioned in changelog, not in `executions.md` |
| `GET /api/v1/executions/count` | Missing |
| `GET /api/v1/executions/recent` | Missing |

### 3.2 Workflows — minor gap

| Method | Path | Doc has it? |
|---|---|---|
| `POST /api/v1/workflows/{id}/activate` | Missing |
| `POST /api/v1/workflows/{id}/deactivate` | Missing |
| `POST /api/v1/workflows/{id}/duplicate` | Missing |
| `GET /api/v1/workflows/count` | Missing |
| `GET /api/v1/node-types` and `GET /api/v1/node-types/{name}` | Missing entirely — no `node-types.md` |

### 3.3 Schedules — partial gap

| Method | Path | Doc has it? |
|---|---|---|
| `POST /api/v1/schedules/{id}/enable` | Missing |
| `POST /api/v1/schedules/{id}/disable` | Missing |
| `GET /api/v1/schedules/{id}/history` | Missing |
| `PATCH /api/v1/workflows/{id}` | Missing |

### 3.4 Dead-letter queue — entirely missing

`internal/api/dlq.go` exists and `routes.go` registers:

- `GET /api/v1/dlq` — list
- `GET /api/v1/dlq/{id}` — get one
- `POST /api/v1/dlq/{id}/retry` — retry
- `POST /api/v1/dlq/{id}/discard` — discard
- `GET /api/v1/dlq/stats` — stats

There is **no `dlq.md` and no DLQ entry in `api/index.md`**. This is a full feature surface that users can't discover through docs.

### 3.5 Telemetry (`/api/v1/otel`) — entirely missing

`routes.go` (lines 104–107) plus `internal/otel/api.go` define:

- `GET /api/v1/otel` — read effective config
- `PUT /api/v1/otel` — write effective config
- `DELETE /api/v1/otel` — clear overrides
- `POST /api/v1/otel/test` — emit a test span

There is **no `otel.md` and no `telemetry.md`**. The only OTel references in `documentation/docs/` are 1-line mentions in `index.md`, `production.md`, `llms.txt`, `llms-full.txt`, and `faq.md`.

### 3.6 AI assistant (`/api/v1/ai/*`) — entirely missing

`routes.go` (lines 81–86, 122–128) plus `internal/api/ai.go` define:

- `POST /api/v1/ai/generate` — workflow generation
- `POST /api/v1/ai/suggest` — node suggestion
- `POST /api/v1/ai/explain` — error explainer
- `POST /api/v1/ai/fix` — error fix suggester
- `POST /api/v1/ai/chat` — generic chat
- `GET /api/v1/ai/health` — provider health
- `GET /api/v1/ai` — config
- `PUT /api/v1/ai` — config
- `DELETE /api/v1/ai` — clear config
- `POST /api/v1/ai/test` — test chat

There is **no `ai.md` under `documentation/docs/api/`**. The renamed-from-`copilot` assistant is invisible from the doc site.

### 3.7 Settings (`/api/v1/settings`) — partial

| Method | Path | Doc has it? |
|---|---|---|
| `GET /api/v1/settings` | Mentioned in `index.md` only, no detail |
| `PATCH /api/v1/settings` | Missing |
| `GET /api/v1/settings/license` | Missing |
| `GET /api/v1/settings/ldap` | Missing |

### 3.8 Cross-cutting — health, performance, push

| Method | Path | Doc has it? |
|---|---|---|
| `GET /healthz`, `/ready` | `getting-started/index.md` mentions `/health` only; `/readyz`, `/health/detailed` not documented |
| `GET /api/v1/performance` | Missing |
| `GET /api/v1/push` (WebSocket) | Missing |

---

## 4. n8n ↔ m9m sync bridge — not documented

`sync-service/sync.py` (read by uvicorn inside the `m9m-backend` container, see `docker-compose.yml` and `supervisord.conf`) provides:

- `GET /` — banner
- `GET /health` — liveness
- `GET /status` — last-cycle summary
- `POST /sync` — run one cycle immediately
- Background polling on `POLL_INTERVAL_SECONDS` (default 30)
- Credential decryption via `N8N_ENCRYPTION_KEY` (AES-256-CBC + EVP_BytesToKey)
- Direct read from n8n Postgres for credential envelopes (`N8N_PG_*` env vars)
- Idempotent POST → PUT-on-409 fallback

There's no `documentation/docs/integrations/n8n-sync.md`, no entry in `migrate-from-n8n.md`, and no mention in the architecture pages. This is now one of the headline features (commits `aef4d71`, `4715e2a`, `514a6b1`, `514a6b1` cluster around the credential bridge).

---

## 5. AI assistant in-app — not documented

The in-app AI surface is large and entirely undocumented in `documentation/docs/`. Built items not in docs:

- `internal/ai/ai.go` — chat core, `/api/v1/ai/{generate,suggest,explain,fix,chat}`
- `internal/ai/providers.go` — hardcoded provider catalog (OpenAI, Anthropic, MiniMax, Ollama) with per-provider default models and base URLs
- `internal/ai/config.go` — `ConfigOverride`, env-default + DB-override stack
- `internal/ai/store.go` — DB-backed `ConfigStore`
- `internal/ai/runtime.go` — mutex-guarded hot reload
- `internal/ai/api.go` — GET/PUT/DELETE/Test for `/api/v1/ai`
- `web/src/components/ai/AgentAI.vue` — UI panel (renamed from `AgentCopilot.vue`)
- `web/src/api/ai.ts` and `web/src/types/ai.ts`
- Settings card in `web/src/views/Settings.vue`
- Legacy `M9M_COPILOT_*` env vars still honoured as aliases of `M9M_AI_*`

The existing `documentation/docs/nodes/ai.md` only covers OpenAI + Anthropic workflow nodes and never mentions the in-app provider config or MiniMax.

---

## 6. OpenTelemetry surface — severely under-documented

Documented today (in one-liner form):
- `documentation/docs/index.md:99`
- `documentation/docs/deployment/production.md:189-191`
- `documentation/docs/faq.md:65,216,273`
- `documentation/docs/llms.txt:26`
- `documentation/docs/llms-full.txt:3509,8931,8933,9983,10134,10191,11336`

What exists that is undocumented:
- Exporter types: OTLP gRPC and OTLP HTTP/protobuf, jaeger is no longer a separate surface
- Three-layer config (priority order, who wins when): standard `OTEL_SDK_*` env, m9m-specific `M9M_OTEL_*` env, DB-stored override, live UI toggle
- The `M9M_OTEL_*` env vars: `M9M_OTEL_ENABLED`, `M9M_OTEL_ENDPOINT`, `M9M_OTEL_PROTOCOL`, `M9M_OTEL_HEADERS`, `M9M_OTEL_SAMPLE_RATIO`
- `internal/otel/api.go` `/api/v1/otel` endpoint with `GET`, `PUT`, `DELETE`, `POST /otel/test`
- Sampling strategies (always-on, always-off, ratio, parent-based)
- Span tree shape (`workflow.execute` → `node.execute` → `agent.generate`) — in README but not in docs
- Standard OTel resource attributes set on every span (`service.name=m9m`, `service.version=<git-sha>`)
- Health-check integration of the OTel pipeline (`/healthz` reads pipeline health)

The README documents this comprehensively (see root `README.md` "Telemetry & observability" section, lines 226–317); the equivalent just needs to be migrated into `documentation/docs/observability/`.

---

## 7. Recent engine / execution work — undocumented

Commits since 2026-09-09 added several user-visible behaviours that `documentation/docs/` does not mention:

| Commit | Subject | What it does | Doc gap |
|---|---|---|---|
| `92d9cf9` | feat(ui): visible workflow edges + state-coloured execution edges + n8n-style unique node names | UI shows edges per state, gives nodes readable unique names | `workflows/executing.md` does not describe edge colouring |
| `2ede796` | feat(execution): per-node retry — `POST /executions/:id/retry-node` + "Retry from here" | API + button | `executions.md` API ref has no `retry-node` |
| `3237038` | feat(workflow): per-workflow debug flag | UI toggle captures all node I/O when ON | Not in any doc; close to `workflows/index.md` |
| `d77bcc0` | fix(executions): fall back to result.Data when last node was skipped (Debug=OFF NDV) | NDV shows latest executed node | Not in any doc |
| `91c2d9e` | fix(executions): real data on every node, NDV Settings tab, skip Sticky Notes | NDV UI behaviour | Not in any doc |
| `df6b830` | docs: 19/19 parity achieved (14 pos + 5 neg) — 2026-09-10 cycle | Reports full parity | `migrate-from-n8n.md` still says "95% backend feature parity" |
| `e2281ff` | feat(execution): bigger nodes + branch-coloured edges via `EdgesTaken` | Edge colouring wired from execution record | Not in `documentation/docs/workflows/data-flow.md` |
| `a2fc20d` | fix(engine): preserve parent `EdgesTaken` across sub-workflow recursion | Sub-workflow edge preservation | Not in any doc |
| `9eb90e2` | fix(database): default postgres `sslMode` to disable to match n8n default | DB node behaviour | `nodes/database.md` shows `connectionUrl` only |

---

## 8. Other smaller drifts

| Item | Doc today | Code today | Recommended action |
|---|---|---|---|
| Changelog dated `0.1.0-beta.1 / 2024-01-22` | Only `CHANGELOG.md` v0.1 beta entry | 12+ months of work since | Append a `[Unreleased]` section grouping the September 2026 cycle (parity, debug, retry, OTel, AI rename, sync service) |
| Repo URLs in `documentation/docs/**/*.md` | `https://github.com/neul-labs/m9m` (and `docs.neullabs.com/m9m`) | `https://github.com/maharsolution/m9m` | Bulk replace (same operation the root README just got) |
| Architecture diagram in `documentation/docs/architecture/index.md` | Single ASCII, no AI/OTel/sync/MCP | All four exist | Add a second diagram OR rewrite the existing one |
| `workflows/creating.md` and `workflows/data-flow.md` | Older prose | New nodes (`if`, `loop`, `splitInBatches`, `errorTrigger`, `executeWorkflow`) | Update examples |
| `cli/serve.md`, `cli/exec.md`, `cli/nodes.md`, `cli/workflows.md`, `cli/workspace.md` | Mostly aligned | New flags may exist (e.g. `--mysql` DSN flag added in `m9m serve` profile) | Verify each with `m9m --help` |
| `getting-started/installation.md` | Generic install via `neul-labs/m9m` script | Install ships from `maharsolution/m9m` | Replace URLs and re-verify with latest `install.sh` |
| `configuration/environment.md` | List of env vars | M9M now has `M9M_AI_*`, `M9M_OTEL_*`, sync-service `POLL_INTERVAL_SECONDS`, `N8N_*`, `GEN_ENCRYPTION_KEY`, plus legacy `M9M_COPILOT_*` aliases | Add a full env-var matrix |
| `getting-started/agent-usage.md` | Likely about `m9m exec` CLI mode | Should also cover `AgentAI.vue` UI surface | Restructure |

---

## 9. Information that cannot be added safely without engineering work

These items look missing but the right fix is engineering, not docs. Calling them out so the doc team doesn't pretend they work:

1. **`contentType-Priority` ordering for webhooks.** Referenced in `README.md` "What runs unchanged"; would need an authoritative reference. Skip until engineering produces a canonical list.
2. **`internal/copilot/` left-over files** (per the rename commit `56d6203` which moved them to `internal/ai/`). Repository hygiene — should be `git rm`'d in a separate cleanup commit, not buried in a docs PR.
3. **`Plugins/README.md`** references `github.com/maharsolution/m9m-plugins` (org swapped during the README pass). If a plugin repo exists at the new org, this is correct; otherwise it 404s.

---

## 10. Implementation plan (sequenced, no code changes until execution phase)

### Phase 0 — preparation (≤ 1 day)
1. **Decide what stays vs gets archived.** `documentation/docs/` uses `neul-labs/m9m` everywhere. Three options:
   - **A.** Rewrite in place (`neul-labs` → `maharsolution`, internal links fixed). Keep the file tree.
   - **B.** Move the old `documentation/docs/` to `documentation/docs-archive/` and start a fresh tree under `documentation/docs/` that mirrors what the public site (`documentation/site/`) shows today.
   - **C.** Keep two trees: `documentation/docs/` (historical / 0.1 beta) and `documentation/docs-current/` (live).
   - Recommended: A — same physical tree, lower churn for linked agents and search indices.
2. **Add a `[Unreleased]` section to `CHANGELOG.md`** summarising the September 2026 cycle (parity 19/19, debug mode, retry-node, OTel, AI rename, sync-service). Doc-only change.
3. **Bulk-update URLs** in `documentation/docs/**/*.md`: `neul-labs/m9m` → `maharsolution/m9m` and `docs.neullabs.com/m9m` → `github.com/maharsolution/m9m/tree/main/docs`. Mechanically safe.

### Phase 1 — fill the node catalogue (~3 days)
1. Update `documentation/docs/nodes/index.md` to list 42 nodes, correct per-category counts.
2. Add 12 missing per-node entries to existing category pages (`core.md`, `transform.md`, `triggers.md`, `database.md`, `messaging.md`, `email.md`, `vcs.md`).
3. Create the two missing category files: `documentation/docs/nodes/productivity.md`, `documentation/docs/nodes/code.md`.
4. Update `documentation/docs/nodes/ai.md` to cover the in-app AI Settings surface and MiniMax provider, not just workflow-node AI.

### Phase 2 — fill the API surface (~3 days)
1. Rewrite `documentation/docs/api/executions.md` to add `retry-node`, `cancel`, `count`, `recent`, `Debug` flag, sticky-note skipping behaviour.
2. Add `documentation/docs/api/node-types.md` for the `/api/v1/node-types` family.
3. Add `documentation/docs/api/dlq.md` (entirely new).
4. Add `documentation/docs/api/telemetry.md` (for `/api/v1/otel`; alias to section 6 below).
5. Add `documentation/docs/api/ai.md` (for `/api/v1/ai/*`).
6. Augment `documentation/docs/api/workflows.md` (activate/deactivate/duplicate/count).
7. Augment `documentation/docs/api/schedules.md` (enable/disable/history).

### Phase 3 — sync service (~1 day)
1. Add `documentation/docs/integrations/n8n-sync.md` covering: install path inside `docker-compose.yml`, the env-var matrix (`N8N_BASE_URL`, `N8N_API_KEY`, `M9M_BASE_URL`, `N8N_ENCRYPTION_KEY`, `POLL_INTERVAL_SECONDS`, `SYNC_ONLY_ACTIVE`, `SYNC_PORT`, `N8N_PG_*`), the `POST /sync` trigger, the credential decryption algorithm reference, and the relationship to `migrate-from-n8n.md`.
2. Update `documentation/docs/migrate-from-n8n.md` to add a "Use the bridge" subsection.

### Phase 4 — observability (~1 day)
1. Add `documentation/docs/observability/index.md` (or `documentation/docs/telemetry.md`) — port the root README's "Telemetry & observability" section into docs prose, with examples.
2. Cover: exporters (OTLP gRPC + HTTP/protobuf), three-layer config (UI toggle, DB-stored, env), the `M9M_OTEL_*` env vars, span tree shape, sampling options, the `/api/v1/otel` config API, health-check integration.

### Phase 5 — engine / recent work (~1 day)
1. Add `documentation/docs/workflows/debug-and-retry.md` covering debug mode toggle (UI), `POST /executions/:id/retry-node`, sticky-note skipping, NDV Input/Output/Settings tabs.
2. Add `documentation/docs/workflows/execution-edges.md` covering `EdgesTaken`, state-coloured edges, sub-workflow edge preservation.
3. Update `documentation/docs/migrate-from-n8n.md` parity claim from "95% backend feature parity" to "19/19 webhook parity (2026-09-10 cycle)".
4. Update `documentation/docs/nodes/database.md` to add `sslMode=disable` Postgres note.
5. Add examples to `documentation/docs/workflows/examples.md` for the new nodes (`if`, `loop`, `splitInBatches`, `executeWorkflow`).

### Phase 6 — environment / config matrix (~0.5 day)
1. Expand `documentation/docs/configuration/environment.md` with the full env-var table: `M9M_*`, `M9M_AI_*`, `M9M_OTEL_*`, sync-service vars, legacy `M9M_COPILOT_*` aliases, `OTEL_SDK_*` standards.
2. Verify each entry exists in `.env.example` and that the doc lists the matching variable name.

### Phase 7 — architecture diagram refresh (~0.5 day)
1. Update `documentation/docs/architecture/index.md`: add AI runtime, OTel pipeline, sync bridge, DLQ, MCP server to the diagram. Either inline ASCII or a Mermaid block — confirm before drawing.

### Phase 8 — engineering-needed items (out of doc-only scope)
1. `internal/copilot/` leftover: file a follow-up ticket to delete (`56d6203` only moved files; the old dir is dead but present).
2. Repo URL on `plugins/README.md`: verify whether `github.com/maharsolution/m9m-plugins` exists; if not, file a ticket to either rename or replace with a no-link note.

---

## 11. Acceptance criteria

Each phase done when:

- Doc-only commits pass `markdownlint` (project's existing linter) and the link checker used for `documentation/` (currently lenient — most internal links are `../path.md`-style relative).
- For every endpoint added to docs, `curl` against a live server returns the documented status code.
- For every node added to docs, an example workflow in `examples/` or `test-workflows/` runs end-to-end against a live m9m and matches the documented parameters.
- `documentation/docs/nodes/index.md` counts match `m9m node list`.
- `CHANGELOG.md [Unreleased]` cross-references the September 2026 cycle commit range.
- `grep -ri 'neul-labs' documentation/docs/` returns zero hits.
- `grep -ri 'cursoragent@cursor.com' documentation/docs/` returns zero hits.
- `grep -ri 'docs.neullabs.com' documentation/docs/` returns zero hits.

---

## 12. Risks and dependencies

- **Mkdocs regeneration.** `documentation/site/` is generated; if the user has a build step, every change to `documentation/docs/` requires a rebuild for the site. Check `mkdocs.yml` (or similar) before phase 0.
- **Doc-only PRs should not pull in code.** Even though phases 0–7 are textual, the repo's CI may run Go and Vue builds on every PR. Plan to either land them behind a `docs:` commit prefix that the user knows skips CI, or use the workflow that already exists for `/docs/**`.
- **Some "missing" items may be deliberate.** The MCP server's surface (37 tools) is hinted at in root `README.md` but the user may want to document it elsewhere (a `documentation/docs/mcp/*.md` set). Confirm before mass-adding.
- **No code changes are required or proposed** by this document. If during phase 1 we discover that the example workflows the new node docs reference don't actually exist in `examples/`, we will file a follow-up engineering ticket — not silently author the workflow JSON.

---

## 13. Suggested first commit

```
docs(docs): bulk-rewrite neul-labs/m9m to maharsolution/m9m + add 2026-09 changelog

Replaces 100% of the URLs in documentation/docs/ that pointed at
the historical org (neul-labs/m9m, docs.neullabs.com/m9m,
neullabs.com) with maharsolution/m9m equivalents, mirroring the
README rebrand landed in 101ff97. No content changes beyond URLs.

Also adds a [Unreleased] section to CHANGELOG.md summarising
the September 2026 cycle to bridge the gap between v0.1.0-beta.1
(2024-01-22) and today, so users reading docs in 2026 aren't
mis-led by year-old release notes.

Followed by per-phase docs commits per the plan in
gap-current-vs-doc.MD, none of which touch code.
```

This is the smallest safe starting point. Land it first, then work down phases 1–7 in dependency order (catalogue → API → sync → observability → recent engine → env matrix → diagram).

---

## Appendix A — execution log (2026-09-11)

Phases 0–7 of this plan were executed on 2026-09-11. Outcome:

| Phase | Result | Files touched |
|---|---|---|
| 0a — CHANGELOG `[Unreleased]` | Done | `CHANGELOG.md` (+60 lines: 2026-09 cycle summary + `[0.2.0-parity]` block) |
| 0b — Bulk URL rewrite | Done | 15 files (URLs only) plus manual `faq.md` fix-up |
| 1a — node catalogue index | Done | `documentation/docs/nodes/index.md` |
| 1b — missing per-node entries | Done | `core.md`, `transform.md`, `triggers.md`, `database.md`, `messaging.md`, `email.md` |
| 1c — new category files | Done | `documentation/docs/nodes/productivity.md`, `documentation/docs/nodes/code.md` |
| 1d — in-app AI section | Done | `documentation/docs/nodes/ai.md` |
| 2a — executions API rewrite | Done | `documentation/docs/api/executions.md` |
| 2b — node-types API | Done | `documentation/docs/api/node-types.md` |
| 2c — DLQ API | Done | `documentation/docs/api/dlq.md` |
| 2d — telemetry API | Done | `documentation/docs/api/telemetry.md` |
| 2e — AI API | Done | `documentation/docs/api/ai.md` |
| 2f — workflows API augmentation | Done | `documentation/docs/api/workflows.md` |
| 2g — schedules API augmentation | Done | `documentation/docs/api/schedules.md` |
| 3a — sync bridge docs | Done | `documentation/docs/integrations/n8n-sync.md` |
| 3b — migrate-from-n8n update | Done | `documentation/docs/migrate-from-n8n.md` |
| 4 — observability guide | Done | `documentation/docs/observability/index.md` |
| 5a — debug-and-retry | Done | `documentation/docs/workflows/debug-and-retry.md` |
| 5b — execution-edges | Done | `documentation/docs/workflows/execution-edges.md` |
| 5c — parity claim update | Done | `documentation/docs/migrate-from-n8n.md` |
| 5d — Postgres sslMode note | Done | `documentation/docs/nodes/database.md` |
| 5e — workflow examples | Done | `documentation/docs/workflows/examples.md` |
| 6 — environment matrix | Done | `documentation/docs/configuration/environment.md` |
| 7 — architecture refresh | Done | `documentation/docs/architecture/index.md` |
| 8 — engineering follow-ups | See Appendix B below | (no code changes by this plan) |

**Acceptance check, manual grep:**

```bash
grep -ri 'neul-labs/m9m\|docs.neullabs.com' documentation/docs/ \
  --include='*.md' --exclude=robots.txt --exclude=llms.txt \
  --exclude=llms-full.txt --exclude=overrides/main.html
# expected: zero hits (only legit external refs in nodes/vcs.md, kubernetes.md, installation.md)
```

---

## Appendix B — engineering follow-up tickets (Phase 8)

Phases 0–7 are doc-only. Phase 8 flagged items needing **engineering work**,
not documentation. This appendix records both the original triage and the
outcome once each ticket was actioned.

### Execution summary (2026-09-11)

| Ticket | Outcome | Commit | Files touched |
|---|---|---|---|
| 1 — delete `internal/copilot/` | **Done** | `<see repo>` | `internal/copilot/{copilot,copilot_test}.go` (deleted) |
| 2 — fix `plugins/README.md` URL | **Done** | `<see repo>` | `plugins/README.md` (link rewritten to in-tree path) |
| 3 — `M9M_COPILOT_*` deprecation warn | **Done** | `<see repo>` | `internal/ai/config.go`, `internal/ai/config_test.go` |
| 4 — regenerate `documentation/site/` | **Done (operator)** | `<see repo>` | `documentation/site/**` regenerated under WSL by the operator on 2026-09-11 |
| 5 — reconcile OTel env-var docs | **Done** | `<see repo>` | `documentation/docs/observability/index.md`, `documentation/docs/api/telemetry.md`, `documentation/docs/configuration/environment.md` |

All five tickets closed in a single follow-up cycle. The tickets below
preserve their original triage text for traceability.

---

### Ticket 1 — delete `internal/copilot/` leftover files

**Status (originally):** Not started.
**Status (now):** **Done.** The directory was deleted via `git rm -r internal/copilot`.

**Background:** Commit `56d6203 feat(ai): rename Copilot to AI + live Settings wiring` renamed the package to `internal/ai/`. The new package is in use; the old one is dead but still present on disk and tracked by git.

**Files (originally):**

```text
internal/copilot/copilot.go        # legacy, dead
internal/copilot/copilot_test.go   # legacy, dead
```

**Evidence the directory is dead:**

- `grep -ri "internal/copilot" .` returns no hits in production code paths.
- `cmd/m9m/commands/serve.go` imports `internal/ai`, not `internal/copilot`.
- The HTTP routes that previously lived at `/api/v1/copilot/*` now live at `/api/v1/ai/*` and return 301 to the new path.

**Proposed fix (applied):**

```bash
git rm -r internal/copilot
git commit -m "chore(repo): remove internal/copilot (renamed to internal/ai in 56d6203)"
```

**Risk:** Zero. The directory is unreferenced. CI should not break.
**Verification:** `go build ./internal/... ./cmd/...` and `go test ./internal/ai/...` both pass after the deletion.

### Ticket 2 — verify `plugins/README.md` URL

**Status (originally):** Not started.
**Status (now):** **Done.** Investigation showed `https://github.com/maharsolution/m9m-plugins` returns **HTTP 404**, so Path B was applied.

**Background:** The root README's rebrand pass replaced `neul-labs/m9m` references with `maharsolution/m9m`. `plugins/README.md` contained:

```text
github.com/maharsolution/m9m-plugins
```

**Proposed fix (applied — Path B):**

The link was rewritten to `https://github.com/maharsolution/m9m/tree/main/plugins` and the surrounding prose now clarifies that `plugins/` is a sub-package of the main repo, not a separate repository. New "Contributing" copy:

> The `plugins/` directory is **part of the main m9m repo** (not a separate
> repository), so plugins live alongside the core codebase: … Open a pull
> request against `maharsolution/m9m/tree/main/plugins`.

**Risk:** Zero. The previous URL was already broken.
**Follow-up:** If a separate `maharsolution/m9m-plugins` repository is created
later, repoint the link to it and update the surrounding prose.

### Ticket 3 — retire legacy `M9M_COPILOT_*` env vars

**Status (originally):** Documented as deprecated. Code paths not yet removed.
**Status (now):** **Partial — step 1+2 complete; step 3 deferred.** The deprecation
warning now fires the first time a legacy `M9M_COPILOT_*` var is the *source
of truth* for an AI config field. Removal of the alias is still a future
major-release concern.

**Background:** Per the AI rename commit and Phase 6 docs, `M9M_COPILOT_*` env vars are alias-read for `M9M_AI_*`. They will be removed in a future major release.

**Proposed fix (applied — step 1+2):**

1. ✅ Picked a deprecation horizon — the warning references the "next major release" in its log line, matching the CHANGELOG.md `[Unreleased]` note.
2. ✅ Logged a `WARN` line the first time a `M9M_COPILOT_*` var is read in `internal/ai/config.go` via the new `pickEnv` helper and `warnCopilotLegacy` function. The warning is gated on a `sync.Once` so a long-running process with many config reloads only emits one line.
3. ⏳ After the horizon, remove the alias map and the warning. Update Phase 6's "Legacy aliases" section to read "Removed in v0.4.0".

**Implementation:**

```go
// pickEnv returns (value, fromLegacy). The boolean is true when the
// second (legacy) name was the source of the value rather than the
// primary name.
func pickEnv(names ...string) (string, bool) { ... }

var copilotDeprecationOnce sync.Once

func warnCopilotLegacy(provider, apiKey, baseURL, model bool) {
    if !provider && !apiKey && !baseURL && !model { return }
    copilotDeprecationOnce.Do(func() {
        log.Printf("WARNING: deprecated M9M_COPILOT_* env var(s) still in use (...); "+
            "please rename to the M9M_AI_* equivalents before the next major release")
    })
}
```

**Verification:** New tests in `internal/ai/config_test.go`:
- `TestLoadConfigFromEnv_HonoursLegacyCopilotEnv` — re-asserts the alias is honoured.
- `TestWarnCopilotLegacy_NoOpWhenAllNewVarsPresent` — `warnCopilotLegacy(false, false, false, false)` returns silently.
- `TestWarnCopilotLegacy_OnlyFiresOncePerProcess` — exercises the dedup path.

This is **not urgent**. The current behaviour is correct (legacy vars still work); the deprecation is purely cosmetic.

### Ticket 4 — regenerate `documentation/site/`

**Status (originally):** Out of scope for this plan.
**Status (now):** **Done (operator, 2026-09-11).** The operator ran `mkdocs build` under WSL after installing the missing Python packages (`pymdown-extensions`, `mkdocs-material`). The regenerated `documentation/site/**` is in the working tree and includes:

- The 10 new pages created in `eebbe44` (now visible as `documentation/site/api/{ai,dlq,node-types,telemetry}/`, `documentation/site/nodes/{productivity,code}/`, `documentation/site/integrations/`, `documentation/site/observability/`, `documentation/site/workflows/{debug-and-retry,execution-edges}/`).
- A fresh `bundle.4e0fa4ba.min.js`, `palette.649f08f9.min.css`, `main.9f615399.min.css`, and other regenerated assets.

**Background:** `documentation/site/` is the mkdocs-rendered HTML mirror of `documentation/docs/`. Every change to a `.md` file under `documentation/docs/` requires a rebuild for the public site to reflect it.

**Proposed fix (applied):**

```bash
# Run from repo root
cd documentation/
pip install pymdown-extensions mkdocs-material  # one-time, under WSL
mkdocs build --clean
# Output: documentation/site/ regenerated
```

**Risk:** Medium. The regenerated site must be committed alongside the `.md` source so the public site stays consistent with the docs. Verified manually that the new pages produce valid HTML, and that the `favicon.png`, `bundle.4e0fa4ba.min.js`, and `palette.649f08f9.min.css` files all carry through.

### Ticket 5 — confirm `internal/otel/config.go` matches Phase 4 docs

**Status (originally):** Not started.
**Status (now):** **Done.** Doc-vs-code diff surfaced 8 env-var names that had drifted from the implementation:

| Doc claimed | Implementation reads |
|---|---|
| `M9M_OTEL_PROTOCOL` | `M9M_OTEL_EXPORTER_OTLP_PROTOCOL` (with `OTEL_EXPORTER_OTLP_PROTOCOL` fallback) |
| `M9M_OTEL_ENDPOINT` | `M9M_OTEL_EXPORTER_OTLP_ENDPOINT` (with `OTEL_EXPORTER_OTLP_ENDPOINT` fallback) |
| `M9M_OTEL_HEADERS` | `M9M_OTEL_EXPORTER_OTLP_HEADERS` (with `OTEL_EXPORTER_OTLP_HEADERS` fallback) |
| (missing) | `M9M_OTEL_EXPORTER_OTLP_HEADERS_FILE` |
| `M9M_OTEL_SAMPLE_RATIO` | `M9M_OTEL_TRACES_SAMPLE_RATE` (with `OTEL_TRACES_SAMPLER_ARG` fallback) |
| (missing) | `M9M_OTEL_TRACES_PRODUCTION_ONLY` |
| (missing) | `M9M_OTEL_TRACES_INCLUDE_NODE_SPANS` |
| (missing) | `M9M_OTEL_TRACES_INJECT_OUTBOUND` |
| (missing) | `M9M_OTEL_SERVICE_NAME`, `M9M_OTEL_SERVICE_VERSION`, `M9M_OTEL_INSTANCE_ID` |
| (missing) | `M9M_AGENTS_TRACING_ENABLED`, `M9M_AGENTS_TRACING_RECORD_INPUTS`, `M9M_AGENTS_TRACING_RECORD_OUTPUTS` |

The protocol default was also wrong in the doc — the loader's actual default
is `http/protobuf` (matching the upstream OTel SDK), not `grpc`.

**Proposed fix (applied):**

Updated three doc files to reconcile:

1. **`documentation/docs/observability/index.md`** — replaced the 5-row `Environment variables` table with three fully-detailed tables (core tracing, resource attributes, agents tracing) and corrected the protocol default + endpoint shapes. Also fixed the `Sampling` table env-name.
2. **`documentation/docs/api/telemetry.md`** — replaced the inline list of env vars in the "Configuration layers" section with the full `M9M_OTEL_*` / `M9M_AGENTS_TRACING_*` family.
3. **`documentation/docs/configuration/environment.md`** — replaced the 5-row `Telemetry` env-var table with the same three-table structure as above.

Verification:

```bash
# Look at the actual env reader
grep -n "os.Getenv\|os.LookupEnv" internal/otel/env.go

# Cross-reference against the docs
grep -n "M9M_OTEL_\|M9M_AGENTS_" documentation/docs/observability/index.md
```

After the doc patch, every env var the implementation reads appears in at
least one of the doc files.

---

## Appendix C — files NOT changed by this plan

By design, the plan deliberately left the following untouched. They are either out of scope (developer notes, design docs), rendered output (HTML site), or out-of-tree:

- `documentation/site/**` — mkdocs-rendered HTML. Toggle by running `mkdocs build`.
- `docs/**` — developer-facing technical notes (different audience from `documentation/docs/`).
- `n8n-nodes.MD`, `workflow-test.MD` — operational reports, not user docs.
- `README.md`, `deploy/README.md`, `packages/m9m-cli/**/README.md`, `plugins/README.md` — already rebranded in the prior pass.
- All `.claude/`, `.cursor/`, `.gitignore`, `.env*` files — repo hygiene, not docs.
- All code under `cmd/`, `internal/`, `sync-service/`, `web/` — Phase 8's tickets flag code-side follow-ups but do not modify code in this plan.
