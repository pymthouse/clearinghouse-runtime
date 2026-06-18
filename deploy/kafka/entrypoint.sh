#!/bin/sh
set -eu

ADVERTISED="${KAFKA_ADVERTISED_ADDR:-kafka:9092}"

# Base image ENTRYPOINT is /entrypoint.sh -> exec /usr/bin/rpk "$@".
# Shell-form CMD becomes /bin/sh -c ..., which rpk rejects (-c flag).
# Exec-form CMD works; entrypoint allows runtime KAFKA_ADVERTISED_ADDR override.
exec /usr/bin/rpk redpanda start \
  --kafka-addr "internal://0.0.0.0:9092" \
  --advertise-kafka-addr "internal://${ADVERTISED}" \
  --mode dev-container \
  --smp 1 \
  --memory 512M \
  --overprovisioned
