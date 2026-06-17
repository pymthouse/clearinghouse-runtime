# clearinghouse deploy/

Docker Compose stack + Railway deploy scripts for the clearinghouse runtime:
**identity webhook → Kafka/Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

## Design decisions

**No Apache DMZ.** The remote signer container runs `go-livepeer` directly —
there is no Apache reverse proxy or `mod_authnz_jwt` layer in front of it.
Identity validation is handled by the `-remoteSignerWebhookUrl` hook (`POST
/authorize` on the `identity-webhook` service) and the shared `WEBHOOK_SECRET`.

The `identity-webhook` container runs `@pymthouse/builder-sdk`'s OIDC handler:
it validates Auth0 JWTs via JWKS and returns `auth_id = "{azp}:{sub}"` to the
signer. No database or local identity storage required.

**CLI port not exposed.** go-livepeer's `-cliAddr` (admin/RPC) is bound to
`127.0.0.1:4935` inside the container and is never published or mapped to the
host — not in `docker-compose.yml`, not in Railway. Only the signing HTTP port
(`8081`) is exposed.

## Local stack

```bash
cp .env.example .env
$EDITOR .env    # Auth0 + Konnect bootstrap secrets

make build
./clearinghouse-bootstrap    # writes .env.livepeer (JWT_*, WEBHOOK_SECRET, REMOTE_SIGNER_WEBHOOK_URL, Konnect vars)

# Optional: set SIGNER_ETH_ADDR in .env.livepeer before starting the stack

make stack-up ENV_FILE=.env.livepeer
make stack-logs ENV_FILE=.env.livepeer
make stack-down ENV_FILE=.env.livepeer
```

Kafka + signer only (no metering):

```bash
docker compose -f deploy/docker-compose.yml --env-file .env.livepeer up -d --build kafka identity-webhook remote-signer
```

Verify CLI port is not published:

```bash
docker compose -f deploy/docker-compose.yml --env-file .env.livepeer port remote-signer 4935
# expected: no output / error (port is not mapped)
docker compose -f deploy/docker-compose.yml --env-file .env.livepeer port remote-signer 8081
# expected: 0.0.0.0:8081
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `REMOTE_SIGNER_WEBHOOK_URL` | no | `http://identity-webhook:8090/authorize` | Identity webhook (`POST /authorize`) |
| `WEBHOOK_SECRET` | yes | — | Shared secret for signer → webhook auth (from bootstrap) |
| `JWT_ISSUER` / `JWT_AUDIENCE` | yes | — | Auth0 issuer + API audience (from bootstrap) |
| `PLATFORM_URL` | no | — | Bootstrap input: sets production webhook to `{PLATFORM_URL}/webhooks/remote-signer` |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_URL` | yes | — | OpenMeter / Konnect base URL (from bootstrap) |
| `OPENMETER_INGEST_URL` | yes | — | Ingest endpoint (`${OPENMETER_URL}/events` — from bootstrap) |
| `OPENMETER_API_KEY` | yes | — | Konnect PAT (`kpat_…`) (from bootstrap) |
| `ETH_USD_PRICE` | no | `3500` | ETH/USD rate for Wei→USD micros conversion |
| `AUTH0_PUBLIC_CLIENT_ID` | no | — | Auth0 public client id (from bootstrap) |

## OpenMeter/Konnect bootstrap

Provision meters, features, and the default pay-per-use plan before starting the collector:

```bash
make build
./clearinghouse-bootstrap
```

Uses [`Kong/sdk-konnect-go`](https://github.com/Kong/sdk-konnect-go) via the Go
`clearinghouse-bootstrap` CLI. Catalog definitions live in `config/meters.json`
and `config/pricing.json`.

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

Markup rules live in [`config/pricing.json`](../config/pricing.json). Collector
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

## Railway deploy

### One-time setup

1. Create a Railway project and three services named exactly: `kafka`,
   `openmeter-collector`, `remote-signer`.
2. Note the project ID from **Railway → project → Settings → General**.
3. Set shell env for manual deploys:
   ```
   RAILWAY_API_TOKEN       # Railway Account → Tokens
   RAILWAY_PROJECT_ID      # from step 2
   OPENMETER_URL
   OPENMETER_API_KEY
   OPENMETER_INGEST_URL    # ${OPENMETER_URL}/events for Konnect
   WEBHOOK_SECRET
   REMOTE_SIGNER_WEBHOOK_URL
   ```

### Apply env and deploy (manual)

```bash
export RAILWAY_API_TOKEN=...
export RAILWAY_PROJECT_ID=...
export OPENMETER_URL=... OPENMETER_API_KEY=... OPENMETER_INGEST_URL=...
export WEBHOOK_SECRET=... REMOTE_SIGNER_WEBHOOK_URL=...

bash scripts/railway-apply-stack-env.sh   # set per-service vars
bash scripts/railway-deploy-stack.sh production
```
