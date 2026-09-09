# n8n Credentials REST API Reference

_Source: <https://docs.n8n.io/connect/n8n-api/credential> (Markdown: <https://docs.n8n.io/connect/n8n-api/credential.md>)_

This file captures the endpoints a `sync-service` must implement to keep the credential store between n8n and m9m in lock-step. The parity cycle described in `workflow-test.MD` Section I depends on this.

## Section 1 — Conventions

| Header | Required | Value |
|--------|----------|-------|
| `X-N8N-API-KEY` | yes | instance owner / admin API key from n8n |
| `Accept` | yes | `application/json` |

Base URL on this project: `http://187.77.113.218:5678`. All paths below are prefixed with `/api/v1`.

> **Security note:** n8n never returns the `data` field of a credential through the public API (it is `writeOnly` on POST/PATCH and stripped on GET). The actual secrets must be obtained out-of-band (e.g. from the n8n database). For parity testing of webhooks we only need the **list of credential names + types** + the credential IDs so that workflow nodes' `node.credentials` map can be resolved.

## Section 2 — Endpoints

### 2.1 `GET /credentials`

List all credentials. Returns `{ data: [ { id, name, type, createdAt, updatedAt, shared } ], nextCursor }`.

`limit` (default 100, max 250) and `cursor` for pagination.

### 2.2 `GET /credentials/{id}`

Returns a single credential envelope: `id, name, type, isManaged, isGlobal, isResolvable, resolvableAllowFallback, resolverId, createdAt, updatedAt`. **No `data` field.**

### 2.3 `POST /credentials`

Create. Body:
```json
{
  "name": "string (required)",
  "type": "string (required, e.g. httpBasicAuth, postgres, mysql)",
  "data": { ... }                       // required, writeOnly
}
```

Response: `200` with full credential envelope.

### 2.4 `PATCH /credentials/{id}`

Partial update. All fields optional. If `type` changes, `data` is required.

### 2.5 `DELETE /credentials/{id}`

Owner-only. Returns `200` on success, `401/404` otherwise.

### 2.6 `POST /credentials/{id}/test`

Tests the stored credential data against its type's validator. Returns `{ status: "OK" | "Error", message }`.

### 2.7 `GET /credentials/schema/{credentialTypeName}`

Returns the JSON Schema for the credential type's `data`. Useful when a sync worker needs to validate a payload before POST.

### 2.8 `PUT /credentials/{id}/transfer`

Body: `{ "destinationProjectId": "..." }`. Project-scoped move.

## Section 3 — What m9m needs

m9m's sync-service currently copies **workflows only**, dropping the per-node `credentials` field. For workflows that reference credentials (e.g. `webhook_callrest` uses `httpBasicAuth`, `process-batch-orders` reads a postgres connection), m9m nodes resolve `node.credentials[type][name]` against an empty store and silently fall back to anonymous — which is why the parity sheet currently shows m9m responses like `{"body":{"variable":"1"}}` for positive/3 instead of an authed upstream call.

We need to mirror the **credential envelope** in m9m:

| m9m API | Method | Path | Purpose |
|---------|--------|------|---------|
| List credentials | `GET` | `/api/v1/credentials` | match n8n (data stripped) |
| Get credential envelope | `GET` | `/api/v1/credentials/{id}` | match n8n |
| Register credential envelope | `POST` | `/api/v1/credentials` | m9m-side insert (data not required for parity testing) |
| Update credential envelope | `PATCH` | `/api/v1/credentials/{id}` | m9m-side update |
| Delete credential | `DELETE` | `/api/v1/credentials/{id}` | m9m-side delete |
| Test credential | `POST` | `/api/v1/credentials/{id}/test` | m9m-side type check |
| Credential type schema | `GET` | `/api/v1/credentials/schema/{type}` | m9m-side schema |

The m9m credential store is keyed by `id` and stores the same envelope as n8n (minus the `data` field for parity reasons — actual secrets belong in a separate vault / m9m config; the parity test only needs the **envelope + id-stable mapping**).

## Section 4 — Sync-service changes required

`sync-service/sync.py` must, on each `poll` / `manual /sync`:

1. `GET /api/v1/credentials?limit=250` from n8n → upsert into m9m's credential store.
2. For each workflow pulled from n8n, **resolve** `nodes[*].credentials`:
   - `node.credentials = { typeName: { id, name } }` — the `id` must match what we stored from step 1.
   - If n8n returns a credential `id` that we have not yet registered, fetch it with `GET /api/v1/credentials/{id}` first.
3. POST/PUT the workflow to m9m with the `credentials` map fully populated.

The current sync loop drops `credentials` from the workflow body because m9m refuses to unmarshal it. Once the credential endpoints exist, the unmarshal path in m9m's `POST /api/v1/workflows` must accept the `credentials` field and store it verbatim (so engine lookup via `Workflow.GetCredential(node, type)` returns the same `{id, name}` pair).

## Section 5 — End-to-end parity blast radius

Once credentials are synced:

- `webhook_callrest` (positive/3) and any other HTTP-Request-with-auth node will start using the actual credential envelope.
- The orchestrator's positive/3 case `{"body":{"variable":"1"}}` may change (the upstream `httpbin` returns 200 if basic auth is provided).
- Negative/3 (`"var":"a"` — auth challenge) behaviour must be re-verified after the credential store changes.
