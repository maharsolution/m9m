"""Import a workflow from the n8n server into a local m9m and export it back.

Local performance runs must not depend on a hand-copied JSON file: the
workflow that is measured has to be the one the server actually runs, and
it has to be the workflow the local m9m actually stores. This script
therefore does exactly three steps:

1. read the workflow from the n8n server (read-only REST call),
2. import it into a local m9m (``POST /api/v1/workflows/import``),
3. export it from the local m9m again (``GET /api/v1/workflows/{id}/export``)

The exported document is the artifact a local test should execute. With
``--reimport`` the script performs a second import/export round trip on
the exported document and reports whether the round trip is stable.

Secrets are never printed: only key names and boolean presence are shown.
The n8n API key is only ever sent to the n8n host, the m9m API key only
to the m9m host.

Usage:
    python scripts/perf/import_export_local.py \
        --name-substr balance --local-url http://127.0.0.1:18099 \
        --out tmp/local-import-export --activate
"""
from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
ENV_FILE = ROOT / ".env"

# Only these keys are read from .env. N8N_API_KEY / M9M_API_KEY are
# secrets, the *URL keys are not, but they are still never printed.
SECRET_KEYS = ("N8N_API_KEY", "M9M_API_KEY")
URL_KEYS = ("N8N_BASE_URL", "M9M_BASE_URL", "N8n_URL", "M9M_URL")

N8N_HEADER = "X-N8N-API-KEY"
M9M_HEADER = "X-API-Key"


def read_env(path: Path) -> dict[str, str]:
    """Read the handful of keys this script needs from the repo .env."""
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for raw in path.read_text(encoding="utf-8-sig").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, _, value = line.partition("=")
        name = name.strip()
        if name.startswith("export "):
            name = name[len("export "):].strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        if name in SECRET_KEYS or name in URL_KEYS:
            values[name] = value
    return values


def request(url: str, *, method: str = "GET", body: bytes | None = None,
            headers: dict[str, str] | None = None, timeout: float = 60.0):
    """Perform one HTTP request and return (status, payload_bytes)."""
    req = urllib.request.Request(url, data=body, method=method)
    for key, value in (headers or {}).items():
        if value:
            req.add_header(key, value)
    if body is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()


def n8n_headers(env: dict[str, str]) -> dict[str, str]:
    key = env.get("N8N_API_KEY", "")
    return {N8N_HEADER: key} if key else {}


def m9m_headers(env: dict[str, str]) -> dict[str, str]:
    key = env.get("M9M_API_KEY", "")
    return {M9M_HEADER: key} if key else {}


def list_n8n_workflows(base: str, env: dict[str, str]) -> list[dict]:
    """Return the n8n workflow index (id + name only)."""
    status, payload = request(f"{base}/api/v1/workflows?limit=250",
                              headers=n8n_headers(env))
    if status != 200:
        raise SystemExit(f"n8n workflow list failed: HTTP {status}")
    data = json.loads(payload)
    items = data.get("data") if isinstance(data, dict) else data
    return [{"id": item.get("id", ""), "name": item.get("name", "")}
            for item in (items or [])]


def fetch_n8n_workflow(base: str, env: dict[str, str], workflow_id: str) -> dict:
    status, payload = request(f"{base}/api/v1/workflows/{workflow_id}",
                              headers=n8n_headers(env))
    if status != 200:
        raise SystemExit(f"n8n workflow fetch failed: HTTP {status}")
    return json.loads(payload)


def import_workflow(local: str, env: dict[str, str], document: dict,
                    activate: bool) -> dict:
    """Import one workflow document into the local m9m."""
    url = f"{local}/api/v1/workflows/import"
    if activate:
        url += "?active=fromJson"
    body = json.dumps(document).encode("utf-8")
    status, payload = request(url, method="POST", body=body,
                              headers=m9m_headers(env))
    data = json.loads(payload) if payload else {}
    if status not in (200, 201):
        raise SystemExit(f"local import failed: HTTP {status}: {data}")
    return data


def export_workflow(local: str, env: dict[str, str], workflow_id: str) -> dict:
    status, payload = request(f"{local}/api/v1/workflows/{workflow_id}/export",
                              headers=m9m_headers(env))
    if status != 200:
        raise SystemExit(f"local export failed: HTTP {status}")
    return json.loads(payload)


def node_types(document: dict) -> list[str]:
    return sorted(str(node.get("type", "")) for node in document.get("nodes", []))


def connection_count(document: dict) -> int:
    connections = document.get("connections") or {}
    return sum(len(value) for value in connections.values())


def describe(document: dict) -> dict:
    return {
        "name": document.get("name", ""),
        "nodes": len(document.get("nodes", []) or []),
        "connections": connection_count(document),
        "types": node_types(document),
        "active": bool(document.get("active")),
    }


def diff_types(source: list[str], target: list[str]) -> tuple[list[str], list[str]]:
    missing = sorted(set(source) - set(target))
    extra = sorted(set(target) - set(source))
    return missing, extra


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workflow-id", default="", help="n8n workflow id")
    parser.add_argument("--name-substr", default="",
                        help="substring of the n8n workflow name")
    parser.add_argument("--n8n-url", default="", help="n8n base url")
    parser.add_argument("--local-url", default="http://127.0.0.1:18099",
                        help="local m9m base url")
    parser.add_argument("--out", default="tmp/local-import-export",
                        help="directory for the exported artifacts")
    parser.add_argument("--activate", action="store_true",
                        help="honour the n8n active flag on import")
    parser.add_argument("--reimport", action="store_true",
                        help="import the exported document once more and compare")
    args = parser.parse_args()

    env = read_env(ENV_FILE)
    presence = {key: bool(env.get(key)) for key in SECRET_KEYS}
    print("credentials: " + ", ".join(
        f"{key}={'set' if ok else 'absent'}" for key, ok in sorted(presence.items())))

    n8n_base = (args.n8n_url or env.get("N8N_BASE_URL")
                or env.get("N8n_URL") or "http://187.77.113.218:5678").rstrip("/")
    local_base = args.local_url.rstrip("/")

    workflow_id = args.workflow_id
    if not workflow_id:
        if not args.name_substr:
            raise SystemExit("either --workflow-id or --name-substr is required")
        index = list_n8n_workflows(n8n_base, env)
        needle = args.name_substr.lower()
        matches = [item for item in index if needle in item["name"].lower()]
        if not matches:
            names = ", ".join(sorted(item["name"] for item in index)[:40])
            raise SystemExit(f"no n8n workflow matches {args.name_substr!r}; "
                             f"available: {names}")
        print(f"candidates: {len(matches)} workflow(s) match {args.name_substr!r}")
        for item in matches[:10]:
            print(f"  - {item['id']}  {item['name']}")
        workflow_id = matches[0]["id"]
        print(f"selected: {workflow_id} ({matches[0]['name']})")

    started = time.perf_counter()
    source = fetch_n8n_workflow(n8n_base, env, workflow_id)
    fetch_ms = (time.perf_counter() - started) * 1000

    started = time.perf_counter()
    summary = import_workflow(local_base, env, source, args.activate)
    import_ms = (time.perf_counter() - started) * 1000

    items = summary.get("workflows") or []
    if not items:
        raise SystemExit(f"local import reported no workflow: {summary}")
    local_id = items[0].get("id", "")
    warnings = items[0].get("warnings") or []

    started = time.perf_counter()
    exported = export_workflow(local_base, env, local_id)
    export_ms = (time.perf_counter() - started) * 1000

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)
    source_path = out_dir / f"n8n-{workflow_id}.json"
    export_path = out_dir / f"m9m-local-{local_id}.json"
    source_path.write_text(json.dumps(source, indent=2), encoding="utf-8")
    export_path.write_text(json.dumps(exported, indent=2), encoding="utf-8")

    src = describe(source)
    dst = describe(exported)
    missing, extra = diff_types(src["types"], dst["types"])

    print(f"\nimport : {src['nodes']} nodes -> local id {local_id} "
          f"(fetch {fetch_ms:.0f} ms, import {import_ms:.0f} ms, "
          f"export {export_ms:.0f} ms)")
    print(f"source : {src['name']} nodes={src['nodes']} "
          f"connections={src['connections']} active={src['active']}")
    print(f"export : {dst['name']} nodes={dst['nodes']} "
          f"connections={dst['connections']} active={dst['active']}")
    if missing:
        print(f"node types missing after export: {missing}")
    if extra:
        print(f"node types added by export: {extra}")
    if warnings:
        print(f"import warnings ({len(warnings)}):")
        for warning in warnings[:10]:
            print(f"  - {warning}")
    print(f"artifacts: {source_path}  {export_path}")

    stable = True
    if args.reimport:
        second = import_workflow(local_base, env, exported, args.activate)
        second_id = (second.get("workflows") or [{}])[0].get("id", "")
        reexported = export_workflow(local_base, env, second_id)
        src2, dst2 = describe(exported), describe(reexported)
        missing2, extra2 = diff_types(src2["types"], dst2["types"])
        stable = (src2["nodes"] == dst2["nodes"]
                  and src2["connections"] == dst2["connections"]
                  and not missing2 and not extra2)
        second_path = out_dir / f"m9m-local-round2-{second_id}.json"
        second_path.write_text(json.dumps(reexported, indent=2), encoding="utf-8")
        print(f"\nround 2: id={second_id} nodes={dst2['nodes']} "
              f"connections={dst2['connections']} stable={stable}")
        if missing2 or extra2:
            print(f"  missing: {missing2}  extra: {extra2}")
        print(f"artifacts: {second_path}")

    return 0 if stable else 1


if __name__ == "__main__":
    sys.exit(main())
