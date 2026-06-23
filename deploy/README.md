# clearinghouse deploy/

Docker Compose stack for the clearinghouse runtime:
**identity-webhook → Kafka/Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

The in-compose **identity-webhook** uses builder-sdk's **API-key provider** (and optional **OIDC** when `JWT_ISSUER` is set)
and is wired to **remote-signer** via `REMOTE_SIGNER_WEBHOOK_URL`. With `USAGE_QUERY_ENABLED=1`, it also serves self-scoped usage/balance reads from OpenMeter.

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

Self-scoped usage reads (requires `USAGE_QUERY_ENABLED=1` and `OPENMETER_URL` + `OPENMETER_API_KEY`):

```bash
docker compose -f deploy/docker-compose.yml --env-file deploy/.env exec identity-webhook \
  curl -sS http://localhost:8090/api/v1/apps/demo-client/usage/me/balance \
    -H "Authorization: Bearer sk_demo_local_key"

docker compose -f deploy/docker-compose.yml --env-file deploy/.env exec identity-webhook \
  curl -sS "http://localhost:8090/api/v1/apps/demo-client/usage/me?startDate=2026-01-01T00:00:00.000Z&endDate=2026-01-31T23:59:59.999Z" \
    -H "Authorization: Bearer sk_demo_local_key"
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
| `WEBHOOK_SECRET` | yes | — | Shared secret passed as `Authorization:Bearer <secret>` to the webhook (from bootstrap) |
| `REMOTE_SIGNER_WEBHOOK_URL` | no | `http://identity-webhook:8090/authorize` | Signer identity webhook URL |
| `IDENTITY_ISSUER` | no | `http://identity-webhook:8090` | Issuer stamped on API-key identities |
| `DEMO_API_KEY` | no | `sk_demo_local_key` | Demo API key accepted by identity-webhook |
| `DEMO_CLIENT_ID` | no | `demo-client` | `client_id` for the demo key |
| `DEMO_USER_ID` | no | `demo-user` | `usage_subject` for the demo key |
| `USAGE_QUERY_ENABLED` | no | `0` | When `1` and OpenMeter creds are set, identity-webhook serves `/api/v1/apps/{clientId}/usage/me` |
| `OPENMETER_TRIAL_FEATURE_KEY` | no | `network_spend` | Entitlement feature for balance reads |
| `JWT_ISSUER` | no | — | When set, OIDC JWTs are accepted alongside API keys (`createFirstMatchEndUserVerifier`) |
| `JWT_AUDIENCE` | no | issuer sans trailing slash | JWT `aud` for OIDC verification |
| `ALLOW_INSECURE_HTTP` | no | `0` | Set `1` for local OIDC issuers on `http://` |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_URL` | yes | — | OpenMeter / Konnect base URL (collector; identity-webhook usage reads when enabled) |
| `OPENMETER_INGEST_URL` | yes | — | Ingest endpoint (`${OPENMETER_URL}/events` for Konnect) |
| `OPENMETER_API_KEY` | yes | — | Konnect PAT (`kpat_…`) (collector; identity-webhook usage reads when enabled) |
| `OPENMETER_DEFAULT_PLAN_KEY` | no | `clearinghouse_default_ppu` | Plan subscribed on customer upsert |
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

### Customer upsert (collector self-heal)

The collector runs a local Go provision sidecar (`deploy/openmeter-collector/provision`, Kong `sdk-konnect-go`) that holds the
OpenMeter admin credentials — the identity webhook does **not** need them.

For each `create_signed_ticket` event:

1. Benthos maps the CloudEvent (including `billable_usd_micros`, initially equal to `network_fee_usd_micros`).
2. `POST http://127.0.0.1:8091/ensure` idempotently creates customer + subscription (`OPENMETER_DEFAULT_PLAN_KEY`).
3. Event is ingested to Konnect.
4. On ingest failure (e.g. `no customer found for event subject`), the collector ensures again and retries once.

### Future admin/query boundary (OAuth later)

When an admin/query API is added, introduce a small internal **billing-gateway** service:

- Move ensure-customer and usage-query logic behind that gateway.
- Protect caller-to-gateway with OAuth (client credentials / service-to-service).
- Keep gateway-to-OpenMeter on backend machine credentials (`kpat_…`).

The collector provision sidecar is the thin local equivalent until that gateway exists.

### API-key identity contract

- End-user presents `Authorization: Bearer sk_…` to the remote signer
- Webhook resolves the key → `auth_id = "{client_id}:{usage_subject}"`
- Collector splits on first colon → `client_id` / `external_user_id`
- Demo key defaults: `sk_demo_local_key` → `demo-client:demo-user`

Example customer key: `demo-client:demo-user`

Customer upsert is handled by the collector provision sidecar (see above). The Go bootstrap CLI
still provisions meters/features/plans; per-event customer+subscription ensure runs in the collector.
