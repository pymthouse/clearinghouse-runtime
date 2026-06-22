#!/bin/sh
set -eu

SIGNER_NETWORK="${SIGNER_NETWORK:-arbitrum-one-mainnet}"
SIGNER_PORT="${SIGNER_PORT:-8081}"
ETH_RPC_URL="${ETH_RPC_URL:-https://arb1.arbitrum.io/rpc}"
KAFKA_BROKERS="${KAFKA_BROKERS:-kafka:9092}"
KAFKA_GATEWAY_TOPIC="${KAFKA_GATEWAY_TOPIC:-livepeer-gateway-events}"

if [ -z "${REMOTE_SIGNER_WEBHOOK_URL:-}" ]; then
  echo "entrypoint: REMOTE_SIGNER_WEBHOOK_URL is required (identity webhook URL)" >&2
  exit 1
fi
if [ -z "${WEBHOOK_SECRET:-}" ]; then
  echo "entrypoint: WEBHOOK_SECRET is required" >&2
  exit 1
fi

if [ ! -f /data/.eth-password ]; then
  echo "" >/data/.eth-password
fi

set -- \
  -remoteSigner \
  "-network=${SIGNER_NETWORK}" \
  "-httpAddr=0.0.0.0:${SIGNER_PORT}" \
  "-cliAddr=127.0.0.1:4935" \
  "-ethUrl=${ETH_RPC_URL}" \
  "-ethPassword=/data/.eth-password" \
  "-datadir=/data" \
  -v=99 \
  "-remoteSignerWebhookUrl=${REMOTE_SIGNER_WEBHOOK_URL}" \
  "-remoteSignerWebhookHeaders=Authorization:Bearer ${WEBHOOK_SECRET}" \
  -monitor \
  "-kafkaBootstrapServers=${KAFKA_BROKERS}" \
  "-kafkaGatewayTopic=${KAFKA_GATEWAY_TOPIC}"

if [ -n "${SIGNER_ETH_ADDR:-}" ]; then
  set -- "$@" "-ethAcctAddr=${SIGNER_ETH_ADDR}"
fi

if [ "${SIGNER_REMOTE_DISCOVERY:-0}" = "1" ] || [ "${SIGNER_REMOTE_DISCOVERY:-0}" = "true" ]; then
  set -- "$@" -remoteDiscovery=true
  if [ -n "${ORCH_WEBHOOK_URL:-}" ]; then
    set -- "$@" "-orchWebhookUrl=${ORCH_WEBHOOK_URL}"
  fi
  if [ -n "${LIVE_AI_CAP_REPORT_INTERVAL:-}" ]; then
    set -- "$@" "-liveAICapReportInterval=${LIVE_AI_CAP_REPORT_INTERVAL}"
  fi
fi

echo "entrypoint: starting livepeer remote-signer on 0.0.0.0:${SIGNER_PORT}" >&2
exec /usr/local/bin/livepeer "$@"
