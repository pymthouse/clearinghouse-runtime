# clearinghouse deploy/

Docker Compose stack for the clearinghouse runtime:
**Kafka/Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

## Design decisions

**No Apache DMZ.** The remote signer container runs `go-livepeer` directly —
there is no Apache reverse proxy or `mod_authnz_jwt` layer in front of it.
Identity validation is handled by the `-remoteSignerWebhookUrl` hook (your
`/authorize` endpoint) and the shared `WEBHOOK_SECRET`.

**CLI port not exposed.** go-livepeer's `-cliAddr` (admin/RPC) is bound to
`127.0.0.1:4935` inside the container and is never published or mapped to the
host — not in `docker-compose.yml`. Only the signing HTTP port
(`8081`) is exposed.

## Local stack

```bash
cp deploy/.env.example deploy/.env
$EDITOR deploy/.env

# If using clearinghouse-bootstrap (feat/go-bootstrap-cli), merge WEBHOOK_SECRET
# and Konnect vars from .env.livepeer into deploy/.env.

docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs -f
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

Kafka + signer only (no metering):

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build kafka remote-signer
```

Verify CLI port is not published:

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env port remote-signer 4935
# expected: no output / error (port is not mapped)
docker compose -f deploy/docker-compose.yml --env-file deploy/.env port remote-signer 8081
# expected: 0.0.0.0:8081
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `REMOTE_SIGNER_WEBHOOK_URL` | yes | — | Identity webhook URL (`/authorize` endpoint) |
| `WEBHOOK_SECRET` | yes | — | Shared secret passed as `Authorization: Bearer` to the webhook (from bootstrap) |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_URL` | yes | — | OpenMeter / Konnect base URL (from bootstrap) |
| `OPENMETER_INGEST_URL` | yes | — | Ingest endpoint (`${OPENMETER_URL}/events` for Konnect) |
| `OPENMETER_API_KEY` | yes | — | Konnect PAT (`kpat_…`) (from bootstrap) |
| `ETH_USD_PRICE` | no | `3500` | ETH/USD rate for Wei→USD micros conversion |
| `AUTH0_PUBLIC_CLIENT_ID` | no | — | Auth0 public client id (from bootstrap) |

## OpenMeter/Konnect bootstrap

Provision meters, features, and the default pay-per-use plan before starting the collector.
Use the Go `clearinghouse-bootstrap` CLI (`feat/go-bootstrap-cli`) or your existing Konnect setup.

Creates (additive — existing pymthouse objects are untouched):

| Object | Key | Purpose |
|--------|-----|---------|
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

### Auth0 identity contract

- Webhook returns `auth_id = "{azp}:{sub}"` (`CLAIM_CLIENT_ID=azp`, `USAGE_SUBJECT_TYPE=auth0_user_id`)
- Collector splits on first colon → `client_id` / `external_user_id`
- Konnect customer key: `{AUTH0_PUBLIC_CLIENT_ID}:{auth0|sub}`

Example customer key: `abc123xyz:auth0|user456`

Per-customer provisioning (Konnect customer + subscription) is a follow-up — not
yet implemented in the Go CLI.
