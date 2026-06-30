# clearinghouse

Docker Compose stack for the clearinghouse runtime:
**identity-webhook → Redpanda → go-livepeer remote signer → OpenMeter/Benthos collector → Konnect metering**.

## Components

| Service | Role | Docs |
| --- | --- | --- |
| **identity-webhook** (`identity-webhook`) | Resolves end-user credentials (API keys and/or Auth0 OIDC JWTs) to `auth_id` for go-livepeer's `/authorize` hook. Self-contained wire protocol + [`jose`](https://github.com/panva/jose) JWKS verification. | [jose](https://github.com/panva/jose) |
| **Redpanda** (`kafka`) | Kafka-compatible event bus. The signer publishes gateway events; the collector consumes them. | [Redpanda docs](https://docs.redpanda.com/) |
| **go-livepeer remote signer** (`remote-signer`) | Signs Livepeer payment tickets and emits `create_signed_ticket` events to Kafka. | [go-livepeer](https://github.com/livepeer/go-livepeer) |
| **OpenMeter collector** (`openmeter-collector`) | Benthos pipeline: filters Kafka events, converts fees to USD micros, POSTs CloudEvents to OpenMeter ingest. Also hosts the **Builder API** (Go, port 8095) for Auth0 user provisioning, RFC 8693 signer token exchange, and OpenMeter customer upsert. | [OpenMeter collector](https://openmeter.io/docs/collectors), [builder-api/README](openmeter-collector/builder-api/README.md) |
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

**Redpanda over Apache Kafka.** The stack uses Redpanda as the Kafka-compatible broker. Redpanda runs as a single-binary dev container with no ZooKeeper dependency and faster local startup.

**Identity & auth.** The in-compose **identity-webhook** implements go-livepeer's remote-signer webhook wire protocol in-repo (`identity-webhook/protocol.mjs`, `identity-webhook/verifiers.mjs`). It accepts **`Authorization: Bearer sk_…`** (API keys) and **`Authorization: Bearer <jwt>`** (Auth0 OIDC tokens from the Builder API RFC 8693 token exchange). Set `OIDC_ISSUER` / `OIDC_AUDIENCE` in `identity-webhook/.env` to verify JWTs; when both OIDC and API keys are configured, JWTs are tried first. The signer calls `/authorize` with `Authorization: Bearer <WEBHOOK_SECRET>`; end-user credentials resolve to `auth_id = "{client_id}:{usage_subject}"`. For local alive checks only, leave `REMOTE_SIGNER_WEBHOOK_URL` empty to omit the webhook hook.

**CLI port not exposed.** go-livepeer's `-cliAddr` (admin/RPC) is bound to `127.0.0.1:4935` inside the container and is never published or mapped to the host. Only the signing HTTP port (`8081`) is exposed.

**Per-service configuration.** Each service has a local `.env` file (copy from `.env.example` before starting). Kafka, remote-signer, and openmeter-collector mount theirs at `/service/.env` and source it in the entrypoint. identity-webhook reads its `.env` via Compose `env_file`.

## Local stack

### 1. Quick check — Kafka + identity webhook + signer

Start here before wiring metering. This runs the broker, identity webhook, and remote signer.

```bash
cp kafka/.env.example kafka/.env
cp identity-webhook/.env.example identity-webhook/.env
cp remote-signer/.env.example remote-signer/.env
$EDITOR identity-webhook/.env remote-signer/.env
# WEBHOOK_SECRET must match in both files (`.env.example` ships a local dev value).
# For a local alive check without an identity webhook:
#   REMOTE_SIGNER_WEBHOOK_URL=
#   WEBHOOK_SECRET=

docker compose up -d --build kafka identity-webhook remote-signer
docker compose logs -f remote-signer
```

Verify the identity webhook (simulates go-livepeer calling `/authorize`; secret matches `.env.example`):

```bash
docker compose exec identity-webhook \
  curl -sS -X POST http://localhost:8090/authorize \
    -H "Authorization: Bearer dev-webhook-secret-change-me" \
    -H "Content-Type: application/json" \
    -d '{"headers":{"Authorization":["Bearer sk_demo_local_key"]}}'
# expected: "status":200, "auth_id":"<publicClientId>:demo-user"
```

Expected result: `remote-signer` starts cleanly, connects to Kafka, and serves the signing HTTP port.

Verify CLI port is not published:

```bash
docker compose port remote-signer 4935
# expected: no output / error (port is not mapped)
docker compose port remote-signer 8081
# expected: 0.0.0.0:8081
```

### 2. Full stack — add metering

After the quick check passes, add the OpenMeter collector and hosted metering configuration. Provision OpenMeter meters/features (see [OpenMeter/Konnect bootstrap](#openmeterkonnect-bootstrap)), then configure the collector:

```bash
cp openmeter-collector/.env.example openmeter-collector/.env
$EDITOR openmeter-collector/.env

docker compose up -d --build
docker compose logs -f
docker compose down
```

## Environment variables

Each service documents its variables in its own `.env.example`:

| Service | Config file | Key variables |
| --- | --- | --- |
| `identity-webhook` | [`identity-webhook/.env.example`](identity-webhook/.env.example) | `WEBHOOK_SECRET`, `IDENTITY_ISSUER`, `DEMO_API_KEY`, `DEMO_CLIENT_ID`, `DEMO_USER_ID`, `OIDC_ISSUER`, `OIDC_AUDIENCE`, `OIDC_CLIENT_CLAIM`, `OIDC_SUBJECT_CLAIM`, `API_KEY_PREFIX` (optional) |
| `kafka` | [`kafka/.env.example`](kafka/.env.example) | `KAFKA_ADVERTISED_ADDR` |
| `remote-signer` | [`remote-signer/.env.example`](remote-signer/.env.example) | `REMOTE_SIGNER_WEBHOOK_URL`, `WEBHOOK_SECRET`, `SIGNER_*`, `KAFKA_BROKERS`, `KAFKA_GATEWAY_TOPIC` |
| `openmeter-collector` | [`openmeter-collector/.env.example`](openmeter-collector/.env.example) | `KAFKA_BROKERS`, `KAFKA_GATEWAY_TOPIC`, `OPENMETER_INGEST_URL`, `OPENMETER_URL`, `OPENMETER_API_KEY`, `PRICE_ORACLE_URL`, `PRICE_ORACLE_REFRESH`, `BUILDER_API_PORT`, `AUTH0_*`, `SIGNER_URL`, `DISCOVERY_URL` |

Signer state (keystore, `.eth-password`, chain DB) is stored under [`remote-signer/data/`](remote-signer/data/), bind-mounted to `/data` in the container.

```bash
mkdir -p remote-signer/data/keystore
cp /path/to/your/keystore/* remote-signer/data/keystore/
cp /path/to/your/.eth-password remote-signer/data/.eth-password

cp remote-signer/.env.example remote-signer/.env
$EDITOR remote-signer/.env
```

Set `SIGNER_ETH_KEYSTORE_PATH=/data/keystore` (container path) and `SIGNER_ETH_ADDR` to your funded signer address. If `SIGNER_ETH_KEYSTORE_PATH` is unset, the entrypoint uses `/data/keystore` when that directory exists.

To change the host signing port from `8081`, use a Compose override file.

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

Collector pipeline config: [`openmeter-collector/collector.yaml`](openmeter-collector/collector.yaml).
The collector emits `billable_usd_micros` as an interim passthrough equal to
`network_fee_usd_micros` so the billable meter validates and accumulates. Phase-2
markup rules (network × pipeline/model multiplier) are not applied yet — until then
`billable_usd_micros == network_fee_usd_micros`.

### Identity contract (collector)

Upstream Kafka events carry `auth_id` as `client_id:usage_subject` (webhook → go-livepeer state → Kafka; unchanged).

The collector parses `auth_id` once (first-colon split) and emits normalized CloudEvents to Konnect/OpenMeter:

- `subject` = end user (`usage_subject`)
- `data.client_id` = tenant
- `data.usage_subject` = end user
- `data.auth_id` retained for compatibility; `data.external_user_id` mirrors `usage_subject` for meter `groupBy`

Demo API key defaults: `sk_demo_local_key` → `{publicClientId}:demo-user` (configured in `identity-webhook/.env` and `openmeter-collector/.env` `DEMO_API_KEYS`).

## Builder API

The Go Builder API runs inside `openmeter-collector` on port **8095**. Scalar docs: `http://localhost:8095/api/v1/docs`.

After Auth0 bootstrap (`auth0-provisioner/provision/bootstrap.sh`) and OpenMeter catalog bootstrap, configure `openmeter-collector/.env` with your Konnect PAT and **Management API M2M** credentials. Tenant domain, issuer, audience, and signer M2M client id/secret are loaded automatically from `auth0-provisioner/provision/.env.livepeer` (mounted into the container).

```bash
# Create user (M2M Basic) — use public client id in path
curl -sS -u "$AUTH0_SIGNER_M2M_CLIENT_ID:$AUTH0_SIGNER_M2M_CLIENT_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"externalUserId":"demo-user","email":"demo@example.com"}' \
  "http://localhost:8095/api/v1/apps/$DEMO_APP_AUTH0_PUBLIC_CLIENT_ID/users"

# Signer session (RFC 8693 token exchange)
curl -sS \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=urn:ietf:params:oauth:grant-type:token-exchange" \
  --data-urlencode "subject_token=sk_..." \
  --data-urlencode "subject_token_type=urn:ietf:params:oauth:token-type:access_token" \
  --data-urlencode "audience=livepeer-clearinghouse" \
  "http://localhost:8095/api/v1/apps/$DEMO_APP_AUTH0_PUBLIC_CLIENT_ID/oidc/token"
```
