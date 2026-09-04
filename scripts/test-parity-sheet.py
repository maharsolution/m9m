#!/usr/bin/env python3
"""
Drive the 6 n8n-vs-m9m webhook parity test cases from the
'Validasi N8N - M9M' Google Sheet:

  https://docs.google.com/spreadsheets/d/1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ

For each case we POST the documented request to n8n:5678 and to m9m:8080,
then write `(status_code, body, headers)` to:
  tmp/validation-<YYYY-MM-DD>/case_<N>.json
and append a verdict row to:
  tmp/validation-<YYYY-MM-DD>/summary.md

The script is intentionally stdlib-only (it shells out to `curl.exe`,
which is already on Windows and is used by `scripts/verify_webhook_parity.py`).

Usage:
  python scripts/test-parity-sheet.py [--out tmp/validation-2026-09-04] [--phase pre|post]
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

# -- Configuration ----------------------------------------------------------

N8N_HOST = "187.77.113.218"
N8N_PORT = 5678
M9M_HOST = "187.77.113.218"
M9M_PORT = 8080

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_OUT = ROOT / "tmp" / f"validation-{_dt.date.today().isoformat()}"


# -- Test case definitions (mirror the Google Sheet) -------------------------

CASES: list[dict[str, Any]] = [
    {
        "no": 1,
        "description": "webhook use code node Flow :Webhook --> Code (java script) return mynewfield : 1",
        "path": "webhook_code",
        "method": "POST",
        "content_type": "application/json",
        "body": '{"variable": "1"}',
        "extra_headers": [],
        "expected_status": 200,
        "note": "n8n echoes trigger JSON + sets myNewField=1; m9m must match.",
    },
    {
        "no": 2,
        "description": "Webhooke use switch node Flow : Webhook -> switch(route) with condition",
        "path": "simple_webhook_3",
        "method": "POST",
        "content_type": "application/json",
        "body": '{"variable": "1"}',
        "extra_headers": [],
        "expected_status": 200,
        "note": "Switch routes payload; response body should be {response: OK}.",
    },
    {
        "no": 3,
        "description": "Webhook use http node with basic auth Flow : Webhook --> http code (basic auth)",
        "path": "webhook_callrest",
        "method": "POST",
        "content_type": "application/json",
        "body": '{"variable": "1"}',
        "extra_headers": [],
        "expected_status": 200,
        "note": "Workflow calls an HTTP node with basic auth; m9m must return OK.",
    },
    {
        "no": 4,
        "description": "Webhook use set node Flow : Webhook -> Set",
        "path": "simple_webhook_2",
        "method": "POST",
        "content_type": "application/json",
        "body": '{"variable": "1"}',
        "extra_headers": [],
        "expected_status": 200,
        "note": "Set node passthrough; response body keys should match.",
    },
    {
        "no": 5,
        "description": "Webhook use set node (formula/helper/calucation)",
        "path": "bocahtuanakal",
        "method": "POST",
        "content_type": "application/json",
        "body": '{"varA": 1, "varB": 2}',
        "extra_headers": [
            "Authorization: Basic YWRtaW46YWRtaW4xMjM=",  # admin:admin123
        ],
        "expected_status": 200,
        "note": "Set node computes total=3; basic-auth header required by workflow.",
    },
    {
        "no": 6,
        "description": "Webhook use xml node (xml to json & json to xml)",
        "path": "webhook_xml",
        "method": "POST",
        "content_type": "application/xml",
        "body": (
            '<?xml version="1.0" encoding="UTF-8"?>'
            '<buku id="001">'
            "<judul>Belajar Pemrograman Web</judul>"
            "<penulis>John Doe</penulis>"
            "<tahun>2026</tahun>"
            '<harga mataUang="IDR">120000</harga>'
            "</buku>"
        ),
        "extra_headers": [],
        "expected_status": 200,
        "note": "XML round-trip; response should be valid XML.",
    },
]


# -- Helpers -----------------------------------------------------------------

def _run_curl(host: str, port: int, case: dict[str, Any]) -> dict[str, Any]:
    """Invoke curl.exe and capture status, headers, body."""
    url = f"http://{host}:{port}/webhook/{case['path']}"
    cmd: list[str] = [
        "curl.exe",
        "-sS",
        "-m", "15",
        "-X", case["method"],
        url,
        "-H", f"Content-Type: {case['content_type']}",
        "-d", case["body"],
    ]
    for h in case.get("extra_headers", []):
        cmd.extend(["-H", h])

    proc = subprocess.run(
        cmd,
        capture_output=True,
        text=False,  # bytes for body
        timeout=20,
    )

    body_bytes = proc.stdout or b""
    body_text = body_bytes.decode("utf-8", errors="replace").strip()

    # Try to extract status line from curl's stderr (%{http_code} is not
    # requested above on purpose because we want the *body*; so probe via
    # `curl -i` separately if needed). For now, do a status-only request.
    status_cmd = list(cmd)
    status_cmd[status_cmd.index("-sS")] = "-sS"  # keep silent
    # Re-issue with -o /dev/null -w to grab the status code without touching body.
    status_only = list(cmd)
    # remove -d body to avoid double-send; instead reuse -w
    status_only.extend(["-o", os.devnull, "-w", "%{http_code}"])
    status_proc = subprocess.run(status_only, capture_output=True, text=True, timeout=20)
    status_code = int(status_proc.stdout.strip() or "0")

    # Normalize JSON for byte-equal comparison.
    normalized: Any = body_text
    if case["content_type"] == "application/json":
        try:
            normalized = json.dumps(json.loads(body_text), indent=2, sort_keys=True)
        except Exception:
            normalized = body_text

    return {
        "host": host,
        "port": port,
        "url": url,
        "status_code": status_code,
        "body_raw": body_text,
        "body_normalized": normalized if isinstance(normalized, str) else body_text,
        "curl_exit": proc.returncode,
        "curl_stderr": (proc.stderr or b"").decode("utf-8", errors="replace"),
    }


def _classify(case_no: int, n8n: dict[str, Any], m9m: dict[str, Any]) -> str:
    """Decide PASS / FAIL given the two responses.

    Heuristic:
    - Both endpoints must return 2xx with a non-empty body for PASS.
    - For JSON bodies we additionally require semantic JSON equality
      after normalising fields that legitimately differ between the
      two servers (host header, webhookUrl — these are env-driven,
      not a parity bug).
    - For XML bodies we require whitespace-collapsed structural equality.
    """
    if n8n["status_code"] != m9m["status_code"]:
        return "FAIL"
    if not (200 <= n8n["status_code"] < 300):
        return "FAIL"
    if not n8n["body_raw"] or not m9m["body_raw"]:
        return "FAIL"
    if case_no == 6:
        # XML: collapse whitespace and compare structural equality.
        def _norm(x: str) -> str:
            return re.sub(r"\s+", " ", x).strip()
        if _norm(n8n["body_raw"]) != _norm(m9m["body_raw"]):
            return "FAIL"
        return "PASS"
    # Default: JSON semantic equality with environment-noise filter.
    return "PASS" if _semantic_json_equal(case_no, n8n["body_raw"], m9m["body_raw"]) else "FAIL"


# Fields that legitimately differ between n8n and m9m because they are
# bound to the listener (host header) or to operator-configured env
# vars (webhookUrl). Filtering them out lets us detect real parity
# bugs while tolerating well-known runtime differences.
_ENV_NOISE_FIELDS = {
    "headers.host",
    "webhookUrl",
}


def _semantic_json_equal(case_no: int, n8n_raw: str, m9m_raw: str) -> bool:
    """Compare two JSON bodies after stripping environment-driven noise.

    Returns True if the bodies are semantically equivalent once the
    known-different fields are removed, False otherwise. Parsing
    failures fall back to raw string comparison so we don't silently
    mask a different kind of regression.
    """
    try:
        a = json.loads(n8n_raw)
        b = json.loads(m9m_raw)
    except Exception:
        return n8n_raw == m9m_raw
    _scrub(a)
    _scrub(b)
    return a == b


def _scrub(node: Any) -> None:
    """Recursively delete environment-driven keys in-place.

    The noise-field set uses dotted paths so we can target nested
    keys (e.g. `headers.host`) without nuking the entire `headers`
    map. The traversal tracks the dotted path from the root and
    compares against `_ENV_NOISE_FIELDS` so multi-segment paths work
    the way the operator would expect.
    """
    _scrub_with_prefix(node, "")


def _scrub_with_prefix(node: Any, prefix: str) -> None:
    if isinstance(node, dict):
        for key in list(node.keys()):
            full_path = f"{prefix}.{key}" if prefix else key
            if full_path in _ENV_NOISE_FIELDS or key in _ENV_NOISE_FIELDS:
                node.pop(key, None)
                continue
            _scrub_with_prefix(node[key], full_path)
    elif isinstance(node, list):
        for item in node:
            _scrub_with_prefix(item, prefix)


def _write_case(out_dir: Path, case: dict[str, Any], n8n: dict[str, Any],
                m9m: dict[str, Any], verdict: str) -> None:
    (out_dir / f"case_{case['no']}.json").write_text(
        json.dumps(
            {
                "no": case["no"],
                "description": case["description"],
                "verdict": verdict,
                "expected_status": case["expected_status"],
                "n8n": n8n,
                "m9m": m9m,
            },
            indent=2,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )


def _write_summary(out_dir: Path, rows: list[dict[str, Any]], phase: str) -> None:
    md = out_dir / "summary.md"
    header = f"# Webhook Parity Summary ({phase})\n\n"
    header += f"Date: {_dt.datetime.now().isoformat(timespec='seconds')}\n\n"
    header += (
        "| # | Path | n8n status | m9m status | Verdict |\n"
        "|---|------|------------|------------|---------|\n"
    )
    body = ""
    for row in rows:
        body += (
            f"| {row['no']} | `{row['path']}` | {row['n8n_status']} | "
            f"{row['m9m_status']} | **{row['verdict']}** |\n"
        )
    overall = "ALL PASS" if all(r["verdict"] == "PASS" for r in rows) else "FAILURES PRESENT"
    md.write_text(header + body + f"\nOverall: **{overall}**\n", encoding="utf-8")


# -- Entry point -------------------------------------------------------------

def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default=str(DEFAULT_OUT))
    parser.add_argument("--phase", default="pre", choices=("pre", "post"))
    args = parser.parse_args(argv)

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    print(f"Output directory: {out_dir}")
    print(f"Phase           : {args.phase}")
    print(f"n8n endpoint    : http://{N8N_HOST}:{N8N_PORT}")
    print(f"m9m endpoint    : http://{M9M_HOST}:{M9M_PORT}")
    print()

    rows: list[dict[str, Any]] = []
    failures = 0
    for case in CASES:
        print(f"[case {case['no']}] {case['path']}")
        try:
            n8n = _run_curl(N8N_HOST, N8N_PORT, case)
            m9m = _run_curl(M9M_HOST, M9M_PORT, case)
        except subprocess.TimeoutExpired as exc:
            print(f"  ! timeout: {exc}")
            failures += 1
            continue

        verdict = _classify(case["no"], n8n, m9m)
        if verdict != "PASS":
            failures += 1

        _write_case(out_dir, case, n8n, m9m, verdict)
        print(f"  n8n HTTP {n8n['status_code']}  | m9m HTTP {m9m['status_code']}  -> {verdict}")

        rows.append(
            {
                "no": case["no"],
                "path": case["path"],
                "n8n_status": n8n["status_code"],
                "m9m_status": m9m["status_code"],
                "verdict": verdict,
            }
        )

    _write_summary(out_dir, rows, args.phase)

    print()
    print(f"Failures: {failures}/{len(CASES)}")
    return 0 if failures == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
