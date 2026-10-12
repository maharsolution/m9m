#!/usr/bin/env python3
"""
Polls n8n's REST API for active workflows and pushes each one into m9m
via its REST API (http://m9m-backend:8080/api/v1/workflows) — NOT via
the m9m CLI, because the CLI's `create` writes to a workspace-scoped
store that the running `m9m serve` process does not read from. Only
the REST API is guaranteed to hit the same storage the live server uses.

Known unknowns, called out rather than silently assumed:
    - Whether POST on an existing id updates it or errors. This script
      tries POST first, and on a non-2xx response whose body suggests a
      duplicate, retries with PUT /api/v1/workflows/<id>. If PUT also
      fails, it logs the raw response and moves on rather than crashing
      the loop, so you can see the exact error text and we can fix this
      script's assumptions instead of guessing further.
    - typeVersion sanitization (float -> int) is lossy. n8n's minor
      version bumps sometimes carry real parameter/behavior differences,
      not just a version label. Treat synced workflows as "probably fine"
      and re-verify anything you depend on precisely.

Deployment model (after the supervisord refactor):
    This file is a FastAPI app that runs under uvicorn inside the main
    m9m container, managed by supervisord alongside `m9m serve`. It
    exposes an HTTP API on SYNC_PORT (default 8001) with three
    read-only endpoints (`/`, `/health`, `/status`) and one trigger
    (`POST /sync`). A background asyncio task also polls n8n every
    POLL_INTERVAL_SECONDS and pushes changes into m9m, so an external
    cron is no longer required.

Runs as a FastAPI/uvicorn ASGI app. A background asyncio task polls
every POLL_INTERVAL_SECONDS; POST /sync triggers an immediate cycle.
"""
import asyncio
import base64
import hashlib
import json
import os
import time
import urllib.error
import urllib.request
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.responses import JSONResponse, PlainTextResponse

N8N_BASE_URL = os.environ.get("N8N_BASE_URL", "http://n8n-designer-frontend:5678")
N8N_API_KEY = os.environ.get("N8N_API_KEY", "")
M9M_BASE_URL = os.environ.get("M9M_BASE_URL", "http://m9m-backend:8080")
POLL_INTERVAL_SECONDS = int(os.environ.get("POLL_INTERVAL_SECONDS", "30"))
ONLY_ACTIVE = os.environ.get("SYNC_ONLY_ACTIVE", "true").lower() == "true"
SYNC_PORT = int(os.environ.get("SYNC_PORT", "8001"))
# N8N_ENCRYPTION_KEY must match the value n8n was started with. The
# docker-compose injects GEN_ENCRYPTION_KEY into the n8n container;
# sync.py reads the same variable so the cipher key stays in lockstep.
# Falls back to N8N_ENCRYPTION_KEY so direct n8n-config deployments
# also work without code changes.
N8N_ENCRYPTION_KEY = os.environ.get("N8N_ENCRYPTION_KEY") or os.environ.get("GEN_ENCRYPTION_KEY", "")

# n8n Postgres connection. n8n's PUBLIC REST API does NOT return the
# `data` field of credentials (it's `writeOnly` per the OpenAPI spec,
# and `?includeData=true` returns REDACTED placeholders). To get the
# actual encrypted blobs we query the same Postgres that backs the
# n8n UI directly. Connection details default to the in-network DSN
# of the docker-compose `postgres-db` service; override via env vars
# if you deploy elsewhere.
N8N_PG_HOST = os.environ.get("N8N_PG_HOST", "postgres-db")
N8N_PG_PORT = int(os.environ.get("N8N_PG_PORT", "5432"))
N8N_PG_USER = os.environ.get("N8N_PG_USER", "database_admin")
N8N_PG_PASSWORD = os.environ.get("N8N_PG_PASSWORD", "Secure_Db_Pass_2026")
N8N_PG_DATABASE = os.environ.get("N8N_PG_DATABASE", "n8n_prod")

_last_hash = {}  # n8n workflow id -> content hash, to skip unchanged workflows

# Concurrency control + last-cycle status surfaced via /status.
_cycle_lock = asyncio.Lock()
_last_cycle_at = None          # iso8601 string of when last cycle finished
_last_cycle_source = None      # "poll" | "manual" | None
_last_cycle_count = 0          # number of workflows touched in the last cycle
_is_cycle_running = False      # toggled inside the lock

# Background poll task handle; created on startup, cancelled on shutdown.
_poll_task: asyncio.Task | None = None


# ---------------------------------------------------------------------------
# HTTP helpers (synchronous; called via asyncio.to_thread from async code)
# ---------------------------------------------------------------------------

def http_json(method, url, headers=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = resp.read().decode()
            return resp.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, raw


def fetch_n8n_workflows():
    status, body = http_json(
        "GET",
        f"{N8N_BASE_URL}/api/v1/workflows",
        headers={"X-N8N-API-KEY": N8N_API_KEY, "Accept": "application/json"},
    )
    if status != 200:
        print(f"[sync] n8n API error {status}: {body}")
        return []
    if isinstance(body, dict):
        return body.get("data", [])
    return body or []


def sanitize_type_versions(workflow):
    """Float typeVersion (n8n minor revisions, e.g. 2.1) -> int, since
    m9m's parser only accepts an integer. See docstring for the caveat."""
    for node in workflow.get("nodes", []):
        tv = node.get("typeVersion")
        if isinstance(tv, float):
            node["typeVersion"] = int(tv)
    return workflow


def push_to_m9m(workflow, source="manual"):
    """Push a single workflow. Returns ("created"|"updated"|"failed", status, body)."""
    wf_id = workflow.get("id")
    headers = {"Content-Type": "application/json"}

    status, body = http_json("POST", f"{M9M_BASE_URL}/api/v1/workflows", headers, workflow)
    if 200 <= status < 300:
        print(f"[sync:{source}] pushed '{workflow.get('name')}' ({wf_id}) -> m9m, status {status}")
        return "created", status, body

    looks_like_duplicate = status in (409,) or (
        isinstance(body, dict) and "exist" in json.dumps(body).lower()
    )
    if looks_like_duplicate and wf_id:
        status2, body2 = http_json("PUT", f"{M9M_BASE_URL}/api/v1/workflows/{wf_id}", headers, workflow)
        if 200 <= status2 < 300:
            print(f"[sync:{source}] updated '{workflow.get('name')}' ({wf_id}) -> m9m, status {status2}")
            return "updated", status2, body2
        print(f"[sync:{source}] PUT fallback also failed for '{workflow.get('name')}' ({wf_id}): "
              f"status {status2}, body: {body2}")
        return "failed", status2, body2

    print(f"[sync:{source}] failed to push '{workflow.get('name')}' ({wf_id}): status {status}, body: {body}")
    return "failed", status, body


# ---------------------------------------------------------------------------
# Credential sync (added 2026-09-09, see parity/parity_gaps.json)
#
# n8n stores credentials as AES-256-CBC ciphertext (EVP_BytesToKey/MD5
# derivation) under N8N_ENCRYPTION_KEY. m9m's REST API accepts the
# plaintext JSON envelope and re-encrypts at rest under its own key —
# but the bridge has to decrypt on the way through. Otherwise the
# `data` field sent to m9m would be the opaque ciphertext blob, and
# the Webhook handler would fail to resolve the username/password it
# needs for auth validation.
# ---------------------------------------------------------------------------

def decrypt_n8n_credential_data(ciphertext_b64):
    """Decrypt an n8n credential.data payload using AES-256-CBC with
    EVP_BytesToKey(MD5) key derivation. Returns the parsed JSON
    envelope.

    Wire format: `base64("Salted__" + 8-byte-salt + AES-256-CBC(
    pkcs7_pad(plaintext), key=evp_bytes_to_key(password, salt),
    iv=evp_bytes_to_key(password, salt)[16:32]))`.

    The same algorithm lives in Go at internal/credentials/n8n_cipher.go
    so we can swap implementations and get identical results — the
    unit test `TestN8NCipherMatchesKnownVector` cross-checks both.
    """
    if not N8N_ENCRYPTION_KEY:
        raise RuntimeError("N8N_ENCRYPTION_KEY is not set; cannot decrypt n8n credentials")
    if not ciphertext_b64:
        return {}

    raw = base64.b64decode(ciphertext_b64)
    if raw[:8] != b"Salted__":
        # Not encrypted (legacy/imported) — return as-is.
        try:
            return json.loads(raw.decode())
        except Exception:
            return {"_raw": ciphertext_b64}
    salt = raw[8:16]
    ct = raw[16:]

    # Local import keeps the cryptography dep optional for callers
    # who only sync workflows (no credentials).
    from cryptography.hazmat.primitives import hashes, padding
    from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

    # EVP_BytesToKey(MD5) — same algorithm n8n uses. See
    # https://github.com/n8n-io/n8n/blob/master/packages/core/src/Cipher.ts
    def evp_bytes_to_key(password, salt, key_len=32, iv_len=16):
        d = b""
        d_i = b""
        while len(d) < key_len + iv_len:
            h = hashes.Hash(hashes.MD5())
            h.update(d_i + password + salt)
            d_i = h.finalize()
            d += d_i
        return d[:key_len], d[key_len:key_len + iv_len]

    password = N8N_ENCRYPTION_KEY.encode("utf-8")
    key, iv = evp_bytes_to_key(password, salt)
    cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
    decryptor = cipher.decryptor()
    pt_padded = decryptor.update(ct) + decryptor.finalize()
    unpadder = padding.PKCS7(128).unpadder()
    pt = unpadder.update(pt_padded) + unpadder.finalize()
    return json.loads(pt.decode())


def fetch_n8n_credentials():
    """Read credentials directly from n8n's Postgres database.

    The n8n PUBLIC REST API does NOT return the `data` field of
    credentials (it's writeOnly per the OpenAPI spec, and
    `?includeData=true` returns REDACTED placeholders), so we query
    the same Postgres that backs the n8n UI directly. The `data`
    column is AES-256-CBC ciphertext; the caller decrypts each
    envelope locally with `decrypt_n8n_credential_data`.

    Returns a list of envelopes shaped like n8n's REST API:
    `{id, name, type, data}` where `data` is the base64 ciphertext
    blob. n8n's REST API doesn't return `data` in list endpoints, but
    the DB does, so this function is the authoritative source.
    """
    if not N8N_PG_HOST:
        print("[sync] N8N_PG_HOST is not set; cannot query n8n Postgres for credentials")
        return []

    try:
        import psycopg2
        import psycopg2.extras
    except ImportError:
        print("[sync] psycopg2 not installed; cannot query n8n Postgres for credentials")
        return []

    try:
        conn = psycopg2.connect(
            host=N8N_PG_HOST,
            port=N8N_PG_PORT,
            user=N8N_PG_USER,
            password=N8N_PG_PASSWORD,
            dbname=N8N_PG_DATABASE,
            connect_timeout=5,
        )
    except Exception as e:
        print(f"[sync] n8n Postgres connect failed: {e}")
        return []

    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute("""
                SELECT id, name, type, data
                FROM credentials_entity
                ORDER BY name
            """)
            rows = cur.fetchall()
    except Exception as e:
        print(f"[sync] n8n Postgres query failed: {e}")
        return []
    finally:
        conn.close()

    envelopes = []
    for row in rows:
        # n8n stores the data column as a JSON-encoded ciphertext
        # string (e.g. `"U2FsdGVkX1+..."`). The outer JSON quotes are
        # part of the stored value; strip them so decrypt gets the raw
        # base64 ciphertext.
        raw = row.get("data") or ""
        if isinstance(raw, str) and len(raw) >= 2 and raw[0] == '"' and raw[-1] == '"':
            raw = raw[1:-1]
        envelopes.append({
            "id": row["id"],
            "name": row["name"],
            "type": row["type"],
            "data": raw,
            "isGlobal": False,  # column not loaded; m9m tolerates default
        })
    print(f"[sync] fetched {len(envelopes)} credentials from n8n Postgres")
    return envelopes


def push_credential_to_m9m(cred_envelope, source="manual"):
    """Decrypt the credential locally, then POST/PUT the plaintext
    envelope into m9m's credential API. m9m re-encrypts at rest under
    its own at-rest key, so the plaintext is never persisted on disk
    by the bridge itself.

    Returns ("created"|"updated"|"failed"|"skipped", status, body).
    """
    cred_id = cred_envelope.get("id")
    headers = {"Content-Type": "application/json"}

    encrypted_data = cred_envelope.get("data", "")
    if not encrypted_data:
        # n8n returns "" for credentials that have no data (e.g. a
        # newly-created form). m9m's storage layer treats empty data
        # as a normal envelope, so we can just pass it through.
        decrypted_data = {}
    else:
        try:
            decrypted_data = decrypt_n8n_credential_data(encrypted_data)
        except Exception as e:
            print(f"[sync:{source}] decrypt failed for credential {cred_envelope.get('name')} "
                  f"({cred_id}): {e}")
            return "failed", 0, str(e)

    plaintext = {
        "id": cred_id,
        "name": cred_envelope.get("name", ""),
        "type": cred_envelope.get("type", ""),
        "data": decrypted_data,
        "isGlobal": cred_envelope.get("isGlobal", False),
    }

    status, body = http_json("POST", f"{M9M_BASE_URL}/api/v1/credentials", headers, plaintext)
    if 200 <= status < 300:
        print(f"[sync:{source}] pushed credential '{plaintext['name']}' ({cred_id}) -> m9m, status {status}")
        return "created", status, body

    looks_like_duplicate = status in (409,) or (
        isinstance(body, dict) and "exist" in json.dumps(body).lower()
    )
    if looks_like_duplicate and cred_id:
        status2, body2 = http_json("PUT", f"{M9M_BASE_URL}/api/v1/credentials/{cred_id}", headers, plaintext)
        if 200 <= status2 < 300:
            print(f"[sync:{source}] updated credential '{plaintext['name']}' ({cred_id}) -> m9m, status {status2}")
            return "updated", status2, body2
        print(f"[sync:{source}] PUT fallback also failed for credential '{plaintext['name']}' "
              f"({cred_id}): status {status2}, body: {body2}")
        return "failed", status2, body2

    print(f"[sync:{source}] failed to push credential '{plaintext['name']}' ({cred_id}): "
          f"status {status}, body: {body}")
    return "failed", status, body


# ---------------------------------------------------------------------------
# Data Table sync (added 2026-10-12, see BUG-DATATABLE-2026-10-12-001)
#
# n8n's "Data Tables" feature is a user-defined key/value table store
# living in n8n's Postgres `data_table` and `data_table_row` tables.
# The schema has shifted across n8n minor releases, so this code is
# defensive: it tries the current (v1.0+) `data_table_row` table
# first, then falls back to the legacy `data_table_rows` table.
#
# On the m9m side, the matching REST surface is at
#   GET    /api/v1/data-tables
#   GET    /api/v1/data-tables/{id}/rows
#   POST   /api/v1/data-tables/{id}/rows      (bulk insert / upsert)
#   DELETE /api/v1/data-tables/{id}/rows/{rowId}
# ---------------------------------------------------------------------------

def fetch_n8n_data_tables():
    """Read all data tables + their rows directly from n8n's Postgres.

    Returns a list of `(tableId, name, [row])` tuples. The
    tableId is the opaque n8n id (e.g. `STeu50G0h27glAL8`); the
    name is for logging only. Row payloads are the JSON
    `data` blob n8n stores on each row.
    """
    if not N8N_PG_HOST:
        print("[sync] N8N_PG_HOST is not set; cannot query n8n Postgres for data tables")
        return []

    try:
        import psycopg2
        import psycopg2.extras
    except ImportError:
        print("[sync] psycopg2 not installed; cannot query n8n Postgres for data tables")
        return []

    try:
        conn = psycopg2.connect(
            host=N8N_PG_HOST,
            port=N8N_PG_PORT,
            user=N8N_PG_USER,
            password=N8N_PG_PASSWORD,
            dbname=N8N_PG_DATABASE,
            connect_timeout=5,
        )
    except Exception as e:
        print(f"[sync] n8n Postgres connect failed: {e}")
        return []

    tables = {}  # tableId -> {"name": ..., "rows": [...]}

    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            # Phase A — table metadata. Optional: skip if the
            # table doesn't exist on this n8n version.
            try:
                cur.execute('SELECT id, name FROM "data_table" ORDER BY name')
                for r in cur.fetchall():
                    tables[r["id"]] = {"name": r.get("name", ""), "rows": []}
            except Exception as e:
                # data_table missing — n8n version that doesn't
                # have data tables at all. Nothing to sync.
                conn.rollback()
                print(f"[sync] n8n has no 'data_table' table ({e}); skipping data table sync")
                return []

            # Phase B — rows. Try the modern table first
            # (data_table_row), fall back to the legacy
            # (data_table_rows) for older n8n versions.
            row_query = None
            for candidate in (
                'SELECT id, "dataTableId" AS table_id, data FROM "data_table_row"',
                'SELECT id, "dataTableId" AS table_id, data FROM "data_table_rows"',
            ):
                try:
                    cur.execute(candidate)
                    row_query = candidate
                    break
                except Exception:
                    conn.rollback()
                    continue
            if row_query is None:
                print("[sync] n8n has neither 'data_table_row' nor 'data_table_rows' table; skipping")
                return []

            for r in cur.fetchall():
                tbl = tables.setdefault(r["table_id"], {"name": "", "rows": []})
                raw = r.get("data") or "{}"
                if isinstance(raw, (dict, list)):
                    payload = raw
                else:
                    try:
                        payload = json.loads(raw)
                    except Exception:
                        payload = {}
                tbl["rows"].append({"id": str(r["id"]), "row": payload})
    except Exception as e:
        print(f"[sync] n8n Postgres query failed: {e}")
        return []
    finally:
        conn.close()

    out = [(tid, info["name"], info["rows"]) for tid, info in tables.items()]
    print(f"[sync] fetched {len(out)} data tables "
          f"({sum(len(rows) for _, _, rows in out)} rows total) from n8n Postgres")
    return out


def push_data_table_to_m9m(table_id, table_name, rows, source="manual"):
    """Bulk-insert (or overwrite) the rows of one table into m9m.

    m9m's POST is idempotent on `id` — re-running the same
    cycle just refreshes the row contents. We never DELETE on
    sync: rows the user removed in n8n stay in m9m until the
    operator clears them via the m9m UI or a manual DELETE.
    """
    if not rows:
        return "skipped", 0, None

    headers = {"Content-Type": "application/json"}
    body = {"rows": rows}
    url = f"{M9M_BASE_URL}/api/v1/data-tables/{table_id}/rows"
    status, resp_body = http_json("POST", url, headers, body)
    if 200 <= status < 300:
        print(f"[sync:{source}] pushed data table '{table_name}' ({table_id}) "
              f"({len(rows)} rows) -> m9m, status {status}")
        return "created", status, resp_body
    print(f"[sync:{source}] failed to push data table '{table_name}' ({table_id}): "
          f"status {status}, body: {resp_body}")
    return "failed", status, resp_body


# ---------------------------------------------------------------------------
# Async cycle (one poll = one manual = same code path)
# ---------------------------------------------------------------------------

async def run_cycle(source: str = "poll") -> dict:
    """Run one sync cycle. `source` is "poll" (background) or "manual" (POST /sync)."""
    global _is_cycle_running, _last_cycle_at, _last_cycle_source, _last_cycle_count

    _is_cycle_running = True
    started_monotonic = time.monotonic()
    started_iso = _iso_now()
    results = []

    try:
        # Phase 0 — data tables. Runs FIRST so a workflow
        # that uses `n8n-nodes-base.dataTable` finds its
        # table when the engine starts executing. The
        # table is just KV storage, so missing rows only
        # manifest as empty `get` results — the workflow
        # still runs. We push it first anyway so the
        # order of operations in /status is
        # data-tables → credentials → workflows.
        try:
            data_tables = await asyncio.to_thread(fetch_n8n_data_tables)
        except Exception as e:
            print(f"[sync:{source}] data table fetch crashed: {e}")
            data_tables = []
        for table_id, table_name, rows in data_tables:
            try:
                verdict, status, _body = await asyncio.to_thread(
                    push_data_table_to_m9m, table_id, table_name, rows, source
                )
                results.append({
                    "kind": "dataTable",
                    "id": table_id,
                    "name": table_name,
                    "rowCount": len(rows),
                    "status": verdict,
                    "http_status": status,
                })
                if verdict == "failed":
                    print(f"[sync:{source}] data table '{table_name}' ({table_id}) push failed: "
                          f"status {status}")
            except Exception as e:
                print(f"[sync:{source}] data table '{table_name}' crashed: {e}")
                results.append({
                    "kind": "dataTable",
                    "id": table_id,
                    "name": table_name,
                    "status": "failed",
                    "detail": str(e),
                })

        # Phase 1 — credentials. Workflows that reference a credential
        # need that credential to exist in m9m's store BEFORE the
        # webhook handler resolves it at request time. Skipping this
        # phase produces the "anon upstream echo" parity gap we saw
        # on positive/3 (webhook_callrest) — m9m accepted the request
        # without auth because it couldn't find the credential.
        try:
            credentials = await asyncio.to_thread(fetch_n8n_credentials)
        except Exception as e:
            print(f"[sync:{source}] credential fetch crashed: {e}")
            credentials = []
        for cred in credentials:
            try:
                verdict, status, _body = await asyncio.to_thread(push_credential_to_m9m, cred, source)
                if verdict == "failed":
                    print(f"[sync:{source}] credential '{cred.get('name')}' ({cred.get('id')}) push failed: "
                          f"status {status}")
            except Exception as e:
                print(f"[sync:{source}] credential crashed: {e}")

        # Phase 2 — workflows.
        try:
            workflows = await asyncio.to_thread(fetch_n8n_workflows)
        except Exception as e:
            print(f"[sync:{source}] fetch crashed: {e}")
            workflows = []

        for wf in workflows:
            try:
                if ONLY_ACTIVE and not wf.get("active", False):
                    results.append({
                        "id": wf.get("id"),
                        "name": wf.get("name"),
                        "status": "filtered",
                    })
                    continue

                wf_id = str(wf.get("id"))
                body_bytes = json.dumps(wf, sort_keys=True).encode()
                digest = hashlib.sha256(body_bytes).hexdigest()

                if _last_hash.get(wf_id) == digest:
                    results.append({
                        "id": wf.get("id"),
                        "name": wf.get("name"),
                        "status": "skipped",
                        "detail": "unchanged since last cycle",
                    })
                    continue  # unchanged since last poll

                sanitize_type_versions(wf)
                verdict, status, _body = await asyncio.to_thread(push_to_m9m, wf, source)
                results.append({
                    "id": wf.get("id"),
                    "name": wf.get("name"),
                    "status": verdict,
                    "http_status": status,
                })
                if verdict != "failed":
                    _last_hash[wf_id] = digest
            except Exception as e:
                # Never let one workflow's failure abort the whole cycle.
                print(f"[sync:{source}] workflow crashed: {e}")
                results.append({
                    "id": wf.get("id"),
                    "name": wf.get("name"),
                    "status": "failed",
                    "detail": str(e),
                })
    finally:
        duration_ms = int((time.monotonic() - started_monotonic) * 1000)
        finished_iso = _iso_now()
        _is_cycle_running = False
        _last_cycle_at = finished_iso
        _last_cycle_source = source
        _last_cycle_count = len(results)

    summary = {
        "triggered_by": source,
        "started_at": started_iso,
        "finished_at": finished_iso,
        "duration_ms": duration_ms,
        "results": results,
    }
    print(f"[sync:{source}] cycle done: {len(results)} items processed in {duration_ms}ms "
          f"(data_tables={sum(1 for r in results if r.get('kind') == 'dataTable')} "
          f"workflows={sum(1 for r in results if r.get('kind') != 'dataTable' and 'id' in r)} "
          f"created={sum(1 for r in results if r['status'] == 'created')} "
          f"updated={sum(1 for r in results if r['status'] == 'updated')} "
          f"skipped={sum(1 for r in results if r['status'] == 'skipped')} "
          f"filtered={sum(1 for r in results if r['status'] == 'filtered')} "
          f"failed={sum(1 for r in results if r['status'] == 'failed')})")
    return summary


def _iso_now() -> str:
    # Fixed-offset local time in ISO-8601-ish format. Avoids the extra datetime import.
    return time.strftime("%Y-%m-%dT%H:%M:%S%z", time.localtime())


# ---------------------------------------------------------------------------
# Background polling
# ---------------------------------------------------------------------------

async def _poll_loop():
    """Poll n8n every POLL_INTERVAL_SECONDS. Sleeps AFTER each cycle so a long
    cycle doesn't compound into zero-delay loops."""
    global _poll_task
    while True:
        try:
            if _cycle_lock.locked():
                # Should not happen with sequential awaits, but guard anyway.
                print("[sync:poll] cycle skipped (already running)")
            else:
                async with _cycle_lock:
                    await run_cycle("poll")
        except asyncio.CancelledError:
            raise
        except Exception as e:
            print(f"[sync:poll] cycle crashed: {e}")
        try:
            await asyncio.sleep(POLL_INTERVAL_SECONDS)
        except asyncio.CancelledError:
            raise


# ---------------------------------------------------------------------------
# FastAPI app + lifespan
# ---------------------------------------------------------------------------

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Start the background poll loop on startup, cancel it on shutdown."""
    global _poll_task
    if not N8N_API_KEY:
        print("[sync] WARNING: N8N_API_KEY is not set — n8n's public API will reject requests.")
    print(f"[sync] polling {N8N_BASE_URL} every {POLL_INTERVAL_SECONDS}s -> pushing to {M9M_BASE_URL}")
    _poll_task = asyncio.create_task(_poll_loop())
    try:
        yield
    finally:
        if _poll_task is not None:
            _poll_task.cancel()
            try:
                await _poll_task
            except asyncio.CancelledError:
                pass
            _poll_task = None


app = FastAPI(
    title="m9m n8m Sync Bridge",
    description=(
        "Polls n8n's REST API and pushes active workflows into m9m via its REST API. "
        "A background task polls every POLL_INTERVAL_SECONDS; POST /sync triggers "
        "an immediate cycle; /health is the liveness probe; /status shows the last cycle."
    ),
    version="2.1.0",
    lifespan=lifespan,
)


@app.get("/", response_class=PlainTextResponse)
def index():
    return (
        "m9m <-> n8n sync bridge\n"
        f"  n8n:        {N8N_BASE_URL}\n"
        f"  m9m:        {M9M_BASE_URL}\n"
        f"  poll:       {POLL_INTERVAL_SECONDS}s (background asyncio task)\n"
        f"  only_active: {ONLY_ACTIVE}\n"
        f"  sync port:  {SYNC_PORT}\n"
        "Endpoints:\n"
        "  GET  /        -> this message\n"
        "  GET  /health  -> liveness probe\n"
        "  GET  /status  -> last-cycle summary\n"
        "  POST /sync    -> run one sync cycle now\n"
        "\n"
        "Sync phases (per cycle, in order):\n"
        "  0. data tables  (n8n Postgres -> m9m /api/v1/data-tables/{id}/rows)\n"
        "  1. credentials  (n8n Postgres -> m9m /api/v1/credentials)\n"
        "  2. workflows    (n8n REST    -> m9m /api/v1/workflows)\n"
    )


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/status")
def status():
    return {
        "last_cycle_at": _last_cycle_at,
        "last_cycle_source": _last_cycle_source,
        "last_cycle_count": _last_cycle_count,
        "is_running": _is_cycle_running,
    }


@app.get("/debug/credentials")
def debug_credentials():
    """Return the full credential envelopes (id, name, type, decrypted data)
    read directly from n8n's Postgres. Useful for verifying what the
    bridge actually pushes; the public API strips `data` for safety.

    Reads the in-process N8N_ENCRYPTION_KEY; on a fresh container the
    N8N_ENCRYPTION_KEY env var must already be set or this returns 503.
    """
    if not N8N_ENCRYPTION_KEY:
        return JSONResponse(status_code=503, content={"error": "N8N_ENCRYPTION_KEY not set"})
    creds = fetch_n8n_credentials()
    out = []
    for c in creds:
        envelope = {
            "id": c.get("id"),
            "name": c.get("name"),
            "type": c.get("type"),
        }
        try:
            envelope["data"] = decrypt_n8n_credential_data(c.get("data", ""))
        except Exception as e:
            envelope["data_error"] = str(e)
        out.append(envelope)
    return {"credentials": out}


@app.post("/sync")
async def sync_endpoint(force: bool = False):
    """Run a single cycle synchronously and return the summary.

    `?force=true` (or `?force=1`) wipes the in-memory content-hash
    cache first, so every workflow is re-pushed into m9m even when
    n8n reports no content change. Use this after manually editing
    m9m to drift it out of sync, or to audit the wire format against
    a fresh fetch.
    """
    if force:
        cleared = len(_last_hash)
        _last_hash.clear()
        print(f"[sync:manual] force=True -> cleared {cleared} cached hashes")
    async with _cycle_lock:
        summary = await run_cycle("manual")
    if any(r.get("status") == "failed" for r in summary["results"]):
        return JSONResponse(status_code=502, content=summary)
    return summary


@app.post("/sync/data-tables")
async def sync_data_tables_endpoint():
    """Run ONLY the data-tables phase. Useful for one-shot
    backfills without re-pushing every credential + workflow.
    """
    async with _cycle_lock:
        try:
            data_tables = await asyncio.to_thread(fetch_n8n_data_tables)
        except Exception as e:
            return JSONResponse(status_code=502, content={"error": str(e)})
        results = []
        for table_id, table_name, rows in data_tables:
            try:
                verdict, status, body = await asyncio.to_thread(
                    push_data_table_to_m9m, table_id, table_name, rows, "manual"
                )
                results.append({
                    "kind": "dataTable",
                    "id": table_id,
                    "name": table_name,
                    "rowCount": len(rows),
                    "status": verdict,
                    "http_status": status,
                })
            except Exception as e:
                results.append({
                    "kind": "dataTable",
                    "id": table_id,
                    "name": table_name,
                    "status": "failed",
                    "detail": str(e),
                })
    if any(r.get("status") == "failed" for r in results):
        return JSONResponse(status_code=502, content={"results": results})
    return {"results": results}


if __name__ == "__main__":
    # Convenience entrypoint for local dev: `python sync.py` boots uvicorn.
    # In production supervisord invokes `uvicorn sync:app` directly.
    import uvicorn

    uvicorn.run("sync:app", host="0.0.0.0", port=int(os.environ.get("SYNC_PORT", "8001")), log_level="info")
