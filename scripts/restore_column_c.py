#!/usr/bin/env python3
"""Restore column C (N8N Request) in both parity sheets from the
original snapshot captured in `edit-0.md`.

The earlier sheet update accidentally overwrote column C (which held
the N8N curl request). This script reads the snapshot in the IDE
uploads directory and writes the correct curl text back to C6:C11
on both sheets. Column E (M9M Request) and the responses stay as-is.
"""
from __future__ import annotations

import os
import pathlib
import re
import sys

SCOPES = ["https://www.googleapis.com/auth/spreadsheets"]
SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"
SNAPSHOT = pathlib.Path(
    "C:/Users/HYPE R Series/.cursor/projects/"
    "c-Users-HYPE-R-Series-OneDrive-workspace-m9m/uploads/edit-0.md"
)

# Mapping: row -> (description keyword, raw curl text).
# Both sheets share the same six webhooks. We use the description
# keyword to match the row.
POSITIVE_CURLS = {
    1: "curl --location 'http://187.77.113.218:5678/webhook/webhook_code' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"1\"}'",
    2: "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_3' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"1\"}'",
    3: "curl --location 'http://187.77.113.218:5678/webhook/webhook_callrest' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"1\"}'",
    4: "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_2' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"1\"}'",
    5: "curl --location 'http://187.77.113.218:5678/webhook/bocahtuanakal' \\\n--header 'Content-Type: application/json' \\\n--header 'Authorization: Basic YWRtaW46YWRtaW4xMjM=' \\\n--data '{ \"varA\": 1, \"varB\": 2}'",
    6: "curl --location 'http://187.77.113.218:5678/webhook/webhook_xml' \\\n--header 'Content-Type: application/xml' \\\n--data '<?xml version=\"1.0\" encoding=\"UTF-8\"?><buku id=\"001\"><judul>Belajar Pemrograman Web</judul><penulis>John Doe</penulis><tahun>2026</tahun><harga mataUang=\"IDR\">120000</harga></buku>'",
}

# Negative sheet maps the same webhooks but the case-1 body differs
# (it's the positive case re-run for parity documentation).
NEGATIVE_CURLS = {
    1: "curl --location 'http://187.77.113.218:5678/webhook/webhook_code' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"1\"}'",
    2: "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_3' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"var\": \"a\" }'",
    3: "curl --location 'http://187.77.113.218:5678/webhook/webhook_callrest' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"var\": \"a\" }'",
    4: "curl --location 'http://187.77.113.218:5678/webhook/simple_webhook_2' \\\n--header 'Content-Type: application/json' \\\n--data '{ \"variable\": \"abcd\" }'",
    5: "curl --location 'http://187.77.113.218:5678/webhook/bocahtuanakal' \\\n--header 'Content-Type: application/json' \\\n--header 'Authorization: Basic YWRtaW46YWRtaW4xMjM=' \\\n--data '{this is not valid JSON'",
    6: "curl --location 'http://187.77.113.218:5678/webhook/webhook_xml' \\\n--header 'Content-Type: application/xml' \\\n--data '<buku><id>001</id></buku>'",
}


def _google_auth():
    from google.oauth2 import service_account
    from googleapiclient.discovery import build

    creds = service_account.Credentials.from_service_account_file(
        os.environ["GOOGLE_APPLICATION_CREDENTIALS"], scopes=SCOPES
    )
    return build("sheets", "v4", credentials=creds, cache_discovery=False)


def _verify_snapshot() -> None:
    """Sanity-check: each curl in our tables must appear in the snapshot."""
    if not SNAPSHOT.exists():
        print(f"WARNING: snapshot not found at {SNAPSHOT}", file=sys.stderr)
        return
    text = SNAPSHOT.read_text(encoding="utf-8")
    for n, curl in POSITIVE_CURLS.items():
        # Compare on the URL fragment + body fragment to avoid
        # spurious header differences.
        url = re.search(r"http://[^']+", curl).group(0)
        body = re.search(r"--data '[^']*'?", curl)
        if url not in text:
            print(f"  WARN positive case {n}: URL {url!r} not in snapshot",
                  file=sys.stderr)
        if body and body.group(0) not in text:
            print(f"  WARN positive case {n}: body {body.group(0)!r} not in snapshot",
                  file=sys.stderr)


def _write_single_cell(service, sheet_name: str, col: str, row: int,
                       value: str) -> None:
    """Write one cell without disturbing neighbours.

    Using a single-cell range (e.g. `C6`) avoids any chance of
    clobbering column D or others when paired with
    `values.update`.
    """
    rng = f"'{sheet_name}'!{col}{row}"
    service.spreadsheets().values().update(
        spreadsheetId=SPREADSHEET_ID,
        range=rng,
        body={"range": rng, "majorDimension": "ROWS", "values": [[value]]},
        valueInputOption="USER_ENTERED",
    ).execute()


def main() -> int:
    _verify_snapshot()
    service = _google_auth()

    for sheet_name, table in [
        ("Webhook Controlling", POSITIVE_CURLS),
        ("Webhook Controlling Negative Test", NEGATIVE_CURLS),
    ]:
        for n, curl in table.items():
            row = 5 + n
            _write_single_cell(service, sheet_name, "C", row, curl)
            _write_single_cell(service, sheet_name, "E", row, curl)
            print(f"  {sheet_name} row {row}: restored C + E")

    return 0


if __name__ == "__main__":
    sys.exit(main())
