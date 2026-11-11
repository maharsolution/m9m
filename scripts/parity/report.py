#!/usr/bin/env python3
"""Render the parity runner output as JSON + Markdown.

    python scripts/parity/report.py
    python scripts/parity/report.py --drift-check

``--drift-check`` compares the report against the parity Google Sheet
and ``workflow-test.MD``: a row that the sheet still marks FAILED while
the live report says PASS (or the other way round) is drift, and the
command exits non-zero so CI can block on it.

The sheet is read through the public API key (read-only). When the key
cannot reach the sheet the drift check degrades to "skipped" instead of
failing the whole run — a missing sheet must not mask a real parity
regression.
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import sys
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
OUT_ROOT = ROOT / "tmp" / "parity-results"

SHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"
SHEET_API_KEY = os.environ.get(
    "PARITY_SHEET_API_KEY", "AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k")
POSITIVE_TAB = "Webhook Controlling"
NEGATIVE_TAB = "Webhook Controlling Negative Test"


def _load_results(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def _default_results_path() -> Path:
    stamp = _dt.date.today().isoformat()
    return OUT_ROOT / stamp / "parity-results.json"


def render_markdown(payload: dict[str, Any]) -> str:
    """Render the results table used by workflow-test.MD."""
    lines: list[str] = []
    lines.append(f"# m9m ↔ n8n parity report — {payload['generated_at']}")
    lines.append("")
    lines.append(f"* n8n: `{payload['n8n_base']}`")
    lines.append(f"* m9m: `{payload['m9m_base']}`")
    lines.append(f"* total: **{payload['total']}**, failed: **{payload['failed']}**")
    lines.append("")
    lines.append("| Suite | Case | Path | n8n | m9m | Verdict | Notes |")
    lines.append("|---|---|---|---|---|---|---|")
    for r in payload["results"]:
        notes = r.get("notes", "")
        if r["verdict"] != "PASS":
            notes = f"{notes} — {r.get('reason', '')}".strip(" —")
        lines.append(
            f"| {r['suite']} | `{r['id']}` | `{r['path']}` | "
            f"{r['n8n']['status_code']} | {r['m9m']['status_code']} | "
            f"**{r['verdict']}** | {notes} |")
    lines.append("")
    failed = [r for r in payload["results"] if r["verdict"] != "PASS"]
    if failed:
        lines.append("## Failures")
        lines.append("")
        for r in failed:
            lines.append(f"### `{r['id']}` {r['name']}")
            lines.append("")
            lines.append(f"* reason: {r.get('reason','')}")
            lines.append(f"* n8n body: `{r['n8n']['body_raw'][:400]}`")
            lines.append(f"* m9m body: `{r['m9m']['body_raw'][:400]}`")
            lines.append("")
    return "\n".join(lines) + "\n"


def _read_sheet(tab: str) -> list[list[str]]:
    rng = f"{tab}!A1:H60"
    url = ("https://sheets.googleapis.com/v4/spreadsheets/"
           f"{SHEET_ID}/values/{urllib.parse.quote(rng, safe='')}"
           f"?key={SHEET_API_KEY}")
    with urllib.request.urlopen(url, timeout=30) as resp:
        return json.load(resp).get("values", [])


def _sheet_rows(tab: str) -> list[dict[str, str]]:
    """Extract the case rows of a sheet tab.

    A row is a case row when column C holds a curl command (the n8n
    request). Column A carries the case number, G the HTTP status and
    H the verdict.
    """
    rows: list[dict[str, str]] = []
    try:
        values = _read_sheet(tab)
    except Exception as exc:  # noqa: BLE001
        print(f"[drift] cannot read sheet tab {tab!r}: {exc}", file=sys.stderr)
        return rows

    for row in values:
        cells = list(row) + [""] * (8 - len(row))
        request = cells[2] or ""
        if "curl" not in request:
            continue
        rows.append({
            "no": cells[0],
            "name": (cells[1] or "").strip(),
            "status": (cells[7] or "").strip().upper(),
            "code": (cells[6] or "").strip(),
        })
    return rows


def _path_of(row: dict[str, str]) -> str:
    """Extract the webhook path from a sheet row name or request."""
    for field in (row.get("name", ""),):
        low = field.lower()
        marker = "webhook/"
        if marker in low:
            rest = low.split(marker, 1)[1]
            return rest.split("'")[0].split('"')[0].strip()
    return ""


def drift_check(payload: dict[str, Any]) -> int:
    """Compare the live report with the sheet verdicts.

    Returns the number of drifted rows. A drifted row is one where the
    sheet still says FAILED but the live run passed, or vice versa.
    """
    print("[drift] reading parity sheet …")
    sheet_rows = _sheet_rows(POSITIVE_TAB) + _sheet_rows(NEGATIVE_TAB)
    if not sheet_rows:
        print("[drift] skipped: sheet rows unavailable")
        return 0

    live_by_path: dict[str, list[dict[str, Any]]] = {}
    for r in payload["results"]:
        live_by_path.setdefault(str(r["path"]).lstrip("/"), []).append(r)

    drifted = 0
    for row in sheet_rows:
        sheet_failed = row["status"] in {"FAILED", "FAIL"}
        path = _path_of(row)
        if not path:
            continue
        runs = live_by_path.get(path)
        if not runs:
            continue
        for run in runs:
            live_failed = run["verdict"] != "PASS"
            if live_failed != sheet_failed:
                drifted += 1
                print(f"[drift] {row['no'] or '-'} {row['name'][:48]!r}: "
                      f"sheet={row['status'] or 'OK'} live={run['verdict']} "
                      f"({run['id']})")

    if drifted:
        print(f"[drift] {drifted} drifted row(s) — sheet and report disagree")
    else:
        print("[drift] no drift detected between sheet and live report")
    return drifted


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="m9m <-> n8n parity report")
    parser.add_argument("--results", default=None,
                        help="path to parity-results.json")
    parser.add_argument("--out", default=None, help="output directory")
    parser.add_argument("--drift-check", action="store_true",
                        help="compare the report with the parity sheet")
    args = parser.parse_args(argv)

    results_path = Path(args.results) if args.results else _default_results_path()
    if not results_path.exists():
        print(f"no results file at {results_path}; run runner.py first",
              file=sys.stderr)
        return 2

    payload = _load_results(results_path)
    out_dir = Path(args.out) if args.out else results_path.parent
    out_dir.mkdir(parents=True, exist_ok=True)

    md = render_markdown(payload)
    (out_dir / "parity-report.md").write_text(md, encoding="utf-8")
    (out_dir / "parity-summary.json").write_text(
        json.dumps({
            "generated_at": payload["generated_at"],
            "total": payload["total"],
            "failed": payload["failed"],
            "suites": sorted({r["suite"] for r in payload["results"]}),
            "cases": [
                {"id": r["id"], "suite": r["suite"], "path": r["path"],
                 "verdict": r["verdict"], "reason": r["reason"]}
                for r in payload["results"]
            ],
        }, indent=2, ensure_ascii=False),
        encoding="utf-8")

    print(f"report written to {out_dir / 'parity-report.md'}")

    if args.drift_check:
        drift = drift_check(payload)
        if drift:
            return 1
    return 0 if payload["failed"] == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
