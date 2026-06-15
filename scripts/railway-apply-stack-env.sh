#!/usr/bin/env bash
# Apply clearinghouse stack env vars to Railway services.
# Used by CI (secrets → env) and locally.
#
#   export RAILWAY_API_TOKEN=...   # Account → Tokens (best for GitHub Actions)
#   # or export RAILWAY_TOKEN=...  # Project → Settings → Tokens
#   export RAILWAY_PROJECT_ID=...
#   export RAILWAY_ENVIRONMENT=production
#   export OPENMETER_URL=...
#   export OPENMETER_API_KEY=...
#   export WEBHOOK_SECRET=...
#   export REMOTE_SIGNER_WEBHOOK_URL=...
#   bash scripts/railway-apply-stack-env.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=lib/railway-auth.sh
source "$ROOT/scripts/lib/railway-auth.sh"

ENV="${RAILWAY_ENVIRONMENT:-production}"
PE_FLAGS="$(railway_pe_flags "$ENV")"

if ! command -v railway >/dev/null 2>&1; then
  echo "Install Railway CLI: npm install -g @railway/cli" >&2
  exit 1
fi

: "${OPENMETER_URL:?OPENMETER_URL is required}"
: "${OPENMETER_API_KEY:?OPENMETER_API_KEY is required}"
: "${WEBHOOK_SECRET:?WEBHOOK_SECRET is required}"
: "${REMOTE_SIGNER_WEBHOOK_URL:?REMOTE_SIGNER_WEBHOOK_URL is required}"

railway_export_auth || exit 1

set_kv() {
  local service="$1"
  shift
  # shellcheck disable=SC2086
  railway_retry railway variable set "$@" --service "$service" $PE_FLAGS --skip-deploys >/dev/null
  echo "  $service: set $# variable(s)"
}

KAFKA_GATEWAY_TOPIC="${KAFKA_GATEWAY_TOPIC:-livepeer-gateway-events}"
ETH_USD_PRICE="${ETH_USD_PRICE:-3500}"
OPENMETER_INGEST_URL="${OPENMETER_INGEST_URL:-${OPENMETER_URL}/events}"
SIGNER_NETWORK="${SIGNER_NETWORK:-arbitrum-one-mainnet}"
ETH_RPC_URL="${ETH_RPC_URL:-https://arb1.arbitrum.io/rpc}"

echo "Applying stack env to Railway environment: $ENV (project $(railway_default_project_id))"

# Warm up API connectivity before bulk writes.
# shellcheck disable=SC2086
railway_retry railway variables --service kafka $PE_FLAGS >/dev/null
echo "Railway API reachable."

# Kafka: override advertised address for Railway private networking.
set_kv kafka \
  "KAFKA_ADVERTISED_ADDR=kafka.railway.internal:9092"

# OpenMeter collector.
set_kv openmeter-collector \
  "KAFKA_BROKERS=kafka.railway.internal:9092" \
  "KAFKA_GATEWAY_TOPIC=${KAFKA_GATEWAY_TOPIC}" \
  "OPENMETER_URL=${OPENMETER_URL}" \
  "OPENMETER_INGEST_URL=${OPENMETER_INGEST_URL}" \
  "OPENMETER_API_KEY=${OPENMETER_API_KEY}" \
  "ETH_USD_PRICE=${ETH_USD_PRICE}"

# Remote signer (direct go-livepeer, no DMZ).
local_signer_args=(
  "SIGNER_NETWORK=${SIGNER_NETWORK}"
  "ETH_RPC_URL=${ETH_RPC_URL}"
  "KAFKA_BROKERS=kafka.railway.internal:9092"
  "KAFKA_GATEWAY_TOPIC=${KAFKA_GATEWAY_TOPIC}"
  "REMOTE_SIGNER_WEBHOOK_URL=${REMOTE_SIGNER_WEBHOOK_URL}"
  "WEBHOOK_SECRET=${WEBHOOK_SECRET}"
)
if [[ -n "${SIGNER_ETH_ADDR:-}" ]]; then
  local_signer_args+=("SIGNER_ETH_ADDR=${SIGNER_ETH_ADDR}")
fi

set_kv remote-signer "${local_signer_args[@]}"

echo "Done. Run bash scripts/railway-deploy-stack.sh $ENV to deploy."
