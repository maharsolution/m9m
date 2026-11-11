# Credentials

Reference for the m9m credentials subsystem: how a credential is stored, how it is
validated, how it is verified against the real service, and what the editor UI
guarantees to the user.

- API: `internal/api/credentials.go`, `internal/api/credential_conn.go`, `internal/api/routes.go`
- UI: `web/src/views/Credentials.vue`, `web/src/api/credentials.ts`, `web/src/stores/credentials.ts`
- Types: `web/src/types/api.ts`

---

## Data flow

```
Credentials.vue  (form state: type + data map)
      │
      │  POST /api/v1/credentials/test   { type, data }
      ▼
TestCredentialPayload                    internal/api/credentials.go
      │
      ▼
validateCredentialData(credType, data)    internal/api/credentials.go
      │  1. schema lookup (credentialSchemas)
      │  2. required-field check (alias-aware)
      │  3. type-specific format checks (OAuth2 URLs, postgres/mysql port range,
      │     mongoDb connectionString-or-host)
      ▼
testCredentialConnection(credType, data)  internal/api/credential_conn.go
      │  returns (result, attempted)
      │  attempted == false  → the type cannot be verified over the wire,
      │                        static validation is the final answer
      │  attempted == true   → the per-type driver below produced the result
      ▼
per-type driver  (postgres / mysql / mongoDb / redis / kafka / rabbitmq /
                  openai / anthropic / slack)
      ▼
{ "status": "OK" | "Error", "message": "…" }
```

Two endpoints reach `validateCredentialData`:

| Endpoint | Handler | Input |
| --- | --- | --- |
| `POST /api/v1/credentials/test` | `TestCredentialPayload` | body `{ type, data }` — an **unsaved** payload, so the test reflects the current form |
| `POST /api/v1/credentials/{id}/test` | `TestCredential` | the credential stored under `{id}` |

### Route ordering

`internal/api/routes.go` registers the static paths **before** `/credentials/{id}`:

```go
api.HandleFunc("/credentials/schema/{type}", s.GetCredentialSchema).Methods("GET", "OPTIONS")
api.HandleFunc("/credentials/types", s.ListCredentialTypes).Methods("GET", "OPTIONS")
api.HandleFunc("/credentials/test", s.TestCredentialPayload).Methods("POST", "OPTIONS")
api.HandleFunc("/credentials/{id}", s.GetCredential).Methods("GET", "OPTIONS")
```

gorilla/mux matches routes in registration order, so registering `/credentials/test`
after `/credentials/{id}` would route the payload test to the single-credential
handler with `id = "test"`.

### Data exposure

| Endpoint | `data` in the response |
| --- | --- |
| `GET /api/v1/credentials` | stripped (`toCredentialResponse`) |
| `GET /api/v1/credentials/{id}` | present (`toCredentialResponseWithData`) — this is what pre-fills the edit modal |

`PATCH` / `PUT` merge rather than replace: `mergeCredentialData` overlays only the
fields present in the request body, so a partial update never wipes untouched secrets.

---

## Per-type verification matrix

Every attempt is bounded by `credentialConnectTimeout = 10 * time.Second`
(`internal/api/credential_conn.go`).

| Credential type | Driver / library | What is verified | Success signal | Typical failure message |
| --- | --- | --- | --- | --- |
| `postgres`, `postgresql` | `database/sql` + `lib/pq` | `sql.Open("postgres", url)` + `PingContext`; `sslmode` from the `ssl` field | `Connected to PostgreSQL <host>/<db> as <user>` | `PostgreSQL connection failed: …` |
| `mysql`, `mySql` | `database/sql` + `go-sql-driver/mysql` | `sql.Open("mysql", dsn)` + `PingContext`; DSN carries `timeout`/`readTimeout`/`writeTimeout` and optional `tls` | `Connected to MySQL <host>/<db> as <user>` | `MySQL connection failed: …` |
| `mongoDb`, `mongodb` | official `mongo-driver` | `mongo.Connect` + `client.Ping`; accepts a `mongodb://` / `mongodb+srv://` URI, or a host/port/user/password form | `Connected to MongoDB database <db>` | `MongoDB connection failed: …` |
| `redis` | raw RESP over TCP | `AUTH [user] password` then `PING`; the reply is parsed and a `-` prefix is treated as an error | `Connected to Redis <host>:<port>` | `Redis connection failed: …`, `Redis authentication failed: …`, `Redis check failed: …` |
| `kafka` | `segmentio/kafka-go` | Native mode: TLS + SASL handshake with the **selected** mechanism (`plain.Mechanism`, `scram.SHA256`, `scram.SHA512`) followed by a real `Metadata` request answered by the broker (`kafkaMetadata`). REST-proxy mode: an authenticated `GET <restProxyUrl>/topics` (Basic auth or `X-Api-Key`) | `Connected to Kafka broker(s) <brokers> (authenticated as <user>, <n> broker(s) reported)`, `Connected to the Kafka REST Proxy at <url>` | `Kafka authentication failed: …`, `Kafka connection failed: …`, `Kafka CA certificate could not be parsed` |
| `rabbitmq` | `rabbitmq/amqp091-go` | full AMQP 0-9-1 handshake via `amqp.DialConfig` with `amqp.PlainAuth`; the configured `vhost` is part of the dial | `Connected to RabbitMQ <host>:<port> (vhost <vhost>, authenticated as <user>)` | `RabbitMQ authentication failed: …`, `RabbitMQ connection failed: …` |
| `openai`, `openAiApi` | `net/http` | authenticated `GET https://api.openai.com/v1/models` with `Authorization: Bearer <key>` | `OpenAI API key is valid` | `OpenAI rejected the API key (HTTP <code>): <body>`, `OpenAI request failed: …` |
| `anthropic` | `net/http` | authenticated `GET https://api.anthropic.com/v1/models` with `x-api-key` and `anthropic-version: 2023-06-01` | `Anthropic API key is valid` | `Anthropic rejected the API key (HTTP <code>): <body>`, `Anthropic request failed: …` |
| `slack` | `net/http` | authenticated `GET https://slack.com/api/auth.test`; success also requires `"ok":true` in the body | `Slack token is valid` | `Slack rejected the token (HTTP <code>): <body>`, `Slack request failed: …` |

Types with no entry in this table (`httpBasicAuth`, `httpHeaderAuth`, `oAuth2Api`,
`apiKey`, `discord`, `jwtAuth`, …) cannot be verified over the wire.
`testCredentialConnection` returns `attempted == false` for them and the static
validation result stands.

### Kafka helpers

- `kafkaCredentialUserPass(data)` — resolves the **single** username / password pair
  of the credential. The canonical keys are `username` / `password` (n8n parity — see
  `n8n/packages/nodes-base/credentials/Kafka.credentials.ts`); `saslUsername` /
  `saslPassword` and the generic `user` alias are honoured as fallbacks. The canonical
  key always wins when it holds a value, so a stale alias left over from an earlier
  save can never shadow the username the operator just edited.
- `kafkaSASLMechanism(data)` — normalises the mechanism: n8n stores the lowercase
  spelling (`plain`, `scram-sha-256`, `scram-sha-512`), m9m the uppercase one.
- `kafkaSASLMechanisms(mechanism, user, password)` — an explicitly selected mechanism
  is used on its own; only when no mechanism is configured does the test fall back to
  PLAIN → SCRAM-SHA-256 → SCRAM-SHA-512.
- `kafkaBrokerList(data)` — splits the comma-separated `brokers` field, defaulting a
  bare host to port `9092`.
- `kafkaTLSConfig(data)` — builds the TLS config (min TLS 1.2) and adds the
  credential's `ca` certificate to the system trust store.
- `kafkaMetadata(brokers, tlsCfg, mech)` — performs the metadata request and returns
  the number of brokers the cluster reported. The connection test calls it through
  the `kafkaMetadataFunc` seam so the unit tests can stub the broker.
- `kafkaRESTAuthMode(data)` — reads the REST-proxy authentication selector
  (`none` / `basic` / `apiKey`). n8n stores a boolean in the same field; that value
  keeps the credential on the native SASL path.
- `testKafkaRESTConnection(...)` — probes `<restProxyUrl>/topics` with Basic auth or
  `X-Api-Key`. HTTP 401 / 403 is reported as `Kafka authentication failed`.
- `isKafkaAuthError(err)` — distinguishes "the broker refused the credentials" from a
  transport failure, so the fallback loop only continues on an auth rejection.
- `scrubKafkaSecrets(msg, user, password)` — scrubs the password **and** the username
  from an error message.

#### Kafka connection-test behaviour

| Credential state | What the test does |
| --- | --- |
| `brokers` + username + mechanism | SASL handshake with **that** mechanism, then a metadata request. A wrong username or password ⇒ `Kafka authentication failed`. |
| `brokers` + username, no mechanism | Falls back to PLAIN → SCRAM-SHA-256 → SCRAM-SHA-512 (in that order) until one succeeds. |
| `brokers` + mechanism, **no** username | Explicit error — the test never falls back to an unauthenticated metadata request that the broker happens to allow. |
| `brokers`, no credentials at all | Anonymous metadata request (a legitimate broker configuration). |
| `restProxyUrl` (+ `authentication: basic` / `apiKey`), or `restProxyUrl` without `brokers` | `GET <restProxyUrl>/topics` with the credentials. HTTP 401 / 403 ⇒ `Kafka authentication failed`. |
| `authentication: basic` / `apiKey` without `restProxyUrl` | Explicit error — the REST Proxy URL is required. |

Mode selection: an explicit `authentication: basic` / `apiKey` always means REST-proxy
mode; a `restProxyUrl` without `brokers` means REST-proxy mode as well. When both
`brokers` and `restProxyUrl` are stored and no authentication mode is selected, the
native broker path is exercised (that is the n8n-parity path — an n8n credential
never carries a REST Proxy URL).

### Kafka credential fields

The schema (`credentialSchemas["kafka"]` in `internal/api/credentials.go`) declares
**exactly one** username and **exactly one** password:

| Property | Type | Purpose |
| --- | --- | --- |
| `brokers` | string | Comma-separated bootstrap servers (native mode). |
| `clientId` | string | Client id surfaced in the broker logs. |
| `saslMechanism` | options | `None` / `PLAIN` / `SCRAM-SHA-256` / `SCRAM-SHA-512`. |
| `username` | string | **The** username — SASL (native) and Basic auth (REST Proxy). |
| `password` | password | **The** password — SASL (native) and Basic auth (REST Proxy). |
| `tls` | boolean | TLS for the native broker connection. |
| `restProxyUrl` | string | Confluent-compatible REST Proxy base URL. |
| `authentication` | options | `none` / `basic` / `apiKey` for the REST Proxy. |
| `apiKey` | password | API key (alternative to Basic) for the REST Proxy. |

The legacy `saslUsername` / `saslPassword` spellings are still accepted as **aliases**
(see [Field aliases](#field-aliases)) — they are never declared as a second pair of
properties, so the editor shows a single username input and a single password input.

The node side (`internal/nodes/messaging/kafka.go`, `ApplyKafkaCredentialAuth`)
cross-bridges `username` ↔ `saslUsername` and `password` ↔ `saslPassword`, so a
credential that stores only the canonical names still authenticates against the
native brokers, and one that stores only the `sasl`-prefixed names still authenticates
against a REST proxy with Basic auth.

### RabbitMQ helpers

- `testRabbitMQConnection(data)` — builds `amqp.Config` with the vhost, `en_US`
  locale and a 5-second dial timeout, then `amqp.DialConfig`.
- `isAMQPAuthError(err)` — matches `ACCESS_REFUSED`, `access refused`,
  `authentication`, `login was refused` and `not allowed`.

---

## Field aliases

Synced n8n credentials frequently use a different key for the same field than the
m9m schema declares — n8n's RabbitMQ credential stores `hostname` / `username`
where the schema says `host` / `user`.

`credentialFieldAliases` in `internal/api/credential_conn.go`:

| Schema field | Accepted aliases |
| --- | --- |
| `host` | `hostname`, `server`, `serverName` |
| `user` | `username`, `login`, `loginName` |
| `database` | `db`, `dbName`, `databaseName` |
| `password` | `pass`, `pwd` |

The aliases are resolved by `credentialFieldValue`, and read through `dataString` /
`dataInt`. They apply to:

- the required-field check in `validateCredentialData`,
- every connection driver (so the driver dials the right host with the right user),
- the UI prefill (see [UI contract](#ui-contract)).

Type-specific aliases live in the UI (`web/src/views/Credentials.vue`,
`typeFieldAliases`), because they are driven by the credential's own schema:

| Schema field (type `kafka`) | Accepted aliases |
| --- | --- |
| `username` | `saslUsername`, `user` |
| `password` | `saslPassword`, `pass`, `pwd` |

The alias keys are folded into the canonical field and then removed from the form, so
the editor renders exactly one username input and one password input, and the
"Additional stored fields" section does not list a second copy of either. The generic
`user` ← `username` alias is skipped for Kafka (`typeAliasBlocklist`), because it
would create a third username input.

The backend mirrors the same resolution in `kafkaCredentialUserPass`
(`internal/api/credential_conn.go`) for the connection test, so the test always uses
the login the operator sees in the form.

---

## PEM normalisation

`normalizePEM(raw)` in `internal/api/credential_conn.go` rebuilds a PEM block before
it reaches `x509.CertPool.AppendCertsFromPEM`.

**Why it is needed.** A certificate that arrives through the n8n sync bridge can be
stored flattened — every newline replaced by a space, or with literal `\n`
two-character sequences instead of real line breaks. `x509` requires a
`-----BEGIN …-----` / `-----END …-----` envelope with a base64 body wrapped in
64-column lines; a flattened blob fails with `Kafka CA certificate could not be
parsed` even though the certificate itself is valid.

**What it does.**

1. Replaces literal `\n` sequences with real newlines, and normalises `\r\n` / `\r`
   to `\n`.
2. Locates the `-----BEGIN …-----` header and the `-----END …-----` footer.
3. Strips every whitespace character from the base64 body.
4. Re-wraps the body at 64 columns and reassembles header + body + footer.

If no `-----BEGIN` marker is present the input is returned unchanged, so a
non-PEM value never crashes the test.

---

## Secret scrubbing

`scrubSecret(msg, secret)` replaces every occurrence of the secret in a driver
error message with `***`:

```go
func scrubSecret(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "***")
}
```

It is applied to every error path in `internal/api/credential_conn.go` — database
driver errors, broker handshakes and HTTP probes alike — so a password, API key or
access token never reaches the browser through the test response. The secret
scrubbing is applied to the *message*, not to the stored credential.

`truncate(s, max)` shortens API error bodies (HTTP probe failures) to 180 characters
plus an ellipsis so a large upstream HTML page cannot flood the UI.

---

## UI contract

`web/src/views/Credentials.vue`

**Prefill.** `openEditModal` fetches the credential through
`credentialsStore.fetchCredential(id)` → `GET /api/v1/credentials/{id}`, which returns
the stored `data` map, and applies `applyFieldAliases` so schema fields are filled
even when the stored key is an alias. Schema defaults are applied **only** for new
credentials — an existing credential keeps its stored values.

**Reveal toggle.** `isSecretField(key)` matches
`pass(word|wd)?|secret|token|api[_-]?key|connectionstring|credential|private`.
Matching fields render as `type="password"` with an eye button; `toggleReveal(name)`
flips the field in the `revealedFields` set. The toggle covers the schema-driven
form, the "additional stored fields" list (`extraDataFields`) and the
no-schema fallback list.

**Draft persistence.** The edit form is mirrored into `sessionStorage` under the key
`m9m:credential-edit-draft`:

```ts
const DRAFT_KEY = 'm9m:credential-edit-draft'
```

The draft holds `{ id, name, type, data, scrollTop, savedAt }`. `writeDraft()` runs
on a 250 ms debounce after every form change; `readDraft()` restores the modal on
mount; `clearDraft()` runs on save and on cancel. This is what stops a discarded or
reloaded browser tab (Chrome discards background tabs) from returning an empty
form.

**Scroll restoration.** The modal panel is `overflow-y-auto overscroll-contain`.
`panelScrollTop` tracks the scroll offset on every `scroll` event, and
`restorePanelScroll(scrollTop)` re-applies it after the schema loads or a test
result banner re-renders. A watcher on `[testResult, loadingCredential,
currentSchema, schemaLoading]` puts the user back where they were when a re-render
reset the panel to the top.

**Select fields.** `optionValues(prop)` adds the stored value as an extra option
when it is not one of the schema options (e.g. a Kafka credential storing
`authentication: true` where the schema expects `none` / `basic` / `apiKey`), so a
select never renders blank.

---

## Testing notes

`internal/api/credentials_test.go` covers the credential API hermetically — no
external service is contacted:

| Test | What it proves |
| --- | --- |
| `TestCredentialTestPayloadEndpoint` | `POST /api/v1/credentials/test` validates an unsaved payload |
| `TestCredentialTestEndpoint` | `POST /api/v1/credentials/{id}/test` validates the stored credential |
| `TestCredentialTestRejectsWrongPassword` | a wrong password is reported as a failure, not "valid" — a fake RESP server stands in for Redis |
| `TestRabbitMQCredentialTestSendsCredentialsAndReportsAuthFailure` | a fake AMQP 0-9-1 broker captures the SASL response and asserts it is exactly `\x00user\x00secret`; the broker answers with `ACCESS_REFUSED` and the test asserts the message reports an authentication failure **and** does not contain the password |
| `TestNormalizePEMParsesFlattenedCertificate` | a dynamically generated ECDSA certificate is flattened onto one line and re-parsed after `normalizePEM` |
| `TestCredentialValidateAcceptsAliasedFields` | a credential stored as `hostname` / `username` is not reported as missing its required fields |
| `TestCredentialSchemaAliasesAndMissingTypes` | `mySql`, `openAiApi`, `rabbitmq` and `redis` all have a schema |
| `TestListCredentialTypes` | the type chooser list is served by the backend |
| `TestCredentialPatchMergesDataFields`, `TestCredentialPatchPartial` | a PATCH only overwrites the fields it carries |
| `TestCredentialWriteThroughToEngine` | create / update / delete reach the in-memory credential store the engine reads |

### Running the tests

```bash
# The whole credentials API package
go test ./internal/api/...

# Only the credential tests
go test ./internal/api/ -run 'TestCredential|TestCreateAndGetCredential|TestListCredential|TestNormalizePEM|TestRabbitMQ' -v

# The packages touched by the same cycle
go test ./internal/api/... ./internal/engine/... ./internal/nodes/transform/... \
          ./internal/nodes/database/... ./internal/nodes/http/... \
          ./internal/nodes/trigger/... ./internal/webhooks/... ./internal/connections/...
```

Most of the API tests run against `storage.NewMemoryStorage()` and need no
environment setup. `TestCredentialWriteThroughToEngine` additionally sets
`M9M_DEV_MODE=true` (via `t.Setenv`) because it wires a real credential manager,
which otherwise refuses to start without a production encryption key.
