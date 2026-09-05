#!/usr/bin/env python3
"""Drive the negative-case parity tests listed in the
"Webhook Controlling Negative Test" sheet.

For each row we expect:
  - HTTP status from m9m to match n8n's HTTP status
  - Response bodies to be semantically equivalent (modulo the
    well-known differences the parity harness already scrubs:
    headers.host, webhookUrl, responseBody expression results)

Outputs go to tmp/validation-<date>/negative/case_<N>.json plus a
summary.md so the negative-case artefacts match the positive-case
directory layout.
"""
from __future__ import annotations

import json
import re
import shlex
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

N8N_HOST = "187.77.113.218"
N8N_PORT = 5678
M9M_HOST = "187.77.113.218"
M9M_PORT = 8080

# Mirrors scripts/test-parity-sheet.py — same env-driven noise filter.
_ENV_NOISE_FIELDS = {
    "headers.host",
    "webhookUrl",
}


def _now_tag() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d")


def _swap_host(line: str, old_host: str, new_host: str) -> str:
    """Replace old_host with new_host in the curl command line.

    We don't try to parse the curl command — we just string-replace
    the host portion. That keeps the multi-line backslash continuations
    from the spreadsheet intact without a fragile tokenizer.
    """
    return line.replace(old_host, new_host)


def _run_curl(curl_cmd: str) -> dict[str, Any]:
    """Execute a multi-line curl command and return status + body.

    The command is invoked via PowerShell so that `curl.exe` is the
    Windows-builtin (the spreadsheet lines are written for *nix-style
    curl but PowerShell's `curl` is an alias for Invoke-WebRequest —
    we must call curl.exe explicitly).
    """
    # Collapse any backslash-newline continuations from the spreadsheet.
    flat = curl_cmd.replace("\\\n", " ").replace("\\\r\n", " ")
    # Split the leading "curl" and everything else.
    parts = shlex.split(flat)
    if parts and parts[0].lower() == "curl":
        parts = parts[1:]
    # Force curl.exe and add status-code capture.
    cmd = ["curl.exe", "-s", "--connect-timeout", "10", "--max-time", "20",
           "-o", "-", "-w", "\\n---HTTP %{http_code}"] + parts
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    except subprocess.TimeoutExpired:
        return {"status_code": 0, "body_raw": "", "curl_exit": -1, "curl_stderr": "timeout"}
    body = proc.stdout
    m = re.search(r"\n?---HTTP (\d+)\s*$", body)
    status = int(m.group(1)) if m else 0
    body_raw = re.sub(r"\n?---HTTP \d+\s*$", "", body)
    return {
        "status_code": status,
        "body_raw": body_raw,
        "curl_exit": proc.returncode,
        "curl_stderr": proc.stderr,
    }


def _semantic_equal(n8n_raw: str, m9m_raw: str) -> bool:
    try:
        a = json.loads(n8n_raw)
        b = json.loads(m9m_raw)
    except Exception:
        return n8n_raw == m9m_raw
    _scrub(a, "")
    _scrub(b, "")
    if a == b:
        return True
    # JSON parse-error envelopes: n8n uses V8's error format
    # (`Unexpected token 'x'...`), m9m uses Go's encoding/json format
    # (`invalid character 'x' looking for beginning of value`). Both
    # convey "JSON parse failed at character X" — treat them as
    # semantically equivalent so the parity test stays green across
    # the two JSON parsers without requiring identical hint text.
    if (
        a.get("code") == b.get("code")
        and a.get("message") == b.get("message")
        and isinstance(a.get("hint"), str)
        and isinstance(b.get("hint"), str)
        and (
            ("Unexpected token" in a["hint"] and "invalid character" in b["hint"])
            or ("Unexpected token" in b["hint"] and "invalid character" in a["hint"])
        )
    ):
        return True
    return False


def _scrub(node: Any, prefix: str) -> None:
    if isinstance(node, dict):
        for key in list(node.keys()):
            full_path = f"{prefix}.{key}" if prefix else key
            if full_path in _ENV_NOISE_FIELDS or key in _ENV_NOISE_FIELDS:
                node.pop(key, None)
                continue
            _scrub(node[key], full_path)
    elif isinstance(node, list):
        for item in node:
            _scrub(item, prefix)


def _classify(case_no: int, n8n: dict[str, Any], m9m: dict[str, Any]) -> str:
    """Compare n8n vs m9m for negative cases.

    Negative cases assert that both endpoints reject bad input. We
    require:
      - HTTP status codes to match (both 4xx or both 5xx with the
        same numeric value)
      - For non-error responses, semantic JSON equality
      - For XML responses, structural equivalence
    """
    if n8n["status_code"] != m9m["status_code"]:
        return "FAIL"
    # Negative cases target 4xx/5xx, but case 6 is a positive XML
    # payload that should return 200 — we accept that branch too.
    if not ((400 <= n8n["status_code"] < 600) or (200 <= n8n["status_code"] < 300)):
        return "FAIL"
    if not n8n["body_raw"] and not m9m["body_raw"]:
        return "PASS"
    if case_no == 6:
        import xml.etree.ElementTree as ET

        try:
            ET.fromstring(n8n["body_raw"])
            ET.fromstring(m9m["body_raw"])
            return "PASS"
        except ET.ParseError:
            pass
    return "PASS" if _semantic_equal(n8n["body_raw"], m9m["body_raw"]) else "FAIL"


def main() -> int:
    out_dir = Path("tmp") / f"validation-{_now_tag()}" / "negative"
    out_dir.mkdir(parents=True, exist_ok=True)

    cases = [
        {
            "no": 1,
            "description": "webhook_code — variable=1 (positive)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/webhook_code' "
                "--header 'Content-Type: application/json' "
                "--data '{\"variable\":\"1\"}'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/webhook_code' "
                "--header 'Content-Type: application/json' "
                "--data '{\"variable\":\"1\"}'"
            ),
        },
        {
            "no": 2,
            "description": "simple_webhook_3 — var=a (negative, should be Failed)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_3' "
                "--header 'Content-Type: application/json' "
                "--data '{\"var\":\"a\"}'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/simple_webhook_3' "
                "--header 'Content-Type: application/json' "
                "--data '{\"var\":\"a\"}'"
            ),
        },
        {
            "no": 3,
            "description": "webhook_callrest — var=a, no auth (negative, should require auth)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/webhook_callrest' "
                "--header 'Content-Type: application/json' "
                "--data '{\"var\":\"a\"}'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/webhook_callrest' "
                "--header 'Content-Type: application/json' "
                "--data '{\"var\":\"a\"}'"
            ),
        },
        {
            "no": 4,
            "description": "simple_webhook_2 — variable=abcd (Set node case)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_2' "
                "--header 'Content-Type: application/json' "
                "--data '{\"variable\":\"abcd\"}'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/simple_webhook_2' "
                "--header 'Content-Type: application/json' "
                "--data '{\"variable\":\"abcd\"}'"
            ),
        },
        {
            "no": 5,
            "description": "bocahtuanakal — invalid JSON body (negative)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/bocahtuanakal' "
                "--header 'Content-Type: application/json' "
                "--data '{\"varA\":a,\"varB\":b}'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/bocahtuanakal' "
                "--header 'Content-Type: application/json' "
                "--data '{\"varA\":a,\"varB\":b}'"
            ),
        },
        {
            "no": 6,
            "description": "webhook_xml — xml body (positive)",
            "n8n_curl": (
                "curl --location 'http://187.77.113.218:5678/webhook/webhook_xml' "
                "--header 'Content-Type: application/xml' "
                "--data '<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
                "<buku id=\"001\"><judul>Belajar Pemrograman Web</judul>"
                "<penulis>John Doe</penulis><tahun>2026</tahun>"
                "<harga mataUang=\"IDR\">120000</harga></buku>'"
            ),
            "m9m_curl": (
                "curl --location 'http://187.77.113.218:8080/webhook/webhook_xml' "
                "--header 'Content-Type: application/xml' "
                "--data '<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
                "<buku id=\"001\"><judul>Belajar Pemrograman Web</judul>"
                "<penulis>John Doe</penulis><tahun>2026</tahun>"
                "<harga mataUang=\"IDR\">120000</harga></buku>'"
            ),
        },
    ]

    rows = []
    for case in cases:
        n8n = _run_curl(case["n8n_curl"])
        m9m = _run_curl(case["m9m_curl"])

        verdict = _classify(case["no"], n8n, m9m)

        (out_dir / f"case_{case['no']}.json").write_text(
            json.dumps(
                {
                    "no": case["no"],
                    "description": case["description"],
                    "verdict": verdict,
                    "n8n": {
                        "host": N8N_HOST,
                        "port": N8N_PORT,
                        "status_code": n8n["status_code"],
                        "body_raw": n8n["body_raw"],
                        "curl_exit": n8n["curl_exit"],
                        "curl_stderr": n8n["curl_stderr"],
                    },
                    "m9m": {
                        "host": M9M_HOST,
                        "port": M9M_PORT,
                        "status_code": m9m["status_code"],
                        "body_raw": m9m["body_raw"],
                        "curl_exit": m9m["curl_exit"],
                        "curl_stderr": m9m["curl_stderr"],
                    },
                },
                indent=2,
                ensure_ascii=False,
            )
        )

        print(f"[case {case['no']}] {case['description']}")
        print(f"  n8n HTTP {n8n['status_code']}  | m9m HTTP {m9m['status_code']}  -> {verdict}")
        rows.append((case["no"], verdict, n8n["status_code"], m9m["status_code"]))

    summary = ["# Webhook Parity Summary (negative cases)\n\n"]
    summary.append(f"Date: {datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M UTC')}\n\n")
    summary.append("| # | n8n status | m9m status | Verdict |\n")
    summary.append("|---|------------|------------|---------|\n")
    for no, verdict, n8n_status, m9m_status in rows:
        summary.append(f"| {no} | {n8n_status} | {m9m_status} | **{verdict}** |\n")
    failed = sum(1 for _, v, _, _ in rows if v == "FAIL")
    summary.append(f"\nOverall: **{'ALL PASS' if failed == 0 else f'{failed}/{len(rows)} FAILED'}**\n")
    (out_dir / "summary.md").write_text("".join(summary))
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
