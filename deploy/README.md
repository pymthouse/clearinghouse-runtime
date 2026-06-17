# clearinghouse deploy/

Docker Compose stack + Railway deploy scripts for the clearinghouse runtime:
**Kafka/Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

## Design decisions

**No Apache DMZ.** The remote signer container runs `go-livepeer` directly —
there is no Apache reverse proxy or `mod_authnz_jwt` layer in front of it.
Identity validation is handled by the `-remoteSignerWebhookUrl` hook (your
`/authorize` endpoint) and the shared `WEBHOOK_SECRET`.

**CLI port not exposed.** go-livepeer's `-cliAddr` (admin/RPC) is bound to
`127.0.0.1:4935` inside the container and is never published or mapped to the
host — not in `docker-compose.yml`, not in Railway. Only the signing HTTP port
(`8081`) is exposed.

## Local stack

```bash
cp deploy/.env.example deploy/.env
$EDITOR deploy/.env  # fill WEBHOOK_SECRET, OPENMETER_API_KEY at minimum

pnpm stack:up    # kafka + remote-signer + openmeter-collector (docker compose up -d --build)
pnpm stack:logs  # tail all logs
pnpm stack:down  # tear down
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
| `WEBHOOK_SECRET` | yes | — | Shared secret passed as `Authorization: Bearer` to the webhook |
| `SIGNER_NETWORK` | no | `arbitrum-one-mainnet` | go-livepeer `-network` |
| `ETH_RPC_URL` | no | public arb1 endpoint | Arbitrum RPC |
| `SIGNER_ETH_ADDR` | no | — | Funded signer Ethereum address |
| `SIGNER_HOST_PORT` | no | `8081` | Host port for the signing HTTP endpoint |
| `KAFKA_GATEWAY_TOPIC` | no | `livepeer-gateway-events` | Kafka topic |
| `OPENMETER_URL` | yes | — | OpenMeter / Konnect base URL |
| `OPENMETER_INGEST_URL` | yes | — | Ingest endpoint (`${OPENMETER_URL}/events` for Konnect) |
| `OPENMETER_API_KEY` | yes | — | Konnect PAT (`kpat_…`) or OpenMeter API key |
| `ETH_USD_PRICE` | no | `3500` | ETH/USD rate for Wei→USD micros conversion |
| `OPENMETER_DEFAULT_PLAN_KEY` | no | `clearinghouse-default-ppu` | Default pay-per-use plan key (Konnect) |
| `OPENMETER_DEFAULT_STARTER_INCLUDED_USD_MICROS` | no | `5000000` | Per-customer trial credit ($5) on billable usage |
| `PRICING_CONFIG_PATH` | no | `config/pricing.json` | Markup rules + plan defaults for bootstrap/collector |
| `AUTH0_PUBLIC_CLIENT_ID` | no | — | Auth0 SPA client id for `provision:customer` default `--client-id` |

## OpenMeter/Konnect bootstrap

Provision meters, features, and the default pay-per-use plan before starting the collector:

```bash
OPENMETER_URL=https://us.api.konghq.com/v3/openmeter \
OPENMETER_API_KEY=kpat_... \
pnpm openmeter:bootstrap
```

Creates (additive — existing pymthouse objects are untouched):

| Object | Key | Purpose |
|--------|-----|---------|
| Meter (existing) | `network_fee_usd_micros` | Raw network cost from signer |
| Meter (new) | `billable_usd_micros` | Post-markup billable amount (collector phase 2) |
| Meter (existing) | `signed_ticket_count` | Request counts |
| Feature (existing) | `network_spend` | pymthouse-compatible network feature |
| Feature (new) | `billable_spend` | Billable usage feature |
| Plan (new) | `clearinghouse-default-ppu` | Pay-per-use: $0.000001/billable micro + **$5 included usage** |

Idempotent — safe to re-run. Konnect and self-hosted OpenMeter both supported
(detected by API key prefix `kpat_`/`spat_` or URL containing `konghq.com`).
Self-hosted bootstrap creates billable meters/features only; **plan creation is Konnect-only**.

### Two-meter billing model

```text
Signer computed_fee (wei)
  → collector: network_fee_usd_micros   (raw network cost — observability)
  → collector: billable_usd_micros      (network × pipeline/model markup — billing)
       → billable_spend feature
            → clearinghouse-default-ppu subscription per customer
```

Markup rules live in [`config/pricing.json`](../config/pricing.json). Collector
pipeline config: [`deploy/openmeter-collector/collector.yaml`](openmeter-collector/collector.yaml).
The collector does not yet emit `billable_usd_micros` (phase 2); until then the billable meter
stays empty while the catalog is ready.

### Auth0 identity contract

Matches [auth0-livepeer](https://github.com/livepeer/auth0-livepeer) bootstrap:

- Webhook returns `auth_id = "{azp}:{sub}"` (`CLAIM_CLIENT_ID=azp`, `USAGE_SUBJECT_TYPE=auth0_user_id`)
- Collector splits on first colon → `client_id` / `external_user_id`
- Konnect customer key: `{AUTH0_PUBLIC_CLIENT_ID}:{auth0|sub}`

Example customer key: `abc123xyz:auth0|user456`

### Per-customer provision

After bootstrap, attach the default plan to each Auth0 user:

```bash
AUTH0_PUBLIC_CLIENT_ID=... \
OPENMETER_URL=https://us.api.konghq.com/v3/openmeter \
OPENMETER_API_KEY=kpat_... \
pnpm provision:customer -- --external-user-id 'auth0|user456'
```

Or pass `--client-id` explicitly. Creates Konnect customer + subscription idempotently.
The subscription rate card shows included usage (e.g. **5,000,000 free units**) on billable spend.

### Coexistence with pymthouse

| Object | pymthouse | clearinghouse |
|--------|-----------|---------------|
| `network_fee_usd_micros` | shared | unchanged |
| `network_spend` + Starter plan | yes | unchanged |
| `billable_usd_micros` / `billable_spend` / `clearinghouse-default-ppu` | — | new |

No existing meters or features are deleted or recreated.

## Railway deploy

### One-time setup

1. Create a Railway project and three services named exactly: `kafka`,
   `openmeter-collector`, `remote-signer`.
2. Note the project ID from **Railway → project → Settings → General**.
3. Set GitHub secrets (or shell env for manual deploys):
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

### CI (GitHub Actions)

`.github/workflows/deploy-railway.yml` triggers on push to `main` (paths:
`deploy/**`, `scripts/railway-*.sh`, `scripts/lib/railway-*.sh`). **Off by
default** — enable by setting the repository variable `RAILWAY_AUTO_DEPLOY=true`
once secrets are configured.

```bash
gh variable set RAILWAY_AUTO_DEPLOY --body true -R livepeer/clearinghouse
```

The workflow supports `workflow_dispatch` with an `environment` input (default
`production`) for manual one-off deploys to any Railway environment.
