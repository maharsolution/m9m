#!/usr/bin/env python3
"""
Read and write the m9m validation Google Sheet using a service
account (SA) so both reads and writes are authorised.

The SA key lives at `m9m/secrets/sheets-sa.json` and the
service-account email (`m9m-backend@telebot-976.iam.gserviceaccount.com`)
must be granted Editor access on the target sheet.

Usage:
    python scripts/sheets_sa.py read '<range>'                # print values
    python scripts/sheets_sa.py write '<range>' '[["..."]]'   # write 2D array
    python scripts/sheets_sa.py append '<range>' '[["..."]]'  # append rows
    python scripts/sheets_sa.py diag                          # auth + scope sanity check

Examples:
    python scripts/sheets_sa.py read 'Validasi N8N - M9M!A1:H10'
    python scripts/sheets_sa.py write "'Webhook Controlling'!G7:G7" '[["DONE"]]'
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

# Path to the service-account JSON (relative to repo root). Override
# with SHEETS_SA_JSON env var if the file is stored elsewhere.
_DEFAULT_SA = Path(__file__).resolve().parents[1] / "secrets" / "sheets-sa.json"
SA_JSON_PATH = Path(os.environ.get("SHEETS_SA_JSON", _DEFAULT_SA))

# Spreadsheet ID — the m9m vs n8n validation sheet.
SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"

# Scopes for read+write. The `spreadsheets` scope is enough for
# values.get / values.update / values.append; the `drive.file`
# scope is left off intentionally so the SA cannot browse Drive.
SCOPES = [
    "https://www.googleapis.com/auth/spreadsheets",
]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _build_service():
    """Build an authorised Sheets v4 service client."""
    try:
        from google.oauth2 import service_account
        from googleapiclient.discovery import build
    except ImportError as exc:
        raise SystemExit(
            "google-auth + google-api-python-client are required.\n"
            "Install with:\n"
            "    pip install --user google-auth google-api-python-client"
        ) from exc

    if not SA_JSON_PATH.exists():
        raise SystemExit(
            f"Service account JSON not found at {SA_JSON_PATH}.\n"
            "Set SHEETS_SA_JSON env var or place the file at the default path."
        )

    creds = service_account.Credentials.from_service_account_file(
        str(SA_JSON_PATH), scopes=SCOPES
    )
    return build("sheets", "v4", credentials=creds, cache_discovery=False)


def _resolve_range(range_name: str) -> str:
    """Pass A1 notation through untouched — callers must quote
    sheet names that contain spaces or dashes when needed."""
    return range_name


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------


def cmd_read(service, range_name: str) -> int:
    resp = (
        service.spreadsheets()
        .values()
        .get(spreadsheetId=SPREADSHEET_ID, range=_resolve_range(range_name))
        .execute()
    )
    values = resp.get("values", [])
    json.dump(values, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")
    return 0


def cmd_write(service, range_name: str, raw_values: str) -> int:
    values = json.loads(raw_values)
    body = {"range": _resolve_range(range_name), "majorDimension": "ROWS", "values": values}
    (
        service.spreadsheets()
        .values()
        .update(
            spreadsheetId=SPREADSHEET_ID,
            range=_resolve_range(range_name),
            body=body,
            valueInputOption="USER_ENTERED",
        )
        .execute()
    )
    print(f"wrote {sum(len(row) for row in values)} cell(s) to {range_name}")
    return 0


def cmd_append(service, range_name: str, raw_values: str) -> int:
    values = json.loads(raw_values)
    body = {"range": _resolve_range(range_name), "majorDimension": "ROWS", "values": values}
    (
        service.spreadsheets()
        .values()
        .append(
            spreadsheetId=SPREADSHEET_ID,
            range=_resolve_range(range_name),
            body=body,
            valueInputOption="USER_ENTERED",
            insertDataOption="INSERT_ROWS",
        )
        .execute()
    )
    print(f"appended {len(values)} row(s) to {range_name}")
    return 0


def cmd_diag(service) -> int:
    """Read the spreadsheet metadata so we can confirm the SA has
    at least read access (the spreadsheet resource itself is the
    cheapest probe)."""
    meta = service.spreadsheets().get(spreadsheetId=SPREADSHEET_ID).execute()
    title = meta.get("properties", {}).get("title", "?")
    sheets = [s["properties"]["title"] for s in meta.get("sheets", [])]
    print(f"title: {title}")
    print(f"sheets: {sheets}")
    return 0


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2

    cmd = argv[1]
    service = _build_service()

    if cmd == "read":
        if len(argv) != 3:
            print("usage: sheets_sa.py read '<range>'", file=sys.stderr)
            return 2
        return cmd_read(service, argv[2])
    if cmd == "write":
        if len(argv) != 4:
            print("usage: sheets_sa.py write '<range>' '<2D JSON>'", file=sys.stderr)
            return 2
        return cmd_write(service, argv[2], argv[3])
    if cmd == "append":
        if len(argv) != 4:
            print("usage: sheets_sa.py append '<range>' '<2D JSON>'", file=sys.stderr)
            return 2
        return cmd_append(service, argv[2], argv[3])
    if cmd == "diag":
        return cmd_diag(service)

    print(f"unknown command: {cmd}", file=sys.stderr)
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
