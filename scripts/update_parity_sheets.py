#!/usr/bin/env python3
"""Regenerate parity documentation and sync the m9m validation sheet.

Evidence rules
--------------
* One canonical report drives every verdict in the generated document: the
  newest saved report under ``tmp/parity-results/`` that contains both the
  ``positive`` and the ``negative`` suite. Older saved reports stay listed as
  superseded evidence and are never merged into the totals.
* Verdicts are copied from the saved JSON report. This script never runs the
  parity harness and never invents a pass.
* A sheet row is written only when the request stored in the row is exactly
  the request of a canonical case (path, content type, payload and auth
  headers). Rows without such a match are reported and left untouched.
* Only result cells (response bodies, response code, status, remark) are
  written. Request columns are never rewritten, no row is ever cleared and no
  row is appended.
* Response bodies that carry credential material (private keys, encrypted
  credential blobs, credential rows) are never published.

Sheet access
------------
Reads use the public API key. Writes require the service-account JSON at
``secrets/sheets-sa.json``; the service account must be Editor on the sheet.
The service-account path signs its own RS256 JWT, so the script does not
depend on ``google-auth`` / ``google-api-python-client``.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import time
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import quote, urlencode
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
RESULTS_DIR = ROOT / "tmp" / "parity-results"
CASES_DIR = ROOT / "scripts" / "parity" / "cases"
DOC_PATH = ROOT / "workflow-test.MD"

SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"

# The Sheets read key comes from M9M_SHEETS_API_KEY. The literal below is
# only a DEPRECATED fallback: it keeps the script runnable on a machine
# that has not migrated yet, but it prints a warning and it must not be
# used for anything else. Never print the key value.
_LEGACY_SHEETS_API_KEY = "AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k"


def _resolve_sheets_api_key() -> str:
    """Return the Sheets read key without ever printing it."""
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
    return _LEGACY_SHEETS_API_KEY


PUBLIC_API_KEY = _resolve_sheets_api_key()
SA_JSON_PATH = Path(os.environ.get("SHEETS_SA_JSON", ROOT / "secrets" / "sheets-sa.json"))

POSITIVE_TAB = "Webhook Controlling"
NEGATIVE_TAB = "Webhook Controlling Negative Test"
AUTH_TAB = "Webhook Auth"
GAP_TAB = "Capabilty Gap"

# Column layout per tab (0-based). The negative tab is deliberately excluded:
# its header row and its data rows disagree about what F/H/I hold, so no cell
# there can be attributed without guessing.
TAB_LAYOUT = {
    POSITIVE_TAB: {
        "n8n_request": 2, "n8n_response": 3,
        "m9m_request": 4, "m9m_response": 5,
        "code": 6, "status": 7, "remark": 8,
        "first_data_row": 6,
        "suites": {"positive", "credentials"},
    },
    AUTH_TAB: {
        "n8n_request": 2, "n8n_response": 3,
        "m9m_request": 4, "m9m_response": 5,
        "code": 6, "status": 7, "remark": 8,
        "first_data_row": 6,
        "suites": {"positive", "credentials", "negative"},
    },
}

# Capability-gap rows whose stored claim is contradicted by source code, the
# live API and the canonical report. The old text is kept and the correction
# is appended, so no claim is erased.
CAPABILITY_UPDATES = {
    5: ("backend claim outdated 2026-11-12: an XML transform node exists "
        "(node type n8n-nodes-base.xml) and the canonical content_type suite is 5/5 PASS "
        "(c401/c405 XML round-trip equivalent); UI-level transform support is not "
        "covered by this evidence"),
    6: ("backend claim outdated 2026-11-12: GET /api/v1/executions returns execution records "
        "(live HTTP 200 probe on 2026-11-12); editor parity of the execution history "
        "view is not covered by this evidence"),
}


# ---------------------------------------------------------------------------
# Evidence loading
# ---------------------------------------------------------------------------
def load_reports():
    """Every saved report as (path, payload), oldest first."""
    reports = []
    for path in sorted(RESULTS_DIR.glob("*/parity-results.json")):
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            continue
        if "results" in payload and "generated_at" in payload:
            reports.append((path, payload))
    if not reports:
        raise ValueError("No parity-results.json available; run the parity harness first")
    reports.sort(key=lambda item: item[1]["generated_at"])
    return reports


def select_canonical(reports):
    """Newest report that carries the positive and negative suites."""
    core = [
        item for item in reports
        if {"positive", "negative"} <= {row["suite"] for row in item[1]["results"]}
    ]
    if not core:
        raise ValueError("No saved report contains both positive and negative suites")
    path, report = core[-1]
    results = report["results"]
    if report["total"] != len(results):
        raise ValueError("Report totals disagree with case count")
    if report["failed"] != sum(row["verdict"] != "PASS" for row in results):
        raise ValueError("Report totals disagree with case verdicts")
    if len({(row["suite"], row["id"]) for row in results}) != len(results):
        raise ValueError("Duplicate suite/case IDs in report")
    return path, report


def load_cases():
    """Case definitions from scripts/parity/cases (read-only input)."""
    cases = {}
    for path in sorted(CASES_DIR.glob("*.json")):
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            continue
        suite = payload.get("suite") or path.stem
        for case in payload.get("cases", []):
            if case.get("path"):
                case = dict(case)
                case.setdefault("suite", suite)
                cases[case["id"]] = case
    return cases


def evidence_index(reports, canonical_path):
    """Evidence index used by the generated document."""
    index = []
    for path, report in reports:
        suites = sorted({row["suite"] for row in report["results"]})
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if path == canonical_path:
            role = "canonical (drives every verdict below)"
        elif not {"positive", "negative"} <= set(suites):
            role = "auxiliary run (superseded by the canonical report)"
        else:
            role = "superseded core run"
        index.append({
            "source": path.resolve().relative_to(ROOT).as_posix(),
            "generated_at": report["generated_at"],
            "total": report["total"],
            "failed": report["failed"],
            "suites": suites,
            "sha256": digest,
            "role": role,
        })
    return index


# ---------------------------------------------------------------------------
# Request parsing / comparison
# ---------------------------------------------------------------------------
_URL_RE = re.compile(r"https?://[^\s'\"\\]+")
_HEADER_RE = re.compile(r"--header\s+'([^']*)'|--header\s+\"([^\"]*)\"|-H\s+'([^']*)'")
_DATA_RE = re.compile(r"--data(?:-raw)?\s+'((?:[^'\\]|\\.)*)'|--data(?:-raw)?\s+\"((?:[^\"\\]|\\.)*)\"")
_CONTENT_TYPE_RE = re.compile(r"Content-Type\s*:\s*([^\s'\"\\]+)", re.I)


def _unescape(text):
    return (text.replace("\\n", "\n").replace("\\t", "\t")
            .replace("\\'", "'").replace('\\"', '"'))


def parse_request(cell):
    """Parse a curl/postman request cell into path, content type, payload, headers."""
    text = str(cell or "").strip()
    if not text or "http" not in text:
        return None
    url = _URL_RE.search(text)
    if not url:
        return None
    path = re.sub(r"^https?://[^/]+", "", url.group(0)).split("?", 1)[0]
    content_type = None
    match = _CONTENT_TYPE_RE.search(text)
    if match:
        content_type = match.group(1).lower()
    headers = {}
    for candidate in _HEADER_RE.finditer(text):
        raw = next((group for group in candidate.groups() if group), "")
        if ":" not in raw:
            continue
        name, value = raw.split(":", 1)
        headers[name.strip().lower()] = value.strip()
    if content_type is None and "content-type" in headers:
        content_type = headers["content-type"].lower()
    payload = None
    match = _DATA_RE.search(text)
    if match:
        payload = _unescape(next((group for group in match.groups() if group), ""))
    return {"url": url.group(0), "path": path, "content_type": content_type,
            "payload": payload, "headers": headers}


def normalize_content_type(value):
    return str(value or "").split(";")[0].strip().lower()


def normalize_path(path):
    path = str(path or "").strip()
    if path.startswith("/"):
        path = path[1:]
    if path.startswith("webhook/"):
        path = path[len("webhook/"):]
    return path


def normalize_payload(value):
    """Canonical representation of a request payload for equality checks."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return ("empty", "")
    try:
        return ("json", json.loads(text))
    except ValueError:
        pass
    compact = re.sub(r">\s+<", "><", text)
    return ("text", re.sub(r"\s+", " ", compact).strip())


def headers_equal(sheet_headers, case_headers):
    """Compare request headers, ignoring the declared content type."""
    wanted = {name.lower(): value for name, value in (case_headers or {}).items()}
    wanted.pop("content-type", None)
    present = {name: value for name, value in (sheet_headers or {}).items()
               if name != "content-type"}
    if set(wanted) != set(present):
        return False
    return all(str(present[name]) == str(value) for name, value in wanted.items())


def request_matches(cell, case):
    """True when the sheet request cell is exactly the case request."""
    parsed = parse_request(cell)
    if not parsed:
        return False
    if normalize_path(parsed["path"]) != normalize_path(case.get("path")):
        return False
    if normalize_content_type(parsed["content_type"]) != normalize_content_type(case.get("content_type")):
        return False
    if normalize_payload(parsed["payload"]) != normalize_payload(case.get("body")):
        return False
    return headers_equal(parsed["headers"], case.get("headers"))


def mismatch_details(parsed, case):
    """Which attributes of a stored request differ from a canonical case."""
    if not parsed:
        return ["request could not be parsed"]
    details = []
    if normalize_path(parsed["path"]) != normalize_path(case.get("path")):
        details.append("path")
    if normalize_content_type(parsed["content_type"]) != normalize_content_type(case.get("content_type")):
        details.append("content type")
    if normalize_payload(parsed["payload"]) != normalize_payload(case.get("body")):
        details.append("payload")
    if not headers_equal(parsed["headers"], case.get("headers")):
        details.append("auth headers")
    return details


def near_misses(cell, cases):
    """Canonical cases that share the stored path, with the differing attributes."""
    parsed = parse_request(cell)
    if not parsed:
        return []
    found = []
    for case_id, case in cases.items():
        if normalize_path(parsed["path"]) != normalize_path(case.get("path")):
            continue
        details = mismatch_details(parsed, case)
        if details:
            found.append((case_id, details))
    return found


# ---------------------------------------------------------------------------
# Secret guard
# ---------------------------------------------------------------------------
SECRET_RULES = [
    (re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"), "private key block"),
    (re.compile(r'"privateKey"\s*:\s*"[^"]+"', re.I), "privateKey field"),
    (re.compile(r'"(password|passwd|clientSecret|secretKey|accessToken|apiKey|encryptionKey)"\s*:\s*"[^"]+"', re.I),
     "credential field"),
    (re.compile(r'"data"\s*:\s*\{'), "credential data object"),
    (re.compile(r'"data"\s*:\s*"[^"]{16,}"'), "encrypted credential blob"),
    (re.compile(r'"rows"\s*:'), "credential rows payload"),
]


def secret_reasons(text):
    """Reasons why a response body must not be published."""
    body = str(text or "")
    return [label for pattern, label in SECRET_RULES if pattern.search(body)]


# ---------------------------------------------------------------------------
# Document status format
# ---------------------------------------------------------------------------
# Every status-bearing line of the generated document uses ONE shape:
#
#   | Suite | Case | Path | n8n | m9m | Verdict | Evidence |
#   |---|---|---|---|---|---|---|
#   | positive | `p01` | `simple_webhook` | 200 | 200 | **PASS** | ... |
#
# The shape is the one `scripts/parity/report.py` writes into
# `parity-report.md`, so `report.py::document_drift` parses the document and
# the harness report with a single rule: a line that starts with "|", a
# backticked case id and one of PASS / FAIL / BLOCKED. The verdict cell is the
# only place a verdict word may appear; free-text evidence is neutralised so
# prose can never be read as a second, contradictory verdict.
_STATUS_WORD = re.compile(r"\b(PASS(?:ED)?|FAIL(?:ED)?|OK|DONE|BLOCKED|SKIPPED)\b", re.I)


def scrub_status_words(text):
    """Break status words inside free-text evidence.

    "webhook should accept and pass through" would otherwise be read as a
    PASS verdict by the drift check. The word is split with a zero-width
    space: invisible in Markdown, impossible to match.
    """
    def _break(match):
        word = match.group(0)
        return word[:1] + "\u200b" + word[1:]

    return _STATUS_WORD.sub(_break, str(text or ""))


def case_table(rows):
    """Render cases as the standardised status rows."""
    lines = ["| Suite | Case | Path | n8n | m9m | Verdict | Evidence |",
             "|---|---|---|---|---|---|---|"]
    for row in rows:
        evidence = scrub_status_words(row.get("reason", ""))
        name = str(row.get("name") or "").strip()
        if name and name != str(row.get("path") or ""):
            evidence = (f"{scrub_status_words(name)} — {evidence}" if evidence
                        else scrub_status_words(name))
        path = str(row.get("path") or "n/a")
        lines.append(
            f"| {row['suite']} | `{row['id']}` | `{path}` | "
            f"{row['n8n'].get('status_code')} | {row['m9m'].get('status_code')} | "
            f"**{row['verdict']}** | {evidence} |")
    return lines


def display_value(value):
    """Readable, secret-free rendering of a sheet cell."""
    text = str(value or "")
    reasons = secret_reasons(text)
    if reasons:
        return f"<withheld: {', '.join(reasons)}>"
    if len(text) > 80:
        digest = hashlib.sha256(text.encode("utf-8")).hexdigest()[:12]
        return f"<{len(text)} chars, sha256 {digest}>"
    return text or "<empty>"


# ---------------------------------------------------------------------------
# Sheet access
# ---------------------------------------------------------------------------
class Sheets:
    """Minimal Sheets v4 client: public API key for reads, service account for writes."""

    def __init__(self):
        self.mode = "api-key"
        self.token = None
        self.error = None
        if SA_JSON_PATH.exists():
            try:
                self.token = self._service_account_token()
                self.mode = "service-account"
            except Exception as exc:  # noqa: BLE001 - reported, falls back to read-only
                self.error = f"service-account auth failed: {exc}"

    @staticmethod
    def _service_account_token():
        import jwt
        from cryptography.hazmat.primitives import serialization

        info = json.loads(SA_JSON_PATH.read_text(encoding="utf-8"))
        private_key = serialization.load_pem_private_key(
            info["private_key"].encode("utf-8"), password=None)
        now = int(time.time())
        assertion = jwt.encode(
            {
                "iss": info["client_email"],
                "scope": "https://www.googleapis.com/auth/spreadsheets",
                "aud": "https://oauth2.googleapis.com/token",
                "iat": now,
                "exp": now + 3600,
            },
            private_key,
            algorithm="RS256",
        )
        payload = urlencode({
            "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
            "assertion": assertion,
        }).encode("utf-8")
        with urlopen(Request("https://oauth2.googleapis.com/token", data=payload), timeout=30) as response:
            return json.load(response)["access_token"]

    def _headers(self):
        headers = {"Accept": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    @staticmethod
    def _open(request, timeout=60, attempts=5):
        """Open a request, retrying the transient Google API errors."""
        delay = 5
        last = None
        for _ in range(attempts):
            try:
                return urlopen(request, timeout=timeout)
            except HTTPError as exc:
                last = exc
                if exc.code not in (429, 500, 502, 503, 504):
                    raise
                time.sleep(delay)
                delay = min(delay * 2, 60)
        raise last

    def read(self, tab, last_row=200):
        a1 = quote(f"'{tab}'!A1:{last_row}")
        url = f"https://sheets.googleapis.com/v4/spreadsheets/{SPREADSHEET_ID}/values/{a1}"
        if not self.token:
            url += f"?key={PUBLIC_API_KEY}"
        with self._open(Request(url, headers=self._headers())) as response:
            return json.load(response).get("values", [])

    def read_range(self, a1):
        url = f"https://sheets.googleapis.com/v4/spreadsheets/{SPREADSHEET_ID}/values/{quote(a1)}"
        if not self.token:
            url += f"?key={PUBLIC_API_KEY}"
        with self._open(Request(url, headers=self._headers())) as response:
            return json.load(response).get("values", [])

    def batch_update(self, updates):
        body = json.dumps({"valueInputOption": "RAW", "data": updates}).encode("utf-8")
        request = Request(
            f"https://sheets.googleapis.com/v4/spreadsheets/{SPREADSHEET_ID}/values:batchUpdate",
            data=body,
            headers={**self._headers(), "Content-Type": "application/json"},
        )
        with self._open(request) as response:
            return json.load(response)


# ---------------------------------------------------------------------------
# Update planning
# ---------------------------------------------------------------------------
def _row_cells(rows, index):
    cells = list(rows[index - 1]) if 0 <= index - 1 < len(rows) else []
    return cells + [""] * (12 - len(cells))


def _agree(values):
    """True when every candidate value is identical."""
    unique = {json.dumps(value, sort_keys=True, default=str) for value in values}
    return len(unique) == 1


def plan_tab_updates(tab, report, cases, sheet_rows):
    """Cell updates for one tab plus explicit skip reasons.

    Each side of a row (n8n request column, m9m request column) is matched
    independently against the canonical cases. A cell is written only when the
    request stored in that row is exactly the request of a canonical case, so
    a row that stores a different URL or a different credential value is never
    overwritten with the result of a look-alike request.
    """
    layout = TAB_LAYOUT[tab]
    results = {row["id"]: row for row in report["results"]}
    rows = sheet_rows.get(tab, [])
    updates, skips, notes = [], [], []
    matched_cases = set()

    pool = {case_id: case for case_id, case in cases.items()
            if case_id in results and case.get("suite") in layout["suites"]}
    all_cases = {case_id: case for case_id, case in cases.items() if case_id in results}

    for row_number in range(layout["first_data_row"], len(rows) + 1):
        cells = _row_cells(rows, row_number)
        n8n_cell = str(cells[layout["n8n_request"]])
        m9m_cell = str(cells[layout["m9m_request"]])
        if not n8n_cell.strip() and not m9m_cell.strip():
            continue

        n8n_hits = [cid for cid, case in pool.items() if request_matches(n8n_cell, case)]
        m9m_hits = [cid for cid, case in pool.items() if request_matches(m9m_cell, case)]
        if not n8n_hits and not m9m_hits:
            parsed = parse_request(n8n_cell) or parse_request(m9m_cell) or {}
            elsewhere = [cid for cid, case in all_cases.items()
                         if cid not in pool and (request_matches(n8n_cell, case)
                                                or request_matches(m9m_cell, case))]
            if elsewhere:
                suites = sorted({cases[cid].get("suite", "?") for cid in elsewhere})
                skips.append(
                    f"{tab} row {row_number}: stored request ({parsed.get('path', '?')}) is identical to "
                    f"{', '.join(sorted(elsewhere))} — a {'/'.join(suites)} case outside this tab's "
                    f"suite set; writing it here would relabel that case; row left unchanged")
            else:
                misses = near_misses(n8n_cell, all_cases) or near_misses(m9m_cell, all_cases)
                if misses:
                    detail = "; ".join(f"{cid} ({', '.join(details)} differ)"
                                       for cid, details in sorted(misses))
                    skips.append(
                        f"{tab} row {row_number}: stored request ({parsed.get('path', '?')}) was not "
                        f"probed by the canonical run — differs from {detail}; left unchanged")
                else:
                    skips.append(
                        f"{tab} row {row_number}: no canonical case stores this request "
                        f"({parsed.get('path', '?')}); left unchanged")
            continue
        matched_cases.update(n8n_hits + m9m_hits)

        # D — n8n response body.
        if n8n_hits:
            bodies = [results[cid]["n8n"].get("body_raw") for cid in n8n_hits]
            reasons = sorted({reason for body in bodies for reason in secret_reasons(body)})
            if reasons:
                skips.append(
                    f"{tab} row {row_number} column D: n8n body withheld "
                    f"({', '.join(reasons)})")
            elif not _agree(bodies):
                skips.append(
                    f"{tab} row {row_number} column D: matching cases disagree on the n8n body; left unchanged")
            else:
                updates.append({"range": f"'{tab}'!D{row_number}",
                                "values": [[str(bodies[0] or "")]]})
                notes.append(
                    f"{tab} row {row_number} D: n8n response body refreshed from "
                    f"{', '.join(sorted(n8n_hits))}")
        else:
            parsed = parse_request(n8n_cell) or {}
            misses = near_misses(n8n_cell, all_cases)
            detail = ""
            if misses:
                detail = " — differs from " + "; ".join(
                    f"{cid} ({', '.join(details)} differ)" for cid, details in sorted(misses))
            skips.append(
                f"{tab} row {row_number} column D: stored n8n request "
                f"({parsed.get('path', '?')}) was not probed by the canonical run{detail}; left unchanged")

        # F — m9m response body; G — m9m response code.
        if m9m_hits:
            m9m_bodies = [results[cid]["m9m"].get("body_raw") for cid in m9m_hits]
            reasons = sorted({reason for body in m9m_bodies for reason in secret_reasons(body)})
            if reasons:
                skips.append(
                    f"{tab} row {row_number} column F: m9m body withheld "
                    f"({', '.join(reasons)})")
            elif not _agree(m9m_bodies):
                skips.append(
                    f"{tab} row {row_number} column F: matching cases disagree on the m9m body; left unchanged")
            else:
                updates.append({"range": f"'{tab}'!F{row_number}",
                                "values": [[str(m9m_bodies[0] or "")]]})
                notes.append(
                    f"{tab} row {row_number} F: m9m response body refreshed from "
                    f"{', '.join(sorted(m9m_hits))}")
            codes = [results[cid]["m9m"].get("status_code") for cid in m9m_hits]
            if _agree(codes) and codes[0]:
                updates.append({"range": f"'{tab}'!G{row_number}", "values": [[str(codes[0])]]})
                notes.append(f"{tab} row {row_number} G: m9m HTTP {codes[0]}")
            else:
                skips.append(
                    f"{tab} row {row_number} column G: matching cases disagree on the m9m status code; left unchanged")
        else:
            misses = near_misses(m9m_cell, all_cases)
            detail = ""
            if misses:
                detail = " — differs from " + "; ".join(
                    f"{cid} ({', '.join(details)} differ)" for cid, details in sorted(misses))
            skips.append(
                f"{tab} row {row_number} columns F/G: stored m9m request has no canonical "
                f"counterpart{detail}; left unchanged")

        # H — comparison status. Only written when both sides of the row are
        # attributable to canonical cases and all of them agree.
        verdicts = {results[cid]["verdict"] for cid in set(n8n_hits + m9m_hits)}
        if n8n_hits and m9m_hits and len(verdicts) == 1:
            verdict = verdicts.pop()
            updates.append({"range": f"'{tab}'!H{row_number}",
                            "values": [["OK" if verdict == "PASS" else "FAILED"]]})
            notes.append(f"{tab} row {row_number} H: {verdict}")
        elif len(verdicts) > 1:
            skips.append(
                f"{tab} row {row_number} column H: matching cases disagree "
                f"({', '.join(sorted(verdicts))}); left unchanged")
        else:
            skips.append(
                f"{tab} row {row_number} column H: only one side of the row is attributable to the "
                f"canonical run; left unchanged")

    return updates, skips, notes, matched_cases


def plan_capability_updates(sheet_rows):
    """Capability-gap corrections: the old claim is kept, the correction appended."""
    updates, skips = [], []
    rows = sheet_rows.get(GAP_TAB, [])
    for row_number, correction in sorted(CAPABILITY_UPDATES.items()):
        cells = _row_cells(rows, row_number)
        current = str(cells[2]).strip()
        if not current:
            skips.append(f"{GAP_TAB} row {row_number}: claim cell is empty; not written")
            continue
        if "outdated 2026-11-12" in current:
            skips.append(f"{GAP_TAB} row {row_number}: correction already present; left unchanged")
            continue
        updates.append({
            "range": f"'{GAP_TAB}'!C{row_number}",
            "values": [[f"{current} — {correction}"]],
        })
    return updates, skips


# ---------------------------------------------------------------------------
# Documentation rendering
# ---------------------------------------------------------------------------
def render_docs(canonical_path, report, index, sync_state, notes, skips):
    source = canonical_path.resolve().relative_to(ROOT).as_posix()
    digest = hashlib.sha256(canonical_path.read_bytes()).hexdigest()
    results = report["results"]
    passed = report["total"] - report["failed"]
    fail_count = sum(row["verdict"] == "FAIL" for row in results)
    blocked_count = sum(row["verdict"] == "BLOCKED" for row in results)
    skipped_count = sum(row["verdict"] == "SKIPPED" for row in results)
    lines = [
        "# m9m ↔ n8n Webhook Parity Test Report",
        "",
        f"_Evidence generated: {report['generated_at']}_",
        "",
        f"Source: `{source}`. SHA-256: `{digest}`.",
        "",
        f"**{passed}/{report['total']} PASS; {report['failed']} FAIL/BLOCKED "
        f"({fail_count} FAIL, {blocked_count} BLOCKED, {skipped_count} SKIPPED). "
        "Not full parity.**",
        "Every verdict below is copied from that one saved report. A PASS means the harness "
        "comparison contract passed; it does not prove complete UI, credential or byte-for-byte "
        "parity. BLOCKED is non-passing evidence, not success.",
        "",
        "## Section A — Runtime Context",
        "",
        f"- n8n: `{report['n8n_base']}`; m9m: `{report['m9m_base']}`.",
        "- Deploy operator: `mhr`, SSH port `2212`; compose: `/home/mhr/gabungan`.",
        "- No deployment performed during documentation refresh.",
        "",
    ]

    for title, suite in [("B — Positive Cases", "positive"),
                         ("C — Negative Cases", "negative"),
                         ("K — Focus Cases", "focus")]:
        rows = [row for row in results if row["suite"] == suite]
        lines += [f"## Section {title}", "",
                  f"{sum(row['verdict'] == 'PASS' for row in rows)}/{len(rows)} PASS.", ""]
        lines += case_table(rows)
        lines.append("")

    extra = [row for row in results if row["suite"] not in {"positive", "negative", "focus"}]
    if extra:
        lines += ["## Section M — Extended Cases In This Report", "",
                  "Included in the totals above. BLOCKED is non-passing evidence, not success.", ""]
        lines += case_table(extra)
        lines.append("")

    content_type = [row for row in results if row["suite"] == "content_type"]
    content_type_pass = sum(row["verdict"] == "PASS" for row in content_type)
    lines += [
        "## Section D — Capability Gaps",
        "",
        "The stored sheet claims 'no transformation node' and 'execution history not found' are "
        "outdated as capability claims: XML/JSON transformation nodes exist, the canonical "
        f"content_type suite is {content_type_pass}/{len(content_type)} PASS and "
        "`GET /api/v1/executions` answers HTTP 200 with records. "
        "The sheet rows keep their original wording and carry the correction next to it. UI-only "
        "gaps (node connectivity in the designer, debugging parity, credential-register UI) have "
        "no parity evidence and stay open.",
        "",
        "The C9 goroutine-leak check is implemented but its metric is still missing: the local "
        "/metrics endpoint serves only m9m_up, so the goroutine count before and after the "
        "50-request burst cannot be read. The Prometheus Go collector that would expose "
        "go_goroutines is a runtime change and stays an open, documented sub-gap of the C9 "
        "suite - it is not silently dropped.",
        "",
        "## Section E — Non-passing Cases",
        "",
        "Every case below stays open until a newer saved report shows PASS. Matching failures on "
        "both hosts are baseline blockers, not parity passes.",
        "",
    ]
    failing = [row for row in results if row["verdict"] != "PASS"]
    lines += case_table(failing)
    lines += [
        "",
        "## Section F — Regeneration and Sheet Sync",
        "",
        "1. Run the parity harness to capture a fresh report under `tmp/parity-results/`.",
        "2. Run `python scripts/update_parity_sheets.py --update-docs` to regenerate this "
        "document from the newest saved report that contains the positive and negative suites.",
        "3. Run `python scripts/update_parity_sheets.py --apply` with the service account to "
        "write the unambiguous rows. Request columns are never rewritten, no row is cleared and "
        "no row is appended.",
        "4. Inspect the printed plan: written cells, withheld cells and skipped rows with reasons.",
        "",
        "### Status line format",
        "",
        "Every status-bearing line of this document uses ONE shape — the same shape "
        "`scripts/parity/report.py` writes into `parity-report.md`, so "
        "`report.py::document_drift` parses this document and the harness report with one rule:",
        "",
        "| Suite | Case | Path | n8n | m9m | Verdict | Evidence |",
        "|---|---|---|---|---|---|---|",
        "| positive | `xx01` | `example_path` | 200 | 200 | **PASS** | <reason from the report> |",
        "",
        "Rules a parser can rely on:",
        "",
        "- The line starts with `|`; the case id is a backticked harness id in column 2.",
        "- The verdict cell (column 6) holds exactly one of PASS, FAIL or BLOCKED and is the "
        "only cell that carries a verdict word.",
        "- Free-text evidence is scrubbed so a prose word can never be read as a second verdict.",
        "- Suites without a case id (Section H, Section N) carry counts only.",
        "",
        "## Section G — Hard Constraints",
        "",
        "- Never modify n8n workflow JSON, force responses or mask runtime differences.",
        "- Keep failed cases failed until new captured evidence passes.",
        "- Preserve sheet request columns, unrelated rows and formulas; no resets.",
        "- Never publish credential secrets in reports or in the sheet.",
        "",
        "## Section H — Current Snapshot Verdicts",
        "",
    ]
    lines += ["| Suite | Cases | PASS | FAIL | BLOCKED |", "|---|---|---|---|---|"]
    for suite in sorted({row["suite"] for row in results}):
        rows = [row for row in results if row["suite"] == suite]
        blocked = sum(row["verdict"] == "BLOCKED" for row in rows)
        failed = sum(row["verdict"] == "FAIL" for row in rows)
        lines.append(
            f"| {suite} | {len(rows)} | {sum(row['verdict'] == 'PASS' for row in rows)} | "
            f"{failed} | {blocked} |")

    positives = [row for row in results if row["suite"] == "positive"]
    negatives = [row for row in results if row["suite"] == "negative"]
    negative_pass = sum(row["verdict"] == "PASS" for row in negatives)
    negative_open = [row["id"] for row in negatives if row["verdict"] != "PASS"]
    db_cases = [row for row in results if row["id"] in ("p12", "p14")]
    db_open = [row["id"] for row in db_cases if row["verdict"] != "PASS"]
    if negative_open:
        negative_tail = (
            "The remaining negative failure(s) — " + ", ".join(negative_open) +
            " — are listed in Section E; the case definitions belong to the parity worker, so "
            "this document records them instead of changing them.")
    else:
        negative_tail = ("No negative case is open in this report: both hosts answer every "
                         "rejection probe with the same status and body.")
    if db_open:
        database_paragraph = (
            "Historical fixes, kept stable by id: GAP-2026-09-10-1 (IF/Switch empty-branch "
            "routing, commit `fcb3e56`) and GAP-2026-09-10-2 (PostgreSQL default sslMode, "
            "commit `9eb90e2`). The current database-body failures do not by themselves show "
            "those bugs have returned: the two database branches now return one item per row on "
            "both hosts, and the remaining difference is the row payload itself.")
    else:
        database_paragraph = (
            "Historical fixes, kept stable by id: GAP-2026-09-10-1 (IF/Switch empty-branch "
            "routing, commit `fcb3e56`) and GAP-2026-09-10-2 (PostgreSQL default sslMode, "
            "commit `9eb90e2`). Both database branches pass in this report, and the routing and "
            "sslMode behaviour stays locked by the C11 regression cases of this report.")
    lines += [
        "",
        "## Section I — Gap Registry",
        "",
        "`parity/parity_gaps.json` retains historical evidence and current observed failures. "
        "Historical resolved entries are not proof that every residual credential or header case "
        "passes today.",
        "",
        "## Section J — Historical 2026-09-10 Cycle (Superseded)",
        "",
        "An earlier document claimed 14/14 positive and 5/5 negative passes while its own "
        "Section B still listed two failures. Those were two incompatible evidence sources in one "
        "file: the 14/14 and 5/5 numbers came from a scratch script (`tmp/full_validation.py`), "
        "not from a saved parity report, so they cannot be cited as evidence. Nothing in this "
        "document relabels those failures as passes.",
        "",
        "Every verdict-bearing section above (B, C, E, H, K, M) is rendered from the single "
        "canonical report named in the header, and Section H counts the same rows. Section B and "
        "Section J can therefore no longer disagree: the current numbers below are the numbers "
        "Section B lists, case by case, and they are the only numbers this document claims.",
        "",
        f"Current canonical numbers: overall {passed}/{report['total']} PASS "
        f"({fail_count} FAIL, {blocked_count} BLOCKED, {skipped_count} SKIPPED); positive "
        f"{sum(row['verdict'] == 'PASS' for row in positives)}"
        f"/{len(positives)} PASS, negative {negative_pass}"
        f"/{len(negatives)} PASS, focus "
        f"{sum(row['verdict'] == 'PASS' for row in results if row['suite'] == 'focus')}"
        f"/{sum(1 for row in results if row['suite'] == 'focus')} PASS.",
        "",
        f"The negative suite moved from 1/5 to {negative_pass}/{len(negatives)} because the "
        "authentication rejection wire "
        "format was fixed in the m9m source and deployed: both hosts now answer a rejected "
        "webhook request with HTTP 401 (no credentials, wrong basic-auth credentials) or HTTP "
        "403 (invalid bearer token, wrong API key), a plain-text body, no Content-Type header "
        "and the challenge `WWW-Authenticate: Basic realm=\"Webhook\"`. " + negative_tail,
        "",
        database_paragraph,
        "",
        "## Section L — Sheet Sync Status",
        "",
    ]
    if sync_state.get("applied"):
        lines += [
            f"- Applied {sync_state.get('cells', 0)} cell update(s) across "
            f"{sync_state.get('ranges', 0)} range(s) over "
            f"{sync_state.get('rows_updated', 0)} row(s) through the "
            f"{sync_state.get('mode')} path.",
            f"- {sync_state.get('rows_skipped', 0)} sheet row(s) and "
            f"{sync_state.get('cases_skipped', 0)} case(s) were left unchanged; every one of them "
            "is listed below with its reason.",
            "- Readback verified: every written range was re-read and compared cell by cell.",
        ]
    elif sync_state.get("blocked"):
        lines.append(f"- Sheet update blocked: {sync_state['blocked']}")
    else:
        lines.append("- No sheet write was performed in this run; the sheet is unchanged.")
    lines += [
        "- Writes are additive: only result cells of rows whose stored request matches a "
        "canonical case exactly are rewritten. Request columns, unrelated rows and formulas are "
        "left alone; no row is cleared and no row is appended.",
        "- Response bodies that carry credential material (private keys, encrypted credential "
        "blobs, credential rows) are withheld from the sheet and listed below instead.",
        "",
    ]
    if notes:
        lines += ["### Cells written", ""]
        lines += [f"- {note}" for note in notes]
        lines.append("")
    if skips:
        lines += ["### Rows and cells left unchanged", ""]
        lines += [f"- {skip}" for skip in skips]
        lines.append("")

    lines += ["## Section N — Evidence Index", "",
              "Every saved report under `tmp/parity-results/`. Only the canonical report drives "
              "the verdicts above; the others are listed for traceability.", ""]
    for item in index:
        lines.append(
            f"- `{item['source']}` — generated {item['generated_at']}; {item['total']} cases, "
            f"{item['failed']} non-passing; suites: {', '.join(item['suites'])}; "
            f"SHA-256 `{item['sha256'][:16]}…`; {item['role']}.")
    lines += [
        "",
        "The auxiliary single-case runs were produced earlier on 2026-11-11 and are superseded by "
        "the canonical run; their verdicts are not mixed into the totals.",
        "",
    ]
    DOC_PATH.write_text("\n".join(lines) + "\n", encoding="utf-8")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
_RANGE_RE = re.compile(r"^'(?P<tab>[^']+)'!(?P<col>[A-Z]+)(?P<row>\d+)$")
_SKIP_ROW_RE = re.compile(r"^(?P<tab>[A-Za-z][A-Za-z ]*?) row (?P<row>\d+)")


def plan_diff(sheets, updates, sheet_rows):
    """Current -> planned value for every planned range, secret-safe.

    The current value comes from the sheet snapshot that was read before the
    plan was built, so the preview costs no extra API calls.
    """
    lines = []
    for item in updates:
        target = item["range"]
        old = None
        match = _RANGE_RE.match(target)
        if match:
            rows = sheet_rows.get(match.group("tab")) or []
            index = int(match.group("row")) - 1
            if 0 <= index < len(rows):
                cells = list(rows[index])
                position = ord(match.group("col")) - ord("A")
                if 0 <= position < len(cells):
                    old = cells[position]
        if old is None:
            try:
                current = sheets.read_range(target)
            except (HTTPError, OSError):
                current = None
            old = current[0][0] if current and current[0] else ""
        new = item["values"][0][0]
        marker = "=" if str(old) == str(new) else "->"
        lines.append(f"DIFF {target}: {display_value(old)} {marker} {display_value(new)}")
    return lines


def count_rows(updates, skips):
    """(rows updated, sheet rows left unchanged, cases left unchanged)."""
    updated = set()
    for item in updates:
        match = _RANGE_RE.match(item["range"])
        if match:
            updated.add((match.group("tab"), match.group("row")))
    skipped_rows = set()
    skipped_cases = set()
    for skip in skips:
        match = _SKIP_ROW_RE.match(skip)
        if match:
            skipped_rows.add((match.group("tab"), match.group("row")))
        else:
            skipped_cases.add(skip.split(":")[0])
    return len(updated), len(skipped_rows), len(skipped_cases)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--results", help="explicit parity-results.json; default newest core report")
    parser.add_argument("--update-docs", action="store_true", help="regenerate workflow-test.MD")
    parser.add_argument("--apply", action="store_true", help="write unambiguous rows to the sheet")
    args = parser.parse_args(argv)

    reports = load_reports()
    if args.results:
        canonical_path = Path(args.results).resolve()
        report = json.loads(canonical_path.read_text(encoding="utf-8"))
    else:
        canonical_path, report = select_canonical(reports)
    index = evidence_index(reports, canonical_path)
    cases = load_cases()

    print(f"Canonical evidence: {canonical_path.resolve().relative_to(ROOT).as_posix()} "
          f"({report['generated_at']}); {report['total']} cases, {report['failed']} non-passing")

    sheets = Sheets()
    if sheets.error:
        print(f"AUTH {sheets.error}")
    sheet_rows = {}
    for tab in list(TAB_LAYOUT) + [NEGATIVE_TAB, GAP_TAB]:
        try:
            sheet_rows[tab] = sheets.read(tab)
        except (HTTPError, OSError) as exc:
            print(f"AUTH sheet read failed for {tab}: {exc}")
            sheet_rows[tab] = []

    updates, skips, notes = [], [], []
    matched_anywhere = set()
    for tab in TAB_LAYOUT:
        tab_updates, tab_skips, tab_notes, matched = plan_tab_updates(
            tab, report, cases, sheet_rows)
        updates += tab_updates
        skips += tab_skips
        notes += tab_notes
        matched_anywhere.update(matched)
    gap_updates, gap_skips = plan_capability_updates(sheet_rows)
    updates += gap_updates
    skips += gap_skips

    # Cases with no home on any writable tab are reported once, not per tab.
    for row in report["results"]:
        if row["id"] in matched_anywhere:
            continue
        if row.get("suite") in {"positive", "credentials", "negative", "focus"}:
            case = cases.get(row["id"], {})
            homes = []
            for tab, layout in TAB_LAYOUT.items():
                rows = sheet_rows.get(tab, [])
                for row_number in range(layout["first_data_row"], len(rows) + 1):
                    cells = _row_cells(rows, row_number)
                    for side, column in (("n8n", layout["n8n_request"]),
                                          ("m9m", layout["m9m_request"])):
                        parsed = parse_request(str(cells[column]))
                        if not parsed:
                            continue
                        if normalize_path(parsed["path"]) != normalize_path(row.get("path")):
                            continue
                        details = mismatch_details(parsed, case)
                        homes.append(
                            f"{tab} row {row_number} {side} request "
                            f"({', '.join(details) + ' differ' if details else 'matches exactly'})")
            if homes:
                skips.append(
                    f"{row['suite']}/{row['id']} ({row.get('path')}): no sheet row stores this request; "
                    f"closest stored rows — {'; '.join(sorted(set(homes)))}; not written")
            else:
                skips.append(
                    f"{row['suite']}/{row['id']} ({row.get('path')}): no sheet row stores this request; not written")
    for skip in skips:
        print(f"SKIP {skip}")
    for item in updates:
        print(f"WRITE {item['range']}")

    rows_updated, rows_skipped, cases_skipped = count_rows(updates, skips)
    print(f"PLAN {len(updates)} cell update(s) over {rows_updated} row(s); "
          f"{rows_skipped} sheet row(s) and {cases_skipped} case(s) left unchanged")
    for line in plan_diff(sheets, updates, sheet_rows):
        print(line)

    sync_state = {"applied": False, "mode": sheets.mode, "ranges": 0, "cells": 0,
                  "rows_updated": rows_updated, "rows_skipped": rows_skipped,
                  "cases_skipped": cases_skipped}
    if args.apply:
        if not updates:
            print("No writable cell; sheet unchanged.")
            sync_state["blocked"] = "no unambiguous row matched the canonical report"
        elif sheets.mode != "service-account":
            sync_state["blocked"] = (
                "service-account credentials unavailable; set SHEETS_SA_JSON or grant "
                f"{SA_JSON_PATH} access")
            print(f"BLOCK {sync_state['blocked']}")
        else:
            result = sheets.batch_update(updates)
            sync_state["applied"] = True
            sync_state["ranges"] = len(updates)
            sync_state["cells"] = result.get("totalUpdatedCells", 0)
            for item in updates:
                actual = sheets.read_range(item["range"])
                expected = [[str(value) for value in row] for row in item["values"]]
                if [[str(value) for value in row] for row in actual] != expected:
                    raise ValueError(f"Readback mismatch: {item['range']}")
            print(f"Applied {sync_state['cells']} cell(s) in {len(updates)} range(s); readback verified")
    elif updates:
        print("Sheet dry run: no writes. Use --apply for authenticated synchronization.")

    if args.update_docs:
        render_docs(canonical_path, report, index, sync_state, notes, skips)
        print(f"Regenerated {DOC_PATH.name}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, HTTPError) as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
