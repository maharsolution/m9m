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

It also doubles as a local-file importer that mirrors the REAL n8n CLI
import semantics (verified against the n8n checkout in ./n8n):

    python sync.py --import workflows.json [--dry-run] [--active-state=from-json]

Without `--import` the module boots uvicorn exactly as before.

The importer pushes the whole batch to m9m's n8n import endpoint
(`POST /api/v1/workflows/import`), which mirrors n8n's batch import:
every entry is validated on its own, one invalid entry never aborts
the rest, credentials are resolved by name + type server-side and the
imported workflows are inactive unless `activeState=fromJson` is asked
for. m9m builds that predate that endpoint fall back to
`POST /api/v1/workflows` per workflow.
"""
import argparse
import asyncio
import base64
import copy
import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.responses import JSONResponse, PlainTextResponse

N8N_BASE_URL = os.environ.get("N8N_BASE_URL", "http://n8n-designer-frontend:5678")
N8N_API_KEY = os.environ.get("N8N_API_KEY", "")
M9M_BASE_URL = os.environ.get("M9M_BASE_URL", "http://m9m-backend:8080")
# The m9m API is closed by default: every /api/v1 path needs a bearer
# token or an X-API-Key header. The bridge authenticates with the
# automation key from M9M_API_KEY (never with the n8n key).
M9M_API_KEY = os.environ.get("M9M_API_KEY", "")
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


def m9m_headers(extra=None):
    """Headers for an m9m API call.

    The m9m API rejects anonymous /api/v1 calls with 401, so the
    automation key travels in the X-API-Key header. The value is never
    printed.
    """
    headers = dict(extra or {})
    if M9M_API_KEY:
        headers["X-API-Key"] = M9M_API_KEY
    return headers


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
    m9m's parser only accepts an integer. See docstring for the caveat.

    Superseded by `normalize_n8n_workflow()`, which also coerces
    string typeVersions and reports non-numeric values; kept because
    it is part of the module's public surface."""
    for node in workflow.get("nodes", []):
        tv = node.get("typeVersion")
        if isinstance(tv, float):
            node["typeVersion"] = int(tv)
    return workflow


# ---------------------------------------------------------------------------
# n8n import/export parity
#
# Canonical reference: docs/N8N_WORKFLOW_PARITY_REFERENCE.md — distilled
# from the real n8n sources in this repo (n8n/ upstream checkout,
# commit 04fb7076, plus n8n/graphify-out/graph.json). The rules below
# mirror it.
#
#   n8n/packages/cli/src/commands/import/workflow.ts
#     - `assertHasWorkflowsToImport()`: every workflow needs BOTH the
#       `nodes` and `connections` properties, otherwise n8n rejects the
#       file with "File does not seem to contain valid workflows.".
#       In `--separate` mode an invalid file is skipped with a warning
#       and the rest of the batch still imports.
#     - the input file may hold a single workflow object OR an array.
#     - `--activeState=false` (the default) deactivates imported
#       workflows; `--activeState=fromJson` honours the JSON `active`
#       field.
#
#   n8n/packages/cli/src/services/import.service.ts
#     - `toNewCredentialFormat()` rewrites OLD credentials
#       (`{"httpBasicAuth": "My Cred"}`) into the NEW shape
#       (`{"httpBasicAuth": {"id": ..., "name": "My Cred"}}`); an id that
#       cannot be resolved stays `null` and is never fatal for the batch.
#     - every node without an id gets a `uuid()`.
#     - every import assigns a NEW `workflow.versionId = uuid()`.
#     - `tags` are objects `{id, name}`, never plain strings.
#
#   n8n/packages/cli/src/workflow-helpers.ts
#     - `replaceInvalidCredentials()` resolves a credential id by NAME +
#       TYPE; exactly one match wins, otherwise `{id: null, name}` is
#       kept. Disabled nodes and `__aiGatewayManaged` entries are
#       skipped.
#
#   n8n/packages/cli/src/commands/export/workflow.ts
#     - `export:workflow` always writes an ARRAY of workflow objects,
#       even for a single `--id`.
#
#   n8n/packages/frontend/editor-ui/.../useCanvasOperations.ts
#     - the editor import drops nodes without a `type`, normalises
#       `position`, names unnamed nodes uniquely and sanitizes
#       connections that reference unknown node names.
#
# The m9m side mirrors the same contract in Go:
#   internal/compatibility/workflow_importer.go
#     - `ValidateN8nWorkflow()` requires BOTH `nodes` and
#       `connections`; `convertTags()` accepts both tag shapes and
#       stores names; `convertCredentials()` accepts both credential
#       shapes and resolves an id by name + type; `convertPosition()`
#       falls back to [0, 0]; a fresh versionId is assigned per
#       import and `active` is forced false unless the caller asks
#       for `activeState: fromJson`.
#   internal/api/workflow_import_export.go
#     - `POST /api/v1/workflows/import` takes a JSON object, a JSON
#       array or an envelope; one bad entry never aborts the batch.
# ---------------------------------------------------------------------------

ACTIVE_STATE_FALSE = "false"
ACTIVE_STATE_FROM_JSON = "from-json"

# Node types m9m registers an executor for. Source of truth:
# cmd/m9m/commands/exec.go -> RegisterAllNodes(), which is what
# `m9m serve` calls (cmd/m9m/commands/serve.go).
KNOWN_NODE_TYPES = frozenset({
    # transform
    "n8n-nodes-base.set",
    "n8n-nodes-base.filter",
    "n8n-nodes-base.code",
    "n8n-nodes-base.merge",
    "n8n-nodes-base.splitInBatches",
    "n8n-nodes-base.itemLists",
    "n8n-nodes-base.switch",
    "n8n-nodes-base.function",
    "n8n-nodes-base.json",
    "n8n-nodes-base.xml",
    "n8n-nodes-base.if",
    "n8n-nodes-base.loop",
    # http
    "n8n-nodes-base.httpRequest",
    # triggers
    "n8n-nodes-base.webhook",
    "n8n-nodes-base.cron",
    "n8n-nodes-base.errorTrigger",
    "n8n-nodes-base.respondToWebhook",
    "n8n-nodes-base.executeWorkflowTrigger",
    "n8n-nodes-base.kafkaTrigger",
    # code execution
    "n8n-nodes-base.executeCommand",
    "n8n-nodes-base.pythonCode",
    # files
    "n8n-nodes-base.readBinaryFile",
    "n8n-nodes-base.writeBinaryFile",
    # databases
    "n8n-nodes-base.postgres",
    "n8n-nodes-base.mySql",
    "n8n-nodes-base.sqlite",
    "n8n-nodes-base.elasticsearch",
    "n8n-nodes-base.redis",
    "n8n-nodes-base.mongoDb",
    # messaging
    "n8n-nodes-base.slack",
    "n8n-nodes-base.discord",
    "n8n-nodes-base.twilio",
    "n8n-nodes-base.microsoftTeams",
    "n8n-nodes-base.kafka",
    # ai
    "@n8n/n8n-nodes-langchain.openAi",
    "@n8n/n8n-nodes-langchain.anthropic",
    # core
    "n8n-nodes-base.start",
    "n8n-nodes-base.noOp",
    "n8n-nodes-base.wait",
    # cloud
    "n8n-nodes-base.awsLambda",
    "n8n-nodes-base.s3",
    "n8n-nodes-base.azureBlobStorage",
    "n8n-nodes-base.gcpCloudStorage",
    # vcs
    "n8n-nodes-base.github",
    "n8n-nodes-base.gitlab",
    # productivity
    "n8n-nodes-base.googleSheets",
    "n8n-nodes-base.notion",
    "n8n-nodes-base.stripe",
    # email
    "n8n-nodes-base.sendEmail",
    "n8n-nodes-base.sendGrid",
    # data + sub-workflows
    "n8n-nodes-base.dataTable",
    "n8n-nodes-base.executeWorkflow",
})

# Node types m9m intentionally SKIPS at runtime
# (internal/engine/engine.go -> isDecorativeNodeType / isTriggerOnlyNodeType).
# They carry no behaviour, so a workflow containing them is still valid.
TOLERATED_NODE_TYPES = frozenset({
    "n8n-nodes-base.manualTrigger",
    "n8n-nodes-base.stickyNote",
    "n8n-nodes-base.note",
    "@n8n/n8n-nodes-langchain.note",
})

# Display names used when the bridge has to invent a name for an unnamed
# node (n8n's editor uses `nodeType.displayName ?? node.type` —
# useCanvasOperations.ts L3303-3308).
NODE_DISPLAY_NAMES = {
    "n8n-nodes-base.set": "Edit Fields (Set)",
    "n8n-nodes-base.filter": "Filter",
    "n8n-nodes-base.code": "Code",
    "n8n-nodes-base.merge": "Merge",
    "n8n-nodes-base.splitInBatches": "Split In Batches",
    "n8n-nodes-base.itemLists": "Item Lists",
    "n8n-nodes-base.switch": "Switch",
    "n8n-nodes-base.function": "Function",
    "n8n-nodes-base.json": "Convert to JSON",
    "n8n-nodes-base.xml": "XML",
    "n8n-nodes-base.if": "If",
    "n8n-nodes-base.loop": "Loop Over Items",
    "n8n-nodes-base.httpRequest": "HTTP Request",
    "n8n-nodes-base.webhook": "Webhook",
    "n8n-nodes-base.cron": "Schedule Trigger",
    "n8n-nodes-base.errorTrigger": "Error Trigger",
    "n8n-nodes-base.respondToWebhook": "Respond to Webhook",
    "n8n-nodes-base.executeWorkflowTrigger": "Execute Sub-workflow Trigger",
    "n8n-nodes-base.kafkaTrigger": "Kafka Trigger",
    "n8n-nodes-base.executeCommand": "Execute Command",
    "n8n-nodes-base.pythonCode": "Python",
    "n8n-nodes-base.readBinaryFile": "Read Binary File",
    "n8n-nodes-base.writeBinaryFile": "Write Binary File",
    "n8n-nodes-base.postgres": "Postgres",
    "n8n-nodes-base.mySql": "MySQL",
    "n8n-nodes-base.sqlite": "SQLite",
    "n8n-nodes-base.elasticsearch": "Elasticsearch",
    "n8n-nodes-base.redis": "Redis",
    "n8n-nodes-base.mongoDb": "MongoDB",
    "n8n-nodes-base.slack": "Slack",
    "n8n-nodes-base.discord": "Discord",
    "n8n-nodes-base.twilio": "Twilio",
    "n8n-nodes-base.microsoftTeams": "Microsoft Teams",
    "n8n-nodes-base.kafka": "Kafka",
    "@n8n/n8n-nodes-langchain.openAi": "OpenAI Chat Model",
    "@n8n/n8n-nodes-langchain.anthropic": "Anthropic Chat Model",
    "n8n-nodes-base.start": "Start",
    "n8n-nodes-base.noOp": "No Operation, do nothing",
    "n8n-nodes-base.wait": "Wait",
    "n8n-nodes-base.awsLambda": "AWS Lambda",
    "n8n-nodes-base.s3": "S3",
    "n8n-nodes-base.azureBlobStorage": "Azure Blob Storage",
    "n8n-nodes-base.gcpCloudStorage": "Google Cloud Storage",
    "n8n-nodes-base.github": "GitHub",
    "n8n-nodes-base.gitlab": "GitLab",
    "n8n-nodes-base.googleSheets": "Google Sheets",
    "n8n-nodes-base.notion": "Notion",
    "n8n-nodes-base.stripe": "Stripe",
    "n8n-nodes-base.sendEmail": "Send Email",
    "n8n-nodes-base.sendGrid": "SendGrid",
    "n8n-nodes-base.dataTable": "Data Table",
    "n8n-nodes-base.executeWorkflow": "Execute Sub-workflow",
    "n8n-nodes-base.manualTrigger": "Manual Trigger",
    "n8n-nodes-base.stickyNote": "Sticky Note",
}


class InvalidWorkflowError(ValueError):
    """A workflow entry cannot be imported.

    Mirrors n8n's `UserError('File does not seem to contain valid
    workflows.')`. In the bridge the failure is per-entry: the batch
    keeps going, matching n8n's `--separate` behaviour.
    """


def _uuid4() -> str:
    return str(uuid.uuid4())


def _is_from_json(active_state) -> bool:
    """True when the caller asked to honour the JSON `active` field."""
    token = str(active_state or "").strip().lower().replace("_", "-")
    return token in ("from-json", "fromjson")


def normalize_tags(tags):
    """Normalize n8n `tags` into the canonical object form `[{id, name}]`.

    n8n stores and exports tags as objects. Older payloads (and
    hand-written fixtures) use plain strings; those are accepted and
    given an id so the result still matches the n8n shape.
    """
    if not isinstance(tags, list):
        return []
    out = []
    for tag in tags:
        if isinstance(tag, str):
            name, tag_id = tag, None
        elif isinstance(tag, dict):
            name = tag.get("name")
            tag_id = tag.get("id")
        else:
            continue
        if name is None or str(name).strip() == "":
            continue
        out.append({
            "id": str(tag_id) if tag_id not in (None, "") else _uuid4(),
            "name": str(name),
        })
    return out


def normalize_credentials(credentials, node_disabled=False):
    """Normalize node credentials to n8n's NEW format.

        OLD: {"httpBasicAuth": "My Cred"}
        NEW: {"httpBasicAuth": {"id": "abc123" | null, "name": "My Cred"}}

    n8n resolves OLD entries against its own credential store by
    matching credential NAME + TYPE, and exactly one match wins —
    otherwise `{id: null, name}` is kept (workflow-helpers.ts L274-335,
    `replaceInvalidCredentials`). The bridge has no credential store
    to resolve against, so an unresolved id stays `None` — the name is
    what matters downstream, and n8n treats an unresolvable credential
    as non-fatal for the batch.

    Two n8n skip rules are mirrored here:

      * credentials on a DISABLED node are skipped entirely — the
        payload is passed through untouched (workflow-helpers.ts
        L286-288);
      * `__aiGatewayManaged` entries are skipped — there is no record
        behind them (workflow-helpers.ts L301-302).
    """
    if not isinstance(credentials, dict):
        return {}
    if node_disabled:
        # Disabled nodes are skipped by n8n's credential repair.
        return copy.deepcopy(credentials)
    out = {}
    for cred_type, value in credentials.items():
        if value is None:
            continue
        if isinstance(value, dict) and value.get("__aiGatewayManaged"):
            # AI Gateway managed credentials have no DB record; n8n
            # leaves them exactly as they are.
            out[cred_type] = copy.deepcopy(value)
            continue
        if isinstance(value, str):
            out[cred_type] = {"id": None, "name": value}
        elif isinstance(value, dict):
            raw_id = value.get("id")
            raw_name = value.get("name")
            out[cred_type] = {
                "id": str(raw_id) if raw_id not in (None, "") else None,
                "name": "" if raw_name is None else str(raw_name),
            }
        # Any third shape is dropped — n8n has only the two above.
    return out


def normalize_node_position(position):
    """Mirror n8n's `ensureNodePosition()` (editor-ui workflowUtils.ts L49-58).

    Returns `[x, y]` as integers (m9m's model types `position` as
    `[]int`); a missing or malformed position becomes `[0, 0]`.
    """
    if isinstance(position, (list, tuple)) and len(position) >= 2:
        try:
            x = float(position[0])
            y = float(position[1])
        except (TypeError, ValueError):
            return [0, 0], False
        if x != x or y != y:  # NaN
            return [0, 0], False
        return [int(x), int(y)], True
    return [0, 0], False


def _is_valid_connection_entry(entry):
    """Mirror n8n's `isValidConnectionEntry()` (workflowUtils.ts L60-71)."""
    if not isinstance(entry, dict):
        return False
    if not isinstance(entry.get("node"), str):
        return False
    if not isinstance(entry.get("type"), str):
        return False
    index = entry.get("index")
    if isinstance(index, bool) or not isinstance(index, (int, float)):
        return False
    return True


def sanitize_connections(connections, valid_node_names):
    """Mirror n8n's `sanitizeConnections()` (workflowUtils.ts L80-115).

    Drops edges whose source or target node name is unknown, plus
    malformed entries. Returns `(sanitized, dropped_count)`.
    """
    if not isinstance(connections, dict):
        return {}, 0
    names = set(valid_node_names)
    sanitized = {}
    dropped = 0
    for source_name, groups in connections.items():
        if source_name not in names:
            dropped += 1
            continue
        if not isinstance(groups, dict):
            dropped += 1
            continue
        clean_groups = {}
        for conn_type, buckets in groups.items():
            if not isinstance(buckets, list):
                dropped += 1
                continue
            clean_buckets = []
            for bucket in buckets:
                if not isinstance(bucket, list):
                    clean_buckets.append(None)
                    continue
                kept = []
                for entry in bucket:
                    if not _is_valid_connection_entry(entry):
                        dropped += 1
                        continue
                    if entry["node"] not in names:
                        dropped += 1
                        continue
                    kept.append(entry)
                clean_buckets.append(kept)
            clean_groups[conn_type] = clean_buckets
        sanitized[source_name] = clean_groups
    return sanitized, dropped


def unique_node_name(base_name, taken_names):
    """Mirror n8n's `uniqueNodeName()` (useUniqueNodeName.ts L27-114).

    A free name is used as-is; otherwise a numeric suffix is appended
    ("Webhook", "Webhook1", "Webhook2", ...).
    """
    if base_name not in taken_names:
        return base_name
    index = 1
    while f"{base_name}{index}" in taken_names:
        index += 1
    return f"{base_name}{index}"


def _coerce_type_version(value, node_label, warnings):
    """Coerce n8n's numeric `typeVersion` into the int m9m's model needs.

    n8n writes minor versions as floats (2.1); m9m's Go model types
    `typeVersion` as int and a float fails JSON unmarshalling. The
    conversion is lossy — see the module docstring.
    """
    if isinstance(value, bool):
        warnings.append(f"node '{node_label}': non-numeric typeVersion {value!r}; defaulted to 1")
        return 1
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        if value != value or value in (float("inf"), float("-inf")):
            warnings.append(f"node '{node_label}': non-numeric typeVersion {value!r}; defaulted to 1")
            return 1
        return int(value)
    if isinstance(value, str):
        try:
            return int(float(value.strip()))
        except (TypeError, ValueError, OverflowError):
            warnings.append(f"node '{node_label}': non-numeric typeVersion {value!r}; defaulted to 1")
            return 1
    warnings.append(f"node '{node_label}': non-numeric typeVersion {value!r}; defaulted to 1")
    return 1


def normalize_n8n_workflow(workflow, active_state=ACTIVE_STATE_FALSE, version_id=None, warnings=None):
    """Return an n8n-shaped, import-ready copy of `workflow`.

    Mirrors the n8n CLI import path:

      * every entry needs BOTH `nodes` and `connections` — otherwise
        `InvalidWorkflowError` is raised (n8n aborts the whole file;
        the bridge reports the entry and continues the batch);
      * workflow `id` stays a string, a missing one is generated;
      * nodes without an `id` get a generated uuid4;
      * credentials are rewritten to the NEW `{id, name}` shape;
      * `tags` become `[{id, name}]` objects;
      * `typeVersion` is coerced to int;
      * `active` is forced to False unless `active_state` asks for
        `from-json`;
      * a fresh `versionId` is assigned on import.

    `warnings` is an optional list the caller can pass in to collect
    non-fatal notes (it is created when omitted).
    """
    if not isinstance(workflow, dict):
        raise InvalidWorkflowError("workflow entry is not a JSON object")
    if "nodes" not in workflow:
        raise InvalidWorkflowError("missing 'nodes' property")
    if "connections" not in workflow:
        raise InvalidWorkflowError("missing 'connections' property")

    log = warnings if warnings is not None else []
    wf = copy.deepcopy(workflow)

    # Workflow id: n8n ids are strings (nanoid/uuid). Missing -> new one.
    wf_id = wf.get("id")
    if wf_id in (None, ""):
        wf["id"] = _uuid4()
    elif not isinstance(wf_id, str):
        wf["id"] = str(wf_id)

    # Every import assigns a NEW versionId (import.service.ts).
    wf["versionId"] = str(version_id) if version_id else _uuid4()

    # `active`: forced false unless the caller honours the JSON flag.
    if _is_from_json(active_state):
        wf["active"] = bool(wf.get("active", False))
    else:
        wf["active"] = False

    # `tags`: n8n shape is an array of {id, name} objects.
    wf["tags"] = normalize_tags(wf.get("tags"))

    # Nodes.
    #
    # Editor-side rules (n8n/packages/frontend/editor-ui/
    # useCanvasOperations.ts `importWorkflowData()` L3266-3313) are
    # applied here as well, because they are what keeps a hand-written
    # or third-party payload loadable:
    #   - nodes without a `type` are dropped (count reported);
    #   - `position` is normalised (missing/invalid -> [0, 0]);
    #   - unnamed nodes get a unique name;
    #   - connections pointing at unknown node names are dropped.
    # Every one of them is a WARNING, never a hard failure.
    nodes = wf.get("nodes")
    if not isinstance(nodes, list):
        nodes = []

    typed_nodes = []
    dropped_typeless = 0
    for node in nodes:
        if not isinstance(node, dict):
            dropped_typeless += 1
            continue
        if not node.get("type"):
            dropped_typeless += 1
            continue
        typed_nodes.append(node)
    if dropped_typeless:
        log.append(
            f"dropped {dropped_typeless} node(s) without a 'type' "
            "(n8n's editor import filters them out)"
        )

    normalized_nodes = []
    for index, node in enumerate(typed_nodes):
        n = copy.deepcopy(node)
        node_id = n.get("id")
        if node_id in (None, ""):
            n["id"] = _uuid4()
        elif not isinstance(node_id, str):
            n["id"] = str(node_id)

        # Position (workflowUtils.ts L49-58).
        position, position_ok = normalize_node_position(n.get("position"))
        n["position"] = position
        if not position_ok:
            log.append(
                f"node '{n.get('name') or f'#{index}'}': position missing or malformed; set to [0, 0]"
            )

        label = n.get("name") or f"#{index}"
        if "typeVersion" in n:
            n["typeVersion"] = _coerce_type_version(n.get("typeVersion"), label, log)
        else:
            n["typeVersion"] = 1

        if "credentials" in n:
            node_credentials = n.get("credentials")
            if isinstance(node_credentials, dict) and node_credentials:
                if n.get("disabled"):
                    log.append(
                        f"node '{label}' is disabled; its credentials were skipped "
                        "(n8n does not repair credentials on disabled nodes)"
                    )
                else:
                    for cred_type, cred in node_credentials.items():
                        if isinstance(cred, dict) and cred.get("__aiGatewayManaged"):
                            log.append(
                                f"node '{label}': credential '{cred_type}' is "
                                "__aiGatewayManaged and was skipped (no record behind it)"
                            )
            n["credentials"] = normalize_credentials(
                node_credentials, node_disabled=bool(n.get("disabled"))
            )
        normalized_nodes.append(n)
    wf["nodes"] = normalized_nodes

    # Connections: drop edges whose source or target node is unknown
    # (n8n's editor import, workflowUtils.ts L80-115). n8n sanitizes
    # connections BEFORE it names unnamed nodes, so the valid-name set
    # is the one carried by the payload.
    node_names = [n.get("name") for n in normalized_nodes]
    sanitized_connections, dropped_connections = sanitize_connections(
        wf.get("connections"), node_names
    )
    if dropped_connections:
        log.append(
            f"dropped {dropped_connections} connection(s) referencing unknown node names"
        )
    wf["connections"] = sanitized_connections

    # Unique names for unnamed nodes (useCanvasOperations.ts L3299-3313).
    taken_names = set()
    for n in normalized_nodes:
        node_name = n.get("name")
        if not node_name:
            display_name = NODE_DISPLAY_NAMES.get(n.get("type"), n.get("type"))
            node_name = unique_node_name(display_name, taken_names)
            log.append(
                f"node '{node_name}' had no name; assigned '{node_name}' "
                "(n8n's editor import names unnamed nodes)"
            )
            n["name"] = node_name
        taken_names.add(n["name"])

    return wf


def validate_n8n_workflow(workflow, known_types=None):
    """Validate a normalized workflow against m9m's requirements.

    Returns a verdict dict:

        {"valid": bool, "errors": [...], "warnings": [...],
         "unsupportedTypes": [...]}

    Structural problems (no nodes, missing name/type) are ERRORS —
    m9m's REST API answers HTTP 400 for them. Node types m9m does not
    implement are WARNINGS: n8n imports them happily, so the workflow
    is still imported and the gap is surfaced instead of silently
    dropping the whole file.

    NOTE: `normalize_n8n_workflow()` already drops nodes without a
    `type`, names unnamed nodes and sanitizes connections, so a
    normalized workflow rarely trips these checks — they stay here as
    the contract for callers that validate a raw payload.
    """
    errors = []
    warnings = []

    if not isinstance(workflow, dict):
        return {
            "valid": False,
            "errors": ["workflow entry is not a JSON object"],
            "warnings": warnings,
            "unsupportedTypes": [],
        }

    name = workflow.get("name")
    if not name or not str(name).strip():
        errors.append("workflow name is missing or empty")
    elif len(str(name)) > 255:
        errors.append("workflow name is longer than 255 characters")

    if "nodes" not in workflow:
        errors.append("workflow is missing the 'nodes' property")
    if "connections" not in workflow:
        errors.append("workflow is missing the 'connections' property")

    nodes = workflow.get("nodes")
    if not isinstance(nodes, list):
        errors.append("workflow 'nodes' is not a list")
        nodes = []
    elif not nodes:
        errors.append("workflow has no nodes")

    for index, node in enumerate(nodes):
        if not isinstance(node, dict):
            errors.append(f"node {index}: not a JSON object")
            continue
        node_name = node.get("name")
        if not node_name:
            errors.append(f"node {index}: missing name")
        elif len(str(node_name)) > 255:
            errors.append(f"node {index}: name is longer than 255 characters")
        if not node.get("type"):
            errors.append(f"node {index}: missing type")

    supported = set(KNOWN_NODE_TYPES)
    if known_types:
        supported |= {t for t in known_types if isinstance(t, str)}

    unsupported = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        node_type = node.get("type")
        if not node_type or node_type in TOLERATED_NODE_TYPES:
            continue
        if node_type not in supported:
            unsupported.append(node_type)

    for node_type in sorted(set(unsupported)):
        warnings.append(
            f"node type '{node_type}' is not implemented by m9m; "
            "the workflow is imported but that node cannot execute"
        )

    return {
        "valid": not errors,
        "errors": errors,
        "warnings": warnings,
        "unsupportedTypes": sorted(set(unsupported)),
    }


def to_m9m_payload(workflow):
    """Adapt a normalized n8n workflow to m9m's REST wire format.

    Two shape differences exist between n8n and m9m:

      * n8n tags are objects `[{id, name}]`; m9m's Go model has
        `Tags []string`, so the tag NAMES are sent;
      * n8n node credentials are `{type: {id, name}}`; m9m's Credential
        model is `{id, name, type}`, so the credential type is copied
        from the map key.

    Everything else (nodes, connections, versionId, active, ...) is
    already n8n-compatible and passes through untouched.
    """
    payload = copy.deepcopy(workflow)

    tags = payload.get("tags")
    if isinstance(tags, list):
        names = []
        for tag in tags:
            tag_name = tag.get("name") if isinstance(tag, dict) else tag
            if tag_name not in (None, ""):
                names.append(str(tag_name))
        payload["tags"] = names

    for node in payload.get("nodes", []):
        if not isinstance(node, dict):
            continue
        credentials = node.get("credentials")
        if not isinstance(credentials, dict):
            continue
        adapted = {}
        for cred_type, cred in credentials.items():
            if isinstance(cred, dict):
                if cred.get("__aiGatewayManaged"):
                    # AI Gateway managed credentials have no record in
                    # m9m's credential store — n8n skips them too
                    # (workflow-helpers.ts L301-302).
                    continue
                adapted[cred_type] = {
                    "id": cred.get("id") or "",
                    "name": cred.get("name") or "",
                    "type": cred.get("type") or cred_type,
                }
            elif isinstance(cred, str):
                adapted[cred_type] = {"id": "", "name": cred, "type": cred_type}
        node["credentials"] = adapted

    return payload


def fetch_m9m_node_types(base_url=None):
    """Ask m9m which node types it can execute (GET /api/v1/node-types).

    Returns a set of node type names. Never raises: an unreachable m9m
    yields an empty set and validation falls back to the static map.
    """
    base = (base_url or M9M_BASE_URL).rstrip("/")
    try:
        status, body = http_json(
            "GET",
            f"{base}/api/v1/node-types",
            headers=m9m_headers({"Accept": "application/json"}),
        )
    except Exception as e:
        print(f"[sync] node-type lookup failed: {e}")
        return set()
    if status != 200 or not isinstance(body, list):
        print(f"[sync] node-type lookup returned status {status}")
        return set()
    names = set()
    for entry in body:
        if isinstance(entry, str):
            names.add(entry)
        elif isinstance(entry, dict):
            name = entry.get("name")
            if isinstance(name, str):
                names.add(name)
    return names


def push_to_m9m(workflow, source="manual"):
    """Push a single workflow. Returns ("created"|"updated"|"failed", status, body)."""
    wf_id = workflow.get("id")
    headers = m9m_headers({"Content-Type": "application/json"})

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

def push_batch_to_m9m(payloads, active_state=ACTIVE_STATE_FALSE, source="import"):
    """Push a batch of n8n-shaped workflows to m9m's import endpoint.

    `POST /api/v1/workflows/import` accepts a JSON array of n8n
    workflow objects and mirrors n8n's batch import:

      * every entry is validated on its own - one invalid entry never
        aborts the rest of the batch;
      * a fresh `versionId` is assigned per workflow;
      * `active` is forced to false unless the request explicitly asks
        for `activeState=fromJson`;
      * credentials are resolved by name + type against m9m's
        credential store (an unresolvable one keeps its name and is
        reported as a warning).

    The payload is the same one `POST /api/v1/workflows` accepts:
    workflow id as a string, tags as plain strings (m9m stores names),
    node credentials in the new `{type: {id, name}}` shape and the
    connections in n8n's source-node map shape.

    Returns `(status, summary)`. `summary` is the parsed response
    `{"imported", "failed", "activeState", "workflows", "errors"}` and
    is None when the endpoint does not exist on the target m9m
    (HTTP 404/405) - the caller then falls back to the per-workflow
    endpoint.
    """
    url = f"{M9M_BASE_URL}/api/v1/workflows/import"
    if _is_from_json(active_state):
        # n8n's `--activeState=fromJson`; m9m honours the JSON `active`
        # flag only when the request asks for it explicitly.
        url += "?activeState=fromJson"
    headers = m9m_headers({"Content-Type": "application/json"})

    status, body = http_json("POST", url, headers, payloads)

    if status in (404, 405):
        # m9m build without the n8n import endpoint.
        print(f"[sync:{source}] m9m has no import endpoint (HTTP {status}); "
              "falling back to POST /api/v1/workflows")
        return status, None

    if status == 400 and isinstance(body, dict) and isinstance(body.get("summary"), dict):
        # The endpoint answers 400 when NOTHING could be imported; the
        # per-item errors live in the summary.
        body = body["summary"]

    return status, body

def _push_import_batch(pushable, active_state=ACTIVE_STATE_FALSE):
    """Push the importable entries of a batch into m9m.

    `pushable` is a list of `(verdict, normalized_workflow)` pairs.
    The verdicts are updated in place with the result of the push.

    m9m's n8n import endpoint is preferred (it mirrors n8n's batch
    import and resolves credentials server-side). m9m builds without
    that endpoint fall back to `POST /api/v1/workflows` per workflow.
    """
    payloads = [to_m9m_payload(normalized) for _verdict, normalized in pushable]
    status, summary = push_batch_to_m9m(payloads, active_state=active_state)

    if summary is None:
        # Fallback: per-workflow endpoint.
        for verdict, normalized in pushable:
            m9m_verdict, http_status, body = push_to_m9m(
                to_m9m_payload(normalized), source="import"
            )
            if m9m_verdict == "failed":
                verdict["status"] = "failed"
                verdict["reason"] = f"m9m rejected the workflow (HTTP {http_status}): {body}"
            else:
                verdict["status"] = "ok"
                verdict["reason"] = None
        return

    if not isinstance(summary, dict):
        for verdict, _normalized in pushable:
            verdict["status"] = "failed"
            verdict["reason"] = (
                f"m9m returned an unexpected import response (HTTP {status}): {summary!r}"
            )
        return

    imported = {}
    for item in summary.get("workflows") or []:
        if isinstance(item, dict) and isinstance(item.get("index"), int):
            imported[item["index"]] = item
    failed = {}
    for item in summary.get("errors") or []:
        if isinstance(item, dict) and isinstance(item.get("index"), int):
            failed[item["index"]] = item

    if not imported and not failed:
        # The whole request was rejected (e.g. an invalid payload) and
        # the endpoint reported no per-item result.
        message = summary.get("message")
        for verdict, _normalized in pushable:
            verdict["status"] = "failed"
            verdict["reason"] = message if isinstance(message, str) and message else (
                f"m9m did not report a result for this workflow (HTTP {status})"
            )
        return

    for position, (verdict, _normalized) in enumerate(pushable):
        item = imported.get(position)
        if item is not None:
            verdict["status"] = "ok"
            verdict["reason"] = None
            if item.get("id"):
                verdict["id"] = item["id"]
            for warning in item.get("warnings") or []:
                verdict["warnings"].append(warning)
            continue
        error = failed.get(position)
        if error is not None:
            verdict["status"] = "failed"
            verdict["reason"] = error.get("message") or "m9m rejected the workflow"
            continue
        verdict["status"] = "failed"
        verdict["reason"] = (
            f"m9m did not report a result for this workflow (HTTP {status})"
        )


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
    headers = m9m_headers({"Content-Type": "application/json"})

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

    headers = m9m_headers({"Content-Type": "application/json"})
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

                # Normalize the n8n payload, then adapt it to m9m's wire
                # format (n8n tags are objects, m9m wants strings).
                # `active_state=from-json` keeps the live `active` flag:
                # m9m registers webhooks only for active workflows, so
                # deactivating here would silently break every webhook.
                # The versionId is carried over from n8n so m9m tracks
                # n8n's version, not a random uuid.
                normalization_warnings = []
                normalized = normalize_n8n_workflow(
                    wf,
                    active_state=ACTIVE_STATE_FROM_JSON,
                    version_id=wf.get("versionId"),
                    warnings=normalization_warnings,
                )
                for warning in normalization_warnings:
                    print(f"[sync:{source}] warning for '{normalized.get('name')}' ({wf_id}): {warning}")
                check = validate_n8n_workflow(normalized)
                if not check["valid"]:
                    print(f"[sync:{source}] skipping '{normalized.get('name')}' ({wf_id}): "
                          f"{'; '.join(check['errors'])}")
                    results.append({
                        "id": wf_id,
                        "name": normalized.get("name"),
                        "status": "skipped",
                        "detail": "; ".join(check["errors"]),
                    })
                    continue
                for warning in check["warnings"]:
                    print(f"[sync:{source}] warning for '{normalized.get('name')}' ({wf_id}): {warning}")

                payload = to_m9m_payload(normalized)
                verdict, status, _body = await asyncio.to_thread(push_to_m9m, payload, source)
                results.append({
                    "id": wf_id,
                    "name": normalized.get("name"),
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


# ---------------------------------------------------------------------------
# Local-file import CLI
#
# Mirrors n8n's `n8n import:workflow --input=file.json` semantics:
#   * the file may hold a single workflow object OR an array of them
#     (n8n's `export:workflow` always writes an array);
#   * an entry missing `nodes` or `connections` is INVALID — n8n aborts
#     the whole file, the bridge reports the entry and keeps going
#     (n8n's `--separate` behaviour);
#   * `--activeState=false` (default) deactivates imported workflows,
#     `--activeState=fromJson` honours the JSON `active` field.
#
# The push target is m9m's n8n import endpoint
# (`POST /api/v1/workflows/import`, see
# internal/api/workflow_import_export.go). It mirrors n8n's batch
# import: per-entry validation, one bad entry never aborts the batch,
# server-side credential resolution by name + type, a fresh versionId
# per workflow and `active=false` unless `activeState=fromJson` is
# asked for. m9m builds without that endpoint fall back to
# `POST /api/v1/workflows` per workflow.
# ---------------------------------------------------------------------------

def import_workflows_from_file(path, active_state=ACTIVE_STATE_FALSE,
                               dry_run=False, known_types=None):
    """Import one workflow object or an array of them from `path`.

    Every entry is normalized and validated on its own; an invalid
    entry is reported and the batch continues. The valid entries are
    pushed as one batch to m9m's import endpoint.

    Returns a list of verdict dicts, one per entry:
        {"index", "name", "id", "status": "ok"|"skipped"|"failed",
         "reason", "warnings", "normalized"}
    """
    with open(path, "r", encoding="utf-8") as fh:
        payload = json.load(fh)

    if isinstance(payload, list):
        entries = payload
    elif isinstance(payload, dict):
        entries = [payload]
    else:
        raise InvalidWorkflowError(
            "import file must contain a workflow object or an array of workflow objects"
        )

    verdicts = []
    pushable = []
    for index, entry in enumerate(entries):
        verdict = {
            "index": index,
            "name": None,
            "id": None,
            "status": "skipped",
            "reason": None,
            "warnings": [],
            "normalized": None,
        }

        if isinstance(entry, dict):
            verdict["name"] = entry.get("name")
            verdict["id"] = entry.get("id")

        try:
            normalization_warnings = []
            normalized = normalize_n8n_workflow(
                entry, active_state=active_state, warnings=normalization_warnings
            )
        except InvalidWorkflowError as e:
            verdict["reason"] = str(e)
            verdicts.append(verdict)
            continue
        except Exception as e:  # defensive — one bad entry never kills the batch
            verdict["reason"] = f"normalization failed: {e}"
            verdicts.append(verdict)
            continue

        verdict["name"] = normalized.get("name")
        verdict["id"] = normalized.get("id")
        verdict["normalized"] = normalized

        check = validate_n8n_workflow(normalized, known_types=known_types)
        # Normalization notes (dropped typeless nodes, sanitized
        # connections, skipped credentials) are warnings too — they
        # never fail an import.
        verdict["warnings"] = list(normalization_warnings) + list(check["warnings"])
        if not check["valid"]:
            verdict["reason"] = "; ".join(check["errors"])
            verdicts.append(verdict)
            continue

        if dry_run:
            verdict["status"] = "ok"
            verdicts.append(verdict)
            continue

        # The entry is valid - it is pushed with the rest of the batch.
        pushable.append((verdict, normalized))
        verdicts.append(verdict)

    if pushable:
        _push_import_batch(pushable, active_state=active_state)

    return verdicts


def _print_import_verdicts(verdicts, dry_run=False):
    """Print one line per workflow plus the normalized JSON in dry-run."""
    for verdict in verdicts:
        label = verdict.get("name") or f"entry #{verdict['index']}"
        wf_id = verdict.get("id")
        if verdict["status"] == "ok":
            print(f"[import] ok       {label} ({wf_id})")
        elif verdict["status"] == "skipped":
            print(f"[import] skipped  {label}: {verdict.get('reason')}")
        else:
            print(f"[import] failed   {label}: {verdict.get('reason')}")
        for warning in verdict.get("warnings", []):
            print(f"[import] warning  {label}: {warning}")

        if dry_run and verdict.get("normalized") is not None:
            print(json.dumps(verdict["normalized"], indent=2, sort_keys=True))


def build_arg_parser():
    parser = argparse.ArgumentParser(
        prog="sync.py",
        description=(
            "n8n -> m9m sync bridge. With no arguments it runs the FastAPI "
            "service (background poller + HTTP API). With --import it "
            "imports a local n8n workflow export file."
        ),
    )
    parser.add_argument(
        "--import",
        dest="import_path",
        metavar="FILE",
        help="import a workflow JSON file (single object or array of objects)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="validate + normalize only; print the result and make zero HTTP requests",
    )
    parser.add_argument(
        "--active-state",
        "--activeState",
        dest="active_state",
        choices=["false", "from-json", "fromJson"],
        default="false",
        help=(
            "'false' (default) deactivates imported workflows; "
            "'from-json' honours the JSON 'active' field (n8n's --activeState=fromJson)"
        ),
    )
    return parser


def main(argv=None):
    """Entry point. Returns a process exit code."""
    args = build_arg_parser().parse_args(argv)

    if args.import_path:
        active_state = ACTIVE_STATE_FROM_JSON if _is_from_json(args.active_state) else ACTIVE_STATE_FALSE
        try:
            # Node-type cross-check needs m9m to be reachable; a dry run
            # must not make ANY HTTP request, so it stays static-only.
            known_types = None if args.dry_run else fetch_m9m_node_types()
            verdicts = import_workflows_from_file(
                args.import_path,
                active_state=active_state,
                dry_run=args.dry_run,
                known_types=known_types,
            )
        except FileNotFoundError:
            print(f"[import] file not found: {args.import_path}")
            return 2
        except json.JSONDecodeError as e:
            print(f"[import] {args.import_path} is not valid JSON: {e}")
            return 2
        except InvalidWorkflowError as e:
            print(f"[import] {e}")
            return 2

        _print_import_verdicts(verdicts, dry_run=args.dry_run)
        ok = sum(1 for v in verdicts if v["status"] == "ok")
        skipped = sum(1 for v in verdicts if v["status"] == "skipped")
        failed = sum(1 for v in verdicts if v["status"] == "failed")
        print(f"[import] {ok} ok, {skipped} skipped, {failed} failed")
        return 1 if failed else 0

    # No --import: boot the FastAPI service exactly as before.
    import uvicorn

    uvicorn.run("sync:app", host="0.0.0.0", port=int(os.environ.get("SYNC_PORT", "8001")), log_level="info")
    return 0


if __name__ == "__main__":
    sys.exit(main())
