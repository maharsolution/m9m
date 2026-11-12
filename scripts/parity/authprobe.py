#!/usr/bin/env python3
"""Read-only authentication probe for the n8n and m9m parity targets.

The probe answers one question per endpoint: does this host require an
API key, and which header does it honour? It never prints, stores or
transmits a secret beyond the request itself - the response body is
discarded, only the HTTP status is reported.

    python scripts/parity/authprobe.py
    python scripts/parity/authprobe.py --json

Probe matrix (all requests are GET, no body, no side effects):

  * ``none``          - no auth header at all
  * ``n8n-key``       - ``X-N8N-API-KEY: <N8N_API_KEY>``
  * ``m9m-key``       - ``X-API-Key: <M9M_API_KEY>`` (only when configured)
  * ``bogus``         - a syntactically valid but wrong key
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
sys.path.insert(0, str(HERE))

from env import api_headers, load_env  # noqa: E402  (local module, path set above)

DEFAULT_N8N = os.environ.get("N8N_BASE_URL", "http://187.77.113.218:5678")
DEFAULT_M9M = os.environ.get("M9M_BASE_URL", "http://187.77.113.218:8080")

PROBE_PATHS = (
    "/healthz",
    "/api/v1/node-types",
    "/api/v1/workflows",
)


def _probe(url: str, headers: dict[str, str], timeout: float) -> dict[str, object]:
    request = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            response.read(2048)
            return {"status": response.status, "error": None}
    except urllib.error.HTTPError as exc:
        exc.read(2048)
        return {"status": exc.code, "error": None}
    except Exception as exc:  # noqa: BLE001 - transport failures are data
        return {"status": 0, "error": f"{type(exc).__name__}: {exc}"}


def probe_host(base: str, host: str, timeout: float = 30.0) -> list[dict[str, object]]:
    """Probe every endpoint of one host with every header variant."""
    n8n_key = os.environ.get("N8N_API_KEY")
    m9m_key = os.environ.get("M9M_API_KEY")

    variants: list[tuple[str, dict[str, str]]] = [("none", {})]
    if n8n_key:
        variants.append(("n8n-key", {"X-N8N-API-KEY": n8n_key}))
    if m9m_key:
        variants.append(("m9m-key", {"X-API-Key": m9m_key}))
    variants.append(("bogus", {"X-N8N-API-KEY": "parity-probe-invalid",
                               "X-API-Key": "parity-probe-invalid"}))

    rows: list[dict[str, object]] = []
    for path in PROBE_PATHS:
        for variant, headers in variants:
            result = _probe(base.rstrip("/") + path, headers, timeout)
            rows.append({
                "host": host,
                "path": path,
                "variant": variant,
                "status": result["status"],
                "error": result["error"],
            })
    return rows


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="parity auth probe")
    parser.add_argument("--n8n-base", default=DEFAULT_N8N)
    parser.add_argument("--m9m-base", default=DEFAULT_M9M)
    parser.add_argument("--timeout", type=float, default=30.0)
    parser.add_argument("--json", action="store_true", dest="as_json")
    args = parser.parse_args(argv)

    present = load_env()
    print(f"credentials: {'; '.join(f'{k}=' + ('set' if v else 'absent') for k, v in sorted(present.items()))}")

    rows = probe_host(args.n8n_base, "n8n", args.timeout)
    rows += probe_host(args.m9m_base, "m9m", args.timeout)

    if args.as_json:
        print(json.dumps(rows, indent=2))
        return 0

    print(f"{'host':5} {'path':24} {'variant':9} status")
    for row in rows:
        detail = f"{row['status']}"
        if row["error"]:
            detail = f"{row['status']} ({row['error']})"
        print(f"{row['host']:5} {row['path']:24} {row['variant']:9} {detail}")

    m9m_unauthenticated = all(
        row["status"] in (200, 301, 302)
        for row in rows
        if row["host"] == "m9m" and row["variant"] == "none")
    n8n_requires_key = any(
        row["status"] in (401, 403)
        for row in rows
        if row["host"] == "n8n" and row["variant"] == "none")
    print()
    print(f"m9m /api/v1 without any key: "
          f"{'HTTP 200 - no auth required' if m9m_unauthenticated else 'rejected - auth required'}")
    print(f"n8n /api/v1 without any key: "
          f"{'rejected - auth required' if n8n_requires_key else 'HTTP 200 - no auth required'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
