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
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
OUT_ROOT = ROOT / "tmp" / "parity-results"

sys.path.insert(0, str(HERE))

from signatures import curl_body_signature  # noqa: E402  (local module, path set above)

SHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"

# The Sheets read key comes from M9M_SHEETS_API_KEY. The literal below is
# a DEPRECATED fallback - it keeps the script runnable on a machine that
# has not migrated yet, but it prints a warning. Never print the value.
_LEGACY_SHEET_API_KEY = "AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k"


def _resolve_sheet_api_key() -> str:
    """Return the Sheets API key without ever printing it."""
    key = (os.environ.get("M9M_SHEETS_API_KEY") or "").strip()
    if key:
        return key

    legacy = (os.environ.get("PARITY_SHEET_API_KEY") or "").strip()
    if legacy:
        print(
            "WARNING: PARITY_SHEET_API_KEY is deprecated - use M9M_SHEETS_API_KEY instead",
            file=sys.stderr,
        )
        return legacy

    print(
        "WARNING: M9M_SHEETS_API_KEY is not set - falling back to the deprecated "
        "built-in Sheets key. Set M9M_SHEETS_API_KEY and retire the literal.",
        file=sys.stderr,
    )
    return _LEGACY_SHEET_API_KEY


SHEET_API_KEY = _resolve_sheet_api_key()
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
            "request": request,
            "suite": "negative" if tab == NEGATIVE_TAB else "positive",
        })
    return rows


def _path_of(row: dict[str, str]) -> str:
    """Extract the webhook path from a sheet row name or request."""
    for field in (row.get("request", ""), row.get("name", "")):
        low = field.lower()
        marker = "webhook/"
        if marker in low:
            rest = low.split(marker, 1)[1]
            return re.split(r"[\s\'\"?]", rest)[0].strip("/")
    return ""


# Status vocabulary understood by the document drift check.
_PASS_STATUSES = {"PASS", "PASSED", "OK", "DONE"}
_FAIL_STATUSES = {"FAIL", "FAILED"}
_BLOCKED_STATUSES = {"BLOCKED", "SKIPPED"}
_KNOWN_STATUSES = _PASS_STATUSES | _FAIL_STATUSES | _BLOCKED_STATUSES

# ``p01``, ``n03``, ``c501``, ``f01`` - a harness case id in backticks.
_CASE_ID = re.compile(r"`([A-Za-z]{1,4}\d{1,4})`")


def _live_index(payload: dict[str, Any]) -> tuple[dict[str, list[dict]], dict[str, list[dict]]]:
    """Index the live results by (path, body digest) and by path alone."""
    by_pair: dict[str, list[dict]] = {}
    by_path: dict[str, list[dict]] = {}
    for row in payload.get("results", []):
        path = str(row.get("path", "")).lstrip("/")
        if path.startswith("webhook/"):
            path = path[len("webhook/"):]
        digest = str(row.get("body_hash", ""))
        by_pair.setdefault(f"{path}|{digest}", []).append(row)
        by_path.setdefault(path, []).append(row)
    return by_pair, by_path


def drift_check(payload: dict[str, Any], document: Path | None = None) -> int:
    """Compare the live report with the sheet verdicts.

    A sheet row is paired with a harness case by webhook path AND the
    digest of the request body, never by the sheet's own "no" column:
    that column is the sheet's 1..N row number, not a harness case id,
    and two rows can share one path (``db_quiry`` has a MySQL and a
    PostgreSQL branch). Every harness case is matched at most once, so
    one case can never be counted twice.

    Returns the number of drifted rows.
    """
    print("[drift] reading parity sheet …")
    sheet_rows = _sheet_rows(POSITIVE_TAB) + _sheet_rows(NEGATIVE_TAB)
    if not sheet_rows:
        print("[drift] BLOCKED: sheet rows unavailable")
        return 1  # unavailable verification is not a successful drift check

    by_pair, by_path = _live_index(payload)
    matched = 0
    drifted = 0
    used: set[str] = set()

    for row in sheet_rows:
        sheet_failed = row["status"] in _FAIL_STATUSES
        path = _path_of(row)
        if not path:
            continue
        digest = curl_body_signature(row.get("request", ""))

        candidates = list(by_pair.get(f"{path}|{digest}", []))
        matched_by = "path+body"
        if not candidates:
            # The sheet row carries no body digest, or the digest is not
            # stored in the report. Fall back to the path, but only when
            # exactly one harness case uses that path - an ambiguous path
            # is never guessed.
            fallback = [case for case in by_path.get(path, [])
                        if str(case.get("id")) not in used]
            if len(fallback) == 1:
                candidates = fallback
                matched_by = "path"
        candidates = [case for case in candidates if str(case.get("id")) not in used]

        if not candidates:
            print(f"[drift] sheet row {row.get('no') or '-'} "
                  f"{row['name'][:44]!r}: no harness case left for path {path!r} "
                  f"(body digest {digest[:8] or '-'})")
            continue
        if len(candidates) > 1:
            print(f"[drift] sheet row {row.get('no') or '-'} "
                  f"{row['name'][:44]!r}: {len(candidates)} harness cases share path "
                  f"{path!r}; not guessed")
            continue

        run = candidates[0]
        used.add(str(run.get("id")))
        matched += 1

        if row["status"] not in _KNOWN_STATUSES:
            drifted += 1
            print(f"[drift] sheet row {row.get('no') or '-'} {row['name'][:44]!r}: "
                  f"unknown sheet status {row['status']!r}")
            continue

        if row["status"] in _BLOCKED_STATUSES:
            # The sheet marks the row as not executed - nothing to compare.
            continue

        live_verdict = str(run.get("verdict", "FAIL"))
        if live_verdict == "SKIPPED":
            # The harness case did not run in this mode: no verdict.
            continue

        live_failed = live_verdict != "PASS"
        if live_failed != sheet_failed:
            drifted += 1
            print(f"[drift] {row.get('no') or '-'} {row['name'][:48]!r}: "
                  f"sheet={row['status'] or 'OK'} live={live_verdict} "
                  f"({run['id']}, matched by {matched_by})")

    # The document guard is part of the same check: the Markdown report
    # and the sheet must agree with the live run.
    drifted += document_drift(payload, document or (ROOT / "workflow-test.MD"))

    if drifted:
        print(f"[drift] {drifted} drifted row(s) — sheet and report disagree")
    else:
        print("[drift] no drift detected between sheet and live report")
    return drifted + (0 if matched else 1)


def _document_case_ids(line: str, live: dict[str, dict],
                        live_only: bool = True) -> list[str]:
    """Harness case ids referenced by one Markdown line.

    Explicit ids (```p01```) win. When a line only names a webhook path
    the path is used, but only if exactly one live case uses it -
    an ambiguous path is never guessed.

    With ``live_only=False`` the explicit ids of the document are
    returned even when the harness did not run them; the path fallback
    is disabled because it needs the live index.
    """
    explicit = _CASE_ID.findall(line)
    if live_only:
        explicit = [match for match in explicit if match in live]
    if explicit:
        return sorted(set(explicit))
    if not live_only:
        return []

    by_path: dict[str, list[str]] = {}
    for case_id, row in live.items():
        path = str(row.get("path", "")).lstrip("/")
        if path.startswith("webhook/"):
            path = path[len("webhook/"):]
        by_path.setdefault(path, []).append(case_id)
    for token in re.findall(r"[A-Za-z0-9_.-]+", line):
        candidates = by_path.get(token, [])
        if len(candidates) == 1:
            return candidates
    return []


def document_drift(payload: dict[str, Any], path: Path,
                    require_evidence: bool = True,
                    compare_verdicts: bool = True) -> int:
    """Check status-bearing Markdown rows, including contradictory B/J rows.

    Both document shapes are understood: the bullet lines written by the
    sheet updater (``- `p01` `simple_webhook`: **PASS**``) and the
    Markdown tables of the report renderer (``| positive | `p01` | ... |``).
    The document is never confused with the sheet: the sheet rows are
    not reused as the document's own fixture.

    Unmatched prose is not guessed into a verdict. Missing evidence fails
    closed. Every case id contributes at most one drift event, so one
    harness case can never be counted twice.

    ``require_evidence`` is switched off for the offline run: there the
    shared-host cases have no verdict, so only the internal consistency
    of the document (no case may claim PASS and FAIL at the same time)
    is checked.

    ``compare_verdicts`` is switched off for the offline run as well:
    the document records the live (shared-host) run while the offline
    run has no live verdicts, so comparing the two would compare two
    different evidence domains. The live run keeps the comparison.
    """
    if not path.is_file():
        print(f"[drift] document unavailable: {path}")
        return 1

    live: dict[str, dict] = {}
    for row in payload.get("results", []):
        live.setdefault(str(row.get("id")), row)

    statuses_per_case: dict[str, list[str]] = {}
    matched_evidence = 0
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        if not (stripped.startswith("|") or stripped.startswith("-")
                or stripped.startswith("*")):
            continue
        statuses = [match.upper() for match in
                    re.findall(r"\b(PASS(?:ED)?|FAIL(?:ED)?|OK|DONE|BLOCKED|SKIPPED)\b",
                               line, re.I)]
        if not statuses:
            continue
        case_ids = _document_case_ids(line, live, live_only=False)
        if any(case_id in live for case_id in case_ids):
            matched_evidence += 1
        for case_id in case_ids:
            statuses_per_case.setdefault(case_id, []).extend(statuses)

    if not statuses_per_case:
        print(f"[drift] document {path.name}: no case evidence found")
        return 1
    if require_evidence and matched_evidence == 0:
        print(f"[drift] document {path.name}: no status line refers to a harness case")
        return 1

    drifted = 0
    for case_id, statuses in sorted(statuses_per_case.items()):
        expected = str(live.get(case_id, {}).get("verdict", ""))
        document_failed = any(status in _FAIL_STATUSES for status in statuses)
        document_blocked = any(status in _BLOCKED_STATUSES for status in statuses)
        document_passed = any(status in _PASS_STATUSES for status in statuses)

        if document_failed and document_passed:
            drifted += 1
            print(f"[drift] document row disagrees: {case_id} "
                  f"states both PASS and FAIL")
            continue

        if not compare_verdicts:
            # Offline mode: the document and the offline run describe two
            # different evidence domains, so only the internal consistency
            # of the document is judged here.
            continue

        if not expected:
            # The document mentions a case the harness did not run in this
            # mode: there is no verdict to compare, never guessed.
            continue
        if expected == "SKIPPED":
            # Not executed in this mode: there is no verdict at all.
            continue

        if expected == "PASS":
            agrees = document_passed
        elif expected == "BLOCKED":
            agrees = document_blocked
        else:
            agrees = document_failed
        if not agrees:
            drifted += 1
            print(f"[drift] document row disagrees: {case_id} "
                  f"(document={'/'.join(sorted(set(statuses)))}, live={expected})")
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
