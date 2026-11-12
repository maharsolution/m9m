---
title: "API Authentication"
description: "m9m closes its API by default: JWT sessions, API keys and the public allowlist."
keywords: "m9m API, REST API, workflow API, n8n API compatibility, auth, API key, JWT"
---

# API Authentication

The m9m API is **closed by default**. Every route under `/api/v1` needs a
credential - a bearer token or an API key - except the small, explicit
public allowlist at the bottom of this page.

The one and only auth system of m9m is `internal/auth`: it owns the users,
the roles, the API keys, the sessions and the JWT format. The users, keys
and sessions live in the **same storage backend as the workflows** (MySQL in
production, memory in tests), so they survive a restart of the instance.

## Environment contract

The authentication configuration is read from the environment at startup
(`DefaultAPIServerConfig` in `internal/api/server.go`). `config/config.yaml`
mirrors the same contract in its `security:` block - it is documentation,
not a second source of truth.

| Variable | Default | Meaning |
|----------|---------|---------|
| `M9M_AUTH_ENABLED` | `true` | `false` turns the auth middleware off and leaves the API open. |
| `M9M_JWT_SECRET` | - | Signs the session tokens. **Minimum 32 characters.** Required when auth is on and no `M9M_API_KEY` is set. Without it the instance generates an ephemeral secret at boot, so session tokens die with the process. |
| `M9M_API_KEY` | - | Optional automation key. Presented in the `X-API-Key` header. |
| `M9M_ADMIN_EMAIL` | - | Owner account of a fresh instance. |
| `M9M_ADMIN_PASSWORD` | - | Password of that owner account. |
| `M9M_ALLOW_FIRST_REGISTER` | `true` when no owner is configured | Opens the first-boot registration window. |

The server **refuses to start** when auth is enabled and neither
`M9M_JWT_SECRET` nor `M9M_API_KEY` is set - an instance never serves an
open API by accident.

```bash
# Generate the secrets (do NOT print them into a log or a ticket)
openssl rand -hex 32          # M9M_JWT_SECRET
openssl rand -hex 32          # M9M_API_KEY
```

## Authentication methods

| Method | Header | Use case |
|--------|--------|----------|
| JWT session token | `Authorization: Bearer <token>` | Web UI, interactive clients |
| API key | `X-API-Key: <key>` | Automation, parity harness, sync bridge |

### A key in the query string is rejected

`?apiKey=...` / `?api_key=...` never authenticates. Query strings end up
in access logs, proxy caches and browser history, so the middleware logs a
security warning and answers `401`.

### JWT session

```bash
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "owner@example.com",
  "password": "..."
}
```

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": { "id": "user_1", "email": "owner@example.com", "role": "admin" }
}
```

The token travels in the `Authorization` header:

```bash
curl http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

Sessions are stored, so `POST /api/v1/auth/logout` revokes the token
immediately and a restart does not invalidate it.

### API key

```bash
POST /api/v1/auth/api-keys
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "name": "CI/CD Integration",
  "permissions": ["workflow:read", "workflow:execute"]
}
```

```json
{
  "apiKey": {
    "id": "apik_...",
    "name": "CI/CD Integration",
    "prefix": "AbCdEfGh",
    "permissions": ["workflow:read", "workflow:execute"],
    "active": true
  },
  "key": "<the plaintext key - shown exactly once>"
}
```

!!! warning "Save the key"
    The plaintext key is returned once and never again. The store keeps
    only the SHA-256 digest plus the first 8 characters (the prefix) as
    the lookup index, so a lost key cannot be recovered - it has to be
    revoked and replaced.

Revoke a key:

```bash
DELETE /api/v1/auth/api-keys/<id>
```

A revoked key answers `401` on its next use.

## Roles and permissions

| Role | Scopes |
|------|--------|
| `admin` (owner) | everything, including user management and the credential export/import |
| `member` | read/write workflows, execute them, read + write credential envelopes |
| `viewer` | read-only workflows and executions |

Two fine-grained grants exist. They are **never** granted by a role -
they have to be attached to an individual API key on purpose:

| Grant | Effect |
|-------|--------|
| `credentials:read-data` | the key may read the secret `data` blob of a credential |
| `credentials:export` | the key may call the credential export/import bundle |

## Credential secrets

`GET /api/v1/credentials/{id}` returns the secret `data` payload **only**
for

* a session of an editor role (`admin` or `member`), or
* an API key with the explicit `credentials:read-data` grant.

Every other caller (an automation key without that grant, a viewer
session) gets the safe envelope - the same shape the list endpoint
returns, without the secrets. Every read of the secret payload is written
to the audit log (metadata only: user, IP, credential names, timestamp -
never values).

`GET /api/v1/credentials/export` and `POST /api/v1/credentials/import`
are admin-only and also write a structured audit entry. The export keeps
the n8n wire format: the credential payload stays encrypted.

## Auth routes

| Route | Auth |
|-------|------|
| `POST /api/v1/auth/login` | public |
| `POST /api/v1/auth/register` | public **only** while the instance has no user |
| `POST /api/v1/auth/invites/accept` | public |
| `POST /api/v1/auth/logout` | session |
| `GET /api/v1/auth/me`, `PATCH /api/v1/auth/me` | session |
| `GET /api/v1/auth/users`, `POST /api/v1/auth/users` | session (admin) |
| `GET/PATCH/DELETE /api/v1/auth/users/{id}` | session (admin) |
| `POST /api/v1/auth/invites` | session |
| `GET/POST /api/v1/auth/api-keys` | session |
| `DELETE /api/v1/auth/api-keys/{id}` | session |

### Bootstrap owner and the first-registration window

With `M9M_ADMIN_EMAIL` + `M9M_ADMIN_PASSWORD` the instance creates its
owner account on the first boot and closes the public registration window.
The call is idempotent: a restart never resets the password.

Without those variables the window stays open until the **first**
registration succeeds - that first account becomes the owner, and the
window closes. Afterwards a new account can only be created by an admin
through an invitation token.

## Public endpoints (the allowlist)

These are the only paths that reach a handler without a credential:

| Endpoint | Why |
|----------|-----|
| `GET /health`, `GET /healthz` | liveness probes |
| `GET /ready` | readiness probe |
| `POST /api/v1/auth/login` | login |
| `POST /api/v1/auth/register` | first-boot registration (only while zero users) |
| `POST /api/v1/auth/invites/accept` | invitation acceptance |
| `POST /api/v1/auth/password/reset`, `.../reset/confirm` | password reset |
| `POST /webhook/*` | webhook triggers - they carry their own authentication |
| `/api/v1/webhooks/*` | webhook management for the webhook nodes |

Everything else under `/api/v1` answers `401` without a credential.

## Error format

```json
{
  "error": true,
  "message": "Unauthorized: valid authentication required",
  "code": 401
}
```

| Status | Meaning |
|--------|---------|
| `401` | no credential, wrong key, expired or revoked session, key in the query string |
| `403` | authenticated, but the role or the grant is insufficient (e.g. credential export without admin) |

## Example: full flow

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@example.com","password":"..."}' \
  | jq -r '.token')

# 2. Create an automation key
API_KEY=$(curl -s -X POST http://localhost:8080/api/v1/auth/api-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"automation","permissions":["workflow:read","workflow:execute"]}' \
  | jq -r '.key')

# 3. Use it
curl http://localhost:8080/api/v1/workflows -H "X-API-Key: $API_KEY"
```

## Operational notes

- Rotate the JWT secret and the API keys on a schedule - see
  `docs/security/rotation-checklist.md`.
- Never put a key into a URL, a log line or a spreadsheet.
- Use TLS in front of the API: the bearer token and the API key are
  bearer credentials.
- Keep `M9M_AUTH_ENABLED=true` in production. `false` reopens the API and
  is the rollback switch of last resort.

## See Also

- `config/config.yaml` - the `security:` block mirrors this contract
- `docs/security/rotation-checklist.md` - rotation of exposed secrets
- [Security Configuration](../configuration/security.md) - full security settings
