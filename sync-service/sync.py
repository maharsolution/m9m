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
    """GET /api/v1/credentials from n8n. Returns list of safe envelopes
    (with `data` still encrypted — caller must call decrypt on each).
    """
    status, body = http_json(
        "GET",
        f"{N8N_BASE_URL}/api/v1/credentials",
        headers={"X-N8N-API-KEY": N8N_API_KEY, "Accept": "application/json"},
    )
    if status != 200:
        print(f"[sync] n8n credentials API error {status}: {body}")
        return []
    if isinstance(body, dict):
        return body.get("data", [])
    return body or []


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
    print(f"[sync:{source}] cycle done: {len(results)} workflows processed in {duration_ms}ms "
          f"(created={sum(1 for r in results if r['status'] == 'created')} "
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


@app.post("/sync")
async def sync_endpoint():
    """Run a single cycle synchronously and return the summary."""
    async with _cycle_lock:
        summary = await run_cycle("manual")
    if any(r.get("status") == "failed" for r in summary["results"]):
        return JSONResponse(status_code=502, content=summary)
    return summary


if __name__ == "__main__":
    # Convenience entrypoint for local dev: `python sync.py` boots uvicorn.
    # In production supervisord invokes `uvicorn sync:app` directly.
    import uvicorn

    uvicorn.run("sync:app", host="0.0.0.0", port=int(os.environ.get("SYNC_PORT", "8001")), log_level="info")
