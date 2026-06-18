#!/bin/sh
set -eu

if [ -z "${PRICE_ORACLE_URL:-}" ]; then
  echo "entrypoint: PRICE_ORACLE_URL is required" >&2
  exit 1
fi

echo "entrypoint: fetching ETH/USD from ${PRICE_ORACLE_URL}" >&2
if ! curl -fsS "$PRICE_ORACLE_URL" >/tmp/price-oracle.json; then
  echo "entrypoint: price oracle unreachable at startup" >&2
  exit 1
fi

if ! grep -Eq '"price"|"usd"|"amount"' /tmp/price-oracle.json; then
  echo "entrypoint: price oracle response missing price field" >&2
  exit 1
fi

rm -f /tmp/price-oracle.json
exec benthos -c /config.yaml "$@"
