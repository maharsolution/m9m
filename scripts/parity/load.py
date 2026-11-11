#!/usr/bin/env python3
"""Stability / load probe for the m9m webhook surface (suite C9).

Fires N sequential requests at one m9m webhook path, records the
latency distribution and checks that:

  * every request answers with the expected status,
  * the response body stays identical across runs (no state leaking
    between executions),
  * the p95 latency stays under the configured budget.

Usage:
    python scripts/parity/load.py --path webhook_code --count 50
    python scripts/parity/load.py --path post_balance --count 20 --body file.json
"""
from __future__ import annotations

import argparse
import json
import os
import statistics
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

DEFAULT_M9M = os.environ.get("M9M_BASE_URL", "http://187.77.113.218:8080")
DEFAULT_BODY = '{"variable":"1"}'


def _post(url: str, body: bytes, content_type: str, timeout: float) -> tuple[int, str]:
    req = urllib.request.Request(
        url, data=body, method="POST",
        headers={"Content-Type": content_type})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace").strip()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", errors="replace").strip()
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}"


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = min(len(ordered) - 1, max(0, int(round((pct / 100) * len(ordered))) - 1))
    return ordered[index]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="m9m webhook load probe")
    parser.add_argument("--path", default="webhook_code")
    parser.add_argument("--count", type=int, default=50)
    parser.add_argument("--base", default=DEFAULT_M9M)
    parser.add_argument("--body", default=DEFAULT_BODY,
                        help="literal body or a path to a JSON file")
    parser.add_argument("--content-type", default="application/json")
    parser.add_argument("--timeout", type=float, default=60.0)
    parser.add_argument("--p95-budget-ms", type=float, default=5000.0)
    parser.add_argument("--out", default=None)
    args = parser.parse_args(argv)

    body_text = args.body
    candidate = Path(args.body)
    if candidate.is_file():
        body_text = candidate.read_text(encoding="utf-8")
    body = body_text.encode("utf-8")

    url = f"{args.base.rstrip('/')}/webhook/{args.path.lstrip('/')}"
    print(f"load: {args.count} requests -> {url}")

    statuses: list[int] = []
    latencies: list[float] = []
    bodies: list[str] = []

    for i in range(args.count):
        started = time.monotonic()
        status, text = _post(url, body, args.content_type, args.timeout)
        elapsed = (time.monotonic() - started) * 1000
        statuses.append(status)
        latencies.append(elapsed)
        bodies.append(text)
        if (i + 1) % 10 == 0 or i == args.count - 1:
            print(f"  [{i + 1}/{args.count}] status={status} "
                  f"latency={elapsed:.0f}ms")

    distinct_bodies = len({b for b in bodies if b})
    summary = {
        "url": url,
        "count": args.count,
        "statuses": sorted(set(statuses)),
        "non_2xx": [s for s in statuses if not (200 <= s < 300)],
        "latency_ms": {
            "min": round(min(latencies), 1) if latencies else 0,
            "mean": round(statistics.fmean(latencies), 1) if latencies else 0,
            "p50": round(percentile(latencies, 50), 1),
            "p95": round(percentile(latencies, 95), 1),
            "max": round(max(latencies), 1) if latencies else 0,
        },
        "distinct_bodies": distinct_bodies,
        "stable_body": distinct_bodies <= 1,
    }
    print(json.dumps(summary, indent=2))

    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(summary, indent=2), encoding="utf-8")
        print(f"summary written to {out}")

    failures: list[str] = []
    if summary["non_2xx"]:
        failures.append(f"{len(summary['non_2xx'])} non-2xx responses")
    if not summary["stable_body"]:
        failures.append("response body changed between runs (state leak)")
    if summary["latency_ms"]["p95"] > args.p95_budget_ms:
        failures.append(
            f"p95 {summary['latency_ms']['p95']}ms exceeds budget "
            f"{args.p95_budget_ms}ms")

    for f in failures:
        print(f"FAIL: {f}", file=sys.stderr)
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
