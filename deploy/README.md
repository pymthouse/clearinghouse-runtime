# clearinghouse deploy/

Docker Compose stack for the clearinghouse runtime:
**identity-webhook → Kafka/Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

The in-compose **identity-webhook** uses builder-sdk's **API-key provider** (not Auth0/OIDC)
and is wired to **remote-signer** via `REMOTE_SIGNER_WEBHOOK_URL`.

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

Verify the identity webhook (simulates go-livepeer calling `/authorize`):

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env exec identity-webhook \
  curl -sS -X POST http://localhost:8090/authorize \
    -H "Authorization: Bearer dev-webhook-secret-change-me" \
    -H "Content-Type: application/json" \
    -d '{"headers":{"Authorization":["Bearer sk_demo_local_key"]}}'
# expected: "status":200, "auth_id":"demo-client:demo-user"
```

Kafka + signer only (no metering):

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build kafka identity-webhook remote-signer
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
| `WEBHOOK_SECRET` | yes | — | Shared secret for signer → webhook (`Authorization: Bearer`) |
| `REMOTE_SIGNER_WEBHOOK_URL` | no | `http://identity-webhook:8090/authorize` | Signer identity webhook URL |
| `IDENTITY_ISSUER` | no | `http://identity-webhook:8090` | Issuer stamped on API-key identities |
| `DEMO_API_KEY` | no | `sk_demo_local_key` | Demo API key accepted by identity-webhook |
| `DEMO_CLIENT_ID` | no | `demo-client` | `client_id` for the demo key |
| `DEMO_USER_ID` | no | `demo-user` | `usage_subject` for the demo key |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_URL` | yes | — | OpenMeter / Konnect base URL (from bootstrap) |
| `OPENMETER_INGEST_URL` | yes | — | Ingest endpoint (`${OPENMETER_URL}/events` for Konnect) |
| `OPENMETER_API_KEY` | yes | — | Konnect PAT (`kpat_…`) (from bootstrap) |
| `ETH_USD_PRICE` | no | `3500` | ETH/USD rate for Wei→USD micros conversion |

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

### API-key identity contract

- End-user presents `Authorization: Bearer sk_…` to the remote signer
- Webhook resolves the key → `auth_id = "{client_id}:{usage_subject}"`
- Collector splits on first colon → `client_id` / `external_user_id`
- Demo key defaults: `sk_demo_local_key` → `demo-client:demo-user`

Example customer key: `demo-client:demo-user`

Per-customer provisioning (Konnect customer + subscription) is a follow-up — not
yet implemented in the Go CLI.
