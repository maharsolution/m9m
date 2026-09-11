---
title: "n8n ↔ m9m sync bridge"
description: "How the bundled sync-service keeps an m9m deployment in lockstep with an n8n instance."
keywords: "m9m, n8n, sync, migration, credentials, workflow bridge, postgres"
---

# n8n ↔ m9m sync bridge

When you run the bundled `docker-compose.yml`, an HTTP service called **`sync-service`** runs alongside `m9m-backend` (managed by `supervisord` inside the `m9m-backend` container). It periodically polls n8n for active workflow and credential changes and pushes them into m9m via the public REST API.

The bridge is **idempotent**: re-running a cycle is safe. It is **credential-aware**: it decrypts n8n's AES-256-CBC + EVP_BytesToKey envelopes locally and re-encrypts them with m9m's at-rest key before pushing.

---

## When to use it

| Scenario | Use the bridge? |
|---|---|
| Migrating from n8n to m9m, want to move workflows one by one | Yes — let it poll and review before activating each in m9m |
| Running n8n and m9m side-by-side, want a hot standby | Yes — set `SYNC_ONLY_ACTIVE=true` and `POLL_INTERVAL_SECONDS=30` |
| Greenfield m9m install, no n8n to pull from | No — disable the sync service in `docker-compose.yml` |
| One-off import of an exported workflow JSON | No — use `m9m exec workflow.json` |

---

## How it ships

The bridge runs inside the **`m9m-backend`** container, started by `supervisord` alongside `m9m serve` itself. The relevant `docker-compose.yml` block looks like:

```yaml
services:
  m9m-backend:
    # ...
    environment:
      N8N_BASE_URL:            "http://n8n-designer-frontend:5678"
      N8N_API_KEY:             "${N8N_API_KEY:-}"
      M9M_BASE_URL:            "http://localhost:8080"
      POLL_INTERVAL_SECONDS:   "30"
      SYNC_ONLY_ACTIVE:        "true"
      SYNC_PORT:               "8001"
      N8N_ENCRYPTION_KEY:      "${N8N_ENCRYPTION_KEY:-}"
      N8N_PG_HOST:             "postgres-db"
      N8N_PG_PORT:             "5432"
      N8N_PG_USER:             "database_admin"
      N8N_PG_PASSWORD:         "${N8N_PG_PASSWORD:-Secure_Db_Pass_2026}"
      N8N_PG_DATABASE:         "n8n_prod"
```

The bridge listens on port **8001** by default; change `SYNC_PORT` to move it.

---

## Endpoints

The bridge is a small FastAPI app exposed on port 8001 (or `SYNC_PORT`).

### `GET /`

Plain-text banner with the current config and a list of endpoints.

```text
m9m <-> n8n sync bridge
  n8n:        http://n8n-designer-frontend:5678
  m9m:        http://m9m-backend:8080
  poll:       30s (background asyncio task)
  only_active: True
  sync port:  8001
Endpoints:
  GET  /        -> this message
  GET  /health  -> liveness probe
  GET  /status  -> last-cycle summary
  POST /sync    -> run one sync cycle now
```

### `GET /health`

```json
{ "status": "ok" }
```

### `GET /status`

Last-cycle summary. Useful for health checks and dashboards.

```json
{
  "last_cycle_at":     "2026-09-11T08:30:00Z",
  "last_cycle_source": "poll",
  "last_cycle_count":  7,
  "is_running":        false
}
```

| Field | Meaning |
|---|---|
| `last_cycle_at` | ISO 8601 timestamp of when the last cycle finished, or `null` if none yet |
| `last_cycle_source` | `"poll"` (background) or `"manual"` (triggered by `POST /sync`) |
| `last_cycle_count` | Number of workflows touched in the last cycle |
| `is_running` | `true` if a cycle is currently in progress |

### `POST /sync`

Run one sync cycle synchronously and return the summary.

```bash
curl -X POST http://localhost:8001/sync
```

Response (HTTP 200):

```json
{
  "results": [
    { "workflowId": "wf-001", "name": "Daily Report",  "status": "ok"    },
    { "workflowId": "wf-002", "name": "Hourly Sync",   "status": "ok"    },
    { "workflowId": "wf-003", "name": "Webhook Echo",  "status": "failed", "error": "..." }
  ],
  "started_at": "2026-09-11T08:30:00Z",
  "duration_ms": 1423
}
```

If **any** result is `failed`, the response is **HTTP 502** with the same body. The bridge does not raise on per-workflow failure — a single bad workflow does not block the cycle.

---

## Background polling

A background `asyncio` task polls n8n every `POLL_INTERVAL_SECONDS` seconds (default 30) and pushes any changes into m9m. On startup, the task is created; on shutdown, it is cancelled cleanly. No external cron is required.

For each n8n workflow, the bridge:

1. Fetches the full workflow from `GET {N8N_BASE_URL}/api/v1/workflows/{id}`.
2. Computes a content hash and skips the workflow if it matches the last cycle's hash (no-op detection).
3. Translates `typeVersion` (n8n's float) to an integer (m9m expects int) — lossy but adequate for parity.
4. Pushes to m9m with `POST /api/v1/workflows`. On non-2xx that looks like a duplicate, retries with `PUT /api/v1/workflows/{id}`. Both failing logs the error and moves on.
5. Pushes credentials it can decrypt from `N8N_PG_*` (see below); credentials whose `data` column is unreadable are skipped with a warning.

The bridge only runs against an instance it can reach over HTTP. If either n8n or m9m is unreachable, the cycle logs the error and waits for the next tick.

---

## Credential decryption

n8n's public REST API does **not** return the `data` field of credentials (it's marked `writeOnly`, and `?includeData=true` returns redacted placeholders). To recover the actual encrypted blobs, the bridge connects directly to **n8n's Postgres database** and reads the `credentials_entity.data` column.

The `data` column is a JSON-encoded string of AES-256-CBC ciphertext with a random 8-byte salt prefix (`"Salted__" + 8-byte salt + ciphertext`), all base64-encoded. Decryption uses OpenSSL's `EVP_BytesToKey` MD5-based KDF — the same algorithm n8n uses (`n8n/packages/core/src/Cipher.ts`).

```python
def decrypt_n8n_credential_data(ciphertext_b64, password):
    raw       = base64.b64decode(ciphertext_b64)
    assert raw.startswith(b"Salted__")
    salt      = raw[8:16]
    ciphertext = raw[16:]
    key, iv   = evp_bytes_to_key(password.encode(), salt)
    plaintext = AES.new(key, AES.MODE_CBC, iv).decrypt(ciphertext)
    return json.loads(unpad(plaintext, 128))
```

`password` is the same key you set as `N8N_ENCRYPTION_KEY` (or `GEN_ENCRYPTION_KEY` — both are accepted).

After decrypting, the bridge re-encrypts the plaintext credentials under m9m's at-rest key by POSTing/PUT-ing them through m9m's `/api/v1/credentials` endpoint. The plaintext is held in process memory only during the cycle.

---

## Environment variables

| Variable | Required? | Default | Description |
|---|---|---|---|
| `N8N_BASE_URL` | yes | `http://n8n-designer-frontend:5678` | n8n REST API base URL |
| `N8N_API_KEY` | yes | — | n8n API key (Settings → API in the n8n UI) |
| `M9M_BASE_URL` | yes | `http://m9m-backend:8080` | m9m REST API base URL |
| `POLL_INTERVAL_SECONDS` | no | `30` | Background poll cadence |
| `SYNC_ONLY_ACTIVE` | no | `true` | If `true`, only push `active: true` workflows from n8n |
| `SYNC_PORT` | no | `8001` | Port the bridge's HTTP API listens on |
| `N8N_ENCRYPTION_KEY` | yes (for credentials) | — | The key n8n was started with; reads `GEN_ENCRYPTION_KEY` as fallback |
| `N8N_PG_HOST` | yes (for credentials) | `postgres-db` | Hostname of n8n's Postgres |
| `N8N_PG_PORT` | no | `5432` | Postgres port |
| `N8N_PG_USER` | no | `database_admin` | Postgres user |
| `N8N_PG_PASSWORD` | no | `Secure_Db_Pass_2026` | Postgres password (change in production) |
| `N8N_PG_DATABASE` | no | `n8n_prod` | Postgres database |

---

## Disabling the bridge

If you do not want the bridge running, comment out the relevant block in `docker-compose.yml` (or set `POLL_INTERVAL_SECONDS=0` to make it a no-op — though the FastAPI process still listens on 8001, so the cleaner option is to remove the `command:` that runs `supervisord` and instead run `m9m serve` directly).

---

## See also

- [Migrate from n8n](../migrate-from-n8n.md) — the bigger migration story
- [Credentials API](../api/credentials.md) — what m9m's credential endpoints expect
- [Workflows API](../api/workflows.md) — what m9m's workflow endpoints expect
- [docker-compose recommendation](../../README.md#docker-compose-recommended)
