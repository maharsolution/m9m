# Aiven Kafka samples

Two m9m workflows that talk to an Aiven Kafka service over the
**native** transport (segmentio/kafka-go over TLS) — no REST proxy
sidecar required.

| File | What it does |
| --- | --- |
| `aiven-kafka-produce.json` | Webhook → Build Payload → **Kafka Produce** → Respond. Publishes one record per webhook call. |
| `aiven-kafka-consume.json` | **Kafka Trigger** → Format Record → Log Record. Joins a consumer group and polls the topic. |
| `aiven-kafka-credential.template.json` | Credential envelope template (broker + username + **service CA**, password left as a placeholder). |

## Broker details (from `aivan-sample.txt`)

| Setting | Value |
| --- | --- |
| Bootstrap server | `kafka-2366031f-tds-024f.a.aivencloud.com:20475` |
| Username | `avnadmin` |
| Password | *from the Aiven console — not committed to the repo* |
| SASL mechanism | `PLAIN` |
| TLS | on, verified against the service CA (`kafkacert.crt`) |
| Topic | `finnet-queue` (also present: `topic-data-example`) |
| REST proxy (optional) | `https://kafka-2366031f-tds-024f.a.aivencloud.com:20466` |

## Setup

1. Create the credential in m9m (**Credentials → New → Kafka**) using
   `aiven-kafka-credential.template.json`, filling in the password.
2. Import the workflow JSON.
3. Attach the credential to the `Kafka Produce` / `Kafka Trigger` node
   (the sample already references `Kafka - Aiven`).

The broker address, CA and username are *also* set as node parameters,
so the sample works even before a credential is attached.

## Running the produce sample

```bash
curl -X POST http://localhost:8080/webhook/aiven-produce \
  -H 'Content-Type: application/json' \
  -d '{"topic":"finnet-queue","id":"order-42","amount":1400}'
```

Response:

```json
{"topic":"finnet-queue","partition":0,"offset":105,"key":"order-42","headers":{}}
```

## Running the consume sample

Activate the workflow (the Kafka Trigger polls on a timer), then
produce a record with the workflow above. The trigger emits:

```json
{
  "topic": "finnet-queue",
  "partition": 0,
  "offset": 105,
  "key": "order-42",
  "value": "{\"id\":\"order-42\",\"amount\":1400}",
  "raw": "<base64>",
  "headers": null,
  "timestamp": "2026-11-09T05:50:53.097Z"
}
```

### `startPosition`

| Value | Behaviour |
| --- | --- |
| `first` | A consumer group with no committed offset replays the topic from the beginning. Use this for a fresh group / smoke test. |
| `last` (default, n8n parity) | Only records produced **after** the trigger joined the group. |

## Notes

- The first poll of a brand-new consumer group needs a moment for the
  group join (FindCoordinator + JoinGroup + SyncGroup); m9m allows an
  extra 20 s grace window on that first fetch.
- `value` on the producer accepts an expression. `={{ $json }}`
  forwards the whole upstream item, `={{ $json.x }}` a single field,
  and an empty value also forwards the whole item (n8n parity).
- mTLS is supported: set `clientCert` + `clientKey` (PEM) on the node
  and/or in the credential for services that require
  `ssl.client.auth=required`.
