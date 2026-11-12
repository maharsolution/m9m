#!/usr/bin/env python3
"""m9m <-> n8n parity runner.

Executes every declared parity case against both runtimes and records
the raw responses so the comparator and the report generator can work
off a single snapshot.

    n8n  http://187.77.113.218:5678
    m9m  http://187.77.113.218:8080

Usage:
    python scripts/parity/runner.py                      # every suite
    python scripts/parity/runner.py --suite positive
    python scripts/parity/runner.py --case p06
    python scripts/parity/runner.py --suite focus --no-body-compare

The runner never rewrites a response to make it match. A FAIL is a
signal that ``internal/nodes/...`` has a bug (workflow-test.MD
Section G).
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
CASES_DIR = HERE / "cases"
OUT_ROOT = ROOT / "tmp" / "parity-results"

sys.path.insert(0, str(HERE))

from compare import classify  # noqa: E402  (local module, path set above)
from env import describe, load_env  # noqa: E402
from signatures import case_signature  # noqa: E402

_ENV_PRESENT = load_env()

DEFAULT_N8N = os.environ.get("N8N_BASE_URL", "http://187.77.113.218:5678")
DEFAULT_M9M = os.environ.get("M9M_BASE_URL", "http://187.77.113.218:8080")


def load_suites(suite_filter: str | None) -> list[dict[str, Any]]:
    """Load every ``cases/*.json`` file, optionally filtered by suite.

    A JSON file in ``cases/`` without a ``cases`` list is not a suite
    (it may be an artifact or a note) and is skipped instead of
    breaking the run.
    """
    suites: list[dict[str, Any]] = []
    for path in sorted(CASES_DIR.glob("*.json")):
        data = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(data.get("cases"), list):
            continue
        suite = data.get("suite") or path.stem
        if suite_filter and suite != suite_filter:
            continue
        data["suite"] = suite
        data["source_file"] = str(path.relative_to(ROOT))
        suites.append(data)
    return suites


def _request(url: str, case: dict[str, Any], timeout: float) -> dict[str, Any]:
    """POST a case to one runtime and capture status/headers/body."""
    body = case.get("body", "")
    headers = {"Content-Type": case.get("content_type", "application/json")}
    headers.update(case.get("headers", {}) or {})

    data = body.encode("utf-8") if isinstance(body, str) else (body or b"")
    req = urllib.request.Request(
        url, data=data, method=case.get("method", "POST"), headers=headers)

    started = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            return {
                "status_code": resp.status,
                "headers": {k: v for k, v in resp.headers.items()},
                "body_raw": raw.strip(),
                "elapsed_ms": int((time.monotonic() - started) * 1000),
                "error": None,
            }
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        return {
            "status_code": exc.code,
            "headers": {k: v for k, v in exc.headers.items()},
            "body_raw": raw.strip(),
            "elapsed_ms": int((time.monotonic() - started) * 1000),
            "error": None,
        }
    except Exception as exc:  # noqa: BLE001 — network failures are results too
        return {
            "status_code": 0,
            "headers": {},
            "body_raw": "",
            "elapsed_ms": int((time.monotonic() - started) * 1000),
            "error": f"{type(exc).__name__}: {exc}",
        }


def _case_url(base: str, case: dict[str, Any]) -> str:
    path = case.get("path", "").lstrip("/")
    return f"{base.rstrip('/')}/webhook/{path}"


def body_signature(case: dict[str, Any]) -> str:
    """Stable fingerprint of a case request body.

    Two cases that hit the same webhook path but carry a different
    payload (the MySQL and the PostgreSQL branch of ``db_quiry``, for
    example) must not be confused with each other by the drift check.
    The signature is a SHA-256 of the canonical JSON form, so the key
    order of the request body does not matter. No secret is stored:
    only the digest is.
    """
    return case_signature(case)


def run_case(case: dict[str, Any], n8n_base: str, m9m_base: str,
             timeout: float, retries: int) -> dict[str, Any]:
    """Run one case against both runtimes and classify the result."""
    if case.get("check"):
        from extended import run_contract
        return run_contract(case, n8n_base, m9m_base, timeout, _request)
    n8n_url = _case_url(n8n_base, case)
    m9m_url = _case_url(m9m_base, case)

    n8n = _request(n8n_url, case, timeout)
    m9m = _request(m9m_url, case, timeout)

    # One retry on transport failures (empty reply / connection reset):
    # the two servers share a host, so a rolling restart of the m9m
    # container can drop a single request.
    for _ in range(max(0, retries)):
        if n8n.get("status_code") == 0 or m9m.get("status_code") == 0:
            if n8n.get("status_code") == 0:
                n8n = _request(n8n_url, case, timeout)
            if m9m.get("status_code") == 0:
                m9m = _request(m9m_url, case, timeout)

    verdict, reason = classify(case, n8n, m9m)
    if n8n.get("status_code") == 0 or m9m.get("status_code") == 0:
        verdict = "FAIL"
        reason = (f"transport error: n8n={n8n.get('error')} m9m={m9m.get('error')}")
    elif (verdict == "FAIL" and case.get("environment_blocked")
          and n8n.get("status_code") == m9m.get("status_code")):
        # Both hosts answer identically, so the two runtimes are in
        # parity; the fixture itself (a credential or an upstream
        # service the shared instance owns) cannot be satisfied here.
        verdict = "BLOCKED"
        reason = (str(case["environment_blocked"]) +
                  f" (both hosts answered HTTP {n8n.get('status_code')})")

    return {
        "id": case.get("id"),
        "name": case.get("name"),
        "path": case.get("path"),
        "method": case.get("method", "POST"),
        "content_type": case.get("content_type"),
        "expected_status": case.get("expected_status"),
        "compare": case.get("compare", "json"),
        "notes": case.get("notes", ""),
        "body_hash": body_signature(case),
        "n8n_url": n8n_url,
        "m9m_url": m9m_url,
        "verdict": verdict,
        "reason": reason,
        "n8n": n8n,
        "m9m": m9m,
    }


def skipped_result(case: dict[str, Any], suite: str, reason: str) -> dict[str, Any]:
    """A case that was not executed (for example in offline mode)."""
    empty = {"status_code": 0, "headers": {}, "body_raw": "", "elapsed_ms": 0, "error": None}
    return {
        "id": case.get("id"),
        "name": case.get("name"),
        "path": case.get("path"),
        "method": case.get("method", "POST"),
        "content_type": case.get("content_type"),
        "expected_status": case.get("expected_status"),
        "compare": case.get("compare", "json"),
        "notes": case.get("notes", ""),
        "body_hash": body_signature(case),
        "suite": suite,
        "verdict": "SKIPPED",
        "reason": reason,
        "n8n": empty,
        "m9m": empty,
    }


def summarise(results: list[dict[str, Any]]) -> dict[str, Any]:
    """Per-suite and overall verdict counts."""
    counts = {"PASS": 0, "FAIL": 0, "BLOCKED": 0, "SKIPPED": 0}
    suites: dict[str, dict[str, int]] = {}
    for row in results:
        verdict = row.get("verdict", "FAIL")
        counts[verdict] = counts.get(verdict, 0) + 1
        bucket = suites.setdefault(row.get("suite", "?"),
                                   {"PASS": 0, "FAIL": 0, "BLOCKED": 0, "SKIPPED": 0, "total": 0})
        bucket[verdict] = bucket.get(verdict, 0) + 1
        bucket["total"] += 1
    return {"counts": counts, "suites": suites}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="m9m <-> n8n parity runner")
    parser.add_argument("--suite", default=None,
                        help="run only this suite (positive, negative, focus, ...)")
    parser.add_argument("--case", default=None,
                        help="run only this case id (e.g. p06, n03, f01)")
    parser.add_argument("--n8n-base", default=DEFAULT_N8N)
    parser.add_argument("--m9m-base", default=DEFAULT_M9M)
    parser.add_argument("--timeout", type=float, default=60.0)
    parser.add_argument("--retries", type=int, default=1)
    parser.add_argument("--out", default=None,
                        help="output directory (default tmp/parity-results/<date>)")
    parser.add_argument("--no-body-compare", action="store_true",
                        help="compare status and headers only")
    parser.add_argument("--offline", action="store_true",
                        help="run only cases marked offline (no shared infrastructure)")
    args = parser.parse_args(argv)

    print(f"credentials: {describe(_ENV_PRESENT)}")
    suites = load_suites(args.suite)
    if not suites:
        print("no parity case files found", file=sys.stderr)
        return 2

    stamp = _dt.date.today().isoformat()
    out_dir = Path(args.out) if args.out else OUT_ROOT / stamp
    out_dir.mkdir(parents=True, exist_ok=True)

    results: list[dict[str, Any]] = []
    deferred_drift: list[tuple[dict[str, Any], dict[str, Any]]] = []
    for suite in suites:
        for case in suite.get("cases", []):
            if args.case and case.get("id") != args.case:
                continue
            effective = dict(case)
            if args.no_body_compare:
                effective["compare"] = "none"
            if args.offline and not effective.get("offline", False):
                result = skipped_result(effective, suite["suite"],
                                        "offline mode: case needs the shared infrastructure")
            elif effective.get("check") == "drift":
                # The drift guard compares the report that this very run
                # produces, so it is evaluated after the loop.
                result = skipped_result(effective, suite["suite"],
                                        "evaluated after the run")
            else:
                result = run_case(effective, args.n8n_base, args.m9m_base,
                                   args.timeout, args.retries)
                result["suite"] = suite["suite"]
            results.append(result)
            if effective.get("check") == "drift" and result["verdict"] == "SKIPPED":
                deferred_drift.append((effective, result))
            print(f"  [{result['suite']}/{result['id']}] {result['verdict']:4} "
                  f"n8n={result['n8n']['status_code']} "
                  f"m9m={result['m9m']['status_code']}  "
                  f"{result['name']}  ({result['reason']})")

    if deferred_drift:
        from extended import evaluate_drift
        current = {"results": results, "offline": bool(args.offline)}
        for case, result in deferred_drift:
            verdict, reason = evaluate_drift(case, current, offline=bool(args.offline))
            result["verdict"] = verdict
            result["reason"] = reason
            print(f"  [{result['suite']}/{result['id']}] {verdict:4} "
                  f"n8n={result['n8n']['status_code']} "
                  f"m9m={result['m9m']['status_code']}  "
                  f"{result['name']}  ({reason})")

    if not results:
        print("no cases matched selection", file=sys.stderr)
        return 2
    failed = [r for r in results if r["verdict"] != "PASS"]
    blocking = [r for r in failed if r["verdict"] in ("FAIL", "BLOCKED")]
    summary = summarise(results)
    payload = {
        "generated_at": _dt.datetime.now(_dt.timezone.utc).isoformat(),
        "n8n_base": args.n8n_base,
        "m9m_base": args.m9m_base,
        "offline": bool(args.offline),
        "total": len(results),
        "failed": len(failed),
        "counts": summary["counts"],
        "suite_counts": summary["suites"],
        "results": results,
    }
    (out_dir / "parity-results.json").write_text(
        json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")

    print(f"\n{len(results) - len(failed)}/{len(results)} PASS "
          f"(FAIL: {summary['counts'].get('FAIL', 0)}, "
          f"BLOCKED: {summary['counts'].get('BLOCKED', 0)}, "
          f"SKIPPED: {summary['counts'].get('SKIPPED', 0)})")
    for suite_name in sorted(summary["suites"]):
        bucket = summary["suites"][suite_name]
        print(f"  {suite_name:24} {bucket['PASS']}/{bucket['total']} PASS, "
              f"{bucket['FAIL']} FAIL, {bucket['BLOCKED']} BLOCKED, {bucket['SKIPPED']} SKIPPED")
    print(f"results written to {out_dir / 'parity-results.json'}")
    # SKIPPED is an expected outcome in offline mode, so only FAIL and
    # BLOCKED make the run unsuccessful.
    return 0 if not blocking else 1


if __name__ == "__main__":
    sys.exit(main())
