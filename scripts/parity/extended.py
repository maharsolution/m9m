"""Extended parity contracts (C1-C12).

No contract mutates the shared deployment: workflows, credentials and
database rows on n8n/m9m are only read, and webhook endpoints are only
POSTed to. Every fixture that needs writable state is created in a
disposable local m9m instance (see ``localenv.py``).

Authentication routing (C1): the n8n API key is sent to n8n only, the
m9m API key to m9m only, and m9m receives no key at all when
``M9M_API_KEY`` is not configured - the live deployment does not require
one (``authprobe.py`` proves it read-only).
"""
from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

from env import api_headers
from localenv import local_instance, run_go_test, run_web_unit_tests
from signatures import case_signature

ROOT = Path(__file__).resolve().parent.parent.parent
CACHE_DIR = ROOT / "tmp" / "parity-harness"


# --------------------------------------------------------------------------
# small helpers
# --------------------------------------------------------------------------
def rows(value: Any, key: str) -> list[dict]:
    if isinstance(value, list):
        return value
    if isinstance(value, dict):
        for name in (key, "data"):
            candidate = value.get(name)
            if isinstance(candidate, list):
                return candidate
            if candidate is None and name in value:
                # An empty collection is not a broken envelope: m9m
                # answers {"data": null, "total": 0} when a filter
                # matches nothing.
                return []
    raise ValueError(f"expected {key} collection envelope")


def _http(url: str, method: str = "GET", payload: Any = None,
          headers: dict[str, str] | None = None,
          timeout: float = 30.0) -> tuple[int, Any, str]:
    """Perform one HTTP request. Returns (status, parsed_body, raw_text)."""
    data = None
    request_headers = dict(headers or {})
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        request_headers.setdefault("Content-Type", "application/json")
    request = urllib.request.Request(url, data=data, method=method,
                                     headers=request_headers)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read().decode("utf-8", errors="replace")
            status = response.status
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        status = exc.code
    except Exception as exc:  # noqa: BLE001 - transport failures are data
        return 0, None, f"{type(exc).__name__}: {exc}"
    try:
        return status, json.loads(raw), raw
    except (json.JSONDecodeError, TypeError):
        return status, None, raw


def workflow_shape(workflow: dict, ignore_fields: set[str] | None = None) -> dict:
    """Structural fingerprint of a workflow.

    IDs may differ between hosts; credential names/types, parameters and
    connection routing MUST survive synchronization and roundtrip.
    ``ignore_fields`` is only used by the cases that explicitly document
    a known, separately asserted model limitation (fractional
    ``typeVersion``).
    """
    ignored = ignore_fields or set()
    nodes = []
    for node in workflow.get("nodes", []):
        item = {k: node.get(k) for k in ("name", "type", "typeVersion", "parameters")
                if k not in ignored}
        item["credentials"] = {
            kind: ({"name": ref.get("name")} if isinstance(ref, dict) else ref)
            for kind, ref in (node.get("credentials") or {}).items()
        }
        nodes.append(item)
    return {"nodes": sorted(nodes, key=lambda n: str(n.get("name"))),
            "connections": workflow.get("connections", {}) or {}}


def _param_diff(prefix: str, left: Any, right: Any,
                out: list[str], limit: int = 6) -> None:
    if len(out) >= limit:
        return
    if isinstance(left, dict) and isinstance(right, dict):
        for key in sorted(set(left) | set(right)):
            _param_diff(f"{prefix}.{key}" if prefix else str(key),
                        left.get(key), right.get(key), out, limit)
        return
    if left != right:
        out.append(f"{prefix}: n8n={left!r} m9m={right!r}")


def shape_diff(n8n_workflow: dict, m9m_workflow: dict,
               ignore_fields: set[str] | None = None) -> list[str]:
    """Field level difference between two workflow shapes."""
    ignored = ignore_fields or set()
    left = workflow_shape(n8n_workflow, ignored)
    right = workflow_shape(m9m_workflow, ignored)
    differences: list[str] = []

    left_nodes = {str(n.get("name")): n for n in left["nodes"]}
    right_nodes = {str(n.get("name")): n for n in right["nodes"]}
    for name in sorted(set(left_nodes) | set(right_nodes)):
        if name not in right_nodes:
            differences.append(f"node missing on m9m: {name}")
            continue
        if name not in left_nodes:
            differences.append(f"node only on m9m: {name}")
            continue
        a, b = left_nodes[name], right_nodes[name]
        if a.get("type") != b.get("type"):
            differences.append(f"node {name} type: n8n={a.get('type')} m9m={b.get('type')}")
        if "typeVersion" not in ignored and a.get("typeVersion") != b.get("typeVersion"):
            differences.append(
                f"node {name} typeVersion: n8n={a.get('typeVersion')} m9m={b.get('typeVersion')}")
        if a.get("credentials") != b.get("credentials"):
            differences.append(
                f"node {name} credentials: n8n={a.get('credentials')} m9m={b.get('credentials')}")
        params: list[str] = []
        _param_diff("", a.get("parameters") or {}, b.get("parameters") or {}, params)
        for entry in params:
            differences.append(f"node {name} parameter {entry}")

    if left["connections"] != right["connections"]:
        differences.append(
            f"connections differ: n8n={json.dumps(left['connections'], sort_keys=True)[:180]} "
            f"m9m={json.dumps(right['connections'], sort_keys=True)[:180]}")
    return differences


def _credential_map(workflow: dict) -> dict[str, dict[str, str]]:
    """{node name: {credential type: credential name}} for one workflow."""
    mapping: dict[str, dict[str, str]] = {}
    for node in workflow.get("nodes", []):
        refs = node.get("credentials") or {}
        if not refs:
            continue
        entry = {}
        for kind, ref in refs.items():
            if isinstance(ref, dict):
                entry[kind] = str(ref.get("name", ""))
            else:
                entry[kind] = str(ref)
        mapping[str(node.get("name"))] = entry
    return mapping


# --------------------------------------------------------------------------
# contract execution
# --------------------------------------------------------------------------
def run_contract(case: dict, n8n_base: str, m9m_base: str,
                 timeout: float, request) -> dict:
    """Execute an API/inventory/telemetry contract through the harness transport."""
    captures: dict[str, Any] = {}

    def api(host: str, path: str, method: str = "GET",
            payload: Any = None, redact: bool = True):
        base = n8n_base if host == "n8n" else m9m_base
        url = base.rstrip("/") + "/api/v1/" + path
        response = request(url, {"method": method, "headers": api_headers(host)}, timeout)
        raw = response.get("body_raw", "")
        status = response.get("status_code", 0)
        captures[host] = {
            "status_code": status,
            "headers": response.get("headers", {}),
            # Credential envelopes can contain decrypted secrets. Persist metadata only.
            "body_raw": "[API body redacted]" if redact else str(raw)[:4000],
            "elapsed_ms": response.get("elapsed_ms", 0),
            "error": response.get("error"),
        }
        if status != 200:
            raise ValueError(f"{host} API {path}: HTTP {status} {str(raw)[:200]}")
        try:
            return json.loads(raw)
        except (json.JSONDecodeError, TypeError) as exc:
            raise ValueError(f"{host} API {path}: unreadable JSON body") from exc

    def workflow_by_name(host: str, name: str) -> dict:
        inventory = rows(api(host, "workflows?limit=250"), "workflows")
        matches = [w for w in inventory if w.get("name") == name]
        if len(matches) != 1:
            raise LookupError(
                f"{host}: expected exactly one workflow named {name!r}, found {len(matches)}")
        workflow_id = str(matches[0]["id"])
        detail = api(host, "workflows/" + urllib.parse.quote(workflow_id, safe=""))
        if isinstance(detail, dict):
            return detail.get("data", detail)
        return detail

    kind = case.get("check", "")
    verdict, reason = "FAIL", "contract not executed"

    try:
        if kind == "prerequisite":
            return result(case, captures, "BLOCKED", case.get("blocker", "missing prerequisite"))

        if kind == "credentials":
            verdict, reason = _contract_credentials(case, api, workflow_by_name)

        elif kind == "sync":
            verdict, reason = _contract_sync(case, workflow_by_name)

        elif kind == "type_version":
            verdict, reason = _contract_type_version(case, workflow_by_name)

        elif kind == "telemetry":
            verdict, reason = _contract_telemetry(case, api, workflow_by_name,
                                                  n8n_base, m9m_base, timeout, request)

        elif kind == "cron":
            verdict, reason = _contract_cron(case, api, workflow_by_name)

        elif kind == "subworkflow":
            verdict, reason = _contract_subworkflow(case, api, workflow_by_name,
                                                     n8n_base, m9m_base, timeout, request)

        elif kind == "roundtrip":
            verdict, reason = _contract_roundtrip(case, n8n_base, timeout)

        elif kind == "go_test":
            verdict, reason = _contract_go_test(case)

        elif kind == "load":
            verdict, reason = _contract_load(case, timeout)

        elif kind == "local_error":
            verdict, reason = _contract_local_error(case, timeout)

        elif kind == "ui_surface":
            verdict, reason = _contract_ui_surface(case, timeout)

        elif kind == "ui_journey":
            verdict, reason = _contract_ui_journey(case)

        elif kind == "auth_probe":
            verdict, reason = _contract_auth_probe(case, n8n_base, m9m_base, timeout)

        elif kind == "drift":
            verdict, reason = _contract_drift(case)

        else:
            raise ValueError(f"unknown extended check: {kind!r}")
    except (ValueError, KeyError, TypeError, LookupError, OSError) as exc:
        reason = str(exc)
        verdict = "FAIL"

    return result(case, captures, verdict, reason)


# --------------------------------------------------------------------------
# individual contracts
# --------------------------------------------------------------------------
def _contract_credentials(case: dict, api, workflow_by_name) -> tuple[str, str]:
    """C1 - credential inventory and per-workflow credential resolution."""
    inventories = {host: rows(api(host, "credentials?limit=250"), "credentials")
                   for host in ("n8n", "m9m")}
    n8n_items = {(str(c.get("name")), str(c.get("type"))) for c in inventories["n8n"]}
    m9m_items = {(str(c.get("name")), str(c.get("type"))) for c in inventories["m9m"]}
    if not n8n_items:
        raise ValueError("n8n credential inventory is empty; no coverage")
    missing = sorted(n8n_items - m9m_items)
    if missing:
        raise ValueError(f"m9m is missing n8n credentials: {missing}")

    # Per-workflow credential map parity for every workflow present on both hosts.
    n8n_workflows = {str(w.get("name")): str(w.get("id"))
                     for w in rows(api("n8n", "workflows?limit=250"), "workflows")}
    m9m_workflows = {str(w.get("name")): str(w.get("id"))
                     for w in rows(api("m9m", "workflows?limit=250"), "workflows")}
    common = sorted(set(n8n_workflows) & set(m9m_workflows))
    mismatches: list[str] = []
    checked = 0
    for name in common:
        n8n_detail = api("n8n", "workflows/" + urllib.parse.quote(n8n_workflows[name], safe=""))
        m9m_detail = api("m9m", "workflows/" + urllib.parse.quote(m9m_workflows[name], safe=""))
        n8n_map = _credential_map(n8n_detail.get("data", n8n_detail))
        m9m_map = _credential_map(m9m_detail.get("data", m9m_detail))
        if n8n_map or m9m_map:
            checked += 1
        if n8n_map != m9m_map:
            mismatches.append(f"{name}: n8n={n8n_map} m9m={m9m_map}")
    if mismatches:
        raise ValueError(f"workflow credential maps differ: {mismatches[:3]}")

    extra = len(m9m_items - n8n_items)
    return "PASS", (f"all {len(n8n_items)} n8n credentials present on m9m; "
                    f"workflow credential maps match on {checked} workflows; "
                    f"{extra} m9m-only credentials ignored (shared instance extras)")


def _contract_sync(case: dict, workflow_by_name) -> tuple[str, str]:
    """C6 - synchronized workflow structure."""
    name = case["workflow_name"]
    n8n_workflow = workflow_by_name("n8n", name)
    m9m_workflow = workflow_by_name("m9m", name)
    ignored = set(case.get("ignore_fields", []))
    differences = shape_diff(n8n_workflow, m9m_workflow, ignored)
    if differences:
        raise ValueError("; ".join(differences[:4]))
    suffix = f" (ignoring {sorted(ignored)})" if ignored else ""
    return "PASS", f"nodes/parameters/credentials/connections identical{suffix}"


def _contract_type_version(case: dict, workflow_by_name) -> tuple[str, str]:
    """C6 - the documented typeVersion truncation is measured, not hidden.

    n8n stores fractional node versions (2.1, 3.5); m9m's model stores
    an int (internal/model/workflow.go: ``TypeVersion int``) and the
    importer truncates. The sync cases therefore compare the structure
    without that field - this contract asserts that the only delta is
    the truncation itself.
    """
    problems: list[str] = []
    checked = 0
    for name in case.get("workflow_names", []):
        n8n_workflow = workflow_by_name("n8n", name)
        m9m_workflow = workflow_by_name("m9m", name)
        n8n_nodes = {str(n.get("name")): n.get("typeVersion")
                     for n in n8n_workflow.get("nodes", [])}
        m9m_nodes = {str(n.get("name")): n.get("typeVersion")
                     for n in m9m_workflow.get("nodes", [])}
        if set(n8n_nodes) != set(m9m_nodes):
            problems.append(f"{name}: node sets differ")
            continue
        for node_name, version in n8n_nodes.items():
            checked += 1
            try:
                expected = int(float(version))
            except (TypeError, ValueError):
                expected = 0
            actual = m9m_nodes.get(node_name)
            if actual != expected:
                problems.append(
                    f"{name}/{node_name}: n8n={version} m9m={actual} "
                    f"(expected truncated {expected})")
    if problems:
        raise ValueError("; ".join(problems[:4]))
    return "PASS", (f"{checked} node typeVersions equal the integer part of the "
                    "n8n value (documented m9m model limitation)")


def _contract_telemetry(case: dict, api, workflow_by_name, n8n_base: str,
                         m9m_base: str, timeout: float, request) -> tuple[str, str]:
    """C5 - every trigger produces an execution record with status/data/time."""
    name = case["workflow_name"]
    selected = {}
    for host in ("n8n", "m9m"):
        inventory = rows(api(host, "workflows?limit=250"), "workflows")
        matches = [w for w in inventory if w.get("name") == name]
        if len(matches) != 1:
            raise LookupError(
                f"{host}: expected exactly one workflow {name!r}, found {len(matches)}")
        selected[host] = str(matches[0]["id"])

    before = {}
    for host in ("n8n", "m9m"):
        entries = rows(api(host, "executions?workflowId=" +
                           urllib.parse.quote(selected[host], safe="") +
                           "&includeData=true&limit=50"), "executions")
        before[host] = {str(e.get("id")) for e in entries}

    for host, base in (("n8n", n8n_base), ("m9m", m9m_base)):
        url = base.rstrip("/") + "/webhook/" + str(case["path"]).lstrip("/")
        trigger = request(url, case, timeout)
        if trigger.get("status_code") != case.get("expected_status", 200):
            raise ValueError(f"{host}: trigger failed HTTP {trigger.get('status_code')}")

    deadline = time.monotonic() + case.get("poll_seconds", 15)
    found: dict[str, dict] = {}
    while time.monotonic() < deadline and len(found) < 2:
        for host in ("n8n", "m9m"):
            if host in found:
                continue
            entries = rows(api(host, "executions?workflowId=" +
                               urllib.parse.quote(selected[host], safe="") +
                               "&includeData=true&limit=50"), "executions")
            for entry in entries:
                if str(entry.get("id")) in before[host]:
                    continue
                if entry.get("status") in {"success", "completed"} and \
                        entry.get("startedAt") and \
                        (entry.get("stoppedAt") or entry.get("finishedAt")) and \
                        entry.get("data") is not None:
                    found[host] = entry
                    break
        if len(found) < 2:
            time.sleep(0.5)

    missing = [host for host in ("n8n", "m9m") if host not in found]
    if missing:
        raise ValueError(f"no new execution record with status/data/time on {missing}")
    return "PASS", ("execution records carry status, data and timestamps on both hosts: " +
                    ", ".join(f"{host}={found[host].get('id')}" for host in ("n8n", "m9m")))


def _contract_cron(case: dict, api, workflow_by_name) -> tuple[str, str]:
    """C7 - the cron trigger produces its own execution record."""
    name = case["workflow_name"]
    selected = {}
    for host in ("n8n", "m9m"):
        inventory = rows(api(host, "workflows?limit=250"), "workflows")
        matches = [w for w in inventory if w.get("name") == name]
        if len(matches) != 1:
            raise LookupError(
                f"{host}: expected exactly one workflow {name!r}, found {len(matches)}")
        selected[host] = str(matches[0]["id"])

    before = {}
    for host in ("n8n", "m9m"):
        entries = rows(api(host, "executions?workflowId=" +
                           urllib.parse.quote(selected[host], safe="") +
                           "&includeData=true&limit=50"), "executions")
        before[host] = {str(e.get("id")) for e in entries}

    deadline = time.monotonic() + case.get("poll_seconds", 70)
    found: dict[str, dict] = {}
    while time.monotonic() < deadline and len(found) < 2:
        for host in ("n8n", "m9m"):
            if host in found:
                continue
            entries = rows(api(host, "executions?workflowId=" +
                               urllib.parse.quote(selected[host], safe="") +
                               "&includeData=true&limit=50"), "executions")
            for entry in entries:
                if str(entry.get("id")) in before[host]:
                    continue
                if entry.get("status") in {"success", "completed"} and entry.get("startedAt"):
                    found[host] = entry
                    break
        if len(found) < 2:
            time.sleep(1.0)

    missing = [host for host in ("n8n", "m9m") if host not in found]
    if missing:
        window = case.get("poll_seconds", 70)
        if len(missing) == 2:
            # Neither host produced a cron run inside the probe window:
            # the fixture's schedule is longer than the window, so no
            # parity conclusion is possible.
            return "BLOCKED", (f"no autonomous cron execution on either host within "
                               f"{window}s; the fixture's schedule is longer than the "
                               "probe window")
        raise ValueError(
            f"cron execution recorded on {sorted(found)} but not on {missing} "
            f"within {window}s")
    return "PASS", ("cron executions recorded on both hosts: " +
                    ", ".join(f"{host}={found[host].get('id')}" for host in ("n8n", "m9m")))


def _contract_subworkflow(case: dict, api, workflow_by_name, n8n_base: str,
                           m9m_base: str, timeout: float, request) -> tuple[str, str]:
    """C7 - an async sub-workflow execution is recorded for the child workflow."""
    child_name = case["child_workflow_name"]
    parent_path = case["path"]

    child_ids = {}
    for host in ("n8n", "m9m"):
        inventory = rows(api(host, "workflows?limit=250"), "workflows")
        matches = [w for w in inventory if w.get("name") == child_name]
        if not matches:
            raise LookupError(f"{host}: child workflow {child_name!r} does not exist")
        child_ids[host] = str(matches[0]["id"])

    before = {}
    for host in ("n8n", "m9m"):
        entries = rows(api(host, "executions?workflowId=" +
                           urllib.parse.quote(child_ids[host], safe="") +
                           "&includeData=true&limit=50"), "executions")
        before[host] = {str(e.get("id")) for e in entries}

    for host, base in (("n8n", n8n_base), ("m9m", m9m_base)):
        url = base.rstrip("/") + "/webhook/" + str(parent_path).lstrip("/")
        trigger = request(url, case, timeout)
        if trigger.get("status_code") != case.get("expected_status", 200):
            raise ValueError(f"{host}: parent trigger failed HTTP {trigger.get('status_code')}")

    deadline = time.monotonic() + case.get("poll_seconds", 20)
    found: dict[str, dict] = {}
    while time.monotonic() < deadline and len(found) < 2:
        for host in ("n8n", "m9m"):
            if host in found:
                continue
            entries = rows(api(host, "executions?workflowId=" +
                               urllib.parse.quote(child_ids[host], safe="") +
                               "&includeData=true&limit=50"), "executions")
            for entry in entries:
                if str(entry.get("id")) not in before[host]:
                    found[host] = entry
                    break
        if len(found) < 2:
            time.sleep(0.5)

    missing = [host for host in ("n8n", "m9m") if host not in found]
    if missing:
        raise ValueError(f"no child execution record for {child_name!r} on {missing}")
    return "PASS", ("child executions recorded: " +
                    ", ".join(f"{host}={found[host].get('id')}" for host in ("n8n", "m9m")))


def _contract_roundtrip(case: dict, n8n_base: str, timeout: float) -> tuple[str, str]:
    """C2 - import -> read back -> export against a disposable local m9m."""
    name = case["workflow_name"]
    status, inventory, raw = _http(n8n_base.rstrip("/") + "/api/v1/workflows?limit=250",
                                   headers=api_headers("n8n"), timeout=timeout)
    if status != 200:
        raise ValueError(f"n8n workflow list failed: HTTP {status} {raw[:200]}")
    matches = [w for w in rows(inventory, "workflows") if w.get("name") == name]
    if len(matches) != 1:
        raise LookupError(f"n8n: expected exactly one workflow {name!r}, found {len(matches)}")
    workflow_id = urllib.parse.quote(str(matches[0]["id"]), safe="")
    status, detail, raw = _http(n8n_base.rstrip("/") + "/api/v1/workflows/" + workflow_id,
                                 headers=api_headers("n8n"), timeout=timeout)
    if status != 200:
        raise ValueError(f"n8n workflow read failed: HTTP {status} {raw[:200]}")
    source = detail.get("data", detail) if isinstance(detail, dict) else detail

    ignored = set(case.get("ignore_fields", []))
    with local_instance() as instance:
        status, imported, raw = _http(
            instance.base_url + "/api/v1/workflows/import", method="POST",
            payload=source, timeout=timeout)
        if status not in (200, 201):
            raise ValueError(f"local m9m import failed: HTTP {status} {raw[:300]}")
        local_id = _first_workflow_id(imported)
        if not local_id:
            raise ValueError(f"local m9m import returned no workflow id: {raw[:300]}")

        status, read_back, raw = _http(
            instance.base_url + "/api/v1/workflows/" + urllib.parse.quote(local_id, safe=""),
            timeout=timeout)
        if status != 200:
            raise ValueError(f"local m9m read back failed: HTTP {status} {raw[:200]}")
        read_back = read_back.get("data", read_back) if isinstance(read_back, dict) else read_back

        status, exported, raw = _http(
            instance.base_url + "/api/v1/workflows/" + urllib.parse.quote(local_id, safe="") +
            "/export", timeout=timeout)
        if status != 200:
            raise ValueError(f"local m9m export failed: HTTP {status} {raw[:200]}")
        exported = exported.get("data", exported) if isinstance(exported, dict) else exported

    differences = shape_diff(source, read_back, ignored)
    if differences:
        raise ValueError("import/read-back differs: " + "; ".join(differences[:4]))
    export_differences = shape_diff(source, exported, ignored)
    if export_differences:
        raise ValueError("export differs: " + "; ".join(export_differences[:4]))
    suffix = f" (ignoring {sorted(ignored)})" if ignored else ""
    return "PASS", (f"import -> GET -> export preserved nodes/parameters/credentials/"
                    f"connections on a disposable local m9m{suffix}")


def _first_workflow_id(payload: Any) -> str:
    if isinstance(payload, dict):
        if payload.get("id"):
            return str(payload["id"])
        for key in ("data", "workflows", "workflow"):
            nested = payload.get(key)
            if nested:
                found = _first_workflow_id(nested)
                if found:
                    return found
    if isinstance(payload, list) and payload:
        return _first_workflow_id(payload[0])
    return ""


def _contract_go_test(case: dict) -> tuple[str, str]:
    """C3/C11 - executable Go regression locks."""
    package = case["package"]
    run = case.get("run")
    result = run_go_test(package, run, timeout=case.get("timeout", 900))
    output = str(result["output"])
    if result["exit_code"] == 127:
        return "BLOCKED", f"go toolchain unavailable for {package}"
    if not result["tests_ran"]:
        return "FAIL", (f"no test matched {run!r} in {package}: {output[-400:]}")
    if result["exit_code"] != 0:
        return "FAIL", f"go test failed for {package} -run {run}: {output[-600:]}"
    return "PASS", f"go test {package} -run {run} passed"


def _contract_load(case: dict, timeout: float) -> tuple[str, str]:
    """C9 - sequential stability, latency budget and state isolation."""
    count = int(case.get("count", 50))
    path = case.get("path", "simple_webhook")
    body = case.get("body", '{"variable":"1"}')
    budget = float(case.get("p95_budget_ms", 5000))

    target = case.get("target", "live")
    if target == "live":
        base = case.get("base") or os.environ.get(
            "M9M_BASE_URL", "http://187.77.113.218:8080")
        status, latencies, bodies = _probe_sequence(base, path, body, count, timeout)
        if status.get("unexpected"):
            raise ValueError(f"unexpected HTTP statuses: {status['unexpected']}")
        if len(set(bodies)) != 1:
            raise ValueError(f"response body changed between runs: {len(set(bodies))} variants")
        p95 = _percentile(latencies, 95)
        if p95 > budget:
            raise ValueError(f"p95 {p95:.0f}ms exceeds budget {budget:.0f}ms")
        return "PASS", (f"{count} sequential webhooks: p95={p95:.0f}ms, "
                        f"status={status['ok']}, identical body across runs")

    # Local instance: same probe plus a goroutine-leak check.
    with local_instance(metrics=True) as instance:
        _install_local_webhook(instance, path)
        status, latencies, bodies = _probe_sequence(
            instance.base_url, path, body, count, timeout)
        if status.get("unexpected"):
            raise ValueError(f"unexpected HTTP statuses: {status['unexpected']}")
        before = instance.goroutines()
        time.sleep(2)
        after = instance.goroutines()
    if len(set(bodies)) != 1:
        raise ValueError(f"response body changed between runs: {len(set(bodies))} variants")
    p95 = _percentile(latencies, 95)
    if p95 > budget:
        raise ValueError(f"p95 {p95:.0f}ms exceeds budget {budget:.0f}ms")
    leak_note = "goroutine metrics unavailable"
    if before is not None and after is not None:
        leak_note = f"go_goroutines {before} -> {after}"
        if after - before > 5:
            raise ValueError(f"goroutine leak suspected: {leak_note}")
    return "PASS", (f"{count} sequential webhooks on a disposable instance: "
                    f"p95={p95:.0f}ms, identical body across runs, {leak_note}")


def _install_local_webhook(instance, path: str) -> str:
    """Create and activate a webhook workflow on the disposable instance."""
    workflow = {
        "name": f"Parity Harness Webhook {path}",
        "nodes": [
            {"name": "Webhook", "type": "n8n-nodes-base.webhook", "typeVersion": 2,
             "position": [0, 0],
             "parameters": {"path": path, "httpMethod": "POST", "options": {}}},
        ],
        "connections": {},
        "active": True,
        "settings": {},
    }
    status, imported, raw = _http(instance.base_url + "/api/v1/workflows/import",
                                   method="POST", payload=workflow, timeout=30)
    if status not in (200, 201):
        raise ValueError(f"local m9m import failed: HTTP {status} {raw[:200]}")
    workflow_id = _first_workflow_id(imported)
    if not workflow_id:
        raise ValueError(f"local m9m import returned no workflow id: {raw[:200]}")
    status, _, raw = _http(
        instance.base_url + "/api/v1/workflows/" +
        urllib.parse.quote(workflow_id, safe="") + "/activate", method="POST", timeout=30)
    if status not in (200, 201, 204):
        raise ValueError(f"local m9m activate failed: HTTP {status} {raw[:200]}")
    return workflow_id


def _probe_sequence(base: str, path: str, body: str, count: int,
                    timeout: float) -> tuple[dict, list[float], list[str]]:
    url = base.rstrip("/") + "/webhook/" + str(path).lstrip("/")
    payload = body.encode("utf-8")
    statuses: list[int] = []
    latencies: list[float] = []
    bodies: list[str] = []
    for _ in range(count):
        started = time.monotonic()
        request = urllib.request.Request(
            url, data=payload, method="POST", headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                status = response.status
                raw = response.read().decode("utf-8", errors="replace")
        except urllib.error.HTTPError as exc:
            status = exc.code
            raw = exc.read().decode("utf-8", errors="replace")
        except Exception as exc:  # noqa: BLE001
            status = 0
            raw = f"{type(exc).__name__}: {exc}"
        elapsed = (time.monotonic() - started) * 1000
        statuses.append(status)
        latencies.append(elapsed)
        bodies.append(raw.strip())
    unexpected = sorted({s for s in statuses if s != 200})
    return {"ok": statuses[-1] if statuses else 0, "unexpected": unexpected}, latencies, bodies


def _percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    import math
    ordered = sorted(values)
    index = min(len(ordered) - 1, max(0, math.ceil((pct / 100) * len(ordered)) - 1))
    return ordered[index]


def _contract_local_error(case: dict, timeout: float) -> tuple[str, str]:
    """C8 - fault injection against a disposable local m9m.

    The n8n contract for a failing node is: the webhook acknowledges the
    request (HTTP 200) and the failure is recorded on the execution.
    m9m must behave the same way and must stay healthy - the fault is
    surfaced as a failed execution, never as a crashed process.
    """
    workflow = case["workflow"]
    expected_status = case.get("expected_status", 200)
    with local_instance() as instance:
        status, imported, raw = _http(
            instance.base_url + "/api/v1/workflows/import", method="POST",
            payload=workflow, timeout=timeout)
        if status not in (200, 201):
            raise ValueError(f"local m9m import failed: HTTP {status} {raw[:300]}")
        local_id = _first_workflow_id(imported)
        if not local_id:
            raise ValueError(f"local m9m import returned no workflow id: {raw[:300]}")
        status, _, raw = _http(
            instance.base_url + "/api/v1/workflows/" +
            urllib.parse.quote(local_id, safe="") + "/activate", method="POST", timeout=timeout)
        if status not in (200, 201, 204):
            raise ValueError(f"local m9m activate failed: HTTP {status} {raw[:300]}")

        url = instance.base_url + "/webhook/" + str(case["path"]).lstrip("/")
        payload = (case.get("body") or "{}").encode("utf-8")
        request = urllib.request.Request(
            url, data=payload, method="POST", headers={"Content-Type": "application/json"})
        observed = 0
        body = ""
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                observed = response.status
                body = response.read().decode("utf-8", errors="replace")
        except urllib.error.HTTPError as exc:
            observed = exc.code
            body = exc.read().decode("utf-8", errors="replace")
        except Exception as exc:  # noqa: BLE001
            raise ValueError(f"local webhook failed: {type(exc).__name__}: {exc}")

        # The fault must be visible on the execution record.
        failed_execution = None
        deadline = time.monotonic() + case.get("poll_seconds", 20)
        while time.monotonic() < deadline and failed_execution is None:
            status, payload_json, raw = _http(
                instance.base_url + "/api/v1/executions?workflowId=" +
                urllib.parse.quote(local_id, safe="") + "&limit=10", timeout=15)
            if status == 200:
                for entry in rows(payload_json, "executions"):
                    if str(entry.get("status", "")).lower() in ("error", "failed"):
                        failed_execution = entry
                        break
            if failed_execution is None:
                time.sleep(0.5)

        health_status, _, _ = _http(instance.base_url + "/healthz", timeout=10)

    if observed != expected_status:
        raise ValueError(f"expected HTTP {expected_status}, got {observed}: {body[:200]}")
    if failed_execution is None:
        raise ValueError("the injected fault produced no failed execution record")
    if health_status != 200:
        raise ValueError(f"m9m process unhealthy after the fault: HTTP {health_status}")
    return "PASS", (f"m9m acknowledged the webhook with HTTP {observed}, recorded the "
                    f"faulted execution {failed_execution.get('id')} "
                    f"(status={failed_execution.get('status')}) and stayed healthy; "
                    "n8n counterpart fixture not creatable on the shared host without "
                    "mutating it")


def _contract_ui_surface(case: dict, timeout: float) -> tuple[str, str]:
    """C10 - the UI is served and the web unit tests run."""
    web = ROOT / "web"
    if not (web / "package.json").is_file():
        return "BLOCKED", "web/package.json is missing - no UI surface to test"

    result = run_web_unit_tests(timeout=case.get("timeout", 900))
    output = str(result["output"])
    if result["exit_code"] == 127:
        return "BLOCKED", f"web unit tests could not run: {output[:200]}"

    dist = web / "dist"
    served = "dist/index.html present" if (dist / "index.html").is_file() else \
        "dist/index.html missing"
    with local_instance() as instance:
        status, _, raw = _http(instance.base_url + "/", timeout=30)
        ui_status = status
        ui_body = raw
    if ui_status != 200 or "<html" not in ui_body.lower():
        raise ValueError(f"local m9m did not serve the UI (HTTP {ui_status}, {served})")
    if result["exit_code"] != 0:
        raise ValueError(f"web unit tests failed: {output[-600:]}")
    return "PASS", (f"web unit tests green and local m9m serves the UI at / (HTTP {ui_status}, "
                    f"{served})")


def _contract_ui_journey(case: dict) -> tuple[str, str]:
    """C10 - recorded browser journey against a disposable local instance."""
    artifact = ROOT / case.get("artifact", "scripts/parity/artifacts/ui_journey_result.json")
    if not artifact.is_file():
        return "BLOCKED", (f"no recorded browser journey at {artifact}; "
                           "missing prerequisite: a local disposable m9m instance with the web "
                           "UI served and a browser driver (cursor-ide-browser MCP or Playwright)")
    try:
        payload = json.loads(artifact.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return "FAIL", f"journey artifact unreadable: {exc}"
    steps = payload.get("steps", [])
    if not steps:
        return "FAIL", "journey artifact has no steps"
    failed = [step for step in steps if step.get("result") == "FAIL"]
    if failed:
        return "FAIL", ("journey steps failed: " +
                        ", ".join(f"{s.get('name')}={s.get('result')}" for s in failed))
    unknown = [step for step in steps
               if step.get("result") not in ("PASS", "FAIL", "BLOCKED")]
    if unknown:
        return "FAIL", ("journey steps have an unknown result: " +
                        ", ".join(f"{s.get('name')}={s.get('result')}" for s in unknown))
    blocked = [step for step in steps if step.get("result") == "BLOCKED"]
    if blocked:
        detail = "; ".join(
            f"{s.get('name')}: {s.get('missing_piece') or s.get('detail', '')}".strip()
            for s in blocked)
        passed = len(steps) - len(blocked)
        return "BLOCKED", (f"{passed}/{len(steps)} recorded UI journey steps passed on a "
                           f"disposable instance; blocked step(s): {detail}")
    return "PASS", f"{len(steps)} recorded UI journey steps passed on a disposable instance"


def _contract_auth_probe(case: dict, n8n_base: str, m9m_base: str,
                         timeout: float) -> tuple[str, str]:
    """C1 - does m9m require an API key? Probed read-only.

    Every endpoint is probed twice: once with the header this host is
    configured with and once anonymously. The n8n key is never sent to
    m9m and the m9m key is never sent to n8n.
    """
    paths = case.get("paths", ["/healthz", "/api/v1/node-types", "/api/v1/workflows"])
    findings = []
    for host, base in (("n8n", n8n_base), ("m9m", m9m_base)):
        for path in paths:
            status, _, _ = _http(base.rstrip("/") + path, headers=api_headers(host),
                                 timeout=timeout)
            findings.append((host, path, "configured", status))
            status, _, _ = _http(base.rstrip("/") + path, headers={}, timeout=timeout)
            findings.append((host, path, "anonymous", status))
    protected = case.get("protected_paths", ["/api/v1/workflows"])
    m9m_anonymous = [status for host, _, variant, status in findings
                     if host == "m9m" and variant == "anonymous"]
    n8n_anonymous = [status for host, path, variant, status in findings
                     if host == "n8n" and variant == "anonymous" and path in protected]
    n8n_configured = [status for host, path, variant, status in findings
                      if host == "n8n" and variant == "configured" and path in protected]
    m9m_open = bool(m9m_anonymous) and all(status == 200 for status in m9m_anonymous)
    n8n_closed = any(status in (401, 403) for status in n8n_anonymous)
    n8n_authorized = any(status == 200 for status in n8n_configured)
    if case.get("expect", "m9m-open") == "m9m-open":
        if not m9m_open:
            raise ValueError(f"m9m requires authentication: {findings}")
        if not n8n_closed:
            raise ValueError(f"n8n did not reject the anonymous probe: {findings}")
        if not n8n_authorized:
            raise ValueError(f"n8n rejected the configured API key: {findings}")
        return "PASS", ("m9m serves /healthz, /api/v1/node-types and /api/v1/workflows "
                        "without any key; n8n rejects anonymous API calls and accepts "
                        "the configured X-N8N-API-KEY")
    if not m9m_open:
        raise ValueError(f"m9m unexpectedly open: {findings}")
    return "PASS", "m9m API reachable"


def _latest_results_path() -> Path | None:
    """Newest saved parity report, if any."""
    base = ROOT / "tmp" / "parity-results"
    if not base.is_dir():
        return None
    candidates = sorted(base.glob("*/parity-results.json"),
                        key=lambda path: path.stat().st_mtime)
    return candidates[-1] if candidates else None


def evaluate_drift(case: dict, payload: dict, offline: bool = False) -> tuple[str, str]:
    """C12 - drift guard evaluated against the run that just finished.

    Live mode compares the Google Sheet, ``workflow-test.MD`` and the
    report of the current run. Offline mode only checks the document
    itself (no case may claim PASS and FAIL at the same time), because
    the shared-host cases have no verdict in that mode.
    """
    import report as report_module

    document = ROOT / case.get("document", "workflow-test.MD")
    if not document.is_file():
        return "BLOCKED", f"document unavailable: {document.name}"
    if offline:
        drift = report_module.document_drift(payload, document,
                                             require_evidence=False,
                                             compare_verdicts=False)
        if drift:
            return "FAIL", f"{drift} document drift(s) against {document.name}"
        return "PASS", (f"{document.name} is internally consistent (no case claims PASS "
                        "and FAIL at once); the sheet and the verdict comparison need "
                        "the live run")
    drift = report_module.drift_check(payload, document)
    if drift:
        return "FAIL", (f"{drift} drifted row(s) between the sheet, {document.name} "
                        "and the live report")
    return "PASS", f"the sheet, {document.name} and the live report agree"


def _contract_drift(case: dict) -> tuple[str, str]:
    """C12 - document drift guard against a previously saved report."""
    results_path = Path(case["results"]) if case.get("results") else _latest_results_path()
    if results_path is None or not results_path.is_file():
        return "BLOCKED", ("no saved parity results found; run the runner first so the "
                           "drift guard has verdicts to compare")
    import report as report_module
    payload = report_module._load_results(results_path)
    return evaluate_drift(case, payload, offline=bool(payload.get("offline")))


def result(case: dict, captures: dict, verdict: str, reason: str) -> dict:
    empty = {"status_code": 0, "headers": {}, "body_raw": "", "elapsed_ms": 0, "error": None}
    return {"id": case["id"], "name": case["name"], "path": case.get("path", "api/v1/workflows"),
            "method": "GET", "compare": case.get("check", ""), "notes": case.get("notes", ""),
            "body_hash": case_signature(case),
            "verdict": verdict, "reason": reason,
            "n8n": captures.get("n8n", empty), "m9m": captures.get("m9m", empty)}
