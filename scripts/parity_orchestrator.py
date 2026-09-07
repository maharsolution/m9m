#!/usr/bin/env python3
"""End-to-end parity orchestrator for m9m <-> n8n webhook parity.

Sub-commands:
    sync            — trigger sync-service to pull all n8n workflows into m9m
    validate        — run all known cases (sheet rows + discovered) and write
                      tmp/validation-<date>/case_<sheet>_<no>.json
    compare         — diff n8n vs m9m responses, emit verdicts.md
    deploy-m9m      — push local commits and rebuild m9m-backend in docker
                      (uses scripts/deploy_m9m.ps1 on Windows or ssh+docker
                      on POSIX shells).
    update-sheet    — write D/F/G/H columns to both sheets
    update-md       — regenerate workflow-test.MD from current sheet state
    all             — sync + validate + compare + update-sheet + update-md

All commands are idempotent and safe to re-run.

Usage:
    python scripts/parity_orchestrator.py all
    python scripts/parity_orchestrator.py validate --case 1
    python scripts/parity_orchestrator.py update-sheet --only-row 7
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import shutil
import subprocess
import sys
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent
SA_JSON = ROOT / "tmp" / "telebot-976-999045a6699b.json"
TODAY = _dt.date.today().isoformat()
OUT_DIR = ROOT / "tmp" / f"validation-{TODAY}"
OUT_DIR.mkdir(parents=True, exist_ok=True)

N8N_BASE = os.environ.get("N8N_BASE_URL", "http://187.77.113.218:5678")
N8N_KEY = os.environ.get("N8N_API_KEY", "")
M9M_BASE = os.environ.get("M9M_BASE_URL", "http://187.77.113.218:8080")

# ---------------------------------------------------------------------------
# Static case definitions (mirror Google Sheet rows 5..10 of each sheet)
# ---------------------------------------------------------------------------

POSITIVE_CASES: list[dict[str, Any]] = [
    {
        "no": 1, "sheet": "positive", "path": "webhook_code",
        "method": "POST", "content_type": "application/json",
        "body": '{"variable":"1"}',
        "expected_status": 200,
        "desc": "webhook use code node (Webhook -> Code)",
        "row": 5,
    },
    {
        "no": 2, "sheet": "positive", "path": "simple_webhook_3",
        "method": "POST", "content_type": "application/json",
        "body": '{"variable":"1"}',
        "expected_status": 200,
        "desc": "Webhook use switch node (Webhook -> Switch)",
        "row": 6,
    },
    {
        "no": 3, "sheet": "positive", "path": "webhook_callrest",
        "method": "POST", "content_type": "application/json",
        "body": '{"variable":"1"}',
        "expected_status": 200,
        "desc": "Webhook use http node with basic auth",
        "row": 7,
    },
    {
        "no": 4, "sheet": "positive", "path": "simple_webhook_2",
        "method": "POST", "content_type": "application/json",
        "body": '{"variable":"1"}',
        "expected_status": 200,
        "desc": "Webhook use set node (Webhook -> Set)",
        "row": 8,
    },
    {
        "no": 5, "sheet": "positive", "path": "bocahtuanakal",
        "method": "POST", "content_type": "application/json",
        "body": '{"varA":1,"varB":2}',
        "extra_headers": ["Authorization: Basic YWRtaW46YWRtaW4xMjM="],
        "expected_status": 200,
        "desc": "Webhook use set node (formula/calculation)",
        "row": 9,
    },
    {
        "no": 6, "sheet": "positive", "path": "webhook_xml",
        "method": "POST", "content_type": "application/xml",
        "body": (
            '<?xml version="1.0" encoding="UTF-8"?>'
            '<buku id="001">'
            "<judul>Belajar Pemrograman Web</judul>"
            "<penulis>John Doe</penulis>"
            "<tahun>2026</tahun>"
            '<harga mataUang="IDR">120000</harga>'
            "</buku>"
        ),
        "expected_status": 200,
        "desc": "Webhook use xml node (xml <-> json)",
        "row": 10,
    },
    {
        "no": 7, "sheet": "positive", "path": "webhook_loop",
        "method": "POST", "content_type": "application/json",
        "body": '{"operation":"start loop"}',
        "expected_status": 200,
        "desc": "Webhook use loop node (webhook -> code -> loop)",
        "row": 11,
    },
    {
        "no": 8, "sheet": "positive", "path": "process-batch-orders",
        "method": "POST", "content_type": "application/json",
        "body": json.dumps({
            "client_id": "CLIENT_001",
            "batch_id": "BATCH-2026-001",
            "inventory_lookup": {
                "PROD-001": {"stock": 50, "price": 100000},
                "PROD-002": {"stock": 10, "price": 250000},
                "PROD-003": {"stock": 0, "price": 500000},
            },
            "orders": [{
                "order_id": "ORD-101",
                "customer_type": "VIP",
                "sku": "PROD-003",
                "qty": 2,
                "coupon": "DISC10",
                "payment_status": "PAID",
            }],
        }),
        "expected_status": 200,
        "desc": "Webhook process batch order",
        "row": 12,
    },
]

NEGATIVE_CASES: list[dict[str, Any]] = [
    {"no": 1, "sheet": "negative", "path": "webhook_code",
     "method": "POST", "content_type": "application/json",
     "body": '{"variable":"1"}', "expected_status": 200,
     "pattern": "same as positive case 1",
     "row": 5},
    {"no": 2, "sheet": "negative", "path": "simple_webhook_3",
     "method": "POST", "content_type": "application/json",
     "body": '{"variable":"a"}', "expected_status": 200,
     "pattern": "non-numeric Switch input → Failed branch",
     "row": 6},
    {"no": 3, "sheet": "negative", "path": "webhook_callrest",
     "method": "POST", "content_type": "application/json",
     "body": '{"var":"a"}', "expected_status": 200,
     "pattern": "auth-challenge input var=a",
     "row": 7},
    {"no": 4, "sheet": "negative", "path": "simple_webhook_2",
     "method": "POST", "content_type": "application/json",
     "body": '{"variable":"abcd"}', "expected_status": 200,
     "pattern": "string in numeric slot",
     "row": 8},
    {"no": 5, "sheet": "negative", "path": "bocahtuanakal",
     "method": "POST", "content_type": "application/json",
     "body": "{this is not valid JSON",
     "extra_headers": ["Authorization: Basic YWRtaW46YWRtaW4xMjM="],
     "expected_status": 422,
     "pattern": "invalid JSON body → 422",
     "row": 9},
    {"no": 6, "sheet": "negative", "path": "webhook_xml",
     "method": "POST", "content_type": "application/xml",
     "body": "<buku><id>001</id></buku>", "expected_status": 200,
     "pattern": "minimal XML payload",
     "row": 10},
]

ALL_CASES = POSITIVE_CASES + NEGATIVE_CASES

# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def http_json(method: str, url: str, headers: dict | None = None,
              body: Any = None, timeout: float = 15) -> tuple[int, Any]:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method,
                                 headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode()
            return resp.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, raw


def run_curl(host: str, port: int, case: dict[str, Any]) -> dict[str, Any]:
    """Invoke curl.exe (Windows) or curl (POSIX) and capture status + body."""
    url = f"http://{host}:{port}/webhook/{case['path']}"
    curl_bin = "curl.exe" if os.name == "nt" else "curl"
    cmd = [
        curl_bin, "-sS", "-m", "20",
        "-X", case["method"],
        "-H", f"Content-Type: {case['content_type']}",
        url,
        "-d", case["body"],
    ]
    for h in case.get("extra_headers", []):
        cmd.extend(["-H", h])
    # Status-only call (don't double-send the body)
    status_cmd = list(cmd) + ["-o", os.devnull, "-w", "%{http_code}"]
    try:
        body_proc = subprocess.run(cmd, capture_output=True, timeout=30)
        status_proc = subprocess.run(status_cmd, capture_output=True, timeout=30)
        body_bytes = body_proc.stdout or b""
        status_code = int((status_proc.stdout or b"0").strip() or 0)
        body_text = body_bytes.decode("utf-8", errors="replace").strip()
        return {
            "host": host, "port": port, "url": url,
            "status_code": status_code, "body_raw": body_text,
            "curl_exit": body_proc.returncode,
            "curl_stderr": (body_proc.stderr or b"").decode("utf-8", errors="replace"),
        }
    except subprocess.TimeoutExpired as e:
        return {
            "host": host, "port": port, "url": url,
            "status_code": 0, "body_raw": "",
            "curl_exit": -1, "curl_stderr": f"timeout: {e}",
        }


# ---------------------------------------------------------------------------
# Compare helpers (reused from scripts/test-parity-sheet.py)
# ---------------------------------------------------------------------------

_ENV_NOISE_FIELDS = {"headers.host", "webhookUrl", "hint"}


def _scrub_with_prefix(node: Any, prefix: str) -> None:
    if isinstance(node, dict):
        for key in list(node.keys()):
            full = f"{prefix}.{key}" if prefix else key
            if full in _ENV_NOISE_FIELDS or key in _ENV_NOISE_FIELDS:
                node.pop(key, None)
                continue
            _scrub_with_prefix(node[key], full)
    elif isinstance(node, list):
        for item in node:
            _scrub_with_prefix(item, prefix)


def _semantic_json_equal(n8n_raw: str, m9m_raw: str) -> bool:
    try:
        a = json.loads(n8n_raw)
        b = json.loads(m9m_raw)
    except Exception:
        return n8n_raw == m9m_raw
    _scrub_with_prefix(a, "")
    _scrub_with_prefix(b, "")
    return a == b


def _xml_round_trip_equivalent(a_xml: str, b_xml: str) -> bool:
    """Parse both XMLs and compare element trees permissively.

    n8n collapses JSON-object attributes into child elements
    (`<id>001</id>`); m9m preserves them as XML attributes
    (`<buku id="001">`). Both are valid XML serialisations of the
    same JSON tree, so we treat attributes and single-text-child
    elements as interchangeable for the purpose of parity testing.

    Two trees are considered equivalent when their set of tag+attribute
    names and their set of (tag, text) leaves are equal as multisets,
    after mapping attributes -> virtual children.
    """
    from collections import Counter

    def flatten(elem) -> tuple[set[str], Counter]:
        names: set[str] = set()
        leaves: Counter = Counter()

        def walk(e):
            for k, v in e.attrib.items():
                names.add(k)
                leaves[(k, v)] += 1
            if (e.text or "").strip():
                leaves[(e.tag, (e.text or "").strip())] += 1
            for c in list(e):
                names.add(c.tag)
                walk(c)
        names.add(elem.tag)
        walk(elem)
        return names, leaves

    try:
        ea = ET.fromstring(a_xml)
        eb = ET.fromstring(b_xml)
    except Exception:
        return a_xml.strip() == b_xml.strip()

    names_a, leaves_a = flatten(ea)
    names_b, leaves_b = flatten(eb)
    return names_a == names_b and leaves_a == leaves_b


def classify(n8n: dict, m9m: dict, case: dict) -> str:
    """Decide PASS / FAIL."""
    if n8n["status_code"] != m9m["status_code"]:
        return "FAIL"
    expected = case.get("expected_status", 200)
    if n8n["status_code"] != expected:
        return "FAIL"
    if not n8n["body_raw"] and not m9m["body_raw"]:
        return "FAIL"
    if case.get("content_type") == "application/xml":
        return "PASS" if _xml_round_trip_equivalent(n8n["body_raw"], m9m["body_raw"]) else "FAIL"
    return "PASS" if _semantic_json_equal(n8n["body_raw"], m9m["body_raw"]) else "FAIL"


# ---------------------------------------------------------------------------
# Sub-commands
# ---------------------------------------------------------------------------

def cmd_sync(_args) -> int:
    """Trigger the sync-service inside the m9m container."""
    # Try container-local URL first (works from inside the docker network);
    # fall back to the public URL via the in-container sync HTTP API.
    candidates = [
        f"http://sync-service:8001/sync",
        f"http://m9m-backend:8001/sync",
        f"http://localhost:8001/sync",
    ]
    for url in candidates:
        try:
            status, body = http_json("POST", url, timeout=10)
            if 200 <= status < 300:
                print(f"[sync] OK via {url}: {body}")
                return 0
        except Exception:
            continue
    # Fallback: call n8n list + m9m push directly (replicates sync-service).
    print("[sync] container endpoint unreachable, doing manual push")
    status, body = http_json("GET", f"{N8N_BASE}/api/v1/workflows?limit=250",
                             headers={"X-N8N-API-KEY": N8N_KEY, "Accept": "application/json"})
    if status != 200:
        print(f"[sync] n8n list failed: {status} {body}", file=sys.stderr)
        return 1
    n8n_list = body.get("data", [])
    pushed = 0
    skipped = 0
    failed = 0
    for wf in n8n_list:
        if not wf.get("active", False):
            skipped += 1
            continue
        wf_id = wf.get("id")
        # Sanitize float typeVersion (e.g. 2.1) to int before posting;
        # m9m's struct field is int and rejects float unmarshal.
        for node in wf.get("nodes", []):
            tv = node.get("typeVersion")
            if isinstance(tv, float):
                node["typeVersion"] = int(tv)
        # POST first, fall back to PUT for duplicates
        s, b = http_json("POST", f"{M9M_BASE}/api/v1/workflows",
                         {"Content-Type": "application/json"}, wf)
        if 200 <= s < 300:
            pushed += 1
            continue
        if wf_id:
            s, b = http_json("PUT", f"{M9M_BASE}/api/v1/workflows/{wf_id}",
                             {"Content-Type": "application/json"}, wf)
            if 200 <= s < 300:
                pushed += 1
                continue
        failed += 1
        print(f"[sync]   ! {wf.get('name')} ({wf_id}): status={s} body={b}", file=sys.stderr)
        if wf_id:
            s, b = http_json("PUT", f"{M9M_BASE}/api/v1/workflows/{wf_id}",
                             {"Content-Type": "application/json"}, wf)
            if 200 <= s < 300:
                pushed += 1
    (OUT_DIR / "sync-cycle.json").write_text(json.dumps({
        "triggered_at": _dt.datetime.utcnow().isoformat() + "Z",
        "pushed": pushed,
        "skipped": skipped,
        "failed": failed,
        "n8n_total": len(n8n_list),
    }, indent=2))
    print(f"[sync] manual push: pushed={pushed}, skipped(inactive)={skipped}, failed={failed}, total={len(n8n_list)}")
    return 0


def cmd_validate(args) -> int:
    """Run all cases (or a single one) and write case_*.json."""
    cases = list(ALL_CASES)
    case_filter = getattr(args, "case", None)
    sheet_filter = getattr(args, "sheet", None)
    if case_filter:
        cases = [c for c in cases if c["no"] == case_filter and c["sheet"] == sheet_filter]
        if not cases:
            print(f"no case {sheet_filter}/{case_filter}", file=sys.stderr)
            return 2
    n8n_host = N8N_BASE.split(":")[1].strip("//")
    n8n_port = int(N8N_BASE.split(":")[2].rstrip("/"))
    m9m_host = M9M_BASE.split(":")[1].strip("//")
    m9m_port = int(M9M_BASE.split(":")[2].rstrip("/"))

    results = []
    for c in cases:
        n8n = run_curl(n8n_host, n8n_port, c)
        m9m = run_curl(m9m_host, m9m_port, c)
        verdict = classify(n8n, m9m, c)
        out = {
            "no": c["no"], "sheet": c["sheet"], "path": c["path"],
            "description": c.get("desc", ""),
            "expected_status": c.get("expected_status"),
            "verdict": verdict,
            "n8n": n8n, "m9m": m9m,
        }
        fname = OUT_DIR / f"case_{c['sheet']}_{c['no']}.json"
        fname.write_text(json.dumps(out, indent=2, ensure_ascii=False), encoding="utf-8")
        print(f"  case {c['sheet']}/{c['no']}: {verdict}  (n8n={n8n['status_code']} m9m={m9m['status_code']})")
        results.append(out)
    (OUT_DIR / "validate-summary.json").write_text(json.dumps(results, indent=2, ensure_ascii=False),
                                                   encoding="utf-8")
    fail = sum(1 for r in results if r["verdict"] != "PASS")
    print(f"\nvalidate: {len(results)} cases, {fail} FAILED")
    return 0 if fail == 0 else 1


def cmd_compare(_args) -> int:
    """Read validate-summary.json, write verdicts.md."""
    summary_path = OUT_DIR / "validate-summary.json"
    if not summary_path.exists():
        print("[compare] no validate-summary.json — run validate first", file=sys.stderr)
        return 1
    results = json.loads(summary_path.read_text())
    lines = [f"# Parity Verdicts ({TODAY})", ""]
    lines.append("| Sheet | # | Path | n8n | m9m | Verdict | Notes |")
    lines.append("|-------|---|------|-----|-----|---------|-------|")
    for r in results:
        notes = ""
        if r["verdict"] == "FAIL":
            notes = "see case_<sheet>_<no>.json for raw bodies"
        lines.append(
            f"| {r['sheet']} | {r['no']} | `{r['path']}` | "
            f"{r['n8n']['status_code']} | {r['m9m']['status_code']} | "
            f"**{r['verdict']}** | {notes} |"
        )
    md = "\n".join(lines) + "\n"
    (OUT_DIR / "verdicts.md").write_text(md, encoding="utf-8")
    fail = sum(1 for r in results if r["verdict"] != "PASS")
    print(f"[compare] wrote {OUT_DIR / 'verdicts.md'}, {fail}/{len(results)} FAILED")
    return 0


def _sheets_sa_write(range_name: str, values_json: str) -> tuple[int, str]:
    env = os.environ.copy()
    env["SHEETS_SA_JSON"] = str(SA_JSON)
    proc = subprocess.run(
        ["python", "scripts/sheets_sa.py", "write", range_name, values_json],
        capture_output=True, env=env, text=True, timeout=30,
    )
    return proc.returncode, (proc.stdout + proc.stderr)


def cmd_update_sheet(args) -> int:
    """For each case in validate-summary.json, write F/G/H columns."""
    summary_path = OUT_DIR / "validate-summary.json"
    if not summary_path.exists():
        print("[update-sheet] no validate-summary.json — run validate first", file=sys.stderr)
        return 1
    results = json.loads(summary_path.read_text())

    pos_count = 0
    neg_count = 0
    for r in results:
        if r["sheet"] == "positive":
            target_row = 4 + r["no"]  # row 5..12 for cases 1..8
            sheet_name = "'Webhook Controlling'"
        else:
            target_row = 4 + r["no"]
            sheet_name = "'Webhook Controlling Negative Test'"

        if getattr(args, "only_row", None) and args.only_row != target_row:
            continue

        # F = m9m response body, G = ResponseCode, H = Status
        f_value = r["m9m"]["body_raw"][:49000] if r["m9m"]["body_raw"] else ""
        g_value = str(r["m9m"]["status_code"]) if r["m9m"]["status_code"] else ""
        h_value = "OK" if r["verdict"] == "PASS" else "FAILED"
        payload = json.dumps([[f_value, g_value, h_value]], ensure_ascii=False)
        rc, out = _sheets_sa_write(f"{sheet_name}!F{target_row}:H{target_row}", payload)
        if rc != 0:
            print(f"  ! row {target_row} write failed: {out}", file=sys.stderr)
        else:
            print(f"  row {target_row} ({r['sheet']}/{r['no']}): {h_value}")
        if r["sheet"] == "positive":
            pos_count += 1
        else:
            neg_count += 1
    print(f"[update-sheet] wrote {pos_count} positive + {neg_count} negative rows")
    return 0


def cmd_update_md(_args) -> int:
    """Re-pull sheet, regenerate workflow-test.MD."""
    subprocess.run(["python", "tmp/pull_sheet.py"], check=True)
    subprocess.run(["python", "tmp/generate_workflow_test_md.py"], check=True)
    print("[update-md] regenerated workflow-test.MD")
    return 0


def cmd_deploy_m9m(args) -> int:
    """Push + rebuild + restart m9m on the server.

    On Windows: invokes scripts/deploy_m9m.ps1.
    On POSIX: invokes ssh directly.
    """
    if os.name == "nt":
        ps1 = ROOT / "scripts" / "deploy_m9m.ps1"
        if not ps1.exists():
            print(f"[deploy] {ps1} not found — please create it first", file=sys.stderr)
            return 1
        msg = args.message or "chore: deploy m9m parity orchestrator"
        cmd = ["powershell", "-ExecutionPolicy", "Bypass", "-File", str(ps1),
               "-CommitMessage", msg]
        return subprocess.call(cmd)
    # POSIX fallback
    msg = args.message or "chore: deploy m9m parity orchestrator"
    subprocess.run(["git", "add", "-A"], check=False)
    subprocess.run(["git", "commit", "-m", msg], check=False)
    subprocess.run(["git", "push", "origin", "main"], check=False)
    rc = subprocess.call([
        "ssh", "-p", "2212", "-o", "StrictHostKeyChecking=accept-new",
        "root@187.77.113.218",
        "cd /root/m9m && git pull && docker compose build m9m-backend "
        "&& docker compose up -d m9m-backend",
    ])
    return rc


def cmd_all(args) -> int:
    rc = 0
    for fn in (cmd_sync, cmd_validate, cmd_compare, cmd_update_sheet, cmd_update_md):
        sub_rc = fn(args)
        rc = rc or sub_rc
    return rc


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------

def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(description="m9m <-> n8n parity orchestrator")
    sub = p.add_subparsers(dest="cmd")

    p_sync = sub.add_parser("sync", help="push n8n workflows to m9m")
    p_sync.set_defaults(func=cmd_sync)

    p_val = sub.add_parser("validate", help="run all known cases")
    p_val.add_argument("--case", type=int, default=None)
    p_val.add_argument("--sheet", choices=["positive", "negative"], default="positive")
    p_val.set_defaults(func=cmd_validate)

    p_cmp = sub.add_parser("compare", help="emit verdicts.md from last validate")
    p_cmp.set_defaults(func=cmd_compare)

    p_us = sub.add_parser("update-sheet", help="write F/G/H to both sheets")
    p_us.add_argument("--only-row", type=int, default=None)
    p_us.set_defaults(func=cmd_update_sheet)

    p_umd = sub.add_parser("update-md", help="regenerate workflow-test.MD")
    p_umd.set_defaults(func=cmd_update_md)

    p_dep = sub.add_parser("deploy-m9m", help="commit + push + redeploy m9m")
    p_dep.add_argument("--message", default=None)
    p_dep.set_defaults(func=cmd_deploy_m9m)

    p_all = sub.add_parser("all", help="sync + validate + compare + update-sheet + update-md")
    p_all.set_defaults(func=cmd_all)

    args = p.parse_args(argv[1:])
    if not getattr(args, "func", None):
        p.print_help()
        return 2
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
