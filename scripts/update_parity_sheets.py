#!/usr/bin/env python3
"""Push the final m9m-vs-n8n parity verdicts into the validation
Google Sheets — both 'Webhook Controlling' (positive cases) and
'Webhook Controlling Negative Test'.

For each row we update:
  D = n8n response body
  F = m9m response body
  G = response code
  H = status ("OK" / "FAILED")

The actual responses are read from tmp/validation-<date>/case_<N>.json
so the sheet reflects what the running systems actually returned
in this validation run (not stale data from earlier runs).

Usage:
  python scripts/update_parity_sheets.py [--date 2026-09-05]
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys
import urllib.parse
import urllib.request

API_KEY = "AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k"
SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"
BASE = f"https://sheets.googleapis.com/v4/spreadsheets/{SPREADSHEET_ID}"


def _api(method: str, path: str, body: dict | None = None) -> dict:
    url = f"{BASE}{path}&key={API_KEY}" if "key=" not in path else f"{BASE}{path}"
    data = None if body is None else json.dumps(body).encode("utf-8")
    for delay in (1.0, 2.0, 4.0):
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                payload = resp.read().decode("utf-8")
                return json.loads(payload) if payload else {}
        except urllib.error.HTTPError as e:
            if e.code == 429:
                import time
                time.sleep(delay)
                continue
            raise SystemExit(f"HTTP {e.code}: {e.read().decode('utf-8', 'replace')}")
    raise SystemExit("rate-limited after 3 retries")


def _range_url(range_a1: str) -> str:
    return f"/values/{urllib.parse.quote(range_a1, safe='!:')}"


def update_sheet_row(sheet_name: str, row: int, n8n_response: str,
                     m9m_response: str, code: int, status: str) -> None:
    """Overwrite row N in `sheet_name` with the four output columns.

    Layout (both sheets):
      D = N8N Response, F = M9M Response, G = ResponseCode, H = Status
    Columns A-C / E hold non-output data that we leave alone.
    """
    rng = f"'{sheet_name}'!D{row}:H{row}"
    values = [[
        n8n_response.strip(),
        "",   # column E = M9N Request — leave unchanged
        m9m_response.strip(),
        str(code),
        status,
    ]]
    body = {"range": rng, "majorDimension": "ROWS", "values": values}
    _api("PUT", _range_url(rng) + "?valueInputOption=USER_ENTERED", body)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--date", default="2026-09-05")
    args = parser.parse_args()

    pos_dir = pathlib.Path("tmp") / f"validation-{args.date}"
    neg_dir = pos_dir / "negative"
    if not pos_dir.is_dir() or not neg_dir.is_dir():
        print(f"missing validation dir: {pos_dir}", file=sys.stderr)
        return 1

    # Positive sheet rows are 6..11 in the sheet (rows 1-5 are headers).
    for n in range(1, 7):
        path = pos_dir / f"case_{n}.json"
        if not path.exists():
            print(f"  skip positive case {n}: {path} missing")
            continue
        data = json.loads(path.read_text())
        verdict = data["verdict"]
        status = "OK" if verdict == "PASS" else "FAILED"
        update_sheet_row(
            "Webhook Controlling",
            row=5 + n,
            n8n_response=data["n8n"]["body_raw"] or "{}",
            m9m_response=data["m9m"]["body_raw"] or "{}",
            code=data["m9m"]["status_code"] or 0,
            status=status,
        )
        print(f"  positive case {n} -> row {5 + n} status={status}")

    # Negative sheet rows are 6..11 in the sheet.
    for n in range(1, 7):
        path = neg_dir / f"case_{n}.json"
        if not path.exists():
            print(f"  skip negative case {n}: {path} missing")
            continue
        data = json.loads(path.read_text())
        verdict = data["verdict"]
        status = "OK" if verdict == "PASS" else "FAILED"
        update_sheet_row(
            "Webhook Controlling Negative Test",
            row=5 + n,
            n8n_response=data["n8n"]["body_raw"] or "{}",
            m9m_response=data["m9m"]["body_raw"] or "{}",
            code=data["m9m"]["status_code"] or 0,
            status=status,
        )
        print(f"  negative case {n} -> row {5 + n} status={status}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
