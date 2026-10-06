# bug-fix.MD — Daftar Bug & Rencana Perbaikan m9m

**Versi:** 1.0 — 2026-10-06
**Source:** `internal/webhooks/*` diff 2026-10-06, `internal/nodes/database/redis.go`, `cmd/m9m/commands/exec.go` (42 executors), `README.md` parity 19/19

---

## Ringkasan
| ID | Judul | Severity | Status |
|---|---|---|---|
| B-01 | Credential per-node stale snapshot — edit Credentials tidak honoured webhook | **P0** | **FIXED** — `CredentialID` + `liveAuthData()` wired di `handler.go:authenticateRequest` |
| B-02 | Auth fallback bypass — `well-formed header` lolos tanpa credential | **P1 Security** | **FIXED** — `handler.go:authenticateRequest` hard-fail 401 jika `CredentialID` set tapi live nil |
| B-03 | Redis bukan native driver — butuh HTTP gateway | P1 Gap | **FIXED** — dual-mode RESP TCP + HTTP fallback, 12 ops, native backend default |
| B-04 | Kafka node missing total | P1 Gap | **FIXED** — `n8n-nodes-base.kafka` producer + `kafkaTrigger` consumer via REST Proxy |
| B-05 | `apiKey` credential map hack `httpHeaderAuth` | P2 | Open |
| B-06 | `readRespondToWebhookHeaders` lower-case lossy | P3 | Open — minor |
| B-07 | Docs `N8N_FEATURE_COMPARISON.md` stale | P2 | Open |
| **B-08** | **Engine `NodeData` capture wired** | **P0** | **FIXED** — `BuildExecutionNodeData` + `result.NodeOutputs` populate per-node via `runExecutionData.ResultData.NodeData` (Debug=ON all, OFF start+last) |
| **B-09** | **NodeDescription.Properties top-10 populate** | **P0** | **FIXED** — `Properties []NodeProperty` populated di HTTP/Set/IF/Switch/Code/Webhook/Cron/Filter/Merge/Wait/Postgres/MySQL/SQLite/Redis |
| **B-10** | **JS Plugin loader `n8n-nodes-*`** | **P1** | **FIXED** — `internal/plugins/{loader,registry,wrapper}.go`, `--plugin-dir` flag, Goja sandbox + Description/Properties/Inputs/Outputs mirror |

---

## B-01 — Credential Save di Node Tidak Persist / Stale Snapshot

**File:** `internal/webhooks/manager.go:resolveCredentialAuthData`, `createWebhookFromNode`, `internal/webhooks/handler.go:authenticateRequest`, `internal/webhooks/webhook.go:Webhook.AuthData/CredentialID`

**Root cause:** `createWebhookFromNode` snapshot `AuthData` saat `RegisterWorkflowWebhooks`. `authenticateRequest` pakai `webhook.AuthData` snapshot. Edit via `PUT /api/v1/credentials/{id}` tidak invalidate `activeHooks[path:method:test]`. Workflow harus re-save baru fresh.

**Repro:**
1. Create credential `httpBasicAuth` user:pass1, create workflow webhook `basicAuth` → `POST /webhook/webhook_callrest` dengan pass1 → 200.
2. `PUT /api/v1/credentials/{id}` pass2 tanpa `PUT /api/v1/workflows/{id}`.
3. Call dengan pass1 → masih 200 (bug), pass2 → 401 (bug).

**Fix (WIP diff sudah ada):**
- `Webhook.CredentialID string` simpan id.
- `authDataFromCredential()` + `resolveCredentialAuthData()` pure.
- `liveAuthData(webhook)` re-read `workflowStorage.GetCredential(CredentialID)` tiap request, fallback snapshot hanya jika `CredentialID==""` atau lookup fail legacy.
- `authenticateRequest`: `authData = manager.liveAuthData(webhook)` before switch.
- Invalidasi: lazy `liveAuthData` cukup; opsi eager refresh di handler `PUT /credentials`.

**Acceptance:**
- Rotation test PASS: old pass 401, new pass 200 tanpa re-save workflow.
- `internal/webhooks/manager_authdata_test.go` extend rotation case.
- No bypass: `CredentialID` set tapi cred missing → 401 (bukan pass).

**Ref:** diff `manager.go:authDataFromCredential`, `liveAuthData`, `handler.go:authenticateRequest` 2026-10-06.

---

## B-02 — Auth Fallback "well-formed header" Bypass

**File:** `internal/webhooks/handler.go:authenticateRequest` case `basic`

```go
if expectedUsername == "" && expectedPassword == "" {
  if username == "" && password == "" { return invalid }
  break // → PASS dengan header apapun
}
```

Credential lookup gagal (`resolveCredentialAuthData` return nil) → handler anggap auth sukses asal header ada. Security hole, false PASS parity.

**Fix:** Jika `CredentialID != ""` tapi `liveAuthData==nil` → return `invalid credentials` 401. Hanya allow `break` ketika `CredentialID==""` (workflow lama tanpa credential binding — legacy compat).

**Test:** webhook `basicAuth` tanpa credential di store → request `Authorization: Basic ...` random → expect 401, bukan 200.

---

## B-03 — Redis Via HTTP Gateway

**File:** `internal/nodes/database/redis.go:RedisHTTPClient`, `sendCommand` POST `baseUrl`

Bukan `go-redis`. Deploy tanpa Upstash/proxy → fail. User expect `n8n-nodes-base.redis` `host:6379`.

**Fix:** Refactor dual-mode:
- Params baru `host`/`port`/`database`/`tls` + credential `redis` (password). Keep `baseUrl` http → fallback HTTP path untuk compat.
- Jika `baseUrl` tidak http: init `redis.NewClient` `go-redis/v9`, map 11 ops existing + `publish`/`expire` ke native cmds.
- `ValidateParameters` tetap.

**Test:** `redis_test.go` dengan `miniredis` — 11 ops PASS tanpa HTTP. Integration `docker compose` redis:6379 SET+GET.

---

## B-04 — Kafka Node Missing

**File:** `cmd/m9m/commands/exec.go:RegisterAllNodes` — tidak ada `kafka`/`kafkaTrigger`

**Fix:**
- New `internal/nodes/messaging/kafka.go` executor `n8n-nodes-base.kafka` (send/sendAndWait) + `internal/nodes/trigger/kafka_trigger.go`.
- Lib `github.com/segmentio/kafka-go` (pure Go). Params topic/key/value/headers/acks/partition/clientId/groupId/sasl/ssl.
- Credential `kafka`. Trigger long-poll emit `{topic,partition,offset,key,value,headers,timestamp}`.
- Register + `docker-compose.yml` kafka/kraft dev.

---

## B-05 — apiKey Map ke httpHeaderAuth

**File:** `manager.go:resolveCredentialAuthData` case `apiKey` → `credType="httpHeaderAuth"`

Hack karena n8n tidak punya dedicated type. Drift risk.

**Fix:** Tambah credential type `apiKey` proper di `storage.Credential` + UI, keep fallback untuk workflow lama import.

---

## B-06 — Header Lower-Case Lossy

**File:** `manager.go:readRespondToWebhookHeaders` → `strings.ToLower(name)`

HTTP case-insensitive, no bug fungsional. Low prio — keep atau preserve canonical via `http.CanonicalHeaderKey`.

---

## B-07 — Docs Stale

**File:** `docs/N8N_FEATURE_COMPARISON.md` (2024-09-22, claim 85%, redis COMPLETE)

**Fix:** Sync dengan reality: redis HTTP-only, kafka missing, 42 executors, parity 19/19. Update table + priority matrix.

---

## Prioritas & ETA

| Phase | Bug | ETA | Status |
|---|---|---|---|
| P0 Week 1 | B-01 + B-02 merge + rotation test | 2d | **DONE 2026-10-06** |
| P0 Week 1 | B-03 Redis native | 3d | **DONE 2026-10-06** |
| P0 Week 1-2 | B-08 NodeData capture + B-09 Properties top-10 | 3d | **DONE 2026-10-06** |
| P1 Week 2 | B-04 Kafka | 5d | **DONE 2026-10-06** |
| P1 Week 2 | B-10 JS plugin loader | 4d | **DONE 2026-10-06** |
| P1 Week 3 | ExecutionDetail NDV tabs (frontend) | 5d | Open (out of backend scope) |
| P2 Week 4 | B-05, B-07 docs sync | 1d | Open |

## Cara Verifikasi Global

```bash
go test ./internal/webhooks -run TestLiveAuthData -v
go test ./internal/nodes/database -run TestRedis -v
curl -u user:pass1 http://localhost:8080/webhook/<path> # after cred rotate
m9m node list | grep -E "kafka|redis"
```

Sinkron dengan `m9m.MD` §3–§4.
