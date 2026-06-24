# clearinghouse deploy/

Docker Compose stack for the clearinghouse runtime:
**Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

## Components

| Service | Role | Docs |
| --- | --- | --- |
| **Redpanda** (`kafka`) | Kafka-compatible event bus. The signer publishes gateway events; the collector consumes them. | [Redpanda docs](https://docs.redpanda.com/) |
| **go-livepeer remote signer** (`remote-signer`) | Signs Livepeer payment tickets and emits `create_signed_ticket` events to Kafka. | [go-livepeer](https://github.com/livepeer/go-livepeer) |
| **OpenMeter collector** (`openmeter-collector`) | Benthos pipeline: filters Kafka events, converts fees to USD micros, POSTs CloudEvents to OpenMeter ingest. | [OpenMeter collector](https://openmeter.io/docs/collectors) |
| **Konnect / OpenMeter** (external) | Hosted metering and billing API. Set `OPENMETER_INGEST_URL` to your ingest endpoint. | [Konnect OpenMeter](https://docs.konghq.com/konnect/openmeter/), [self-hosted OpenMeter](https://openmeter.io/docs/deploy/kubernetes) |

Data flow:

```text
Signer HTTP request
  → identity webhook (/authorize)
  → signed ticket + Kafka create_signed_ticket event
  → collector transforms event
  → OpenMeter ingest API
```

## Design decisions

**Redpanda over Apache Kafka.** The stack uses Redpanda as the Kafka-compatible broker. Redpanda Kafka runs as a single-binary dev container with no ZooKeeper dependency and faster local startup.

**Identity & auth.** The signer container runs `go-livepeer` directly. Every signing request is authorized by go-livepeer's `-remoteSignerWebhookUrl` hook, which calls your `/authorize` endpoint with `Authorization: Bearer <WEBHOOK_SECRET>` — no reverse proxy or gateway in front of the signer.

**CLI port not exposed.** go-livepeer's `-cliAddr` (admin/RPC) is bound to `127.0.0.1:4935` inside the container and is never published or mapped to the host. Only the signing HTTP port (`8081`) is exposed.

## Local stack

### 1. Quick check — Kafka + signer

Start with the minimum services to confirm the signer and broker are alive. You still need a real identity webhook URL (or the mock profile below).

```bash
cp deploy/.env.example deploy/.env
$EDITOR deploy/.env

docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build kafka remote-signer
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs -f remote-signer
```

Verify CLI port is not published:

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env port remote-signer 4935
# expected: no output / error (port is not mapped)
docker compose -f deploy/docker-compose.yml --env-file deploy/.env port remote-signer 8081
# expected: 0.0.0.0:8081
```

### 2. Full stack — add metering

Provision OpenMeter meters/features (see [OpenMeter/Konnect bootstrap](#openmeterkonnect-bootstrap)), then set `OPENMETER_INGEST_URL`, `OPENMETER_API_KEY`, and `ETH_USD_PRICE` in `deploy/.env`.

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs -f
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

### 3. Local smoke test (mock profile)

The `mock` Compose profile starts a tiny HTTP server that accepts signer authorize callbacks and collector ingest POSTs. **Opt in only** — it is not started by default.

Set these in `deploy/.env` when using the mock:

```bash
REMOTE_SIGNER_WEBHOOK_URL=http://mock-services:8080/authorize
OPENMETER_INGEST_URL=http://mock-services:8080/events
OPENMETER_API_KEY=mock-local-key
WEBHOOK_SECRET=dev-secret
```

Start Kafka, mock services, and the collector:

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env --profile mock up -d --build kafka mock-services openmeter-collector
```

Produce a sample `create_signed_ticket` event (matches [`openmeter-collector/collector.yaml`](openmeter-collector/collector.yaml) expectations):

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env exec kafka \
  rpk topic produce livepeer-gateway-events --format '%v' <<'EOF'
{"type":"create_signed_ticket","data":{"auth_id":"demo-client:demo-user","computed_fee":"1000000000000000","request_id":"smoke-req-1","pipeline":"lv2v","model_id":"daydream-video","pixels":"1920","current_time":"2026-06-24T12:00:00Z"}}
EOF
```

Confirm the mock received ingest traffic:

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env --profile mock logs mock-services
# expected: mock-services: POST /events ... ingest payload=...
```

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `REMOTE_SIGNER_WEBHOOK_URL` | yes | — | Identity webhook URL (`/authorize` endpoint) |
| `WEBHOOK_SECRET` | yes | — | Shared secret passed as `Authorization: Bearer <secret>` to the webhook |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_ETH_KEYSTORE_PATH` | no | — | Optional keystore directory or keyfile path passed to `-ethKeystorePath` |
| `SIGNER_DATA_DIR` | no | `./data` | Host directory bind-mounted to `/data` for signer state, keystore, and password file |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `SIGNER_REMOTE_DISCOVERY` | no | `0` | Enables remote signer orchestrator discovery when set to `1` or `true` |
| `ORCH_WEBHOOK_URL` | no | — | Optional orchestrator discovery webhook passed to `-orchWebhookUrl` when remote discovery is enabled |
| `LIVE_AI_CAP_REPORT_INTERVAL` | no | — | Optional capacity report interval passed to `-liveAICapReportInterval` when remote discovery is enabled |
| `KAFKA_ADVERTISED_ADDR` | no | `kafka:9092` | Redpanda advertised Kafka address (broker container) |
| `KAFKA_BROKERS` | no | `kafka:9092` | Kafka bootstrap servers |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_INGEST_URL` | collector only | — | Ingest endpoint. Konnect: `https://<region>.api.konghq.com/v3/openmeter/events`. Self-hosted: `https://<host>/api/v1/events` |
| `OPENMETER_API_KEY` | collector only | — | Konnect PAT (`kpat_…`) or self-hosted API key |
| `ETH_USD_PRICE` | collector only | — | ETH/USD rate for Wei→USD micros conversion |

## OpenMeter/Konnect bootstrap

Provision meters, features, and the default pay-per-use plan before starting the collector.
Use the Go `clearinghouse-bootstrap` CLI or your existing Konnect setup.

Creates:

| Object | Key | Purpose |
| --- | --- | --- |
| Meter | `network_fee_usd_micros` | Raw network cost from signer |
| Meter | `billable_usd_micros` | Post-markup billable amount (collector phase 2) |
| Meter | `signed_ticket_count` | Request counts |
| Feature | `network_spend` | Trial/network spend feature |
| Feature | `billable_spend` | Billable usage feature |
| Plan | `clearinghouse_default_ppu` | Pay-per-use rate card |

Idempotent — safe to re-run.

### Two-meter billing model

```text
Signer computed_fee (wei)
  → collector: network_fee_usd_micros   (raw network cost — observability)
  → collector: billable_usd_micros      (network × pipeline/model markup — billing)
       → billable_spend feature
            → clearinghouse_default_ppu subscription per customer
```

Markup rules are defined in the bootstrap CLI catalog. Collector
pipeline config: [`deploy/openmeter-collector/collector.yaml`](openmeter-collector/collector.yaml).
The collector does not yet emit `billable_usd_micros` (phase 2); until then the billable meter
stays empty while the catalog is ready.

### Identity contract (collector)

The collector expects Kafka `auth_id` as `client_id:external_user_id` (first-colon split).
Konnect customer key matches that compound id (e.g. `demo-client:demo-user`).
