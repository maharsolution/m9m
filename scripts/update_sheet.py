#!/usr/bin/env python3
"""Update the m9m-vs-n8n validation Google Sheet via the Sheets API v4.

Usage:
    python scripts/update_sheet.py read 'Validasi N8N - M9M!A1:H20'
    python scripts/update_sheet.py write 'Validasi N8N - M9M!G7:G7' '[["DONE"]]'
    python scripts/update_sheet.py append 'Validasi N8N - M9M!A:H' '[["7","..."]]'

The API key is intentionally hard-coded so this script is self-contained
when run inside the workspace — see .claude/skills/google-sheets-updater/SKILL.md
for the auth model and failure-mode notes.
"""
from __future__ import annotations

import json
import sys
import time
import urllib.parse
import urllib.request

API_KEY = "AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k"
SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"
BASE = f"https://sheets.googleapis.com/v4/spreadsheets/{SPREADSHEET_ID}"


def _request(method: str, url: str, body: dict | None = None) -> dict:
    """Issue a single API request with exponential backoff on 429."""
    data = None if body is None else json.dumps(body).encode("utf-8")
    last_err: Exception | None = None
    for attempt, delay in enumerate([1.0, 2.0, 4.0]):
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                payload = resp.read().decode("utf-8")
                return json.loads(payload) if payload else {}
        except urllib.error.HTTPError as e:
            # Retry only on rate-limit; surface everything else immediately.
            if e.code != 429:
                raise SystemExit(f"HTTP {e.code} on {method} {url}: {e.read().decode('utf-8', 'replace')}")
            last_err = e
            time.sleep(delay)
    raise SystemExit(f"exhausted retries on {method} {url}: {last_err}")


def _with_key(url: str) -> str:
    sep = "&" if "?" in url else "?"
    return f"{url}{sep}key={urllib.parse.quote(API_KEY)}"


def cmd_read(range_a1: str) -> dict:
    encoded = urllib.parse.quote(range_a1, safe="!A:")
    url = _with_key(f"{BASE}/values/{encoded}")
    return _request("GET", url)


def cmd_write(range_a1: str, values_json: str) -> dict:
    encoded = urllib.parse.quote(range_a1, safe="!A:")
    url = _with_key(
        f"{BASE}/values/{encoded}?valueInputOption=USER_ENTERED"
    )
    body = {"range": range_a1, "values": json.loads(values_json)}
    return _request("PUT", url, body)


def cmd_append(range_a1: str, values_json: str) -> dict:
    encoded = urllib.parse.quote(range_a1, safe="!A:")
    url = _with_key(
        f"{BASE}/values/{encoded}:append?valueInputOption=USER_ENTERED"
        "&insertDataOption=INSERT_ROWS"
    )
    body = {"range": range_a1, "values": json.loads(values_json)}
    return _request("POST", url, body)


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2
    op = argv[1]
    if op == "read":
        if len(argv) < 3:
            print("usage: update_sheet.py read <range>")
            return 2
        result = cmd_read(argv[2])
        print(json.dumps(result, indent=2, ensure_ascii=False))
    elif op == "write":
        if len(argv) < 4:
            print("usage: update_sheet.py write <range> '<json-values>'")
            return 2
        result = cmd_write(argv[2], argv[3])
        print(json.dumps(result, indent=2, ensure_ascii=False))
    elif op == "append":
        if len(argv) < 4:
            print("usage: update_sheet.py append <range> '<json-values>'")
            return 2
        result = cmd_append(argv[2], argv[3])
        print(json.dumps(result, indent=2, ensure_ascii=False))
    else:
        print(f"unknown operation: {op!r}")
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
