# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Platform and operations engineers who self-host m9m as a replacement for n8n in production. They arrive with an existing n8n estate — workflow JSON, expressions, credentials, node habits — and are responsible for keeping automations running: migrating them, securing their credentials, and diagnosing failures under uptime pressure. The web UI is the operator's working surface (author, run, monitor); the CLI, Go SDK, and MCP server serve the same operators from the terminal and from AI agents.

## Product Purpose

m9m runs n8n workflow JSON unchanged on a single Go binary. It exists so a team can move a production automation estate off n8n without rewriting anything, and operate it with a smaller footprint and fewer runtime failure modes: same JSON, same expression syntax, same credential model, same node catalog; different runtime — statically linked, no Node.js, no npm dependency tree.

Success means an ops engineer can point m9m at an n8n instance, import workflows and credentials, and trust repeated execution — deterministic, fast, and observable — without babysitting process restarts or npm upgrades.

## Positioning

A drop-in n8n replacement. The mechanism a neighbouring product cannot truthfully copy is the combination of n8n-compatible artifacts (workflow JSON, expression syntax, credential envelopes, node types, REST wire formats) with a zero-npm deterministic runtime — plus the shipped n8n → m9m sync bridge and a built-in MCP server for AI agents.

Canonical tagline (verbatim, from `CLAUDE.md`): "The n8n alternative without the bugs — faster, more reliable workflow automation."
Canonical one-liner: "m9m is a drop-in n8n alternative in Go — 5–10× faster, 70% lower memory, deterministic execution, zero npm dependencies. Run n8n workflows unchanged."

## Operating Context

- **Self-hosted deployments.** Docker Compose is the recommended path; Helm chart and manifests live in `deploy/`; images ship via GHCR and Docker Hub. The internal deployment runs m9m beside n8n on a shared server, with images built by GitHub Actions into a private Harbor registry and pulled by a deploy script that recreates only `m9m-backend`.
- **Migration is the entry ritual.** Import n8n workflow JSON; credentials travel as encrypted envelopes (instance-key cipher compatible with n8n's `N8N_ENCRYPTION_KEY` scheme), and a workflow bundle can carry them via `includeCredentials=true`.
- **Sync bridge.** `sync-service/sync.py` polls n8n's REST API with `X-N8N-API-KEY`, decrypts workflow and credential envelopes locally, and pushes them into m9m — the estate stays in lock-step without hand-written glue.
- **Parity is a working practice, not a slogan.** `scripts/parity` runs a 79-case suite that compares m9m against a live n8n across API contracts, wire formats, sync behaviour, fault handling, stability and drift. At 2026-11-12: 67 PASS, 7 FAIL, 5 BLOCKED. `workflow-test.MD`, `parity/parity_gaps.json` and a Google Sheets validation sheet hold the record; the sheet is the source of truth the team reconciles against.
- **Operator surfaces.** Web UI (Vue) for authoring, running and monitoring; CLI (`serve`, `exec`, `schedule`, `node list`, `benchmark`, `demo`); MCP server with 37 tools for Claude Code and Cursor; Prometheus metrics.
- **Credentials.** Stored encrypted at rest. API authentication and credential-secret redaction are being introduced precisely because the earlier open API exposed `data` on request.
- **Performance baseline.** The live `post_balance` webhook runs at a ~3.5 s median on the shared host (n8n on the same host: ~3.8 s); before the retry-loop fix the same webhook took ~23 s.

## Capabilities and Constraints

- Runs n8n workflow JSON, expression syntax, and credentials unchanged across 40+ built-in node types (42 executors registered). Community `n8n-nodes-*` npm packages and n8n Cloud-only features are not supported (`docs/migration/from-n8n.md`).
- Single statically linked Go binary; macOS, Linux, Windows; AMD64 and ARM64; MIT licensed.
- Custom logic is sandboxed JavaScript (Goja) or Python — not arbitrary npm imports.
- Deterministic execution; no JavaScript-runtime heap growth; no npm transitive-dependency CVEs in the runtime path.
- Queue backends: memory, Redis, RabbitMQ. Storage: MySQL in production, SQLite/memory for tests. Public surface: REST `/api/v1`, webhooks, Prometheus metrics, MCP.
- Known open parity gaps at 2026-11-12 (documented, not hidden): binary response body shape (c406), AI `ai_languageModel` / `ai_memory` connection types (c601), node-name quoting (c607), sub-workflow child execution record (c702), missing-parameter error shape (c802), credential list row shape (p12/p14).
- Open decisions: no accessibility standard has been confirmed for this product; no image-generation pipeline exists, so new surfaces are built code-first.
- Secret hygiene backlog: the exposed Mongo Atlas password and Aiven Kafka password still need rotation, and the Google Sheets API key currently sits in script source — move to environment, rotate.

## Brand Commitments

- Name: `m9m`, lowercase, never stylised differently.
- Tagline and one-liner are fixed strings; use them verbatim in every user-facing surface (README, docs, package descriptions, slides).
- The editor must feel familiar to n8n users (operator-confirmed binding constraint): n8n interaction habits, node vocabulary and workflow mental model are the baseline the UI is measured against.
- Voice: operator-facing and factual. No invented testimonials, customers, benchmarks, pricing or deployment claims.
- Identity assets beyond the wordmark are not recorded; no DESIGN.md or logo system exists yet.

## Evidence on Hand

- `README.md` plus `docs/performance-report.md`: measured comparison against n8n (5–10× execution, ~500 ms cold start, ~150 MB idle memory, ~300 MB container), reproducible with `m9m benchmark`.
- Live parity record: `workflow-test.MD` (regenerated from the latest live report), `parity/parity_gaps.json` (23 observations), Google Sheets validation sheet, `scripts/parity/**` harness with 55 pytest cases and Go parity locks.
- Live deployment evidence: m9m and n8n side by side on the shared host, health and node-type endpoints, and measured webhook latency (7 hits, 3304–3967 ms) with the pre-fix baseline (~23 s) recorded for comparison.
- Runnable examples: `examples/`, `test-workflows/`, `m9m demo` (six workflows executing end to end).
- Documentation tree: `docs/` including `migration/from-n8n.md`, per-node references (`docs/nodes/*.md`), `docs/mcp/README.md`, `docs/deployment/production.md`.
- Absences future work must not fabricate: no DESIGN.md, no logo or brand asset library, no customer testimonials, no case studies or press, no pricing or licence claims beyond MIT, no accessibility conformance report, no recorded visual-identity decisions.

## Product Principles

1. **Compatibility before novelty.** n8n workflows, expressions, credentials and node behaviour stay untouched; m9m-native additions are strictly additive to that contract.
2. **Runtime reliability is the product.** Deterministic execution, bounded retries, pooled clients, no leak-prone runtime — the operator's trust depends on the same run producing the same result.
3. **Honest surfaces.** Report real parity state and real measured numbers. A visible FAIL is worth more than a hidden regression.
4. **Small operational footprint.** One binary, minimal dependencies, straightforward Compose deployment — fewer moving parts to diagnose at 3am.
5. **Operate-first interface.** The UI exists so an operator can build, run and diagnose without hand-editing JSON, and it must feel like n8n to the people already fluent in it.

## Accessibility & Inclusion

No product-specific accessibility requirement or conformance report has been established yet. Treat WCAG AA semantics as an open decision to confirm with the operator before it is claimed anywhere; do not assert conformance from the absence of evidence.
