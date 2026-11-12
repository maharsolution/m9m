# n8n / m9m Credential API Contract

_Updated 2026-11-12 from current local route/handler source and the canonical parity report. Not a new live API certification._

## Sources and scope

- m9m routes: `internal/api/routes.go`; envelopes/handlers: `internal/api/credentials.go`.
- n8n public API reference: [credentials](https://docs.n8n.io/connect/n8n-api/credential/).
- Canonical saved evidence: `tmp/parity-results/fase-d-2026-11-12/parity-results.json`, generated `2026-11-11T22:50:30.399439+00:00` (25/55 PASS; credentials suite 2/2 PASS; negative suite 4/5 PASS). Older reports under `tmp/parity-results/` are superseded and are not merged into these numbers.
- n8n base: `http://187.77.113.218:5678`; m9m base: `http://187.77.113.218:8080`; prefix `/api/v1`.

Do not conflate n8n's public API with its editor REST API. Endpoint availability, authorization and secret visibility differ. The former document's blanket claim that m9m had no credential API/store is obsolete.

## Current m9m routes

- `GET /credentials`: `{data: [...], nextCursor: null}`. List items omit secret `data`. `limit` defaults to 100, capped at 250; current handler truncates rather than implementing cursor pagination. Do not assume a complete inventory from one limited page.
- `POST /credentials`: accepts `name`, `type`, `data`, optional `id`/`isGlobal`; returns a data-stripped envelope. An ID-stable envelope alone does not supply usable authentication secrets.
- `GET /credentials/{id}`: envelope **with stored `data`**, for editor prefill - but ONLY for an authenticated editor session (JWT of an `admin`/`member` role) or an API key with the explicit `credentials:read-data` grant. Every other caller gets the list-style safe envelope without `data`. This intentionally differs from the n8n public API's secret-stripped contract.
- `GET /credentials/export` and `POST /credentials/import`: admin-only (owner session, or a key with the explicit `credentials:export` grant). Both write a structured audit entry (user, IP, credential names, timestamp - never values). The export keeps the n8n wire format: the credential payload stays encrypted.
- `PUT /credentials/{id}` and `PATCH /credentials/{id}`: update stored credential; changing type requires `data`. Current implementation merges credential data for partial updates.
- `DELETE /credentials/{id}`: successful deletion returns **204**, not an assumed n8n 200.
- `POST /credentials/{id}/test`: validates the stored credential.
- `POST /credentials/test`: validates unsaved `{type, data}` form values.
- `GET /credentials/types`: `{data: [...]}` type catalog.
- `GET /credentials/schema/{type}`: local credential schema; unknown type returns 404. Do not assume the schema is byte-identical to n8n JSON Schema.
- `PUT /credentials/{id}/transfer`: registered project-transfer route; route presence alone does not certify n8n project semantics.

Test responses use `{status: "OK" | "Error", message: "..."}`. Validation/reachability support varies by credential type; a successful field validation is not proof every provider can authenticate or execute.

Envelope fields include `id`, `name`, `type`, `isManaged`, `isGlobal`, `isResolvable`, `resolvableAllowFallback`, optional `resolverId`, `createdAt`, `updatedAt`; detail reads may include `data`.

## Secret handling

**m9m detail reads can expose credential secrets. Restrict API access to authorized operators, use encrypted transport, never copy detail payloads into parity reports, logs or Google Sheets.** Public credential envelopes from n8n cannot reconstruct secrets. Provision secrets through an authorized secret-management path; never infer successful authentication from name/ID matching.

The API itself enforces this: the secret `data` blob of a credential is only served to an editor session or to a key with the explicit `credentials:read-data` grant, and the credential export/import bundle is admin-only. Both paths write a structured audit entry that carries metadata only (user, IP, credential names, timestamp) and never a value. The parity evidence files under `tmp/parity-results/` are redacted in place for the same reason: a credential `data` object that is captured in a raw response body must not keep a live password, an SSH private key or a connection string.

## API authentication of the credential routes

Every route under `/api/v1` - the credential routes included - requires a credential:

| Caller | `GET /api/v1/credentials` | `GET /api/v1/credentials/{id}` | `/credentials/export`, `/credentials/import` |
|--------|--------------------------|-------------------------------|-----------------------------------------------|
| no credential | 401 | 401 | 401 |
| wrong API key | 401 | 401 | 401 |
| valid API key (no grant) | 200 (envelope) | 200 (**no `data`**) | 403 |
| valid API key + `credentials:read-data` | 200 (envelope) | 200 (with `data`) | 403 unless `credentials:export` |
| JWT `viewer` | 200 (envelope) | 200 (**no `data`**) | 403 |
| JWT `member` (editor) | 200 (envelope) | 200 (with `data`) | 403 |
| JWT `admin` (owner) | 200 (envelope) | 200 (with `data`) | 200 |

The key is presented in the `X-API-Key` header only; a key in the query
string is rejected. The env contract is documented in
`documentation/docs/api/authentication.md` and in the `security:` block of
`config/config.yaml`.

## Workflow mapping and sync requirements

Preserve `nodes[*].credentials = {typeName: {id, name}}` across import/export. Provision referenced credentials before executing workflows; retain stable IDs where supported. Compare envelopes and workflow references separately from secret values. Missing credentials must not silently downgrade execution to anonymous.

This document does not change `sync-service/sync.py`, assert that every sync provider is implemented, or authorize extracting database secrets. Verify the actual sync path before claiming complete credential parity.

## Dynamic options (`loadOptions`) resolve credentials server-side

The Google Sheets resource locator is served by a `loadOptions` provider, not by the browser:

- `cmd/m9m/commands/load_options.go` registers the provider for node type `n8n-nodes-base.googleSheets`, property `spreadsheetId`, action `loadSpreadsheets`; `registerLoadOptionProviders()` is called at server start (`cmd/m9m/commands/serve.go`).
- The provider resolves the OAuth access token through the server-side credential store: `CredentialResolver.AccessTokenForType("googleSheetsOAuth2Api")` (`internal/api/node_type_options.go`, `internal/credentials/manager.go`). The token is read from the n8n `oauthTokenData.access_token` envelope, the JSON-encoded variant or a flat `accessToken` field, and environment placeholders are resolved before use.
- The listing itself comes from the Drive API (`internal/nodes/productivity/google_sheets_options.go`), because the Sheets API has no "list spreadsheets" endpoint — the same approach n8n's `loadSpreadsheets` action uses.
- **The browser never receives the token.** The panel asks the backend for the option list; the backend answers with the resource names/ids it already holds. A plain spreadsheet id stays valid, so workflows saved before the locator existed keep working.

## Authentication rejection wire format (implemented)

The rejection path of the webhook handler mirrors n8n exactly (`internal/webhooks/auth_failure.go`, `internal/webhooks/jwt.go`):

| Situation | n8n | m9m (implemented) |
|---|---|---|
| no credentials presented (basic auth, no header) | HTTP 401, plain text `Authorization is required!` | HTTP 401, same body |
| basic auth with wrong credentials | HTTP 401, plain text `Authentication data is wrong!` | HTTP 401, same body |
| bearer token missing | HTTP 401, plain text `No token provided` | HTTP 401, same body |
| bearer token present but not acceptable | HTTP 403, plain text `invalid token` | HTTP 403, same body |
| API key / header auth missing or wrong | HTTP 403, plain text `Authorization data is wrong!` | HTTP 403, same body |

Shared properties of every rejection:

- **401 versus 403 is meaningful**: 401 means "no credentials were presented", 403 means "credentials were presented and are not acceptable".
- The body is **plain text**, not a JSON envelope.
- **No `Content-Type` header is sent.** Go's `net/http` would sniff the body and add `text/plain; charset=utf-8`; the handler suppresses the header entry so the key stays present and the sniffer stays off.
- Every rejection carries the challenge `WWW-Authenticate: Basic realm="Webhook"`, whatever the configured auth type.
- The JSON envelope for the other webhook error paths (404 not registered, 400/422 bad body) is unchanged — n8n sends the same `{code,message,hint}` JSON there.

Live evidence (read-only webhook POSTs with dummy credentials, 2026-11-12, `http://187.77.113.218:5678` vs `http://187.77.113.218:8080`): both hosts answered 401/401/403/403/403 with no `Content-Type` and the same challenge header, and identical bodies for the missing-credentials, wrong-basic-auth and wrong-API-key cases.

One residual difference: for a bearer token that is not a JWT the n8n body is `jwt malformed`, and for a token signed with a disallowed algorithm it is `invalid algorithm`, while the deployed m9m build still answers `invalid token`. The current working-tree source implements the full n8n error taxonomy (`jwt malformed`, `invalid algorithm`, `jwt signature is required`, `jwt expired`, `jwt not yet valid`), but that build is not deployed. Tracked as `GAP-2026-11-12-JWT-ERROR-BODY` in `parity/parity_gaps.json`.

## SQL nodes emit one item per row

Database nodes return one `model.DataItem` per row (`internal/nodes/database/rows.go`, `scanRows`). The webhook response is an array of row objects; it is **not** a single object wrapping a `rows` array. The column values are passed through untouched, so the driver decides whether a column arrives as a string, a number, a bool, `[]byte` or nil, the same way n8n's drivers do.

Live evidence (canonical report, 2026-11-12): the MySQL branch (`p12`) and the PostgreSQL branch (`p14`) each answer HTTP 200 with **10 array items on both hosts**, and the item key sets are identical (`created_at, data, id, name, type, updated_at` for p12; `createdAt, data, id, isGlobal, isManaged, isResolvable, name, resolvableAllowFallback, resolverId, type, updatedAt, usageScope` for p14). The remaining difference is the row payload itself — the credential envelope encoding and the id/name/timestamp values — not the row-per-item shape. Tracked as `GAP-2026-11-11-DATABASE-BODY`.

## Latest evidence and remaining verification

Canonical report `tmp/parity-results/fase-d-2026-11-12/parity-results.json` (25/55 PASS). Credentials-relevant verdicts only:

- PASS: `c101` basic-auth credential resolution in a workflow and `c103` HTTP Request with embedded credentials (`webhook_callrest`) — credentials suite **2/2 PASS**. `p05` (`webhook_callrest`) and `p11` (`process-batch-orders`) also PASS.
- PASS: `n01`, `n02`, `n05` — both hosts answer HTTP 401 with the identical plain-text body, no `Content-Type` and the `WWW-Authenticate` challenge. The negative suite is now **4/5**.
- FAIL: `n04` — both hosts answer HTTP 403 with the identical body `invalid token`; the case expects HTTP 401, which contradicts n8n's own behaviour. The case definition belongs to the parity worker.
- FAIL: `p08` JWT bearer and `p09` API key header — both hosts answer HTTP 403. `p09` returns the identical body on both hosts; `p08` differs only in the error text (see above). Both cases expect HTTP 200, so they also fail because the stored fixture credential is not accepted by n8n either. Fixture credential validity remains unproven, so no authenticated success is inferred.
- FAIL: `p12` and `p14` — the `db_quiry` branches return HTTP 200 on both hosts with the same row-per-item shape but different row payloads. Dataset drift versus execution divergence is still open.
- FAIL: `c105` credential inventory — the n8n REST API answered HTTP 200, so the comparison ran and reported "credential names/types differ" instead of being blocked by 401.
- FAIL: `c201` roundtrip, `c501` telemetry, `c601`/`c605`/`c701` sync — n8n REST API HTTP 200, comparison ran, "nodes/parameters/credentials/connections differ". The earlier 401 blocker on the n8n REST API is gone; the payload differences are what remains.
- Still BLOCKED (no comparison possible): `c301` expression runtime contract, `c602`–`c613` n8n C6 sync fixtures not found, `c702`, `c802`, `c901`, `c1001`, `c1101`, `c1201`.

Security note tied to the parity run: the `db_quiry` responses (rows 14 and 15 of the `Webhook Controlling` tab) contain credential rows and encrypted credential blobs. Those bodies were withheld from the Google Sheet and from this document by the secret guard in `scripts/update_parity_sheets.py`; only the HTTP status and the comparison status were written. Endpoint presence does not close the residual credential gap.
