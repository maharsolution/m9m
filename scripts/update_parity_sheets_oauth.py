#!/usr/bin/env python3
"""Push the final m9m-vs-n8n parity verdicts into the validation
Google Sheets using an OAuth2 service account.

Prerequisites (one-time, ~5 min):

1. Create a Google Cloud project (or reuse an existing one).
2. Enable the Google Sheets API for that project.
3. Create a service account (no IAM roles needed — Sheets API
   reads the sheet's sharing settings for authorization).
4. Download the service account JSON key.
5. Place it somewhere readable, e.g.
       /path/to/m9m/secrets/sheets-sa.json
6. Share the target spreadsheet with the service account's email
   (found in the JSON key as `client_email`) — Editor permission.

Usage:

    export GOOGLE_APPLICATION_CREDENTIALS=/path/to/sheets-sa.json
    python scripts/update_parity_sheets_oauth.py [--date 2026-09-05]

The script walks `tmp/validation-<date>/case_*.json` (positive) and
`tmp/validation-<date>/negative/case_*.json` (negative) and writes
the four output columns (D = N8N Response, F = M9M Response,
G = ResponseCode, H = Status) for rows 6-11 of each sheet.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys

# Service-account scope. `spreadsheets` lets us both read and write.
SCOPES = ["https://www.googleapis.com/auth/spreadsheets"]
SPREADSHEET_ID = "1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ"


def _google_auth():
    """Return an authenticated `googleapiclient` Sheets client.

    Requires google-auth and google-api-python-client (already in
    go.mod-adjacent vendored deps? If not, `pip install --user
    google-auth google-api-python-client`).
    """
    try:
        from google.oauth2 import service_account
        from googleapiclient.discovery import build
    except ImportError as e:
        raise SystemExit(
            "missing deps — run: pip install --user google-auth google-api-python-client"
        ) from e

    creds = service_account.Credentials.from_service_account_file(
        # Use the env var if set, otherwise fall back to the standard
        # location in the workspace's secrets/ dir.
        pathlib.Path(
            __import__("os").environ.get(
                "GOOGLE_APPLICATION_CREDENTIALS",
                "secrets/sheets-sa.json",
            )
        ),
        scopes=SCOPES,
    )
    return build("sheets", "v4", credentials=creds, cache_discovery=False)


def _swap_port(curl_text: str, src_port: int, dst_port: str) -> str:
    """Return curl text with `localhost:src_port` / `:src_port` swapped.

    n8n lives on 5678 by default; m9m is on 8080. We only rewrite the
    port inside the URL token of the curl invocation. Other `:NNNN`
    occurrences (e.g. inside headers, JSON bodies) are left intact.
    """
    import re

    # Match :<port> immediately following `localhost` or an IP literal,
    # optionally wrapped in single quotes by the user when copy-pasted.
    pattern = re.compile(
        r"(localhost|127\.0\.0\.1|\d{1,3}(?:\.\d{1,3}){3})"
        rf"(:{src_port})\b"
    )
    return pattern.sub(rf"\1:{dst_port}", curl_text)


def _read_n8n_request(service, sheet_name: str, row: int) -> str | None:
    """Fetch the existing M9M Request cell (column C) so we can swap
    the port and write it back to column E. Returns None if empty.
    """
    rng = f"'{sheet_name}'!C{row}"
    resp = (
        service.spreadsheets()
        .values()
        .get(spreadsheetId=SPREADSHEET_ID, range=rng)
        .execute()
    )
    values = resp.get("values", [])
    if not values or not values[0]:
        return None
    return str(values[0][0])


def _update_row(service, sheet_name: str, row: int, m9m_request: str,
                n8n_resp: str, m9m_resp: str, code: int,
                status: str) -> None:
    """Write the output columns D, E, F, G, H for row N.

    Layout written (with intentional blanks for untouched columns):

        C (M9M Request, untouched)  D (N8N Response)  E (M9M Request, port-swapped)
        F (M9M Response)            G (ResponseCode)  H (Status)

    Columns A, B, and C are left untouched. The script uses
    `valueInputOption=USER_ENTERED` so JSON-like responses are stored
    as strings (a leading `{` would otherwise be interpreted as a
    formula by Sheets).
    """
    rng = f"'{sheet_name}'!D{row}:H{row}"
    values = [[
        n8n_resp.strip(),
        m9m_request.strip(),
        m9m_resp.strip(),
        str(code),
        status,
    ]]
    body = {"range": rng, "majorDimension": "ROWS", "values": values}
    service.spreadsheets().values().update(
        spreadsheetId=SPREADSHEET_ID,
        range=rng,
        body=body,
        valueInputOption="USER_ENTERED",
    ).execute()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--date", default="2026-09-05")
    args = parser.parse_args()

    pos_dir = pathlib.Path("tmp") / f"validation-{args.date}"
    neg_dir = pos_dir / "negative"
    if not pos_dir.is_dir() or not neg_dir.is_dir():
        print(f"missing validation dir: {pos_dir}", file=sys.stderr)
        return 1

    service = _google_auth()

    # Positive sheet rows 6..11.
    for n in range(1, 7):
        path = pos_dir / f"case_{n}.json"
        if not path.exists():
            print(f"  skip positive case {n}: {path} missing")
            continue
        data = json.loads(path.read_text())
        status = "OK" if data["verdict"] == "PASS" else "FAILED"
        # Read existing M9M Request (column C) and port-swap it for column E.
        existing_m9m_curl = _read_n8n_request(
            service, "Webhook Controlling", 5 + n
        ) or ""
        m9m_curl = _swap_port(existing_m9m_curl, 5678, "8080")
        _update_row(
            service,
            "Webhook Controlling",
            row=5 + n,
            m9m_request=m9m_curl,
            n8n_resp=data["n8n"]["body_raw"] or "{}",
            m9m_resp=data["m9m"]["body_raw"] or "{}",
            code=data["m9m"]["status_code"] or 0,
            status=status,
        )
        print(f"  positive case {n} -> row {5 + n} status={status}")

    # Negative sheet rows 6..11.
    for n in range(1, 7):
        path = neg_dir / f"case_{n}.json"
        if not path.exists():
            print(f"  skip negative case {n}: {path} missing")
            continue
        data = json.loads(path.read_text())
        status = "OK" if data["verdict"] == "PASS" else "FAILED"
        existing_m9m_curl = _read_n8n_request(
            service, "Webhook Controlling Negative Test", 5 + n
        ) or ""
        m9m_curl = _swap_port(existing_m9m_curl, 5678, "8080")
        _update_row(
            service,
            "Webhook Controlling Negative Test",
            row=5 + n,
            m9m_request=m9m_curl,
            n8n_resp=data["n8n"]["body_raw"] or "{}",
            m9m_resp=data["m9m"]["body_raw"] or "{}",
            code=data["m9m"]["status_code"] or 0,
            status=status,
        )
        print(f"  negative case {n} -> row {5 + n} status={status}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
