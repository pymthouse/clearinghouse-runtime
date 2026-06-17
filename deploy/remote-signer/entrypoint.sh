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

ARGS="-remoteSigner"
ARGS="$ARGS -network=${SIGNER_NETWORK}"
ARGS="$ARGS -httpAddr=0.0.0.0:${SIGNER_PORT}"
# CLI/admin port: loopback only — never published, no Apache DMZ in front of this container.
ARGS="$ARGS -cliAddr=127.0.0.1:4935"
ARGS="$ARGS -ethUrl=${ETH_RPC_URL}"
ARGS="$ARGS -ethPassword=/data/.eth-password"
ARGS="$ARGS -datadir=/data"
ARGS="$ARGS -v=99"

if [ -n "${SIGNER_ETH_ADDR:-}" ]; then
  ARGS="$ARGS -ethAcctAddr=${SIGNER_ETH_ADDR}"
fi

ARGS="$ARGS -remoteSignerWebhookUrl=${REMOTE_SIGNER_WEBHOOK_URL}"
ARGS="$ARGS -remoteSignerWebhookHeaders=Authorization:Bearer ${WEBHOOK_SECRET}"

ARGS="$ARGS -monitor"
ARGS="$ARGS -kafkaBootstrapServers=${KAFKA_BROKERS}"
ARGS="$ARGS -kafkaGatewayTopic=${KAFKA_GATEWAY_TOPIC}"

if [ "${SIGNER_REMOTE_DISCOVERY:-0}" = "1" ] || [ "${SIGNER_REMOTE_DISCOVERY:-0}" = "true" ]; then
  ARGS="$ARGS -remoteDiscovery=true"
  if [ -n "${ORCH_WEBHOOK_URL:-}" ]; then
    ARGS="$ARGS -orchWebhookUrl=${ORCH_WEBHOOK_URL}"
  fi
  if [ -n "${LIVE_AI_CAP_REPORT_INTERVAL:-}" ]; then
    ARGS="$ARGS -liveAICapReportInterval=${LIVE_AI_CAP_REPORT_INTERVAL}"
  fi
fi

echo "entrypoint: starting livepeer $ARGS" >&2
exec /usr/local/bin/livepeer $ARGS
