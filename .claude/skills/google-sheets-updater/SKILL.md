---
name: google-sheets-updater
description: Read and write the m9m validation Google Sheet programmatically using either a public API key (read-only) or a service account (read+write). Use this skill when the user wants to inspect, validate, or mark parity-test results (DONE / FAIL / status text) in the validation spreadsheet without manual browser interaction.
---

# google-sheets-updater

Read and update the m9m-vs-n8n validation Google Sheet at

    `https://docs.google.com/spreadsheets/d/1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ`.

The spreadsheet contains three sheets:
  * `Webhook Controlling` — positive test cases (No, name, n8n request/response, m9m request/response, code, status)
  * `Webhook Controlling Negative Test` — negative test cases
  * `Capabilty Gap` — known gaps

Data starts at **row 5** (header at row 1, description at rows 2–4).
Column layout:
  * A — `No` (case number, e.g. "1", "2", ...)
  * B — `Webhook Controling` (case name)
  * C — `Request` (n8n curl)
  * D — `Response` (n8n response)
  * E — `Request` (m9m curl, port-swapped 5678 -> 8080)
  * F — `Response` (m9m response)
  * G — `ResponseCode` (HTTP status)
  * H — `Status` (`OK` / `FAILED` / etc.)

## Auth model

Two auth paths are supported:

### 1. Service account (preferred — supports read+write)

The service-account JSON lives at `secrets/sheets-sa.json`. The
service-account email (`m9m-backend@telebot-976.iam.gserviceaccount.com`)
must already have been granted Editor access on the target
spreadsheet.

Required Python deps (already in the m9m dev image):
  * `google-auth`
  * `google-api-python-client`

### 2. Public API key (read-only — fallback)

The API key (`AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k`) only
works when the sheet is shared publicly ("Anyone with the link").
Reads work; writes return `403 PERMISSION_DENIED`. Use the SA
path for any update operation.

## Helper scripts

### Service account — `scripts/sheets_sa.py`

```bash
# Auth + scope sanity check
python scripts/sheets_sa.py diag

# Read a range
python scripts/sheets_sa.py read "'Webhook Controlling'!A5:H12"

# Mark case 6 (sheet row 11) as DONE with the m9m response
python scripts/sheets_sa.py write \
    "'Webhook Controlling'!F11:H11" \
    '[["<xml>...</xml>","200","OK"]]'

# Append a fresh row
python scripts/sheets_sa.py append \
    "'Webhook Controlling'!A:H" \
    '[["8","extra case","","","","","","OK"]]'
```

The `read` command prints a JSON 2D array to stdout. The
`write` / `append` commands take the value as a JSON-encoded
2D array on the command line so multi-line strings survive the
shell.

### API key — `scripts/update_sheet.py` (legacy)

```bash
python scripts/update_sheet.py read  'Validasi N8N - M9M!A1:H20'
python scripts/update_sheet.py write 'Validasi N8N - M9M!G7:G7' '[["DONE"]]'
python scripts/update_sheet.py append 'Validasi N8N - M9M!A:H' '[["..."]]'
```

Reads work with the API key. Writes require the SA path.

## Failure modes

- **403 PERMISSION_DENIED on write** — using the API key path; switch
  to `scripts/sheets_sa.py` which uses the service account.
- **403 PERMISSION_DENIED on read** — sheet isn't public AND the SA
  wasn't granted access. Ask the user to share the sheet with
  `m9m-backend@telebot-976.iam.gserviceaccount.com`.
- **400 INVALID_ARGUMENT** — range syntax wrong; A1 notation requires
  quotes around sheet names that contain spaces or dashes.
- **429 RATE_LIMITED** — back off and retry. Default retry is 3
  attempts with exponential backoff (1s, 2s, 4s).

## When NOT to use this skill

- Sheet has formulas referencing other sheets and you need to refresh
  those formulas — use the browser automation path instead, or send
  a manual `_refresh` via Apps Script.
- The cell value needs to be a complex object (rich text, hyperlinks,
  data validation) — the API only writes plain strings/numbers.
