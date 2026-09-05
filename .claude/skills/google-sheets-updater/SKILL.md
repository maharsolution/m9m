---
name: google-sheets-updater
description: Update Google Sheets via the public Sheets API v4 using a service-account-style API key. Use this skill when the user wants to mark parity-test results (DONE / FAIL / status text) in the validation spreadsheet without manual browser interaction.
---

# google-sheets-updater

Update a Google Sheet programmatically using a public API key and the Sheets
REST API v4. The skill targets the m9m-vs-n8n validation spreadsheet at
`https://docs.google.com/spreadsheets/d/1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ`.

## Auth model

Public-sheet API-key auth (no OAuth, no service account). This only works when
the sheet is shared publicly ("Anyone with the link") — otherwise the API
returns `403 PERMISSION_DENIED`. The API key is held as a constant below
rather than read from env so the skill is self-contained.

```text
API_KEY = AIzaSyC-SHoCO1DoZp3tOuyYa_W7AVVfr2tKf6k
SPREADSHEET_ID = 1PAz38gDk2Fa_6SlBtC3fWss3YfL_KgroEAwCpiY7aDQ
```

## Endpoint shapes

- Read a range: `GET https://sheets.googleapis.com/v4/spreadsheets/{ID}/values/{RANGE}?key={KEY}`
- Write a range: `PUT https://sheets.googleapis.com/v4/spreadsheets/{ID}/values/{RANGE}?valueInputOption=USER_ENTERED&key={KEY}` with body `{"range":"...", "values":[[...]]}`
- Append rows: `POST .../values/{RANGE}:append?valueInputOption=USER_ENTERED&key={KEY}` with same body shape
- Batch update: `POST .../values:batchUpdate?key={KEY}` with `{"valueInputRange":{"range":..., "values":[[...]]}}`

## Common ranges for the validation sheet

- `Validasi N8N - M9M!A:H` — full table (cases + status)
- `Validasi N8N - M9M!G:G` — Status column only
- `Validasi N8N - M9M!H:H` — Notes column

## Helper script

`scripts/update_sheet.py` (created alongside this skill) wraps the three
common operations:

```bash
# Read a range
python scripts/update_sheet.py read 'Validasi N8N - M9M!A1:H20'

# Mark case 6 as DONE
python scripts/update_sheet.py write 'Validasi N8N - M9M!G7:G7' '[["DONE"]]'

# Append a fresh row
python scripts/update_sheet.py append 'Validasi N8N - M9M!A:H' '[["7","extra","","","","","","OK"]]'
```

## Failure modes

- **403 PERMISSION_DENIED** — sheet isn't public. Ask the user to either
  set the sharing to "Anyone with the link" or provide a service-account JSON.
- **400 INVALID_ARGUMENT** — range syntax wrong; A1 notation requires
  quotes in the URL path which `requests` handles automatically if you pass
  the range as a query path segment.
- **429 RATE_LIMITED** — back off and retry; the API key has a per-minute
  quota. Default retry is 3 attempts with exponential backoff (1s, 2s, 4s).

## When NOT to use this skill

- Sheet has formulas referencing other sheets and you need to refresh those
  formulas — use the browser automation path instead, or send a manual
  `_refresh` via Apps Script.
- The cell value needs to be a complex object (rich text, hyperlinks,
  data validation) — the API only writes plain strings/numbers.
